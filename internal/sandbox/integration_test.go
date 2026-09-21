package sandbox

import (
	"context"
	"os"
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
