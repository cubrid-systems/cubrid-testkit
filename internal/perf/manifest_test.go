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
	if len(s.Cases) != 4 || len(s.Fixtures) != 2 {
		t.Fatalf("cases=%d fixtures=%d", len(s.Cases), len(s.Fixtures))
	}
	var ids []string
	for _, c := range s.Cases {
		ids = append(ids, c.ID)
	}
	if strings.Join(ids, ",") != "cdc.extract_rate,lib.backupdb,storage.bulk_update,txn.commit_single" {
		t.Errorf("cases are not by id: %v", ids)
	}
	c := s.Case("txn.commit_single")
	if c.JDBC == nil || c.JDBC.Main != "CommitSingle" || len(c.JDBC.Args) != 2 {
		t.Errorf("jdbc client = %+v", c.JDBC)
	}
	if c.MaxPassS("case") != (1+5)*2*120 || c.MaxPassS("round") != (1+1)*5*2*120 {
		t.Errorf("MaxPassS = %d / %d", c.MaxPassS("case"), c.MaxPassS("round"))
	}
	if c.Conf["data_buffer_size"] != "4G" || !filepath.IsAbs(c.Dir) {
		t.Errorf("conf=%v dir=%q", c.Conf, c.Dir)
	}
	// A number or a bool in conf is the text cubrid.conf will carry.
	d := s.Case("cdc.extract_rate")
	if d.CDC == nil || d.CDC.Bin != "cdc_extract" || d.Conf["supplemental_log"] != "2" || d.Conf["cdc_flag"] != "true" {
		t.Errorf("cdc case = %+v conf=%v", d.CDC, d.Conf)
	}
	u := s.Case("lib.backupdb")
	if u.Utility == nil || u.Utility.Ops != 1 || u.Utility.Argv[1] != "backupdb" {
		t.Errorf("utility client = %+v", u.Utility)
	}
	if f := s.Fixtures["wide_100k"]; f.Reset != ResetRestoreSnapshot || f.Size != "4G" {
		t.Errorf("fixture = %+v", f)
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
		}, "missing client.main, client.args; unknown key client.argv, client.ops"},
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
		{"no clients", "txn.commit_single", func(m map[string]any) { delete(m, "clients") }, "missing clients"},
		{"zero clients on a jdbc case", "txn.commit_single", func(m map[string]any) { m["clients"] = 0 }, "clients must be 1 or more for a jdbc client"},
		{"clients on a utility", "lib.backupdb", func(m map[string]any) { m["clients"] = 2 }, "clients must be 0 for a utility"},
		{"a topology off the list", "txn.commit_single", func(m map[string]any) { m["topology"] = "ha" }, `topology "ha" is not one of single`},
		{"an op off the list", "txn.commit_single", func(m map[string]any) { m["op"] = "txn" }, `op "txn" is not one of`},
		{"a metric off the list", "txn.commit_single", func(m map[string]any) { m["metric"] = "p99" }, `metric "p99" is not one of`},
		{"a background off the list", "txn.commit_single", func(m map[string]any) { m["background"] = "off" }, `background "off" is not one of`},
		// A statdump name is the engine's own spelling, not a pattern.
		{"a misspelt statdump counter", "txn.commit_single", func(m map[string]any) { m["counters"] = []string{"Num_file_iosynchs"} }, `counter "Num_file_iosynchs"`},
		// Go's decoder matches field names without regard to case; the schema
		// is closed below the top level too.
		{"a fixture key in the wrong case", "txn.commit_single", func(m map[string]any) {
			m["fixture"] = map[string]any{"NAME": "narrow_1m", "version": 1}
		}, "unknown key fixture.NAME"},
		{"a required key set to null", "txn.commit_single", func(m map[string]any) { m["conf"] = nil }, "missing conf"},
		{"a conf value that is an object", "txn.commit_single", func(m map[string]any) {
			m["conf"] = map[string]any{"data_buffer_size": map[string]any{"v": 1}}
		}, "conf.data_buffer_size must be a string"},
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
		{"an unknown key", func(_ string, m map[string]any) { m["comment"] = "x" }, "unknown key comment"},
		{"a load script that is not there", func(_ string, m map[string]any) { m["load"] = "populate.sh" }, `load names "populate.sh"`},
		{"negative rows", func(_ string, m map[string]any) { m["rows"] = -1 }, "rows must be 0 or more"},
		{"no size", func(_ string, m map[string]any) { delete(m, "size") }, "missing size"},
		{"a size createdb would not take", func(_ string, m map[string]any) { m["size"] = "2 GB" }, `size wants a volume size like 2G`},
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

// Tab-completion leaves a trailing slash, and "." from inside the directory
// is the same directory; the id rule reads the directory, so both have to
// name the right one.
func TestACaseIsReadFromWhateverSpellingOfItsDirectory(t *testing.T) {
	root := copySuite(t)
	dir := caseDir(root, "txn.commit_single")
	for _, spelling := range []string{dir, dir + string(filepath.Separator), filepath.Join(dir, "..", "commit_single")} {
		if c, err := ReadCase(spelling); err != nil || c.Dir != dir {
			t.Errorf("ReadCase(%q): err=%v dir=%q", spelling, err, c.Dir)
		}
	}
	t.Chdir(dir)
	if c, err := ReadCase("."); err != nil || c.Dir != dir {
		t.Errorf(`ReadCase("."): err=%v dir=%q`, err, c.Dir)
	}
	t.Chdir(filepath.Join(root, "fixtures", "wide_100k"))
	if f, err := ReadFixture("."); err != nil || f.Name != "wide_100k" {
		t.Errorf(`ReadFixture("."): err=%v`, err)
	}
}

// A case whose fixture cannot load is not ok: the rules that span both
// files are still applied to what did decode, and the fixture is named.
func TestACaseSaysWhenItsFixtureIsBroken(t *testing.T) {
	root := copySuite(t)
	rewrite(t, filepath.Join(root, "fixtures", "wide_100k", "fixture.json"), func(m map[string]any) { m["load"] = "nope.sh" })
	dir := caseDir(root, "storage.bulk_update")
	rewrite(t, filepath.Join(dir, "case.json"), func(m map[string]any) { m["warm_s"] = 0 })
	_, err := ReadCase(dir)
	if err == nil {
		t.Fatal("accepted")
	}
	for _, want := range []string{"fixture wide_100k has problems of its own", "warm_s must be above 0"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q does not say %q", err, want)
		}
	}
}

// Three broken files are three reports, and a case.json at the wrong depth
// is not silently nothing.
func TestTheSuiteReportsEveryBrokenFile(t *testing.T) {
	root := copySuite(t)
	rewrite(t, filepath.Join(root, "fixtures", "narrow_1m", "fixture.json"), func(m map[string]any) { m["rows"] = -1 })
	rewrite(t, filepath.Join(caseDir(root, "lib.backupdb"), "case.json"), func(m map[string]any) { m["grade"] = "Z" })
	rewrite(t, filepath.Join(caseDir(root, "txn.commit_single"), "case.json"), func(m map[string]any) { m["repeats"] = 1 })
	if err := os.WriteFile(filepath.Join(root, "cases", "txn", "case.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSuite(root)
	if err == nil {
		t.Fatal("accepted")
	}
	for _, want := range []string{"narrow_1m/fixture.json", "lib/backupdb/case.json", "txn/commit_single/case.json", "cases/txn/case.json: a case.json belongs in"} {
		if !strings.Contains(err.Error(), filepath.FromSlash(want)) {
			t.Errorf("the suite's error does not name %q:\n%v", want, err)
		}
	}
	// What did read is still there, for list; and a case whose fixture is the
	// broken one is not a second report of the fixture.
	if s == nil || s.Case("storage.bulk_update") == nil || s.Case("cdc.extract_rate") == nil {
		t.Errorf("the readable cases were dropped with the broken ones: %+v", s)
	}
	if strings.Contains(err.Error(), "problems of its own") {
		t.Errorf("the fixture's problem was reported again through its cases:\n%v", err)
	}
}

func TestKnownCounters(t *testing.T) {
	for _, ok := range []string{"Num_file_iosynches", "DWB_flush_block", "Time_ha_replication_delay", "Num_dwb_flushed_block_volumes",
		"dev_flushes", "net_packets", "cas.rw_syscalls", "server.cpu_user", "client.ctxsw_vol", "syscalls_by_type"} {
		if !KnownCounter(ok) {
			t.Errorf("%s is on the list and was refused", ok)
		}
	}
	for _, bad := range []string{"fsyncs", "rw_syscalls", "cas.fsync", "master.cpu_user", "num_file_iosynches", "Num_file_iosynchs", "Num_", ""} {
		if KnownCounter(bad) {
			t.Errorf("%q is not on the list and was accepted", bad)
		}
	}
}
