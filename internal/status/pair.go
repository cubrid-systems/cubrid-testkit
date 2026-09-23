package status

import "time"

// The topology panel: what the run is measuring against, while it measures.
//
// # Why this page needs one the other suites do not
//
// `sql` and `shell` ask one node a question and read the answer. Their page
// therefore shows the run: slots, cases, rate, and the machine they compete
// for. An HA run's verdict is not about a node, it is about a *pair* -- the
// oracle is the two of them disagreeing -- so the page has to show the thing
// the verdict is about, and the machine panel does not: both nodes of a
// rootless pair are processes on the host this page is already drawing.
//
// What goes in it is decided by what has actually gone wrong. Every entry
// below is a thing that cost this project a run or a wrong conclusion:
//
//   - **roles and states.** A run against a pair that is not serving reports
//     `wait_timeout` per case, honestly and uselessly, for as long as nobody
//     looks. Four times on 2026-09-22 every CUBRID process on the host was
//     stopped by something outside the run, and the page would have said so
//     in the second it happened.
//   - **fail_counter, per node.** The suite repairs a stranded object by
//     making the master issue a DROP the slave will accept, and the CREATE in
//     front of it is *meant* to fail there -- which moves the counter. A
//     reader who meets a non-zero counter has to be able to see it move while
//     the run explains itself, rather than find it afterwards and blame the
//     engine.
//   - **apply and copy lag.** The difference between "the slave is behind" and
//     "the two databases differ" is the whole oracle, and the wait that
//     separates them is the one thing this suite refuses to do with a sleep.
//   - **the faults in force.** Group B puts them there on purpose, and a
//     condition left on after a measurement is the next run's mystery.
//   - **the artifact.** Every finding in evidence/ha carries a *Trees* line
//     naming the engine build and how the pair was made. It was written from
//     memory each time. This is where it comes from instead.
type Pair struct {
	Cluster string `json:"cluster"`
	DB      string `json:"db,omitempty"`
	Backend string `json:"backend,omitempty"`
	Image   string `json:"image,omitempty"`
	Network string `json:"network,omitempty"`
	// Ping is the witness and how it is checked, together, because one without
	// the other says nothing: `icmp` to nowhere is not a witness.
	Ping string `json:"ping,omitempty"`
	// Engine is the build string the nodes run, which is what a finding cites.
	Engine string     `json:"engine,omitempty"`
	Nodes  []PairNode `json:"nodes"`
	// Faults are the conditions csb has in force, one line each.
	Faults []string `json:"faults,omitempty"`
	// Note says why the reading is not what it should be -- the cluster could
	// not be reached, or is not serving -- and is empty when it is.
	Note string `json:"note,omitempty"`
	// Age is how many seconds ago this was sampled. A stale panel that says so
	// is useful; one that looks live is not.
	Age int `json:"age"`
}

// PairNode is one node's line.
type PairNode struct {
	Name  string `json:"name"`
	Live  bool   `json:"live"`
	Role  string `json:"role"`
	State string `json:"state"`
	// Built is what the node was created as, which is how an inverted topology
	// is told from an original one: Role answers "now", this answers "then".
	Built string `json:"built,omitempty"`
	// Applied and Copied are pages, Fail is the applier's own error count.
	Applied  int64 `json:"applied,omitempty"`
	Copied   int64 `json:"copied,omitempty"`
	ApplyLag int64 `json:"applyLag"`
	CopyLag  int64 `json:"copyLag"`
	Fail     int64 `json:"fail"`
}

// Pair records the topology the run is measuring against. Safe on a nil Board,
// like everything else here, and safe to call as often as the caller samples.
func (b *Board) Pair(p *Pair) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pair = p
	b.pairAt = time.Now()
}

// pairView is the panel as the page reads it, with the age filled in at
// snapshot time rather than at sample time.
func (b *Board) pairView() *Pair {
	if b.pair == nil {
		return nil
	}
	p := *b.pair
	if !b.pairAt.IsZero() {
		p.Age = int(time.Since(b.pairAt).Seconds())
	}
	return &p
}
