package perf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stageSuite writes a suite with the given files (path relative to the suite
// → content) and returns its directory.
func stageSuite(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for p, content := range files {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// The common C sources are staged beside the Java ones, so a second case can
// run a cdc-api program whose source is not in its own directory.
func TestStageWorkCopiesCommonCSources(t *testing.T) {
	suite := stageSuite(t, map[string]string{
		"scripts/common/PerfClient.java":     "class PerfClient {}",
		"scripts/common/cdc_extract.c":       "int main (void) { return 0; }",
		"cases/cdc/extract_update/case.json": "{}",
		"fixtures/cdc_upd/fixture.json":      "{}",
	})
	r := &Runner{SuiteDir: suite}
	s := &Side{Role: "target", Work: t.TempDir()}
	c := &Case{ID: "cdc.extract_update", Dir: filepath.Join(suite, "cases", "cdc", "extract_update"), Fixture: FixtureRef{Name: "cdc_upd"}}
	if err := r.stageWork(s, []*Case{c}); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"PerfClient.java", "cdc_extract.c"} {
		if _, err := os.Stat(filepath.Join(s.Work, "perf", "src", f)); err != nil {
			t.Errorf("src/%s not staged: %v", f, err)
		}
	}
}

// A case source with the name of a common one would replace it in the one
// directory every source shares: refused, as two cases with one name are.
func TestStageWorkRefusesACaseSourceNamedLikeACommonOne(t *testing.T) {
	suite := stageSuite(t, map[string]string{
		"scripts/common/cdc_extract.c":               "int main (void) { return 0; }",
		"cases/cdc/extract_insert/src/cdc_extract.c": "int main (void) { return 1; }",
		"cases/cdc/extract_insert/case.json":         "{}",
		"fixtures/cdc_ins/fixture.json":              "{}",
	})
	r := &Runner{SuiteDir: suite}
	s := &Side{Role: "target", Work: t.TempDir()}
	c := &Case{ID: "cdc.extract_insert", Dir: filepath.Join(suite, "cases", "cdc", "extract_insert"), Fixture: FixtureRef{Name: "cdc_ins"}}
	err := r.stageWork(s, []*Case{c})
	if err == nil || !strings.Contains(err.Error(), "scripts/common") {
		t.Fatalf("want a refusal naming scripts/common, got %v", err)
	}
}
