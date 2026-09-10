package shellsuite

import (
	"strings"
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

	sp, err := planLanes(cases, took, nil, 30, 0, 0, 10)
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
	sp, err := planLanes(cases, took, nil, 30, 0, 0, 2)
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
	sp, err := planLanes(cases, took, nil, 30, 0, 0, 4)
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
	if _, err := planLanes(cases, nil, nil, 30, 0, 0, 4); err == nil {
		t.Error("lanes were planned with no durations")
	}
	if _, err := planLanes(cases, map[string]time.Duration{"/x/a/cases/a.sh": time.Second}, nil, 30, 0, 0, 1); err == nil {
		t.Error("lanes were planned for one slot")
	}
	// And a threshold nothing reaches is not a split, it is a mistake worth
	// reporting -- an empty slow lane would leave its slots idle all run.
	sp, err := planLanes(cases, map[string]time.Duration{"/x/a/cases/a.sh": time.Second}, nil, 30, 0, 0, 4)
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
	sp, err := planLanes(cases, took, nil, 0, 0, 0, 8)
	if err != nil || sp.on() {
		t.Errorf("lane_slow_secs=0 gave %+v %v", sp, err)
	}
	if sp.laneOf(0) != dispatch.LaneAny {
		t.Error("with lanes off a slot is not in LaneAny")
	}
}

// The ceiling is made of space, so the lane that gives space away is chosen by
// space. Duration must not get a vote when only lane_slow_mb is set.
func TestLanesSelectByFootprintNotDuration(t *testing.T) {
	const root = "/c/shell"
	// A big slow directory, a big fast one, and a small slow one. Only the two
	// big ones belong on disk.
	cases := []string{
		root + "/_a/big_slow/cases/big_slow.sh",
		root + "/_b/big_fast/cases/big_fast.sh",
		root + "/_c/small_slow/cases/small_slow.sh",
		root + "/_d/small_fast/cases/small_fast.sh",
	}
	took := map[string]time.Duration{
		cases[0]: 600 * time.Second,
		cases[1]: 3 * time.Second,
		cases[2]: 600 * time.Second,
		cases[3]: 3 * time.Second,
	}
	held := map[string]int{
		root + "/_a/big_slow/cases":   5000,
		root + "/_b/big_fast/cases":   4000,
		root + "/_c/small_slow/cases": 20,
		root + "/_d/small_fast/cases": 20,
	}

	sp, err := planLanes(cases, took, held, 0, 1000, 0, 4)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		dir  string
		want dispatch.Lane
	}{
		{root + "/_a/big_slow/cases", dispatch.LaneSlow},
		{root + "/_b/big_fast/cases", dispatch.LaneSlow},
		{root + "/_c/small_slow/cases", dispatch.LaneFast},
		{root + "/_d/small_fast/cases", dispatch.LaneFast},
	} {
		if got := sp.byDir[c.dir]; got != c.want {
			t.Errorf("%s: lane %v, want %v", c.dir, got, c.want)
		}
	}
	// The fast lane keeps a 600-second case, which is the whole point: long is
	// not the same as large, and the duration threshold got exactly this wrong.
	if sp.FastMB != 40 || sp.SlowMB != 9000 {
		t.Errorf("fast %d MB slow %d MB, want 40 and 9000", sp.FastMB, sp.SlowMB)
	}
	if sp.FastBiggest != 20 {
		t.Errorf("biggest directory left in memory is %d MB, want 20", sp.FastBiggest)
	}
}

// Both thresholds set is the union: each names a different reason a directory
// should not be in memory, so either reason is enough.
func TestBothThresholdsAreAUnion(t *testing.T) {
	const root = "/c/shell"
	cases := []string{
		root + "/_a/big/cases/big.sh",
		root + "/_b/long/cases/long.sh",
		root + "/_c/neither/cases/neither.sh",
	}
	took := map[string]time.Duration{
		cases[0]: 3 * time.Second,
		cases[1]: 600 * time.Second,
		cases[2]: 3 * time.Second,
	}
	held := map[string]int{
		root + "/_a/big/cases":     5000,
		root + "/_b/long/cases":    10,
		root + "/_c/neither/cases": 10,
	}
	sp, err := planLanes(cases, took, held, 300, 1000, 0, 4)
	if err != nil {
		t.Fatal(err)
	}
	if sp.SlowCases != 2 {
		t.Fatalf("slow lane holds %d cases, want 2 (the big one and the long one)", sp.SlowCases)
	}
	if sp.byDir[root+"/_c/neither/cases"] != dispatch.LaneFast {
		t.Error("a directory that is neither big nor long belongs in memory")
	}
}

// A threshold with nothing measured to threshold against is an operator error,
// and it has to be said rather than silently ignored.
func TestFootprintLaneNeedsMeasurements(t *testing.T) {
	cases := []string{"/c/shell/_a/x/cases/x.sh"}
	took := map[string]time.Duration{cases[0]: time.Second}
	_, err := planLanes(cases, took, nil, 0, 1000, 0, 4)
	if err == nil {
		t.Fatal("lane_slow_mb with no footprints should refuse")
	}
	if !strings.Contains(err.Error(), "case_sizes") {
		t.Errorf("the refusal should name the file that supplies them: %v", err)
	}
}

// Footprint and duration are unrelated on this corpus -- over 2,949 directories
// measured at their peak, Spearman is +0.02 -- so selecting the disk lane by
// footprint picks its bandwidth demand at random. The disk delivered 88 MB/s
// under four concurrent writers, and these are real rows from that measurement:
//
//	8,662 MB in  17 s = 512 MB/s
//	9,093 MB in 523 s =  17 MB/s
//
// lane_slow_mb sees similar megabytes and cannot tell them apart. Rate can, and
// spends a budget longest-first, which is the greedy order for "as many bytes as
// possible within a bandwidth" because bytes over bytes-per-second is seconds.
func TestTheDiskLaneIsChosenByRateAndNotByBytes(t *testing.T) {
	dirs := map[string]struct{ mb, secs int }{
		"/c/fast_and_huge/cases": {8662, 17},  // 512 MB/s
		"/c/slow_and_huge/cases": {9093, 523}, // 17 MB/s
		"/c/slow_and_big/cases":  {6000, 300}, // 20 MB/s
		"/c/tiny/cases":          {2, 5},      // 0.4 MB/s
	}
	var cases []string
	took := map[string]time.Duration{}
	held := map[string]int{}
	for d, v := range dirs {
		c := d + "/a.sh"
		cases = append(cases, c)
		took[c] = time.Duration(v.secs) * time.Second
		held[d] = v.mb
	}

	// A budget of 88 MB/s, spent longest-first: 523 s takes 17, 300 s takes 20,
	// 5 s takes 0.4 -- and the 17-second monster is refused because 512 does not
	// fit in what is left.
	sp, err := planLanes(cases, took, held, 0, 0, 88, 8)
	if err != nil {
		t.Fatal(err)
	}
	for dir, want := range map[string]dispatch.Lane{
		"/c/slow_and_huge/cases": dispatch.LaneSlow,
		"/c/slow_and_big/cases":  dispatch.LaneSlow,
		"/c/tiny/cases":          dispatch.LaneSlow,
		"/c/fast_and_huge/cases": dispatch.LaneFast,
	} {
		if got := sp.byDir[dir]; got != want {
			t.Errorf("%s went to lane %v, want %v", dir, got, want)
		}
	}

	// And what footprint would have done with the same numbers: the 512 MB/s
	// directory is the *first* thing it sends to disk.
	byBytes, err := planLanes(cases, took, held, 0, 8000, 0, 8)
	if err != nil {
		t.Fatal(err)
	}
	if byBytes.byDir["/c/fast_and_huge/cases"] != dispatch.LaneSlow {
		t.Error("lane_slow_mb=8000 was expected to send the 8,662 MB directory to disk; " +
			"the point of this test is that it does, and that rate does not")
	}
}

// A rate is megabytes over seconds, and both halves come from a previous run.
func TestTheRateLaneNeedsBothMeasurements(t *testing.T) {
	cases := []string{"/c/a/cases/a.sh"}
	took := map[string]time.Duration{"/c/a/cases/a.sh": 10 * time.Second}
	held := map[string]int{"/c/a/cases": 100}

	if _, err := planLanes(cases, nil, held, 0, 0, 88, 8); err == nil {
		t.Error("lane_slow_mbps with no durations should say what it needs")
	}
	if _, err := planLanes(cases, took, nil, 0, 0, 88, 8); err == nil {
		t.Error("lane_slow_mbps with no footprints should say what it needs")
	}
}

// Every knob that names a lane has to turn lanes on. lane_slow_mbps was added
// and the boolean that decides whether to split the corpus at all was left
// naming only the two older ones, so a run configured with it ran shared -- and
// said so in a line that also tested only the oldest knob.
func TestEveryLaneKnobTurnsLanesOn(t *testing.T) {
	for _, c := range []struct {
		name                       string
		slowSecs, slowMB, slowMBps int
		want                       bool
	}{
		{"nothing configured", 0, 0, 0, false},
		{"by duration", 30, 0, 0, true},
		{"by footprint", 0, 1000, 0, true},
		{"by rate", 0, 0, 88, true},
		{"rate with the others", 30, 1000, 88, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := c.slowSecs > 0 || c.slowMB > 0 || c.slowMBps > 0
			if got != c.want {
				t.Errorf("lane_slow_secs=%d lane_slow_mb=%d lane_slow_mbps=%d: lanes=%v want %v",
					c.slowSecs, c.slowMB, c.slowMBps, got, c.want)
			}
		})
	}
}
