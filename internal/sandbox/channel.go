package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
)

// Node is an exec.Channel that reaches one node of a sandbox cluster.
//
// The fourth implementation of the contract, beside Local, SSH and a slot's.
// The suite does not learn a fourth way to run something: it is handed one of
// these and behaves as it always did (contract C3).
//
// # Why not SSH
//
// Because the nodes do not run sshd. cluster-sandbox says so in the note it
// attaches to the conf this channel is built from, and it is not an oversight:
// a node is a container (or, with a second backend, a namespace), and the way in
// is the backend's own exec rather than a daemon listening on a port that would
// have to be published, authenticated and cleaned up.
//
// # The profile is not prepended, and that is the difference from SSH
//
// exec.SSH puts exec.Profile in front of every script, because an ssh exec
// session reads no profile and a case would otherwise find none of $CUBRID,
// $CTP_HOME or the PATH a QA machine is set up with. A sandbox node is not that
// situation: csb sets the engine's environment on the node and runs the command
// through a login shell, so sourcing a ~/.bash_profile that belongs to the host
// would import the wrong machine's setup, not the missing one.
type Node struct {
	// CLI is the cluster this node belongs to.
	CLI *CLI
	// Name is the node, as `describe --format ctp` wrote it into ssh.host. It is
	// passed to csb as a selector, so a role ("master") works here too.
	Name string
}

// NewNode returns the channel for one node of a cluster.
func NewNode(cli *CLI, name string) *Node { return &Node{CLI: cli, Name: name} }

func (n *Node) Describe() string { return "csb:" + n.CLI.Cluster + "/" + n.Name }

// Close releases nothing: a call is a subprocess that has already exited, and
// the cluster outlives this run. Tearing it down here would destroy an
// environment somebody else provisioned and may still be using.
func (n *Node) Close() error { return nil }

// execResult is one node's entry in `node exec`'s data.
type execResult struct {
	Exit   int    `json:"exit"`
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

// Run executes a script on the node.
//
// The whole script goes as ONE argument after `--`. csb joins what follows with
// spaces, so a script passed in pieces would come back with its newlines turned
// into spaces -- and a shell script whose line breaks became spaces is a
// different program that usually still runs.
//
// A non-zero exit is carried in the Result and is not an error, which is the
// contract and is also what csb does: `node exec` exits 0 with a
// `remote_exit_nonzero` note when the command ran and failed. The two agree, so
// nothing here has to reconstruct the distinction from an exit status.
func (n *Node) Run(ctx context.Context, script string) (exec.Result, error) {
	env, err := n.CLI.call(ctx, "node", "exec", n.Name, "--", script)
	if err != nil {
		return exec.Result{}, err
	}
	var byNode map[string]execResult
	if jerr := json.Unmarshal(env.Data, &byNode); jerr != nil {
		return exec.Result{}, fmt.Errorf("sandbox: %s: cannot read the exec result: %w", n.Describe(), jerr)
	}
	// A selector that resolved to nothing is not a command that produced no
	// output: it is a node this run believes in and the cluster does not.
	if len(byNode) == 0 {
		return exec.Result{}, fmt.Errorf("sandbox: %s resolved to no node in cluster %q",
			n.Name, n.CLI.Cluster)
	}
	// One entry is the normal case. More than one means the name was a selector
	// matching several, which would make "the result" a choice this package is
	// not entitled to make.
	if len(byNode) > 1 {
		names := make([]string, 0, len(byNode))
		for k := range byNode {
			names = append(names, k)
		}
		return exec.Result{}, fmt.Errorf("sandbox: %q selects %d nodes (%s); a channel is one node",
			n.Name, len(byNode), strings.Join(names, ", "))
	}
	for _, r := range byNode {
		return exec.Result{Stdout: r.Stdout, Stderr: r.Stderr, ExitCode: r.Exit}, nil
	}
	return exec.Result{}, nil
}

// Put and Get are refused, and say what would have to exist instead.
//
// csb has no file-transfer verb. What it has is ADR-002 operation 11 --
// host-side access to a node's database directory -- which is a path on this
// machine rather than a copy across a channel, and is how seeding and `node
// logs` already work over there. Implementing Put as a base64 blob squeezed
// through `node exec` would work for small files and fail silently at whatever
// size the argument list stops fitting, which is worse than not having it.
//
// Nothing in this runner calls these. The two suites that run against a topology
// move files with the case's own scripts, and this refusal is what a third
// caller should meet rather than a transfer that works until it does not.
func (n *Node) Put(ctx context.Context, local, remote string) error {
	return fmt.Errorf("sandbox: %s cannot copy %s to %s: csb has no file-transfer verb. "+
		"Use the node's host-side directory (cluster-sandbox ADR-002 operation 11)",
		n.Describe(), local, remote)
}

// Get is Put's direction, refused for Put's reason.
func (n *Node) Get(ctx context.Context, remote, local string) error {
	return fmt.Errorf("sandbox: %s cannot copy %s to %s: csb has no file-transfer verb. "+
		"Use the node's host-side directory (cluster-sandbox ADR-002 operation 11)",
		n.Describe(), remote, local)
}
