package haapply

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/sandbox"
)

// Runner drives one pair through patterns. Every statement goes from the
// client node to the master or the slave by host, so a node's own load is not
// in what is measured (ADR-EXT-012 3-5).
type Runner struct {
	Pair   *sandbox.Pair
	Client *sandbox.Node
	Rounds int
	Rows   int
	// Log takes what a run says on the way: refusals, timeouts.
	Log io.Writer
	// WitnessWait is how long a round waits for T2's witness on the slave.
	// StallFor is how long the slave's count of a round's ballast may stand
	// still, short of the round, before the slave counts as stopped.
	// NoReadFor is how long catch-up may go without reading the slave at all
	// before it calls the run broken rather than the slave. BarrierFor bounds
	// the wait for the slave to apply a marker.
	WitnessWait time.Duration
	StallFor    time.Duration
	NoReadFor   time.Duration
	BarrierFor  time.Duration
}

const txTimeout = 10 * time.Minute

func (r *Runner) logf(format string, a ...any) {
	if r.Log != nil {
		fmt.Fprintf(r.Log, format+"\n", a...)
	}
}

func (r *Runner) at(host string) string { return shellQuote(r.Pair.DB + "@" + host) }

func (r *Runner) slave() string { return r.Pair.Slaves[0] }

// sh runs a script on the client node and returns what it printed.
func (r *Runner) sh(ctx context.Context, script string, timeout time.Duration) (string, error) {
	res, err := r.Client.RunFor(ctx, script, timeout)
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return res.Stdout, fmt.Errorf("client %s: exit %d: %s", r.Client.Name, res.ExitCode, strings.TrimSpace(res.Stderr))
	}
	return res.Stdout, nil
}

// txScript is the shell for one transaction on the master, tagging its result
// <tag>=ok, <tag>=refused (with <tag>err=<the master's first ERROR line>) or
// <tag>=failed rc=<n> for a csql that ended badly without saying why. It ends
// the script on anything but ok.
func (r *Runner) txScript(tag, sql string) string {
	return fmt.Sprintf(`out=$(csql -u dba --no-auto-commit -c %[2]s %[3]s 2>&1); rc=$?
case "$out" in *ERROR*) echo "%[1]s=refused"; echo "%[1]serr=$(echo "$out" | grep ERROR | head -1)"; exit 0 ;; esac
[ $rc -ne 0 ] && { echo "%[1]s=failed rc=$rc"; exit 0; }
echo "%[1]s=ok"
`, tag, shellQuote(sql+" COMMIT;"), r.at(r.Pair.Master))
}

// tx runs one transaction on the master. A write the master refuses is not an
// error of the run: it comes back as the master's first ERROR line.
func (r *Runner) tx(ctx context.Context, sql string) (refused string, err error) {
	out, err := r.sh(ctx, r.txScript("tx", sql), txTimeout)
	if err != nil {
		return "", err
	}
	tags := parseTags(out)
	switch {
	case tags["tx"] == "ok":
		return "", nil
	case tags["tx"] == "refused":
		return tags["txerr"], nil
	}
	return "", fmt.Errorf("csql on the client: %s", firstNonEmpty(tags["tx"], strings.TrimSpace(out)))
}

// q is a one-value read; no value is an error, because every read here
// expects one.
func (r *Runner) q(ctx context.Context, host, sql string) (string, error) {
	out, err := r.sh(ctx, "csql -u dba -t -c "+shellQuote(sql)+" "+r.at(host)+" 2>&1", 2*time.Minute)
	if err != nil {
		return "", err
	}
	v := firstNumber(out)
	if v == "" {
		return "", fmt.Errorf("%s on %s read no value: %s", sql, host, strings.TrimSpace(out))
	}
	return v, nil
}

// dump is a table as csql prints it, without what differs between runs.
func (r *Runner) dump(ctx context.Context, host, sql string) (string, error) {
	out, err := r.sh(ctx, "csql -u dba -c "+shellQuote(sql)+" "+r.at(host)+" 2>&1", 2*time.Minute)
	if err != nil {
		return "", err
	}
	if strings.Contains(out, "ERROR") || strings.TrimSpace(out) == "" {
		return "", fmt.Errorf("%s on %s: %s", sql, host, firstNonEmpty(firstError(out), "no output"))
	}
	return normalise(out), nil
}

func firstError(out string) string {
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "ERROR") {
			return strings.TrimSpace(l)
		}
	}
	return ""
}

// writeset reads the master's counters. A build without the writeset reads as
// zero, which a run reports rather than refuses.
func (r *Runner) writeset(ctx context.Context) (Writeset, error) {
	out, err := r.sh(ctx, "csql -u dba -l -c 'SHOW WRITESET STATUS' "+r.at(r.Pair.Master)+" 2>/dev/null; true", 2*time.Minute)
	return parseWriteset(out), err
}

func (r *Runner) failCounter(ctx context.Context) (int, error) {
	s, err := r.q(ctx, r.slave(), "SELECT fail_counter FROM db_ha_apply_info")
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(s)
}

func (r *Runner) barrier(ctx context.Context) error {
	_, err := r.Pair.WaitForReplication(ctx, r.BarrierFor)
	return err
}

// roundScript commits T1 (the ballast, then the pattern's first operation),
// then T2 with its witness, and then reads the slave until the witness is
// there: the count of T1's ballast beside it says which was applied first.
// One script on the client, so no csb round trip sits between T1's commit and
// T2's, or between T2's commit and the first look -- a gap there lets the slave
// finish T1 first and hides an overtake (the shell prototype ran them back to
// back in the same way).
func (r *Runner) roundScript(p string, round int, t1, t2 string) string {
	ballast := fmt.Sprintf("INSERT INTO ballast_%s SELECT %d + ROWNUM, %d, REPEAT('x', 1500) FROM db_root CONNECT BY LEVEL <= %d; %s",
		p, round*r.Rows, round, r.Rows, t1)
	witness := fmt.Sprintf("%s INSERT INTO w_%s VALUES (%d);", t2, p, round)
	look := shellQuote(fmt.Sprintf("SELECT (SELECT COUNT(*) FROM ballast_%[1]s WHERE r = %[2]d) FROM w_%[1]s WHERE r = %[2]d", p, round))
	return r.txScript("t1", ballast) + r.txScript("t2", witness) + fmt.Sprintf(`t0=$(date +%%s)
while :; do
  got=$(csql -u dba -t -c %s %s 2>/dev/null | awk '/^ *-?[0-9]+ *$/ { print $1; exit }')
  [ -n "$got" ] && break
  [ $(( $(date +%%s) - t0 )) -ge %d ] && { got=never; break; }
done
echo "got=$got"
`, look, r.at(r.slave()), int(r.WitnessWait.Seconds()))
}

// parseTags reads the key=value lines a script printed, ignoring anything a
// login profile may have printed before them.
func parseTags(out string) map[string]string {
	tags := map[string]string{}
	for _, l := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(l), "=")
		if ok && k != "" && !strings.ContainsAny(k, " \t") {
			tags[k] = v
		}
	}
	return tags
}

// errStalled is a slave whose count of a round's ballast stopped moving short
// of the round.
type errStalled struct{ msg string }

func (e errStalled) Error() string { return e.msg }

// catchUp waits for all of round's ballast on the slave, so the next round
// starts from a slave that has applied everything before it. It tells three
// things apart: a slave it has not read for NoReadFor (the run's failure), a
// slave whose count still moves (it waits), and a slave read at the same count
// short of the round for StallFor (the engine's result). T1 is one
// transaction, so the count is mostly 0 and then the round: StallFor is the
// time the slave gets to apply T1. A stop is only called on a read, so a slave
// that cannot be read is never taken for one that stopped.
func (r *Runner) catchUp(ctx context.Context, p string, round int) error {
	count := shellQuote(fmt.Sprintf("SELECT COUNT(*) FROM ballast_%s WHERE r = %d", p, round))
	script := fmt.Sprintf(`now=$(date +%%s); seen=$now; last=""; moved=$now
while :; do
  c=$(csql -u dba -t -c %s %s 2>/dev/null | awk '/^ *-?[0-9]+ *$/ { print $1; exit }')
  [ "$c" = "%d" ] && { echo caught=ok; exit 0; }
  now=$(date +%%s)
  if [ -n "$c" ]; then
    seen=$now
    [ "$c" != "$last" ] && { last=$c; moved=$now; }
    [ $(( now - moved )) -ge %d ] && { echo caught=stalled; echo "last=$c"; exit 0; }
  fi
  [ $(( now - seen )) -ge %d ] && { echo caught=noread; exit 0; }
  sleep 0.2
done
`, count, r.at(r.slave()), r.Rows, int(r.StallFor.Seconds()), int(r.NoReadFor.Seconds()))
	out, err := r.sh(ctx, script, r.StallFor+catchUpBound)
	if err != nil {
		return err
	}
	tags := parseTags(out)
	switch tags["caught"] {
	case "ok":
		return nil
	case "stalled":
		return errStalled{fmt.Sprintf("the slave's count of round %d's ballast stood at %s of %d for %s",
			round, tags["last"], r.Rows, r.StallFor)}
	case "noread":
		return fmt.Errorf("round %d: the client could not read the slave for %s", round, r.NoReadFor)
	}
	return fmt.Errorf("round %d: catch-up said %q", round, strings.TrimSpace(out))
}

// catchUpBound is how much longer than StallFor a slave whose count keeps
// moving may take over one round before the call itself gives up.
const catchUpBound = 2 * time.Hour

// Pattern runs one pattern's rounds and compares its tables at the end. An
// error is the run's own failure; what the engine did is in the outcome.
func (r *Runner) Pattern(ctx context.Context, p Pattern) (Outcome, error) {
	o := Outcome{Pattern: p.Name, Rounds: r.Rounds, Final: "same"}
	n := p.Name
	setup := []string{
		fmt.Sprintf("DROP TABLE IF EXISTS child_%[1]s; DROP TABLE IF EXISTS parent_%[1]s; DROP TABLE IF EXISTS sub_%[1]s; "+
			"DROP TABLE IF EXISTS t_%[1]s; DROP TABLE IF EXISTS w_%[1]s; DROP TABLE IF EXISTS ballast_%[1]s; "+
			"CREATE TABLE ballast_%[1]s (i INT PRIMARY KEY, r INT, pad VARCHAR(2000)); "+
			"CREATE INDEX ballast_%[1]s_r ON ballast_%[1]s (r); CREATE TABLE w_%[1]s (r INT PRIMARY KEY);", n),
		p.Setup(n),
	}
	for _, s := range setup {
		ref, err := r.tx(ctx, s)
		if err != nil {
			return o, err
		}
		if ref != "" {
			return o, fmt.Errorf("the master refused the setup: %s", ref)
		}
	}
	if err := r.barrier(ctx); err != nil {
		return o, err
	}
	fail0, err := r.failCounter(ctx)
	if err != nil {
		return o, err
	}
	for round := 1; round <= r.Rounds; round++ {
		rd := p.Round(n, round)
		if rd.Pre != "" {
			ref, err := r.tx(ctx, rd.Pre)
			if err != nil {
				return o, err
			}
			if ref != "" {
				r.logf("%s round %d: master refused the pre-write: %s", n, round, ref)
				o.Refused++
			}
			if err := r.barrier(ctx); err != nil {
				return o, err
			}
		}
		// Counted around T1 and T2 alone: the pre-write and the barrier are
		// DDL and would add a fallback and a clear of their own.
		w0, err := r.writeset(ctx)
		if err != nil {
			return o, err
		}
		out, err := r.sh(ctx, r.roundScript(n, round, rd.T1, rd.T2), 2*txTimeout+r.WitnessWait)
		if err != nil {
			return o, err
		}
		w1, err := r.writeset(ctx)
		if err != nil {
			return o, err
		}
		o.Writeset.add(w1.minus(w0))
		tags := parseTags(out)
		switch {
		case tags["t1"] == "refused":
			// csql leaves the transaction uncommitted and it is rolled back:
			// no ballast to wait for, and no T2 was sent.
			r.logf("%s round %d: master refused T1: %s", n, round, tags["t1err"])
			o.Refused++
			continue
		case tags["t1"] != "ok":
			return o, fmt.Errorf("round %d, T1: csql on the client: %s", round, firstNonEmpty(tags["t1"], strings.TrimSpace(out)))
		case tags["t2"] == "refused":
			r.logf("%s round %d: master refused T2: %s", n, round, tags["t2err"])
			o.Refused++
		case tags["t2"] != "ok":
			return o, fmt.Errorf("round %d, T2: csql on the client: %s", round, firstNonEmpty(tags["t2"], strings.TrimSpace(out)))
		case tags["got"] == "never":
			o.Never++
		case tags["got"] == "0":
			o.Ahead++
		}
		if err := r.catchUp(ctx, n, round); err != nil {
			if stalled, ok := err.(errStalled); ok {
				// The slave stopped applying: that is the engine's result, and the
				// pattern's row has to say it rather than vanish with the run.
				r.logf("%s round %d: %s", n, round, stalled.msg)
				o.Stalled = true
				o.Final = "stalled"
				o.Rounds = round
				if fail1, ferr := r.failCounter(ctx); ferr == nil {
					o.FailDelta = fail1 - fail0
				}
				r.dropBallast(ctx, n)
				return o, nil
			}
			return o, err
		}
	}
	if err := r.barrier(ctx); err != nil {
		return o, err
	}
	for _, t := range tablesOf(p) {
		m, err := r.dump(ctx, r.Pair.Master, "SELECT * FROM "+t+" ORDER BY 1")
		if err != nil {
			return o, err
		}
		s, err := r.dump(ctx, r.slave(), "SELECT * FROM "+t+" ORDER BY 1")
		if err != nil {
			return o, err
		}
		if m != s && o.Final == "same" {
			o.Final = "differ:" + t
		}
	}
	fail1, err := r.failCounter(ctx)
	if err != nil {
		return o, err
	}
	o.FailDelta = fail1 - fail0
	r.dropBallast(ctx, n)
	return o, nil
}

// dropBallast removes a pattern's ballast, which is the rounds of it on both
// nodes: a run of many patterns fills a disk if it stays. A failure here is
// said, and does not cost the pattern its outcome.
func (r *Runner) dropBallast(ctx context.Context, p string) {
	if ref, err := r.tx(ctx, "DROP TABLE ballast_"+p+";"); err != nil {
		r.logf("%s: the ballast table was not dropped: %v", p, err)
	} else if ref != "" {
		r.logf("%s: the ballast table was not dropped: %s", p, ref)
	}
}

func firstNonEmpty(a ...string) string {
	for _, s := range a {
		if s != "" {
			return s
		}
	}
	return ""
}
