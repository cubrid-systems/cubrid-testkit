package shellsuite

import (
	"os"
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
