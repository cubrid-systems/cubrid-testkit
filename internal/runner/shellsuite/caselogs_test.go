package shellsuite

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCaseLogsModeIsCheckedRatherThanGuessed(t *testing.T) {
	// A misspelt mode that quietly means "off" is a run that discovers at the
	// end that it kept nothing.
	if _, err := NewCaseLogs("/x/result/shell/current_runtime_logs", "/corpus/shell", "faill", 0); err == nil {
		t.Error("a misspelt mode was accepted")
	}
	for _, mode := range []string{"", "off"} {
		l, err := NewCaseLogs("/x/result/shell/current_runtime_logs", "/corpus/shell", mode, 0)
		if err != nil || l != nil {
			t.Errorf("mode %q: got %v, %v; want off", mode, l, err)
		}
	}
}

// Beside current_runtime_logs, never inside it: that tree is the frozen surface,
// and a directory nobody expects there is a change to it.
func TestCaseLogsSitBesideTheFrozenTree(t *testing.T) {
	l, err := NewCaseLogs("/x/result/shell/current_runtime_logs", "/corpus/shell", "fail", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := l.Dir(), "/x/result/shell/case-logs"; got != want {
		t.Errorf("case logs go to %q, want %q", got, want)
	}
}

// The name is the case path. CTP names a snapshot AUTO_<build>_<datetime>, which
// is unique when one case runs at a time and stops being so at eight.
func TestTwoCasesInTheSameSecondDoNotCollide(t *testing.T) {
	const sc = "/corpus/shell"
	a := destOf("/logs", sc, sc+"/_06_issues/bug_a/cases/bug_a.sh", 1)
	b := destOf("/logs", sc, sc+"/_06_issues/bug_b/cases/bug_b.sh", 1)
	if a == b {
		t.Fatalf("two cases share a destination: %s", a)
	}
	if !strings.Contains(a, "bug_a") {
		t.Errorf("the destination does not name the case: %s", a)
	}
	// A retry is a second capture of the same case and must not overwrite the
	// first: the pair is the whole point when a case is intermittent.
	if r := destOf("/logs", sc, sc+"/_06_issues/bug_a/cases/bug_a.sh", 2); r == a {
		t.Errorf("a retry overwrote the first attempt: %s", r)
	}
}

// The tree under case-logs is the corpus's own shape. Keeping the absolute path
// buries every capture under a copy of wherever the corpus was checked out --
// which is what the first version did.
func TestTheDestinationIsTheCaseAsTheCorpusNamesIt(t *testing.T) {
	const sc = "/tmp/somewhere/deep/corpus/shell"
	got := destOf("/logs", sc, sc+"/_06_issues/_14_1h/bug_a/cases/bug_a.sh", 1)
	if want := "/logs/_06_issues/_14_1h/bug_a/cases"; got != want {
		t.Errorf("destination is %q, want %q", got, want)
	}
	// A case outside the scenario -- a hand-edited list -- still gets somewhere
	// rather than escaping the tree.
	out := destOf("/logs", sc, "/elsewhere/x/cases/x.sh", 1)
	if !strings.HasPrefix(out, "/logs/") {
		t.Errorf("a case outside the scenario escaped: %s", out)
	}
}

// The verdict decides the tier, not whether there is a capture at all.
func TestWhatEachModeKeeps(t *testing.T) {
	fail, _ := NewCaseLogs("/x/result/shell/current_runtime_logs", "/corpus/shell", "fail", 0)
	all, _ := NewCaseLogs("/x/result/shell/current_runtime_logs", "/corpus/shell", "all", 0)
	if fail.wants(true) {
		t.Error("mode=fail kept a passing case")
	}
	if !fail.wants(false) {
		t.Error("mode=fail dropped a failing case")
	}
	if !all.wants(true) || !all.wants(false) {
		t.Error("mode=all did not keep both")
	}
	var off *CaseLogs
	if off.wants(false) {
		t.Error("a nil CaseLogs kept something")
	}
}

// The expensive tier is the broker's SQL log, which is 99.6% of the log tree. A
// passing case must not pay for it.
func TestTheExpensiveTierIsOnlyForFailures(t *testing.T) {
	ok := CaptureScript("/d", "/c/cases", false)
	nok := CaptureScript("/d", "/c/cases", true)
	for _, want := range []string{"log/server", "log/*.err"} {
		if !strings.Contains(ok, want) {
			t.Errorf("a passing case does not keep %s", want)
		}
	}
	if strings.Contains(ok, "log/broker") {
		t.Error("a passing case copies the broker log, which is the expensive one")
	}
	if !strings.Contains(nok, "log/broker") {
		t.Error("a failing case does not keep the broker log")
	}
	// But the case's own files are cheap and are kept either way: comparing a
	// run that passed against one that failed is how an intermittent case is
	// diagnosed, and the passing run is half of that comparison.
	for _, script := range []string{ok, nok} {
		if !strings.Contains(script, "/case/") {
			t.Error("the case's own output is not kept")
		}
	}
	// Never the install: it is the same 748 MB for every case in a run.
	for _, script := range []string{ok, nok} {
		for _, never := range []string{"${CUBRID}/bin", "${CUBRID}/lib", "cp -rp ${CUBRID} "} {
			if strings.Contains(script, never) {
				t.Errorf("the capture copies the install: %s", never)
			}
		}
	}
	// Copy, never move: the case is still the source of truth for its verdict.
	if strings.Contains(ok, "mv ") || strings.Contains(nok, "mv ") {
		t.Error("the capture moves rather than copies")
	}
}

// A budget that is gone stops the run spending more, and says so once.
func TestTheBudgetStopsAndSaysSoOnce(t *testing.T) {
	l, err := NewCaseLogs("/x/result/shell/current_runtime_logs", "/corpus/shell", "all", 1) // 1 MB
	if err != nil {
		t.Fatal(err)
	}
	if l.spend(512 << 10) {
		t.Error("half the budget reported it was gone")
	}
	if !l.spend(512 << 10) {
		t.Error("the budget was spent and did not say so")
	}
	if l.spend(512 << 10) {
		t.Error("the budget said it was gone twice")
	}
	if !l.done() {
		t.Error("a spent budget does not stop further captures")
	}
	if s := l.Summary(); !strings.Contains(s, "budget") {
		t.Errorf("the summary does not mention the budget: %q", s)
	}
}

func TestNothingKeptSaysNothing(t *testing.T) {
	l, _ := NewCaseLogs("/x/result/shell/current_runtime_logs", "/corpus/shell", "fail", 0)
	if s := l.Summary(); s != "" {
		t.Errorf("a run that kept nothing announced %q", s)
	}
	var off *CaseLogs
	if s := off.Summary(); s != "" {
		t.Errorf("case logs off announced %q", s)
	}
}

// The script must survive a destination with a space in it, because a scenario
// path is the operator's and not ours.
func TestAPathWithASpaceIsQuoted(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "a b", "case")
	script := CaptureScript(dest, "/c/cases", true)
	if !strings.Contains(script, `"`+dest+`/server"`) && !strings.Contains(script, `"`+dest+`"`) {
		t.Errorf("the destination is not quoted:\n%s", script)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
}
