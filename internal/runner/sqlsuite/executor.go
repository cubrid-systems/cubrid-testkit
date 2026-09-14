package sqlsuite

import (
	"bufio"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
)

// Executor runs one case and hands back what CQT would write to its .result.
//
// It is the half of CQT that ADR-016 keeps behind an interface: parsing a case
// into statements, sending them, and rendering what comes back. The other half
// -- which cases, which answer, the verdict, every file and line about it -- is
// the runner's, so that a second executor changes how a case is run and nothing
// about how it is judged.
type Executor interface {
	// Run executes one case, returning its rendering and the milliseconds the
	// executor measured for it.
	Run(ctx context.Context, caseFile string) (rendered []byte, ms int64, err error)
	Close() error
}

// errExecutorGone is an executor that stopped answering: every case after it
// would fail the same way, so it ends the run rather than the case.
var errExecutorGone = errors.New("the sql executor is gone")

// place is somewhere to run things: a slot, or the machine when the run is
// neither contained nor parallel.
type place interface {
	Channel() exec.Channel
	Command(ctx context.Context, dir string, argv ...string) *osexec.Cmd
}

// machine is the place a run has when it has no slots: this process's own
// namespaces, as CTP ran.
type machine struct{}

func (machine) Channel() exec.Channel { return exec.NewLocal("") }
func (machine) Command(ctx context.Context, dir string, argv ...string) *osexec.Cmd {
	return exec.Command(ctx, dir, nil, argv...)
}

//go:embed TestkitExecutor.java
var executorSource []byte

// compileExecutor compiles the embedded executor against the CQT of the CTP
// this run uses, and returns the directory the class is in.
//
// Compiled per run rather than shipped as a class: it calls CQT's private
// methods by name, and against the jar it runs with is the only place a
// mismatch can show up as an error rather than as different output.
func compileExecutor(ctx context.Context, ctpHome string) (string, error) {
	dir, err := os.MkdirTemp("", "testkit-sqlexec-")
	if err != nil {
		return "", err
	}
	src := filepath.Join(dir, "TestkitExecutor.java")
	if err := os.WriteFile(src, executorSource, 0o644); err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	javac := filepath.Join(os.Getenv("JAVA_HOME"), "bin", "javac")
	cmd := osexec.CommandContext(ctx, javac, "-nowarn", "-d", dir,
		"-cp", filepath.Join(ctpHome, "sql", "lib", "*"), src)
	if out, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(dir)
		return "", fmt.Errorf("compile the sql executor against %s: %v: %s",
			filepath.Join(ctpHome, "sql", "lib"), err, strings.TrimSpace(string(out)))
	}
	return dir, nil
}

// jdbc is the executor that is CQT: its parser, its connection, its renderer,
// called one case at a time (ADR-016).
type jdbc struct {
	cmd   *osexec.Cmd
	in    io.WriteCloser
	out   *bufio.Reader
	cases int // what CQT's own discovery found
	// startup is what CQT printed while it started, which CQT's own run prints
	// after "Result Root Dir:".
	startup []string

	mu   sync.Mutex
	tail []string // the last lines of its standard error, for when it dies
	done chan struct{}
}

// startJDBC starts the executor in p and waits until it is ready. resultDir is
// the run's result directory, which every executor is told so that CQT's case
// records point where the runner writes.
//
// It is launched the way run.sh launches CQT -- do_configure's environment, the
// working directory sql/lib, every jar there on the classpath after
// $CLASSPATH, the same JVM options and the same arguments -- because CQT reads
// some of that (the working directory, the environment) and the rest is the
// honest way to say "this is CQT". stderr is CQT's own output once it has
// started; each line goes to say.
func startJDBC(ctx context.Context, p place, s *settings, e engine, classDir, resultDir, jvm string, say func(string)) (*jdbc, error) {
	script := s.header(e, "") + stagesScript + `
sql_env
CPLIB=$CTP_HOME/sql/lib
CPCLASSES=""
cd $CPLIB
for clz in $(ls *.jar);do
     CPCLASSES=${CPCLASSES}:$CPLIB/$clz
done
exec "$JAVA_HOME/bin/java" ` + jvm + ` -Dtestkit.result_dir=` + shQuote(resultDir) + ` -classpath "${CLASSPATH}:${CPCLASSES}:` + classDir + `" TestkitExecutor ` +
		strings.Join([]string{shQuote(s.category), shQuote(s.alias), shQuote(e.bits), shQuote(s.jdbcConfig), shQuote(s.cqtArg())}, " ") +
		// The protocol on fd 3, and the JVM's own standard output with its
		// standard error: see TestkitExecutor.
		" 3>&1 1>&2\n"

	return attach(p.Command(ctx, "", "bash", "-c", script), say)
}

// javaOptions is how the executor's JVM is started: CTP's options when the run
// has one place, as run.sh starts CQT.
//
// Slots share the machine, and CTP's sizing is for a JVM that has it to
// itself. Measured with eight slots on sql: every executor reached 2 GB before
// its first case, 16 GB between them, and each parallel collector takes a
// thread per core. So a slotted executor starts at 256 MB, stops at 1 GB --
// the largest answer in the corpus is 8.5 MB -- and collects with two threads.
// None of it reaches a rendering, which the comparison of one slot with many
// checks.
//
// And with slots, testkit.follow_order: each case starts with CQT's
// server-message flag where CTP's single run would have left it. A serial run
// does not need it -- its flag already is CTP's -- and must not have it: the
// prediction assumes every hint takes effect, and a case whose connection died
// before its hint is a case where that is not so.
func javaOptions(places int) string {
	if places == 1 {
		return "-Xms1024m -XX:+UseParallelGC"
	}
	return "-Xms256m -Xmx1g -XX:+UseParallelGC -XX:ParallelGCThreads=2 -Dtestkit.follow_order=true"
}

// attach starts cmd and speaks the executor's protocol with it until it says
// it is ready.
func attach(cmd *osexec.Cmd, say func(string)) (*jdbc, error) {
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start the sql executor: %w", err)
	}
	x := &jdbc{cmd: cmd, in: in, out: bufio.NewReaderSize(stdout, 1<<20), done: make(chan struct{})}
	go func() {
		defer close(x.done)
		sc := bufio.NewScanner(stderr)
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for sc.Scan() {
			line := sc.Text()
			x.mu.Lock()
			x.tail = append(x.tail, line)
			if len(x.tail) > 20 {
				x.tail = x.tail[1:]
			}
			x.mu.Unlock()
			if say != nil {
				say(line)
			}
		}
		// A line too long for the scanner ends it; the rest is still read, or
		// the JVM fills the pipe, blocks on its next write and never answers.
		io.Copy(io.Discard, stderr)
	}()

	line, err := x.out.ReadString('\n')
	if err != nil {
		x.Close()
		return nil, fmt.Errorf("the sql executor did not start: %w%s", err, x.why())
	}
	line = strings.TrimSpace(line)
	rest, ok := strings.CutPrefix(line, "READY ")
	f := strings.Fields(rest)
	if !ok || len(f) != 2 {
		x.Close()
		return nil, fmt.Errorf("the sql executor did not start: %s%s", line, x.why())
	}
	size, err1 := strconv.Atoi(f[1])
	x.cases, err = strconv.Atoi(f[0])
	if err != nil || err1 != nil || size < 0 {
		x.Close()
		return nil, fmt.Errorf("the sql executor said %q", line)
	}
	said := make([]byte, size)
	if _, err := io.ReadFull(x.out, said); err != nil {
		x.Close()
		return nil, fmt.Errorf("the sql executor did not start: %w%s", err, x.why())
	}
	if len(said) > 0 {
		x.startup = strings.Split(strings.TrimSuffix(string(said), "\n"), "\n")
	}
	return x, nil
}

// Run implements Executor.
func (x *jdbc) Run(ctx context.Context, caseFile string) ([]byte, int64, error) {
	return x.ask("X", caseFile)
}

// Failure is a failed case's text for the JUnit report, built by CQT's own
// JunitXmlWriter from the case, its answer and the .result the runner copied
// into the result tree. "" when CQT would have written none.
func (x *jdbc) Failure(caseFile string) string {
	text, _, err := x.ask("F", caseFile)
	if err != nil {
		return ""
	}
	return string(text)
}

func (x *jdbc) ask(verb, caseFile string) ([]byte, int64, error) {
	if strings.ContainsAny(caseFile, "\r\n") {
		return nil, 0, fmt.Errorf("a case path cannot hold a line break: %q", caseFile)
	}
	if _, err := io.WriteString(x.in, verb+" "+caseFile+"\n"); err != nil {
		return nil, 0, fmt.Errorf("%w: %v%s", errExecutorGone, err, x.why())
	}
	line, err := x.out.ReadString('\n')
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %v%s", errExecutorGone, err, x.why())
	}
	line = strings.TrimRight(line, "\n")
	if msg, ok := strings.CutPrefix(line, "E "); ok {
		return nil, 0, fmt.Errorf("the sql executor: %s", msg)
	}
	f := strings.Fields(strings.TrimPrefix(line, "R "))
	if !strings.HasPrefix(line, "R ") || len(f) != 2 {
		// Neither a reply nor an error: the two sides no longer agree on where a
		// reply starts, and every answer after this one would belong to the case
		// before it. That is the end of this executor, not a failed case.
		return nil, 0, fmt.Errorf("%w: it said %q where a reply belongs", errExecutorGone, line)
	}
	size, err1 := strconv.Atoi(f[0])
	ms, err2 := strconv.ParseInt(f[1], 10, 64)
	if err1 != nil || err2 != nil || size < 0 {
		return nil, 0, fmt.Errorf("the sql executor said %q", line)
	}
	body := make([]byte, size)
	if _, err := io.ReadFull(x.out, body); err != nil {
		return nil, 0, fmt.Errorf("%w mid-result: %v%s", errExecutorGone, err, x.why())
	}
	return body, ms, nil
}

// Close ends the executor: end of input is its signal to exit, and one that does
// not is killed. Its standard error is read to the end first, which is what
// Wait requires of a pipe and what keeps its last words.
func (x *jdbc) Close() error {
	x.in.Close()
	select {
	case <-x.done:
	case <-time.After(30 * time.Second):
		syscall.Kill(-x.cmd.Process.Pid, syscall.SIGKILL)
		<-x.done
	}
	return x.cmd.Wait()
}

// why is the end of what the executor said on its way out, for an error.
func (x *jdbc) why() string {
	// Wait briefly for the stderr reader: when the process has died, the last
	// thing it wrote is usually the reason.
	select {
	case <-x.done:
	case <-time.After(2 * time.Second):
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	if len(x.tail) == 0 {
		return ""
	}
	return "\n  " + strings.Join(x.tail, "\n  ")
}
