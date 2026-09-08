package shellsuite

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cubrid-systems/cubrid-testkit/internal/cli"
	"github.com/cubrid-systems/cubrid-testkit/internal/conf"
	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
	"github.com/cubrid-systems/cubrid-testkit/internal/runner"
	"github.com/cubrid-systems/cubrid-testkit/internal/topology"
)

func request(t *testing.T, body string) (runner.Request, string) {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "conf"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "conf", "shell.conf")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &conf.Home{Path: home}
	cfg, err := h.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return runner.Request{Task: cli.Shell, Home: h, ConfigPath: path, Config: cfg}, home
}

func TestValidate(t *testing.T) {
	t.Run("a configuration with no scenario cannot describe a run", func(t *testing.T) {
		req, _ := request(t, "env.instance1.ssh.host=localhost\n")
		err := NewShell().Validate(req)
		var exit *runner.ExitError
		if err == nil {
			t.Fatal("a scenario-less configuration was accepted")
		}
		if !asExit(err, &exit) || exit.Code != 255 {
			t.Errorf("got %v, want exit 255", err)
		}
	})

	// CTP added an environment called "local" when none was configured, and only
	// then checked for an empty list -- so its "Not found any environment
	// instance" error could never fire. A run that names no machine is how the
	// suite is driven against the engine on the machine you are sitting at.
	t.Run("a configuration with no machines is a local run", func(t *testing.T) {
		req, _ := request(t, "scenario=/somewhere\n")
		if err := NewShell().Validate(req); err != nil {
			t.Fatalf("a local run was refused: %v", err)
		}
	})

	t.Run("asking for a case update fails rather than being ignored", func(t *testing.T) {
		req, _ := request(t, "scenario=/somewhere\ntestcase_update_yn=yes\n")
		if err := NewShell().Validate(req); err == nil {
			t.Fatal("testcase_update_yn=yes was accepted and would have been ignored")
		}
	})
}

func asExit(err error, target **runner.ExitError) bool {
	e, ok := err.(*runner.ExitError)
	if ok {
		*target = e
	}
	return ok
}

func TestLocalInstanceCarriesTheDefaultRoles(t *testing.T) {
	req, _ := request(t, strings.Join([]string{
		"scenario=/somewhere",
		"default.cubrid.cubrid_port_id=1723",
		"default.ssh.user=qa",
	}, "\n"))

	inst := topology.Local(req.Config)
	if inst.EnvID() != "local" {
		t.Errorf("EnvID = %q, want local", inst.EnvID())
	}
	if !inst.IsLocal() {
		t.Error("the local instance does not say it is local")
	}
	if got := inst.Role(topology.RoleCUBRID)["cubrid_port_id"]; got != "1723" {
		t.Errorf("the default engine role did not reach the local instance: %q", got)
	}
	if got := inst.SSH().User; got != "qa" {
		t.Errorf("the default ssh role did not reach the local instance: %q", got)
	}
}

// The whole task, over real case scripts, with the two destructive steps
// intercepted.
//
// A real local run resets processes before every case, which on this machine
// means killing anything whose name contains "cub", every JVM, every sleep, and
// releasing every shared-memory segment the user holds. That cannot run in a
// test, so the channel records those requests and skips them -- which also proves
// the runner asked.
func TestTheWholeTaskOverRealCases(t *testing.T) {
	scenario := t.TempDir()
	writeCase(t, scenario, "passing", `echo " : OK passing" >> passing.result`)
	writeCase(t, scenario, "failing", `echo " : NOK it did not work" >> failing.result`)

	// A helper sharing a cases/ directory. It must not be run as a case.
	helpers := filepath.Join(scenario, "passing", "cases")
	if err := os.WriteFile(filepath.Join(helpers, "PrintInfo.sh"),
		[]byte("echo helper ran >> /dev/null\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	req, home := request(t, strings.Join([]string{
		"scenario=" + scenario,
		"test_category=shell",
		"testcase_retry_num=0",
	}, "\n"))

	ch := &guardedChannel{inner: exec.NewLocal("")}
	s := &Shell{Channels: func(*topology.Instance) (exec.Channel, exec.Channel, error) {
		return ch, ch, nil
	}}

	if err := s.Run(t.Context(), req); err != nil {
		t.Fatalf("the run failed: %v", err)
	}

	dir := filepath.Join(home, "result", "shell", "current_runtime_logs")

	all := readFile(t, filepath.Join(dir, "dispatch_tc_ALL.txt"))
	if strings.Contains(all, "PrintInfo.sh") {
		t.Errorf("a helper was dispatched as a case:\n%s", all)
	}
	if n := strings.Count(strings.TrimSpace(all), "\n") + 1; n != 2 {
		t.Errorf("dispatched %d cases, want 2:\n%s", n, all)
	}

	fin := readFile(t, filepath.Join(dir, "dispatch_tc_FIN_local.txt"))
	for _, want := range []string{"passing.sh", "failing.sh"} {
		if !strings.Contains(fin, want) {
			t.Errorf("%s never finished:\n%s", want, fin)
		}
	}

	status := readFile(t, filepath.Join(dir, "test_status.data"))
	for _, want := range []string{
		"total_case_count=2",
		"total_success_case_count=1",
		"total_fail_case_count=1",
	} {
		if !strings.Contains(status, want) {
			t.Errorf("missing %q in:\n%s", want, status)
		}
	}

	var v any
	report := readFile(t, filepath.Join(dir, "test-shell.xml"))
	if err := xml.Unmarshal([]byte(report), &v); err != nil {
		t.Errorf("the report does not parse: %v\n%s", err, report)
	}

	// main_snapshot.properties records what was tested, and monitor_local.log is
	// created whether or not anything is written to it.
	for _, name := range []string{"main_snapshot.properties", "feedback.log", "current_task_id"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s was not written: %v", name, err)
		}
	}

	if !ch.asked("cubrid service stop") {
		t.Error("no process reset was requested")
	}
	if !ch.asked(".CUBRID_SHELL_FM") {
		t.Error("no snapshot or restore was requested")
	}
}

// A run that finds nothing says so and stops, and that is not a failure.
func TestARunWithNoCasesIsNotAFailure(t *testing.T) {
	req, _ := request(t, "scenario="+t.TempDir()+"\ntest_category=shell\n")

	ch := &guardedChannel{inner: exec.NewLocal("")}
	s := &Shell{Channels: func(*topology.Instance) (exec.Channel, exec.Channel, error) {
		return ch, ch, nil
	}}

	if err := s.Run(t.Context(), req); err != nil {
		t.Fatalf("an empty scenario was reported as a failure: %v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return string(body)
}

// The runner runs on one machine (ADR-014). A configuration that names several
// describes a fleet, and a fleet is the operations layer's -- so the first is
// used and the rest are named rather than silently dropped.
func TestSeveralConfiguredMachinesBecomeOneAndAWarning(t *testing.T) {
	req, _ := request(t, strings.Join([]string{
		"scenario=/somewhere",
		"env.instance1.ssh.host=alpha",
		"env.instance2.ssh.host=beta",
		"env.instance3.ssh.host=gamma",
	}, "\n"))

	configured, err := topology.From(req.Config)
	if err != nil {
		t.Fatal(err)
	}
	machine, extra := oneMachine(req.Config, configured)

	if machine.EnvID() != "env1" {
		t.Errorf("chose %s, want the first configured machine", machine.EnvID())
	}
	if len(extra) != 2 || extra[0] != "env2" || extra[1] != "env3" {
		t.Errorf("unused machines = %v, want [env2 env3] so they can be reported", extra)
	}
}

func TestNoConfiguredMachineIsThisMachine(t *testing.T) {
	req, _ := request(t, "scenario=/somewhere\n")

	configured, err := topology.From(req.Config)
	if err != nil {
		t.Fatal(err)
	}
	machine, extra := oneMachine(req.Config, configured)

	if !machine.IsLocal() {
		t.Errorf("chose %s, want the local machine", machine.EnvID())
	}
	if len(extra) != 0 {
		t.Errorf("nothing was configured, so nothing can be left out: %v", extra)
	}
}

// Slots are only for the real opener, and the guard for that must not depend on
// a field the constructor fills in. It did once: NewShell set Channels, so the
// guard was false for every real run and parallel_slots stopped opening any slot
// at all -- a four-slot run reported one.
func TestTheRealShellCanOpenSlots(t *testing.T) {
	if s := NewShell(); s.Channels != nil {
		t.Error("NewShell set Channels, which makes the run look like a caller supplied its own")
	}
}
