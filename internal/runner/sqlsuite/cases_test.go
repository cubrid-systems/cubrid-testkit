package sqlsuite

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// corpus lays out a scenario the way the sql family has one: cases/ beside
// answers/, and directories that are not cases.
func corpus(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, body := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root + "/"
}

func names(cases []string, scenario string) []string {
	out := make([]string, 0, len(cases))
	for _, c := range cases {
		out = append(out, strings.TrimPrefix(c, scenario))
	}
	return out
}

// What counts as a case is CQT's rule, not this runner's: the .sql files under
// cases/, in the order File.list sorted by String.compareTo gives, with answers
// and common left out. Every number CQT records -- N, the percentage on every
// progress line, the order the result tree is written in -- is measured from
// that list, so its contents and its order are both the frozen surface.
func TestWhatCountsAsACaseAndInWhatOrder(t *testing.T) {
	scenario := corpus(t, map[string]string{
		"_01_object/cases/1002.sql":      "select 1;",
		"_01_object/cases/1001.sql":      "select 1;",
		"_01_object/cases/_first.sql":    "select 1;",
		"_01_object/cases/notacase.txt":  "not a case",
		"_01_object/answers/1001.answer": "1",
		// A .sql file under answers/ is what the rule exists for: the suffix
		// alone would take it, and CQT does not.
		"_01_object/answers/1001.sql":    "not a case either",
		"_01_object/answers/1002.answer": "2",
		"_02_more/cases/a.sql":           "select 1;",
		"_02_more/common/helper.sql":     "a helper, not a case",
		"_02_more/answers/a.answer":      "a",
	})

	set, err := discover(&settings{scenario: scenario}, cqtConfig{})
	if err != nil {
		t.Fatal(err)
	}
	got := names(set.all, scenario)
	want := []string{
		"_01_object/cases/1001.sql",
		"_01_object/cases/1002.sql",
		"_01_object/cases/_first.sql",
		"_02_more/cases/a.sql",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("discovered\n  %v\nwant\n  %v", got, want)
	}
	// "_" is 0x5F and a digit is 0x3x, so _first sorts after 1002 by code unit
	// and before it by every locale collation. The order is Java's.
	if got[len(got)-2] != "_01_object/cases/_first.sql" {
		t.Error("the order is not String.compareTo's")
	}
}

// A case with no answer stays in the list and never runs: CQT's build() keeps
// it, counts it in N, and the loop records it as failed without executing it.
func TestACaseWithNoAnswerIsCountedAndNotRun(t *testing.T) {
	scenario := corpus(t, map[string]string{
		"fam/cases/has.sql":      "select 1;",
		"fam/cases/none.sql":     "select 1;",
		"fam/answers/has.answer": "1",
	})
	set, err := discover(&settings{scenario: scenario}, cqtConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if len(set.all) != 2 {
		t.Fatalf("N is %d, want both cases: %v", len(set.all), names(set.all, scenario))
	}
	if len(set.answer) != 1 {
		t.Errorf("%d cases would run, want the one with an answer", len(set.answer))
	}
	if _, ok := set.answer[scenario+"fam/cases/none.sql"]; ok {
		t.Error("a case with no answer was given one")
	}
}

// The run mode picks an answer: <name>.answer_<run_mode>, then the secondary,
// then the plain one. The secondary is the literal "null" when the XML has no
// such element, because Java concatenated a null reference into the name.
func TestTheRunModeChoosesTheAnswer(t *testing.T) {
	for _, c := range []struct {
		name  string
		files map[string]string
		cfg   cqtConfig
		want  string
	}{
		{
			name: "no run mode takes the plain answer",
			files: map[string]string{
				"fam/cases/a.sql": "x", "fam/answers/a.answer": "1",
				"fam/answers/a.answer_cci": "2",
			},
			cfg:  cqtConfig{},
			want: "fam/answers/a.answer",
		},
		{
			name: "the run mode's answer wins",
			files: map[string]string{
				"fam/cases/a.sql": "x", "fam/answers/a.answer": "1",
				"fam/answers/a.answer_cci": "2",
			},
			cfg:  cqtConfig{hasRunMode: true, runMode: "cci"},
			want: "fam/answers/a.answer_cci",
		},
		{
			name: "the secondary is next",
			files: map[string]string{
				"fam/cases/a.sql": "x", "fam/answers/a.answer": "1",
				"fam/answers/a.answer_jdbc": "2",
			},
			cfg:  cqtConfig{hasRunMode: true, runMode: "cci", hasSecondary: true, runModeSecondary: "jdbc"},
			want: "fam/answers/a.answer_jdbc",
		},
		{
			name: "an XML with no secondary looks for the word null",
			files: map[string]string{
				"fam/cases/a.sql": "x", "fam/answers/a.answer": "1",
				"fam/answers/a.answer_null": "2",
			},
			cfg:  cqtConfig{hasRunMode: true, runMode: "cci"},
			want: "fam/answers/a.answer_null",
		},
		{
			name: "and falls back to the plain one",
			files: map[string]string{
				"fam/cases/a.sql": "x", "fam/answers/a.answer": "1",
			},
			cfg:  cqtConfig{hasRunMode: true, runMode: "cci"},
			want: "fam/answers/a.answer",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			scenario := corpus(t, c.files)
			set, err := discover(&settings{scenario: scenario}, c.cfg)
			if err != nil {
				t.Fatal(err)
			}
			got := strings.TrimPrefix(set.answer[scenario+"fam/cases/a.sql"], scenario)
			if got != c.want {
				t.Errorf("judged against %q, want %q", got, c.want)
			}
		})
	}
}

// The exclusion file is CQT's, and it is not shell's: a line is a path fragment
// and nothing is a comment, so a "#" line excludes the cases whose path
// contains it. Same key, different rules, and a run that assumed shell's would
// silently run cases it was told to skip.
func TestTheExclusionFileFollowsCQTsRules(t *testing.T) {
	scenario := corpus(t, map[string]string{
		"_01_object/cases/a.sql":      "x",
		"_01_object/answers/a.answer": "1",
		"_02_more/cases/b.sql":        "x",
		"_02_more/answers/b.answer":   "1",
		"_03_last/cases/c.sql":        "x",
		"_03_last/answers/c.answer":   "1",
	})
	list := filepath.Join(t.TempDir(), "exclusions.txt")
	if err := os.WriteFile(list, []byte("_02_more\n\n_03_last/cases/c.sql\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	set, err := discover(&settings{scenario: scenario, excludeFile: list}, cqtConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if got := names(set.all, scenario); strings.Join(got, ",") != "_01_object/cases/a.sql" {
		t.Errorf("kept %v, want only the case nothing excluded", got)
	}

	// A file that is not there is no filter and no message. Every shipped conf
	// names ${CTP_HOME}/conf/exclusions.txt, which CTP does not ship, so a run
	// that stopped for it would be every run.
	set, err = discover(&settings{scenario: scenario, excludeFile: filepath.Join(t.TempDir(), "gone.txt")}, cqtConfig{})
	if err != nil {
		t.Fatalf("a missing exclusion file stopped the run: %v", err)
	}
	if len(set.all) != 3 {
		t.Errorf("a missing exclusion file excluded something: %v", names(set.all, scenario))
	}
}
