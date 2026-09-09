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
	"path/filepath"
	"strings"
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
	cmd.Env = append(inside(os.Environ()), insideEnv+"=1")
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
			// Exited() before ExitCode(). A process killed by a signal has no
			// exit code and ExitCode() answers -1 for it -- the same -1 this
			// function uses for "there was nothing to contain". The caller reads
			// that as "carry on", and carries on *uncontained*: the reset then
			// empties the real $CUBRID/conf and the process sweep selects across
			// the whole machine. An OOM kill or a stray SIGKILL is enough.
			if ee.ProcessState != nil && !ee.ProcessState.Exited() {
				fail("the contained run was killed: %v", ee.ProcessState)
			}
			return ee.ExitCode()
		}
		fail("cannot contain the run: %v", err)
	}
	return 0
}

// inside is the environment as it is true in the namespace rather than outside
// it.
//
// $USER said hgryoo while every process in the namespace ran as root, and 23 of
// the corpus's 24 uses of $USER are a process or IPC filter -- `ps -u $USER`,
// `ipcs | grep $USER`, `ps -ef | grep $USER`. All of them silently matched
// nothing. Seven cases failed on it directly: _37_cubrid/_01_service,
// _02_server, _03_manager and bug_xdbms79, _34_cub_auto/bug_xdbms212,
// _35_cub_js/bug_xdbms212 and _36_cub_master/bug_xdbms40 -- exactly the set of
// cases in the corpus that select processes by user. _02_server printed
// "cubrid server start: success" and then "DB testdb can't start!" on the next
// line, because `ps -u hgryoo` could not see the server it had just started.
//
// CTP's own cleanup is on the same list (init.sh kills by `ps -u $USER` and
// sweeps broker segments by `ipcs | grep $USER`), so it has been finding nothing
// too -- which does not fail a case, it just quietly leaves things behind.
//
// Setting it to root is not a workaround for the namespace, it is the namespace
// described accurately: in here the processes really are root's, and a filter
// that says so selects exactly what the case meant. Nothing in the corpus uses
// $USER as a path or a database user -- that is $USERNAME, a different variable
// CTP sets.
func inside(env []string) []string {
	out := make([]string, 0, len(env)+2)
	for _, kv := range env {
		switch {
		case strings.HasPrefix(kv, "USER="), strings.HasPrefix(kv, "LOGNAME="):
		default:
			out = append(out, kv)
		}
	}
	return append(out, "USER=root", "LOGNAME=root")
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

	// A bash-compatible /bin/sh, inside this namespace only.
	//
	// The shell task has always required one: CTP's init.sh line 51 is
	// `function get_os(){`, its unittest plug-in is written with
	// `function init { }` and sources files, and on a distribution where
	// /bin/sh is dash every case dies on init.sh's first line and reports a
	// blank result. Measured: CTP run on this machine outside the bind failed
	// all seventeen cases that way, and every one of them was
	// "Syntax error: \"(\" unexpected".
	//
	// It is a default rather than a setting because the requirement is not
	// optional and the machine is not asked to change: the bind lives in this
	// run's mount namespace and nothing outside it sees a different /bin/sh.
	// ShellEnv still overrides, for a machine whose bash-compatible shell is
	// somewhere else or is called something else.
	if sh := shellForSh(); sh != "" {
		if err := syscall.Mount(sh, "/bin/sh", "", syscall.MS_BIND, ""); err != nil {
			return fmt.Errorf("bind %s over /bin/sh: %w", sh, err)
		}
	}
	return nil
}

// shellForSh is what should be bound over /bin/sh, or "" for leaving it alone.
//
// Nothing is bound when /bin/sh already resolves to the same file -- which is
// every QA machine, so the common case does no work and the mount table stays
// as it was. And nothing is bound when no bash can be found: a machine whose sh
// is ksh runs this suite fine, and failing the run there would be worse than
// letting a case report the real error.
func shellForSh() string {
	want := os.Getenv(ShellEnv)
	if want == "" {
		found, err := exec.LookPath(Shell)
		if err != nil {
			return ""
		}
		want = found
	}
	wantReal, err := filepath.EvalSymlinks(want)
	if err != nil {
		return ""
	}
	if shReal, err := filepath.EvalSymlinks("/bin/sh"); err == nil && shReal == wantReal {
		return ""
	}
	return want
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
