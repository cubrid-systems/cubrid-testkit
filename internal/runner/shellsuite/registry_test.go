package shellsuite

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A registry entry from a run that is over fails every case that walks
// databases.txt, and the case log blames make_tz rather than the registry.
func TestAnInheritedRegistryIsReported(t *testing.T) {
	reg := t.TempDir()
	scenario := t.TempDir()
	body := strings.Join([]string{
		"#db-name\tvol-path",
		"db14907\t\t" + filepath.Join(scenario, "_06_issues", "cases") + "\tlocalhost",
		"bug606\t\t/somewhere/else/_01_sqlx/cases\tlocalhost",
		"qadb\t\t/older/still/cases\tlocalhost",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(reg, "databases.txt"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CUBRID_DATABASES", reg)

	got := staleDatabases(scenario)
	if len(got) != 2 {
		t.Fatalf("reported %d entries, want 2 (the two outside the scenario): %v", len(got), got)
	}
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "bug606") || !strings.Contains(joined, "qadb") {
		t.Errorf("the two stale entries should be named: %v", got)
	}
	if strings.Contains(joined, "db14907") {
		t.Error("a database inside this run's scenario is this run's, not an inheritance")
	}
}

func TestNoRegistryIsNotAComplaint(t *testing.T) {
	t.Setenv("CUBRID_DATABASES", t.TempDir())
	if got := staleDatabases(t.TempDir()); len(got) != 0 {
		t.Errorf("a machine with no registry yet has nothing to report: %v", got)
	}
	t.Setenv("CUBRID_DATABASES", "")
	if got := staleDatabases("/x"); got != nil {
		t.Errorf("no registry configured, nothing to say: %v", got)
	}
}

// The reclaim and the registry move together, or the registry names databases
// whose files are gone and the next case that walks it fails.
func TestPruningTheRegistryDropsOnlyTheReclaimedDirectory(t *testing.T) {
	reg := t.TempDir()
	dir := "/corpus/shell/_39_fig_cake/cbrd_25452/cases"
	body := strings.Join([]string{
		"#db-name\tvol-path",
		"db25452\t\t" + dir + "/db25452\tlocalhost\t" + dir + "/db25452",
		"db14907\t\t" + dir + "\tlocalhost\t" + dir,
		"keepme\t\t/corpus/shell/_06_issues/other/cases\tlocalhost",
		// A path that merely starts with the same characters is a different
		// directory, and must survive.
		"neighbour\t\t" + dir + "_backup\tlocalhost",
		"",
	}, "\n")
	path := filepath.Join(reg, "databases.txt")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	script := "export CUBRID_DATABASES=" + reg + "\n" + PruneRegistryScript(dir)
	out, err := exec.Command("bash", "-c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("prune failed: %v: %s", err, out)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	for _, gone := range []string{"db25452", "db14907"} {
		if strings.Contains(text, gone) {
			t.Errorf("%s was under the reclaimed directory and should be gone:\n%s", gone, text)
		}
	}
	for _, kept := range []string{"#db-name", "keepme", "neighbour"} {
		if !strings.Contains(text, kept) {
			t.Errorf("%s should have survived:\n%s", kept, text)
		}
	}
}

// No registry configured is the ordinary case for a run without slots, and the
// script has to be a no-op rather than an error.
func TestPruningWithoutARegistryIsQuiet(t *testing.T) {
	out, err := exec.Command("bash", "-c", "unset CUBRID_DATABASES\n"+PruneRegistryScript("/x")).CombinedOutput()
	if err != nil {
		t.Fatalf("prune should be a no-op: %v: %s", err, out)
	}
}

// A user namespace maps one uid, so tar cannot restore an archive's recorded
// ownership: it prints "Cannot change ownership" and exits non-zero even though
// the files are there. 51 case scripts in this corpus unpack something, and the
// exit status is what fails them.
func TestASlotCanUnpackAnArchiveSomebodyElseOwned(t *testing.T) {
	if os.Getenv("TESTKIT_CONTAINED") != "1" {
		t.Skip("not contained; run under TESTKIT_CONTAIN=1")
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "f"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("tar", "-czf", filepath.Join(dir, "a.tar.gz"), "-C", dir, "src").CombinedOutput(); err != nil {
		t.Fatalf("cannot make the archive: %v: %s", err, out)
	}
	if err := os.RemoveAll(filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("tar", "-zxf", "a.tar.gz")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "TAR_OPTIONS=--no-same-owner")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("TAR_OPTIONS did not let the slot unpack it: %v: %s", err, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "src", "f")); err != nil {
		t.Errorf("the archive did not unpack: %v", err)
	}
}
