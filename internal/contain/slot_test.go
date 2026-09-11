package contain

import (
	"context"
	"os"
	osexec "os/exec"
	"path/filepath"
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
	ns, err := Open(t.Name(), slotRoot(t))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { ns.Close() })
	return ns
}

// slotRoot is a slot root made the way a run makes one, and removed when the
// test is done with it.
func slotRoot(t *testing.T) string {
	t.Helper()
	root, err := NewSlotRoot()
	if err != nil {
		t.Fatalf("slot root: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	return root
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
	if _, err := Open("slot0", t.TempDir()); err == nil {
		t.Error("a namespace was opened from an uncontained runner")
	}
}

// The port space is what two slots collide on -- the master, the two brokers,
// and whatever a case starts on its own. A network namespace each means every
// slot runs on the shipped 1523 and nothing has to be reallocated, which is what
// keeps a slotted run's configuration byte-identical to a serial one's.
func TestTwoSlotsHoldTheSamePort(t *testing.T) {
	a, b := namespace(t), namespace(t)

	// -u because the listener writes to a file and then sleeps: block-buffered,
	// its output would arrive only when it exits, which is after the check.
	hold := func(tag string) string {
		return `setsid python3 -u -c 'import socket,time
s=socket.socket(); s.bind(("127.0.0.1",1523)); s.listen(1)
print("bound"); time.sleep(5)' >/tmp/port-` + tag + `.out 2>&1 </dev/null &
sleep 1; cat /tmp/port-` + tag + `.out`
	}

	if got := run(t, a, hold("a")); !strings.Contains(got, "bound") {
		t.Fatalf("the first slot could not take 1523: %q", got)
	}
	if got := run(t, b, hold("b")); !strings.Contains(got, "bound") {
		t.Errorf("the second slot could not take 1523 while the first held it: %q", got)
	}
}

// Loopback is down in a fresh network namespace, and every connection a case
// makes goes through it.
func TestLoopbackIsUp(t *testing.T) {
	ns := namespace(t)
	if got := run(t, ns, "ip -o link show lo"); !strings.Contains(got, "UP") {
		t.Errorf("loopback is not up: %q", got)
	}
}

// What one slot writes into the install must not be what another reads, and the
// list of places a run writes has already been wrong once -- it named lib/ and
// not locales/loclib/, where make_locale builds before moving the result. An
// overlay does not need the list to be right.
func TestSlotsWriteIntoTheInstallWithoutSeeingEachOther(t *testing.T) {
	a, b := namespace(t), namespace(t)

	install := t.TempDir()
	if err := os.WriteFile(install+"/shared", []byte("from the install\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.Overlay(install, t.TempDir()); err != nil {
		t.Fatalf("overlay a: %v", err)
	}
	if err := b.Overlay(install, t.TempDir()); err != nil {
		t.Fatalf("overlay b: %v", err)
	}

	if got := run(t, a, "cat "+install+"/shared"); got != "from the install" {
		t.Errorf("the install does not read through the overlay: %q", got)
	}
	run(t, a, "echo a > "+install+"/conf")
	run(t, b, "echo b > "+install+"/conf")
	if got := run(t, a, "cat "+install+"/conf"); got != "a" {
		t.Errorf("the other slot's write is visible here: %q", got)
	}
	if _, err := os.Stat(install + "/conf"); err == nil {
		t.Error("a slot's write reached the install itself")
	}
}

// POSIX shared memory is a file on a tmpfs, and a mount namespace inherits the
// tmpfs it was cloned from -- so an IPC namespace, which separates System V
// segments, leaves /dev/shm shared. cub_broker and cub_cas use both.
func TestSlotsDoNotShareDevShm(t *testing.T) {
	a, b := namespace(t), namespace(t)

	run(t, a, "echo mine > /dev/shm/probe")
	if got := run(t, b, "cat /dev/shm/probe 2>&1 || echo absent"); !strings.Contains(got, "absent") {
		t.Errorf("the other slot sees this slot's POSIX shared memory: %q", got)
	}
	if _, err := os.Stat("/dev/shm/probe"); err == nil {
		t.Error("a slot's POSIX shared memory reached the machine's /dev/shm")
	}
}

// A slot has to answer "how do I reach this machine", because 108 case scripts
// in this corpus ask -- and a fresh network namespace answers with nothing.
func TestASlotHasAnAddressItWillAdmitTo(t *testing.T) {
	ns := namespace(t)

	out, err := ns.run(context.Background(), 10*time.Second, "hostname -I")
	if err != nil {
		t.Fatalf("hostname -I: %v: %s", err, out)
	}
	got := strings.TrimSpace(out)
	if got == "" {
		t.Fatal("`hostname -I` is empty in the slot, which is what breaks the 108 cases that read it")
	}
	if !strings.Contains(got, SlotAddress) {
		t.Errorf("`hostname -I` says %q, want it to contain %s", got, SlotAddress)
	}

	// And the address has to be usable, not just present: the cases that read it
	// go on to connect to it.
	out, err = ns.run(context.Background(), 15*time.Second,
		"(timeout 5 nc -l "+SlotAddress+" 15999 >/dev/null 2>&1 &); sleep 0.4; "+
			"timeout 3 bash -c 'echo hi > /dev/tcp/"+SlotAddress+"/15999' && echo reachable")
	if err != nil || !strings.Contains(out, "reachable") {
		t.Errorf("nothing can be reached on the slot's own address: %v: %s", err, out)
	}
}

// overlayfs answers every refusal with "wrong fs type, bad option, bad
// superblock on overlay, missing codepage or helper program, or other error",
// which names none of them. Two of the reasons are checkable, and both were met
// getting a run to work in a container.
func TestAnOverlayRefusalSaysWhatItCanWorkOut(t *testing.T) {
	dir := t.TempDir()

	// A plain directory: nothing to add, and nothing invented.
	plain := filepath.Join(dir, "plain")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := whyOverlayFailed(plain); got != "" {
		t.Errorf("a plain directory needs no explanation, got %q", got)
	}

	// A mount point: a lower layer cannot be one. Measured -- the same overlay
	// succeeds on a plain directory of the same filesystem.
	if isMountPoint("/proc") {
		if got := whyOverlayFailed("/proc"); !strings.Contains(got, "mount point") {
			t.Errorf("a mount point should be named as one, got %q", got)
		}
	}

	// And a path that does not exist is not a mount point, rather than a panic.
	if isMountPoint(filepath.Join(dir, "nothing-here")) {
		t.Error("a missing path is not a mount point")
	}
}

// A user namespace maps one uid, so a directory owned by another one is nobody
// in there and cannot be written -- which arrives as a bare "permission denied"
// on a path this process could write to a moment earlier.
func TestAPermissionDeniedOnAnUnmappedOwnerSaysSo(t *testing.T) {
	// Not a permission error: nothing to add.
	if got := whyMkdirFailed("/tmp", os.ErrExist); got != "" {
		t.Errorf("only a permission error gets the explanation, got %q", got)
	}
	// Owned by root, and this test is not root (skipped when it is).
	if os.Getuid() == 0 {
		t.Skip("running as root, so there is no unmapped owner to find")
	}
	got := whyMkdirFailed("/var/lib/nothing-here", os.ErrPermission)
	if !strings.Contains(got, "user namespace") || !strings.Contains(got, "TESTKIT_SLOT_ROOT") {
		t.Errorf("the explanation should name the namespace and the knob, got %q", got)
	}
}
