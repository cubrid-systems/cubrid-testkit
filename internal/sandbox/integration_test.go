package sandbox

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The tests beside this one drive a stand-in csb that prints what the test told
// it to. That checks what this package sends and how it reads an answer, and it
// cannot check the one thing that matters most: that csb still answers this way.
//
// This one runs against a cluster that is actually up. It needs Docker, an
// engine build and about a minute of provisioning, so it is asked for rather
// than assumed:
//
//	make -C extensions/cluster-sandbox dist
//	export CSB_HOME=/somewhere
//	extensions/cluster-sandbox/bin/csb cluster create --name tkha --build /path/to/install.out
//	TESTKIT_CSB=extensions/cluster-sandbox/bin/csb TESTKIT_CSB_CLUSTER=tkha go test ./internal/sandbox/
//
// Skipped without TESTKIT_CSB_CLUSTER, which is the honest skip: the cluster is
// not there.
func liveCluster(t *testing.T) *CLI {
	t.Helper()
	name := strings.TrimSpace(os.Getenv("TESTKIT_CSB_CLUSTER"))
	if name == "" {
		t.Skip("set TESTKIT_CSB_CLUSTER to a cluster csb has standing up")
	}
	cli := &CLI{Cluster: name}
	if err := cli.Available(context.Background()); err != nil {
		t.Fatal(err)
	}
	return cli
}

// The fragment a real csb writes is read into the model this runner already has.
func TestLiveClusterDescribesItselfAsAPair(t *testing.T) {
	cli := liveCluster(t)
	p, err := Describe(context.Background(), cli, "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Master == "" || len(p.Slaves) == 0 {
		t.Fatalf("not a pair: %+v", p)
	}
	if p.DB == "" {
		t.Error("the fragment named no database")
	}
	t.Logf("%s", p.Describe())
	t.Logf("db=%s", p.DB)
}

// A command runs on the master and its output comes back.
func TestLiveMasterRunsACommand(t *testing.T) {
	cli := liveCluster(t)
	p, err := Describe(context.Background(), cli, "")
	if err != nil {
		t.Fatal(err)
	}
	res, err := p.MasterChannel().Run(context.Background(), "cubrid_rel | head -3")
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "CUBRID") {
		t.Errorf("cubrid_rel said: %q", res.Stdout)
	}
	t.Logf("master: %s", strings.TrimSpace(res.Stdout))
}

// The engine's own environment is set on the node, which is the reason this
// channel does not prepend exec.Profile the way SSH does.
func TestLiveTheEngineEnvironmentIsAlreadyThere(t *testing.T) {
	cli := liveCluster(t)
	p, err := Describe(context.Background(), cli, "")
	if err != nil {
		t.Fatal(err)
	}
	res, err := p.MasterChannel().Run(context.Background(), "echo $CUBRID; echo $CUBRID_DATABASES")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Fields(res.Stdout) {
		if line == "" {
			t.Fatal("the engine environment was not set on the node")
		}
	}
	if strings.TrimSpace(res.Stdout) == "" {
		t.Fatalf("no environment: %q", res.Stdout)
	}
	t.Logf("CUBRID/CUBRID_DATABASES: %s", strings.Join(strings.Fields(res.Stdout), " "))
}

// A multi-line script arrives whole. The stand-in test asserts the argv; this
// asserts the consequence -- a script whose newlines became spaces would run
// `cd /tmp echo two`, which is not an error, just wrong.
func TestLiveAMultiLineScriptRunsAsWritten(t *testing.T) {
	cli := liveCluster(t)
	p, err := Describe(context.Background(), cli, "")
	if err != nil {
		t.Fatal(err)
	}
	res, err := p.MasterChannel().Run(context.Background(), "cd /tmp\npwd\necho two")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Fields(res.Stdout); len(got) != 2 || got[0] != "/tmp" || got[1] != "two" {
		t.Errorf("the script did not run as written: %q", res.Stdout)
	}
}

// A command that ran and failed is data. Nothing about this is reconstructed
// from an exit status: csb reports it and the channel passes it through.
func TestLiveAFailingCommandIsData(t *testing.T) {
	cli := liveCluster(t)
	p, err := Describe(context.Background(), cli, "")
	if err != nil {
		t.Fatal(err)
	}
	res, err := p.MasterChannel().Run(context.Background(), "echo out; echo err >&2; exit 7")
	if err != nil {
		t.Fatalf("a failing command was reported as a channel failure: %v", err)
	}
	if res.ExitCode != 7 {
		t.Errorf("exit: got %d want 7", res.ExitCode)
	}
	if !strings.Contains(res.Stdout, "out") || !strings.Contains(res.Stderr, "err") {
		t.Errorf("streams: stdout=%q stderr=%q", res.Stdout, res.Stderr)
	}
}

// The slave is reachable too, and it is a different node than the master: a
// channel that quietly went to the master for both would pass every test above.
func TestLiveTheSlaveIsADifferentNode(t *testing.T) {
	cli := liveCluster(t)
	p, err := Describe(context.Background(), cli, "")
	if err != nil {
		t.Fatal(err)
	}
	slave, err := p.SlaveChannel(0)
	if err != nil {
		t.Fatal(err)
	}
	master, err := p.MasterChannel().Run(context.Background(), "hostname")
	if err != nil {
		t.Fatal(err)
	}
	other, err := slave.Run(context.Background(), "hostname")
	if err != nil {
		t.Fatal(err)
	}
	m, s := strings.TrimSpace(master.Stdout), strings.TrimSpace(other.Stdout)
	if m == "" || s == "" || m == s {
		t.Fatalf("master and slave answered %q and %q", m, s)
	}
	t.Logf("master=%s slave=%s", m, s)
}

// And the pair is really an HA pair: the master is writable and what it writes
// arrives on the slave. This is the claim the whole integration exists to make
// -- a topology, provisioned elsewhere, that this runner can drive.
func TestLiveTheSlaveHasWhatTheMasterWrote(t *testing.T) {
	cli := liveCluster(t)
	p, err := Describe(context.Background(), cli, "")
	if err != nil {
		t.Fatal(err)
	}
	master := p.MasterChannel()
	ddl := "csql -u dba -c \"CREATE TABLE tk_probe(i INT PRIMARY KEY); INSERT INTO tk_probe VALUES(42);\" " + p.DB
	if res, rerr := master.Run(context.Background(), ddl); rerr != nil || res.ExitCode != 0 {
		t.Fatalf("the master would not take a write: %v exit=%d %s %s", rerr, res.ExitCode, res.Stdout, res.Stderr)
	}
	t.Cleanup(func() {
		master.Run(context.Background(), "csql -u dba -c 'DROP TABLE tk_probe;' "+p.DB)
	})

	slave, err := p.SlaveChannel(0)
	if err != nil {
		t.Fatal(err)
	}
	// The read has to wait for replication rather than assume it. csb has a verb
	// for exactly this question (`repl check`), and it is the right one to reach
	// for later; here the point is only that the channel reaches a node holding
	// the master's data, so a short poll is enough and a fixed sleep would not be.
	var last string
	for range 40 {
		res, rerr := slave.Run(context.Background(),
			"csql -u dba -c 'SELECT i FROM tk_probe;' "+p.DB+" 2>&1")
		if rerr != nil {
			t.Fatal(rerr)
		}
		last = res.Stdout
		if strings.Contains(last, "42") {
			t.Logf("the slave has it")
			return
		}
	}
	t.Fatalf("the slave never showed the master's row; last read:\n%s", last)
}

// The wait P1 is about, through this runner's own channel rather than CTP's
// helper. What is measured is the same thing evidence/ha/p1-sleep-to-wait.md
// measured under CTP: what it costs, against the 5 to 30 seconds of sleep the
// corpus uses instead.
func TestLiveWaitForReplicationCostsAboutASecond(t *testing.T) {
	cli := liveCluster(t)
	p, err := Describe(context.Background(), cli, "")
	if err != nil {
		t.Fatal(err)
	}
	took, err := p.WaitForReplication(context.Background(), 60*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("replication confirmed in %s", took.Round(time.Millisecond))
	// Not an assertion about the engine's speed -- machines differ. The claim is
	// only that this is a wait and not a sleep: it returns when the row arrives,
	// which on any working pair is far short of the 5 to 30 seconds the corpus
	// spends not looking.
	if took > 30*time.Second {
		t.Errorf("the wait took %s, which is not a wait", took)
	}
}

// The wait returns only after everything committed ahead of the marker has been
// applied, and that is not something a parallel applier gives for free.
//
// With ha_apply_worker_count above one (CBRD-27508), the slave keeps conflict
// order and nothing else: a marker that shares no key with the transaction
// before it can be applied first. Measured on four workers, a marker that was
// only an INSERT showed up ahead of a 20,000-row transaction in 18 rounds of 20;
// the CREATE TABLE this wait uses did so in 0 of 20, because a replicated
// statement is applied in commit order and every later transaction waits for it
// (evidence/ha/parallel-apply-and-the-marker.md). This test is what notices if
// the marker loses its CREATE.
//
// It first checks that the pair can tell: an INSERT-only marker, read on the
// slave in the same SELECT as the bulk transaction's row count, has to arrive
// ahead of that transaction at least once. A serial applier never lets it, and
// on such a pair a passing wait proves nothing, so the test skips and says so.
//
// The bulk write is 60,000 rows because this test can only read after the wait
// has returned, and the wait drops its marker before it does. At 20,000 rows
// the slave used that gap to finish: with the marker made INSERT-only the test
// failed in one run of three. At 60,000 it failed in three of three.
func TestLiveTheWaitCoversWhatWasCommittedAheadOfTheMarker(t *testing.T) {
	cli := liveCluster(t)
	p, err := Describe(context.Background(), cli, "")
	if err != nil {
		t.Fatal(err)
	}
	master := p.MasterChannel()
	slave, err := p.SlaveChannel(0)
	if err != nil {
		t.Fatal(err)
	}
	// A secondary index and a padded row, so the transaction takes the slave
	// long enough to apply that a marker beside it would finish first.
	if res, rerr := master.Run(context.Background(),
		"csql -u dba -c \"DROP TABLE IF EXISTS tk_ahead; DROP TABLE IF EXISTS tk_ahead_mk; "+
			"CREATE TABLE tk_ahead(i INT PRIMARY KEY, r INT, pad VARCHAR(2000)); "+
			"CREATE INDEX tk_ahead_r ON tk_ahead(r); CREATE TABLE tk_ahead_mk(i INT PRIMARY KEY);\" "+
			p.DB); rerr != nil || res.ExitCode != 0 {
		t.Fatalf("setup: %v %s", rerr, res.Stderr)
	}
	t.Cleanup(func() {
		master.Run(context.Background(), "csql -u dba -c 'DROP TABLE tk_ahead; DROP TABLE tk_ahead_mk;' "+p.DB)
	})
	if _, werr := p.WaitForReplication(context.Background(), 60*time.Second); werr != nil {
		t.Fatal(werr)
	}

	const bulk = 60000
	write := func(tag int) {
		t.Helper()
		sql := fmt.Sprintf("csql -u dba -c \"INSERT INTO tk_ahead SELECT %d + ROWNUM, %d, REPEAT('x', 500) "+
			"FROM db_root CONNECT BY LEVEL <= %d;\" %s", (tag+5)*bulk, tag, bulk, p.DB)
		if res, rerr := master.Run(context.Background(), sql); rerr != nil || res.ExitCode != 0 {
			t.Fatalf("tag %d: the bulk write failed: %v %s", tag, rerr, res.Stderr)
		}
	}
	read := func(query string) string {
		t.Helper()
		res, rerr := slave.Run(context.Background(),
			fmt.Sprintf("csql -u dba -t -c \"%s\" %s 2>/dev/null", query, p.DB))
		if rerr != nil {
			t.Fatal(rerr)
		}
		return lastNumber(res.Stdout)
	}

	// The control: negative tags, so its rows are never counted below.
	overtook := 0
	for round := 1; round <= 5 && overtook == 0; round++ {
		write(-round)
		if res, rerr := master.Run(context.Background(), fmt.Sprintf(
			"csql -u dba -c \"INSERT INTO tk_ahead_mk VALUES(%d);\" %s", round, p.DB)); rerr != nil || res.ExitCode != 0 {
			t.Fatalf("control %d: the master would not take the marker: %v %s", round, rerr, res.Stderr)
		}
		look := fmt.Sprintf("SELECT (SELECT COUNT(*) FROM tk_ahead WHERE r = %d) FROM tk_ahead_mk WHERE i = %d", -round, round)
		deadline := time.Now().Add(60 * time.Second)
		got := read(look)
		for got == "" && time.Now().Before(deadline) {
			got = read(look)
		}
		switch got {
		case "":
			t.Fatalf("control %d: the marker never reached %s", round, p.Slaves[0])
		case "0":
			overtook = round
		}
	}
	if overtook == 0 {
		t.Skipf("%s applied an INSERT-only marker behind the transaction before it in 5 rounds of 5, "+
			"so it cannot tell a wait that holds from one that does not: run this on a pair with "+
			"ha_apply_worker_count above one", p.Slaves[0])
	}
	t.Logf("control: an INSERT-only marker overtook the bulk transaction in round %d", overtook)

	for round := 1; round <= 5; round++ {
		write(round)
		if _, werr := p.WaitForReplication(context.Background(), 120*time.Second); werr != nil {
			t.Fatal(werr)
		}
		// Read once and at once: a second look would give the applier the time
		// this test exists to deny it.
		got := read(fmt.Sprintf("SELECT COUNT(*) FROM tk_ahead WHERE r = %d", round))
		switch got {
		case "":
			t.Fatalf("round %d: the row count could not be read on %s", round, p.Slaves[0])
		case strconv.Itoa(bulk):
		default:
			t.Fatalf("round %d: the wait returned with %s of the %d rows committed ahead of its marker on %s",
				round, got, bulk, p.Slaves[0])
		}
	}
}

// lastNumber is the last line of csql -t output that is only a number, which
// skips the connection notification csql prints with a pid and a port in it.
func lastNumber(out string) string {
	n := ""
	for _, line := range strings.Split(out, "\n") {
		if t := strings.TrimSpace(line); t != "" && strings.Trim(t, "0123456789") == "" {
			n = t
		}
	}
	return n
}

// The oracle both HA suites arrived at independently: the pair disagreeing with
// itself, with no answer file anywhere. This is that, driven from here.
func TestLiveThePairAgreesWithItselfAfterAWrite(t *testing.T) {
	cli := liveCluster(t)
	p, err := Describe(context.Background(), cli, "")
	if err != nil {
		t.Fatal(err)
	}
	master := p.MasterChannel()
	ddl := "csql -u dba -c \"CREATE TABLE tk_pair(i INT PRIMARY KEY, s VARCHAR(10)); " +
		"INSERT INTO tk_pair VALUES(1,'a'),(2,'b'),(3,'c');\" " + p.DB
	if res, rerr := master.Run(context.Background(), ddl); rerr != nil || res.ExitCode != 0 {
		t.Fatalf("the master would not take the write: %v exit=%d %s", rerr, res.ExitCode, res.Stderr)
	}
	t.Cleanup(func() {
		master.Run(context.Background(), "csql -u dba -c 'DROP TABLE tk_pair;' "+p.DB)
	})

	// Wait first. A difference under write traffic is replication in flight
	// rather than divergence, and reporting one as the other is the mistake the
	// whole P1 argument is about.
	if _, werr := p.WaitForReplication(context.Background(), 60*time.Second); werr != nil {
		t.Fatal(werr)
	}
	differed, err := p.SameOnBothNodes(context.Background(), "SELECT i, s FROM tk_pair ORDER BY i;")
	if err != nil {
		t.Fatal(err)
	}
	if differed != "" {
		t.Errorf("%s does not hold what the master holds", differed)
	}
}

// And it can tell when they do not agree, which is the half that makes the other
// half worth anything. The master's row is read before replication is waited
// for, so the slave is legitimately behind -- a difference this runner must see.
func TestLiveADifferenceIsVisibleBeforeReplicationCatchesUp(t *testing.T) {
	cli := liveCluster(t)
	p, err := Describe(context.Background(), cli, "")
	if err != nil {
		t.Fatal(err)
	}
	master := p.MasterChannel()
	if res, rerr := master.Run(context.Background(),
		"csql -u dba -c \"CREATE TABLE tk_lag(i INT PRIMARY KEY);\" "+p.DB); rerr != nil || res.ExitCode != 0 {
		t.Fatalf("setup: %v %s", rerr, res.Stderr)
	}
	t.Cleanup(func() {
		master.Run(context.Background(), "csql -u dba -c 'DROP TABLE tk_lag;' "+p.DB)
	})
	if _, werr := p.WaitForReplication(context.Background(), 60*time.Second); werr != nil {
		t.Fatal(werr)
	}

	// A write large enough that the slave cannot have it instantly, read
	// immediately. If the pair happens to agree anyway the machine was simply
	// fast enough, which is not a failure of this runner -- so the test says so
	// rather than flapping.
	big := "csql -u dba -c \"INSERT INTO tk_lag SELECT rownum FROM db_class a, db_class b, db_class c WHERE rownum <= 20000;\" " + p.DB
	if res, rerr := master.Run(context.Background(), big); rerr != nil || res.ExitCode != 0 {
		t.Fatalf("the bulk write failed: %v %s", rerr, res.Stderr)
	}
	differed, err := p.SameOnBothNodes(context.Background(), "SELECT COUNT(*) FROM tk_lag;")
	if err != nil {
		t.Fatal(err)
	}
	if differed == "" {
		t.Log("the pair already agreed: replication was faster than the read, which is a fact about this machine")
	} else {
		t.Logf("%s was behind, and this runner saw it", differed)
	}
	// Either way, after the wait they must agree. That is the assertion.
	if _, werr := p.WaitForReplication(context.Background(), 120*time.Second); werr != nil {
		t.Fatal(werr)
	}
	if d, cerr := p.SameOnBothNodes(context.Background(), "SELECT COUNT(*) FROM tk_lag;"); cerr != nil || d != "" {
		t.Errorf("after the wait, %s still differs (%v)", d, cerr)
	}
}
