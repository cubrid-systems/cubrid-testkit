package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/cubrid-systems/cubrid-testkit/internal/conf"
	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
	"github.com/cubrid-systems/cubrid-testkit/internal/topology"
)

// Instance is the name the fragment is asked for.
//
// topology.From only recognises `env.instance<N>.`, which is CTP's own shape,
// and csb names the instance after the cluster unless it is told otherwise. So
// it is told otherwise: asking for instance1 means the fragment arrives in the
// shape this runner already reads, and no key name is invented at either end.
const Instance = "instance1"

// Pair is a master and its slaves, as a sandbox cluster describes them.
//
// It is deliberately not a fleet. ADR-014 allows exactly this: one run, in one
// place, driving a database that happens to span nodes. Nothing here deploys,
// schedules or chooses between machines -- the cluster exists before the run
// starts and outlives it.
type Pair struct {
	// Cluster is the csb cluster these nodes belong to.
	Cluster string
	// Conf is the fragment as csb rendered it, kept whole. A run records what it
	// was given rather than a reconstruction of it: the evidence for "which
	// topology was this" has to be the bytes, not this package's reading of them.
	Conf string
	// Instance is the configuration parsed out of Conf.
	Instance *topology.Instance
	// Master and Slaves are node names, as ssh.host carries them.
	Master string
	Slaves []string
	// DB is the database csb created, from ha_db_list. Empty when the fragment
	// did not name one.
	DB string

	cli *CLI
}

// describeData is what `cluster describe --format ctp` puts under data.
type describeData struct {
	Format string `json:"format"`
	Conf   string `json:"conf"`
}

// Describe asks a cluster to render itself as a CTP fragment and reads it.
//
// ctpHome is only for ${CTP_HOME} substitution, which the fragment does not use
// today; it is passed rather than looked up so that a value read here means the
// same as a value read from a file (conf.ParseText).
func Describe(ctx context.Context, cli *CLI, ctpHome string) (*Pair, error) {
	env, err := cli.call(ctx, "cluster", "describe", "--format", "ctp", "--instance", Instance)
	if err != nil {
		return nil, err
	}
	var d describeData
	if jerr := json.Unmarshal(env.Data, &d); jerr != nil {
		return nil, fmt.Errorf("sandbox: cluster %q: cannot read the describe: %w", cli.Cluster, jerr)
	}
	if strings.TrimSpace(d.Conf) == "" {
		return nil, fmt.Errorf("sandbox: cluster %q described itself as an empty fragment", cli.Cluster)
	}

	cfg, err := conf.ParseText("csb:"+cli.Cluster, d.Conf, ctpHome)
	if err != nil {
		return nil, err
	}
	instances, err := topology.From(cfg)
	if err != nil {
		return nil, fmt.Errorf("sandbox: cluster %q: %w", cli.Cluster, err)
	}
	switch {
	case len(instances) == 0:
		// The fragment parsed and held no env.instanceN key. The likeliest cause
		// is a csb that did not take --instance and named the instance after the
		// cluster instead, which topology.From cannot see -- so say that rather
		// than "0 instances", which reads as an empty cluster.
		return nil, fmt.Errorf("sandbox: cluster %q described no env.%s.* keys. "+
			"Check that this csb takes --instance; without it the fragment is named after the cluster",
			cli.Cluster, Instance)
	case len(instances) > 1:
		return nil, fmt.Errorf("sandbox: cluster %q described %d instances and a run uses one (ADR-014)",
			cli.Cluster, len(instances))
	}
	inst := instances[0]

	p := &Pair{Cluster: cli.Cluster, Conf: d.Conf, Instance: inst, cli: cli}
	p.Master = strings.TrimSpace(inst.Role(topology.RoleMaster)["ssh.host"])
	p.DB = strings.TrimSpace(inst.Role(topology.RoleHA)["ha_db_list"])
	for _, role := range slaveRoles(cfg) {
		if h := strings.TrimSpace(inst.Role(role)["ssh.host"]); h != "" {
			p.Slaves = append(p.Slaves, h)
		}
	}
	// A pair with no master or no slave is not a topology this runner can use,
	// and saying so here is cheaper than a case failing to connect to one.
	if p.Master == "" || len(p.Slaves) == 0 {
		return nil, fmt.Errorf("sandbox: cluster %q is not an HA pair: master=%q slaves=%d",
			cli.Cluster, p.Master, len(p.Slaves))
	}
	return p, nil
}

// slaveRoles is the slave role names present in a fragment, in order.
//
// csb writes `slave` for one and `slave1`, `slave2` ... for several, which is
// CTP's own convention (conf/ha_repl.conf: "A master can have multiple slaves,
// for instance, slave1, slave2"). topology.From carries a role it finds whether
// or not topology.Roles names it, so reading them back is a question of which
// names are there and in what order -- which only the keys can answer.
func slaveRoles(cfg *conf.Config) []string {
	seen := map[string]bool{}
	prefix := "env." + Instance + "."
	for _, k := range cfg.Keys() {
		rest, ok := strings.CutPrefix(k, prefix)
		if !ok {
			continue
		}
		role, _, ok := strings.Cut(rest, ".")
		if !ok || !strings.HasPrefix(role, topology.RoleSlave) {
			continue
		}
		seen[role] = true
	}
	roles := make([]string, 0, len(seen))
	for r := range seen {
		roles = append(roles, r)
	}
	// "slave" before "slave1" before "slave2": sorting the names gives that, and
	// the order is the one the fragment lists rather than a map's.
	sort.Strings(roles)
	return roles
}

// MasterChannel and SlaveChannel are the channels for those nodes.
func (p *Pair) MasterChannel() exec.Channel { return NewNode(p.caseCLI(), p.Master) }

// caseCLI is the cluster with the bound a case's SQL needs rather than the one
// a read needs. Every channel this type hands out runs case SQL -- that is what
// the type is for -- so the distinction lives here and no caller has to
// remember it. A CLI the caller gave an explicit Timeout keeps it.
func (p *Pair) caseCLI() *CLI {
	if p.cli == nil || p.cli.Timeout > 0 {
		return p.cli
	}
	long := *p.cli
	long.Timeout = CaseTimeout
	return &long
}

// SlaveChannel returns the channel for slave n, counting from zero.
func (p *Pair) SlaveChannel(n int) (exec.Channel, error) {
	if n < 0 || n >= len(p.Slaves) {
		return nil, fmt.Errorf("sandbox: cluster %q has %d slave(s) and slave %d was asked for",
			p.Cluster, len(p.Slaves), n)
	}
	return NewNode(p.caseCLI(), p.Slaves[n]), nil
}

// Nodes names every node, master first. What a run records as "where this ran".
func (p *Pair) Nodes() []string {
	return append([]string{p.Master}, p.Slaves...)
}

// Describe names the topology for a log line.
func (p *Pair) Describe() string {
	return fmt.Sprintf("csb cluster %q: master %s, slave(s) %s",
		p.Cluster, p.Master, strings.Join(p.Slaves, ", "))
}
