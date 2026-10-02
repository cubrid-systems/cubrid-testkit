package perf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeBranches(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "branches.conf")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBranchesReadTheRegistrations(t *testing.T) {
	all, err := ReadBranches(filepath.Join("testdata", "suite", "branches.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("registrations = %+v", all)
	}
	kim := all[0]
	if kim.Name != "CBRD-27238_escalation" || kim.Repo != "kim/cubrid" || kim.Owner != "kim" || len(kim.Cases) != 2 || !kim.Until.IsZero() {
		t.Errorf("first = %+v", kim)
	}
	if !kim.Selects("txn.commit_single") || kim.Selects("lib.backupdb") || !kim.Selects("storage.heap_scan") {
		t.Error("cases= does not select by glob")
	}
	park := all[1]
	if park.Until.Format("2006-01-02") != "2026-11-30" || !park.Selects("anything.at_all") {
		t.Errorf("second = %+v", park)
	}
	// A line without repo= is on CUBRID/cubrid.
	if all[2].Repo != DefaultRepo {
		t.Errorf("third repo = %q", all[2].Repo)
	}
}

func TestBranchRefusalsNameTheLine(t *testing.T) {
	cases := []struct{ name, text, want string }{
		{"an unknown key", "b1 owner=x untill=2026-01-01\n", "line 1: unknown key untill"},
		{"no owner", "b1 repo=x/y\n", "line 1: b1 has no owner="},
		{"a repo that is not owner/repo", "b1 owner=x repo=cubrid\n", `repo="cubrid" is not owner/repo`},
		{"a date that is not a date", "b1 owner=x until=2026-13-01\n", `until="2026-13-01" is not YYYY-MM-DD`},
		{"the same branch twice", "b1 owner=x\n\nb1 owner=y\n", "line 3: b1 is already registered on line 1"},
		{"a glob that cannot match", "b1 owner=x cases=txn.[\n", "does not match anything"},
		{"a value with no key", "b1 owner=x =y\n", `"=y" is not key=value`},
		{"a key=value where the name should be", "owner=x b1\n", "the branch name comes first"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ReadBranches(writeBranches(t, c.text))
			if err == nil {
				t.Fatalf("%s: accepted", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("%s: %q does not say %q", c.name, err, c.want)
			}
		})
	}
}

// Expiry and the cap take a registration out with a reason, not an error --
// the file is right, the week just has no room for it.
func TestActiveLeavesOutExpiredAndBeyondMax(t *testing.T) {
	all, err := ReadBranches(writeBranches(t, strings.Join([]string{
		"a owner=x until=2026-10-01",
		"b owner=x",
		"c owner=x",
		"d owner=x",
	}, "\n")+"\n"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	run, left := Active(all, 2, now)
	if len(run) != 2 || run[0].Name != "b" || run[1].Name != "c" {
		t.Errorf("run = %+v", run)
	}
	if len(left) != 2 || !strings.Contains(left[0], "a: expired 2026-10-01") || !strings.Contains(left[1], "d: beyond branches.max=2") {
		t.Errorf("left = %v", left)
	}
	// until= is inclusive: the day itself still runs.
	if all[0].Expired(time.Date(2026, 10, 1, 23, 0, 0, 0, time.UTC)) {
		t.Error("a registration expired on its own until day")
	}
}
