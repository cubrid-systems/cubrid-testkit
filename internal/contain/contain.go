// Package contain puts the run in namespaces of its own.
//
// The reset before every case matches process names as substrings across
// everything the user owns, so a run and anything else the same user is doing
// cannot share a machine. Containment is the answer, and the interesting part is
// that it changes what the reset *is*: inside a PID namespace `ps -e` is already
// exactly this run's work, and inside an IPC namespace so is `ipcs`. A filter
// that can be wrong becomes a property of where the process is.
//
// docs/project/concept/beyond-axis.md B-T2.
package contain

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

// Env asks for containment. It is off unless set to "1", and that default is not
// only ADR-015's rule about axis B: the sweep selects nothing today, so turning
// this on is the first time the reset will actually kill anything.
const Env = "TESTKIT_CONTAIN"

// insideEnv marks the re-executed process so it does not try again.
const insideEnv = "TESTKIT_CONTAINED"

// OuterUserEnv is $USER as it was before containment made it root (see inside).
// The cases need root, which is what the namespace is; a record of who ran the
// test -- main.info's user line -- needs the account.
const OuterUserEnv = "TESTKIT_OUTER_USER"

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
	cmd.Env = append(inside(os.Environ()), insideEnv+"=1", OuterUserEnv+"="+os.Getenv("USER"))
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUSER | syscall.CLONE_NEWNS |
			syscall.CLONE_NEWPID | syscall.CLONE_NEWIPC,
		UidMappings:                []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getuid(), Size: 1}},
		GidMappings:                []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getgid(), Size: 1}},
		GidMappingsEnableSetgroups: false,
		// Killing this process must not leave the run behind. The child is PID
		// 1 of a PID namespace, so nothing outside can see it: `ps` shows the
		// argv of a process that reports pid 1 in there, and the result tree's
		// lock file names a "pid 7" that does not exist out here. Observed
		// three times in one session -- the outer process killed, the inner one
		// still holding the lock, and the next run refused with
		// "another run already has ...".
		//
		// Pdeathsig is for SIGKILL, which cannot be forwarded. It is delivered
		// when the *thread* that made the child goes away, not the process,
		// which is why this goroutine holds its OS thread until Wait returns:
		// a thread the runtime retired would kill a healthy run.
		Pdeathsig: syscall.SIGKILL,
	}

	// And the signals that can be forwarded are, so that a stop is a stop and
	// not a kill: the child's own init passes them on to the work, which has a
	// signal.NotifyContext of its own.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := cmd.Start(); err != nil {
		fail("cannot contain the run: %v", err)
	}
	sig := make(chan os.Signal, 4)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sig)
	go func() {
		for s := range sig {
			if n, ok := s.(syscall.Signal); ok && cmd.Process != nil {
				_ = cmd.Process.Signal(n)
			}
		}
	}()
	if err := cmd.Wait(); err != nil {
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

// inContainer reports whether this process looks containerised, which changes
// what a refused mount most likely means. A guess, and only ever used to add a
// sentence to an error.
func inContainer() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	if b, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		if strings.Contains(string(b), "docker") || strings.Contains(string(b), "containerd") {
			return true
		}
	}
	return false
}

// inside is the environment as it is true in the namespace rather than outside
// it.
//
// $USER said the user outside while every process in the namespace ran as root, and 23 of
// the corpus's 24 uses of $USER are a process or IPC filter -- `ps -u $USER`,
// `ipcs | grep $USER`, `ps -ef | grep $USER`. All of them silently matched
// nothing. Seven cases failed on it directly: _37_cubrid/_01_service,
// _02_server, _03_manager and bug_xdbms79, _34_cub_auto/bug_xdbms212,
// _35_cub_js/bug_xdbms212 and _36_cub_master/bug_xdbms40 -- exactly the set of
// cases in the corpus that select processes by user. _02_server printed
// "cubrid server start: success" and then "DB testdb can't start!" on the next
// line, because `ps -u <that user>` could not see the server it had just started.
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
		// A container is the case worth naming: Docker masks paths under /proc,
		// and a mount over a masked one is refused. Neither seccomp=unconfined
		// nor --cap-add SYS_ADMIN lifts it; systempaths=unconfined does, and
		// --privileged is not needed. Measured while getting a run to work in
		// the CI image, where the bare error named none of it.
		hint := ""
		if inContainer() {
			hint = "\n  This looks like a container. Docker masks paths under /proc and refuses" +
				"\n  a mount over one: run it with --security-opt systempaths=unconfined."
		}
		return fmt.Errorf("mount /proc: %w%s", err, hint)
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

	// And a machine name that resolves to loopback, because a slot's network
	// namespace has nothing else. See hosts.go: this is invisible on a host
	// whose hostname maps to 127.0.1.1 and fatal in a container whose hostname
	// maps to eth0.
	dir, err := os.MkdirTemp("", "testkit-ns-")
	if err != nil {
		return fmt.Errorf("make a place for this namespace's files: %w", err)
	}
	if err := bindHosts(dir); err != nil {
		return err
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

// initEnv marks the process the namespace's init forked to do the work.
const initEnv = "TESTKIT_CONTAINED_INIT"

// Init makes PID 1 of the namespace an init and nothing else: it starts this
// program again and then does nothing but collect orphans until that child
// exits, whose exit code it returns. Every other process gets -1 and carries on.
//
// The split is the whole point, and it is what the previous shape got wrong.
// Collecting orphans means Wait4(-1), which takes *any* child -- including the
// ones os/exec is waiting for. os/exec then gets ECHILD, which is neither an
// ExitError nor ErrWaitDelay, so the case's verdict is thrown away as
// "Runtime error (local run: waitid: no child processes)". Measured: the first
// two failures of a 24-slot run were that and nothing else, two of 53 judged.
//
// The reaping cannot simply be dropped instead. PID 1 inherits every orphan in
// the namespace, a daemon that double-forks becomes one, and an unreaped zombie
// is how a `cubrid server stop` was seen polling for a server that had already
// exited, for twenty-three minutes. So the reaping stays, and moves to a
// process that runs no commands of its own and therefore has nothing to steal.
//
// It was dormant until now: this ran under a wrapper whose PID 1 was a shell,
// so `os.Getpid() != 1` returned before installing anything. Containment
// entered through Enter makes this process PID 1 for the first time.
func Init() int {
	if !Active() || os.Getpid() != 1 || os.Getenv(initEnv) == "1" {
		return -1
	}
	self, err := os.Executable()
	if err != nil {
		fail("cannot find own path: %v", err)
	}
	child, err := syscall.ForkExec(self, os.Args, &syscall.ProcAttr{
		Env:   append(os.Environ(), initEnv+"=1"),
		Files: []uintptr{0, 1, 2},
	})
	if err != nil {
		fail("cannot start the contained run: %v", err)
	}

	// PID 1 of a namespace receives only the signals it handles, and the run has
	// to stay stoppable from outside it.
	sig := make(chan os.Signal, 4)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		for s := range sig {
			if n, ok := s.(syscall.Signal); ok {
				_ = syscall.Kill(child, n)
			}
		}
	}()

	for {
		var ws syscall.WaitStatus
		pid, err := syscall.Wait4(-1, &ws, 0, nil)
		if err == syscall.EINTR {
			continue
		}
		if err != nil {
			fail("cannot wait in the namespace: %v", err)
		}
		if pid != child {
			continue // an orphan, which is what this process is here for
		}
		if ws.Signaled() {
			return 128 + int(ws.Signal())
		}
		return ws.ExitStatus()
	}
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
