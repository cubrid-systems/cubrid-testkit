package perf

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// copySuite puts testdata/suite in a temporary directory, so a test can break
// one file without the next test reading the breakage.
func copySuite(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "regression")
	src := filepath.Join("testdata", "suite")
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		dst := filepath.Join(root, rel)
		if info.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func caseDir(root, id string) string {
	module, name, _ := strings.Cut(id, ".")
	return filepath.Join(root, "cases", module, name)
}

// rewrite applies a change to one JSON file as a map, so a test says which key
// it breaks and nothing else about the file.
func rewrite(t *testing.T, path string, change func(m map[string]any)) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	change(m)
	out, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSuiteReadsEveryCaseAndFixture(t *testing.T) {
	s, err := LoadSuite(copySuite(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Cases) != 3 || len(s.Fixtures) != 2 {
		t.Fatalf("cases=%d fixtures=%d", len(s.Cases), len(s.Fixtures))
	}
	if got := []string{s.Cases[0].ID, s.Cases[1].ID, s.Cases[2].ID}; strings.Join(got, ",") != "lib.backupdb,storage.bulk_update,txn.commit_single" {
		t.Errorf("cases are not by id: %v", got)
	}
	c := s.Case("txn.commit_single")
	if c.JDBC == nil || c.JDBC.Main != "CommitSingle" || len(c.JDBC.Args) != 2 {
		t.Errorf("jdbc client = %+v", c.JDBC)
	}
	if c.MaxPassS() != (1+5)*2*120 {
		t.Errorf("MaxPassS = %d", c.MaxPassS())
	}
	u := s.Case("lib.backupdb")
	if u.Utility == nil || u.Utility.Ops != 1 || u.Utility.Argv[1] != "backupdb" {
		t.Errorf("utility client = %+v", u.Utility)
	}
	if s.Fixtures["wide_100k"].Reset != ResetRestoreSnapshot {
		t.Errorf("fixture = %+v", s.Fixtures["wide_100k"])
	}
}

// The Spec's list of what validate refuses (§7.2), one case each, and the
// refusal has to name the thing -- a reader fixes a manifest from the message.
func TestCaseRefusalsNameTheProblem(t *testing.T) {
	cases := []struct {
		name   string
		id     string
		change func(m map[string]any)
		want   string
	}{
		{"a missing required key", "txn.commit_single", func(m map[string]any) { delete(m, "owner") }, "missing owner"},
		{"an unknown key", "txn.commit_single", func(m map[string]any) { m["tolerence"] = 0.1 }, "unknown key tolerence"},
		{"a wrong type", "txn.commit_single", func(m map[string]any) { m["repeats"] = "5" }, "repeats is string, want int"},
		{"a counter off the list", "txn.commit_single", func(m map[string]any) { m["counters"] = []string{"dev_flushes", "fsyncs"} }, `counter "fsyncs"`},
		{"an id that is not the directory", "txn.commit_single", func(m map[string]any) { m["id"] = "txn.commit_one" }, `id "txn.commit_one" does not match`},
		{"a module that is not the directory", "txn.commit_single", func(m map[string]any) { m["module"] = "storage" }, `module "storage" does not match`},
		{"repeats below 3", "txn.commit_single", func(m map[string]any) { m["repeats"] = 2 }, "repeats must be 3 or more"},
		{"warmup below 1", "txn.commit_single", func(m map[string]any) { m["warmup"] = 0 }, "warmup must be 1 or more"},
		{"tolerance of zero", "txn.commit_single", func(m map[string]any) { m["tolerance"] = 0 }, "tolerance must be above 0"},
		{"a client shaped for another driver", "txn.commit_single", func(m map[string]any) {
			m["client"] = map[string]any{"argv": []string{"x"}, "ops": 1}
		}, "missing main, args"},
		{"a restored snapshot with no warming", "storage.bulk_update", func(m map[string]any) { m["warm_s"] = 0 }, "warm_s must be above 0"},
		{"a fixture version the suite does not have", "txn.commit_single", func(m map[string]any) {
			m["fixture"] = map[string]any{"name": "narrow_1m", "version": 2}
		}, "version 1 in the suite and the case assumes 2"},
		{"a fixture the suite does not have", "txn.commit_single", func(m map[string]any) {
			m["fixture"] = map[string]any{"name": "huge_1g", "version": 1}
		}, `fixture "huge_1g" is not in`},
		{"a grade off the list", "txn.commit_single", func(m map[string]any) { m["grade"] = "S" }, `grade "S" is not one of A, B, C`},
		{"a utility with no ops", "lib.backupdb", func(m map[string]any) {
			m["client"] = map[string]any{"argv": []string{"cubrid", "backupdb"}, "ops": 0}
		}, "client.ops must be 1 or more"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := copySuite(t)
			dir := caseDir(root, c.id)
			rewrite(t, filepath.Join(dir, "case.json"), c.change)
			_, err := ReadCase(dir)
			if err == nil {
				t.Fatalf("%s: accepted", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("%s: %q does not say %q", c.name, err, c.want)
			}
			// And the whole suite reports the same, with the file named.
			_, err = LoadSuite(root)
			if err == nil || !strings.Contains(err.Error(), filepath.Join(dir, "case.json")) {
				t.Errorf("the suite does not name the broken file: %v", err)
			}
		})
	}
}

func TestFixtureRefusalsNameTheProblem(t *testing.T) {
	cases := []struct {
		name   string
		change func(root string, m map[string]any)
		want   string
	}{
		{"a name that is not the directory", func(_ string, m map[string]any) { m["name"] = "narrow" }, `name "narrow" does not match`},
		{"a reset off the list", func(_ string, m map[string]any) { m["reset"] = "reload" }, `reset "reload" is not one of`},
		{"an unknown key", func(_ string, m map[string]any) { m["size"] = "1G" }, "unknown key size"},
		{"a load script that is not there", func(_ string, m map[string]any) { m["load"] = "populate.sh" }, `load names "populate.sh"`},
		{"no rows", func(_ string, m map[string]any) { m["rows"] = 0 }, "rows must be 1 or more"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := copySuite(t)
			dir := filepath.Join(root, "fixtures", "narrow_1m")
			rewrite(t, filepath.Join(dir, "fixture.json"), func(m map[string]any) { c.change(root, m) })
			_, err := ReadFixture(dir)
			if err == nil {
				t.Fatalf("%s: accepted", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("%s: %q does not say %q", c.name, err, c.want)
			}
		})
	}
}

// Every problem in a file comes out at once: a reader should not fix one key
// per run.
func TestEveryProblemIsReportedTogether(t *testing.T) {
	root := copySuite(t)
	dir := caseDir(root, "txn.commit_single")
	rewrite(t, filepath.Join(dir, "case.json"), func(m map[string]any) {
		m["repeats"] = 1
		m["warmup"] = 0
		m["grade"] = "Z"
	})
	_, err := ReadCase(dir)
	if err == nil {
		t.Fatal("accepted")
	}
	for _, want := range []string{"repeats must", "warmup must", `grade "Z"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q does not say %q", err, want)
		}
	}
}

func TestKnownCounters(t *testing.T) {
	for _, ok := range []string{"Num_file_iosynches", "dev_flushes", "net_packets", "cas.rw_syscalls", "server.cpu_user", "client.ctxsw_vol", "syscalls_by_type"} {
		if !KnownCounter(ok) {
			t.Errorf("%s is on the list and was refused", ok)
		}
	}
	for _, bad := range []string{"fsyncs", "rw_syscalls", "cas.fsync", "master.cpu_user", "num_file_iosynches", ""} {
		if KnownCounter(bad) {
			t.Errorf("%q is not on the list and was accepted", bad)
		}
	}
}
