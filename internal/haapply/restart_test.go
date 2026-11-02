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
		"kills something":  {"restart", "--cluster", "x", "--kill", "server"},
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
	if err != nil || a.cfg.Clients != 8 || a.cfg.Iters != 3000 || a.cfg.Kill != KillApplier || a.cfg.KillEvery != 5*time.Second {
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
		{"a log that lost lines", func(o *RestartOutcome) { o.Restarts, o.Dup670 = -1, -1 }, Inconclusive},
		{"a lost log that gained -670s", func(o *RestartOutcome) { o.Restarts, o.Dup670 = -1, 2 }, Missed},
		{"a lost log and a failure", func(o *RestartOutcome) { o.Restarts, o.Dup670, o.FailDelta = -1, -1, 1 }, Missed},
		{"a lost log and a key back", func(o *RestartOutcome) { o.Restarts, o.Dup670, o.UnmovedSlave = -1, -1, 1 }, Missed},
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
// start to the applier log, as heartbeat starting it again would: first
// rotating it as er_file_backup does under $STATE/rotate, and followed by a
// -670 under $STATE/dup670. The slave's timeout fails under
// $STATE/notimeout.
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
	l := shellQuote(applierLog)
	pkill := "#!/bin/sh\n[ -f \"$STATE/pkillbroken\" ] && exit 127\n[ -f \"$STATE/noapplier\" ] && exit 1\n" +
		"[ -f \"$STATE/rotate\" ] && { rm -f " + l + ".bak; mv " + l + " " + l + ".bak; }\n" +
		"echo 'log applier: log applier started.' >> " + l + "\n" +
		"[ -f \"$STATE/dup670\" ] && echo 'failed to apply update replication log, server error: -670' >> " + l + "\nexit 0\n"
	timeout := "#!/bin/sh\n[ -f \"$STATE/notimeout\" ] && exit 127\nshift\nexec \"$@\"\n"
	for name, script := range map[string]string{"pkill": pkill, "timeout": timeout} {
		if err := os.WriteFile(filepath.Join(os.Getenv(fakeCSBEnv), name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, host := range []string{"m1", "s1"} {
		setState(t, state, "rows."+host, "6")
		setState(t, state, "sum."+host, "123")
	}
	r.BarrierFor = 2 * time.Second
	dir := t.TempDir()
	return r, state, RestartConfig{Clients: 2, Iters: 3, Kill: KillApplier, KillEvery: 200 * time.Millisecond, Timeout: time.Minute,
		Host: filepath.Join(dir, "restart"), Node: filepath.Join(dir, "restart"), Save: filepath.Join(dir, "ledger.d")}
}

func TestARestartRunKillsTheApplierWhileTheClientsRun(t *testing.T) {
	r, state, c := restartPair(t)
	c.Kill = "" // the applier, as RestartConfig's zero value
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

// A slave whose applier log never says the applier started has no applier
// this run can count the starts of, so the run does not begin.
func TestARestartRunNeedsAnApplierLogThatSaysItStarted(t *testing.T) {
	r, state, c := restartPair(t)
	setState(t, state, "markers", "5")
	setState(t, os.Getenv("CUBRID"), "log/db@localhost_applylogdb_db_m1.err", "")
	if _, err := r.Restart(context.Background(), c); err == nil || !strings.Contains(err.Error(), "says the applier started") {
		t.Errorf("err %v, want a refusal for a log with no start", err)
	}
	if _, err := os.Stat(filepath.Join(c.Host, "c1.log")); err == nil {
		t.Errorf("the clients ran on a slave with no applier start")
	}
}

// The marker read before each kill is bounded by timeout(1), so a slave with
// the marker table and no working timeout does not begin a run every read of
// which would fail.
func TestARestartRunNeedsTimeoutOnTheSlave(t *testing.T) {
	r, state, c := restartPair(t)
	setState(t, state, "markers", "5")
	setState(t, state, "notimeout", "")
	if _, err := r.Restart(context.Background(), c); err == nil || !strings.Contains(err.Error(), "no timeout(1) on s1") {
		t.Errorf("err %v, want a refusal for a slave with no timeout", err)
	}
	if _, err := os.Stat(filepath.Join(c.Host, "c1.log")); err == nil {
		t.Errorf("the clients ran on a slave with no timeout")
	}
}

// An applier log that lost lines during the run leaves its starts and -670s
// unknown: the row says so, and the run is judged without them.
func TestAnApplierLogThatLostLinesIsNotHeld(t *testing.T) {
	if raceEnabled {
		t.Skip("a restart run through the fake takes seconds under -race; TestARestartRunKillsTheApplierWhileTheClientsRun runs the kill loop there")
	}
	r, state, c := restartPair(t)
	setState(t, state, "markers", "5")
	setState(t, state, "rotate", "")
	setState(t, os.Getenv("CUBRID"), "log/db@localhost_applylogdb_db_m1.err.bak", strings.Repeat("log applier: log applier started.\n", 50))
	o, err := r.Restart(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if o.Kills == 0 || o.Backlog != o.Kills || o.Restarts != -1 || o.Dup670 != -1 || JudgeRestart(o) != Inconclusive {
		t.Errorf("kills %d backlog %d restarts %d -670 %d verdict %s, want kills with markers, -1, -1, inconclusive",
			o.Kills, o.Backlog, o.Restarts, o.Dup670, JudgeRestart(o))
	}
	var b bytes.Buffer
	restartRow(&b, o, JudgeRestart(o))
	if f := strings.Split(strings.TrimSpace(b.String()), "\t"); len(f) != 15 || f[5] != "-" || f[12] != "-" {
		t.Errorf("the row %q should say - for the restarts and the -670s", b.String())
	}
}

// Lines are only lost, so -670s that still grew across a rotation came during
// the run: at least that many, and the run is missed.
func TestMinus670sThatGrewAcrossARotationAreMissed(t *testing.T) {
	if raceEnabled {
		t.Skip("a restart run through the fake takes seconds under -race; TestARestartRunKillsTheApplierWhileTheClientsRun runs the kill loop there")
	}
	r, state, c := restartPair(t)
	setState(t, state, "markers", "5")
	setState(t, state, "rotate", "")
	setState(t, os.Getenv("CUBRID"), "log/db@localhost_applylogdb_db_m1.err.bak", strings.Repeat("log applier: log applier started.\n", 50))
	setState(t, state, "dup670", "")
	o, err := r.Restart(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if o.Restarts != -1 || o.Dup670 < 1 || JudgeRestart(o) != Missed {
		t.Errorf("restarts %d -670 %d verdict %s, want -1, at least 1, missed", o.Restarts, o.Dup670, JudgeRestart(o))
	}
}

// A kill the slave's channel could not run is said, and the loop goes on to
// the next; the clients run to their end either way.
func TestAKillThatCannotBeMadeIsSaidAndTheRunGoesOn(t *testing.T) {
	if raceEnabled {
		t.Skip("a restart run through the fake takes seconds under -race; TestARestartRunKillsTheApplierWhileTheClientsRun runs the kill loop there")
	}
	r, state, c := restartPair(t)
	setState(t, state, "markers", "5")
	t.Setenv("FAKE_EXEC_FAIL", "cub_admin [a]pplylogdb")
	var log bytes.Buffer
	r.Log = &log
	o, err := r.Restart(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(log.String(), "a kill on s1 was not made"); o.Kills != 0 || n < 2 || JudgeRestart(o) != Inconclusive {
		t.Errorf("kills %d, %d failed kills said, verdict %s: want none, the loop past the first, inconclusive; log %q",
			o.Kills, n, JudgeRestart(o), log.String())
	}
}

// crashHooks makes the fake csb's node kill and start append "crash <node>"
// and "start <node>" to one file, the start also a start to the applier log
// as heartbeat would, and returns the file.
func crashHooks(t *testing.T, state, crashFirst string) string {
	t.Helper()
	nodes := filepath.Join(state, "nodelog")
	applierLog := filepath.Join(os.Getenv("CUBRID"), "log", "db@localhost_applylogdb_db_m1.err")
	t.Setenv("FAKE_NODE_KILL", crashFirst+`echo "crash $1" >> `+shellQuote(nodes))
	t.Setenv("FAKE_NODE_START", `echo "start $1" >> `+shellQuote(nodes)+
		"; echo 'log applier: log applier started.' >> "+shellQuote(applierLog))
	return nodes
}

// Under --kill node the run crashes the slave node and then starts it, each
// time, through csb and with no pkill: the fake pkill would add a start of
// its own to the applier log.
func TestARestartRunCanCrashTheSlaveNode(t *testing.T) {
	if raceEnabled {
		t.Skip("a restart run through the fake takes seconds under -race; TestARestartRunKillsTheApplierWhileTheClientsRun runs the kill loop there")
	}
	r, state, c := restartPair(t)
	c.Kill = KillNode
	setState(t, state, "markers", "7")
	nodes := crashHooks(t, state, "")
	o, err := r.Restart(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(nodes)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	for i, l := range lines {
		if want := [2]string{"crash s1", "start s1"}[i%2]; l != want {
			t.Fatalf("line %d of the node calls is %q, want %q:\n%s", i+1, l, want, b)
		}
	}
	if o.Kills < 1 || len(lines) != 2*o.Kills || o.Backlog != o.Kills || o.Restarts != o.Kills {
		t.Errorf("kills %d, node calls %d, backlog %d, restarts %d: want a crash and a start for every kill, each with markers, and no pkill",
			o.Kills, len(lines), o.Backlog, o.Restarts)
	}
	if v := JudgeRestart(o); v != Held {
		t.Errorf("verdict %s, want held", v)
	}
}

// A crashed node that does not start again ends the kills, and the run fails
// once the clients end, saying how to bring it back, rather than wait for the
// final marker on a slave that is down.
func TestACrashedNodeThatDoesNotStartEndsTheRun(t *testing.T) {
	if raceEnabled {
		t.Skip("a restart run through the fake takes seconds under -race; TestARestartRunKillsTheApplierWhileTheClientsRun runs the kill loop there")
	}
	r, state, c := restartPair(t)
	c.Kill = KillNode
	setState(t, state, "markers", "7")
	t.Setenv("FAKE_NODE_START", "exit 1")
	var log bytes.Buffer
	r.Log = &log
	o, err := r.Restart(context.Background(), c)
	if err == nil || !strings.Contains(err.Error(), "csb node start s1") {
		t.Errorf("err %v, want the run to fail saying how to start s1", err)
	}
	if o.Kills != 1 || !o.Down || !strings.Contains(log.String(), "s1 did not start again after a crash or an attempt at one") {
		t.Errorf("kills %d down %v, log %q: want one kill, the slave down and the failed start said", o.Kills, o.Down, log.String())
	}
}

// A run stopped while a crash is under way still starts the node: a crash
// and its start do not take the run's cancel.
func TestAStoppedRunStillStartsTheNodeItCrashed(t *testing.T) {
	r, state, _ := restartPair(t)
	crashing := filepath.Join(state, "crashing")
	nodes := crashHooks(t, state, "touch "+shellQuote(crashing)+"; sleep 2; ")
	ch, err := r.Pair.SlaveChannel(0)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { // cancel once the crash is under way
		for ctx.Err() == nil {
			if _, err := os.Stat(crashing); err == nil {
				cancel()
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()
	var log bytes.Buffer
	r.Log = &log
	_, killed, down := r.killOnce(ctx, ch, killScript("db", false, false), KillNode)
	b, _ := os.ReadFile(nodes)
	if !killed || down || string(b) != "crash s1\nstart s1\n" || !strings.Contains(log.String(), "s1 is started again first") {
		t.Errorf("killed %v down %v, node calls %q, log %q: want the crash and then the start despite the cancel, said",
			killed, down, b, log.String())
	}
}

func TestTheKillScriptKillsTheApplierOnlyWhenAskedTo(t *testing.T) {
	if s := killScript("db", true, true); !strings.Contains(s, "pkill -9 -f 'cub_admin [a]pplylogdb'") || !strings.Contains(s, "timeout 10") {
		t.Errorf("the applier's kill script reads the markers and kills:\n%s", s)
	}
	if s := killScript("db", true, false); strings.Contains(s, "pkill") || !strings.Contains(s, "markers=") {
		t.Errorf("a node crash's script only reads the markers:\n%s", s)
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

// A crash csb said failed may still have reached the node, so the start
// follows it; the crash is not counted.
func TestAFailedCrashIsStillFollowedByAStart(t *testing.T) {
	r, state, _ := restartPair(t)
	nodes := crashHooks(t, state, "")
	t.Setenv("FAKE_NODE_KILL", `echo "crash $1" >> `+shellQuote(nodes)+"; exit 1")
	ch, err := r.Pair.SlaveChannel(0)
	if err != nil {
		t.Fatal(err)
	}
	_, killed, down := r.killOnce(context.Background(), ch, killScript("db", false, false), KillNode)
	b, _ := os.ReadFile(nodes)
	if killed || down || string(b) != "crash s1\nstart s1\n" {
		t.Errorf("killed %v down %v, node calls %q: want the start after the failed crash, and no kill counted", killed, down, b)
	}
}
