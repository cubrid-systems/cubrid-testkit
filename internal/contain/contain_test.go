package contain

import (
	"os"
	"os/exec"
	"strings"
	"testing"
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
}

// The re-executed process is PID 1 of its own namespace, keeps $USER, sees its
// own /proc, and collects orphans. Skipped where the kernel does not allow an
// unprivileged user namespace, because that is a property of the machine rather
// than of this code.
func TestAContainedRunIsAloneAndReaps(t *testing.T) {
	if _, err := os.Stat("/proc/self/ns/pid"); err != nil {
		t.Skip("no namespace support here")
	}
	probe := buildProbe(t)

	cmd := exec.Command(probe)
	cmd.Env = append(os.Environ(), Env+"=1", "USER=probe-user")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "operation not permitted") ||
			strings.Contains(err.Error(), "operation not permitted") {
			t.Skipf("unprivileged user namespaces are not available: %v", err)
		}
		t.Fatalf("probe failed: %v\n%s", err, out)
	}

	got := string(out)
	for _, want := range []string{"pid=1", "user=probe-user", "proc=ok", "reaped=yes"} {
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
	"strconv"
	"strings"

	"github.com/cubrid-systems/cubrid-testkit/internal/contain"
)

func main() {
	if code := contain.Enter(); code >= 0 {
		os.Exit(code)
	}
	if err := contain.Setup(); err != nil {
		fmt.Println("proc=failed", err)
		return
	}
	fmt.Println("proc=ok")
	contain.Reap()
	fmt.Println("pid=" + strconv.Itoa(os.Getpid()))
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
	_ = syscall.Getpid()
}
`
	if err := os.WriteFile(dir+"/main.go", []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := dir + "/probe"
	build := exec.Command("go", "build", "-o", bin, dir+"/main.go")
	build.Dir = mustModuleRoot(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Skipf("cannot build the probe here: %v\n%s", err, out)
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

// $USER said hgryoo while every process in the namespace ran as root, so every
// `ps -u $USER` in the corpus silently matched nothing -- including the one in
// _37_cubrid/_02_server, which printed "cubrid server start: success" and then
// "DB testdb can't start!" on the next line.
func TestInsideSaysWhoTheProcessesActuallyAre(t *testing.T) {
	got := inside([]string{"PATH=/bin", "USER=hgryoo", "HOME=/home/hgryoo", "LOGNAME=hgryoo"})
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
		case kv == "HOME=/home/hgryoo":
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
