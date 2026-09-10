package shellsuite

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/conf"
	"github.com/cubrid-systems/cubrid-testkit/internal/dispatch"
	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
	"github.com/cubrid-systems/cubrid-testkit/internal/feedback"
	"github.com/cubrid-systems/cubrid-testkit/internal/result"
)

func TestRunScriptRunsTheCaseFromItsOwnDirectory(t *testing.T) {
	c, err := Split("shell/_01_utility/backupdb/cases/backupdb.sh")
	if err != nil {
		t.Fatal(err)
	}
	got := RunScript(c, CaseOptions{Bits: "64", BigSpaceDir: "/big", BuildID: "11.4.0.1234"})

	for _, want := range []string{
		"cd shell/_01_utility/backupdb/cases",
		"ulimit -c unlimited",
		`if [ "$JAVA_HOME_64" ]; then`,
		"echo > backupdb.result",
		"sh backupdb.sh 2>&1",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}

	// The result file must be truncated before the case runs, or a previous
	// attempt's verdict would be read as this one's.
	if strings.Index(got, "echo > backupdb.result") > strings.Index(got, "sh backupdb.sh") {
		t.Error("the result file is truncated after the case runs")
	}
}

func TestRunScriptLeavesOutWhatIsNotConfigured(t *testing.T) {
	c, _ := Split("a/b/cases/b.sh")
	got := RunScript(c, CaseOptions{})

	for _, unwanted := range []string{"JAVA_HOME_", "CUBRID_CHARSET", "EXCLUDED_CORES_BY_ASSERT_LINE"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("unconfigured %s appeared in:\n%s", unwanted, got)
		}
	}
}

// The exported build id is spelled with three i's. Nothing in the case corpus
// reads it, but it is what cases have been able to see for years.
func TestTheBuildIDKeepsItsTypo(t *testing.T) {
	c, _ := Split("a/b/cases/b.sh")
	got := RunScript(c, CaseOptions{BuildID: "11.4"})
	if !strings.Contains(got, "export TEST_BUIILD_ID=11.4") {
		t.Errorf("TEST_BUIILD_ID was corrected; cases see the old spelling:\n%s", got)
	}
}

func TestAnEmptyHostAndUserBecomeShellExpansions(t *testing.T) {
	c, _ := Split("a/b/cases/b.sh")
	got := RunScript(c, CaseOptions{})
	for _, want := range []string{
		"export TEST_SSH_HOST=`hostname -i`",
		"export TEST_SSH_USER=`echo $USER`",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestVerdict(t *testing.T) {
	cases := []struct {
		name    string
		items   []string
		success bool
		core    bool
	}{
		{"a clean pass", []string{" : OK backupdb"}, true, false},
		{"nothing at all", nil, true, false},
		{"one NOK anywhere fails", []string{" : OK a", " : NOK b", " : OK c"}, false, false},
		{"a core", []string{" : NOK found core file /home/x/core.1"}, false, true},
		{"a fatal error", []string{" : NOK found fatal error"}, false, true},
		{"NOK in prose still fails", []string{"the answer was NOK for this case"}, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := verdictOf(c.items)
			if v.Success != c.success {
				t.Errorf("Success = %v, want %v", v.Success, c.success)
			}
			if v.HasCore != c.core {
				t.Errorf("HasCore = %v, want %v", v.HasCore, c.core)
			}
		})
	}
}

func TestABlankResultProducesBothOfCTPsLines(t *testing.T) {
	c, _ := Split("a/b/cases/b.sh")
	got := blankResultItems(c, time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC))

	if len(got) != 2 {
		t.Fatalf("got %d items, want 2", len(got))
	}
	if !strings.Contains(got[0], `a/b/cases\b.result`) {
		t.Errorf("the backslash in the first line is gone: %q", got[0])
	}
	if got[1] != " : NOK blank result" {
		t.Errorf("second line = %q", got[1])
	}
}

func TestResultItemFlagging(t *testing.T) {
	if got := resultItem("", "the case said this"); got != "the case said this" {
		t.Errorf("a case's own line was rewritten: %q", got)
	}
	if got := resultItem("NOK", "timeout"); got != " : NOK timeout" {
		t.Errorf("got %q", got)
	}
}

// ---------------------------------------------------------------------------
// A worker driving real cases.

// guardedChannel runs scripts locally, except the two that would take the
// machine down with them.
//
// KillScript kills every process the user owns whose name contains "cub", every
// JVM, every sleep, and -- when not local -- every shell script. Running it in a
// test would kill the test. RestoreScript deletes $CUBRID. Both are recorded and
// skipped, so the test can still assert that the worker asked for them.
type guardedChannel struct {
	inner *exec.Local

	// version is what the build-identity probe answers. A test machine has no
	// CUBRID on it, and refusing to run without one is the runner behaving
	// correctly, so the answer is supplied rather than the check removed.
	version string

	mu  sync.Mutex
	ran []string
}

func (g *guardedChannel) Run(ctx context.Context, script string) (exec.Result, error) {
	g.mu.Lock()
	g.ran = append(g.ran, script)
	g.mu.Unlock()

	switch {
	case strings.Contains(script, versionScript):
		v := g.version
		if v == "" {
			v = "CUBRID 11.4.5 (11.4.5.1875-74d17e9) (64bit release build for Linux) (Apr 29 2026 15:30:55)"
		}
		return exec.Result{Stdout: v}, nil
	case strings.Contains(script, "cubrid service stop"):
		return exec.Result{Stdout: "(kill skipped by the test)"}, nil
	case strings.Contains(script, ".CUBRID_SHELL_FM"):
		return exec.Result{Stdout: "(restore skipped by the test)"}, nil
	case strings.Contains(script, "do_check_more_errors"):
		return exec.Result{}, nil
	}
	return g.inner.Run(ctx, script)
}

func (g *guardedChannel) Put(ctx context.Context, l, r string) error { return g.inner.Put(ctx, l, r) }
func (g *guardedChannel) Get(ctx context.Context, r, l string) error { return g.inner.Get(ctx, r, l) }
func (g *guardedChannel) Describe() string                           { return "test" }
func (g *guardedChannel) Close() error                               { return nil }

func (g *guardedChannel) count() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.ran)
}

func (g *guardedChannel) asked(substr string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, s := range g.ran {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

// writeCase creates <root>/<name>/cases/<name>.sh containing body.
func writeCase(t *testing.T, root, name, body string) string {
	t.Helper()
	dir := filepath.Join(root, name, "cases")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name+".sh")
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// recordingFeedback keeps the case events so a test can read what feedback was
// told, which is not always what the worker log says.
type recordingFeedback struct {
	feedback.Null
	stopped []feedback.CaseStop
}

func (r *recordingFeedback) CaseStop(ev feedback.CaseStop) { r.stopped = append(r.stopped, ev) }

func (r *recordingFeedback) find(name string) *feedback.CaseStop {
	for i := range r.stopped {
		if r.stopped[i].Case == name {
			return &r.stopped[i]
		}
	}
	return nil
}

func newSink(t *testing.T) *result.Sink {
	t.Helper()
	s, err := result.Open(&conf.Home{Path: t.TempDir()}, "shell", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestAWorkerRunsCasesAndRecordsTheirVerdicts(t *testing.T) {
	root := t.TempDir()
	pass := writeCase(t, root, "passing", `echo " : OK passing" >> passing.result`)
	fail := writeCase(t, root, "failing", `echo CONSOLE-MARKER; echo " : NOK it did not work" >> failing.result`)

	ch := &guardedChannel{inner: exec.NewLocal("")}
	sink := newSink(t)
	q := dispatch.New([]string{pass, fail}, 0)

	reported := &recordingFeedback{}
	w := &Worker{
		EnvID: "env1", Channel: ch, Queue: q, Sink: sink,
		Report: reported, Local: true,
	}
	if err := w.Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	fin, err := os.ReadFile(filepath.Join(sink.Dir(), "dispatch_tc_FIN_env1.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{pass, fail} {
		if !strings.Contains(string(fin), want) {
			t.Errorf("%s is missing from the finished list:\n%s", want, fin)
		}
	}

	log, err := os.ReadFile(filepath.Join(sink.Dir(), "test_env1.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), " : OK passing") {
		t.Errorf("the passing case's result is not in the worker log:\n%s", log)
	}

	// The console output belongs to feedback, and to the worker log only once --
	// as the case produced it. CTP wrote the items to workerLog and the banner
	// plus the console to resultCont, and the two never met.
	if n := strings.Count(string(log), "CONSOLE-MARKER"); n != 1 {
		t.Errorf("the failing case's console output is in the worker log %d times, want 1:\n%s", n, log)
	}
	if strings.Contains(string(log), consoleBanner) {
		t.Error("the console banner reached the worker log; CTP put it in resultCont only")
	}
	failed := reported.find(fail)
	if failed == nil {
		t.Fatal("feedback never heard about the failing case")
	}
	if !strings.Contains(failed.ResultText, consoleBanner) ||
		!strings.Contains(failed.ResultText, "CONSOLE-MARKER") {
		t.Errorf("feedback did not get the banner and the console output:\n%s", failed.ResultText)
	}

	if !ch.asked("cubrid service stop") {
		t.Error("the worker never asked for a process reset")
	}
	if !ch.asked(".CUBRID_SHELL_FM") {
		t.Error("the worker never asked to restore CUBRID")
	}
}

// A case that writes nothing is not a pass. CTP re-read the file six times a
// second apart before deciding, which is a real wait; the test shortens nothing
// and instead checks the two items it produces.
func TestACaseThatWritesNoResultFails(t *testing.T) {
	root := t.TempDir()
	silent := writeCase(t, root, "silent", `true`)

	ch := &guardedChannel{inner: exec.NewLocal("")}
	sink := newSink(t)
	q := dispatch.New([]string{silent}, 0)

	w := &Worker{EnvID: "env1", Channel: ch, Queue: q, Sink: sink, Report: feedback.Null{}, Local: true}
	if err := w.Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	log, err := os.ReadFile(filepath.Join(sink.Dir(), "test_env1.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), "blank result") {
		t.Errorf("a silent case was not recorded as blank:\n%s", log)
	}
}

// A retried case has produced no verdict yet: it must not be printed and must not
// be written to the finished list, or resuming would skip a case that never
// passed.
func TestARetriedCaseIsNotYetRecordedAsFinished(t *testing.T) {
	root := t.TempDir()
	flaky := writeCase(t, root, "flaky", strings.Join([]string{
		`n=$(cat /tmp/none 2>/dev/null || cat .attempts 2>/dev/null || echo 0)`,
		`n=$((n+1)); echo $n > .attempts`,
		`if [ "$n" -ge 2 ]; then echo " : OK flaky" >> flaky.result;`,
		`else echo " : NOK not yet" >> flaky.result; fi`,
	}, "\n"))

	ch := &guardedChannel{inner: exec.NewLocal("")}
	sink := newSink(t)
	q := dispatch.New([]string{flaky}, 1)

	w := &Worker{EnvID: "env1", Channel: ch, Queue: q, Sink: sink, Report: feedback.Null{}, Local: true}
	if err := w.Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	fin, err := os.ReadFile(filepath.Join(sink.Dir(), "dispatch_tc_FIN_env1.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(fin), flaky); got != 1 {
		t.Errorf("the case appears %d times in the finished list, want 1:\n%s", got, fin)
	}

	log, err := os.ReadFile(filepath.Join(sink.Dir(), "test_env1.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), " : NOK not yet") {
		t.Errorf("the first attempt is missing, so nothing was retried:\n%s", log)
	}
	if !strings.Contains(string(log), " : OK flaky") {
		t.Errorf("the retry is missing:\n%s", log)
	}
}

func TestAMonitorIsInertWithoutATimeout(t *testing.T) {
	ch := &guardedChannel{inner: exec.NewLocal("")}
	m := &Monitor{Worker: &Worker{}, Channel: ch, Timeout: 0}

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	m.Watch(ctx) // returns immediately rather than polling until the deadline

	if n := ch.count(); n != 0 {
		t.Errorf("a disabled monitor still ran %d scripts", n)
	}
}

// The monitor does not cancel the case. Cancelling closes the channel and leaves
// the case's processes running on the far machine; killing them makes the case's
// own command return, which is the only way the next case starts clean.
func TestTheMonitorKillsTheProcessesRatherThanTheConnection(t *testing.T) {
	ch := &guardedChannel{inner: exec.NewLocal("")}
	w := &Worker{EnvID: "env1", Channel: ch, Sink: newSink(t), Report: feedback.Null{}, Local: true}
	w.beginCase(time.Now().Add(-time.Hour), "a/b/cases/b.sh", func() {})

	m := &Monitor{Worker: w, Channel: ch, Timeout: time.Second, Local: true, EscalateAfter: -1}
	m.check(t.Context())

	if !ch.asked("cubrid service stop") {
		t.Error("the monitor did not kill anything")
	}
	if !w.tookTooLong() {
		t.Error("the case was not marked as timed out")
	}
}

// CTP resolves again every three seconds for as long as the case keeps running:
// resolveTimeout leaves test.startTime set. This used to resolve once, because
// markTimedOut cleared the start time and every later check gave up before doing
// anything.
func TestTheMonitorKeepsResolvingWhileTheCaseRuns(t *testing.T) {
	ch := &guardedChannel{inner: exec.NewLocal("")}
	w := &Worker{EnvID: "env1", Channel: ch, Sink: newSink(t), Report: feedback.Null{}, Local: true}
	w.beginCase(time.Now().Add(-time.Hour), "a/b/cases/b.sh", func() {})

	m := &Monitor{Worker: w, Channel: ch, Timeout: time.Second, Local: true, EscalateAfter: -1}
	m.check(t.Context())
	first := ch.count()
	m.check(t.Context())

	if second := ch.count(); second <= first {
		t.Errorf("the monitor resolved once and stopped: %d scripts after one pass, %d after two", first, second)
	}
}

// A sweep that does not free the case used to leave the worker blocked for good.
// docs/evidence/regression-shell.md records the run where that happened.
func TestTheMonitorEndsACaseTheSweepDidNotFree(t *testing.T) {
	ch := &guardedChannel{inner: exec.NewLocal("")}
	w := &Worker{EnvID: "env1", Channel: ch, Sink: newSink(t), Report: feedback.Null{}, Local: true}

	aborted := false
	w.beginCase(time.Now().Add(-time.Hour), "a/b/cases/b.sh", func() { aborted = true })

	m := &Monitor{Worker: w, Channel: ch, Timeout: time.Second, Local: true, EscalateAfter: time.Nanosecond}
	m.check(t.Context()) // resolves, and starts the grace period
	if aborted {
		t.Error("the monitor gave up on the first pass, before the sweep had a chance")
	}
	m.check(t.Context()) // the grace period has passed; the case is still running
	if !aborted {
		t.Error("the case was never ended, so the worker would wait for it for ever")
	}
}

// Off means off: a run that wants CTP's behaviour unaltered gets it.
func TestEscalationCanBeTurnedOff(t *testing.T) {
	ch := &guardedChannel{inner: exec.NewLocal("")}
	w := &Worker{EnvID: "env1", Channel: ch, Sink: newSink(t), Report: feedback.Null{}, Local: true}

	aborted := false
	w.beginCase(time.Now().Add(-time.Hour), "a/b/cases/b.sh", func() { aborted = true })

	m := &Monitor{Worker: w, Channel: ch, Timeout: time.Second, Local: true, EscalateAfter: -1}
	m.check(t.Context())
	m.check(t.Context())
	if aborted {
		t.Error("escalation ran with a negative grace period")
	}
}
