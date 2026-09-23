// Package sandbox drives cluster-sandbox, the tool that provisions the
// multi-node topologies this runner tests against.
//
// The two projects meet where ADR-014 drew the line. "The system under test has
// a topology; the runner does not have a fleet" -- so standing a master and a
// slave up is not this runner's job, and it does not become this runner's job by
// being needed. cluster-sandbox owns the environment; this package asks it for
// one and runs cases on it.
//
// # The form of the integration
//
// A subprocess and an artifact, never a library (ADR-001 Consequence 4). `csb`
// is invoked, `--json` is read, and nothing links against it. Two reasons and
// either alone would decide it: the extension is pinned as a submodule and has
// to be able to move without this repository recompiling, and its JSON envelope
// is a stated contract where its Go packages are not.
//
// # Why the CTP fragment and not the JSON describe
//
// `csb cluster describe --format ctp` renders the same artifact as an
// `ha_repl.conf` fragment -- `env.<instance>.master.ssh.host`, `.slave.ssh.host`,
// `ha_db_list`, and the engine parameters. Those key names are CTP's frozen
// surface (ADR-003), which this runner already parses and already turns into
// topology.Instance. Reading the fragment therefore adds no parser and no second
// model of what a node is.
//
// What changes is the transport, and cluster-sandbox says so itself in the note
// it attaches to that output:
//
//	the ssh.host values are container names: these nodes run no sshd and
//	publish no port, so the frozen key names are filled for a docker-exec
//	Channel (cubrid-testkit ADR-014)
//
// So `ssh.host` names a node, `Node` is the channel that reaches it, and the
// frozen key keeps its meaning of "where this instance is" while ceasing to mean
// "an address sshd answers on".
package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// DefaultBin is the tool's name on PATH. It is a binary the operator installs,
// not something this repository builds: the submodule pins which revision the
// evidence was produced against, and `go build ./...` here must not depend on it.
const DefaultBin = "csb"

// BinEnv overrides the binary, for a checkout built somewhere of its own.
const BinEnv = "TESTKIT_CSB"

// DefaultTimeout bounds one csb call. Provisioning verbs are slower than this
// and are not called from here: a run attaches to a cluster somebody already
// stood up, so every call this package makes is a read or an exec.
//
// It is not large enough for every case in the sql corpus. `_09_partition` holds
// statements that take minutes on their own -- one case creates a table with
// 2,048 partitions -- and a run over `_01_object` hit this bound eight times.
// That is a real limit, and a caller should be able to raise it (`CLI.Timeout`);
// what it must not do is disguise it, which is what `ErrTimedOut` below is for.
//
// Provisioning is the exception and has a bound of its own. `cluster create`
// builds an image if the recipe changed, creates a database and seeds a slave
// from it; `cluster destroy` waits for servers to flush. Neither belongs under a
// bound chosen for reads.
const DefaultTimeout = 2 * time.Minute

// ProvisionTimeout bounds `cluster create` and `cluster destroy`. Measured
// rather than guessed: creating a pair on this machine takes about a minute
// when the image is already built, and the first create after a recipe change
// takes several.
const ProvisionTimeout = 15 * time.Minute

// ErrTimedOut is a call that ran past this package's own bound and was killed
// for it. It is a distinct error because the remedy is distinct: the node did
// not fail and the cluster is not unreachable -- the work was longer than the
// caller allowed. Test with errors.Is.
var ErrTimedOut = errors.New("sandbox: the call outlived its timeout")

// CLI is one cluster, reached through the csb command.
type CLI struct {
	// Bin is the command. Empty means BinEnv, then DefaultBin.
	Bin string
	// Cluster is the --cluster every call carries. Required.
	Cluster string
	// Timeout bounds one call. Zero means DefaultTimeout.
	Timeout time.Duration
}

// Bind returns the CLI for a cluster.
func Bind(cluster string) *CLI { return &CLI{Cluster: cluster} }

func (c *CLI) bin() string {
	if c.Bin != "" {
		return c.Bin
	}
	if b := strings.TrimSpace(os.Getenv(BinEnv)); b != "" {
		return b
	}
	return DefaultBin
}

func (c *CLI) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return DefaultTimeout
}

// Note is one entry of the envelope's notes: a code, a severity and a sentence.
// They are not prose to print at a reader -- the code is what a caller matches
// on, which is why the field is kept rather than flattened into a string.
type Note struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// envelope is csb's stated contract: `schema` is "csb/v1" and everything a
// command produces is under `data`.
//
// The schema is checked rather than assumed. A tool that has moved to a second
// version will still print JSON, and a reader that shrugged at the version would
// mis-read it quietly -- which is the failure this whole integration is arranged
// to avoid.
type envelope struct {
	Schema  string          `json:"schema"`
	Command string          `json:"command"`
	OK      bool            `json:"ok"`
	Data    json.RawMessage `json:"data"`
	Notes   []Note          `json:"notes"`
}

// Schema is the envelope version this package reads.
const Schema = "csb/v1"

// call runs one csb command and returns its envelope.
//
// The cluster and --json go immediately after the noun and the verb, and not at
// the end, because `node exec` ends in `-- <command>` and everything after that
// separator belongs to the command rather than to csb. Appending them would have
// handed the node a script with `--cluster hadb --json` stuck on the end of it,
// and the node would have run it.
//
// A non-zero exit is not by itself an error: csb reports a command that ran and
// failed inside the envelope, and only a tool that could not run at all leaves
// nothing to read. So the output is parsed first and the exit status is used to
// explain a parse that produced nothing.
func (c *CLI) call(ctx context.Context, noun, verb string, rest ...string) (*envelope, error) {
	if strings.TrimSpace(c.Cluster) == "" {
		return nil, fmt.Errorf("sandbox: no cluster named")
	}
	return c.callRaw(ctx, noun, verb, rest...)
}

// callRaw is call without the cluster requirement, so that the one verb which
// is about the machine rather than about a cluster can use the same envelope
// reading, the same timeout and the same error wording as everything else.
func (c *CLI) callRaw(ctx context.Context, noun, verb string, rest ...string) (*envelope, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()

	argv := []string{noun, verb}
	if strings.TrimSpace(c.Cluster) != "" {
		argv = append(argv, "--cluster", c.Cluster)
	}
	argv = append(append(argv, "--json"), rest...)
	cmd := exec.CommandContext(ctx, c.bin(), argv...)
	var out, errOut strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errOut
	runErr := cmd.Run()

	// A deadline this package set killed the child, and `exec` reports that as
	// `signal: killed` -- which reads exactly like something outside the run
	// killing it. Said plainly instead, with the bound named: a caller deciding
	// what to do needs to know the node was fine and the clock was not.
	if runErr != nil && ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("%w: %s %s %s did not finish within %s",
			ErrTimedOut, c.bin(), noun, verb, c.timeout())
	}

	var env envelope
	if jerr := json.Unmarshal([]byte(out.String()), &env); jerr != nil {
		if runErr != nil {
			return nil, fmt.Errorf("sandbox: %s %s %s: %w: %s",
				c.bin(), noun, verb, runErr, strings.TrimSpace(errOut.String()))
		}
		return nil, fmt.Errorf("sandbox: %s %s %s printed no envelope: %v: %s",
			c.bin(), noun, verb, jerr, strings.TrimSpace(out.String()))
	}
	if env.Schema != Schema {
		return nil, fmt.Errorf("sandbox: %s speaks %q and this runner reads %q",
			c.bin(), env.Schema, Schema)
	}
	if !env.OK {
		return &env, fmt.Errorf("sandbox: %s %s %s: %s",
			c.bin(), noun, verb, notesLine(env.Notes))
	}
	return &env, nil
}

// notesLine is what a failed call says. The codes are kept alongside the
// messages: a caller reading a log needs the sentence, and a caller deciding
// what to do needs the code.
func notesLine(notes []Note) string {
	if len(notes) == 0 {
		return "no notes"
	}
	parts := make([]string, 0, len(notes))
	for _, n := range notes {
		parts = append(parts, n.Code+": "+n.Message)
	}
	return strings.Join(parts, "; ")
}

// Available reports whether the tool can be found and answers, and says why not
// when it cannot.
//
// Called before a run rather than at the first case: a topology that is not
// there is a setup failure, and finding out at case 400 wastes the 399 before it.
func (c *CLI) Available(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, c.bin(), "--version").Run(); err != nil {
		return fmt.Errorf("sandbox: %s does not answer --version (%v). "+
			"Build it from extensions/cluster-sandbox and put it on PATH, or set %s",
			c.bin(), err, BinEnv)
	}
	return nil
}

// callLong is `call` with the provisioning bound rather than the read bound.
func (c *CLI) callLong(ctx context.Context, noun, verb string, rest ...string) (*envelope, error) {
	long := *c
	if long.Timeout < ProvisionTimeout {
		long.Timeout = ProvisionTimeout
	}
	return long.call(ctx, noun, verb, rest...)
}

// callNoCluster is for the one question that is about the machine rather than
// about a cluster: `cluster ls`. It exists because `call` requires a cluster and
// is right to -- every other verb is meaningless without one.
func (c *CLI) callNoCluster(ctx context.Context, noun, verb string, rest ...string) (*envelope, error) {
	anon := *c
	anon.Cluster = ""
	return anon.callRaw(ctx, noun, verb, rest...)
}

// callLongNoCluster is the provisioning bound without the cluster requirement,
// for the one destructive verb that selects its own targets.
func (c *CLI) callLongNoCluster(ctx context.Context, noun, verb string, rest ...string) (*envelope, error) {
	long := *c
	long.Cluster = ""
	if long.Timeout < ProvisionTimeout {
		long.Timeout = ProvisionTimeout
	}
	return long.callRaw(ctx, noun, verb, rest...)
}

// decode reads the envelope's data into v.
func (e *envelope) decode(v any) error {
	if len(e.Data) == 0 {
		return fmt.Errorf("sandbox: %s returned no data", e.Command)
	}
	if err := json.Unmarshal(e.Data, v); err != nil {
		return fmt.Errorf("sandbox: %s returned data this runner cannot read: %w", e.Command, err)
	}
	return nil
}
