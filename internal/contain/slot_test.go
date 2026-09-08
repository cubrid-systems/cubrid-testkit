package contain

import (
	"context"
	"os"
	osexec "os/exec"
	"strings"
	"testing"
	"time"
)

// A namespace can only be opened from inside a contained runner, and these tests
// have to be run there. They skip rather than fail elsewhere: the alternative is
// a suite that goes red on a laptop for a reason that is not about the code.
func namespace(t *testing.T) *Namespace {
	t.Helper()
	if !Active() {
		t.Skipf("not contained; run under %s=1", Env)
	}
	if _, err := osexec.LookPath("nsenter"); err != nil {
		t.Skip("nsenter is not on PATH")
	}
	ns, err := Open(t.Name())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { ns.Close() })
	return ns
}

func run(t *testing.T, ns *Namespace, script string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := ns.Channel("").Run(ctx, script)
	if err != nil {
		t.Fatalf("run %q: %v", script, err)
	}
	return strings.TrimSpace(res.Stdout)
}

// The whole design rests on the namespace outliving each command: a case starts
// a server in one and stops it in another. If it did not, every slot would take
// its own server down the moment the command that started it returned.
func TestTheNamespaceOutlivesTheCommand(t *testing.T) {
	ns := namespace(t)

	run(t, ns, "mkdir -p /tmp/nsprobe && mount -t tmpfs none /tmp/nsprobe && echo alive > /tmp/nsprobe/f")
	if got := run(t, ns, "cat /tmp/nsprobe/f"); got != "alive" {
		t.Errorf("a mount made by one command is not there for the next: %q", got)
	}
	if _, err := os.Stat("/tmp/nsprobe/f"); err == nil {
		t.Error("the mount is visible outside the namespace, so it is not contained")
	}

	run(t, ns, "setsid sleep 300 >/dev/null 2>&1 </dev/null & sleep 0.3; echo started")
	if got := run(t, ns, `pgrep -xf 'sleep 300' | wc -l`); got == "0" {
		t.Error("a process started by one command is gone by the next")
	}
}

// Inside a PID namespace `ps -e` is already exactly this slot's work, which is
// what lets the reset stop matching process names across the whole machine.
func TestProcessesAndSegmentsAreTheSlotsOwn(t *testing.T) {
	ns := namespace(t)

	inside := run(t, ns, "ps -e --no-headers | wc -l")
	outside, err := osexec.Command("bash", "-c", "ps -e --no-headers | wc -l").Output()
	if err != nil {
		t.Fatal(err)
	}
	if inside == strings.TrimSpace(string(outside)) {
		t.Errorf("ps sees the same %s processes inside and out, so the namespace is not doing anything", inside)
	}

	// A collision here does not fail, it interferes, which is why the isolation
	// matters more than the count.
	if got := run(t, ns, "ipcs -m | grep -c '^0x' || true"); got == "" {
		t.Error("ipcs reported nothing at all")
	}
}

// A slot whose keeper has died must fail rather than quietly run on the machine.
func TestAGoneNamespaceIsAnErrorRatherThanTheMachine(t *testing.T) {
	ns := namespace(t)
	ch := ns.Channel("")

	if err := ns.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if ns.Alive() {
		t.Fatal("still alive after Close")
	}
	if _, err := ch.Run(context.Background(), "true"); err == nil {
		t.Error("a command on a closed namespace succeeded, which means it ran somewhere else")
	}
}

// Killing the keeper takes the namespace's processes with it. Getting this wrong
// leaves a slot's servers running after the slot is finished with them, and the
// next run inherits them.
func TestClosingTakesTheProcessesWithIt(t *testing.T) {
	ns := namespace(t)
	run(t, ns, "setsid sleep 301 >/dev/null 2>&1 </dev/null & sleep 0.3; echo started")

	ns.Close()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		out, _ := osexec.Command("bash", "-c", `pgrep -xf 'sleep 301' | wc -l`).Output()
		if strings.TrimSpace(string(out)) == "0" {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Error("a process started in the namespace is still running after it was closed")
}

// Enter is for the runner and Open is for a slot; asking for a slot outside a
// contained runner is a mistake worth naming rather than a nil to dereference.
func TestASlotOutsideAContainedRunnerIsRefused(t *testing.T) {
	if Active() {
		t.Skip("this process is contained")
	}
	if _, err := Open("slot0"); err == nil {
		t.Error("a namespace was opened from an uncontained runner")
	}
}
