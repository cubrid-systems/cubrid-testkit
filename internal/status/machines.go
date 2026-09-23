package status

import (
	"os"
	"sort"
	"time"
)

// The machines a run competes for, and what is standing on each of them.
//
// # Why this is a list
//
// It was one struct, because a run was one machine's business. That is still
// true of `shell`, `sql` and `isolation` -- ADR-014 says the runner runs on one
// machine and means it. It stopped being true of the page the moment a run
// could drive several pairs: a pair is a topology, a topology can span machines
// (`cluster-sandbox` ADR-002), and the question "why is this slow" then has one
// answer per machine rather than one answer.
//
// Today the list has one entry and that is not a placeholder -- it is where
// every cluster actually is. What changes when csb can place a node elsewhere
// is the number of rows, not the shape of the page.
//
// # Why the clusters are on it
//
// Because the number an operator needs is per cluster, and the machine panel
// was already reporting a filesystem. A filesystem at 98% does not say which
// pair to remove. Eleven pairs on this host reached 53 GB, every one of them
// standing for a good reason at the time, and the page could have said so at
// any point in the two days it took.

// MachineCluster is one cluster as a machine's row lists it.
type MachineCluster struct {
	Name       string `json:"name"`
	Containers int    `json:"containers"`
	Bytes      int64  `json:"bytes"`
	// Run is the label a testkit run claimed it with, empty when nobody did.
	// Shown because "which of these is mine" is the first question anyone asks
	// of a list of eleven.
	Run string `json:"run,omitempty"`
	// Mine marks the clusters this run is driving, so the row reads as the
	// run's own footprint and not as an inventory it happens to sit beside.
	Mine bool `json:"mine,omitempty"`
}

// Machine is one machine's row: what it is, how it is doing, and what is on it.
type Machine struct {
	Name string `json:"name"`
	// Local marks the machine this process is on. Its numbers come from /proc
	// directly; another machine's would have to arrive over a channel, and the
	// distinction is worth showing rather than hiding, because a remote reading
	// can be stale in a way a local one cannot.
	Local    bool             `json:"local"`
	Stats    machineView      `json:"stats"`
	Clusters []MachineCluster `json:"clusters,omitempty"`
	// Note is what this row could not find out -- a machine that would not
	// answer, most often. Said rather than left as zeroes, which read as idle.
	Note string `json:"note,omitempty"`
	// AgeMs is how long ago this row was sampled, filled at snapshot time.
	AgeMs int64 `json:"ageMs,omitempty"`
}

// ThisMachine is the name this host calls itself, and the name csb writes into
// a node's artifact when it creates one here. The two have to agree or a
// cluster cannot be put on a row.
func ThisMachine() string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "this machine"
}

// MachineRow records what is known about one machine.
//
// Keyed by name and replaced whole, like a pair's panel: a sampler that failed
// halfway should leave the previous answer standing with an age on it rather
// than a half-filled row.
func (b *Board) MachineRow(m Machine) {
	if b == nil || m.Name == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.machines == nil {
		b.machines = map[string]*Machine{}
		b.machineAt = map[string]time.Time{}
	}
	cp := m
	b.machines[m.Name] = &cp
	b.machineAt[m.Name] = time.Now()
}

// machineViews is the section as the page reads it.
//
// The local machine sorts first and the rest by name: the one you are on is the
// one you look at, and after that alphabetical is the only order that does not
// move under you.
func (b *Board) machineViews() []Machine {
	local := Machine{Name: ThisMachine(), Local: true, Stats: b.sampler.snapshot()}
	if b.sampler == nil {
		local.Note = "nothing said which disk this run competes for, so only the cores are known"
	}
	out := []Machine{}
	for name, m := range b.machines {
		cp := *m
		cp.AgeMs = time.Since(b.machineAt[name]).Milliseconds()
		if cp.Name == local.Name {
			// The local row's numbers are this process's own reading; a sampler
			// pushing clusters does not get to overwrite them with zeroes.
			cp.Local, cp.Stats = true, local.Stats
			if cp.Note == "" {
				cp.Note = local.Note
			}
			local = cp
			continue
		}
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return append([]Machine{local}, out...)
}
