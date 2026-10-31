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
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/sandbox"
)

// The stress workload (ADR-EXT-012 3-2): several clients on one small key
// space, each running a script its seed names, and at the end the question
// whether the slave holds what the master holds. Many statements fail on the
// master -- duplicate keys, missing parents, deadlocks -- and that is fine:
// what the master ends up holding is the answer.

// stressTables are the tables the workload writes, each with the column a
// primary-key read orders by.
var stressTables = []struct{ name, key string }{{"s_kv", "id"}, {"s_parent", "p"}, {"s_child", "c"}}

const stressSetup = `DROP TABLE IF EXISTS s_child; DROP TABLE IF EXISTS s_parent; DROP TABLE IF EXISTS s_kv;
DROP TABLE IF EXISTS s_ballast;
CREATE TABLE s_kv (id INT PRIMARY KEY, uk INT UNIQUE, v INT);
CREATE TABLE s_parent (p INT PRIMARY KEY, v INT);
CREATE TABLE s_child (c INT PRIMARY KEY, p INT, v INT,
  CONSTRAINT fk_s FOREIGN KEY (p) REFERENCES s_parent (p) ON DELETE CASCADE);
CREATE TABLE s_ballast (i INT PRIMARY KEY, c INT, pad VARCHAR(1000));`

// orphanQuery counts children whose parent is gone. One on a node breaks the
// foreign key on that node itself, and says a divergence began on the master
// rather than in replication.
const orphanQuery = "SELECT COUNT(*) FROM s_child c WHERE NOT EXISTS (SELECT 1 FROM s_parent p WHERE p.p = c.p)"

// stressSQL is client c's script for seed: the shell prototype's generator
// (evidence/ha/parallel-apply-stress.sh) statement for statement, so a seed
// names the same SQL in both. The sequence is Park–Miller, exact in the
// prototype's doubles and in these integers alike.
func stressSQL(seed, c, txns, keys, ballast int, ops []int) string {
	st := int64((seed*1000+c)%2147483646 + 1)
	rnd := func() float64 {
		st = st * 16807 % 2147483647
		return float64(st) / 2147483647
	}
	pick := func(n int) int { return int(rnd() * float64(n)) }
	var b strings.Builder
	for t := 1; t <= txns; t++ {
		if rnd() < 0.3 {
			fmt.Fprintf(&b, "INSERT INTO s_ballast SELECT %d + ROWNUM, %d, REPEAT('b', 900) FROM db_root CONNECT BY LEVEL <= %d;\n",
				c*100000000+t*10000, c, ballast)
		}
		n := 1 + pick(3)
		for i := 0; i < n; i++ {
			k := 1 + pick(keys)
			u := 1 + pick(keys)
			k2 := 1 + pick(keys)
			switch ops[pick(len(ops))] {
			case 0:
				fmt.Fprintf(&b, "UPDATE s_kv SET v = v + 1 WHERE id = %d;\n", k)
			case 1:
				fmt.Fprintf(&b, "UPDATE s_kv SET uk = %d WHERE id = %d;\n", u, k)
			case 2:
				fmt.Fprintf(&b, "DELETE FROM s_kv WHERE id = %d;\n", k)
			case 3:
				fmt.Fprintf(&b, "INSERT INTO s_kv VALUES (%d, %d, 0);\n", k, u)
			case 4:
				fmt.Fprintf(&b, "UPDATE s_kv SET id = %d WHERE id = %d;\n", k2, k)
			case 5:
				fmt.Fprintf(&b, "INSERT INTO s_parent VALUES (%d, 0);\n", k)
			case 6:
				fmt.Fprintf(&b, "DELETE FROM s_parent WHERE p = %d;\n", k)
			case 7:
				fmt.Fprintf(&b, "INSERT INTO s_child VALUES (%d, %d, 0);\n", k, u)
			case 8:
				fmt.Fprintf(&b, "UPDATE s_child SET p = %d WHERE c = %d;\n", u, k)
			default:
				fmt.Fprintf(&b, "UPDATE s_parent SET v = v + 1 WHERE p = %d;\n", k)
			}
		}
		if rnd() < 0.05 {
			b.WriteString("ROLLBACK;\n")
		} else {
			b.WriteString("COMMIT;\n")
		}
	}
	return b.String()
}

// StressConfig is one stress run.
type StressConfig struct {
	Clients, Txns, Keys, Ballast, Seed int
	Ops                                []int
	// Timeout bounds the clients; past it they are stopped and the run is
	// hung.
	Timeout time.Duration
	// Host is the directory the scripts and logs go to on this host, and Node
	// the same directory as the client node sees it. Save is where a run's
	// evidence is kept beside its ledger: the scripts and logs, copied from
	// Host, and both nodes' reads of any table that differs.
	Host, Node, Save string
}

// TableCheck is one table at the end of a run: whether the two nodes read it
// alike through its primary key, and its rows on each node counted through
// the heap and through the primary key.
type TableCheck struct {
	Name                                     string
	Same                                     bool
	HeapMaster, HeapSlave, PKMaster, PKSlave int
}

// StressOutcome is what a run found. The fields up to Hung are the run's own
// and are zero for --compare-only.
type StressOutcome struct {
	Seed      int
	Ran       bool
	Errors    int // ERROR lines in the clients' logs: the master refusing
	FailDelta int
	Writeset
	OrphanPolls, OrphanPeak int // polls of the master that saw an orphan while the clients ran
	Hung                    bool

	// Stalled is a slave that did not apply up to the marker in BarrierFor;
	// the comparison is then of a slave behind the master.
	Stalled                     bool
	Tables                      []TableCheck
	BallastSame                 bool
	OrphansMaster, OrphansSlave int
}

// The verdicts of a stress run (ADR-EXT-012 3-3), in the order they win.
const (
	StressDamaged  = "damaged"
	StressStalled  = "stalled"
	StressDiverged = "diverged"
	StressHung     = "hung"
	StressSame     = "same"
)

// JudgeStress is a run's verdict. Damaged is a node whose heap holds rows its
// primary key does not; a slave still applying is read in two statements that
// can see two states, so a stalled slave's counts do not make it. Stalled
// comes before diverged because a slave that is behind differs for that alone.
// Diverged is the slave not holding what the master holds.
func JudgeStress(o StressOutcome) string {
	diverged := !o.BallastSame || o.FailDelta != 0 || o.OrphansMaster != o.OrphansSlave
	for _, t := range o.Tables {
		if t.HeapMaster != t.PKMaster || (!o.Stalled && t.HeapSlave != t.PKSlave) {
			return StressDamaged
		}
		diverged = diverged || !t.Same || t.HeapMaster != t.HeapSlave
	}
	switch {
	case o.Stalled:
		return StressStalled
	case diverged:
		return StressDiverged
	case o.Hung:
		return StressHung
	}
	return StressSame
}

// Stress runs the workload and compares the pair, or with compareOnly only
// compares the tables an earlier run left. An error is the run's own failure.
func (r *Runner) Stress(ctx context.Context, c StressConfig, compareOnly bool) (StressOutcome, error) {
	o := StressOutcome{Seed: c.Seed, Ran: !compareOnly}
	var fail0 int
	if !compareOnly {
		// Kept whatever happens below: the seed names the SQL but not how the
		// clients interleaved, so a run is not looked at twice.
		defer r.keepFiles(c)
		var err error
		if fail0, err = r.runStress(ctx, c, &o); err != nil {
			return o, err
		}
	}
	barrierErr := r.barrier(ctx)
	switch {
	case barrierErr == nil:
	case errors.Is(barrierErr, sandbox.ErrNotReplicated):
		// A finding if the slave can still be read; if it cannot, the
		// comparison fails and the run with it.
		r.logf("stress: the slave did not apply up to the marker: %v", barrierErr)
		o.Stalled = true
	default:
		return o, barrierErr
	}
	if err := r.compareStress(ctx, c, &o); err != nil {
		if barrierErr != nil {
			return o, fmt.Errorf("%w; then %v", barrierErr, err)
		}
		return o, err
	}
	if !compareOnly {
		fail1, err := r.failCounter(ctx)
		if err != nil {
			return o, err
		}
		o.FailDelta = fail1 - fail0
	}
	return o, nil
}

// runStress makes the tables, writes the clients' scripts where the client
// node reads them, and runs the clients at once. It returns fail_counter as
// it was before them.
func (r *Runner) runStress(ctx context.Context, c StressConfig, o *StressOutcome) (int, error) {
	// The directory is this seed's, so what an earlier run of it left goes
	// first, before anything that can fail would have it kept as this run's.
	if err := os.RemoveAll(c.Host); err != nil {
		return 0, err
	}
	if err := os.MkdirAll(c.Host, 0o755); err != nil {
		return 0, err
	}
	ref, err := r.tx(ctx, stressSetup)
	if err != nil {
		return 0, err
	}
	if ref != "" {
		return 0, fmt.Errorf("the master refused the setup: %s", ref)
	}
	if err := r.barrier(ctx); err != nil {
		return 0, err
	}
	for i := 1; i <= c.Clients; i++ {
		sql := stressSQL(c.Seed, i, c.Txns, c.Keys, c.Ballast, c.Ops)
		if err := os.WriteFile(filepath.Join(c.Host, fmt.Sprintf("c%d.sql", i)), []byte(sql), 0o644); err != nil {
			return 0, err
		}
	}
	fail0, err := r.failCounter(ctx)
	if err != nil {
		return 0, err
	}
	w0, err := r.writeset(ctx)
	if err != nil {
		return 0, err
	}
	out, err := r.sh(ctx, r.clientsScript(c), c.Timeout+5*time.Minute)
	if err != nil {
		// The script did not end its clients: an interrupt, or csb's own
		// bound. Clients left writing would be in the next run's tables.
		if _, kerr := r.Client.Run(context.Background(), "kill -9 $(cat "+shellQuote(c.Node+"/pids")+") 2>/dev/null; true"); kerr != nil {
			r.logf("stress: the clients may still be running on %s: %v", r.Client.Name, kerr)
		}
		return 0, err
	}
	tags := parseTags(out)
	switch tags["clients"] {
	case "done":
	case "hung":
		r.logf("stress: the clients had not finished after %s and were stopped", c.Timeout)
		o.Hung = true
	default:
		return 0, fmt.Errorf("the clients' script said %q", strings.TrimSpace(out))
	}
	o.OrphanPolls, _ = strconv.Atoi(tags["orphan_polls"])
	o.OrphanPeak, _ = strconv.Atoi(tags["orphan_peak"])
	// Counted before the barrier, whose DDL would add a fallback and a clear.
	w1, err := r.writeset(ctx)
	if err != nil {
		return 0, err
	}
	o.Writeset = w1.minus(w0)
	for i := 1; i <= c.Clients; i++ {
		b, err := os.ReadFile(filepath.Join(c.Host, fmt.Sprintf("c%d.log", i)))
		if err != nil {
			return 0, err
		}
		// csql says "Execute OK." for each COMMIT and ROLLBACK. A client that
		// finished without one ran nothing -- it could not connect, say -- and
		// a pair that was not written to would read the same.
		if !o.Hung && !strings.Contains(string(b), "Execute OK.") {
			return 0, fmt.Errorf("client %d ended no transaction: %s", i, firstNonEmpty(firstLine(string(b)), "an empty log"))
		}
		o.Errors += strings.Count(string(b), "ERROR:")
	}
	return fail0, nil
}

func firstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}

// keepFiles copies the clients' scripts and logs beside the ledger. A pair
// this run stood up takes its work directory with it.
func (r *Runner) keepFiles(c StressConfig) {
	entries, err := os.ReadDir(c.Host)
	if err == nil {
		err = os.MkdirAll(c.Save, 0o755)
	}
	if err != nil {
		r.logf("stress: the clients' files were not kept: %v", err)
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(c.Host, e.Name()))
		if err == nil {
			err = os.WriteFile(filepath.Join(c.Save, e.Name()), b, 0o644)
		}
		if err != nil {
			r.logf("stress: %s was not kept: %v", e.Name(), err)
		}
	}
}

// clientsScript starts every client's csql on the master at once and, until
// they end, counts the master's orphans: one a later statement cleaned up is
// gone from the final state but not from what happened. Past the timeout it
// stops them; a poll is bounded too, so a master that stops answering does
// not hold the script past it. The pids file is for a run that ends without
// the script.
func (r *Runner) clientsScript(c StressConfig) string {
	return fmt.Sprintf(`cd %[1]s || exit 1
command -v timeout >/dev/null || { echo "no timeout(1) on the client node to bound the polls" >&2; exit 1; }
pids=""
i=1
while [ $i -le %[2]d ]; do
  csql -u dba --no-auto-commit -i c$i.sql %[3]s > c$i.log 2>&1 &
  pids="$pids $!"
  i=$((i + 1))
done
echo "$pids" > pids
alive() { for p in $pids; do kill -0 $p 2>/dev/null && return 0; done; return 1; }
end=$(( $(date +%%s) + %[4]d )); seen=0; peak=0
while alive; do
  if [ $(date +%%s) -ge $end ]; then
    kill -9 $pids 2>/dev/null; wait
    echo clients=hung; echo "orphan_polls=$seen"; echo "orphan_peak=$peak"; exit 0
  fi
  o=$(timeout 30 csql -u dba -t -c %[5]s %[3]s 2>/dev/null | awk '/^ *-?[0-9]+ *$/ { print $1; exit }')
  if [ -n "$o" ] && [ "$o" -gt 0 ]; then seen=$((seen + 1)); [ "$o" -gt "$peak" ] && peak=$o; fi
done
wait
echo clients=done; echo "orphan_polls=$seen"; echo "orphan_peak=$peak"
`, shellQuote(c.Node), c.Clients, r.at(r.Pair.Master), int(c.Timeout.Seconds()), shellQuote(orphanQuery))
}

// compareStress reads the tables on both nodes. A primary-key read passes a
// row the heap has and no index has (an insert that lost its index entries),
// so each table's heap is also counted, through an expression no index covers,
// and its primary key on its own.
func (r *Runner) compareStress(ctx context.Context, c StressConfig, o *StressOutcome) error {
	m, s := r.Pair.Master, r.slave()
	for _, t := range stressTables {
		tc := TableCheck{Name: t.name}
		same, err := r.sameOnBoth(ctx, c, t.name, "SELECT * FROM "+t.name+" ORDER BY 1")
		if err != nil {
			return err
		}
		tc.Same = same
		heap := "SELECT COUNT(*) FROM " + t.name + " WHERE NVL(v, 0) = NVL(v, 0)"
		if tc.HeapMaster, err = r.count(ctx, m, heap); err != nil {
			return err
		}
		if tc.HeapSlave, err = r.count(ctx, s, heap); err != nil {
			return err
		}
		pk := "SELECT " + t.key + " FROM " + t.name + " ORDER BY 1"
		if tc.PKMaster, err = r.rows(ctx, m, pk); err != nil {
			return err
		}
		if tc.PKSlave, err = r.rows(ctx, s, pk); err != nil {
			return err
		}
		o.Tables = append(o.Tables, tc)
	}
	same, err := r.sameOnBoth(ctx, c, "s_ballast", "SELECT c, COUNT(*) FROM s_ballast GROUP BY c ORDER BY c")
	if err != nil {
		return err
	}
	o.BallastSame = same
	if o.OrphansMaster, err = r.count(ctx, m, orphanQuery); err != nil {
		return err
	}
	o.OrphansSlave, err = r.count(ctx, s, orphanQuery)
	return err
}

// sameOnBoth reads sql on both nodes and, when they differ, keeps both reads
// as <name>.master and <name>.slave beside the ledger.
func (r *Runner) sameOnBoth(ctx context.Context, c StressConfig, name, sql string) (bool, error) {
	dm, err := r.dump(ctx, r.Pair.Master, sql)
	if err != nil {
		return false, err
	}
	ds, err := r.dump(ctx, r.slave(), sql)
	if err != nil {
		return false, err
	}
	if dm == ds {
		return true, nil
	}
	for node, d := range map[string]string{"master": dm, "slave": ds} {
		err := os.MkdirAll(c.Save, 0o755)
		if err == nil {
			err = os.WriteFile(filepath.Join(c.Save, name+"."+node), []byte(d), 0o644)
		}
		if err != nil {
			r.logf("stress: %s's read on the %s was not kept: %v", name, node, err)
		}
	}
	return false, nil
}

func (r *Runner) count(ctx context.Context, host, sql string) (int, error) {
	v, err := r.q(ctx, host, sql)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(v)
}

// rows is how many rows a one-column read of numbers returned.
func (r *Runner) rows(ctx context.Context, host, sql string) (int, error) {
	out, err := r.sh(ctx, "csql -u dba -t -c "+shellQuote(sql)+" "+r.at(host)+" 2>&1", 2*time.Minute)
	if err != nil {
		return 0, err
	}
	if e := firstError(out); e != "" {
		return 0, fmt.Errorf("%s on %s: %s", sql, host, e)
	}
	n := 0
	for _, l := range strings.Split(out, "\n") {
		if numberLine.MatchString(l) {
			n++
		}
	}
	return n, nil
}

// The stress ledger (ADR-EXT-012 3-6): the run's identity, a header, and one
// row; a field a --compare-only run has no value for is "-".
func stressHead(w io.Writer, meta []string) {
	fmt.Fprintf(w, "# %s\n", strings.Join(meta, " "))
	cols := []string{"seed", "master_errors", "conflict_write", "conflict_ref", "fallback", "clear", "fail_counter_delta",
		"orphan_polls", "orphan_peak", "clients"}
	for _, t := range stressTables {
		cols = append(cols, t.name)
	}
	for _, t := range stressTables {
		cols = append(cols, "heap_"+t.name)
	}
	cols = append(cols, "s_ballast", "orphans_master", "orphans_slave", "slave", "verdict")
	fmt.Fprintln(w, strings.Join(cols, "\t"))
}

// counts is a table's rows as heap_<table> has them: through the heap on the
// master and the slave, then through the primary key on each.
func (t TableCheck) counts() string {
	return fmt.Sprintf("%d/%d/%d/%d", t.HeapMaster, t.HeapSlave, t.PKMaster, t.PKSlave)
}

func (o StressOutcome) slave() string {
	if o.Stalled {
		return "stalled"
	}
	return "caught"
}

func stressRow(w io.Writer, o StressOutcome, verdict string) {
	run := []string{"-", "-", "-", "-", "-", "-", "-", "-", "-"}
	if o.Ran {
		clients := "done"
		if o.Hung {
			clients = "hung"
		}
		run = []string{strconv.Itoa(o.Errors), strconv.Itoa(o.Write), strconv.Itoa(o.Ref), strconv.Itoa(o.Fallback),
			strconv.Itoa(o.Clear), strconv.Itoa(o.FailDelta), strconv.Itoa(o.OrphanPolls), strconv.Itoa(o.OrphanPeak), clients}
	}
	seed := "-"
	if o.Ran {
		seed = strconv.Itoa(o.Seed)
	}
	f := append([]string{seed}, run...)
	for _, t := range o.Tables {
		f = append(f, sameOrDiffer(t.Same))
	}
	for _, t := range o.Tables {
		f = append(f, t.counts())
	}
	f = append(f, sameOrDiffer(o.BallastSame), strconv.Itoa(o.OrphansMaster), strconv.Itoa(o.OrphansSlave), o.slave(), verdict)
	fmt.Fprintln(w, strings.Join(f, "\t"))
}

// stressLine is the run on one line, as the shell prototype printed it.
func stressLine(o StressOutcome, verdict string) string {
	var b strings.Builder
	b.WriteString(verdict)
	if o.Ran {
		fmt.Fprintf(&b, " seed=%d master_errors=%d conflict_write=%d conflict_ref=%d fallback=%d clear=%d fail_counter_delta=%d hung=%t",
			o.Seed, o.Errors, o.Write, o.Ref, o.Fallback, o.Clear, o.FailDelta, o.Hung)
	}
	b.WriteString(" |")
	for _, t := range o.Tables {
		fmt.Fprintf(&b, " %s=%s", t.Name, sameOrDiffer(t.Same))
	}
	for _, t := range o.Tables {
		if t.HeapMaster != t.HeapSlave || t.HeapMaster != t.PKMaster || t.HeapSlave != t.PKSlave {
			fmt.Fprintf(&b, " %s.heap(master=%d,slave=%d,master_by_pk=%d,slave_by_pk=%d)",
				t.Name, t.HeapMaster, t.HeapSlave, t.PKMaster, t.PKSlave)
		}
	}
	fmt.Fprintf(&b, " s_ballast=%s orphans(master=%d,slave=%d) slave=%s",
		sameOrDiffer(o.BallastSame), o.OrphansMaster, o.OrphansSlave, o.slave())
	if o.Ran {
		fmt.Fprintf(&b, " master_orphan_polls_during=%d peak=%d", o.OrphanPolls, o.OrphanPeak)
	}
	return b.String()
}

func sameOrDiffer(same bool) string {
	if same {
		return "same"
	}
	return "differ"
}
