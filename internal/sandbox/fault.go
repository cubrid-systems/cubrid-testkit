package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// The fault verbs, which are how group B asks its question.
//
// # Why this file changes what this package is
//
// Everything else here reads a cluster or runs a command on one. These four
// change it: they cut a network, make two masters, and put it back. The
// package comment says a run "attaches to a cluster somebody already stood up,
// so every call this package makes is a read or an exec" -- that was true of
// group A and is no longer true here. Standing a topology **up** is still
// cluster-sandbox's (ADR-014); disturbing one that is already up is the
// suite's, because the disturbance is the thing under test
// (design/module-ha.md §4, P3-P6).
//
// # Why they are thin
//
// Not one of these decides anything. csb owns what a partition is, which
// mechanism is which, and how a split brain is produced on this configuration;
// this file names the verb and reads the answer. A wrapper that added a policy
// -- a retry, a default flavour, a wait of its own -- would be a second model
// of the fault, and the reason the integration is a subprocess and an artifact
// (ADR-001 Consequence 4) is that there is to be only one.

// Fault is a condition csb has put on a cluster and can take off again.
//
// Cut names the nodes the target can no longer reach, which is not the same as
// the target: `fault partition slave` cuts the master's route to the slave, so
// the target is the slave and the cut is the master. A caller that reports
// only one of them has said half of what happened.
type Fault struct {
	Kind      string    `json:"kind"`
	Target    string    `json:"target"`
	Mechanism string    `json:"mechanism"`
	Cut       []string  `json:"cut"`
	Since     time.Time `json:"since"`
}

// String is the one-line form a verdict or a log needs.
func (f Fault) String() string {
	s := f.Kind
	if f.Target != "" {
		s += " " + f.Target
	}
	if f.Mechanism != "" {
		s += " (" + f.Mechanism + ")"
	}
	if len(f.Cut) > 0 {
		s += ", cut from " + strings.Join(f.Cut, ", ")
	}
	return s
}

// Partition makes the selected node unreachable and returns the condition.
//
// mechanism is csb's word and both of its values are real: `blackhole` removes
// the route and `drop` leaves the route and discards the packets. They are
// different engine code paths -- P3 is explicit that both must be expressible
// -- so this does not default to one and hide the other. Empty takes csb's
// default.
func (c *CLI) Partition(ctx context.Context, selector, mechanism string) (Fault, error) {
	args := []string{selector}
	if m := strings.TrimSpace(mechanism); m != "" {
		args = append(args, "--mechanism", m)
	}
	env, err := c.call(ctx, "fault", "partition", args...)
	if err != nil {
		return Fault{}, err
	}
	var data struct {
		Cut       []string `json:"cut"`
		Mechanism string   `json:"mechanism"`
		Target    string   `json:"target"`
	}
	if jerr := json.Unmarshal(env.Data, &data); jerr != nil {
		return Fault{}, fmt.Errorf("sandbox: reading the partition csb made: %w", jerr)
	}
	return Fault{
		Kind:      "partition",
		Target:    data.Target,
		Mechanism: data.Mechanism,
		Cut:       data.Cut,
		Since:     time.Now().UTC(),
	}, nil
}

// SplitBrain is two masters, on request, and what the group said while it
// happened.
//
// P4 is that split brain is reachable **on purpose**: a state a test can
// create and must detect, rather than one it hopes never to meet. Reaching it
// is configuration-dependent -- a cluster whose ping host survives the
// partition gets there in seconds, one without ping hosts gets there another
// way -- and which of those this cluster produces is csb's to know. flavour
// empty asks for whichever this configuration gives.
//
// CancelReason is the engine's own sentence about why it did not fail back,
// kept verbatim because it is evidence: it names the mechanism in the engine's
// words rather than in this suite's.
type SplitBrain struct {
	Flavour      string `json:"flavour"`
	Masters      int    `json:"masters"`
	Partitioned  string `json:"partitioned"`
	CancelReason string `json:"cancel_reason"`
}

// SplitBrain drives the cluster to two masters and reports what it reached.
//
// wait bounds the engine's part, not csb's: zero leaves csb's own default. The
// call itself is bounded by the CLI timeout like every other, and a split brain
// that has not arrived by then is an error rather than a silent nothing --
// P4's "any test which reaches it unintentionally fails" needs the state to be
// unambiguous in both directions.
func (c *CLI) SplitBrain(ctx context.Context, flavour string, wait time.Duration) (SplitBrain, error) {
	var args []string
	if f := strings.TrimSpace(flavour); f != "" {
		args = append(args, "--flavour", f)
	}
	if wait > 0 {
		args = append(args, "--wait", wait.String())
	}
	env, err := c.call(ctx, "fault", "splitbrain", args...)
	if err != nil {
		return SplitBrain{}, err
	}
	var sb SplitBrain
	if jerr := json.Unmarshal(env.Data, &sb); jerr != nil {
		return SplitBrain{}, fmt.Errorf("sandbox: reading the split brain csb made: %w", jerr)
	}
	if sb.Masters < 2 {
		return sb, fmt.Errorf("sandbox: asked for a split brain and got %d master(s)", sb.Masters)
	}
	return sb, nil
}

// Faults is what is in force now.
func (c *CLI) Faults(ctx context.Context) ([]Fault, error) {
	env, err := c.call(ctx, "fault", "ls")
	if err != nil {
		return nil, err
	}
	var data struct {
		Faults []Fault `json:"faults"`
	}
	if jerr := json.Unmarshal(env.Data, &data); jerr != nil {
		return nil, fmt.Errorf("sandbox: reading the faults in force: %w", jerr)
	}
	return data.Faults, nil
}

// ClearFaults reverses the conditions on a selector, or on the whole cluster
// when the selector is empty, and returns what it took off.
//
// Clearing is not recovery and csb says so in a note of its own: the network is
// restored and the topology afterwards may be healthy and inverted. So a caller
// that heals a partition has not thereby restored the roles, and a test that
// wants the original master back has to say so and wait for it. That
// distinction is the whole of P5 -- the window after the heal is where the
// divergence is made.
func (c *CLI) ClearFaults(ctx context.Context, selector string) ([]Fault, error) {
	var args []string
	if s := strings.TrimSpace(selector); s != "" {
		args = append(args, s)
	}
	env, err := c.call(ctx, "fault", "clear", args...)
	if err != nil {
		return nil, err
	}
	var data struct {
		Cleared []Fault `json:"cleared"`
	}
	if jerr := json.Unmarshal(env.Data, &data); jerr != nil {
		return nil, fmt.Errorf("sandbox: reading what csb cleared: %w", jerr)
	}
	return data.Cleared, nil
}
