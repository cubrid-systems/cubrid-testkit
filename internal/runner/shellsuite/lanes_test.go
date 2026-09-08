package shellsuite

import (
	"testing"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/dispatch"
)

// corpus builds a case list and its durations from (name, seconds) pairs. Two
// names under the same directory share it, which is the case affinity exists
// for.
func corpus(pairs ...any) ([]string, map[string]time.Duration) {
	var cases []string
	took := map[string]time.Duration{}
	for i := 0; i < len(pairs); i += 2 {
		name := pairs[i].(string)
		secs := pairs[i+1].(int)
		p := "/x/" + name + ".sh"
		if at := len(name) - 2; at > 0 && name[at] == '#' {
			// "dir#a" means case a in directory dir.
			p = "/x/" + name[:at] + "/cases/" + name[at+1:] + ".sh"
		} else {
			p = "/x/" + name + "/cases/" + name + ".sh"
		}
		cases = append(cases, p)
		took[p] = time.Duration(secs) * time.Second
	}
	return cases, took
}

// The threshold selects directories, and the slots follow the work: a lane with
// 40% of the case-seconds gets 40% of the slots, which is what makes the two
// lanes finish together.
func TestTheSlotsFollowTheWork(t *testing.T) {
	// 3 long directories at 100s = 300 case-s, 7 short at 10s = 70 case-s.
	var pairs []any
	for _, n := range []string{"l1", "l2", "l3"} {
		pairs = append(pairs, n, 100)
	}
	for _, n := range []string{"s1", "s2", "s3", "s4", "s5", "s6", "s7"} {
		pairs = append(pairs, n, 10)
	}
	cases, took := corpus(pairs...)

	sp, err := planLanes(cases, took, 30, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !sp.on() {
		t.Fatal("a corpus with three 100-second cases produced no split")
	}
	if sp.SlowCases != 3 || sp.FastCases != 7 {
		t.Errorf("the threshold selected %d slow and %d fast cases", sp.SlowCases, sp.FastCases)
	}
	if sp.SlowWork != 300 || sp.FastWork != 70 {
		t.Errorf("work split is %v/%v, want 300/70", sp.SlowWork, sp.FastWork)
	}
	// 300 of 370 case-seconds is 81%, so 8 of 10 slots.
	if sp.SlowSlots != 8 || sp.FastSlots != 2 {
		t.Errorf("slots split %d fast / %d slow, want 2/8", sp.FastSlots, sp.SlowSlots)
	}
	if sp.FastSlots+sp.SlowSlots != 10 {
		t.Error("the split does not account for every slot")
	}
	// The fast slots come first, so slot0 is a fast slot whether or not lanes
	// are on.
	if sp.laneOf(0) != dispatch.LaneFast || sp.laneOf(9) != dispatch.LaneSlow {
		t.Error("the fast slots are not the first ones")
	}
}

// Neither lane may be starved: a lane with no slots is cases that never run.
func TestNeitherLaneIsStarved(t *testing.T) {
	// One 200s case against 40 short ones: the slow lane's share rounds to zero.
	pairs := []any{"one_long", 200}
	for i := 0; i < 40; i++ {
		pairs = append(pairs, "s"+string(rune('a'+i%26))+string(rune('a'+i/26)), 1)
	}
	cases, took := corpus(pairs...)
	sp, err := planLanes(cases, took, 30, 2)
	if err != nil {
		t.Fatal(err)
	}
	if sp.FastSlots < 1 || sp.SlowSlots < 1 {
		t.Errorf("a lane was starved: %d fast / %d slow", sp.FastSlots, sp.SlowSlots)
	}
}

// A directory lands in one lane, decided by its longest case -- because the
// whole directory goes to one slot and a slot is in one lane.
func TestADirectoryGoesByItsLongestCase(t *testing.T) {
	cases, took := corpus("multi#a", 5, "multi#b", 90, "solo", 5)
	sp, err := planLanes(cases, took, 30, 4)
	if err != nil {
		t.Fatal(err)
	}
	if sp.byDir["/x/multi/cases"] != dispatch.LaneSlow {
		t.Error("a directory holding a 90-second case is not in the slow lane")
	}
	if sp.byDir["/x/solo/cases"] != dispatch.LaneFast {
		t.Error("a directory of one 5-second case is not in the fast lane")
	}
	if sp.SlowCases != 2 {
		t.Errorf("the slow lane took %d cases; both of the directory's should go", sp.SlowCases)
	}
	// And the five-second sibling's seconds count against the slow lane, because
	// that is where it will run.
	if sp.SlowWork != 95 {
		t.Errorf("the slow lane's work is %v, want 95", sp.SlowWork)
	}
}

// Lanes are a policy over a measurement. Without the measurement they are a
// guess, and the run says so rather than guessing.
func TestLanesRefuseWithoutDurations(t *testing.T) {
	cases, _ := corpus("a", 1)
	if _, err := planLanes(cases, nil, 30, 4); err == nil {
		t.Error("lanes were planned with no durations")
	}
	if _, err := planLanes(cases, map[string]time.Duration{"/x/a/cases/a.sh": time.Second}, 30, 1); err == nil {
		t.Error("lanes were planned for one slot")
	}
	// And a threshold nothing reaches is not a split, it is a mistake worth
	// reporting -- an empty slow lane would leave its slots idle all run.
	sp, err := planLanes(cases, map[string]time.Duration{"/x/a/cases/a.sh": time.Second}, 30, 4)
	if err != nil {
		t.Fatal(err)
	}
	if sp.on() {
		t.Error("a threshold no case reaches produced a split")
	}
}

// Off is off: no threshold means no lanes and no error.
func TestNoThresholdIsNoLanes(t *testing.T) {
	cases, took := corpus("a", 100)
	sp, err := planLanes(cases, took, 0, 8)
	if err != nil || sp.on() {
		t.Errorf("lane_slow_secs=0 gave %+v %v", sp, err)
	}
	if sp.laneOf(0) != dispatch.LaneAny {
		t.Error("with lanes off a slot is not in LaneAny")
	}
}
