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

// Discovery over a real connection, against the real corpus. It is the one
// correction everything else rests on -- 3,452 cases rather than 3,722 -- and
// this is the only place it is checked through the channel that will carry it.
//
// Needs TESTKIT_SSH_HOST and TESTKIT_SHELL_CORPUS.
func TestDiscoverOverSSHFindsTheRealCorpus(t *testing.T) {
	host := os.Getenv("TESTKIT_SSH_HOST")
	root := os.Getenv("TESTKIT_SHELL_CORPUS")
	if host == "" || root == "" {
		t.Skip("set TESTKIT_SSH_HOST and TESTKIT_SHELL_CORPUS")
	}
	scenario := filepath.Join(root, "shell")
	if _, err := os.Stat(scenario); err != nil {
		t.Skipf("no shell tree under %s", root)
	}

	ch := exec.NewSSH(exec.SSHConfig{
		Host:     host,
		Port:     envOr("TESTKIT_SSH_PORT", "22"),
		User:     envOr("TESTKIT_SSH_USER", os.Getenv("USER")),
		Password: os.Getenv("TESTKIT_SSH_PASSWORD"),
	})
	defer ch.Close()

	got, err := Discover(t.Context(), ch, scenario)
	if err != nil {
		t.Fatal(err)
	}

	var want []string
	err = filepath.WalkDir(scenario, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".sh") {
			return err
		}
		if IsCase(p) {
			want = append(want, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(want)

	if !slices.Equal(got, want) {
		t.Fatalf("discovery over ssh found %d cases, walking the tree found %d", len(got), len(want))
	}
	t.Logf("%d cases discovered over ssh, byte for byte the same list as the local walk", len(got))

	if !slices.IsSorted(got) {
		t.Error("the list came back unsorted, so two runs would dispatch differently")
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// Include exists for the loop a fix goes round: a run leaves failures, the
// engine is rebuilt, and the question is whether those cases pass now -- not
// whether the other 3,400 still do, which takes two hours and answers something
// else.
func TestIncludeKeepsOnlyWhatIsNamed(t *testing.T) {
	cases := []string{
		"/run/shell/_06_issues/_11_1h/bug_bts_4823/cases/bug_bts_4823.sh",
		"/run/shell/_06_issues/_11_1h/bug_bts_4824/cases/bug_bts_4824.sh",
		"/run/shell/_01_utility/_38_csql/csql2/cases/csql2.sh",
	}
	kept, dropped := Include(cases, ParseExcluded(
		"_06_issues/_11_1h/bug_bts_4823/cases/bug_bts_4823.sh\n_01_utility/_38_csql/csql2\n"))
	if len(kept) != 2 || len(dropped) != 1 {
		t.Fatalf("kept %d dropped %d: %v", len(kept), len(dropped), kept)
	}
	// The trailing slash ParseExcluded adds is what stops 4823 selecting 4824.
	for _, c := range kept {
		if strings.Contains(c, "4824") {
			t.Errorf("a longer name that merely starts the same was selected: %s", c)
		}
	}

	// The point of matching on a fragment: a list written against one scenario
	// root still selects under another, which is what makes it work when the
	// corpus has moved and the engine is a different build.
	elsewhere := []string{"/somewhere/else/_01_utility/_38_csql/csql2/cases/csql2.sh"}
	if kept, _ := Include(elsewhere, ParseExcluded("_01_utility/_38_csql/csql2\n")); len(kept) != 1 {
		t.Error("a list from one root did not select the same case under another")
	}

	// No patterns is not "select nothing": it is "this filter is off".
	if kept, _ := Include(cases, nil); len(kept) != 3 {
		t.Errorf("an empty pattern list dropped cases: %d kept", len(kept))
	}
}
