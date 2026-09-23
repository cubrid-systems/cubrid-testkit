package sandbox

import "testing"

// A channel that runs a case's SQL is bounded by what the corpus needs, not by
// what a read needs. The corpus's largest case takes about 250 s on a cold
// database, and the two-minute read bound turned it into `case_failed` on every
// run at every level of machine load.
func TestACaseChannelIsBoundedForCasesAndNotForReads(t *testing.T) {
	p := &Pair{Cluster: "hadb", Master: "hadb-n1", Slaves: []string{"hadb-n2"}, cli: Bind("hadb")}

	node, ok := p.MasterChannel().(*Node)
	if !ok {
		t.Fatal("MasterChannel is not a *Node")
	}
	if node.CLI.Timeout != CaseTimeout {
		t.Errorf("master channel is bounded at %v, want CaseTimeout %v", node.CLI.Timeout, CaseTimeout)
	}
	ch, err := p.SlaveChannel(0)
	if err != nil {
		t.Fatal(err)
	}
	if ch.(*Node).CLI.Timeout != CaseTimeout {
		t.Errorf("slave channel is bounded at %v, want CaseTimeout %v", ch.(*Node).CLI.Timeout, CaseTimeout)
	}

	// The reading CLI is untouched: a pair that has stopped serving must be
	// discoverable in seconds, not in ten minutes.
	if p.cli.Timeout != 0 {
		t.Errorf("the cluster's own CLI was modified: %v", p.cli.Timeout)
	}
	if p.cli.timeout() != DefaultTimeout {
		t.Errorf("reads are bounded at %v, want DefaultTimeout %v", p.cli.timeout(), DefaultTimeout)
	}
}

// A caller who set a bound meant it. Overriding it here would silently undo an
// operator's decision, which is the kind of thing a helper must never do.
func TestAnExplicitBoundIsKept(t *testing.T) {
	cli := Bind("hadb")
	cli.Timeout = DefaultTimeout
	p := &Pair{Cluster: "hadb", Master: "hadb-n1", cli: cli}
	if got := p.MasterChannel().(*Node).CLI.Timeout; got != DefaultTimeout {
		t.Errorf("an explicit bound became %v, want it kept at %v", got, DefaultTimeout)
	}
}
