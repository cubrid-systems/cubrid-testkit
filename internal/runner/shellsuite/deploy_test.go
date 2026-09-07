package shellsuite

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cubrid-systems/cubrid-testkit/internal/conf"
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
		cmd := exec.Command("bash", "-n")
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

	out, err := exec.Command("bash", "-c",
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
		cmd := exec.Command("bash", "-n")
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
