package haapply

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRestartRefusesWhatItCannotRunBeforeTouchingAPair(t *testing.T) {
	cases := map[string][]string{
		"neither":          {"restart"},
		"a pattern":        {"restart", "--cluster", "x", "pk_update"},
		"no clients":       {"restart", "--cluster", "x", "--clients", "0"},
		"too many clients": {"restart", "--cluster", "x", "--clients", "21"},
		"no iterations":    {"restart", "--cluster", "x", "--iters", "0"},
		"too many keys":    {"restart", "--cluster", "x", "--iters", "100001"},
		"kills too often":  {"restart", "--cluster", "x", "--kill-every", "500ms"},
		"no time":          {"restart", "--cluster", "x", "--timeout", "0s"},
		"keep on cluster":  {"restart", "--cluster", "x", "--keep"},
	}
	for name, args := range cases {
		var errOut bytes.Buffer
		if code := Main(args, &bytes.Buffer{}, &errOut); code != 2 {
			t.Errorf("%s: exit %d, want 2 (stderr %q)", name, code, errOut.String())
		}
	}
	a, err := parseRestart([]string{"--cluster", "x"}, &bytes.Buffer{})
	if err != nil || a.cfg.Clients != 8 || a.cfg.Iters != 3000 || a.cfg.KillEvery != 5*time.Second {
		t.Errorf("the defaults: %+v, %v", a, err)
	}
}

// A client inserts a key and commits, then moves it and commits; no key of
// one client, fresh or moved, is another's, and the last fits an INT.
func TestARestartClientMovesEachKeyItInserts(t *testing.T) {
	want := "INSERT INTO r_kv VALUES (30000001, 3, 1);\nCOMMIT;\nUPDATE r_kv SET id = 35000001 WHERE id = 30000001;\nCOMMIT;\n" +
		"INSERT INTO r_kv VALUES (30000002, 3, 2);\nCOMMIT;\nUPDATE r_kv SET id = 35000002 WHERE id = 30000002;\nCOMMIT;\n"
	if got := restartSQL(3, 2); got != want {
		t.Errorf("restartSQL(3, 2) =\n%s\nwant\n%s", got, want)
	}
	for c := 1; c < maxRestartClients; c++ {
		if c*restartStride+maxRestartIters+restartMove >= (c+1)*restartStride {
			t.Errorf("client %d's last moved key reaches client %d's", c, c+1)
		}
	}
	if maxRestartClients*restartStride+maxRestartIters+restartMove > math.MaxInt32 {
		t.Errorf("the last client's last moved key is past INT")
	}
}

func TestARestartVerdictNeedsAKillWithCommitsToSkip(t *testing.T) {
	ok := func() RestartOutcome {
		return RestartOutcome{Kills: 3, Backlog: 2, RowsMaster: 10, RowsSlave: 10, SumMaster: "55", SumSlave: "55"}
	}
	cases := []struct {
		name string
		mod  func(*RestartOutcome)
		want string
	}{
		{"alike, kills with a backlog", func(*RestartOutcome) {}, Held},
		{"the applier failed", func(o *RestartOutcome) { o.FailDelta = 1 }, Missed},
		{"a -670 in its log", func(o *RestartOutcome) { o.Dup670 = 2 }, Missed},
		{"a key brought back", func(o *RestartOutcome) { o.UnmovedSlave, o.RowsSlave = 1, 11 }, Missed},
		{"the keys sum apart", func(o *RestartOutcome) { o.SumSlave = "56" }, Missed},
		{"stalled and failed", func(o *RestartOutcome) { o.Stalled, o.FailDelta = true, 4 }, Missed},
		{"stalled behind", func(o *RestartOutcome) { o.Stalled, o.UnmovedSlave = true, 3 }, Inconclusive},
		{"no kill", func(o *RestartOutcome) { o.Kills, o.Backlog = 0, 0 }, Inconclusive},
		{"no backlog", func(o *RestartOutcome) { o.Backlog = 0 }, Inconclusive},
		{"no marker table", func(o *RestartOutcome) { o.Backlog = -1 }, Inconclusive},
	}
	for _, c := range cases {
		o := ok()
		c.mod(&o)
		if got := JudgeRestart(o); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

// restartPair is a fake pair for a restart run: the clients take two seconds, the
// slave's pkill finds an applier unless $STATE/noapplier exists (and fails as
// pkill without procps would under $STATE/pkillbroken), and each kill adds a
// start to the applier log, as heartbeat starting it again would.
func restartPair(t *testing.T) (*Runner, string, RestartConfig) {
	r, state, _ := fakePair(t, "0")
	setState(t, state, "clientlog", "Execute OK. (0.001 sec)\n")
	t.Setenv("CLIENTSLEEP", "2")
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "log"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CUBRID", home)
	applierLog := filepath.Join(home, "log", "db@localhost_applylogdb_db_m1.err")
	setState(t, home, "log/db@localhost_applylogdb_db_m1.err", "log applier: log applier started.\n")
	pkill := "#!/bin/sh\n[ -f \"$STATE/pkillbroken\" ] && exit 127\n[ -f \"$STATE/noapplier\" ] && exit 1\n" +
		"echo 'log applier: log applier started.' >> " + shellQuote(applierLog) + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(os.Getenv(fakeCSBEnv), "pkill"), []byte(pkill), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"m1", "s1"} {
		setState(t, state, "rows."+host, "6")
		setState(t, state, "sum."+host, "123")
	}
	r.BarrierFor = 2 * time.Second
	dir := t.TempDir()
	return r, state, RestartConfig{Clients: 2, Iters: 3, KillEvery: 200 * time.Millisecond, Timeout: time.Minute,
		Host: filepath.Join(dir, "restart"), Node: filepath.Join(dir, "restart"), Save: filepath.Join(dir, "ledger.d")}
}

func TestARestartRunKillsTheApplierWhileTheClientsRun(t *testing.T) {
	r, state, c := restartPair(t)
	setState(t, state, "markers", "7")
	o, err := r.Restart(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if o.Kills < 1 || o.Backlog != o.Kills || o.MarkersPeak != 7 || o.Restarts != o.Kills {
		t.Errorf("kills %d, backlog %d, peak %d, restarts %d: want kills while the clients ran, each with 7 markers and a start",
			o.Kills, o.Backlog, o.MarkersPeak, o.Restarts)
	}
	if v := JudgeRestart(o); v != Held {
		t.Errorf("verdict %s, want held: %+v", v, o)
	}
	for i := 1; i <= c.Clients; i++ {
		got, err := os.ReadFile(filepath.Join(c.Save, fmt.Sprintf("c%d.sql", i)))
		if err != nil || string(got) != restartSQL(i, c.Iters) {
			t.Errorf("client %d's script was not kept as written: %v", i, err)
		}
	}
	txlog, _ := os.ReadFile(filepath.Join(state, "txlog"))
	if !strings.Contains(string(txlog), "CREATE TABLE r_kv") {
		t.Errorf("the table was not made; writes:\n%s", txlog)
	}
}

// A slave with no marker table (a build before #8052 72650ce0d) cannot say
// whether a restart had commits to skip, so a run there that sees no second
// apply is not held.
func TestWithoutAMarkerTableARestartRunIsNotHeld(t *testing.T) {
	if raceEnabled {
		t.Skip("a restart run through the fake takes seconds under -race; TestARestartRunKillsTheApplierWhileTheClientsRun runs the kill loop there")
	}
	r, _, c := restartPair(t)
	o, err := r.Restart(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if o.Kills == 0 || o.Backlog != -1 || JudgeRestart(o) != Inconclusive {
		t.Errorf("kills %d backlog %d verdict %s, want kills, no backlog to read, inconclusive", o.Kills, o.Backlog, JudgeRestart(o))
	}
	var b bytes.Buffer
	restartRow(&b, o, JudgeRestart(o))
	if f := strings.Split(strings.TrimSpace(b.String()), "\t"); len(f) != 15 || f[3] != "-" {
		t.Errorf("the row %q should say - for the backlog", b.String())
	}
}

// The applier failing after the kills is a second apply, whatever the tables
// read.
func TestAFailingApplierAfterTheKillsIsMissed(t *testing.T) {
	if raceEnabled {
		t.Skip("a restart run through the fake takes seconds under -race; TestARestartRunKillsTheApplierWhileTheClientsRun runs the kill loop there")
	}
	r, state, c := restartPair(t)
	setState(t, state, "markers", "3")
	setState(t, state, "fail", "5")
	setState(t, state, "unmoved.s1", "2")
	o, err := r.Restart(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if o.FailDelta != 5 || o.UnmovedSlave != 2 || JudgeRestart(o) != Missed {
		t.Errorf("fail %d unmoved %d verdict %s, want 5, 2, missed", o.FailDelta, o.UnmovedSlave, JudgeRestart(o))
	}
}

// A kill that finds no applier, heartbeat not having started it again yet,
// is not counted.
func TestAKillThatFindsNoApplierIsNotCounted(t *testing.T) {
	if raceEnabled {
		t.Skip("a restart run through the fake takes seconds under -race; TestARestartRunKillsTheApplierWhileTheClientsRun runs the kill loop there")
	}
	r, state, c := restartPair(t)
	setState(t, state, "markers", "4")
	setState(t, state, "noapplier", "")
	o, err := r.Restart(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if o.Kills != 0 || o.Restarts != 0 || JudgeRestart(o) != Inconclusive {
		t.Errorf("kills %d restarts %d verdict %s, want none and inconclusive", o.Kills, o.Restarts, JudgeRestart(o))
	}
}

func TestTheRestartLedgerCarriesTheRunAndItsVerdict(t *testing.T) {
	var b bytes.Buffer
	restartHead(&b, []string{"cluster=x", "clients=2"})
	o := RestartOutcome{Kills: 4, Backlog: 4, MarkersPeak: 9, Restarts: 4, RowsMaster: 6, RowsSlave: 6,
		SumMaster: "1", SumSlave: "1"}
	restartRow(&b, o, JudgeRestart(o))
	lines := strings.Split(strings.TrimSpace(b.String()), "\n")
	if len(lines) != 3 || lines[0] != "# cluster=x clients=2" {
		t.Fatalf("ledger:\n%s", b.String())
	}
	head, row := strings.Split(lines[1], "\t"), strings.Split(lines[2], "\t")
	if len(head) != len(row) || head[len(head)-1] != "verdict" || row[len(row)-1] != Held {
		t.Errorf("header %v and row %v do not line up on a verdict", head, row)
	}
	if !strings.HasPrefix(restartLine(o, Held), "held ") {
		t.Errorf("the line starts with the verdict: %s", restartLine(o, Held))
	}
}

// A marker read that fails does not make the slave one without a marker
// table: that kill counts, and only its backlog is unknown.
func TestAFailedMarkerReadIsNotANoTableSlave(t *testing.T) {
	if raceEnabled {
		t.Skip("a restart run through the fake takes seconds under -race; TestARestartRunKillsTheApplierWhileTheClientsRun runs the kill loop there")
	}
	r, state, c := restartPair(t)
	setState(t, state, "markers", "ERR")
	o, err := r.Restart(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if o.Kills == 0 || o.Backlog != 0 || JudgeRestart(o) != Inconclusive {
		t.Errorf("kills %d backlog %d verdict %s, want kills, a backlog of 0 (not -), inconclusive", o.Kills, o.Backlog, JudgeRestart(o))
	}
}

// A pkill that fails (no procps on the slave, say) is said, and no kill is
// counted.
func TestAPkillThatFailsIsSaidAndNotCounted(t *testing.T) {
	if raceEnabled {
		t.Skip("a restart run through the fake takes seconds under -race; TestARestartRunKillsTheApplierWhileTheClientsRun runs the kill loop there")
	}
	r, state, c := restartPair(t)
	setState(t, state, "markers", "5")
	setState(t, state, "pkillbroken", "")
	var log bytes.Buffer
	r.Log = &log
	o, err := r.Restart(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if o.Kills != 0 || !strings.Contains(log.String(), `pkill on s1 said "127"`) {
		t.Errorf("kills %d, log %q: want none and the pkill failure said", o.Kills, log.String())
	}
}

func TestARestartRunWillNotWriteOverAnotherRunsFiles(t *testing.T) {
	dir := t.TempDir()
	ledger := filepath.Join(dir, "r.tsv")
	if err := os.MkdirAll(filepath.Join(dir, "r.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	setState(t, dir, "r.d/c1.log", "an earlier run's")
	var errOut bytes.Buffer
	if code := Main([]string{"restart", "--cluster", "x", "--ledger", ledger}, &bytes.Buffer{}, &errOut); code != 1 ||
		!strings.Contains(errOut.String(), "already holds files") {
		t.Errorf("exit %d, stderr %q: want 1 and a refusal", code, errOut.String())
	}
	if _, err := os.Stat(ledger); err == nil {
		t.Errorf("a ledger was made by a run that did not start")
	}
}
