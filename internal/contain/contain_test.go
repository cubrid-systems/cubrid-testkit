package contain

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Off is the default, and this is the test that says so: a run that did not ask
// for containment must not be re-executed, must not try to mount anything, and
// must not start a reaper.
func TestNothingHappensUnlessAsked(t *testing.T) {
	t.Setenv(Env, "")
	t.Setenv(insideEnv, "")

	if Wanted() {
		t.Error("containment was wanted without being asked for")
	}
	if Active() {
		t.Error("reported active outside the namespace")
	}
	if code := Enter(); code != -1 {
		t.Errorf("Enter returned %d rather than -1 when there was nothing to do", code)
	}
	if err := Setup(); err != nil {
		t.Errorf("Setup tried to do something outside the namespace: %v", err)
	}
	if code := Init(); code != -1 {
		t.Errorf("Init returned %d rather than -1 when there was nothing to contain", code)
	}
}

// The re-executed process runs under an init of its own rather than being one,
// is root in the namespace, sees its own /proc, and has its orphans collected.
// Skipped where the kernel does not allow an unprivileged user namespace,
// because that is a property of the machine rather than of this code.
func TestAContainedRunIsAloneAndReaps(t *testing.T) {
	if _, err := os.Stat("/proc/self/ns/pid"); err != nil {
		t.Skip("no namespace support here")
	}
	probe := buildProbe(t)

	cmd := exec.Command(probe)
	cmd.Env = append(os.Environ(), Env+"=1", "USER=probe-user")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if permissionRefused(string(out)) || permissionRefused(err.Error()) {
			t.Skipf("unprivileged user namespaces are not available: %v", err)
		}
		t.Fatalf("probe failed: %v\n%s", err, out)
	}

	got := string(out)
	// The probe reports a Setup failure on its standard output and exits 0, so
	// the check above cannot see it. A machine that refuses to make its mounts
	// private, or to mount /proc, is the same kind of machine as one that
	// refuses the namespace itself -- a property of the machine, which this test
	// skips on rather than fails on. Seen in CI as
	// "proc=failed make mounts private: permission denied", where the check
	// above missed it twice over: the wording is EACCES rather than EPERM, and
	// the probe had not failed.
	if strings.Contains(got, "proc=failed") && permissionRefused(got) {
		t.Skipf("this machine does not allow the namespace setup: %s", strings.TrimSpace(got))
	}
	// ispid1=no is the fix, stated as an assertion: PID 1 is the init that
	// collects orphans, and the process that runs commands is its child. Were
	// they one process, its Wait4(-1) would take os/exec's own children and the
	// case would be thrown away as "waitid: no child processes". reaped=yes says
	// the orphan is still collected -- by the init, on this process's behalf.
	//
	// Not pid=2: threads take numbers from the same space, and the Go runtime
	// has several before it forks anything.
	//
	// user=root because inside() says so, and says why at length: in here the
	// processes really are root's, and 23 of the corpus's 24 uses of $USER are a
	// process or IPC filter.
	for _, want := range []string{"ispid1=no", "user=root", "proc=ok", "reaped=yes"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in probe output:\n%s", want, got)
		}
	}
	// The whole point: what the namespace can see is the run and nothing else.
	// A machine that is running anything at all has more than a handful.
	if !strings.Contains(got, "alone=yes") {
		t.Errorf("the namespace could see processes that are not the run's:\n%s", got)
	}
}

// buildProbe compiles a program that calls this package the way main does.
func buildProbe(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := `package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/cubrid-systems/cubrid-testkit/internal/contain"
)

func main() {
	if code := contain.Enter(); code >= 0 {
		os.Exit(code)
	}
	if code := contain.Init(); code >= 0 {
		os.Exit(code)
	}
	if err := contain.Setup(); err != nil {
		fmt.Println("proc=failed", err)
		return
	}
	fmt.Println("proc=ok")
	// Held open when asked, so a test can kill the process that contains this
	// one and see whether this one goes with it.
	if hold := os.Getenv("PROBE_HOLD"); hold != "" {
		_ = os.WriteFile(hold, []byte("ready"), 0o644)
		select {}
	}
	if os.Getpid() == 1 {
		fmt.Println("ispid1=yes")
	} else {
		fmt.Println("ispid1=no", os.Getpid())
	}
	fmt.Println("user=" + os.Getenv("USER"))

	// Orphan a process and give the reaper a chance at it.
	_ = exec.Command("/bin/sh", "-c", "( sleep 0.1 & ) ; sleep 0.6").Run()
	out, _ := exec.Command("/bin/sh", "-c", "ps -e -o pid,stat,comm").Output()
	zombies, lines := 0, 0
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n")[1:] {
		lines++
		if f := strings.Fields(l); len(f) > 1 && strings.Contains(f[1], "Z") {
			zombies++
		}
	}
	if zombies == 0 {
		fmt.Println("reaped=yes")
	} else {
		fmt.Println("reaped=no", zombies)
	}
	if lines > 0 && lines < 10 {
		fmt.Println("alone=yes", lines)
	} else {
		fmt.Println("alone=no", lines)
	}
}
`
	// Inside the module, or the probe cannot import internal/ and the build
	// fails -- which buildProbe reports as a skip, so this test said nothing at
	// all for as long as it has existed. The leading dot keeps the directory out
	// of ./... while go build still takes the file by name.
	root := mustModuleRoot(t)
	pkg, err := os.MkdirTemp(root, ".probe")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(pkg) })
	if err := os.WriteFile(pkg+"/main.go", []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := dir + "/probe"
	build := exec.Command("go", "build", "-o", bin, pkg+"/main.go")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("cannot build the probe: %v\n%s", err, out)
	}
	return bin
}

func mustModuleRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Skipf("cannot find the module root: %v", err)
	}
	p := strings.TrimSpace(string(out))
	if p == "" || p == os.DevNull {
		t.Skip("not in a module")
	}
	return p[:strings.LastIndex(p, "/")]
}

// $USER said the outside user while every process in the namespace ran as root, so every
// `ps -u $USER` in the corpus silently matched nothing -- including the one in
// _37_cubrid/_02_server, which printed "cubrid server start: success" and then
// "DB testdb can't start!" on the next line.
func TestInsideSaysWhoTheProcessesActuallyAre(t *testing.T) {
	got := inside([]string{"PATH=/bin", "USER=qa", "HOME=/home/qa", "LOGNAME=qa"})
	var user, logname, home int
	for _, kv := range got {
		switch {
		case strings.HasPrefix(kv, "USER="):
			user++
			if kv != "USER=root" {
				t.Errorf("USER is %q, and in here the processes are root's", kv)
			}
		case strings.HasPrefix(kv, "LOGNAME="):
			logname++
		case kv == "HOME=/home/qa":
			home++
		}
	}
	if user != 1 || logname != 1 {
		t.Errorf("USER appears %d times and LOGNAME %d; a duplicate leaves which one wins to the exec", user, logname)
	}
	// $HOME is not touched: it is where ERROR_BACKUP and the template store
	// live, and they belong to the user outside.
	if home != 1 {
		t.Error("HOME was changed; it points at directories the run shares with the machine")
	}
}

// The bind over /bin/sh is a default rather than a setting, because the
// requirement is not optional: CTP's init.sh line 51 is `function get_os(){`,
// and where /bin/sh is dash every case dies on the first line with
// "Syntax error: \"(\" unexpected". But it must do nothing on the machines that
// already have it right, which is every QA machine.
func TestTheShellBindDoesNothingWhenShIsAlreadyRight(t *testing.T) {
	t.Setenv(ShellEnv, "/bin/sh")
	if got := shellForSh(); got != "" {
		t.Errorf("shellForSh() = %q; /bin/sh is already itself, so there is nothing to bind", got)
	}
}

// An explicit setting still wins, for a machine whose bash-compatible shell is
// somewhere else or is called something else.
func TestTheShellBindHonoursTheSetting(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("no bash on this machine")
	}
	t.Setenv(ShellEnv, bash)
	got := shellForSh()
	sh, shErr := filepath.EvalSymlinks("/bin/sh")
	want, wantErr := filepath.EvalSymlinks(bash)
	if shErr == nil && wantErr == nil && sh == want {
		// /bin/sh is bash here, so nothing to do.
		if got != "" {
			t.Errorf("shellForSh() = %q where /bin/sh is already bash", got)
		}
		return
	}
	if got != bash {
		t.Errorf("shellForSh() = %q, want %q", got, bash)
	}
}

// A machine with no bash is left alone rather than failed: its sh may be ksh,
// which runs this suite, and failing the run would be worse than letting a case
// report the real error.
func TestNoBashLeavesShAlone(t *testing.T) {
	t.Setenv(ShellEnv, "")
	t.Setenv("PATH", t.TempDir())
	if got := shellForSh(); got != "" {
		t.Errorf("shellForSh() = %q with no bash on PATH", got)
	}
}

// A run mounts its overlays on whatever is in its slot root, so the root has to
// be one no earlier run has written to. Keyed on the pid it was not: contained,
// the pid is the namespace's and recurs, and a run started with the previous
// run's $CUBRID writes already in its upper layer.
func TestSlotRootIsFreshPerRun(t *testing.T) {
	base := t.TempDir()
	t.Setenv(SlotRootEnv, base)

	first, err := NewSlotRoot()
	if err != nil {
		t.Fatal(err)
	}
	// What a run leaves behind: a change to the install, in slot0's upper layer.
	left := filepath.Join(first, "slot0", "CUBRID", "upper", "conf")
	if err := os.MkdirAll(left, 0o755); err != nil {
		t.Fatal(err)
	}

	second, err := NewSlotRoot()
	if err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Fatalf("two runs were given the same slot root %s", first)
	}
	if entries, err := os.ReadDir(second); err != nil || len(entries) != 0 {
		t.Errorf("the second run's slot root is not empty: %v %v", entries, err)
	}
	// The setting says where runs make their roots; it is not one itself, so
	// two runs sharing it are still kept apart.
	for _, root := range []string{first, second} {
		if filepath.Dir(root) != base {
			t.Errorf("slot root %s was not made in %s", root, base)
		}
	}
}

// Enter uses -1 for "there was nothing to contain", and Go uses -1 for "killed
// by a signal". The caller tests code >= 0, so the second reads as the first and
// the run continues uncontained -- where the reset empties the real $CUBRID and
// the process sweep selects across the whole machine.
func TestASignalIsNotTheSameAsNothingToContain(t *testing.T) {
	cmd := exec.Command("bash", "-c", "kill -9 $$")
	err := cmd.Run()
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("a signalled process should give an ExitError: %v", err)
	}
	if ee.ExitCode() != -1 {
		t.Skipf("this platform reports %d for a signalled process, not -1", ee.ExitCode())
	}
	if ee.ProcessState.Exited() {
		t.Fatal("a signalled process must not report Exited()")
	}
	// Which is the whole point: the code alone cannot tell the two apart, and
	// Exited() can.
}

// Wait4(-1) takes any child, including the ones os/exec is waiting on: its Wait
// is a waitid peek followed by a wait4, and losing that race gives ECHILD --
// "waitid: no child processes" -- which is neither an ExitError nor
// ErrWaitDelay, so the case's verdict is discarded as a runtime error. Two of
// the first 53 cases judged in a 24-slot run died that way and nothing else did.
//
// So the process that runs commands must never be the one collecting orphans.
// Init is the split, and here -- where this process is not PID 1 of a namespace
// -- it must install nothing at all.
func TestTheProcessThatRunsCommandsCollectsNoOrphans(t *testing.T) {
	if os.Getpid() == 1 {
		t.Skip("this process is PID 1, which is the case Init is for")
	}
	t.Setenv(Env, "1")
	t.Setenv(insideEnv, "1")

	if code := Init(); code != -1 {
		t.Fatalf("Init returned %d in a process that is not PID 1 of a namespace", code)
	}

	// Demonstrated by the thing a reaper here would break: children started and
	// waited for while SIGCHLD traffic is going on.
	for i := 0; i < 20; i++ {
		go func() { _ = exec.Command("true").Run() }()
	}
	for i := 0; i < 20; i++ {
		out, err := exec.Command("bash", "-c", "echo ok").Output()
		if err != nil {
			t.Fatalf("a child's status was stolen: %v", err)
		}
		if strings.TrimSpace(string(out)) != "ok" {
			t.Fatalf("wrong output: %q", out)
		}
	}
}

// Killing the outer process must take the namespace with it. The child is PID 1
// of a PID namespace, so nothing outside can see it by its own number: the
// result tree's lock file named a "pid 7" that does not exist out here, and
// three times in one session a killed run left the next one refused with
// "another run already has ...".
//
// SIGKILL, because that is the case forwarding cannot cover and the one that
// actually happened.
func TestKillingTheRunnerTakesTheNamespaceWithIt(t *testing.T) {
	if _, err := os.Stat("/proc/self/ns/pid"); err != nil {
		t.Skip("no namespace support here")
	}
	probe := buildProbe(t)

	marker := filepath.Join(t.TempDir(), "ready")
	cmd := exec.Command(probe)
	cmd.Env = append(os.Environ(), Env+"=1", "USER=probe-user", "PROBE_HOLD="+marker)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Skipf("the probe never contained itself here: %v", err)
	}

	// From inside, the contained process is pid 1 and says so; its host pid is
	// only visible out here, as a child of the process that made it.
	inner := childrenOf(t, cmd.Process.Pid)
	if len(inner) == 0 {
		t.Skip("the kernel did not list the child; /proc children needs CONFIG_PROC_CHILDREN")
	}

	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_, _ = cmd.Process.Wait()

	for _, pid := range inner {
		gone := false
		for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); {
			if err := syscall.Kill(pid, 0); err != nil {
				gone = true
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if !gone {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("pid %d outlived the process that contained it; a run killed this way "+
				"keeps the result tree's lock, and the next run is refused by a pid "+
				"that does not exist outside the namespace", pid)
		}
	}
}

// childrenOf reads the kernel's own list rather than scanning /proc for a parent,
// which races with the exit this test is about.
func childrenOf(t *testing.T, pid int) []int {
	t.Helper()
	tasks, err := filepath.Glob(fmt.Sprintf("/proc/%d/task/*/children", pid))
	if err != nil {
		return nil
	}
	var out []int
	for _, f := range tasks {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, s := range strings.Fields(string(b)) {
			if n, err := strconv.Atoi(s); err == nil {
				out = append(out, n)
			}
		}
	}
	return out
}

// permissionRefused reports whether a message is the kernel saying no, in either
// of the two wordings it uses: EPERM is "operation not permitted" and EACCES is
// "permission denied", and a machine that forbids unprivileged namespaces can
// answer with either.
func permissionRefused(s string) bool {
	return strings.Contains(s, "operation not permitted") || strings.Contains(s, "permission denied")
}
