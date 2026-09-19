package sandbox

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// fragment is what `csb cluster describe --format ctp --instance instance1`
// writes, copied from cluster-sandbox's WriteCTPConf rather than invented: the
// comment block, the master, the slave, ha_db_list, and the engine parameters.
const fragment = `# ha_repl fragment for the csb cluster "hadb", written by csb.
# The ssh.host values are CONTAINER NAMES, not hosts: these nodes run no
# sshd and publish no port. Reach them with a docker-exec Channel --
#     docker exec <ssh.host> bash -lc '<command>'

env.instance1.master.ssh.host=hadb-master
env.instance1.master.ssh.user=1000:1000
env.instance1.slave.ssh.host=hadb-slave
env.instance1.slave.ssh.user=1000:1000

# the database csb created
env.instance1.ha.ha_db_list=testdb
env.instance1.cubrid.cubrid_port_id=1523
env.instance1.ha.ha_port_id=59901
`

// describing wraps a fragment in the envelope `cluster describe` returns.
func describing(t *testing.T, conf string) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"schema": Schema, "command": "cluster describe", "ok": true,
		"data": map[string]any{"format": "ctp", "conf": conf},
	})
	if err != nil {
		t.Fatal(err)
	}
	return "cat <<'CSBEOF'\n" + string(b) + "\nCSBEOF"
}

// The fragment is read with the parser this runner already has, and becomes the
// model it already has. Nothing here invents a key name: they are CTP's frozen
// surface, and only the transport changed.
func TestAFragmentBecomesAPair(t *testing.T) {
	bin, log := fakeCSB(t, describing(t, fragment))
	p, err := Describe(context.Background(), &CLI{Bin: bin, Cluster: "hadb"}, "/ctp")
	if err != nil {
		t.Fatal(err)
	}
	if p.Master != "hadb-master" {
		t.Errorf("master: %q", p.Master)
	}
	if len(p.Slaves) != 1 || p.Slaves[0] != "hadb-slave" {
		t.Errorf("slaves: %v", p.Slaves)
	}
	if p.DB != "testdb" {
		t.Errorf("db: %q", p.DB)
	}
	// The engine parameters came along, which is what makes the fragment worth
	// reading rather than just the two node names.
	if got := p.Instance.Role("cubrid")["cubrid_port_id"]; got != "1523" {
		t.Errorf("cubrid_port_id: %q", got)
	}
	// And the bytes are kept whole: the evidence for "which topology was this"
	// has to be what csb said, not this package's reading of it.
	if p.Conf != fragment {
		t.Errorf("the fragment was not kept as it arrived")
	}
	// instance1, because topology.From only recognises env.instance<N>.
	if !contains(argv(t, log), "--instance") || !contains(argv(t, log), Instance) {
		t.Errorf("the instance was not named: %v", argv(t, log))
	}
}

// Several slaves are slave1, slave2 ... which is CTP's own convention and is
// what csb writes when there is more than one. topology.Roles does not name
// them and cannot -- the count is the configuration's -- so From carries any
// role the keys mention and this reads back which ones, in order.
func TestSeveralSlavesAreFoundAndOrdered(t *testing.T) {
	conf := strings.ReplaceAll(fragment, "env.instance1.slave.ssh.host=hadb-slave",
		"env.instance1.slave1.ssh.host=hadb-slave1\nenv.instance1.slave2.ssh.host=hadb-slave2")
	bin, _ := fakeCSB(t, describing(t, conf))
	p, err := Describe(context.Background(), &CLI{Bin: bin, Cluster: "hadb"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Slaves) != 2 || p.Slaves[0] != "hadb-slave1" || p.Slaves[1] != "hadb-slave2" {
		t.Fatalf("slaves: %v", p.Slaves)
	}
	if _, err := p.SlaveChannel(2); err == nil {
		t.Error("a slave that does not exist was handed out")
	}
	if got := p.Nodes(); len(got) != 3 || got[0] != "hadb-master" {
		t.Errorf("nodes: %v", got)
	}
}

// A single node is not a topology this runner can use, and saying so here costs
// less than a case failing to connect to a slave that is not there.
func TestAClusterThatIsNotAPairIsRefused(t *testing.T) {
	conf := "env.instance1.master.ssh.host=lonely\n"
	bin, _ := fakeCSB(t, describing(t, conf))
	_, err := Describe(context.Background(), &CLI{Bin: bin, Cluster: "one"}, "")
	if err == nil || !strings.Contains(err.Error(), "not an HA pair") {
		t.Fatalf("got %v", err)
	}
}

// The channels are the nodes the fragment named, and they say which cluster they
// belong to: a log line from a two-cluster machine has to be readable.
func TestTheChannelsNameTheirNodes(t *testing.T) {
	bin, _ := fakeCSB(t, describing(t, fragment))
	p, err := Describe(context.Background(), &CLI{Bin: bin, Cluster: "hadb"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := p.MasterChannel().Describe(); got != "csb:hadb/hadb-master" {
		t.Errorf("master channel: %q", got)
	}
	slave, err := p.SlaveChannel(0)
	if err != nil {
		t.Fatal(err)
	}
	if got := slave.Describe(); got != "csb:hadb/hadb-slave" {
		t.Errorf("slave channel: %q", got)
	}
}

func TestAnEmptyFragmentIsRefused(t *testing.T) {
	bin, _ := fakeCSB(t, describing(t, "   \n"))
	_, err := Describe(context.Background(), &CLI{Bin: bin, Cluster: "hadb"}, "")
	if err == nil || !strings.Contains(err.Error(), "empty fragment") {
		t.Fatalf("got %v", err)
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

// A csb that did not take --instance names the instance after the cluster, and
// topology.From cannot see those keys. The failure then looks like an empty
// cluster, so the error names the real cause instead.
func TestAFragmentNamedAfterTheClusterSaysWhatIsWrong(t *testing.T) {
	conf := "env.hadb.master.ssh.host=m\nenv.hadb.slave.ssh.host=s\n"
	bin, _ := fakeCSB(t, describing(t, conf))
	_, err := Describe(context.Background(), &CLI{Bin: bin, Cluster: "hadb"}, "")
	if err == nil || !strings.Contains(err.Error(), "--instance") {
		t.Fatalf("got %v", err)
	}
}
