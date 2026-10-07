package perf

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A fake install tree: enough for isInstallTree and readFingerprint.
func fakeBuild(t *testing.T, dir, commit, buildType string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "cub_server"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	info := map[string]any{
		"schema": "cubrid-build-info/1", "commit": commit, "label": filepath.Base(dir), "built_at": "2026-10-07T01:00:00+09:00",
		"compiler": "gcc 8.5.0", "build_type": buildType, "cxx_flags": "-O2 -g -DNDEBUG",
		"thirdparty": map[string]any{"lz4": map[string]any{"opt": "-O3", "sha256": "x"}, "re2": map[string]any{"opt": nil, "sha256": nil}},
	}
	b, _ := json.Marshal(info)
	if err := os.WriteFile(filepath.Join(dir, "build-info.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeManifest(t *testing.T, path string, written time.Time, pairs map[string]*ManifestPair) {
	t.Helper()
	m := BuildsManifest{Schema: buildsSchema, Written: written.Format(time.RFC3339), Pairs: pairs}
	if err := writeJSON(path, &m); err != nil {
		t.Fatal(err)
	}
}

// FR-1.1: the session resolves nothing itself. A pair the manifest has is
// taken from it; a pair that names two install trees needs no manifest; a
// symbolic pair without a manifest entry, and a registration without one,
// are "build missing"; a stale manifest skips everything.
func TestPairsFromTakesTheManifestOrInstallTrees(t *testing.T) {
	dir := t.TempDir()
	tgt := fakeBuild(t, filepath.Join(dir, "develop-9fc1a2b"), "9fc1a2b000", "RelWithDebInfo")
	ref := fakeBuild(t, filepath.Join(dir, "11.4.6-0e7d3c1"), "0e7d3c1000", "RelWithDebInfo")
	skipped := "fetch"
	manifest := &BuildsManifest{Schema: buildsSchema, Written: time.Now().Format(time.RFC3339), Pairs: map[string]*ManifestPair{
		"develop":   {Target: ManifestBuild{Ref: "develop-HEAD", Commit: "9fc1a2b000", Build: tgt}, Reference: ManifestBuild{Ref: ref, Commit: "0e7d3c1000", Build: ref}},
		"feature/x": {Skipped: &skipped},
		"gone":      {Target: ManifestBuild{Build: filepath.Join(dir, "nope")}, Reference: ManifestBuild{Build: ref}},
	}}
	conf := &Conf{Pairs: []Pair{
		{Name: "develop", Target: "develop-HEAD", Reference: ref},
		{Name: "local", Target: tgt, Reference: ref},
		{Name: "symbolic", Target: "merge-base", Reference: ref},
	}}
	branches := []Branch{{Name: "feature/x", Owner: "kim"}, {Name: "gone", Owner: "lee"}, {Name: "unbuilt", Owner: "park"}}

	pairs := pairsFrom(conf, branches, manifest, false)
	want := map[string]string{"develop": "", "local": "", "symbolic": "build missing", "feature/x": "fetch", "unbuilt": "build missing"}
	for _, p := range pairs {
		if w, ok := want[p.Name]; ok && p.Skipped != w {
			t.Errorf("%s: skipped=%q, want %q", p.Name, p.Skipped, w)
		}
	}
	if got := pairs[0].Target.Build; got != tgt {
		t.Errorf("develop's target build = %s, want %s", got, tgt)
	}
	if got := pairs[0].Target.Commit; got != "9fc1a2b000" {
		t.Errorf("develop's target commit = %s", got)
	}
	var gone *SessionPair
	for _, p := range pairs {
		if p.Name == "gone" {
			gone = p
		}
	}
	if gone == nil || !strings.HasPrefix(gone.Skipped, "build missing: ") {
		t.Errorf("a manifest pair whose tree is not there must be skipped by name: %+v", gone)
	}
	if pairs[3].Owner != "kim" || pairs[3].branch == nil {
		t.Errorf("a registration's pair carries its owner and its branch: %+v", pairs[3])
	}
	// The order: conf pairs in file order, then the registrations.
	if pairs[0].Name != "develop" || pairs[1].Name != "local" || pairs[3].Name != "feature/x" {
		t.Errorf("order = %s %s %s %s", pairs[0].Name, pairs[1].Name, pairs[2].Name, pairs[3].Name)
	}

	stale := pairsFrom(conf, branches, manifest, true)
	for _, p := range stale {
		if p.Skipped != "builds stale" {
			t.Errorf("stale manifest: %s skipped=%q", p.Name, p.Skipped)
		}
	}
	// No manifest at all: only the pair that names two trees runs.
	none := pairsFrom(conf, nil, nil, false)
	if none[0].Skipped != "build missing" || none[1].Skipped != "" {
		t.Errorf("without a manifest: develop=%q local=%q", none[0].Skipped, none[1].Skipped)
	}
}

func TestBuildsManifestIsStaleAfterADay(t *testing.T) {
	now := time.Now()
	fresh := &BuildsManifest{Written: now.Add(-3 * time.Hour).Format(time.RFC3339)}
	old := &BuildsManifest{Written: now.Add(-30 * time.Hour).Format(time.RFC3339)}
	unreadable := &BuildsManifest{Written: "yesterday"}
	if fresh.stale(now) || !old.stale(now) || !unreadable.stale(now) {
		t.Errorf("fresh=%t old=%t unreadable=%t", fresh.stale(now), old.stale(now), unreadable.stale(now))
	}
}

// FR-7.1: the order is random per session and the same for the same
// session; conf groups keep that order between them.
func TestConfGroupsFollowTheShuffledOrder(t *testing.T) {
	var cases []*Case
	for i, conf := range []map[string]string{{"a": "1"}, {}, {"a": "1"}, {"b": "2"}, {}, {"a": "1"}} {
		cases = append(cases, &Case{ID: "m.c" + string(rune('0'+i)), Conf: conf})
	}
	a := shuffled(cases, seedFrom("20261011_perf-weekly/develop"))
	b := shuffled(cases, seedFrom("20261011_perf-weekly/develop"))
	for i := range a {
		if a[i] != b[i] {
			t.Fatal("the same seed gave two orders")
		}
	}
	groups := confGroups(a)
	if len(groups) != 3 {
		t.Fatalf("%d groups, want 3", len(groups))
	}
	if groups[0].Key != confKey(a[0]) {
		t.Errorf("the first group is not the first case's: %q vs %q", groups[0].Key, confKey(a[0]))
	}
	n := 0
	for _, g := range groups {
		n += len(g.Cases)
		for _, c := range g.Cases {
			if confKey(c) != g.Key {
				t.Errorf("%s in group %q", c.ID, g.Key)
			}
		}
	}
	if n != len(cases) {
		t.Errorf("%d cases in groups, want %d", n, len(cases))
	}
}

func TestBudgetBoundsAndTheClock(t *testing.T) {
	c := &Case{ID: "x.y", Warmup: 1, Repeats: 5, BudgetS: 120, WarmS: 10}
	if got := caseBound(c); got != 6*2*120*time.Second+fixtureSwitchBound {
		t.Errorf("caseBound = %s", got)
	}
	if got := remeasureBound(c); got != 5*2*130*time.Second {
		t.Errorf("remeasureBound = %s", got)
	}
	now := time.Now()
	k := clock{End: now.Add(time.Hour)}
	if !k.fits(now, 59*time.Minute) || k.fits(now, 61*time.Minute) {
		t.Error("the clock does not compare with what is left")
	}
}

// Spec §13 A1: a dry run writes the builds' commits and fingerprints to
// session.json, stands no cluster up, and exits 0.
func TestSessionDryRunWritesThePlan(t *testing.T) {
	dir := t.TempDir()
	suite := filepath.Join("testdata", "suite")
	abs, _ := filepath.Abs(suite)
	tgt := fakeBuild(t, filepath.Join(dir, "develop-9fc1a2b"), "9fc1a2b000", "RelWithDebInfo")
	ref := fakeBuild(t, filepath.Join(dir, "11.4.6-0e7d3c1"), "0e7d3c1000", "RelWithDebInfo")
	conf := strings.Join([]string{
		"pair.local = " + tgt + " ; " + ref,
		"suite = " + abs,
		"branches = " + filepath.Join(abs, "branches.conf"),
		"builds = " + dir,
		"builds.manifest = " + filepath.Join(dir, "builds.json"),
		"canaries = txn.commit_single, lib.backupdb",
		"canary_tolerance = 0.05",
		"interleave = case",
		"session_budget_s = 3600",
		"csb = /nonexistent/csb",
		"csb_home = " + filepath.Join(dir, "csb"),
		"client_image = localhost/perf-client:none",
		"report.mode = dry",
		"conbench.url = http://example.invalid:5000",
	}, "\n") + "\n"
	confPath := filepath.Join(dir, "perf.conf")
	if err := os.WriteFile(confPath, []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	code, _, errs := run("session", "-c", confPath, "--dry-run", "--out", out, "--id", "20261011_perf-weekly")
	if code != ExitOK {
		t.Fatalf("code=%d\n%s", code, errs)
	}
	b, err := os.ReadFile(filepath.Join(out, "session.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc SessionDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Kind != "session" || doc.ID != "20261011_perf-weekly" || doc.State != "complete" || doc.ExitReason != "dry-run" {
		t.Errorf("doc = kind %s id %s state %s reason %s", doc.Kind, doc.ID, doc.State, doc.ExitReason)
	}
	if len(doc.Pairs) < 3 {
		t.Fatalf("%d pairs; the conf pair and the registrations are expected", len(doc.Pairs))
	}
	local := doc.Pairs[0]
	if local.Name != "local" || local.Skipped != "" || local.Target.Commit != "9fc1a2b000" || local.Reference.Commit != "0e7d3c1000" {
		t.Errorf("local pair = %+v", local)
	}
	if local.Target.Fingerprint.Compiler != "gcc 8.5.0" || local.Target.Fingerprint.Thirdparty["lz4"] != "-O3" || local.Target.Fingerprint.Partial {
		t.Errorf("fingerprint = %+v", local.Target.Fingerprint)
	}
	for _, p := range doc.Pairs[1:] {
		if p.Skipped != "build missing" {
			t.Errorf("registration %s: skipped=%q, want build missing (no manifest)", p.Name, p.Skipped)
		}
	}
	if !strings.Contains(errs, "canaries: txn.commit_single lib.backupdb") || !strings.Contains(errs, "pair local:") {
		t.Errorf("the plan was not said:\n%s", errs)
	}
	if strings.Contains(errs, "cluster create") || exists(filepath.Join(out, "clusters")) {
		t.Error("a dry run created a cluster")
	}
	// A Debug build is refused before anything else (Spec §12).
	fakeBuild(t, tgt, "9fc1a2b000", "Debug")
	if code, _, errs := run("session", "-c", confPath, "--dry-run", "--out", out); code != ExitRefused || !strings.Contains(errs, "Debug") {
		t.Errorf("Debug build: code=%d\n%s", code, errs)
	}
	// --pair must name a pair.
	if code, _, _ := run("session", "-c", confPath, "--dry-run", "--pair", "nope", "--out", out); code != ExitRefused {
		t.Errorf("--pair nope: code=%d", code)
	}
}

func TestLeaseIsHeldOnlyByItsRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".lease.json")
	if leaseHeldBy(path, "20261011_perf-weekly") {
		t.Error("no file, yet held")
	}
	_ = os.WriteFile(path, []byte(`{"id":"20261011_perf-weekly","client":"hub","user":"cubrid","since":"x","heartbeat":"y"}`), 0o644)
	if !leaseHeldBy(path, "20261011_perf-weekly") || leaseHeldBy(path, "20261011_perf-weekly-1") {
		t.Error("the lease is its run's and nobody else's")
	}
}

// Guard stage 1 looks at the host's processes and leaves the containers'
// alone: Conbench's postgres and the clusters' servers live there.
func TestForeignServersSkipContainers(t *testing.T) {
	proc := t.TempDir()
	mk := func(pid, comm, cgroup string) {
		d := filepath.Join(proc, pid)
		_ = os.MkdirAll(d, 0o755)
		_ = os.WriteFile(filepath.Join(d, "comm"), []byte(comm+"\n"), 0o644)
		_ = os.WriteFile(filepath.Join(d, "cgroup"), []byte("0::"+cgroup+"\n"), 0o644)
		_ = os.WriteFile(filepath.Join(d, "cmdline"), []byte(comm+"\x00testdb\x00"), 0o644)
		_ = os.WriteFile(filepath.Join(d, "status"), []byte("Name:\t"+comm+"\nUid:\t1000\t1000\t1000\t1000\n"), 0o644)
	}
	mk("100", "cub_server", "/user.slice/user-1000.slice/user@1000.service/user.slice/libpod-abc.scope")
	mk("200", "postgres", "/user.slice/user-1000.slice/session-3.scope")
	mk("300", "bash", "/user.slice/user-1000.slice/session-3.scope")
	mk("400", "mysqld", "/system.slice/docker-def.scope")
	got := foreignServers(proc)
	if len(got) != 1 || got[0].PID != 200 || got[0].Comm != "postgres" || got[0].UID != 1000 {
		t.Errorf("foreign = %v", got)
	}
}

// Guard stage 3: the server cores' busy time minus the clusters' is what
// something else used; swap-in or more than a core of it is contamination.
func TestHostDeltaReadsTheServerCoresAndJudges(t *testing.T) {
	stat := "cpu  1 2 3 4 5 6 7 8 0 0\ncpu0 100 0 50 1000 10 1 2 0 0 0\ncpu1 200 0 50 1000 10 1 2 0 0 0\ncpu2 999 0 0 0 0 0 0 0 0 0\n"
	busy, err := serverBusyFromStat(stat, []int{0, 1})
	if err != nil || busy != (100+50+1+2+200+50+1+2)/clockTicks {
		t.Errorf("busy = %v, %v", busy, err)
	}
	if _, err := serverBusyFromStat(stat, []int{0, 7}); err == nil {
		t.Error("a core /proc/stat does not have must be an error")
	}
	cores, err := parseCPUList("0-7,16-23")
	if err != nil || len(cores) != 16 || cores[8] != 16 {
		t.Errorf("parseCPUList = %v, %v", cores, err)
	}
	g := &hostGuard{}
	pre := hostSnapshot{at: time.Unix(0, 0), busyS: 100, clusterS: 90, pswpin: 5}
	post := hostSnapshot{at: time.Unix(60, 0), busyS: 190, clusterS: 170, pswpin: 5}
	d := g.delta(pre, post)
	if d.ForeignCPU != 10 || d.ElapsedS != 60 || d.contaminated() != "" {
		t.Errorf("10 s of foreign CPU over 60 s is not contamination: %+v (%s)", d, d.contaminated())
	}
	post.busyS = 100 + 90 + 80
	if d := g.delta(pre, post); d.contaminated() == "" {
		t.Errorf("90 s of foreign CPU over 60 s is contamination: %+v", d)
	}
	post.busyS, post.pswpin = 190, 6
	if d := g.delta(pre, post); !strings.Contains(d.contaminated(), "swapped") {
		t.Errorf("a page swapped in is contamination: %+v", d)
	}
	post.unreadable = []string{"x"}
	if d := g.delta(pre, post); d.contaminated() != "" {
		t.Error("an unreadable counter is not contamination")
	}
}

func TestCanaryPassesWithinTheTolerance(t *testing.T) {
	ok := caseRun{entry: CaseEntry{ID: "sql.pk_select"}, verdict: Verdict{Status: StatusOK, Ratio: f(1.03)}}
	out := caseRun{entry: CaseEntry{ID: "sql.pk_select"}, verdict: Verdict{Status: StatusOK, Ratio: f(1.08)}}
	null := caseRun{entry: CaseEntry{ID: "sql.pk_select", Reason: "target rep 1: budget"}, verdict: Verdict{Status: StatusNull}}
	if !canaryOf(ok, 0.05).OK || canaryOf(out, 0.05).OK || canaryOf(null, 0.05).OK {
		t.Error("canary verdicts are wrong")
	}
	if r := canaryOf(null, 0.05).Reason; !strings.Contains(r, "budget") {
		t.Errorf("a null canary says why: %q", r)
	}
}

func TestFingerprintDiffNamesWhatMoved(t *testing.T) {
	a := Fingerprint{Compiler: "gcc 8.5.0", BuildType: "RelWithDebInfo", CXXFlags: "-O2", Thirdparty: map[string]string{"lz4": "", "re2": "-O3"}}
	b := Fingerprint{Compiler: "gcc 8.5.1", BuildType: "RelWithDebInfo", CXXFlags: "-O2", Thirdparty: map[string]string{"lz4": "-O3", "re2": "-O3"}}
	d := fingerprintDiff(a, b)
	if !strings.Contains(d, "compiler gcc 8.5.0 → gcc 8.5.1") || !strings.Contains(d, "lz4 ? → -O3") || strings.Contains(d, "re2") {
		t.Errorf("diff = %q", d)
	}
	if fingerprintDiff(a, a) != "" {
		t.Error("the same fingerprint differs")
	}
}

// §5.10: the flag table is the grade-A flags of valid pairs; grade B stays
// in the sidecar; the pending table is earlier rows not in ledger.md;
// ledger_rows.md carries the flag rows.
func TestSummaryTablesAndLedgerRows(t *testing.T) {
	dir := t.TempDir()
	runs := filepath.Join(dir, "runs")
	_ = os.MkdirAll(filepath.Join(runs, "20261004_perf-weekly", "results"), 0o755)
	_ = os.WriteFile(filepath.Join(runs, "20261004_perf-weekly", "results", "ledger_rows.md"), []byte(
		"| 세션 | 케이스 | 비율 | 판정 | 근거·JIRA | 결과 ID |\n|---|---|---|---|---|---|\n"+
			"| 20261004_perf-weekly | storage.create_index@v1 | 1.08 | (미판정) | | develop/storage.create_index |\n"+
			"| 20261004_perf-weekly | txn.commit_single@v1 | 1.11 | (미판정) | | develop/txn.commit_single |\n"), 0o644)
	suiteRoot := filepath.Join(dir, "suite")
	_ = os.MkdirAll(suiteRoot, 0o755)
	_ = os.WriteFile(filepath.Join(suiteRoot, "ledger.md"), []byte(
		"| 세션 | 케이스 | 비율 | 판정 | 근거·JIRA | 결과 ID |\n|---|---|---|---|---|---|\n"+
			"| 20261004_perf-weekly | txn.commit_single@v1 | 1.11 | noise | | cb:1 |\n"), 0o644)

	s := &Session{Runner: Runner{Log: os.Stderr, Started: time.Now().Add(-2 * time.Hour), Out: filepath.Join(dir, "out"), SessionID: "20261011_perf-weekly", SuiteDir: suiteRoot}}
	_ = os.MkdirAll(s.Out, 0o755)
	s.runs = runs
	s.Conf = &Conf{ReportMode: "dry", ConbenchURL: "http://hub:5000/"}
	s.Suite = &Suite{Root: suiteRoot, Cases: []*Case{
		{ID: "txn.commit_single", Version: 1, Grade: "A", Owner: "hgryoo", Metric: "latency_s", Tolerance: 0.05},
		{ID: "storage.backupdb", Version: 1, Grade: "B", Owner: "kim", Metric: "elapsed_s", Tolerance: 0.05},
		{ID: "sql.pk_select", Version: 1, Grade: "A", Owner: "lee", Metric: "ops_per_s", Tolerance: 0.05},
	}}
	develop := &SessionPair{Name: "develop", Valid: true, CasesRun: 3, Flags: 2,
		Target: BuildRef{Ref: "develop-HEAD", Build: "/b/develop-9fc1a2b", Commit: "9fc1a2b000"}, Reference: BuildRef{Ref: "/b/11.4.6-0e7d3c1", Build: "/b/11.4.6-0e7d3c1", Commit: "0e7d3c1000"},
		Canaries: []CanaryResult{{ID: "sql.pk_select", OK: true}, {ID: "txn.commit_single", OK: true}},
		entries: []CaseEntry{
			{ID: "txn.commit_single", Version: 1, Ratio: f(1.11), Tolerance: 0.05, Flag: FlagRegression, Status: StatusOK},
			{ID: "storage.backupdb", Version: 1, Ratio: f(1.2), Tolerance: 0.05, Flag: FlagRegression, Status: StatusOK},
			{ID: "sql.pk_select", Version: 1, Ratio: f(0.93), Tolerance: 0.05, Flag: FlagImprovement, Status: StatusOK},
		}}
	gone := &SessionPair{Name: "feature/dwb-rework", Skipped: "fetch", branch: &Branch{Name: "feature/dwb-rework"}}
	s.Pairs = []*SessionPair{develop, gone}
	s.doc = &SessionDoc{State: "complete", ExitReason: "done", Pinning: PinningCPUSet, Pairs: s.Pairs, Ended: time.Now()}
	if err := s.summary(); err != nil {
		t.Fatal(err)
	}
	md, _ := os.ReadFile(filepath.Join(s.Out, "summary.md"))
	text := string(md)
	for _, want := range []string{
		"# perf-weekly", "develop 9fc1a2b vs 11.4.6 0e7d3c1",
		"세션: develop: 유효 (카나리 2/2 허용폭 안)",
		"## flag 2", "| develop | txn.commit_single@v1 | 1.11 ↑ | ±0.05 | hgryoo | `testkit perf run txn.commit_single",
		"| develop | sql.pk_select@v1 | 0.93 ↓ (improvement) | ±0.05 | lee |",
		"## 미판정 (지난 세션) 1", "| storage.create_index@v1 | 20261004_perf-weekly | 1.08 |",
		"| feature/dwb-rework | 제외됨 — fetch |",
		"conbench: http://hub:5000/cubrid/e/runs/20261011_perf-weekly",
		"report.mode=dry",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("summary.md lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "backupdb") {
		t.Error("a grade-B case is in the flag table")
	}
	rows, _ := os.ReadFile(filepath.Join(s.Out, "ledger_rows.md"))
	if !strings.Contains(string(rows), "| 20261011_perf-weekly | txn.commit_single@v1 | 1.11 | (미판정) | | develop/txn.commit_single |") || strings.Contains(string(rows), "backupdb") {
		t.Errorf("ledger_rows.md:\n%s", rows)
	}
}
