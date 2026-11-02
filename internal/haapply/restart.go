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

	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
	"github.com/cubrid-systems/cubrid-testkit/internal/sandbox"
)

// The restart workload (ADR-EXT-012 3-2): a worker commits a transaction on
// its own connection before the coordinator stores the apply info, so an
// applier that dies in between meets, when it starts again, commits the slave
// already has. Applied a second time, an INSERT of a key that a later
// transaction moved brings the key back (#8052 72650ce0d). Each client inserts
// fresh keys of its own and moves each one in the next transaction, so the
// workers apply the clients in parallel, while the slave's applier is killed
// again and again, or the whole slave node crashed: then the server's restart
// recovery decides what the restarted applier finds, the marker rows among it.

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
// that key and its commit, iters times. With statements, both carry the
// USE_SBR hint, so the master replicates them as statements and the slave
// runs each again as SQL, rather than applying the rows they changed.
func restartSQL(c, iters int, statements bool) string {
	hint := ""
	if statements {
		hint = "/*+ USE_SBR */ "
	}
	var b strings.Builder
	for i := 1; i <= iters; i++ {
		k := c*restartStride + i
		fmt.Fprintf(&b, "INSERT %[1]sINTO r_kv VALUES (%[2]d, %[3]d, %[4]d);\nCOMMIT;\nUPDATE %[1]sr_kv SET id = %[5]d WHERE id = %[2]d;\nCOMMIT;\n",
			hint, k, c, i, k+restartMove)
	}
	return b.String()
}

// What a restart run kills: the slave's applier, which heartbeat starts
// again, or the whole slave node, crashed and started again.
const (
	KillApplier = "applier"
	KillNode    = "node"
)

// RestartConfig is one restart run.
type RestartConfig struct {
	Clients, Iters int
	// Kill is KillApplier or KillNode.
	Kill string
	// Statements makes the workload's DML replicate as statements.
	Statements bool
	// KillEvery is the time from the end of one kill to the next while the
	// clients run; Timeout bounds the clients.
	KillEvery, Timeout time.Duration
	// Host, Node and Save are as in StressConfig.
	Host, Node, Save string
}

// RestartOutcome is what a run found.
type RestartOutcome struct {
	Errors int // ERROR lines in the clients' logs: the master refusing
	Hung   bool
	// Kills is the kills that found an applier to kill: under KillNode the
	// crashes csb made of a slave whose applier ran. Backlog is those that
	// found, just before, markers above the slave's stored apply position, so
	// the applier that came up had commits to skip; -1 when the slave has no
	// marker table. A kill whose marker read failed counts in neither way.
	// MarkersPeak is the most markers one kill found.
	Kills, Backlog, MarkersPeak int
	// Down is a crashed slave node that did not start again; it ends the
	// kills and stops the clients.
	Down bool
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
	if c.Kill == "" {
		c.Kill = KillApplier
	}
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
		if err := os.WriteFile(filepath.Join(c.Host, fmt.Sprintf("c%d.sql", i)), []byte(restartSQL(i, c.Iters, c.Statements)), 0o644); err != nil {
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

	// A crashed node that does not start again fails the run whatever the
	// clients do, so it stops them too. A stop of the run kills the clients
	// (runClients) before it waits for a crash and its start under way.
	stop := make(chan struct{})
	cctx, stopClients := context.WithCancel(ctx)
	defer stopClients()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		r.killLoop(ctx, c, stop, &o)
		if o.Down {
			stopClients()
		}
	}()
	_, hung, errs, err := r.runClients(cctx, "restart", c.Host, clients{Node: c.Node, N: c.Clients, Timeout: c.Timeout, Poll: "  sleep 1"})
	stopped := cctx.Err() != nil && ctx.Err() == nil // by a slave that is down
	close(stop)
	wg.Wait()
	var down error
	if o.Down {
		down = fmt.Errorf("%s did not start again after a crash or an attempt at one; `csb node start %s` brings it back", r.slave(), r.slave())
	}
	if err != nil {
		if down != nil && stopped {
			r.logf("restart: the clients were stopped: %v", err)
			return o, down
		}
		return o, errors.Join(err, down)
	}
	if down != nil {
		return o, down
	}
	o.Hung, o.Errors = hung, errs

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

// killLoop kills until stop is closed, waiting c.KillEvery after each
// attempt, and reads the markers above the stored apply position just before
// each kill. A kill that could not be made is said and the loop goes on: a
// run with no kill is inconclusive. A crashed node that does not start again
// ends the kills, and Restart then stops the clients.
func (r *Runner) killLoop(ctx context.Context, c RestartConfig, stop <-chan struct{}, o *RestartOutcome) {
	ch, err := r.Pair.SlaveChannel(0)
	if err != nil {
		r.logf("restart: no channel to the slave, so no kill: %v", err)
		return
	}
	script := killScript(r.Pair.DB, o.Backlog >= 0, c.Kill)
	wait := time.NewTimer(c.KillEvery)
	defer wait.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		case <-wait.C:
		}
		select {
		case <-stop: // the clients ended as the wait did: no kill after them
			return
		default:
		}
		markers, killed, down := r.killOnce(ctx, ch, script, c.Kill)
		if killed {
			o.Kills++
			if o.Backlog >= 0 {
				if m, err := strconv.Atoi(markers); err != nil {
					r.logf("restart: the markers on %s could not be read before kill %d", r.slave(), o.Kills)
				} else if m > 0 {
					o.Backlog++
					o.MarkersPeak = max(o.MarkersPeak, m)
				}
			}
		}
		if down {
			o.Down = true
			return
		}
		wait.Reset(c.KillEvery)
	}
}

// killOnce reads the markers and kills: the applier, which heartbeat starts
// again, or the node, which it starts again itself. It says what the read
// found, whether there was an applier to kill, and whether a crashed node is
// down. A node whose applier is not up again is not crashed: what a crash
// asks is what an applier that starts again finds.
//
// A crash and its start run to their end once begun, even when the run is
// being stopped, so that no pair is left down: neither takes the run's
// cancel, and csb runs out of reach of a terminal's ^C (Pair.CrashSlave).
// Only a second interrupt during the crash ends testkit before the start,
// and it says so (secondInterrupt). A
// crash that failed may still have reached the node, and `csb node start`
// begins with a service stop, so the start follows it either way.
func (r *Runner) killOnce(ctx context.Context, ch exec.Channel, script, kill string) (markers string, killed, down bool) {
	res, err := ch.Run(ctx, script)
	if err != nil {
		r.logf("restart: a kill on %s was not made: %v", r.slave(), err)
		return "", false, false
	}
	tags := parseTags(res.Stdout)
	if kill == KillNode {
		switch tags["applier"] {
		case "0":
		case "1": // not up again yet since the last start
			return "", false, false
		default:
			r.logf("restart: pgrep on %s said %q: %s", r.slave(), tags["applier"], strings.TrimSpace(res.Stderr))
			return "", false, false
		}
		cycle := context.WithoutCancel(ctx)
		crashErr := r.Pair.CrashSlave(cycle, 0)
		if crashErr != nil {
			r.logf("restart: a crash of %s may not have been made: %v", r.slave(), crashErr)
		}
		if ctx.Err() != nil {
			r.logf("restart: stopping; %s is started again first (a second interrupt ends testkit now, saying what it leaves)", r.slave())
		}
		if err := r.Pair.StartSlave(cycle, 0); err != nil {
			r.logf("restart: %s did not start again after a crash or an attempt at one, so no more kills: %v", r.slave(), err)
			return tags["markers"], crashErr == nil, true
		}
		return tags["markers"], crashErr == nil, false
	}
	switch tags["pkill"] {
	case "0":
		return tags["markers"], true, false
	case "1": // heartbeat had not started it again yet
	default:
		r.logf("restart: pkill on %s said %q: %s", r.slave(), tags["pkill"], strings.TrimSpace(res.Stderr))
	}
	return "", false, false
}

// killScript runs on the slave: the marker read, then for KillApplier the
// kill of the applier, and for KillNode whether an applier runs to be
// crashed. The read is bounded, so a slave server that stops answering does
// not hold the kill. The bracket keeps pkill and pgrep from matching the
// shell that runs this script, whose command line holds the pattern.
func killScript(db string, markers bool, kill string) string {
	s := "m=-"
	if markers {
		s = fmt.Sprintf(`c=csql
if [ -x "$CUBRID/bin/csql" ]; then c="$CUBRID/bin/csql"; export LD_LIBRARY_PATH="$CUBRID/lib${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"; fi
m=$(timeout 10 $c -u dba -t -c %s %s 2>/dev/null | awk '/^ *[0-9]+ *$/ { print $1; exit }')`,
			shellQuote(backlogQuery), shellQuote(db+"@localhost"))
	}
	switch kill {
	case KillApplier:
		s += `
pkill -9 -f 'cub_admin [a]pplylogdb'
echo "pkill=$?"`
	case KillNode:
		s += `
pgrep -f 'cub_admin [a]pplylogdb' >/dev/null
echo "applier=$?"`
	}
	return s + `
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
		"master_rows", "slave_rows", "key_sum_master", "key_sum_slave", "unmoved_master", "unmoved_slave",
		"fail_counter_delta", "apply_670", "slave", "verdict"}, "\t"))
}

func (o RestartOutcome) fields(verdict string) []string {
	clients, slave := "done", "caught"
	if o.Hung {
		clients = "hung"
	}
	if o.Stalled {
		slave = "stalled"
	}
	known := func(n int) string {
		if n < 0 {
			return "-"
		}
		return strconv.Itoa(n)
	}
	return []string{strconv.Itoa(o.Errors), clients, strconv.Itoa(o.Kills), known(o.Backlog), strconv.Itoa(o.MarkersPeak),
		known(o.Restarts), strconv.Itoa(o.RowsMaster), strconv.Itoa(o.RowsSlave), o.SumMaster, o.SumSlave,
		strconv.Itoa(o.UnmovedMaster), strconv.Itoa(o.UnmovedSlave), strconv.Itoa(o.FailDelta), known(o.Dup670),
		slave, verdict}
}

func restartRow(w io.Writer, o RestartOutcome, verdict string) {
	fmt.Fprintln(w, strings.Join(o.fields(verdict), "\t"))
}

func restartLine(o RestartOutcome, verdict string) string {
	f := o.fields(verdict)
	return fmt.Sprintf("%s master_errors=%s clients=%s kills=%s kills_backlog=%s markers_peak=%s restarts=%s "+
		"rows(master=%s,slave=%s) key_sum(master=%s,slave=%s) unmoved(master=%s,slave=%s) fail_counter_delta=%s apply_670=%s slave=%s",
		verdict, f[0], f[1], f[2], f[3], f[4], f[5], f[6], f[7], f[8], f[9], f[10], f[11], f[12], f[13], f[14])
}
