// Package contain puts the run in namespaces of its own.
//
// The reset before every case matches process names as substrings across
// everything the user owns, so a run and anything else the same user is doing
// cannot share a machine. Containment is the answer, and the interesting part is
// that it changes what the reset *is*: inside a PID namespace `ps -e` is already
// exactly this run's work, and inside an IPC namespace so is `ipcs`. A filter
// that can be wrong becomes a property of where the process is.
//
// docs/concept/beyond-axis.md B-T2.
package contain

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// Env asks for containment. It is off unless set to "1", and that default is not
// only ADR-015's rule about axis B: the sweep selects nothing today, so turning
// this on is the first time the reset will actually kill anything.
const Env = "TESTKIT_CONTAIN"

// insideEnv marks the re-executed process so it does not try again.
const insideEnv = "TESTKIT_CONTAINED"

// ShellEnv names an interpreter to bind over /bin/sh inside the namespace. Empty
// leaves /bin/sh alone, which is what a QA machine wants.
const ShellEnv = "TESTKIT_CONTAIN_SH"

// Wanted reports whether containment was asked for.
func Wanted() bool { return os.Getenv(Env) == "1" }

// Active reports whether this process is the contained one.
func Active() bool { return os.Getenv(insideEnv) == "1" }

// Enter re-executes this program inside new user, mount, PID and IPC namespaces
// and returns the child's exit code, or -1 when there was nothing to do.
//
// The uid maps to 0, and that is forced rather than chosen. An unprivileged
// process gets one mapping; mapping the uid to itself keeps $USER meaningful but
// execve then drops the capabilities, so /proc cannot be remounted and ps still
// shows the whole machine. Mapping it to 0 keeps the capabilities and costs
// `ps -u $USER`, which is why the reset stops selecting by user -- see
// KillScript. $USER itself is left alone: cases read it.
func Enter() int {
	if !Wanted() || Active() {
		return -1
	}
	self, err := os.Executable()
	if err != nil {
		fail("cannot find own path: %v", err)
	}
	cmd := exec.Command(self, os.Args[1:]...)
	cmd.Env = append(os.Environ(), insideEnv+"=1")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUSER | syscall.CLONE_NEWNS |
			syscall.CLONE_NEWPID | syscall.CLONE_NEWIPC,
		UidMappings:                []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getuid(), Size: 1}},
		GidMappings:                []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getgid(), Size: 1}},
		GidMappingsEnableSetgroups: false,
	}
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if ok := asExit(err, &ee); ok {
			return ee.ExitCode()
		}
		fail("cannot contain the run: %v", err)
	}
	return 0
}

// Setup is what the contained process does before anything else: give itself a
// private /proc, so that `ps` reports the namespace rather than the machine.
//
// Without it the namespace exists and nothing can see it, which is the failure
// the comparison wrapper had for a week.
func Setup() error {
	if !Active() {
		return nil
	}
	// Without MS_PRIVATE the remount propagates back to the machine's /proc.
	if err := syscall.Mount("", "/", "", syscall.MS_REC|syscall.MS_PRIVATE, ""); err != nil {
		return fmt.Errorf("make mounts private: %w", err)
	}
	if err := syscall.Mount("proc", "/proc", "proc", 0, ""); err != nil {
		return fmt.Errorf("mount /proc: %w", err)
	}

	// ShellEnv exists for machines this suite was never meant to run on. The
	// shell task has always required a bash-compatible /bin/sh -- CTP's own
	// unittest plug-in is written with `function init { }` and sources files --
	// and on distributions where /bin/sh is dash every case dies on init.sh's
	// first line. A QA machine does not need this; a developer's laptop does,
	// and containment is what took away the ability to arrange it outside.
	if sh := os.Getenv(ShellEnv); sh != "" {
		if err := syscall.Mount(sh, "/bin/sh", "", syscall.MS_BIND, ""); err != nil {
			return fmt.Errorf("bind %s over /bin/sh: %w", sh, err)
		}
	}
	return nil
}

// Reap collects orphans for as long as the process runs.
//
// PID 1 of a namespace inherits every orphan in it, and a daemon that
// double-forks becomes one. Left unreaped it stays a zombie its parent never
// learns about -- which is how a `cubrid server stop` was seen polling for a
// server that had already exited, for twenty-three minutes.
func Reap() {
	if !Active() {
		return
	}
	ch := make(chan os.Signal, 8)
	signal.Notify(ch, syscall.SIGCHLD)
	go func() {
		for range ch {
			for {
				var ws syscall.WaitStatus
				pid, err := syscall.Wait4(-1, &ws, syscall.WNOHANG, nil)
				if pid <= 0 || err != nil {
					break
				}
			}
		}
	}()
}

func asExit(err error, target **exec.ExitError) bool {
	e, ok := err.(*exec.ExitError)
	if ok {
		*target = e
	}
	return ok
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "testkit: "+format+"\n", a...)
	os.Exit(1)
}
