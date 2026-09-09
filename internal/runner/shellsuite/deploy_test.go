package shellsuite

import (
	"context"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/conf"
	"github.com/cubrid-systems/cubrid-testkit/internal/contain"
	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
	"github.com/cubrid-systems/cubrid-testkit/internal/topology"
)

func instanceFrom(t *testing.T, text string) *topology.Instance {
	t.Helper()
	path := filepath.Join(t.TempDir(), "x.conf")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := (&conf.Home{Path: "/opt/ctp"}).Load(path)
	if err != nil {
		t.Fatal(err)
	}
	insts, err := topology.From(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(insts) != 1 {
		t.Fatalf("got %d instances, want 1", len(insts))
	}
	return insts[0]
}

func TestConfigureScriptWritesEachRoleToItsSection(t *testing.T) {
	inst := instanceFrom(t, strings.Join([]string{
		"env.instance1.cubrid.cubrid_port_id=1723",
		"env.instance1.cubrid.max_clients=100",
		"env.instance1.broker1.BROKER_PORT=30000",
		"env.instance1.broker2.BROKER_PORT=33000",
		"env.instance1.cm.cm_port=8001",
	}, "\n"))

	got := ConfigureScript(inst)

	for _, want := range []string{
		"ini.sh -s 'common' --separator '||' -u 'cubrid_port_id=1723||max_clients=100||' $CUBRID/conf/cubrid.conf",
		"ini.sh -s 'cm' --separator '||' -u 'cm_port=8001||' $CUBRID/conf/cm.conf",
		"ini.sh -s '%query_editor' --separator '||' -u 'BROKER_PORT=30000||' $CUBRID/conf/cubrid_broker.conf",
		"ini.sh -s '%BROKER1' --separator '||' -u 'BROKER_PORT=33000||' $CUBRID/conf/cubrid_broker.conf",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing line:\n  %s\nin:\n%s", want, got)
		}
	}
}

// broker1 is written to %query_editor and broker2 to %BROKER1. The names do not
// line up, and a reader who assumes they do will put the ports in the wrong
// broker.
func TestBrokerRolesGoToTheSectionsTheyAreNamedAfterInTheConfNotInTheRole(t *testing.T) {
	inst := instanceFrom(t, "env.instance1.broker1.BROKER_PORT=30000")
	got := ConfigureScript(inst)

	if strings.Contains(got, "-s '%BROKER1'") {
		t.Error("broker1 was written to the broker2 section")
	}
	if !strings.Contains(got, "-s '%query_editor'") {
		t.Errorf("broker1 did not reach the query_editor section:\n%s", got)
	}
}

func TestMasterSHMIDFallsBackToThePortNumber(t *testing.T) {
	inst := instanceFrom(t, "env.instance1.cubrid.cubrid_port_id=1723")
	got := ConfigureScript(inst)

	want := "ini.sh -s 'broker' -u MASTER_SHM_ID=1723 $CUBRID/conf/cubrid_broker.conf"
	if !strings.Contains(got, want) {
		t.Errorf("missing:\n  %s\nin:\n%s", want, got)
	}
}

func TestAnExplicitMasterSHMIDWins(t *testing.T) {
	inst := instanceFrom(t, strings.Join([]string{
		"env.instance1.cubrid.cubrid_port_id=1723",
		"env.instance1.brokercommon.MASTER_SHM_ID=1822",
	}, "\n"))
	got := ConfigureScript(inst)

	if !strings.Contains(got, "MASTER_SHM_ID=1822 $CUBRID") {
		t.Errorf("explicit MASTER_SHM_ID not used:\n%s", got)
	}
}

// CTP tested five of the six roles for emptiness and left out brokercommon, so a
// configuration that set only broker-wide parameters was skipped entirely --
// including the MASTER_SHM_ID that keeps two installs off each other's shared
// memory. Two instances that collide do not fail; they interfere.
func TestBrokerCommonAloneStillProducesAScript(t *testing.T) {
	inst := instanceFrom(t, "env.instance1.brokercommon.MASTER_SHM_ID=1822")

	got := ConfigureScript(inst)
	if got == "" {
		t.Fatal("a brokercommon-only instance produced no configuration script")
	}
	if !strings.Contains(got, "MASTER_SHM_ID=1822") {
		t.Errorf("MASTER_SHM_ID missing:\n%s", got)
	}
}

func TestAnInstanceThatConfiguresNothingProducesNoScript(t *testing.T) {
	inst := instanceFrom(t, "env.instance1.ssh.host=localhost")
	if got := ConfigureScript(inst); got != "" {
		t.Errorf("got a script for an instance with no engine parameters:\n%s", got)
	}
}

func TestParamListIsOrderedSoTwoRunsMatch(t *testing.T) {
	props := map[string]string{"z": "1", "a": "2", "m": "3"}
	if got, want := paramList(props), "a=2||m=3||z=1||"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if paramList(nil) != "" {
		t.Error("an empty role produced a parameter list")
	}
}

func TestKillScriptIsValidShell(t *testing.T) {
	for _, local := range []bool{true, false} {
		cmd := osexec.Command("bash", "-n")
		cmd.Stdin = strings.NewReader(KillScript(local, false))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("local=%v: bash -n rejected the script: %v\n%s", local, err, out)
		}
	}
}

// Killing every *.sh the user owns is fine from another machine and fatal when
// the runner is the process being swept.
func TestKillScriptSparesShellScriptsWhenRunningLocally(t *testing.T) {
	const sweep = `grep -i '\.sh'`
	if strings.Contains(KillScript(true, false), sweep) {
		t.Error("the local sweep would kill the case that asked for it")
	}
	if !strings.Contains(KillScript(false, false), sweep) {
		t.Error("the remote sweep no longer clears leftover scripts")
	}
}

// The JVM sweep has never worked: "[ $isExistPid -eq 0]" has no space before the
// bracket, so the test is a syntax error and the branch is never taken. It is
// kept verbatim rather than repaired, because repairing it would widen what the
// runner kills on machines where nothing has been killed in years. This test
// fails if someone tidies it.
func TestTheJVMSweepIsKeptExactlyAsBrokenAsItWas(t *testing.T) {
	if !strings.Contains(KillScript(false, false), "-eq 0]") {
		t.Fatal("the JVM sweep was repaired; see the comment on KillScript")
	}

	out, err := osexec.Command("bash", "-c",
		`x=1; if [ $x -eq 0]; then echo taken; else echo "not taken"; fi`).CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "not taken") {
		t.Errorf("the branch is reachable after all, so the sweep does kill JVMs: %s", out)
	}
}

func TestRestoreAndSnapshotAreValidShell(t *testing.T) {
	for name, script := range map[string]string{
		"snapshot": SnapshotScript(),
		"restore":  RestoreScript(),
	} {
		cmd := osexec.Command("bash", "-n")
		cmd.Stdin = strings.NewReader(script)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s: bash -n rejected the script: %v\n%s", name, err, out)
		}
	}
}

// Restoring is what makes each case start from the same install, and the core
// sweep in it is half of why a run can fill a disk -- the other half being the
// ulimit CTP appends to the remote profile.
func TestRestoreClearsTheStateACaseCanLeaveBehind(t *testing.T) {
	got := RestoreScript()
	for _, want := range []string{
		"${CUBRID}/conf/",
		"${CUBRID}/databases/",
		"${CUBRID}/var/",
		"${CUBRID}/log",
		`-name "core.[0-9][0-9]*"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("restore no longer clears %s:\n%s", want, got)
		}
	}
}

// Containment changes what the sweep selects on, and that is the point of B-T2
// rather than a detail of it: inside a PID namespace "everything here" is
// already "everything this run started", so there is no owner to match and
// nothing that can be matched wrongly.
func TestTheSweepSelectsByNamespaceWhenContained(t *testing.T) {
	loose := KillScript(true, false)
	tight := KillScript(true, true)

	if !strings.Contains(loose, "ps -u $USER") {
		t.Error("the uncontained sweep no longer reproduces CTP's selector")
	}
	if strings.Contains(tight, "ps -u $USER") {
		t.Error("the contained sweep still mentions $USER, which selects nothing inside")
	}
	if !strings.Contains(tight, "ps -e -f") {
		t.Error("the contained sweep lost the machine-state dump the worker log carries")
	}
	if !strings.Contains(tight, "ps -e -o pid,comm") {
		t.Error("the contained sweep does not select the namespace")
	}
	if !strings.Contains(loose, "ipcs | grep $USER") {
		t.Error("the uncontained shared-memory sweep changed")
	}
	if strings.Contains(tight, "ipcs | grep $USER") {
		t.Error("the contained shared-memory sweep still filters by owner")
	}
	// Both must still stop the services first: that line is what actually works
	// today, and it is the only reason a hung case's master ever died.
	for name, s := range map[string]string{"uncontained": loose, "contained": tight} {
		if !strings.HasPrefix(s, "cubrid service stop") {
			t.Errorf("%s sweep no longer starts with cubrid service stop", name)
		}
	}
}

// The reset is rooted at $CUBRID and every command in it is destructive. With
// the variable empty the paths do not fail, they become absolute paths at the
// root of the filesystem -- and `find ${CUBRID}/ -name "core"` becomes `find /`,
// which deletes every directory named exactly "core" on the machine.
//
// It ran. 69 of them went across this machine's /data, including
// node_modules/undici/lib/core out of a VS Code server. Files called core.js
// survived, which is the signature of that command and of nothing else.
func TestTheResetRefusesWithoutACubridInstall(t *testing.T) {
	dir := t.TempDir()
	// A directory named exactly "core", the shape the sweep destroys, somewhere
	// the script would reach if it ran from the root.
	victim := filepath.Join(dir, "node_modules", "undici", "lib", "core")
	if err := os.MkdirAll(victim, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(victim, "util.js"), []byte("//\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct{ name, cubrid string }{
		{"unset", ""},
		{"empty", `""`},
		{"a directory that is not an install", dir},
	} {
		t.Run(c.name, func(t *testing.T) {
			script := "set -e\nunset CUBRID\n"
			if c.cubrid != "" {
				script = "set -e\nexport CUBRID=" + c.cubrid + "\n"
			}
			// Run from the temporary directory so that a find rooted at "/" would
			// still have to walk to reach the victim -- what is being asserted is
			// that the script exits before any of it runs.
			cmd := osexec.Command("bash", "-c", script+RestoreScript())
			cmd.Dir = dir
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("the reset must refuse; it returned success:\n%s", out)
			}
			if !strings.Contains(string(out), "refuses to run") {
				t.Errorf("the refusal should say why:\n%s", out)
			}
			if _, serr := os.Stat(victim); serr != nil {
				t.Fatalf("a directory named core was deleted: %v", serr)
			}
		})
	}
}

// The reset's blast radius was the machine because a mount namespace shares the
// filesystem except where something is mounted over it. The process sweep is the
// other half of the same question, and the answer has to be demonstrated rather
// than read: a PID namespace does contain `ps -e`, but only if the sweep runs
// inside one and only if it was told it is contained.
func TestTheSweepCannotReachOutsideTheSlot(t *testing.T) {
	if os.Getenv("TESTKIT_CONTAINED") != "1" {
		t.Skip("not contained; run under TESTKIT_CONTAIN=1")
	}
	// A process outside the slot, named like something the sweep hunts.
	outside := osexec.Command("bash", "-c", "exec -a cub_master_decoy sleep 120")
	if err := outside.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = outside.Process.Kill()
		_ = outside.Wait()
	}()

	ns, err := contain.Open("sweeptest")
	if err != nil {
		t.Fatalf("open slot: %v", err)
	}
	defer ns.Close()

	// The sweep as a contained run issues it.
	if _, err := ns.Channel("").Run(context.Background(), KillScript(true, true)); err != nil {
		t.Logf("the sweep returned an error, which it often does: %v", err)
	}

	// Give any kill a moment to land, then check the outsider is still there.
	time.Sleep(500 * time.Millisecond)
	if err := outside.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("the sweep reached a process outside the slot: %v", err)
	}
}

// And the uncontained sweep is CTP's, which selects by $USER across the whole
// machine. That is documented as item B in docs/evidence/ctp-improvements.md;
// what must not happen is a contained run quietly taking that path.
func TestAContainedRunDoesNotUseTheMachineWideSelector(t *testing.T) {
	contained := KillScript(true, true)
	if strings.Contains(contained, "ps -u $USER") {
		t.Error("a contained sweep must not select by user: that is the selector that reaches the whole machine")
	}
	if !strings.Contains(contained, "ps -e") {
		t.Error("a contained sweep should look at its own PID namespace")
	}
	loose := KillScript(true, false)
	if !strings.Contains(loose, "ps -u $USER") {
		t.Error("the uncontained sweep is CTP's and is kept verbatim")
	}
}

// The reset refuses when $CUBRID is not an installation, and the refusal has to
// reach a log. It did not: Run reports a command's own non-zero exit in the
// Result and keeps err for not being able to run it at all, quietly checked err
// alone, and the guard exits 1 with its explanation on stderr. All three
// together meant the reset could refuse 3,244 times in silence while every case
// ran against the previous case's leftovers.
func TestARefusedResetIsReportedAndNotSwallowed(t *testing.T) {
	res, err := (&exec.Local{}).Run(context.Background(),
		"unset CUBRID\n"+RestoreScript())
	if err != nil {
		t.Fatalf("the script ran, so err must be nil: %v", err)
	}
	if res.ExitCode == 0 {
		t.Fatal("the reset must refuse without a CUBRID installation")
	}
	// The explanation is on stderr, which Output() does not carry -- so anything
	// that reports this failure has to read Stderr.
	if !strings.Contains(res.Stderr, "refuses to run") {
		t.Errorf("the refusal should explain itself on stderr, got %q / %q", res.Stdout, res.Stderr)
	}
	if strings.Contains(res.Output(), "refuses to run") {
		t.Error("this test is meaningless if the explanation is on stdout")
	}
}
