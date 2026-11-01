package haapply

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/sandbox"
)

// The restart workload (ADR-EXT-012 3-2): a worker commits a transaction on
// its own connection before the coordinator stores the apply info, so an
// applier that dies in between meets, when it starts again, commits the slave
// already has. Applied a second time, an INSERT of a key that a later
// transaction moved brings the key back (#8052 72650ce0d). Each client inserts
// fresh keys of its own and moves each one in the next transaction, so the
// workers apply the clients in parallel, while the slave's applier is killed
// again and again.

const restartSetup = `DROP TABLE IF EXISTS r_kv;
CREATE TABLE r_kv (id INT PRIMARY KEY, c INT, i INT);`

// A client's key i is c×10^7 + i, moved to that plus restartMove; i stays
// under restartMove so that no moved key is another's fresh one.
const (
	restartStride = 10000000
	restartMove   = 5000000
)

// unmovedQuery counts the keys still where they were inserted: none on the
// master once every client has finished, and on the slave each is a key a
// second apply brought back.
var unmovedQuery = fmt.Sprintf("SELECT COUNT(*) FROM r_kv WHERE MOD(id, %d) < %d", restartStride, restartMove)

// markerTableQuery says whether the slave has the applied-tx marker table
// (#8052 72650ce0d and after).
const markerTableQuery = "SELECT COUNT(*) FROM db_class WHERE class_name = 'ha_applied_tx_marker'"

// backlogQuery counts the markers above the apply position the slave stored:
// the commits an applier starting now must skip. The coordinator deletes the
// markers at or below it when it stores it, but a delete that fails leaves
// them, and those are nothing to skip.
const backlogQuery = "SELECT COUNT(*) FROM ha_applied_tx_marker m, db_ha_apply_info a " +
	"WHERE m.copied_log_path = a.copied_log_path AND (m.commit_pageid > a.committed_lsa_pageid " +
	"OR (m.commit_pageid = a.committed_lsa_pageid AND m.commit_offset > a.committed_lsa_offset))"

// restartSQL is client c's script: an INSERT and its commit, then the move of
// that key and its commit, iters times.
func restartSQL(c, iters int) string {
	var b strings.Builder
	for i := 1; i <= iters; i++ {
		k := c*restartStride + i
		fmt.Fprintf(&b, "INSERT INTO r_kv VALUES (%d, %d, %d);\nCOMMIT;\nUPDATE r_kv SET id = %d WHERE id = %d;\nCOMMIT;\n",
			k, c, i, k+restartMove, k)
	}
	return b.String()
}

// RestartConfig is one restart run.
type RestartConfig struct {
	Clients, Iters int
	// KillEvery is the time between two kills of the slave's applier while the
	// clients run; Timeout bounds the clients.
	KillEvery, Timeout time.Duration
	// Host, Node and Save are as in StressConfig.
	Host, Node, Save string
}

// RestartOutcome is what a run found.
type RestartOutcome struct {
	Errors int // ERROR lines in the clients' logs: the master refusing
	Hung   bool
	// Kills is the kills that found an applier to kill. Backlog is those that
	// found, just before, markers above the slave's stored apply position, so
	// the applier that came up had commits to skip; -1 when the slave has no
	// marker table. A kill whose marker read failed counts in neither way.
	// MarkersPeak is the most markers one kill found.
	Kills, Backlog, MarkersPeak int
	// Restarts is how many times the slave's applier log says it started, -1
	// when the log lost lines during the run.
	Restarts int
	Stalled  bool
	// The table on each node: its rows, the sum of its keys, and the keys not
	// moved.
	RowsMaster, RowsSlave       int
	SumMaster, SumSlave         string
	UnmovedMaster, UnmovedSlave int
	FailDelta                   int
	// Dup670 is the -670s the slave's applier log gained: an INSERT or a key
	// change that met a key the slave already had. Where the log lost lines it
	// is the gain if the count still grew, a floor since lines are only lost,
	// and -1 otherwise.
	Dup670 int
}

// JudgeRestart is a run's verdict. Missed is a second apply seen: the applier
// failing, a -670 in its log, or a caught-up slave whose table is not the
// master's. A stalled slave that did not fail is behind, which says nothing.
// Held needs a kill whose applier had commits to skip, so a slave with no
// marker table is never held, and a log that lost lines cannot show that no
// -670 came, so neither is that run: fail_counter alone can miss one, since
// a kill can come before the coordinator stores it.
func JudgeRestart(o RestartOutcome) string {
	failed := o.FailDelta != 0 || o.Dup670 > 0
	differ := o.RowsMaster != o.RowsSlave || o.SumMaster != o.SumSlave || o.UnmovedMaster != o.UnmovedSlave
	switch {
	case failed || (!o.Stalled && differ):
		return Missed
	case o.Stalled || o.Kills == 0 || o.Backlog < 1 || o.Dup670 < 0:
		return Inconclusive
	}
	return Held
}

// Restart runs the workload with the kills and compares the pair.
func (r *Runner) Restart(ctx context.Context, c RestartConfig) (RestartOutcome, error) {
	var o RestartOutcome
	defer r.keepFiles(c.Host, c.Save, "restart")
	if err := os.RemoveAll(c.Host); err != nil {
		return o, err
	}
	if err := os.MkdirAll(c.Host, 0o755); err != nil {
		return o, err
	}
	ref, err := r.tx(ctx, restartSetup)
	if err != nil {
		return o, err
	}
	if ref != "" {
		return o, fmt.Errorf("the master refused the setup: %s", ref)
	}
	if err := r.barrier(ctx); err != nil {
		return o, err
	}
	for i := 1; i <= c.Clients; i++ {
		if err := os.WriteFile(filepath.Join(c.Host, fmt.Sprintf("c%d.sql", i)), []byte(restartSQL(i, c.Iters)), 0o644); err != nil {
			return o, err
		}
	}
	markers, err := r.count(ctx, r.slave(), markerTableQuery)
	if err != nil {
		return o, err
	}
	if markers == 0 {
		o.Backlog = -1
	} else if err := r.slaveHasTimeout(ctx); err != nil {
		return o, err
	}
	fail0, err := r.failCounter(ctx)
	if err != nil {
		return o, err
	}
	log0, err := r.applierLog(ctx)
	if err != nil {
		return o, err
	}
	if log0.started == 0 {
		return o, fmt.Errorf("no applier log under $CUBRID/log on %s says the applier started", r.slave())
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		r.killLoop(ctx, c.KillEvery, stop, &o)
	}()
	out, err := r.sh(ctx, r.restartClientsScript(c), c.Timeout+5*time.Minute)
	close(stop)
	wg.Wait()
	if err != nil {
		if _, kerr := r.Client.Run(context.Background(), "kill -9 $(cat "+shellQuote(c.Node+"/pids")+") 2>/dev/null; true"); kerr != nil {
			r.logf("restart: the clients may still be running on %s: %v", r.Client.Name, kerr)
		}
		return o, err
	}
	switch tags := parseTags(out); tags["clients"] {
	case "done":
	case "hung":
		r.logf("restart: the clients had not finished after %s and were stopped", c.Timeout)
		o.Hung = true
	default:
		return o, fmt.Errorf("the clients' script said %q", strings.TrimSpace(out))
	}
	for i := 1; i <= c.Clients; i++ {
		b, err := os.ReadFile(filepath.Join(c.Host, fmt.Sprintf("c%d.log", i)))
		if err != nil {
			return o, err
		}
		if !o.Hung && !strings.Contains(string(b), "Execute OK.") {
			return o, fmt.Errorf("client %d ended no transaction: %s", i, firstNonEmpty(firstLine(string(b)), "an empty log"))
		}
		o.Errors += strings.Count(string(b), "ERROR:")
	}

	barrierErr := r.barrier(ctx)
	switch {
	case barrierErr == nil:
	case errors.Is(barrierErr, sandbox.ErrNotReplicated):
		r.logf("restart: the slave did not apply up to the marker: %v", barrierErr)
		o.Stalled = true
	default:
		return o, barrierErr
	}
	if err := r.compareRestart(ctx, &o); err != nil {
		if barrierErr != nil {
			return o, fmt.Errorf("%w; then %v", barrierErr, err)
		}
		return o, err
	}
	fail1, err := r.failCounter(ctx)
	if err != nil {
		return o, err
	}
	o.FailDelta = fail1 - fail0
	log1, err := r.applierLog(ctx)
	if err != nil {
		return o, err
	}
	o.Restarts, o.Dup670 = log1.started-log0.started, log1.dup670-log0.dup670
	if log1.lostSince(log0) {
		r.logf("restart: the applier log on %s lost lines during the run (rotated): starts %d to %d, -670s %d to %d",
			r.slave(), log0.started, log1.started, log0.dup670, log1.dup670)
		o.Restarts = -1
		if o.Dup670 <= 0 {
			o.Dup670 = -1
		}
	}
	return o, nil
}

// slaveHasTimeout fails the run where the kill script's bound on the marker
// read is missing: every read would fail and leave the run inconclusive.
func (r *Runner) slaveHasTimeout(ctx context.Context) error {
	ch, err := r.Pair.SlaveChannel(0)
	if err != nil {
		return err
	}
	res, err := ch.Run(ctx, "timeout 1 true 2>/dev/null && echo timeout=yes")
	if err != nil {
		return err
	}
	if parseTags(res.Stdout)["timeout"] != "yes" {
		return fmt.Errorf("no timeout(1) on %s to bound the marker reads", r.slave())
	}
	return nil
}

// killLoop kills the slave's applier every d until stop is closed, reading
// the markers above the stored apply position just before each kill.
// Heartbeat starts the applier again. A kill that could not be made is said
// and the loop goes on: the clients run to their end either way, and a run
// with no kill is inconclusive.
func (r *Runner) killLoop(ctx context.Context, d time.Duration, stop <-chan struct{}, o *RestartOutcome) {
	ch, err := r.Pair.SlaveChannel(0)
	if err != nil {
		r.logf("restart: no channel to the slave, so no kill: %v", err)
		return
	}
	script := killScript(r.Pair.DB, o.Backlog >= 0)
	tick := time.NewTicker(d)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		res, err := ch.Run(ctx, script)
		if err != nil {
			r.logf("restart: a kill on %s was not made: %v", r.slave(), err)
			continue
		}
		tags := parseTags(res.Stdout)
		switch tags["pkill"] {
		case "0":
		case "1":
			continue // heartbeat had not started it again yet
		default:
			r.logf("restart: pkill on %s said %q: %s", r.slave(), tags["pkill"], strings.TrimSpace(res.Stderr))
			continue
		}
		o.Kills++
		if o.Backlog < 0 {
			continue
		}
		m, err := strconv.Atoi(tags["markers"])
		if err != nil {
			r.logf("restart: the markers on %s could not be read before kill %d", r.slave(), o.Kills)
			continue
		}
		if m > 0 {
			o.Backlog++
			o.MarkersPeak = max(o.MarkersPeak, m)
		}
	}
}

// killScript runs on the slave. The marker read is bounded, so a slave server
// that stops answering does not hold the kill. The bracket keeps pkill from
// matching the shell that runs this script, whose command line holds the
// pattern.
func killScript(db string, markers bool) string {
	read := "m=-"
	if markers {
		read = fmt.Sprintf(`c=csql
if [ -x "$CUBRID/bin/csql" ]; then c="$CUBRID/bin/csql"; export LD_LIBRARY_PATH="$CUBRID/lib${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"; fi
m=$(timeout 10 $c -u dba -t -c %s %s 2>/dev/null | awk '/^ *[0-9]+ *$/ { print $1; exit }')`,
			shellQuote(backlogQuery), shellQuote(db+"@localhost"))
	}
	return read + `
pkill -9 -f 'cub_admin [a]pplylogdb'
echo "pkill=$?"
echo "markers=${m:--}"
`
}

type applierLogCounts struct{ started, dup670 int }

// lostSince says whether the log lost lines after a was read: a count that
// went down. A rotation (er_file_backup, past error_log_size, 512 MB by
// default) unlinks the .bak, so one that drops no more start lines and no
// more -670 lines than the run adds is not seen; only as many -670 lines lost
// as added can make such a run held. The inodes cannot tell either, since the
// new log can take the one the .bak freed.
func (b applierLogCounts) lostSince(a applierLogCounts) bool {
	return b.started < a.started || b.dup670 < a.dup670
}

// applierLog counts, in the slave's applier logs and the copy a rotation
// keeps, the starts and the -670s.
func (r *Runner) applierLog(ctx context.Context) (applierLogCounts, error) {
	ch, err := r.Pair.SlaveChannel(0)
	if err != nil {
		return applierLogCounts{}, err
	}
	res, err := ch.Run(ctx, `l=$(cat "$CUBRID"/log/*_applylogdb_*.err* 2>/dev/null)
echo "started=$(printf '%s\n' "$l" | grep -c 'log applier started')"
echo "dup670=$(printf '%s\n' "$l" | grep -c -- '-670')"`)
	if err != nil {
		return applierLogCounts{}, fmt.Errorf("reading the applier log on %s: %w", r.slave(), err)
	}
	tags := parseTags(res.Stdout)
	started, err1 := strconv.Atoi(tags["started"])
	dup, err2 := strconv.Atoi(tags["dup670"])
	if err1 != nil || err2 != nil {
		return applierLogCounts{}, fmt.Errorf("the applier log on %s read as %q", r.slave(), strings.TrimSpace(res.Stdout))
	}
	return applierLogCounts{started, dup}, nil
}

// restartClientsScript starts every client's csql on the master at once and
// waits for them, stopping them past the timeout.
func (r *Runner) restartClientsScript(c RestartConfig) string {
	return fmt.Sprintf(`cd %[1]s || exit 1
pids=""
i=1
while [ $i -le %[2]d ]; do
  csql -u dba --no-auto-commit -i c$i.sql %[3]s > c$i.log 2>&1 &
  pids="$pids $!"
  i=$((i + 1))
done
echo "$pids" > pids
alive() { for p in $pids; do kill -0 $p 2>/dev/null && return 0; done; return 1; }
end=$(( $(date +%%s) + %[4]d ))
while alive; do
  if [ $(date +%%s) -ge $end ]; then kill -9 $pids 2>/dev/null; wait; echo clients=hung; exit 0; fi
  sleep 1
done
wait
echo clients=done
`, shellQuote(c.Node), c.Clients, r.at(r.Pair.Master), int(c.Timeout.Seconds()))
}

// compareRestart reads the table on both nodes: its rows, the sum of its keys
// and the keys not moved.
func (r *Runner) compareRestart(ctx context.Context, o *RestartOutcome) error {
	m, s := r.Pair.Master, r.slave()
	const rows = "SELECT COUNT(*) FROM r_kv"
	const sum = "SELECT NVL(SUM(CAST(id AS BIGINT)), 0) FROM r_kv"
	var err error
	if o.RowsMaster, err = r.count(ctx, m, rows); err != nil {
		return err
	}
	if o.RowsSlave, err = r.count(ctx, s, rows); err != nil {
		return err
	}
	if o.SumMaster, err = r.q(ctx, m, sum); err != nil {
		return err
	}
	if o.SumSlave, err = r.q(ctx, s, sum); err != nil {
		return err
	}
	if o.UnmovedMaster, err = r.count(ctx, m, unmovedQuery); err != nil {
		return err
	}
	o.UnmovedSlave, err = r.count(ctx, s, unmovedQuery)
	return err
}

// The restart ledger: the run's identity, a header, and one row. backlog is
// "-" on a slave with no marker table, restarts and apply_670 where the
// applier log lost lines.
func restartHead(w io.Writer, meta []string) {
	fmt.Fprintf(w, "# %s\n", strings.Join(meta, " "))
	fmt.Fprintln(w, strings.Join([]string{"master_errors", "clients", "kills", "kills_backlog", "markers_peak", "restarts",
		"master_rows", "slave_rows", "key_sum", "unmoved_master", "unmoved_slave", "fail_counter_delta", "apply_670",
		"slave", "verdict"}, "\t"))
}

func (o RestartOutcome) fields(verdict string) []string {
	clients, slave, sum := "done", "caught", "same"
	if o.Hung {
		clients = "hung"
	}
	if o.Stalled {
		slave = "stalled"
	}
	if o.SumMaster != o.SumSlave {
		sum = "differ"
	}
	known := func(n int) string {
		if n < 0 {
			return "-"
		}
		return strconv.Itoa(n)
	}
	return []string{strconv.Itoa(o.Errors), clients, strconv.Itoa(o.Kills), known(o.Backlog), strconv.Itoa(o.MarkersPeak),
		known(o.Restarts), strconv.Itoa(o.RowsMaster), strconv.Itoa(o.RowsSlave), sum,
		strconv.Itoa(o.UnmovedMaster), strconv.Itoa(o.UnmovedSlave), strconv.Itoa(o.FailDelta), known(o.Dup670),
		slave, verdict}
}

func restartRow(w io.Writer, o RestartOutcome, verdict string) {
	fmt.Fprintln(w, strings.Join(o.fields(verdict), "\t"))
}

func restartLine(o RestartOutcome, verdict string) string {
	f := o.fields(verdict)
	return fmt.Sprintf("%s master_errors=%s clients=%s kills=%s kills_backlog=%s markers_peak=%s restarts=%s "+
		"rows(master=%s,slave=%s) key_sum=%s unmoved(master=%s,slave=%s) fail_counter_delta=%s apply_670=%s slave=%s",
		verdict, f[0], f[1], f[2], f[3], f[4], f[5], f[6], f[7], f[8], f[9], f[10], f[11], f[12], f[13])
}
