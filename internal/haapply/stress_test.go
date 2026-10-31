package haapply

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var allOps = []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

// The golden files are the shell prototype's generator run with mawk (busybox
// awk prints the same bytes): the awk program of
// docs/project/evidence/ha/parallel-apply-stress.sh, with
// SEED c TXNS KEYS OPS BALLAST of 45 1 40 20 0..9 3000, 45 3 40 20 0..9 3000,
// and 83 2 40 12 5,6,7,8 500.
func TestASeedNamesTheShellPrototypesSQL(t *testing.T) {
	cases := []struct {
		file                         string
		seed, c, txns, keys, ballast int
		ops                          []int
	}{
		{"stress-seed45-c1.sql", 45, 1, 40, 20, 3000, allOps},
		{"stress-seed45-c3.sql", 45, 3, 40, 20, 3000, allOps},
		{"stress-seed83-c2-ops5678.sql", 83, 2, 40, 12, 500, []int{5, 6, 7, 8}},
	}
	for _, c := range cases {
		want, err := os.ReadFile(filepath.Join("testdata", c.file))
		if err != nil {
			t.Fatal(err)
		}
		if got := stressSQL(c.seed, c.c, c.txns, c.keys, c.ballast, c.ops); got != string(want) {
			gl, wl := strings.Split(got, "\n"), strings.Split(string(want), "\n")
			for i := 0; i < len(gl) && i < len(wl); i++ {
				if gl[i] != wl[i] {
					t.Errorf("%s: line %d is %q, the shell's %q", c.file, i+1, gl[i], wl[i])
					break
				}
			}
			if len(gl) != len(wl) {
				t.Errorf("%s: %d lines, the shell's %d", c.file, len(gl), len(wl))
			}
		}
	}
}

func TestAStressVerdictNamesTheWorstOfWhatItFound(t *testing.T) {
	ok := func() []TableCheck {
		return []TableCheck{{"s_kv", true, 3, 3, 3, 3}, {"s_parent", true, 2, 2, 2, 2}, {"s_child", true, 1, 1, 1, 1}}
	}
	cases := []struct {
		name string
		mod  func(*StressOutcome)
		want string
	}{
		{"all alike", func(*StressOutcome) {}, StressSame},
		{"a heap-only row on the master", func(o *StressOutcome) { o.Tables[2] = TableCheck{"s_child", true, 9, 7, 7, 7} }, StressDamaged},
		{"a heap-only row on the slave", func(o *StressOutcome) { o.Tables[2] = TableCheck{"s_child", true, 7, 9, 7, 7} }, StressDamaged},
		{"damaged wins over the rest", func(o *StressOutcome) {
			o.Tables[2] = TableCheck{"s_child", false, 9, 7, 7, 7}
			o.Hung, o.Stalled, o.FailDelta = true, true, 3
		}, StressDamaged},
		{"a slave behind", func(o *StressOutcome) { o.Stalled = true }, StressStalled},
		// Read while it applies, a slave's two counts can see two states.
		{"a slave behind, read apart", func(o *StressOutcome) {
			o.Stalled, o.Tables[0].PKSlave = true, 2
		}, StressStalled},
		{"stalled wins over diverged", func(o *StressOutcome) { o.Stalled, o.Tables[0].Same = true, false }, StressStalled},
		{"a table read differently", func(o *StressOutcome) { o.Tables[0].Same = false }, StressDiverged},
		{"the slave's heap short", func(o *StressOutcome) {
			o.Tables[1].HeapSlave, o.Tables[1].PKSlave = 1, 1
		}, StressDiverged},
		{"the ballast differs", func(o *StressOutcome) { o.BallastSame = false }, StressDiverged},
		{"an apply failure", func(o *StressOutcome) { o.FailDelta = 1 }, StressDiverged},
		{"an orphan on one node", func(o *StressOutcome) { o.OrphansMaster = 1 }, StressDiverged},
		{"diverged wins over hung", func(o *StressOutcome) { o.Hung, o.BallastSame = true, false }, StressDiverged},
		{"stopped clients, alike nodes", func(o *StressOutcome) { o.Hung = true }, StressHung},
		// The master's orphans are the master's state, and a slave that has them
		// too holds what the master holds.
		{"orphans on both", func(o *StressOutcome) { o.OrphansMaster, o.OrphansSlave = 2, 2 }, StressSame},
	}
	for _, c := range cases {
		o := StressOutcome{Ran: true, Tables: ok(), BallastSame: true}
		c.mod(&o)
		if got := JudgeStress(o); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

func TestStressRefusesWhatItCannotRunBeforeTouchingAPair(t *testing.T) {
	cases := map[string][]string{
		"neither":                   {"stress"},
		"a pattern":                 {"stress", "--cluster", "x", "pk_update"},
		"compare-only on a build":   {"stress", "--build", "/b", "--compare-only"},
		"compare-only with a seed":  {"stress", "--cluster", "x", "--compare-only", "--seed", "3"},
		"compare-only with clients": {"stress", "--cluster", "x", "--compare-only", "--clients", "2"},
		"no clients":                {"stress", "--cluster", "x", "--clients", "0"},
		"too many clients":          {"stress", "--cluster", "x", "--clients", "22"},
		"too many transactions":     {"stress", "--cluster", "x", "--txns", "10000"},
		"keys past INT":             {"stress", "--cluster", "x", "--clients", "21", "--txns", "9999"},
		"too much ballast":          {"stress", "--cluster", "x", "--ballast", "10001"},
		"no keys":                   {"stress", "--cluster", "x", "--keys", "0"},
		"a negative seed":           {"stress", "--cluster", "x", "--seed", "-1"},
		"a seed past the shell's":   {"stress", "--cluster", "x", "--seed", "1000000"},
		"an operation that is not":  {"stress", "--cluster", "x", "--ops", "1,10"},
		"operations that are not":   {"stress", "--cluster", "x", "--ops", "a"},
		"no time":                   {"stress", "--cluster", "x", "--timeout", "0s"},
		"under a second":            {"stress", "--cluster", "x", "--timeout", "500ms"},
		"keys past an INT":          {"stress", "--cluster", "x", "--keys", "2147483648"},
		"workers on cluster":        {"stress", "--cluster", "x", "--workers", "2"},
	}
	for name, args := range cases {
		var errOut bytes.Buffer
		if code := Main(args, &bytes.Buffer{}, &errOut); code != 2 {
			t.Errorf("%s: exit %d, want 2 (stderr %q)", name, code, errOut.String())
		}
	}
	a, err := parseStress([]string{"--cluster", "x"}, &bytes.Buffer{})
	if err != nil || len(a.cfg.Ops) != 10 || a.cfg.Clients != 8 || a.cfg.Seed != 1 {
		t.Errorf("the defaults: %+v, %v", a, err)
	}
}

// stressPair is a fake pair whose client scripts and logs go to a directory
// the fake node and this test both see. Each client ends its transactions.
func stressPair(t *testing.T) (*Runner, string, StressConfig) {
	r, state, _ := fakePair(t, "0")
	setState(t, state, "clientlog", "Execute OK. (0.001 sec)\n")
	dir := t.TempDir()
	return r, state, StressConfig{Clients: 3, Txns: 5, Keys: 4, Ballast: 10, Seed: 7, Ops: allOps,
		Timeout: time.Minute, Host: filepath.Join(dir, "stress-7"), Node: filepath.Join(dir, "stress-7"),
		Save: filepath.Join(dir, "ledger.d")}
}

// The damaged pair of fk-index-insert-loses-the-victims-error, as its tables
// read: the master's heap has 9 children, its primary key 7, the slave 7 and 7.
func TestCompareOnlyCallsAHeapOnlyRowDamaged(t *testing.T) {
	r, state, c := stressPair(t)
	for host, n := range map[string]string{"m1": "9", "s1": "7"} {
		setState(t, state, "heap.s_child."+host, n)
		setState(t, state, "pk.s_child."+host, "7")
	}
	o, err := r.Stress(context.Background(), c, true)
	if err != nil {
		t.Fatal(err)
	}
	if v := JudgeStress(o); v != StressDamaged {
		t.Errorf("verdict %s, want %s: %+v", v, StressDamaged, o)
	}
	if txlog, _ := os.ReadFile(filepath.Join(state, "txlog")); len(txlog) != 0 {
		t.Errorf("compare-only wrote to the master:\n%s", txlog)
	}
	var row bytes.Buffer
	stressRow(&row, o, StressDamaged)
	f := strings.Split(strings.TrimSpace(row.String()), "\t")
	if f[0] != "-" || f[9] != "-" || f[15] != "9/7/7/7" || f[19] != "caught" || f[len(f)-1] != StressDamaged {
		t.Errorf("row %q", row.String())
	}
	if _, err := os.Stat(c.Save); err == nil {
		t.Errorf("a comparison with nothing different kept files in %s", c.Save)
	}
}

// A run makes the tables, gives each client the script its seed names, counts
// the master's refusals in their logs, judges the pair, and keeps the clients'
// files beside the ledger.
func TestARunGivesEachClientItsScriptAndCountsTheRefusals(t *testing.T) {
	r, state, c := stressPair(t)
	setState(t, state, "clientlog", "Execute OK. (0.001 sec)\nERROR: Operation would have caused one or more unique constraint violations.\n")
	o, err := r.Stress(context.Background(), c, false)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= c.Clients; i++ {
		got, err := os.ReadFile(filepath.Join(c.Host, fmt.Sprintf("c%d.sql", i)))
		if err != nil || string(got) != stressSQL(7, i, 5, 4, 10, allOps) {
			t.Errorf("client %d's script is not its seed's: %v", i, err)
		}
	}
	txlog, _ := os.ReadFile(filepath.Join(state, "txlog"))
	if !strings.Contains(string(txlog), "CREATE TABLE s_child") {
		t.Errorf("the tables were not made; writes:\n%s", txlog)
	}
	if o.Errors != 3 || o.Hung || len(o.Tables) != 3 || JudgeStress(o) != StressSame {
		t.Errorf("outcome %+v (%s), want 3 refusals and same", o, JudgeStress(o))
	}
	for _, f := range []string{"c1.sql", "c3.log", "pids"} {
		if _, err := os.Stat(filepath.Join(c.Save, f)); err != nil {
			t.Errorf("%s was not kept beside the ledger: %v", f, err)
		}
	}
}

// A client that ended no transaction -- it could not connect, say -- leaves a
// pair that reads the same, and that is the run's failure, not a verdict.
func TestAClientThatRanNothingFailsTheRun(t *testing.T) {
	r, state, c := stressPair(t)
	setState(t, state, "clientlog", "ERROR: Failed to connect to database server, 'db', on the following host(s): m1\n")
	if _, err := r.Stress(context.Background(), c, false); err == nil || !strings.Contains(err.Error(), "ended no transaction") {
		t.Errorf("err %v, want a client that ended no transaction", err)
	}
}

// A table the nodes read differently is kept as both reads.
func TestADifferingTableIsKeptAsBothReads(t *testing.T) {
	r, _, c := stressPair(t)
	t.Setenv("DIFFER", "s_kv")
	o, err := r.Stress(context.Background(), c, false)
	if err != nil {
		t.Fatal(err)
	}
	if JudgeStress(o) != StressDiverged {
		t.Errorf("verdict %s, want %s", JudgeStress(o), StressDiverged)
	}
	for node, host := range map[string]string{"master": "m1", "slave": "s1"} {
		b, err := os.ReadFile(filepath.Join(c.Save, "s_kv."+node))
		if err != nil || !strings.Contains(string(b), host) {
			t.Errorf("the %s's read of s_kv was not kept: %q, %v", node, b, err)
		}
	}
}

// A slave that does not apply up to the marker is a finding when it can still
// be read: compared, and stalled.
func TestASlaveThatDoesNotCatchUpIsStalled(t *testing.T) {
	r, _, c := stressPair(t)
	t.Setenv("STALL", "1")
	o, err := r.Stress(context.Background(), c, true)
	if err != nil {
		t.Fatal(err)
	}
	if !o.Stalled || len(o.Tables) != 3 || JudgeStress(o) != StressStalled {
		t.Errorf("outcome %+v (%s), want stalled and compared", o, JudgeStress(o))
	}
}

// A master that will not take the marker says nothing about the slave: the
// run fails.
func TestAMasterThatRefusesTheMarkerFailsTheRun(t *testing.T) {
	r, _, c := stressPair(t)
	t.Setenv("NOMARKER", "1")
	o, err := r.Stress(context.Background(), c, true)
	if err == nil || o.Stalled {
		t.Errorf("err %v, stalled %t: want the run's failure", err, o.Stalled)
	}
}

// An earlier run's files beside the ledger would be taken for this one's, and
// are not this run's to remove: it stops before touching a pair.
func TestARunWillNotWriteOverAnotherRunsFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "l.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	setState(t, filepath.Join(dir, "l.d"), "c1.log", "an earlier run's")
	var errOut bytes.Buffer
	code := Main([]string{"stress", "--cluster", "x", "--ledger", filepath.Join(dir, "l.tsv")}, &bytes.Buffer{}, &errOut)
	if code != 1 || !strings.Contains(errOut.String(), "already holds files") {
		t.Errorf("exit %d, stderr %q", code, errOut.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "l.tsv")); err == nil {
		t.Errorf("a ledger was made")
	}
}

// A ledger that was at the path is replaced once there is a pair to compare.
func TestAComparisonReplacesAnEarlierLedger(t *testing.T) {
	fakeEnv(t, "0")
	ledger := filepath.Join(t.TempDir(), "l.tsv")
	if err := os.WriteFile(ledger, []byte(strings.Repeat("an earlier run\n", 1000)), 0o644); err != nil {
		t.Fatal(err)
	}
	var errOut bytes.Buffer
	if code := Main([]string{"stress", "--cluster", "fake", "--compare-only", "--ledger", ledger},
		&bytes.Buffer{}, &errOut); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut.String())
	}
	b, err := os.ReadFile(ledger)
	if err != nil || !strings.HasPrefix(string(b), "# cluster=fake ") || strings.Contains(string(b), "an earlier run") {
		t.Errorf("ledger %q, %v", b, err)
	}
}

// Clients still running at the timeout are stopped, and the pair is compared
// all the same.
func TestClientsPastTheTimeoutAreStoppedAndThePairStillCompared(t *testing.T) {
	r, _, c := stressPair(t)
	t.Setenv("HANG", "1")
	c.Timeout = time.Second
	start := time.Now()
	o, err := r.Stress(context.Background(), c, false)
	if err != nil {
		t.Fatal(err)
	}
	if !o.Hung || len(o.Tables) != 3 || JudgeStress(o) != StressHung {
		t.Errorf("outcome %+v (%s), want hung and compared", o, JudgeStress(o))
	}
	// The fake client waits five minutes; this is well short of it, and long
	// enough for the race detector's slowness.
	if d := time.Since(start); d > 2*time.Minute {
		t.Errorf("the stop took %s", d)
	}
}
