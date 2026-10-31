package haapply

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/sandbox"
)

// fakeCSQL stands in for csql on the client: a write is refused when its SQL
// names what $REFUSE names, and the slave's look answers $LOOK (empty: no row
// yet).
const fakeCSQL = `#!/bin/sh
sql=""
for a in "$@"; do case "$a" in *SELECT*|*INSERT*|*DELETE*|*UPDATE*) sql="$a" ;; esac; done
case " $* " in
*" --no-auto-commit "*)
  if [ -n "$REFUSE" ] && echo "$sql" | grep -q "$REFUSE"; then echo "ERROR: Operation would have caused one or more unique constraint violations."; exit 1; fi
  if [ -n "$CRASH" ] && echo "$sql" | grep -q "$CRASH"; then exit 137; fi
  echo "1 row affected."; exit 0 ;;
esac
[ -n "$LOOK" ] && printf '\n     %s\n' "$LOOK"
exit 0
`

// The round's script is run by sh on the client; here by sh with csql faked,
// to see what each outcome prints.
func TestTheRoundScriptReportsEachOutcome(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "csql"), []byte(fakeCSQL), 0o755); err != nil {
		t.Fatal(err)
	}
	r := &Runner{Pair: &sandbox.Pair{DB: "db", Master: "m1", Slaves: []string{"s1"}}, Rows: 100, WitnessWait: time.Second}
	p := find("uk_reinsert")
	rd := p.Round(p.Name, 3)
	script := r.roundScript(p.Name, 3, rd.T1, rd.T2)

	cases := []struct {
		name, refuse, crash, look string
		want                      map[string]string
	}{
		{"T2 seen first", "", "", "0", map[string]string{"t1": "ok", "t2": "ok", "got": "0"}},
		{"T1 applied first", "", "", "100", map[string]string{"t1": "ok", "t2": "ok", "got": "100"}},
		{"witness never arrives", "", "", "", map[string]string{"t1": "ok", "t2": "ok", "got": "never"}},
		{"T1 refused", "ballast_", "", "0", map[string]string{"t1": "refused", "t2": "", "got": ""}},
		{"T2 refused", "INSERT INTO w_", "", "0", map[string]string{"t1": "ok", "t2": "refused", "got": ""}},
		// A csql that dies without an ERROR line is the run's failure, not a refusal.
		{"T2's csql killed", "", "INSERT INTO w_", "0", map[string]string{"t1": "ok", "t2": "failed rc=137", "got": ""}},
	}
	for _, c := range cases {
		cmd := exec.Command("sh", "-c", script)
		cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "REFUSE="+c.refuse, "CRASH="+c.crash, "LOOK="+c.look)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		tags := parseTags(string(out))
		for k, v := range c.want {
			if tags[k] != v {
				t.Errorf("%s: %s=%q, want %q (output %q)", c.name, k, tags[k], v, out)
			}
		}
		if c.want["t1"] == "refused" && tags["t1err"] == "" {
			t.Errorf("%s: the master's error line was not passed on: %q", c.name, out)
		}
	}
}

// Catch-up's three ends, with StallFor at three seconds and NoReadFor at two;
// the script's clock is whole seconds, so a stall is called up to a second
// early. T1 is one transaction, so a slave that is only slow reads 0 and then
// the round.
func TestCatchUpTellsASlowSlaveFromAStoppedOneAndFromNoRead(t *testing.T) {
	cases := []struct {
		name   string
		count  string
		then   []string // the count half a second apart from the start
		result string   // ok, stalled or noread
	}{
		{"applied", "100", nil, "ok"},
		{"T1 lands inside StallFor", "0", []string{"100"}, "ok"},
		{"the count keeps moving", "0", []string{"40", "80", "100"}, "ok"},
		{"T1 never lands", "0", nil, "stalled"},
		{"stuck short of the round", "40", nil, "stalled"},
		{"never readable", "ERR", nil, "noread"},
		// Read once, then not: the run's failure, not a slave that stopped.
		{"unreadable after a read", "0", []string{"ERR"}, "noread"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, state, _ := fakePair(t, c.count)
			for i, v := range c.then {
				later(time.Duration(i+1)*500*time.Millisecond, state, "count", v)
			}
			err := r.catchUp(context.Background(), "pk_update", 1)
			_, stalled := err.(errStalled)
			got := "noread"
			switch {
			case err == nil:
				got = "ok"
			case stalled:
				got = "stalled"
			case !strings.Contains(err.Error(), "could not read the slave"):
				t.Fatalf("catch-up failed otherwise: %v", err)
			}
			if got != c.result {
				t.Errorf("got %s (%v), want %s", got, err, c.result)
			}
		})
	}
}

// A slave that stops applying is the engine's result: the pattern's row says
// so with the rounds it ran, the failures counted, and its ballast dropped.
func TestAStalledPatternIsAnOutcomeNotAnError(t *testing.T) {
	r, state, _ := fakePair(t, "0")
	setState(t, state, "look", "0")
	setState(t, state, "fail", "2")
	o, err := r.Pattern(context.Background(), *find("pk_update"))
	if err != nil {
		t.Fatalf("a stalled slave ended the run: %v", err)
	}
	if !o.Stalled || o.Final != "stalled" || o.Rounds != 1 || o.FailDelta != 2 || o.Ahead != 1 {
		t.Errorf("outcome %+v, want stalled after round 1 with ahead 1 and fail 2", o)
	}
	if v := Judge([]Outcome{{Pattern: "ctl", Rounds: 3, Ahead: 3, Final: "same"}, o})[1]; v != Missed {
		t.Errorf("a stalled pattern judged %s, want %s", v, Missed)
	}
	txlog, _ := os.ReadFile(filepath.Join(state, "txlog"))
	if !strings.Contains(string(txlog), "DROP TABLE ballast_pk_update") {
		t.Errorf("the ballast was left on the master; writes:\n%s", txlog)
	}
}

// A csql that dies on the client without an ERROR line is the run's failure,
// and says which transaction it was.
func TestAKilledCsqlIsTheRunsFailure(t *testing.T) {
	for _, c := range []struct{ crash, want string }{
		{"INSERT INTO ballast_", "round 1, T1"},
		{"INSERT INTO w_", "round 1, T2"},
	} {
		r, state, _ := fakePair(t, "100")
		setState(t, state, "look", "100")
		t.Setenv("CRASH", c.crash)
		_, err := r.Pattern(context.Background(), *find("pk_update"))
		if err == nil || !strings.Contains(err.Error(), c.want) || !strings.Contains(err.Error(), "rc=137") {
			t.Errorf("csql killed on %q: err %v, want one naming %q and rc=137", c.crash, err, c.want)
		}
	}
}

// The pair's comparison is asked of both nodes, and two answers are the
// run's failure rather than a guess.
func TestTrailingSpaceIsAskedOfBothNodes(t *testing.T) {
	for _, c := range []struct {
		master, slave string
		want, fails   bool
	}{
		{"1", "1", true, false},
		{"0", "0", false, false},
		{"1", "0", false, true},
		{"0", "1", false, true},
		{"1", "ERR", false, true}, // the slave read no value
	} {
		r, state, _ := fakePair(t, "0")
		setState(t, state, "trailing.m1", c.master)
		setState(t, state, "trailing.s1", c.slave)
		got, err := r.IgnoresTrailingSpace(context.Background())
		if (err != nil) != c.fails || (err == nil && got != c.want) {
			t.Errorf("master %s slave %s: %t, %v", c.master, c.slave, got, err)
		}
	}
}

func TestTheStallBoundGrowsWithTheBallast(t *testing.T) {
	for rows, want := range map[int]time.Duration{
		1: 2 * time.Minute, 20000: 2 * time.Minute, 20001: 4 * time.Minute, 200000: 20 * time.Minute,
	} {
		if got := stallFor(rows); got != want {
			t.Errorf("stallFor(%d) = %s, want %s", rows, got, want)
		}
	}
}
