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
// no business occupying it.
//
// There are two ways to say which cases those are, and only one of them has
// survived a measurement.
//
// By duration (lane_slow_secs) was the first, and it lost: predicted 440 s,
// measured 696. The reasoning below is sound about bandwidth and still wrong
// about selection, because duration picks the I/O-heavy cases and those are the
// ones memory helps most. It is kept because the argument is worth reading and
// the knob is worth having, not because it worked.
//
// By rate (lane_slow_mbps) is the third, and the one the measurements point at.
// It exists because footprint and duration turned out to be unrelated: over
// 2,949 directories measured at their peak, Spearman(footprint, duration) is
// +0.02. Selecting by footprint therefore picks the rate at random, and rate is
// the quantity the disk is bounded by. Measured on this corpus:
//
//	peak      longest   MB/s
//	8,662 MB     17 s   512.5   <- worst possible thing to put on disk
//	6,153 MB     40 s   155.4
//	9,289 MB    160 s    58.1
//	9,093 MB    523 s    17.4   <- same bytes, a thirtieth of the load
//
// The disk delivered 88 MB/s under four concurrent writers, so lane_slow_mb
// cannot tell the last row from the first: it sees similar megabytes.
//
// What the two lanes want is a knapsack -- move as many bytes as possible off
// the tmpfs without asking the disk for more MB/s than it has -- and its greedy
// order falls out of the arithmetic: value over weight is bytes divided by
// bytes-per-second, which is seconds. So take the longest directories first and
// stop when the summed rate reaches the budget. Duration is the right *order*
// even though, on its own, it was the wrong *criterion*.
//
// By footprint (lane_slow_mb) is the second, and it was chosen to relieve the
// bound that actually breaks: the ceiling is made of space. Eighteen cases into
// this corpus, three directories that each hold gigabytes overlap and take
// 18,417 MB of an 18,432 MB ceiling -- and they overlap precisely because they
// are also long, so longest-first starts them together. Space is not a proxy
// for anything here. It is the quantity the ceiling counts. Measured, the fixed cost memory removes is about
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
	// MB is the footprint each lane was given, summed over its directories, and
	// FastBiggest is the largest single directory left in memory. Neither is the
	// fast lane's peak -- that is whatever subset happens to overlap -- but the
	// sum is its ceiling and the largest is its floor, so an operator choosing a
	// threshold against scenario_ram_mb has both ends of the range.
	FastMB, SlowMB, FastBiggest int
}

// planLanes decides the split from what a previous run measured.
//
// A directory goes to the slow lane when it holds at least slowMB, or when its
// longest case took at least slowSecs. Either criterion alone is enough; a run
// that sets both is asking for the union, which is the only combination that
// makes sense when each names a different reason a case should not be in memory.
//
// By directory and not by case because a directory is the unit a slot owns --
// see dispatch.Queue -- and by its longest case because the whole directory
// lands in one lane.
//
// The slots are divided in proportion to the case-seconds each lane holds, which
// is what makes the two lanes finish together: a lane with 40% of the work and
// 40% of the slots takes the same wall clock as the other. Both lanes get at
// least one slot, and a run whose split would starve a lane keeps every slot in
// the fast one.
func planLanes(cases []string, took map[string]time.Duration, held map[string]int, slowSecs, slowMB, slowMBps, slots int) (laneSplit, error) {
	if slowSecs <= 0 && slowMB <= 0 && slowMBps <= 0 {
		return laneSplit{}, nil
	}
	if slowSecs > 0 && len(took) == 0 {
		return laneSplit{}, fmt.Errorf("lane_slow_secs needs durations: run once with case_plan set, or unset lane_slow_secs")
	}
	if slowMB > 0 && len(held) == 0 {
		return laneSplit{}, fmt.Errorf("lane_slow_mb needs footprints: run once with case_sizes set, or unset lane_slow_mb")
	}
	if slowMBps > 0 && (len(held) == 0 || len(took) == 0) {
		return laneSplit{}, fmt.Errorf("lane_slow_mbps needs both footprints and durations: run once with case_sizes and case_plan set, or unset lane_slow_mbps")
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

	// The rate budget, spent longest-first: that is the greedy order for
	// "as many bytes as possible within a bandwidth", because bytes over
	// bytes-per-second is seconds. A directory with no footprint asks nothing of
	// the disk and is left where it is.
	byRate := map[string]bool{}
	if slowMBps > 0 {
		dirs := make([]string, 0, len(worst))
		for dir := range worst {
			if held[dir] > 0 && worst[dir] > 0 {
				dirs = append(dirs, dir)
			}
		}
		sort.Slice(dirs, func(i, j int) bool { return worst[dirs[i]] > worst[dirs[j]] })
		budget := float64(slowMBps)
		for _, dir := range dirs {
			rate := float64(held[dir]) / worst[dir]
			if rate > budget {
				continue // no room for this one; a slower one may still fit
			}
			byRate[dir] = true
			budget -= rate
		}
	}

	out := laneSplit{byDir: map[string]dispatch.Lane{}}
	for dir, w := range worst {
		slow := slowSecs > 0 && w >= float64(slowSecs)
		if slowMB > 0 && held[dir] >= slowMB {
			slow = true
		}
		if byRate[dir] {
			slow = true
		}
		if slow {
			out.byDir[dir] = dispatch.LaneSlow
			out.SlowWork += total[dir]
			out.SlowCases += count[dir]
			out.SlowMB += held[dir]
		} else {
			out.byDir[dir] = dispatch.LaneFast
			out.FastWork += total[dir]
			out.FastCases += count[dir]
			out.FastMB += held[dir]
			if held[dir] > out.FastBiggest {
				out.FastBiggest = held[dir]
			}
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
func (l laneSplit) describe(slowSecs, slowMB int) string {
	at := ""
	switch {
	case slowSecs > 0 && slowMB > 0:
		at = fmt.Sprintf("at %ds or %d MB", slowSecs, slowMB)
	case slowMB > 0:
		at = fmt.Sprintf("at %d MB", slowMB)
	default:
		at = fmt.Sprintf("at %ds", slowSecs)
	}
	return fmt.Sprintf(
		"[INFO] lanes %s: tmpfs %d slots for %d cases (%.0f case-s, %d MB over %d dirs, biggest %d MB), "+
			"disk %d slots for %d cases (%.0f case-s, %d MB)",
		at, l.FastSlots, l.FastCases, l.FastWork, l.FastMB, l.FastCases, l.FastBiggest,
		l.SlowSlots, l.SlowCases, l.SlowWork, l.SlowMB)
}

// slowest names the directories the slow lane took, biggest first, for the
// operator who wants to see what the threshold actually selected.
//
// Ordered by footprint when there is one, because that is what the threshold is
// now usually chosen against, and by duration otherwise.
func (l laneSplit) slowest(took map[string]time.Duration, held map[string]int, cases []string, n int) []string {
	type row struct {
		name string
		secs float64
		mb   int
	}
	seen := map[string]bool{}
	var rows []row
	for _, c := range cases {
		split, err := Split(c)
		if err != nil || l.byDir[split.Dir] != dispatch.LaneSlow || seen[split.Dir] {
			continue
		}
		seen[split.Dir] = true
		rows = append(rows, row{split.Dir, took[c].Seconds(), held[split.Dir]})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].mb != rows[j].mb {
			return rows[i].mb > rows[j].mb
		}
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
		out = append(out, fmt.Sprintf("%d MB %.0fs %s", r.mb, r.secs, r.name))
	}
	return out
}
