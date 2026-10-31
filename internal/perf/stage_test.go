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

func stageCase(suite, id, fixture string) *Case {
	module, name, _ := strings.Cut(id, ".")
	return &Case{ID: id, Dir: filepath.Join(suite, "cases", module, name), Fixture: FixtureRef{Name: fixture},
		Driver: "cdc-api", CDC: &CDCClient{Bin: "cdc_extract"}}
}

// Two cases run one cdc-api program from scripts/common, neither with a
// source of its own: the C source and its header are staged once, beside the
// common Java.
func TestStageWorkCopiesCommonCSources(t *testing.T) {
	suite := stageSuite(t, map[string]string{
		"scripts/common/PerfClient.java":     "class PerfClient {}",
		"scripts/common/cdc_extract.c":       "int main (void) { return 0; }",
		"scripts/common/cdc_extract.h":       "",
		"cases/cdc/extract_insert/case.json": "{}",
		"cases/cdc/extract_update/case.json": "{}",
		"fixtures/cdc_ins/fixture.json":      "{}",
		"fixtures/cdc_upd/fixture.json":      "{}",
	})
	r := &Runner{SuiteDir: suite}
	s := &Side{Role: "target", Work: t.TempDir()}
	cases := []*Case{stageCase(suite, "cdc.extract_insert", "cdc_ins"), stageCase(suite, "cdc.extract_update", "cdc_upd")}
	if err := r.stageWork(s, cases); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"PerfClient.java", "cdc_extract.c", "cdc_extract.h"} {
		if _, err := os.Stat(filepath.Join(s.Work, "perf", "src", f)); err != nil {
			t.Errorf("src/%s not staged: %v", f, err)
		}
	}
	if errs := checkSources(suite, cases); len(errs) != 0 {
		t.Errorf("checkSources: %v", errs)
	}
}

// A source in a case named like a common one (C or Java), or like another
// case's, would replace the other in the one directory every source shares:
// refused by stageWork and, before any cluster, by checkSources.
func TestStageWorkRefusesASourceNameTwice(t *testing.T) {
	for _, tc := range []struct {
		name, other string
		files       map[string]string
	}{
		{"case and common C", "scripts/common", map[string]string{
			"scripts/common/cdc_extract.c":               "int main (void) { return 0; }",
			"cases/cdc/extract_insert/src/cdc_extract.c": "int main (void) { return 1; }",
		}},
		{"case and common Java", "scripts/common", map[string]string{
			"scripts/common/RangeScan.java":               "class RangeScan {}",
			"cases/cdc/extract_insert/src/RangeScan.java": "class RangeScan {}",
		}},
		{"two cases", "cdc.extract_insert", map[string]string{
			"scripts/common/PerfClient.java":             "class PerfClient {}",
			"cases/cdc/extract_insert/src/cdc_extract.c": "int main (void) { return 0; }",
			"cases/cdc/extract_update/src/cdc_extract.c": "int main (void) { return 1; }",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.files["fixtures/cdc_ins/fixture.json"] = "{}"
			tc.files["fixtures/cdc_upd/fixture.json"] = "{}"
			suite := stageSuite(t, tc.files)
			cases := []*Case{stageCase(suite, "cdc.extract_insert", "cdc_ins"), stageCase(suite, "cdc.extract_update", "cdc_upd")}
			r := &Runner{SuiteDir: suite}
			s := &Side{Role: "target", Work: t.TempDir()}
			if err := r.stageWork(s, cases); err == nil || !strings.Contains(err.Error(), tc.other) {
				t.Errorf("stageWork: want a refusal naming %s, got %v", tc.other, err)
			}
			errs := checkSources(suite, cases)
			if len(errs) == 0 || !strings.Contains(errs[0].Error(), tc.other) {
				t.Errorf("checkSources: want a refusal naming %s, got %v", tc.other, errs)
			}
		})
	}
}

// A cdc-api case whose program has no source anywhere is named before any
// cluster is built.
func TestCheckSourcesWantsTheCdcProgramsSource(t *testing.T) {
	suite := stageSuite(t, map[string]string{"cases/cdc/extract_update/case.json": "{}"})
	errs := checkSources(suite, []*Case{stageCase(suite, "cdc.extract_update", "cdc_upd")})
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "cdc_extract.c") {
		t.Errorf("want one error naming cdc_extract.c, got %v", errs)
	}
	// In another case's src/ is not enough: it is staged only when that case
	// runs in the same cluster.
	other := stageSuite(t, map[string]string{
		"cases/cdc/extract_insert/src/cdc_extract.c": "int main (void) { return 0; }",
		"cases/cdc/extract_update/case.json":         "{}",
	})
	errs = checkSources(other, []*Case{stageCase(other, "cdc.extract_insert", "cdc_ins"), stageCase(other, "cdc.extract_update", "cdc_upd")})
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "extract_update") {
		t.Errorf("program in another case's src/: want one error for cdc.extract_update, got %v", errs)
	}
}
