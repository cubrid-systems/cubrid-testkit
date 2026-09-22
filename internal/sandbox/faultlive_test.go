package sandbox

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// The first group B measurement: a fault, a heal, and then the oracle.
//
// Everything this suite has established so far is about a topology that is
// holding still (design/module-ha.md §3-1). P5 is the property that the pair
// comparison runs **after** a disturbance and not only after a clean write,
// because the window after a healed partition is where a difference is made
// that no gauge afterwards reports (cluster-sandbox
// `findings/active-active-window.md`).
//
// What this asserts is the mechanics: that the fault can be produced, that it
// can be taken off, that the group settles to one active node again, and that
// the oracle can be asked afterwards and answers. What the two databases then
// hold is **reported and not asserted** -- encoding "the nodes must differ"
// would make an engine defect a requirement of this suite, and encoding "the
// nodes must agree" would fail the test on the day it finds what it is for.
// The verdict of a run belongs in a finding; what belongs here is that the
// question can be asked at all.
//
// Needs a cluster of its own. It makes two masters out of the one it is given,
// so it must not be pointed at a cluster something else is using:
//
//	extensions/cluster-sandbox/bin/csb cluster create --name gbha --build /path/to/install
//	TESTKIT_CSB=extensions/cluster-sandbox/bin/csb TESTKIT_CSB_CLUSTER=gbha \
//	  go test ./internal/sandbox/ -run TestLiveSplitBrain -v -timeout 40m -count=3
//
// # The two arms, and why they differ by one write
//
// cluster-sandbox measured this and found a one-directional merge: what the
// promoted slave wrote came back to the restored master, and what the master
// wrote during the split never reached the standby, seven runs and no
// exceptions, on the same engine commit this runs against
// (`findings/active-active-window.md`). The first runs here came out the other
// way -- the union, on both nodes -- with the split verified open at write
// time, so the difference is not addressing and it is not the build.
//
// The one methodological difference visible from here is a **write after the
// heal**. This suite's wait is a marker row written on the master and polled on
// the slave, so asking "has it replicated yet" pushes the master's log forward;
// the sandbox harness reads thirty seconds later and writes nothing. If that is
// what pulls the stranded row across, then the row is not lost but unsent, and
// which of those it is changes what the finding means.
//
// So the measurement is run both ways and the arms differ in that alone.
func TestLiveSplitBrainLeavesWhatTheGaugesDoNotReport(t *testing.T) {
	for _, arm := range []struct {
		name          string
		postHealWrite bool
	}{
		{"a-marker-is-written-after-the-heal", true},
		{"nothing-is-written-after-the-heal", false},
	} {
		t.Run(arm.name, func(t *testing.T) { splitBrainRun(t, arm.postHealWrite) })
	}
}

// observeAfterHeal are the moments the quiet arm reads the standby, in seconds
// after the heal.
//
// Thirty because that is when the sandbox finding took its direct read, so the
// first sample is comparable with it. The later two are what tells a row that
// is lost from a row that has not arrived yet -- "permanent" is a claim about
// the last sample and not the first, and nothing so far has watched past the
// first.
var observeAfterHeal = []time.Duration{30 * time.Second, 90 * time.Second, 150 * time.Second}

func splitBrainRun(t *testing.T, postHealWrite bool) {
	cli := liveCluster(t)
	ctx := context.Background()

	pair, err := Describe(ctx, cli, "")
	if err != nil {
		t.Fatal(err)
	}
	if pair.DB == "" || len(pair.Slaves) != 1 {
		t.Skipf("this measurement wants a two-node pair with a database, got %+v", pair)
	}
	first, second := pair.Master, pair.Slaves[0]

	// Whatever happens below, the cluster is handed back without a condition
	// on it. A partition left in force outlives the test process.
	defer func() {
		if _, cerr := cli.ClearFaults(context.Background(), ""); cerr != nil {
			t.Errorf("could not clear the faults this test made: %v", cerr)
		}
	}()
	if _, cerr := cli.ClearFaults(ctx, ""); cerr != nil {
		t.Fatalf("the cluster would not come clean before the test: %v", cerr)
	}
	if err := settle(ctx, t, cli, 2*time.Minute); err != nil {
		t.Fatal(err)
	}

	// 1. A row both nodes agree on, so that what follows is a difference this
	//    test made and not one it inherited.
	sql(ctx, t, cli, first, pair.DB, "DROP TABLE IF EXISTS tk_p5;")
	sql(ctx, t, cli, first, pair.DB,
		"CREATE TABLE tk_p5(id INT PRIMARY KEY, tag VARCHAR(20)); INSERT INTO tk_p5 VALUES(1, 'before');")
	if _, werr := pair.WaitForReplication(ctx, 60*time.Second); werr != nil {
		t.Fatalf("the pair would not replicate before the fault: %v", werr)
	}
	defer sql(context.Background(), t, cli, first, pair.DB, "DROP TABLE IF EXISTS tk_p5;")

	// 2. Two masters, by whichever route this cluster's configuration gives.
	sb, serr := cli.SplitBrain(ctx, "", 60*time.Second)
	if serr != nil {
		t.Fatalf("no split brain: %v", serr)
	}
	t.Logf("split brain: flavour %s, %d masters, partitioned %s", sb.Flavour, sb.Masters, sb.Partitioned)
	if sb.CancelReason != "" {
		t.Logf("the engine's own reason for not failing back: %s", sb.CancelReason)
	}

	// 3. One row per side, written while neither side can see the other.
	sql(ctx, t, cli, first, pair.DB, fmt.Sprintf("INSERT INTO tk_p5 VALUES(101, '%s');", first))
	sql(ctx, t, cli, second, pair.DB, fmt.Sprintf("INSERT INTO tk_p5 VALUES(201, '%s');", second))

	// The control, and it is not optional. Two nodes holding the same rows
	// after a heal has two explanations -- a merge, and both writes having gone
	// to one node -- and they are indistinguishable from the end state alone.
	// cluster-sandbox has published this exact mistake against itself: a reader
	// reported a bidirectional merge whose real cause was a selector that
	// resolved to nothing, so the INSERTs entered no database and the pair was
	// identical afterwards because replication had worked normally
	// (`findings/active-active-window.md`, "Re-measured, because a reader
	// reported the opposite"). So the split is read while it is open, and a
	// measurement whose writes did not land where they were addressed is void
	// rather than interesting.
	duringFirst, duringSecond := ids(ctx, t, cli, first, pair.DB), ids(ctx, t, cli, second, pair.DB)
	t.Logf("during the split: %s holds %v, %s holds %v", first, duringFirst, second, duringSecond)
	if !has(duringFirst, "101") || has(duringFirst, "201") {
		t.Fatalf("the split is not what this measures: %s holds %v, wanted its own row and not the other side's",
			first, duringFirst)
	}
	if !has(duringSecond, "201") || has(duringSecond, "101") {
		t.Fatalf("the split is not what this measures: %s holds %v, wanted its own row and not the other side's",
			second, duringSecond)
	}

	// 4. The heal. csb says in a note of its own that this is not recovery: the
	//    network is back and the roles are the group's business.
	cleared, cerr := cli.ClearFaults(ctx, "")
	if cerr != nil {
		t.Fatalf("the fault would not come off: %v", cerr)
	}
	for _, f := range cleared {
		t.Logf("cleared: %s", f)
	}
	if err := settle(ctx, t, cli, 3*time.Minute); err != nil {
		t.Fatal(err)
	}

	// 5. The oracle, after the fault. Something has to separate divergence from
	//    replication still in flight, and the two arms separate it differently:
	//    one asks the pair (a marker, which is a write), the other only waits.
	if postHealWrite {
		took, werr := pair.WaitForReplication(ctx, 60*time.Second)
		if werr != nil {
			t.Logf("the pair did not carry a marker across after the heal: %v", werr)
		} else {
			t.Logf("a marker crossed after the heal in %s", took.Round(time.Millisecond))
		}
	} else {
		waited := time.Duration(0)
		for _, at := range observeAfterHeal {
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-time.After(at - waited):
			}
			waited = at
			t.Logf("wrote nothing; %s after the heal %s holds %v and %s holds %v",
				at, first, ids(ctx, t, cli, first, pair.DB), second, ids(ctx, t, cli, second, pair.DB))
		}
	}
	const read = "SELECT id, tag FROM tk_p5 ORDER BY id"
	differing, oerr := pair.SameOnBothNodes(ctx, read)
	if oerr != nil {
		t.Fatalf("the oracle could not be asked after the fault: %v", oerr)
	}

	// 6. What each node holds, and what the group says about itself while it
	//    holds it.
	t.Logf("%s holds:\n%s", first, rows(ctx, t, cli, first, pair.DB, read))
	t.Logf("%s holds:\n%s", second, rows(ctx, t, cli, second, pair.DB, read))
	nodes, herr := cli.HAStatus(ctx)
	if herr != nil {
		t.Fatalf("the group would not say how it is doing: %v", herr)
	}
	for _, n := range nodes {
		t.Logf("gauge %s: role %s, state %s, fail_counter %d, apply lag %d page(s)",
			n.Name, n.Role, n.ServerState, n.Replication.FailCounter, n.Replication.ApplyLagPages)
	}

	switch {
	case differing == "" && Healthy(nodes):
		t.Log("VERDICT: the pair agrees and says it is healthy")
	case differing == "":
		t.Log("VERDICT: the pair agrees while its own gauges do not read healthy")
	case Healthy(nodes):
		t.Logf("VERDICT: %s disagrees with the master over data, and every gauge reads healthy. "+
			"This is P5: the engine's opinion of itself is not the oracle", differing)
	default:
		t.Logf("VERDICT: %s disagrees with the master over data, and the gauges say so too", differing)
	}
}

// settle waits for the group to have exactly one node calling itself active.
//
// Zero is a group mid-transition and two is a split brain; a measurement taken
// during either is a measurement of the clock. So this is a poll on state and
// not a sleep, for the same reason every other wait in this package is.
func settle(ctx context.Context, t *testing.T, cli *CLI, timeout time.Duration) error {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		nodes, err := cli.HAStatus(ctx)
		if err != nil {
			return err
		}
		active := Actives(nodes)
		if len(active) == 1 {
			t.Logf("settled: %s is active", active[0])
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("sandbox: the group still has %d active node(s) after %s: %v",
				len(active), timeout, active)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// sql runs statements on one named node and fails the test if the node could
// not be reached. A statement the engine refuses is the caller's to read: a
// DROP of a table that is not there is refused every time this test starts
// clean.
func sql(ctx context.Context, t *testing.T, cli *CLI, node, db, statements string) string {
	t.Helper()
	res, err := NewNode(cli, node).Run(ctx,
		fmt.Sprintf("csql -u dba -c %q %s 2>&1", statements, db))
	if err != nil {
		t.Fatalf("%s would not run %q: %v", node, statements, err)
	}
	return res.Stdout
}

// ids is the key column as a set, which is what a control compares. Read with
// -t -N so the answer is one bare value per line and nothing has to be guessed
// out of csql's table drawing.
func ids(ctx context.Context, t *testing.T, cli *CLI, node, db string) []string {
	t.Helper()
	res, err := NewNode(cli, node).Run(ctx,
		fmt.Sprintf("csql -u dba -t -N -c \"SELECT id FROM tk_p5 ORDER BY id\" %s 2>/dev/null", db))
	if err != nil {
		t.Fatalf("%s would not answer a read: %v", node, err)
	}
	var out []string
	for _, line := range strings.Split(res.Stdout, "\n") {
		s := strings.TrimSpace(line)
		if s == "" || strings.HasPrefix(s, "Time:") || strings.HasPrefix(s, "Program 'csql'") ||
			strings.Contains(s, " sec)") || strings.HasPrefix(s, "ERROR") {
			continue
		}
		out = append(out, s)
	}
	return out
}

func has(rows []string, id string) bool {
	for _, r := range rows {
		if r == id {
			return true
		}
	}
	return false
}

// rows is a read rendered for a log: csql's connection notification carries a
// path and a pid that say nothing about the answer.
func rows(ctx context.Context, t *testing.T, cli *CLI, node, db, query string) string {
	t.Helper()
	out := sql(ctx, t, cli, node, db, query)
	var keep []string
	for _, line := range strings.Split(out, "\n") {
		s := strings.TrimSpace(line)
		if s == "" || strings.HasPrefix(s, "Time:") || strings.HasPrefix(s, "Program 'csql'") {
			continue
		}
		keep = append(keep, "    "+s)
	}
	return strings.Join(keep, "\n")
}
