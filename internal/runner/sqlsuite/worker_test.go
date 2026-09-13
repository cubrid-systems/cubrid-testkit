package sqlsuite

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/cubrid-systems/cubrid-testkit/internal/dispatch"
	"github.com/cubrid-systems/cubrid-testkit/internal/result"
)

// scripted is an Executor that answers from a function and remembers what it
// was asked, which is how a test sees which cases the loop chose to run.
type scripted struct {
	reply func(caseFile string) ([]byte, int64, error)

	mu    sync.Mutex
	asked []string
}

func (x *scripted) Run(_ context.Context, caseFile string) ([]byte, int64, error) {
	x.mu.Lock()
	x.asked = append(x.asked, caseFile)
	x.mu.Unlock()
	return x.reply(caseFile)
}

func (x *scripted) Close() error { return nil }

func (x *scripted) was(name string) bool {
	x.mu.Lock()
	defer x.mu.Unlock()
	for _, c := range x.asked {
		if filepath.Base(c) == name+".sql" {
			return true
		}
	}
	return false
}

// always is an executor that gives every case the same rendering.
func always(rendered string) *scripted {
	return &scripted{reply: func(string) ([]byte, int64, error) { return []byte(rendered), 7, nil }}
}

// aRun is a corpus of cases in one directory with an answer for each name the
// map gives, and the records a loop writes into. The cases are returned in the
// order they were named, which is the order the queue hands them out.
func aRun(t *testing.T, out *bytes.Buffer, names []string, answers map[string]string) (*work, []string) {
	t.Helper()
	home := t.TempDir()
	scenario := filepath.Join(home, "corpus", "sql") + "/"
	dir := filepath.Join(scenario, "_01_dir")
	for _, sub := range []string{"cases", "answers"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o775); err != nil {
			t.Fatal(err)
		}
	}
	cs := &caseSet{answer: map[string]string{}}
	for _, n := range names {
		c := filepath.Join(dir, "cases", n+".sql")
		if err := os.WriteFile(c, []byte("select 1;\n"), 0o664); err != nil {
			t.Fatal(err)
		}
		cs.all = append(cs.all, c)
		if want, has := answers[n]; has {
			a := filepath.Join(dir, "answers", n+".answer")
			if err := os.WriteFile(a, []byte(want), 0o664); err != nil {
				t.Fatal(err)
			}
			cs.answer[c] = a
		}
	}
	rec, err := result.OpenSQL(result.SQLRun{
		CTPHome: home, Type: "sql", Alias: "sql", Bits: "64",
		Build: "11.5.0.0000-0000000", Scenario: scenario, Mode: "release",
		XMLSummary: true, Cases: cs.all, Answers: cs.answer, Out: out,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &work{cases: cs, rec: rec, total: len(cs.all)}, cs.all
}

// run takes the whole queue in one place, as a single-slot run does.
func (w *work) run(t *testing.T, cases []string, x Executor) error {
	t.Helper()
	return w.loop(t.Context(), "slot0", machine{}, dispatch.New(cases, 0), x)
}

// A case with no answer is in N and in neither column: CQT's loop continues
// before executing it, and its summary counts every case in total but only a
// case that ran in success or fail (SummaryInfo). So Total is not
// Success + Fail, and main.info's execute_case is the two added up.
func TestACaseWithNoAnswerIsInNAndInNeitherColumn(t *testing.T) {
	var out bytes.Buffer
	w, cases := aRun(t, &out, []string{"has", "none"}, map[string]string{"has": "ok"})
	x := always("ok")
	if err := w.run(t, cases, x); err != nil {
		t.Fatal(err)
	}

	if x.was("none") {
		t.Error("a case with no answer was handed to the executor")
	}
	if !x.was("has") {
		t.Error("the case with an answer was not run")
	}
	c, err := w.rec.End()
	if err != nil {
		t.Fatal(err)
	}
	if c.Total != 2 || c.Success != 1 || c.Fail != 0 {
		t.Errorf("counts were %+v; want 2 total, 1 success, 0 fail", c)
	}
}

func TestAFailedCaseIsAVerdictAndNotAnError(t *testing.T) {
	var out bytes.Buffer
	names := []string{"pass", "differs", "after"}
	w, cases := aRun(t, &out, names, map[string]string{"pass": "ok", "differs": "ok", "after": "ok"})
	x := &scripted{reply: func(c string) ([]byte, int64, error) {
		if filepath.Base(c) == "differs.sql" {
			return []byte("something else"), 7, nil
		}
		return []byte("ok"), 7, nil
	}}
	if err := w.run(t, cases, x); err != nil {
		t.Fatalf("a case failing ended the run: %v", err)
	}
	for _, n := range names {
		if !x.was(n) {
			t.Errorf("%s did not run", n)
		}
	}
	if c, err := w.rec.End(); err != nil || c.Fail != 1 || c.Success != 2 {
		t.Errorf("counts were %+v (%v); want 1 fail and 2 successes", c, err)
	}
	// The rendering reaches the records, which write it beside the case.
	if b, err := os.ReadFile(filepath.Join(filepath.Dir(cases[1]), "differs.result")); err != nil {
		t.Errorf("no .result beside the failed case: %v", err)
	} else if string(b) != "something else" {
		t.Errorf(".result holds %q", b)
	}
}

func TestAnExecutorThatGoesEndsTheRun(t *testing.T) {
	var out bytes.Buffer
	names := []string{"first", "gone", "never"}
	w, cases := aRun(t, &out, names, map[string]string{"first": "ok", "gone": "ok", "never": "ok"})
	x := &scripted{reply: func(c string) ([]byte, int64, error) {
		if filepath.Base(c) == "gone.sql" {
			return nil, 0, fmt.Errorf("reading the reply: %w", errExecutorGone)
		}
		return []byte("ok"), 7, nil
	}}

	q := dispatch.New(cases, 0)
	err := w.loop(t.Context(), "slot0", machine{}, q, x)
	if !errors.Is(err, errExecutorGone) {
		t.Fatalf("got %v, want errExecutorGone", err)
	}
	// Which place lost its executor, because a slotted run has several.
	if !bytes.Contains([]byte(err.Error()), []byte("slot0")) {
		t.Errorf("the error should name the place: %v", err)
	}
	if x.was("never") {
		t.Error("a case was run after the executor was gone")
	}
	// The queue is left with work in it: the run ends short rather than
	// counting the rest as anything.
	if q.Finished() {
		t.Error("the queue was drained by a run that ended early")
	}
}

func TestAnErrorOnOneCaseIsThatCasesOwn(t *testing.T) {
	var out bytes.Buffer
	names := []string{"bad", "after"}
	w, cases := aRun(t, &out, names, map[string]string{"bad": "ok", "after": "ok"})
	x := &scripted{reply: func(c string) ([]byte, int64, error) {
		if filepath.Base(c) == "bad.sql" {
			return nil, 0, errors.New("the executor could not read the case")
		}
		return []byte("ok"), 7, nil
	}}
	if err := w.run(t, cases, x); err != nil {
		t.Fatalf("one case's error ended the run: %v", err)
	}
	if !x.was("after") {
		t.Error("the run stopped at the failing case")
	}
	if c, err := w.rec.End(); err != nil || c.Fail != 1 || c.Success != 1 {
		t.Errorf("counts were %+v (%v); want 1 fail and 1 success", c, err)
	}
}

// The index on a progress line is where the case started, not where it
// finished: with slots the lines arrive out of order, and every case still
// gets exactly one number between 1 and N.
func TestEveryCaseIsNumberedOnceInStartOrder(t *testing.T) {
	var out bytes.Buffer
	var names []string
	answers := map[string]string{}
	for i := 0; i < 8; i++ {
		n := "c" + strconv.Itoa(i)
		names = append(names, n)
		answers[n] = "ok"
	}
	w, cases := aRun(t, &out, names, answers)

	// Two places on one queue, as a two-slot run is.
	q := dispatch.New(cases, 0)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = w.loop(t.Context(), "slot"+strconv.Itoa(i), machine{}, q, always("ok"))
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("slot%d: %v", i, err)
		}
	}
	if _, err := w.rec.End(); err != nil {
		t.Fatal(err)
	}

	seen := regexp.MustCompile(`\((\d+)/8 `).FindAllStringSubmatch(out.String(), -1)
	var got []int
	for _, m := range seen {
		n, _ := strconv.Atoi(m[1])
		got = append(got, n)
	}
	sort.Ints(got)
	if len(got) != 8 {
		t.Fatalf("%d progress lines for 8 cases:\n%s", len(got), out.String())
	}
	for i, n := range got {
		if n != i+1 {
			t.Fatalf("the numbers were %v; want 1..8 once each", got)
		}
	}
}

// newCores is CQT's search (CommonFileUtile.getCoreFiles), which a failed case
// runs in its own place because a slot's $CUBRID is its own.
func TestACoreIsFoundTheWayCQTFindsOne(t *testing.T) {
	cubrid := t.TempDir()
	t.Setenv("CUBRID", cubrid)
	write := func(rel string) {
		if err := os.MkdirAll(filepath.Join(cubrid, filepath.Dir(rel)), 0o775); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cubrid, rel), []byte("not really a core"), 0o664); err != nil {
			t.Fatal(err)
		}
	}
	// Not a core: the suffix has to be digits.
	write("core.notanumber")
	seen := map[string]bool{}
	if found := newCores(t.Context(), machine{}, seen); len(found) > 0 {
		t.Errorf("core.notanumber was taken for a core: %v", found)
	}

	// A core, in any case, at any depth -- and the path comes back, because it
	// is what gdb is pointed at.
	write("databases/CORE.1234")
	found := newCores(t.Context(), machine{}, seen)
	if len(found) != 1 || found[0] != filepath.Join(cubrid, "databases/CORE.1234") {
		t.Errorf("found %v, want the one core under databases/", found)
	}
	// The same core is one core: the next failed case does not report it again.
	if found := newCores(t.Context(), machine{}, seen); len(found) > 0 {
		t.Errorf("a core already reported came back again: %v", found)
	}
	write("core.7")
	if found := newCores(t.Context(), machine{}, seen); len(found) != 1 {
		t.Errorf("a core that appeared after the last search: %v", found)
	}
}

// A case that dumps core gets CQT's <case>.err beside its failure copies.
func TestACaseThatCoresGetsItsStackBesideIt(t *testing.T) {
	if _, err := osexec.LookPath("gdb"); err != nil {
		t.Skip("gdb is not on this machine")
	}
	cubrid := t.TempDir()
	t.Setenv("CUBRID", cubrid)
	t.Setenv("CORE_BACKUP_DIR", "")
	// Not a real core: gdb will refuse it, which is the path that has to leave
	// the run standing.
	if err := os.WriteFile(filepath.Join(cubrid, "core.42"), []byte("not a core"), 0o664); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	w, cases := aRun(t, &out, []string{"crashes"}, map[string]string{"crashes": "ok"})
	if err := w.run(t, cases, always("something else")); err != nil {
		t.Fatalf("a core ended the run: %v", err)
	}
	c, err := w.rec.End()
	if err != nil {
		t.Fatal(err)
	}
	if c.Fail != 1 {
		t.Errorf("counts were %+v; want the case failed", c)
	}
	// No stack, no file -- CQT writes nothing for a core it could not read.
	if _, err := os.Stat(filepath.Join(w.rec.CaseResultDir(cases[0]), "crashes.err")); !os.IsNotExist(err) {
		t.Errorf("a core gdb refused still produced a .err: %v", err)
	}
}

// The same, with a core gdb can actually read: the whole chain, from the
// verdict that triggers the search to the file a reader opens.
func TestARealCoreReachesTheErrFile(t *testing.T) {
	for _, tool := range []string{"gdb", "gcore", "sleep"} {
		if _, err := osexec.LookPath(tool); err != nil {
			t.Skipf("%s is not on this machine", tool)
		}
	}
	cubrid := t.TempDir()
	t.Setenv("CUBRID", cubrid)
	t.Setenv("CORE_BACKUP_DIR", "")

	sleep, _ := osexec.LookPath("sleep")
	cmd := osexec.Command(sleep, "300")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
	}()
	// gcore leaves core.<pid>, which is the name CQT's search looks for.
	if out, err := osexec.Command("gcore", "-o", filepath.Join(cubrid, "core"), strconv.Itoa(cmd.Process.Pid)).CombinedOutput(); err != nil {
		t.Skipf("gcore could not take a core here: %v\n%s", err, out)
	}
	core := "core." + strconv.Itoa(cmd.Process.Pid)
	if _, err := os.Stat(filepath.Join(cubrid, core)); err != nil {
		t.Skipf("gcore wrote no %s: %v", core, err)
	}

	var out bytes.Buffer
	w, cases := aRun(t, &out, []string{"crashes"}, map[string]string{"crashes": "ok"})
	if err := w.run(t, cases, always("something else")); err != nil {
		t.Fatalf("a core ended the run: %v", err)
	}
	if _, err := w.rec.End(); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(w.rec.CaseResultDir(cases[0]), "crashes.err")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no .err beside the case: %v", err)
	}
	got := string(b)
	for _, want := range []string{
		"SUMMARY:\n",
		"CORE_DIR:" + cubrid + "\n",
		core + " [",
		"\n==================" + core + "==================\n#0",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the .err has no %q:\n%s", want, got)
		}
	}
}

func TestACancelledRunStopsBetweenCases(t *testing.T) {
	var out bytes.Buffer
	names := []string{"one", "two", "three"}
	w, cases := aRun(t, &out, names, map[string]string{"one": "ok", "two": "ok", "three": "ok"})

	ctx, cancel := context.WithCancel(t.Context())
	x := &scripted{reply: func(string) ([]byte, int64, error) {
		cancel()
		return []byte("ok"), 7, nil
	}}
	if err := w.loop(ctx, "slot0", machine{}, dispatch.New(cases, 0), x); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	if len(x.asked) != 1 {
		t.Errorf("%d cases ran after the first cancelled the run", len(x.asked))
	}
}
