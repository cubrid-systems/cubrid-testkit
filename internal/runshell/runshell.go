// Package runshell is the other CLI: one case, run over and over until it fails.
//
// It is not a smaller version of the shell task. The shell task runs a corpus
// once and reports what happened; this runs a single case in a loop and stops
// the moment it does not pass. That is the tool for reproducing something
// intermittent, and it is what a developer reaches for after the suite has
// already told them which case is unreliable.
//
// CTP shipped it as shell/init_path/run_shell.sh, a launcher for RunShellMain.
// Of its fourteen options, six are test execution and are here; seven are QA
// operations and are not (docs/concept/external-surface-freeze.md §1-4); one is
// dead in CTP itself.
package runshell

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
)

// Options are the six axis-T options of run_shell.sh.
type Options struct {
	// Loop runs the case repeatedly until it fails. Without it the case runs once.
	Loop bool
	// MaxLoop and MaxTime bound the loop. Zero means unbounded, which is what
	// CTP's Integer.MAX_VALUE default amounted to.
	MaxLoop int
	MaxTime time.Duration
	// ExtendScript is sourced and asked to `verify <dir> <name>.result` instead of
	// the result file being read directly.
	ExtendScript string
	// PromptContinue is CTP's --prompt-continue: nil asks, true and false answer
	// for the operator.
	PromptContinue *bool
}

// Outcome says how the loop ended. The strings are CTP's, and they are what a
// developer greps the terminal for.
type Outcome string

const (
	OutcomeStop    Outcome = "STOP"      // ran out of loops, time, or was told to stop
	OutcomeNOK     Outcome = "QUIT(NOK)" // the case reported a failure
	OutcomeEmpty   Outcome = "QUIT(FAIL1)"
	OutcomeRuntime Outcome = "QUIT(FAIL2)"
)

// Result is what the run amounted to.
type Result struct {
	Outcome Outcome
	// Loops is how many attempts were made, including the failing one.
	Loops int
	// Info is the detail behind a non-STOP outcome: the result text, or an error.
	Info string
}

// Failed reports whether the loop ended because the case did.
func (r Result) Failed() bool { return r.Outcome != OutcomeStop }

// Case is the case a run is about.
type Case struct {
	// Dir is the cases/ directory the script lives in.
	Dir string
	// Name is the case name -- the directory above cases/, and the stem of both
	// the script and the result file.
	Name string
}

// Script and Result are the two files a case owns.
func (c Case) Script() string { return c.Name + ".sh" }
func (c Case) Result() string { return c.Name + ".result" }

// Locate resolves what the user pointed at into a case.
//
// The argument may be the case directory, its cases/ subdirectory, or a file
// inside either; an empty argument means the working directory. CTP accepted all
// of those and this does too, because a developer runs this from wherever they
// happen to be standing.
func Locate(arg string) (Case, error) {
	if strings.TrimSpace(arg) == "" {
		arg = "."
	}
	abs, err := filepath.Abs(arg)
	if err != nil {
		return Case{}, fmt.Errorf("Not found test case to execute.")
	}
	info, err := os.Stat(abs)
	if err != nil {
		return Case{}, fmt.Errorf("Not found test case to execute.")
	}
	if !info.IsDir() {
		abs = filepath.Dir(abs)
	}
	if filepath.Base(abs) != "cases" {
		nested := filepath.Join(abs, "cases")
		if info, err := os.Stat(nested); err != nil || !info.IsDir() {
			return Case{}, fmt.Errorf("Not found test case to execute.")
		}
		abs = nested
	}
	return Case{Dir: abs, Name: filepath.Base(filepath.Dir(abs))}, nil
}

// Meta is what the machine says about the build under test.
type Meta struct {
	Rel     string // the whole cubrid_rel line
	BuildID string
	Bits    string // "64" or "32" -- not "64bits"; this tool spells it shorter
	OS      string
	RelType string // "release" or "debug"
}

// relScript asks the installed engine what it is.
const relScript = "$CUBRID/bin/cubrid_rel 2>/dev/null"

// ReadMeta runs cubrid_rel and reads it.
//
// The build id is what lies between the first parentheses, which for
// "CUBRID 11.4.5 (11.4.5.1875-74d17e9) (64bit release build for Linux)" is
// 11.4.5.1875-74d17e9.
//
// Worth knowing: CTP had two build-id parsers and they disagreed. This one is
// right; CommonUtils.getBuildId cut at the next "-", ")" or "." after the
// version and ran away when a build had no commit suffix
// (docs/evidence/spec-corrections.md). The fix applied there made the other one
// agree with this, rather than inventing a third rule.
func ReadMeta(ctx context.Context, ch exec.Channel) (Meta, error) {
	res, err := ch.Run(ctx, relScript)
	if err != nil {
		return Meta{}, fmt.Errorf("Not found current $CUBRID.")
	}
	out := strings.TrimSpace(res.Output())
	if !strings.Contains(out, "CUBRID") {
		return Meta{}, fmt.Errorf("Not found current $CUBRID.")
	}

	m := Meta{Rel: out, Bits: "32"}
	if strings.Contains(out, "64bit") {
		m.Bits = "64"
	}
	lower := strings.ToLower(out)
	switch {
	case strings.Contains(lower, "linux"):
		m.OS = "linux"
	case strings.Contains(lower, "windows"):
		m.OS = "windows"
	default:
		return Meta{}, fmt.Errorf("Not linux or windows for CUBRID")
	}
	switch {
	case strings.Contains(lower, "release"):
		m.RelType = "release"
	case strings.Contains(lower, "debug"):
		m.RelType = "debug"
	default:
		return Meta{}, fmt.Errorf("Not debug or release for CUBRID")
	}

	open := strings.Index(out, "(")
	close := strings.Index(out, ")")
	if open == -1 || close < open {
		return Meta{}, fmt.Errorf("Unknown CUBRID version")
	}
	m.BuildID = strings.TrimSpace(out[open+1 : close])
	if m.BuildID == "" {
		return Meta{}, fmt.Errorf("Unknown CUBRID version")
	}
	return m, nil
}

// stopFile is checked between iterations. Touching it in the case directory ends
// the loop after the attempt in flight -- the only way to stop a running loop
// without killing it, and not something the specification had.
const stopFile = "STOP"

// Run is the loop.
type Run struct {
	Case    Case
	Options Options
	Channel exec.Channel
	Meta    Meta

	// Out is where the banner and the verdict go.
	Out io.Writer
	// In is where a prompt is answered from. Nil means no prompt can be answered,
	// which is the same as answering no.
	In io.Reader
}

// Go runs the case until it fails or the loop runs out.
func (r *Run) Go(ctx context.Context) Result {
	r.banner()

	started := time.Now()
	limit := r.Options.MaxLoop
	if !r.Options.Loop {
		limit = 1
	}

	for i := 0; limit == 0 || i < limit; i++ {
		fmt.Fprintf(r.Out, "LOOP: %d\n", i+1)

		if err := r.once(ctx); err != nil {
			out, info := classify(err)
			fmt.Fprintf(r.Out, "\nResult: %s\n%s\n\n", out, info)
			return Result{Outcome: out, Loops: i + 1, Info: info}
		}

		if r.Options.MaxTime > 0 && time.Since(started) >= r.Options.MaxTime {
			return Result{Outcome: OutcomeStop, Loops: i + 1}
		}
		if _, err := os.Stat(filepath.Join(r.Case.Dir, stopFile)); err == nil {
			fmt.Fprintf(r.Out, "Found %s; stopping.\n", stopFile)
			return Result{Outcome: OutcomeStop, Loops: i + 1}
		}
	}
	return Result{Outcome: OutcomeStop, Loops: limit}
}

// once is one attempt: run the case, then decide what it said.
func (r *Run) once(ctx context.Context) error {
	script := strings.Join([]string{
		"cd " + r.Case.Dir,
		"set -x ; sh " + r.Case.Script() + " 2>&1",
	}, "\n")
	res, err := r.Channel.Run(ctx, script)
	if err != nil {
		return &failure{outcome: OutcomeRuntime, info: err.Error()}
	}
	fmt.Fprint(r.Out, res.Output())

	if r.Options.ExtendScript != "" {
		return r.verifyThroughScript(ctx)
	}
	return r.verifyResultFile()
}

// verifyResultFile is the default: read the file the case wrote.
func (r *Run) verifyResultFile() error {
	body, err := os.ReadFile(filepath.Join(r.Case.Dir, r.Case.Result()))
	if err != nil {
		return &failure{outcome: OutcomeEmpty, info: err.Error()}
	}
	text := string(body)
	if strings.TrimSpace(text) == "" {
		return &failure{outcome: OutcomeEmpty, info: "Unknown. The result file is empty."}
	}
	if strings.Contains(text, "NOK") {
		return &failure{outcome: OutcomeNOK, info: text}
	}
	return nil
}

// verifyThroughScript hands the verdict to the operator's own script, which is
// what --extend-script is for: a case whose result cannot be judged by looking
// for NOK.
func (r *Run) verifyThroughScript(ctx context.Context) error {
	script := strings.Join([]string{
		"export PATH=.:$PATH",
		"source " + r.Options.ExtendScript,
		"verify " + r.Case.Dir + " " + r.Case.Result(),
	}, "\n")
	res, err := r.Channel.Run(ctx, script)
	if err != nil {
		return &failure{outcome: OutcomeRuntime, info: err.Error()}
	}
	out := res.Output()
	switch {
	case strings.Contains(out, "NOK"):
		return &failure{outcome: OutcomeNOK, info: out}
	case !strings.Contains(out, "OK"):
		// Neither word: the script did not answer, and an unanswered verify is not
		// a pass.
		return &failure{outcome: OutcomeEmpty, info: out}
	}
	return nil
}

// Continue asks whether to pick up where a previous run left off.
//
// --prompt-continue answers for the operator, which is what makes this usable
// from a script. Without it, and without somewhere to read from, the answer is
// no: a tool that blocks forever on a prompt nobody can see is worse than one
// that starts over.
func (r *Run) Continue() bool {
	if r.Options.PromptContinue != nil {
		return *r.Options.PromptContinue
	}
	if r.In == nil {
		return false
	}
	fmt.Fprintln(r.Out, "Found existing tests. Do you hope to continue previous tests? [Y/N]:")
	scanner := bufio.NewScanner(r.In)
	for scanner.Scan() {
		switch strings.TrimSpace(scanner.Text()) {
		case "Y":
			return true
		case "N":
			return false
		}
		fmt.Fprintln(r.Out, "Please input [Y/N]:")
	}
	return false
}

// banner is the parameter block CTP printed before the first loop.
//
// It lists the options this runner does not have, because it is a frozen surface
// and because a reader comparing two runs should see the same shape. They print
// the values they would have had if nobody set them, which is what an operator
// who does not set them already sees.
func (r *Run) banner() {
	fmt.Fprintln(r.Out, "====> start to test ")
	fmt.Fprintln(r.Out, "Test parameters: ")
	fmt.Fprintf(r.Out, "   testcase     :\t%s\n", filepath.Join(r.Case.Dir, r.Case.Script()))
	fmt.Fprintf(r.Out, "   update-build :\tfalse\n")

	loopDesc := ""
	if r.Options.Loop {
		loopDesc = fmt.Sprintf(" (%s loops, %s seconds)", bound(r.Options.MaxLoop), boundTime(r.Options.MaxTime))
	}
	fmt.Fprintf(r.Out, "   loop         :\t%v%s\n", r.Options.Loop, loopDesc)
	fmt.Fprintf(r.Out, "   enable-report:\tfalse\n")
	fmt.Fprintf(r.Out, "   report-cron  :\tnull\n")
	fmt.Fprintf(r.Out, "   mailto       :\tnull\n")
	fmt.Fprintf(r.Out, "   mailcc       :\tnull\n")
	fmt.Fprintf(r.Out, "   issue        :\tnull\n")
	fmt.Fprintf(r.Out, "   extend-script:\t%s\n", orNull(r.Options.ExtendScript))
	// CTP printed this too, and it was always null: the option's registration is
	// commented out while its read is not (freeze §11-20).
	fmt.Fprintf(r.Out, "   config       :\tnull\n")
	fmt.Fprintf(r.Out, "   env:HOME     :\t%s\n", os.Getenv("HOME"))
	fmt.Fprintf(r.Out, "   env:CTP_HOME :\t%s\n", os.Getenv("CTP_HOME"))
	fmt.Fprintf(r.Out, "   env:CUBRID   :\t%s (%s, %sbits, %s, %s)\n",
		os.Getenv("CUBRID"), r.Meta.BuildID, r.Meta.Bits, r.Meta.OS, r.Meta.RelType)
	fmt.Fprintln(r.Out)
}

func bound(n int) string {
	if n <= 0 {
		return "max"
	}
	return fmt.Sprint(n)
}

func boundTime(d time.Duration) string {
	if d <= 0 {
		return "max"
	}
	return fmt.Sprint(int(d.Seconds()))
}

func orNull(s string) string {
	if s == "" {
		return "null"
	}
	return s
}

// failure carries an outcome out of the attempt that produced it.
type failure struct {
	outcome Outcome
	info    string
}

func (f *failure) Error() string { return f.info }

func classify(err error) (Outcome, string) {
	if f, ok := err.(*failure); ok {
		return f.outcome, f.info
	}
	return OutcomeRuntime, err.Error()
}
