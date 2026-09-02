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
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"path/filepath"
)

// Result is what a command left behind.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Combined returns stdout followed by stderr, which is what CTP looked at when it
// ran a script with 2>&1.
func (r Result) Combined() string { return r.Stdout + r.Stderr }

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
const Shell = "bash"

func (l *Local) Run(ctx context.Context, script string) (Result, error) {
	f, err := os.CreateTemp("", ".testkit-exec-*.sh")
	if err != nil {
		return Result{}, fmt.Errorf("script file: %w", err)
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err := f.WriteString(script + "\n"); err != nil {
		f.Close()
		return Result{}, fmt.Errorf("script file: %w", err)
	}
	if err := f.Close(); err != nil {
		return Result{}, fmt.Errorf("script file: %w", err)
	}

	cmd := osexec.CommandContext(ctx, Shell, name)
	cmd.Dir = l.Dir
	cmd.Env = l.Env

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
