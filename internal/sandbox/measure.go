package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
)

// A measurement cluster: one database node and one client node, pinned,
// with the engine parameters and the broker's CAS count decided up front.
//
// # Why a second create and not options on the first
//
// Create (lifecycle.go) stands an HA pair up for the frozen ha_repl path, and
// its two arguments are the whole of what that path decides. The perf runner
// (ADR-EXT-011) decides a great deal more -- the preset, a client image, where
// each kind of node sits, cubrid.conf keys, broker keys -- and every one of
// those is a flag cluster-sandbox gained for it (its PRs #8–#12). Folding them
// into Create would make the HA call site carry fields it must never set.
// So this is a second entry point with its own options, and the HA path is
// untouched.

// CreateOptions is what `csb cluster create` takes for a measurement cluster.
// Zero values are csb's defaults; the name is the CLI's cluster.
type CreateOptions struct {
	Preset       string // single or ha
	Build        string // the install tree, bind-mounted read-only
	DB           string // the first database; empty is the cluster name
	Clients      int
	ClientImage  string
	WithBroker   bool
	CPUSet       string
	ClientCPUSet string
	Set          []string // key=value for cubrid.conf / cubrid_ha.conf
	BrokerSet    []string // KEY=VALUE for the broker section
	Labels       map[string]string
	// Timeout bounds the call and is also passed to csb as --timeout, so the
	// two clocks agree (Design §5.3: both, every call).
	Timeout time.Duration
}

// CreateWith stands a cluster up as the options say.
func (c *CLI) CreateWith(ctx context.Context, o CreateOptions) error {
	args := []string{"--name", c.Cluster, "--preset", firstNonEmpty(o.Preset, "single"), "--build", o.Build}
	if o.DB != "" {
		args = append(args, "--db", o.DB)
	}
	if o.Clients > 0 {
		args = append(args, "--clients", strconv.Itoa(o.Clients))
	}
	if o.ClientImage != "" {
		args = append(args, "--client-image", o.ClientImage)
	}
	if o.WithBroker {
		args = append(args, "--with-broker")
	}
	if o.CPUSet != "" {
		args = append(args, "--cpuset", o.CPUSet)
	}
	if o.ClientCPUSet != "" {
		args = append(args, "--client-cpuset", o.ClientCPUSet)
	}
	for _, kv := range o.Set {
		args = append(args, "--set", kv)
	}
	for _, kv := range o.BrokerSet {
		args = append(args, "--broker-set", kv)
	}
	keys := make([]string, 0, len(o.Labels))
	for k := range o.Labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "--label", k+"="+o.Labels[k])
	}
	t := o.Timeout
	if t <= 0 {
		t = ProvisionTimeout
	}
	args = append(args, "--timeout", t.String())
	long := *c
	long.Timeout = t + 30*time.Second
	_, err := long.call(ctx, "cluster", "create", args...)
	return err
}

// DestroyPurge takes a cluster down and removes its record too, and says
// what it removed (nothing, for a name that had nothing). The perf runner
// uses fixed names per session and csb's create resumes a cluster of the
// same name, so what is left of an earlier session has to go entirely.
func (c *CLI) DestroyPurge(ctx context.Context) ([]string, error) {
	env, err := c.callLong(ctx, "cluster", "destroy", "--purge")
	if err != nil {
		return nil, err
	}
	var data struct {
		Removed []string `json:"removed"`
	}
	_ = json.Unmarshal(env.Data, &data)
	return data.Removed, nil
}

// DescribeRaw returns `cluster describe` as csb printed it, for a results
// directory that keeps the reproducible artifact beside the numbers.
func (c *CLI) DescribeRaw(ctx context.Context) (json.RawMessage, error) {
	env, err := c.call(ctx, "cluster", "describe")
	if err != nil {
		return nil, err
	}
	return env.Data, nil
}

// RunFor is Run with the bound the caller names, on both clocks: this
// package's deadline and csb's own --timeout. A measured pass is as long as
// its budget says, and the default two minutes would cut the long ones with
// a kill that reads like the node failing.
func (n *Node) RunFor(ctx context.Context, script string, timeout time.Duration) (exec.Result, error) {
	bounded := *n.CLI
	bounded.Timeout = timeout + 30*time.Second
	env, err := bounded.call(ctx, "node", "exec", n.Name, "--timeout", timeout.String(), "--", script)
	if err != nil {
		return exec.Result{}, err
	}
	return oneResult(env, n)
}

// Home is csb's state root, resolved the way csb resolves it: $CSB_HOME, else
// ~/.local/share/csb. The perf runner needs it because a node's /work is a
// directory under it on this host (clusters/<name>/work), and that is how
// files get in and out of a node -- csb has no transfer verb (see Put).
func Home() string {
	if h := os.Getenv("CSB_HOME"); h != "" {
		return h
	}
	base, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".csb")
	}
	return filepath.Join(base, ".local", "share", "csb")
}

// WorkDir is the host path a cluster's /work is bind-mounted from.
func WorkDir(cluster string) string {
	return filepath.Join(Home(), "clusters", cluster, "work")
}

func oneResult(env *envelope, n *Node) (exec.Result, error) {
	var byNode map[string]execResult
	if jerr := json.Unmarshal(env.Data, &byNode); jerr != nil {
		return exec.Result{}, fmt.Errorf("sandbox: %s: cannot read the exec result: %w", n.Describe(), jerr)
	}
	if len(byNode) != 1 {
		return exec.Result{}, fmt.Errorf("sandbox: %q resolved to %d node(s) in cluster %q; a channel is one node",
			n.Name, len(byNode), n.CLI.Cluster)
	}
	for _, r := range byNode {
		return exec.Result{Stdout: r.Stdout, Stderr: r.Stderr, ExitCode: r.Exit}, nil
	}
	return exec.Result{}, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
