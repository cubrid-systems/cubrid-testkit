package sandbox

import (
	"context"
	"os"
	"strings"
	"testing"
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
