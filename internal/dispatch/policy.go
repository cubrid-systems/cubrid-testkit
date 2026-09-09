package dispatch

import (
	"fmt"
	"sort"
	"strings"
)

// Policy decides which of the waiting cases a free slot may start.
//
// The queue's order is the schedule -- longest first, from the plan -- and it is
// right about makespan. What it does not know is that a slot is not free just
// because it is idle: the run shares a memory ceiling, a machine, and a disk,
// and the order that minimises total time also puts the heaviest cases at the
// front, where N slots start N of them at once.
//
// Measured: at 24 slots the corpus tmpfs went from empty to 25,584 MB of 25,600
// in under three minutes with **zero cases finished**. Nothing was wrong with
// the order. What was missing was anyone asking whether the machine could take
// another one yet.
//
// So the order proposes and a policy disposes. The queue offers candidates in
// its own order and hands out the first the policy admits; a run without a
// policy gets the order alone, which is what it had before.
//
// Two are implemented and they answer different questions. HeavyCap is about the
// start, where a feedback rule cannot help because nothing has been written yet
// and every slot is empty at once. Headroom is about everything after that,
// where the only trustworthy number is the one the machine reports. They compose
// with All.
type Policy interface {
	// Admit reports whether c may start while running is in flight.
	//
	// A policy is never asked to admit the only case: when nothing is running
	// the queue hands one out regardless, because a policy that can refuse
	// everything is a policy that can stop the run.
	Admit(c string, running []string) bool

	// Describe is the line the run prints, because a schedule that differs from
	// the plan's order is a decision an operator has to be able to check.
	Describe() string
}

// All admits a case only when every policy admits it. Nil and empty members are
// skipped, so a caller can build the list without checking what it configured.
func All(ps ...Policy) Policy {
	var kept []Policy
	for _, p := range ps {
		if p != nil {
			kept = append(kept, p)
		}
	}
	if len(kept) == 0 {
		return nil
	}
	if len(kept) == 1 {
		return kept[0]
	}
	return all(kept)
}

type all []Policy

func (a all) Admit(c string, running []string) bool {
	for _, p := range a {
		if !p.Admit(c, running) {
			return false
		}
	}
	return true
}

func (a all) Describe() string {
	parts := make([]string, 0, len(a))
	for _, p := range a {
		parts = append(parts, p.Describe())
	}
	return strings.Join(parts, "; ")
}

// HeavyCap limits how many of the heavy cases run at once.
//
// This is the rule for the start of a run and nothing else can be. A ceiling is
// empty when the first case begins, so a rule that watches the ceiling admits
// every slot at once -- and the order has arranged for those to be the heaviest
// cases in the corpus. By the time the ceiling says stop, the burst has already
// happened.
//
// Heavy is named rather than inferred, because what makes a case heavy depends
// on which resource is scarce. Today it is the corpus's longest cases, which are
// also its largest writers; the caller decides.
type HeavyCap struct {
	heavy map[string]bool
	max   int
}

// NewHeavyCap allows at most max of the named cases to run at once. A max below
// one is treated as one: the alternative is a rule that never admits a heavy
// case, which never finishes the run.
func NewHeavyCap(heavy []string, max int) *HeavyCap {
	if len(heavy) == 0 {
		return nil
	}
	if max < 1 {
		max = 1
	}
	m := make(map[string]bool, len(heavy))
	for _, h := range heavy {
		m[h] = true
	}
	return &HeavyCap{heavy: m, max: max}
}

func (h *HeavyCap) Admit(c string, running []string) bool {
	if h == nil || !h.heavy[c] {
		return true
	}
	n := 0
	for _, r := range running {
		if h.heavy[r] {
			n++
		}
	}
	return n < h.max
}

func (h *HeavyCap) Describe() string {
	if h == nil {
		return ""
	}
	return fmt.Sprintf("at most %d of the %d heaviest cases at once", h.max, len(h.heavy))
}

// Headroom refuses to start anything while a measured resource is above a
// fraction of its limit.
//
// Feedback rather than prediction, and deliberately so: what a case will write
// is not known well enough to plan with. The run measures per-directory
// footprints, and on the full corpus the twenty-four cases at the head of the
// queue measured 377 MB between them while actually filling 25,584. That figure
// is what a directory *held when it retired*, not what it peaked at while
// running, and a ceiling is about the peak.
//
// The machine's own answer needs none of that. It is late -- it can only stop
// the next case, never the ones already running -- which is exactly why HeavyCap
// exists alongside it.
type Headroom struct {
	usage   func() (used, limit int)
	percent int
	what    string
}

// NewHeadroom refuses new cases while usage is at or above percent of the limit.
func NewHeadroom(what string, usage func() (int, int), percent int) *Headroom {
	if usage == nil || percent <= 0 || percent >= 100 {
		return nil
	}
	return &Headroom{usage: usage, percent: percent, what: what}
}

func (h *Headroom) Admit(c string, running []string) bool {
	if h == nil {
		return true
	}
	used, limit := h.usage()
	if limit <= 0 {
		return true
	}
	return used*100 < limit*h.percent
}

func (h *Headroom) Describe() string {
	if h == nil {
		return ""
	}
	return fmt.Sprintf("nothing new while %s is over %d%% full", h.what, h.percent)
}

// Heaviest names the n longest cases in a plan, for HeavyCap.
func Heaviest(took map[string]float64, n int) []string {
	if n < 1 || len(took) == 0 {
		return nil
	}
	names := make([]string, 0, len(took))
	for c := range took {
		names = append(names, c)
	}
	sort.Slice(names, func(i, j int) bool {
		if took[names[i]] != took[names[j]] {
			return took[names[i]] > took[names[j]]
		}
		return names[i] < names[j]
	})
	if n > len(names) {
		n = len(names)
	}
	return names[:n]
}
