package shellsuite

import (
	"context"
	"encoding/xml"
	"io"
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
	// The request carries the path; the runner reads the file itself.
	if _, err := h.Load(path); err != nil {
		t.Fatal(err)
	}
	return runner.Request{Task: cli.Shell, Home: h, ConfigPath: path}, home
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

	inst := topology.Local(configOf(req))
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

// cp over a corpus meets what a run as root left in it and exits 1. The run goes
// on with what came across, as CTP's did, and -- unlike CTP's -- says what did
// not. Its status went unread before, and nothing was said.
func TestAScenarioNotCopiedWholeIsSaidAndTheRunGoesOn(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "ws")
	req, _ := request(t, "scenario=/corpus\ntestcase_workspace_dir="+workspace+"\n")
	ch := answers{{"cp -r", exec.Result{ExitCode: 1,
		Stderr: "cp: cannot open '/corpus/a/cases/db/lob': Permission denied"}}}

	var got string
	var err error
	out := printed(t, func() {
		got, err = NewShell().prepareWorkspace(t.Context(), ch, configOf(req), true)
	})
	if err != nil || got != workspace {
		t.Fatalf("the run stopped over what cp could not read: %q, %v", got, err)
	}
	if !strings.Contains(out, "[WARN]") || !strings.Contains(out, "Permission denied") {
		t.Errorf("what was not copied was not said:\n%s", out)
	}
}

// find does the same, and discovery keeps what it could list.
func TestDiscoveryGoesOnPastWhatItCannotRead(t *testing.T) {
	ch := answers{{"find ", exec.Result{ExitCode: 1,
		Stdout: "/corpus/a/cases/a.sh\n",
		Stderr: "find: '/corpus/b/cases/db/lob': Permission denied"}}}

	var cases []string
	var err error
	out := printed(t, func() { cases, err = Discover(t.Context(), ch, "/corpus") })
	if err != nil || len(cases) != 1 {
		t.Fatalf("discovery gave up on what it could read: %v, %v", cases, err)
	}
	if !strings.Contains(out, "[WARN]") || !strings.Contains(out, "Permission denied") {
		t.Errorf("what could not be read was not said:\n%s", out)
	}

	// Only 1 means that. A find that was killed listed some of the corpus, and
	// that list must not be recorded as the whole run.
	killed := answers{{"find ", exec.Result{ExitCode: -1, Stdout: "/corpus/a/cases/a.sh\n"}}}
	if _, err := Discover(t.Context(), killed, "/corpus"); err == nil {
		t.Error("a find that was killed was taken for a whole list")
	}
}

// An exclusion list that is not there stops the run. CTP read nothing from it
// and ran every case it was meant to keep out.
func TestAMissingExclusionListStopsTheRun(t *testing.T) {
	req, _ := request(t, "scenario=/ws\ntestcase_exclude_from_file=/nowhere/excluded.txt\n")
	ch := answers{
		{"cat ", exec.Result{ExitCode: 1, Stderr: "cat: /nowhere/excluded.txt: No such file or directory"}},
		{"find ", exec.Result{Stdout: "/ws/a/cases/a.sh\n"}},
	}
	_, _, _, err := (&Shell{}).caseList(t.Context(), ch, newSink(t), configOf(req), "/ws", false)
	if err == nil || !strings.Contains(err.Error(), "No such file") {
		t.Errorf("a missing exclusion list was read as an empty one: %v", err)
	}
}

// printed runs f and returns what it wrote to standard output.
func printed(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	// Deferred, so that an f that fails the test does not leave every later
	// test writing into a pipe nobody reads.
	func() {
		defer func() {
			os.Stdout = saved
			w.Close()
		}()
		f()
	}()
	return <-done
}

// grep answers 1 when nothing matched, and a macro no case names is no reason to
// stop a run. Its 2 is a file it could not read, and a skip list built without
// that file would run the cases the macro is there to keep out.
func TestTheMacroSkipTellsNoMatchFromAnUnreadableFile(t *testing.T) {
	req, _ := request(t, "scenario=/ws\ntestcase_exclude_by_macro=LINUX_NOT_SUPPORTED\n")
	found := exec.Result{Stdout: "/ws/a/cases/a.sh\n"}

	nothing := answers{{"grep ", exec.Result{ExitCode: 1}}, {"find ", found}}
	cases, skipped, _, err := (&Shell{}).caseList(t.Context(), nothing, newSink(t), configOf(req), "/ws", false)
	if err != nil || len(cases) != 1 || len(skipped) != 0 {
		t.Errorf("no case names the macro, and got %v, %v, %v", cases, skipped, err)
	}

	unreadable := answers{
		{"grep ", exec.Result{ExitCode: 2, Stderr: "grep: /ws/b/cases/b.sh: Permission denied"}},
		{"find ", found},
	}
	_, _, _, err = (&Shell{}).caseList(t.Context(), unreadable, newSink(t), configOf(req), "/ws", false)
	if err == nil || !strings.Contains(err.Error(), "Permission denied") {
		t.Errorf("a case file grep could not read was ignored: %v", err)
	}

	// Nor is a grep that never finished -- killed, or cancelled with the run.
	killed := answers{{"grep ", exec.Result{ExitCode: -1}}, {"find ", found}}
	if _, _, _, err := (&Shell{}).caseList(t.Context(), killed, newSink(t), configOf(req), "/ws", false); err == nil {
		t.Error("a grep that was killed was read as one that matched nothing")
	}
}

// answers is a channel that replies to a script by the first fragment it
// contains, and with an empty success to anything else.
type answers []struct {
	fragment string
	res      exec.Result
}

func (a answers) Run(_ context.Context, script string) (exec.Result, error) {
	for _, x := range a {
		if strings.Contains(script, x.fragment) {
			return x.res, nil
		}
	}
	return exec.Result{}, nil
}
func (answers) Put(context.Context, string, string) error { return nil }
func (answers) Get(context.Context, string, string) error { return nil }
func (answers) Describe() string                          { return "answers" }
func (answers) Close() error                              { return nil }

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

	configured, err := topology.From(configOf(req))
	if err != nil {
		t.Fatal(err)
	}
	machine, extra := oneMachine(configOf(req), configured)

	if machine.EnvID() != "env1" {
		t.Errorf("chose %s, want the first configured machine", machine.EnvID())
	}
	if len(extra) != 2 || extra[0] != "env2" || extra[1] != "env3" {
		t.Errorf("unused machines = %v, want [env2 env3] so they can be reported", extra)
	}
}

func TestNoConfiguredMachineIsThisMachine(t *testing.T) {
	req, _ := request(t, "scenario=/somewhere\n")

	configured, err := topology.From(configOf(req))
	if err != nil {
		t.Fatal(err)
	}
	machine, extra := oneMachine(configOf(req), configured)

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

// The machine has to be able to hold the whole ceiling, not the gate.
//
// The gate holds back new cases only; the ones already running fill the rest,
// and they do -- a 24-slot run with the gate at 80% finished with 22,528 MB of
// its 22,528 MB ceiling in use. Comparing the gate instead is what the first
// version of this check did, and it stayed silent for the run systemd-oomd then
// killed at 2,951 of 3,204 cases.
func TestTheWholeCeilingIsComparedAgainstTheMachine(t *testing.T) {
	const perSlotMB = 175
	for _, c := range []struct {
		name                    string
		limit, slots, avl, high int
		wantWarn                bool
	}{
		// The run that was killed: 22528 + 24*175 = 26728 against 25108 free.
		// The old check compared 22528*80% = 18022 and said nothing.
		{"the ceiling that was killed", 22528, 24, 25108, 80, true},
		{"the same run, gate only", 22528, 24, 25108, 80, true},
		{"lowered until it fits", 12288, 24, 25108, 80, false},
		{"what sizing.sh recommends", 8672, 8, 11688, 80, false},
		{"a ceiling that fits but the slots do not", 8672, 24, 11688, 80, true},
		{"no ceiling configured", 0, 24, 25108, 80, false},
		// MemAvailable unreadable: judging on a zero would warn about every run.
		{"the machine will not say", 22528, 24, 0, 80, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			reserve := c.slots * perSlotMB
			got := c.limit > 0 && c.avl > 0 && c.limit+reserve > c.avl
			if got != c.wantWarn {
				t.Errorf("ceiling=%d slots=%d available=%d MB: need %d, warned=%v want %v",
					c.limit, c.slots, c.avl, c.limit+reserve, got, c.wantWarn)
			}
		})
	}
}

// And the number it reads is the kernel's own, not a total that includes what
// is already spoken for.
func TestMemAvailableIsWhatTheKernelWillGive(t *testing.T) {
	got := memAvailableMB()
	if got < 0 {
		t.Fatalf("negative: %d", got)
	}
	if _, err := os.ReadFile("/proc/meminfo"); err != nil {
		if got != 0 {
			t.Errorf("no /proc/meminfo here, so it must answer 0, got %d", got)
		}
		return
	}
	if got == 0 {
		t.Error("this machine has /proc/meminfo and MemAvailable was not read")
	}
}

// feedback_type was in the configuration reference, in the recommended settings
// and in feedback's own doc comments, and no code read it: every run wrote a
// feedback.log whatever the file said. It is read now, and what each answer
// means is migration-exclusions.md's decision.
func TestFeedbackTypeDecidesWhetherARunKeepsFeedback(t *testing.T) {
	for _, c := range []struct {
		value string
		kept  bool
	}{
		{"", true},
		{"file", true},
		{"db", true},    // the events are kept; only their destination changes
		{"none", false}, // CTP's own "no feedback at all"
	} {
		t.Run("feedback_type="+c.value, func(t *testing.T) {
			scenario := t.TempDir()
			writeCase(t, scenario, "passing", `echo " : OK passing" >> passing.result`)
			lines := []string{"scenario=" + scenario, "test_category=shell", "testcase_retry_num=0"}
			if c.value != "" {
				lines = append(lines, "feedback_type="+c.value)
			}
			req, home := request(t, strings.Join(lines, "\n"))

			ch := &guardedChannel{inner: exec.NewLocal("")}
			s := &Shell{Channels: func(*topology.Instance) (exec.Channel, exec.Channel, error) {
				return ch, ch, nil
			}}
			if err := s.Run(t.Context(), req); err != nil {
				t.Fatalf("the run failed: %v", err)
			}

			log := filepath.Join(home, "result", "shell", "current_runtime_logs", "feedback.log")
			_, err := os.Stat(log)
			if c.kept && err != nil {
				t.Errorf("no feedback.log: %v", err)
			}
			if !c.kept && err == nil {
				t.Error("a run told to keep no feedback wrote feedback.log anyway")
			}
		})
	}
}
