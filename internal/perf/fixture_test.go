package perf

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A restore replaces the database directory's contents and keeps the
// directory: the container holds that inode (D10).
func TestRestoreReplacesContentsAndKeepsTheDirectory(t *testing.T) {
	root := t.TempDir()
	db := filepath.Join(root, "perf_narrow_1m")
	other := filepath.Join(root, "perf_narrow_1m_other")
	snap := filepath.Join(root, "snap")
	for _, d := range []string{db, other} {
		if err := os.MkdirAll(filepath.Join(d, "lob"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(db, "perf_narrow_1m"), "volume v1")
	write(filepath.Join(db, "perf_narrow_1m_lgat"), "log v1")
	write(filepath.Join(other, "perf_narrow_1m_other"), "another database")
	if err := snapshot(db, snap); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(db)

	// The server wrote; a new file appeared; a file changed.
	write(filepath.Join(db, "perf_narrow_1m"), "volume v2 dirty")
	write(filepath.Join(db, "perf_narrow_1m_x001"), "a new volume")
	if err := restore(db, snap); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(db)
	if !os.SameFile(before, after) {
		t.Error("the database directory was replaced, not refilled")
	}
	if b, _ := os.ReadFile(filepath.Join(db, "perf_narrow_1m")); string(b) != "volume v1" {
		t.Errorf("volume = %q", b)
	}
	if _, err := os.Stat(filepath.Join(db, "perf_narrow_1m_x001")); err == nil {
		t.Error("a volume made after the snapshot survived the restore")
	}
	if _, err := os.Stat(filepath.Join(db, "lob")); err != nil {
		t.Error("the lob directory did not come back")
	}
	// The other database, whose name shares the prefix, is untouched.
	if b, _ := os.ReadFile(filepath.Join(other, "perf_narrow_1m_other")); string(b) != "another database" {
		t.Errorf("the neighbouring database was touched: %q", b)
	}
	if err := restore(db, filepath.Join(root, "nowhere")); err == nil {
		t.Error("a restore from a missing snapshot was accepted")
	}
}

func TestJudgeCountsACounterThatAppearedAsAChange(t *testing.T) {
	cs := &Case{ID: "x.y", Metric: "latency_s", Tolerance: 0.05, Repeats: 3, Counters: []string{"Num_log_archives", "client.rw_syscalls"}, JudgeCounters: []string{"Num_log_archives", "client.rw_syscalls"}}
	cr := &caseResult{Case: cs}
	for k := 1; k <= 3; k++ {
		cr.Passes = append(cr.Passes,
			measuredPass("target", k, 0.0020, map[string]float64{"Num_log_archives": 0.001, "client.rw_syscalls": 53.0 / 2e6}),
			measuredPass("reference", k, 0.0020, map[string]float64{"Num_log_archives": 0, "client.rw_syscalls": 51.0 / 2e6}))
	}
	v := judge(cs, cr, 3)
	if v.Flag != FlagWorkloadChange || v.Why != "Num_log_archives" {
		t.Errorf("a counter that went from 0 to something: flag=%s why=%s", v.Flag, v.Why)
	}
	// Syscall counts never decide: a JVM's socket I/O is invisible to them.
	cs.Counters, cs.JudgeCounters = []string{"client.rw_syscalls"}, []string{"client.rw_syscalls"}
	if v := judge(cs, cr, 3); v.Flag != FlagNone {
		t.Errorf("rw_syscalls flagged: %s", v.Flag)
	}
}

// FR-21 reads counts of work, not times, ratios or gauges.
func TestDeterministicIsCountValuedOnly(t *testing.T) {
	for _, yes := range []string{"Num_file_iosynches", "Num_log_append_records", "Num_dwb_flushed_block_volumes", "dev_flushes", "net_packets"} {
		if !deterministic(yes) {
			t.Errorf("%s should decide", yes)
		}
	}
	for _, no := range []string{"Num_object_locks_time_waited_usec", "Time_ha_replication_delay", "Data_page_buffer_hit_ratio",
		"Num_data_page_fixed", "Num_data_page_lru1", "Num_data_page_avoid_victim", "client.rw_syscalls", "dev_reads", "server.cpu_user", "not_a_name"} {
		if deterministic(no) {
			t.Errorf("%s should not decide", no)
		}
	}
}

func TestL0NullFieldIsMissingNotZero(t *testing.T) {
	pre, _ := parseL0([]byte(`{"roles":{"server":{"10":{"utime_ms":100,"stime_ms":50,"runq_wait_ms":null,"syscr":10,"syscw":5}}},"disk":null}`))
	post, _ := parseL0([]byte(`{"roles":{"server":{"10":{"utime_ms":400,"stime_ms":150,"runq_wait_ms":null,"syscr":30,"syscw":5}}},"disk":null}`))
	d, missing := l0Delta(pre, post)
	if d["server.cpu_user"] != 300 || d["server.rw_syscalls"] != 20 {
		t.Errorf("delta = %v", d)
	}
	if _, ok := d["server.runq_wait"]; ok || !strings.Contains(missing["server.runq_wait"], "no runq_wait_ms") {
		t.Errorf("a null field became a number: delta=%v missing=%v", d["server.runq_wait"], missing)
	}
	if _, ok := d["server.page_faults"]; ok {
		t.Error("a field absent from the snapshot became a number")
	}
	if _, said := missing["disk"]; !said {
		t.Error("a null disk row was not reported")
	}
}

func TestLastLineCarriesEveryValue(t *testing.T) {
	cs := &Case{ID: "x.y", Metric: "elapsed_s", Tolerance: 0.05, Repeats: 3}
	cr := &caseResult{Case: cs}
	cr.Passes = append(cr.Passes, measuredPass("target", 1, 10, nil), nullPass("target", 2), measuredPass("target", 3, 12, nil),
		measuredPass("reference", 1, 10, nil), measuredPass("reference", 2, 10, nil), measuredPass("reference", 3, 10, nil))
	v := judge(cs, cr, 3)
	line := lastLine(cs, v, caseEntry(cs, cr, v))
	for _, want := range []string{"x.y ratio=1.0954", "pairs=[1,1.2]", "confirmed=false", "target_mean=11", "reference_mean=10", "unit=s", "flag=none", "status=ok", "target=[10,null,12]", "reference=[10,10,10]"} {
		if !strings.Contains(line, want) {
			t.Errorf("line lacks %q: %s", want, line)
		}
	}
}

func TestNamesThatBecomeDatabaseNamesAreChecked(t *testing.T) {
	root := copySuite(t)
	// A fixture whose name has a dash: perf_narrow-1m is not a database
	// createdb will make (M0 e2e).
	bad := filepath.Join(root, "fixtures", "narrow-1m")
	if err := os.Rename(filepath.Join(root, "fixtures", "narrow_1m"), bad); err != nil {
		t.Fatal(err)
	}
	rewrite(t, filepath.Join(bad, "fixture.json"), func(m map[string]any) { m["name"] = "narrow-1m" })
	if _, err := ReadFixture(bad); err == nil || !strings.Contains(err.Error(), "must be lowercase letters, digits and underscores") {
		t.Errorf("a dashed fixture name was accepted: %v", err)
	}
	// One whose database name is over 17 characters: createdb stops in
	// logpb_initialize_log_names (the hub, 2026-10-25).
	long := filepath.Join(root, "fixtures", "narrow_1m_backup")
	if err := os.Rename(bad, long); err != nil {
		t.Fatal(err)
	}
	rewrite(t, filepath.Join(long, "fixture.json"), func(m map[string]any) { m["name"] = "narrow_1m_backup" })
	if _, err := ReadFixture(long); err == nil || !strings.Contains(err.Error(), "makes the database perf_narrow_1m_backup, 21 characters; the engine takes at most 17") {
		t.Errorf("a fixture name too long for a database was accepted: %v", err)
	}
	// Twelve characters make seventeen, which the engine takes.
	edge := filepath.Join(root, "fixtures", "narrow_1m_bk")
	if err := os.Rename(long, edge); err != nil {
		t.Fatal(err)
	}
	rewrite(t, filepath.Join(edge, "fixture.json"), func(m map[string]any) { m["name"] = "narrow_1m_bk" })
	if _, err := ReadFixture(edge); err != nil {
		t.Errorf("a fixture whose database name is 17 characters was refused: %v", err)
	}
	dir := caseDir(root, "txn.commit_single")
	rewrite(t, filepath.Join(dir, "case.json"), func(m map[string]any) {
		m["client"] = map[string]any{"main": "Outer$Inner; rm -rf /", "args": []string{}}
	})
	if _, err := ReadCase(dir); err == nil || !strings.Contains(err.Error(), "is not a Java class name") {
		t.Errorf("a client.main with shell in it was accepted: %v", err)
	}
}

// The vacuum statistics are accumulators counted only while a watcher is
// registered: idle is the difference at or below 0 with vacuumed not moving,
// over two reads, or three when the difference is negative. The caught-up and
// behind values are from statdump on the hub around a txn.log_append_bulk
// pass (11.4.6, 2026-10-27), where to_vacuum alone never read 0; the lines
// are in the engine's format ("%-29s = %10llu", and both names are longer
// than 29).
func TestVacuumIdleReadsTheAccumulators(t *testing.T) {
	read := func(to, done int64) vacuumCounts {
		return parseVacuumCounts(fmt.Sprintf("Num_vacuum_log_pages_vacuumed = %10d\nNum_vacuum_log_pages_to_vacuum = %10d\nNum_vacuum_prefetch_requests_log_pages = %10d\n", done, to, done))
	}
	idle := func(rs ...vacuumCounts) bool { return vacuumIdle(rs) }
	caughtUp, behind := read(13795, 13795), read(27528, 17856)
	if !caughtUp.ok || caughtUp.backlog() != 0 || behind.backlog() != 9672 {
		t.Fatalf("parse: %+v %+v", caughtUp, behind)
	}
	if !idle(caughtUp, caughtUp) {
		t.Error("caught up and still: not idle")
	}
	if idle(caughtUp) {
		t.Error("one read: idle")
	}
	if idle(behind, behind) {
		t.Error("9672 pages behind: idle")
	}
	// Blocks handed over before a restart and finished after it leave
	// vacuumed above to_vacuum: idle only after a longer still stretch.
	afterRestart := read(400, 431)
	if idle(afterRestart, afterRestart) {
		t.Error("negative difference, still over one interval: idle")
	}
	if !idle(afterRestart, afterRestart, afterRestart) {
		t.Error("negative difference, still over two intervals: not idle")
	}
	if idle(afterRestart, afterRestart, read(400, 462)) {
		t.Error("negative difference but vacuumed moving: idle")
	}
	// A fresh server, no block handed over yet.
	if !idle(read(0, 0), read(0, 0)) {
		t.Error("fresh server: not idle")
	}
	if missing := parseVacuumCounts("Num_vacuum_log_pages_to_vacuum =          5\n"); missing.ok || idle(missing, missing) {
		t.Error("one statistic missing: ok or idle")
	}
}

// The checkpoint before a defer case's window is done only when one started
// after the request and every started one ended: the request only wakes the
// daemon, and one already running when asked is not the one asked for.
func TestCheckpointDoneWaitsForTheOneAskedFor(t *testing.T) {
	read := func(started, ended int64) checkpointCounts {
		return parseCheckpointCounts(fmt.Sprintf("Num_log_start_checkpoints     = %10d\nNum_log_end_checkpoints       = %10d\n", started, ended))
	}
	before := read(41, 41)
	if !before.ok || before.started != 41 || before.ended != 41 {
		t.Fatalf("parse: %+v", before)
	}
	if checkpointDone(before, read(41, 41)) {
		t.Error("not started yet: done")
	}
	if checkpointDone(before, read(42, 41)) {
		t.Error("started, not ended: done")
	}
	if !checkpointDone(before, read(42, 42)) {
		t.Error("started after the request and ended: not done")
	}
	// One was running when asked: its end is not the checkpoint asked for.
	running := read(42, 41)
	if checkpointDone(running, read(42, 42)) {
		t.Error("the running one ended: done")
	}
	if !checkpointDone(running, read(43, 43)) {
		t.Error("a later one started and ended: not done")
	}
	if missing := parseCheckpointCounts("Num_log_start_checkpoints     =         42\n"); missing.ok || checkpointDone(before, missing) {
		t.Error("one statistic missing: ok or done")
	}
}
