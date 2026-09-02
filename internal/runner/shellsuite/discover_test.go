package shellsuite

import (
	"context"
	"os"
	osexec "os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
)

func TestIsCaseWantsTheScriptNamedAfterItsDirectory(t *testing.T) {
	cases := []struct {
		path string
		want bool
		why  string
	}{
		{"shell/_05_addition/loaddb_10001/cases/loaddb_10001.sh", true, "the shape every real case has"},
		{"shell/_05_addition/recovery/cases/common.sh", false, "a helper sourced by the case"},
		{"shell/_05_addition/csql_autocommit/cases/PrintInfo.sh", false, "the most common helper, 38 copies"},
		{"shell/_05_addition/bug_sus583/cases/bug_sus495.sh", false, "another case's script left in the wrong directory"},
		{"shell/_07_index/_01_deadlock/cases/tc_ds_01.sh", false, "a whole directory CTP never runs"},
		{"a/b.sh", false, "too short to have a grandparent"},
		{"cases/x.sh", false, "no grandparent either"},
	}
	for _, c := range cases {
		if got := IsCase(c.path); got != c.want {
			t.Errorf("IsCase(%q) = %v, want %v -- %s", c.path, got, c.want, c.why)
		}
	}
}

func TestSplitDerivesTheDirectoryAndTheResultFile(t *testing.T) {
	got, err := Split("shell/_01_utility/backupdb/cases/backupdb.sh")
	if err != nil {
		t.Fatal(err)
	}
	want := Case{
		Path:   "shell/_01_utility/backupdb/cases/backupdb.sh",
		Dir:    "shell/_01_utility/backupdb/cases",
		Script: "backupdb.sh",
		Result: "backupdb.result",
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// CTP split on the *substring* "cases", so a case directory whose name ends in
// "cases" would send the worker to a directory that does not exist. Matching the
// path segment gives the same answer on every real case and the right one here.
func TestSplitIsNotFooledByCasesInsideAName(t *testing.T) {
	const p = "shell/_05_addition/loadcases/cases/loadcases.sh"

	got, err := Split(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.Dir != "shell/_05_addition/loadcases/cases" {
		t.Errorf("Dir = %q, want the cases directory", got.Dir)
	}

	// What CTP would have done, kept here so the difference stays visible.
	at := strings.LastIndex(p, "cases")
	if ctpDir := p[:at+5]; ctpDir == got.Dir {
		t.Fatal("this path no longer distinguishes the two rules; pick another")
	}
}

func TestSplitRejectsAPathWithNoCasesSegment(t *testing.T) {
	for _, p := range []string{"shell/foo/foo.sh", "", "cases"} {
		if _, err := Split(p); err == nil {
			t.Errorf("Split(%q) succeeded, want an error", p)
		}
	}
}

func TestParseSkipped(t *testing.T) {
	const key = "SKIP_THIS_CASE"
	out := ParseSkipped(strings.Join([]string{
		"a/cases/a.sh:SKIP_THIS_CASE",
		"b/cases/b.sh:  SKIP_THIS_CASE  ",
		"c/cases/c.sh:\tSKIP_THIS_CASE",
		"d/cases/d.sh:# not the key",
		"e/cases/e.sh:SKIP_THIS_CASE: because of CUBRIDQA-1",
		"f/cases/f.sh:",
		"garbage with no colon",
		"",
	}, "\n"), key)

	want := []string{"a/cases/a.sh", "b/cases/b.sh", "c/cases/c.sh"}
	if !slices.Equal(out, want) {
		t.Errorf("got %v, want %v", out, want)
	}
}

// Worth its own test because it is a trap, not a design: a skip macro that
// explains itself with a colon splits into three fields and is ignored, so the
// case someone meant to disable runs anyway.
func TestASkipMacroContainingAColonIsIgnored(t *testing.T) {
	out := ParseSkipped("e/cases/e.sh:SKIP: see CUBRIDQA-1", "SKIP")
	if len(out) != 0 {
		t.Errorf("got %v, want nothing skipped", out)
	}
}

func TestParseExcluded(t *testing.T) {
	out := ParseExcluded(strings.Join([]string{
		"# a comment",
		"-- another comment style",
		"",
		"  _05_addition/recovery  ",
		"_11_pl/cases/",
	}, "\n"))

	want := []string{"_05_addition/recovery/", "_11_pl/cases/"}
	if !slices.Equal(out, want) {
		t.Errorf("got %v, want %v", out, want)
	}
}

func TestExcludeRemovesMatchesAndReportsThemBackwards(t *testing.T) {
	cases := []string{
		"shell/_05_addition/aaa/cases/aaa.sh",
		"shell/_05_addition/recovery/cases/recovery.sh",
		"shell/_05_addition/recovery2/cases/recovery2.sh",
		"shell/_06_issues/bbb/cases/bbb.sh",
	}
	kept, removed := Exclude(cases, []string{"_05_addition/recovery/", "_06_issues/"})

	wantKept := []string{
		"shell/_05_addition/aaa/cases/aaa.sh",
		"shell/_05_addition/recovery2/cases/recovery2.sh",
	}
	if !slices.Equal(kept, wantKept) {
		t.Errorf("kept %v, want %v", kept, wantKept)
	}
	wantRemoved := []string{
		"shell/_05_addition/recovery/cases/recovery.sh",
		"shell/_06_issues/bbb/cases/bbb.sh",
	}
	if !slices.Equal(removed, wantRemoved) {
		t.Errorf("removed %v, want %v", removed, wantRemoved)
	}
}

// The trailing slash is what stops "recovery" from taking "recovery2" with it.
func TestExcludeDoesNotMatchALongerSiblingName(t *testing.T) {
	cases := []string{"shell/x/recovery2/cases/recovery2.sh"}
	kept, removed := Exclude(cases, ParseExcluded("recovery"))
	if len(removed) != 0 {
		t.Errorf("removed %v, want nothing", removed)
	}
	if len(kept) != 1 {
		t.Errorf("kept %v, want the case", kept)
	}
}

type fakeChannel struct{ out string }

func (f fakeChannel) Run(context.Context, string) (exec.Result, error) {
	return exec.Result{Stdout: f.out}, nil
}
func (fakeChannel) Put(context.Context, string, string) error { return nil }
func (fakeChannel) Get(context.Context, string, string) error { return nil }
func (fakeChannel) Describe() string                          { return "fake" }
func (fakeChannel) Close() error                              { return nil }

func TestDiscoverFiltersHelpersAndSorts(t *testing.T) {
	ch := fakeChannel{out: strings.Join([]string{
		"shell/z/cases/z.sh",
		"shell/a/cases/PrintInfo.sh",
		"shell/a/cases/a.sh",
		"",
		"  shell/m/cases/m.sh  ",
	}, "\n")}

	got, err := Discover(t.Context(), ch, "shell")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"shell/a/cases/a.sh", "shell/m/cases/m.sh", "shell/z/cases/z.sh"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// find's order is readdir's order, and readdir's order is not reproducible: three
// consecutive runs over the same tree give three different lists. Sorting is what
// makes two runs of the same suite comparable at all.
func TestDiscoverIsIndependentOfFindOrder(t *testing.T) {
	lines := []string{"shell/a/cases/a.sh", "shell/b/cases/b.sh", "shell/c/cases/c.sh"}

	first, err := Discover(t.Context(), fakeChannel{out: strings.Join(lines, "\n")}, "shell")
	if err != nil {
		t.Fatal(err)
	}
	slices.Reverse(lines)
	second, err := Discover(t.Context(), fakeChannel{out: strings.Join(lines, "\n")}, "shell")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(first, second) {
		t.Errorf("shuffling find's output changed the dispatch list: %v vs %v", first, second)
	}
}

// The rule in one place: a real tree, walked by IsCase and by CTP's awk, must give
// the same list. Set TESTKIT_SHELL_CORPUS to the checkout of
// cubrid-testcases-private-ex to run it.
func TestAgreesWithCTPsAwkOnTheRealCorpus(t *testing.T) {
	root := os.Getenv("TESTKIT_SHELL_CORPUS")
	if root == "" {
		t.Skip("set TESTKIT_SHELL_CORPUS to a cubrid-testcases-private-ex checkout")
	}
	scenario := filepath.Join(root, "shell")
	if _, err := os.Stat(scenario); err != nil {
		t.Skipf("no shell tree under %s", root)
	}

	// CTP: find ... | xargs -i echo {} | awk -F "/" '{ if( $(NF-2)".sh"== $NF) print }'
	cmd := osexec.Command("bash", "-c",
		`find "$1" -name "*.sh" -type f -print | awk -F "/" '{ if( $(NF-2)".sh"== $NF) print }'`,
		"bash", scenario)
	raw, err := cmd.Output()
	if err != nil {
		t.Fatalf("running CTP's discovery: %v", err)
	}
	var theirs []string
	for _, l := range strings.Split(string(raw), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			theirs = append(theirs, l)
		}
	}
	slices.Sort(theirs)

	var ours []string
	err = filepath.WalkDir(scenario, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".sh") {
			return err
		}
		if IsCase(p) {
			ours = append(ours, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(ours)

	if !slices.Equal(ours, theirs) {
		t.Fatalf("IsCase and CTP's awk disagree: %d vs %d cases", len(ours), len(theirs))
	}
	t.Logf("%d cases; %d scripts sit directly in a cases/ directory without being one",
		len(ours), countUnderCases(t, scenario)-len(ours))

	// And every one of them splits the same way under both rules.
	for _, p := range ours {
		got, err := Split(p)
		if err != nil {
			t.Fatalf("Split(%q): %v", p, err)
		}
		at := strings.LastIndex(p, "cases")
		if ctpDir := p[:at+5]; ctpDir != got.Dir {
			t.Errorf("Split(%q).Dir = %q, CTP would say %q", p, got.Dir, ctpDir)
		}
	}
}

func countUnderCases(t *testing.T, scenario string) int {
	t.Helper()
	n := 0
	filepath.WalkDir(scenario, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if strings.HasSuffix(p, ".sh") && filepath.Base(filepath.Dir(p)) == "cases" {
			n++
		}
		return nil
	})
	return n
}
