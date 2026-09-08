package shellsuite

import (
	"fmt"
	"sort"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/dispatch"
)

// laneSplit is how a run divides its slots and its cases between the two lanes.
//
// The whole idea in one sentence: a case that cannot get speed out of memory has
// no business occupying it. Measured, the fixed cost memory removes is about
// 0.2 s a case -- createdb is 0.24 s on tmpfs against 0.04 s for a copy -- so a
// five-second case gets 4% of itself back and a 195-second case gets 0.1%, while
// holding the ceiling for three minutes. On the six cases of _01_sqlx that were
// measured first, three of the six were 96% of the time.
//
// It relieves two bounds rather than one, and the second is the surprise. The
// disk bound is bandwidth, and bandwidth per case is inversely proportional to
// duration: a 195-second case writing 220 MB writes at 1.1 MB/s, a five-second
// case at 44. Summed over this family's durations, everything on disk demands
// 240 MB/s against the 88 the disk delivered under four concurrent writers --
// which is where the measured four-slot bound came from -- while the cases over
// 30 seconds demand 20. The slow lane is nearly free on disk precisely because
// it holds the cases that are slow.
type laneSplit struct {
	// byDir is which lane each case directory belongs to.
	byDir map[string]dispatch.Lane
	// FastSlots and SlowSlots divide the run's slots.
	FastSlots, SlowSlots int
	// Work is the case-seconds each lane was given, which is what the slot
	// split is proportional to.
	FastWork, SlowWork float64
	// Cases counts them.
	FastCases, SlowCases int
}

// planLanes decides the split from what a previous run measured.
//
// A directory goes to the slow lane when its longest case took at least
// slowSecs. By directory and not by case because a directory is the unit a slot
// owns -- see dispatch.Queue -- and by its longest case because the whole
// directory lands in one lane.
//
// The slots are divided in proportion to the case-seconds each lane holds, which
// is what makes the two lanes finish together: a lane with 40% of the work and
// 40% of the slots takes the same wall clock as the other. Both lanes get at
// least one slot, and a run whose split would starve a lane keeps every slot in
// the fast one.
func planLanes(cases []string, took map[string]time.Duration, slowSecs int, slots int) (laneSplit, error) {
	if slowSecs <= 0 {
		return laneSplit{}, nil
	}
	if len(took) == 0 {
		return laneSplit{}, fmt.Errorf("lanes need durations: run once with case_plan set, or unset lane_slow_secs")
	}
	if slots < 2 {
		return laneSplit{}, fmt.Errorf("lanes need at least two slots, and parallel_slots is %d", slots)
	}

	// The longest case in each directory, and the directory's total.
	worst := map[string]float64{}
	total := map[string]float64{}
	count := map[string]int{}
	for _, c := range cases {
		split, err := Split(c)
		if err != nil {
			continue
		}
		secs := took[c].Seconds()
		total[split.Dir] += secs
		count[split.Dir]++
		if secs > worst[split.Dir] {
			worst[split.Dir] = secs
		}
	}

	out := laneSplit{byDir: map[string]dispatch.Lane{}}
	for dir, w := range worst {
		if w >= float64(slowSecs) {
			out.byDir[dir] = dispatch.LaneSlow
			out.SlowWork += total[dir]
			out.SlowCases += count[dir]
		} else {
			out.byDir[dir] = dispatch.LaneFast
			out.FastWork += total[dir]
			out.FastCases += count[dir]
		}
	}
	work := out.FastWork + out.SlowWork
	if out.SlowCases == 0 || work <= 0 {
		// Nothing is slow enough to be worth a lane of its own. Say so by
		// returning no split rather than by making an empty lane.
		return laneSplit{}, nil
	}
	out.SlowSlots = int(float64(slots)*out.SlowWork/work + 0.5)
	if out.SlowSlots < 1 {
		out.SlowSlots = 1
	}
	if out.SlowSlots > slots-1 {
		out.SlowSlots = slots - 1
	}
	out.FastSlots = slots - out.SlowSlots
	return out, nil
}

// on reports whether a split is worth applying.
func (l laneSplit) on() bool { return len(l.byDir) > 0 && l.SlowSlots > 0 }

// laneOf is which lane a slot index belongs to. The fast slots come first so
// that a run's slot0 is a fast slot whether or not lanes are on.
func (l laneSplit) laneOf(i int) dispatch.Lane {
	if !l.on() {
		return dispatch.LaneAny
	}
	if i < l.FastSlots {
		return dispatch.LaneFast
	}
	return dispatch.LaneSlow
}

// describe is the line the run prints, because a split chosen from a file is a
// decision an operator has to be able to check.
func (l laneSplit) describe(slowSecs int) string {
	return fmt.Sprintf(
		"[INFO] lanes at %ds: tmpfs %d slots for %d cases (%.0f case-s), disk %d slots for %d cases (%.0f case-s)",
		slowSecs, l.FastSlots, l.FastCases, l.FastWork, l.SlowSlots, l.SlowCases, l.SlowWork)
}

// slowest names the directories the slow lane took, longest first, for the
// operator who wants to see what the threshold actually selected.
func (l laneSplit) slowest(took map[string]time.Duration, cases []string, n int) []string {
	type row struct {
		name string
		secs float64
	}
	var rows []row
	for _, c := range cases {
		split, err := Split(c)
		if err != nil || l.byDir[split.Dir] != dispatch.LaneSlow {
			continue
		}
		rows = append(rows, row{c, took[c].Seconds()})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].secs != rows[j].secs {
			return rows[i].secs > rows[j].secs
		}
		return rows[i].name < rows[j].name
	})
	var out []string
	for i, r := range rows {
		if i >= n {
			break
		}
		out = append(out, fmt.Sprintf("%.0fs %s", r.secs, r.name))
	}
	return out
}
