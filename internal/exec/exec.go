// Package exec runs commands somewhere.
//
// "Somewhere" is the only thing that varies between a local run and a run against
// a remote machine, so it is the only thing the interface names. A Format or a
// Runner should never know which one it has -- that is what lets the same case
// logic drive a local unittest and a case shipped over SSH.
//
// docs/design/contracts.md C3.
package exec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// cancelGrace is how long Wait will keep reading a cancelled command's output
// before giving up on it. Something outside the process group holding the pipe
// is a bug worth a few seconds, not a reason to block a whole run.
const cancelGrace = 5 * time.Second

// Result is what a command left behind.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Output is what CTP's callers saw, and it is standard output alone.
//
// Both of CTP's channels discard standard error. The remote one never reads it:
// SSHConnect takes exec.getInputStream() and nothing else. The local one appears
// to keep it -- it concatenates stdout and stderr -- and then truncates the
// result at the completion marker, which sits at the end of stdout, so everything
// stderr contributed is cut away again.
//
// The effect is visible in a worker log: a script whose commands write to stderr
// leaves no trace of them unless the script redirects internally, which is why a
// case is run as "sh <case>.sh 2>&1" and the reset script is not.
//
// Stderr is still captured, because a channel that throws away the explanation of
// its own failure is no use. It is just not what the frozen logs contain.
func (r Result) Output() string { return r.Stdout }

// Channel runs commands and moves files.
type Channel interface {
	// Run executes a shell script and waits. A non-zero exit is reported in the
	// Result, not as an error: a failing test case is data, not a malfunction.
	// The error return is for the channel itself failing.
	Run(ctx context.Context, script string) (Result, error)

	// Put copies a local path to the far side; Get copies the other way.
	Put(ctx context.Context, local, remote string) error
	Get(ctx context.Context, remote, local string) error

	// Describe names this channel for logs and markers.
	Describe() string

	Close() error
}

// Local runs on this machine. unittest is local by nature, and it is also how the
// suite is exercised without a remote host.
type Local struct {
	// Dir is the working directory. Empty means the process's own.
	Dir string
	// Env replaces the environment when non-nil, and extends it otherwise.
	Env []string
	// ScriptDir is where the script file is written. Empty means the process's
	// own temporary directory, which is what a local run has always used. A
	// channel into a namespace has to name somewhere else: a slot gets a /tmp of
	// its own, and a script written into this process's /tmp is not in the one
	// the command will read.
	ScriptDir string
	// SourceProfile prepends Profile, as CTP's remote path always did and its
	// local paths did not agree about: the shell task reached even a local machine
	// through SSHConnect and got the profile, while unittest called LocalInvoker
	// directly and did not. Both are reproduced rather than unified.
	SourceProfile bool
}

// NewLocal returns a channel that runs here.
func NewLocal(dir string, env ...string) *Local {
	return &Local{Dir: dir, Env: append(os.Environ(), env...)}
}

func (l *Local) Describe() string { return "local" }

// Shell is the interpreter local scripts run under.
//
// CTP writes the script to a temporary file and runs `sh <file> 2>&1`
// (common/LocalInvoker.exec). We keep the temporary file -- it sidesteps every
// quoting question that `-c` raises -- but we say bash rather than sh, and the
// reason is in CTP's own assets.
//
// shell/local/unittest.sh, the one unittest plug-in CTP ships, is written with
// `function init { ... }`. That is bash and ksh syntax; dash rejects it. The
// plug-in protocol also sources the file, and `source` is likewise not POSIX. So
// the contract has always required a bash-compatible /bin/sh, and it has held
// because CTP runs on distributions where /bin/sh is bash.
//
// Saying bash outright reproduces what happens on those machines and additionally
// works where /bin/sh is dash, which is where CTP fails with "source: not found".
// No frozen output changes; a deviation from the literal command line is recorded
// in the freeze spec.
// Profile is what runs before anything else, on both channels.
//
// An ssh exec session is neither a login nor an interactive shell, so it reads
// no profile: none of $CUBRID, $JAVA_HOME, $CTP_HOME or the PATH a QA machine is
// set up with would be there. CTP put this line in front of every script it
// sent, and it is the reason a case can assume any of them.
//
// The dance around CTP_HOME is deliberate. The profile on a QA machine usually
// sets it too, and the caller's value has to win -- so it is saved first and put
// back afterwards, but only when the caller actually had one.
//
// It runs *before* the frame opens, so whatever a profile prints on the way past
// is discarded rather than mixed into a case's output.
const Profile = `pri_ctp_home=$CTP_HOME; if  [ -f ~/.bash_profile ]; then . ~/.bash_profile; fi; if [ "$pri_ctp_home" != "" ];then export CTP_HOME=${pri_ctp_home}; fi; `

const Shell = "bash"

func (l *Local) Run(ctx context.Context, script string) (Result, error) {
	return l.RunWith(ctx, script, nil)
}

// RunWith is Run with the command line handed to wrap first, so a caller can
// put the script somewhere other than this process's own namespaces without
// this package learning what a namespace is. nil means run it here.
func (l *Local) RunWith(ctx context.Context, script string, wrap func(argv ...string) []string) (Result, error) {
	f, err := os.CreateTemp(l.ScriptDir, ".testkit-exec-*.sh")
	if err != nil {
		return Result{}, fmt.Errorf("script file: %w", err)
	}
	name := f.Name()
	defer os.Remove(name)
	body := script + "\n"
	if l.SourceProfile {
		body = Profile + "\n" + body
	}
	if _, err := f.WriteString(body); err != nil {
		f.Close()
		return Result{}, fmt.Errorf("script file: %w", err)
	}
	if err := f.Close(); err != nil {
		return Result{}, fmt.Errorf("script file: %w", err)
	}

	argv := []string{Shell, name}
	if wrap != nil {
		argv = wrap(argv...)
	}
	cmd := osexec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = l.Dir
	cmd.Env = l.Env

	// A case is a tree, not a process, so cancellation has to reach the tree.
	// Without Setpgid it signals the shell alone, the descendants survive holding
	// the pipe, and Wait never returns. WaitDelay bounds the case where one of
	// them escapes the group anyway.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = cancelGrace

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	res := Result{Stdout: stdout.String(), Stderr: stderr.String()}
	if err == nil {
		return res, nil
	}
	var exitErr *osexec.ExitError
	if ok := asExit(err, &exitErr); ok {
		res.ExitCode = exitErr.ExitCode()
		return res, nil
	}
	// A command that starts a daemon leaves the daemon holding the pipe.
	//
	// cub_master, cub_broker and cub_server all outlive the shell that started
	// them and inherit its stdout, so the shell exits, its output is complete,
	// and Wait still blocks -- which is what WaitDelay is here to bound. But the
	// error it then returns is not a failed case: the process exited on its own
	// and ProcessState has its status. Reporting it as a runtime error threw the
	// verdict away and failed the case whatever it had done.
	//
	// Measured: _36_cub_master/bug_xdbms40 and _40_broker/itrack03 both failed
	// with "WaitDelay expired before I/O complete" and no verdict at all.
	//
	// A cancelled context is the other half of WaitDelay's job and still an
	// error -- that is a case that would not stop, and Exited() is false for a
	// process the cancel killed.
	if errors.Is(err, osexec.ErrWaitDelay) && ctx.Err() == nil &&
		cmd.ProcessState != nil && cmd.ProcessState.Exited() {
		res.ExitCode = cmd.ProcessState.ExitCode()
		return res, nil
	}
	return res, fmt.Errorf("local run: %w", err)
}

func (l *Local) Put(ctx context.Context, local, remote string) error { return copyFile(local, remote) }
func (l *Local) Get(ctx context.Context, remote, local string) error { return copyFile(remote, local) }
func (l *Local) Close() error                                        { return nil }

func copyFile(from, to string) error {
	if from == to {
		return nil
	}
	src, err := os.Open(from)
	if err != nil {
		return err
	}
	defer src.Close()
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	dst, err := os.Create(to)
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = io.Copy(dst, src)
	return err
}

func asExit(err error, target **osexec.ExitError) bool {
	e, ok := err.(*osexec.ExitError)
	if ok {
		*target = e
	}
	return ok
}
