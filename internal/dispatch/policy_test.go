package dispatch

import (
	"strconv"
	"strings"
	"testing"
)

// The order puts the heaviest cases first, so N slots start N of them at once.
// Measured on the full corpus at 24 slots: the tmpfs went from empty to
// 25,584 MB of 25,600 in under three minutes with zero cases finished.
func TestHeavyCasesDoNotFillEverySlotAtTheStart(t *testing.T) {
	cases := []string{"h1", "h2", "h3", "h4", "l1", "l2", "l3", "l4"}
	q := New(cases, 0)
	q.Policy(nil, NewHeavyCap([]string{"h1", "h2", "h3", "h4"}, 2))

	var got []string
	for i := 0; i < 4; i++ {
		tk, ok := q.Claim()
		if !ok {
			t.Fatalf("claim %d refused with cases left", i)
		}
		got = append(got, tk.Case)
	}
	heavy := 0
	for _, c := range got {
		if strings.HasPrefix(c, "h") {
			heavy++
		}
	}
	if heavy != 2 {
		t.Fatalf("four slots took %v; want exactly 2 heavy", got)
	}
	// And the light ones came in the order the schedule wanted them.
	if got[2] != "l1" || got[3] != "l2" {
		t.Errorf("the order should be kept among what is admitted: %v", got)
	}

	// A heavy case finishing lets the next heavy one start.
	q.Complete(Ticket{Case: got[0]}, true, false)
	tk, ok := q.Claim()
	if !ok || !strings.HasPrefix(tk.Case, "h") {
		t.Errorf("a freed heavy slot should take a heavy case, got %v %v", tk.Case, ok)
	}
}

// A policy that can refuse every case is a policy that can stop the run.
func TestAPolicyIsNeverAskedToAdmitTheOnlyCase(t *testing.T) {
	q := New([]string{"h1", "h2"}, 0)
	q.Policy(nil, NewHeavyCap([]string{"h1", "h2"}, 1))

	first, ok, again := q.claimOnce("slot0", LaneAny)
	if !ok || again {
		t.Fatal("the first case was refused")
	}
	// Both cases are heavy, so there is no other work: the preference relaxes
	// rather than idle a slot through the tail. That is the whole difference
	// between a preference and a constraint.
	if _, ok, _ := q.claimOnce("slot1", LaneAny); !ok {
		t.Fatal("with nothing else to run, the stagger has to yield")
	}
	q.Complete(first, true, false)
}

func TestHeadroomStopsNewCasesWhenTheCeilingIsFull(t *testing.T) {
	used, limit := 0, 1000
	q := New([]string{"a", "b", "c"}, 0)
	q.Policy(NewHeadroom("the corpus tmpfs", func() (int, int) { return used, limit }, 80), nil)

	if _, ok, _ := q.claimOnce("slot0", LaneAny); !ok {
		t.Fatal("an empty ceiling should admit")
	}
	used = 850
	if _, ok, again := q.claimOnce("slot1", LaneAny); ok || !again {
		t.Fatalf("a full ceiling should hold the claimant, not end it: ok=%v again=%v", ok, again)
	}
	used = 100
	if _, ok, _ := q.claimOnce("slot1", LaneAny); !ok {
		t.Fatal("room again should admit again")
	}
}

// The two answer different questions and have to compose.
func TestPoliciesCompose(t *testing.T) {
	used := 0
	p := All(
		NewHeavyCap([]string{"h1", "h2"}, 1),
		NewHeadroom("tmpfs", func() (int, int) { return used, 100 }, 80),
		nil,
	)
	if !p.Admit("h1", nil) {
		t.Error("nothing running, nothing full: admit")
	}
	if p.Admit("h2", []string{"h1"}) {
		t.Error("the heavy cap should refuse")
	}
	used = 90
	if p.Admit("l1", nil) {
		t.Error("the ceiling should refuse")
	}
	if !strings.Contains(p.Describe(), "heaviest") || !strings.Contains(p.Describe(), "full") {
		t.Errorf("the run has to be able to print what it is doing: %q", p.Describe())
	}
	if All(nil, nil) != nil {
		t.Error("no policies is no policy, not an empty one")
	}
}

func TestHeaviestNamesTheLongest(t *testing.T) {
	got := Heaviest(map[string]float64{"a": 1, "b": 100, "c": 50, "d": 7}, 2)
	if len(got) != 2 || got[0] != "b" || got[1] != "c" {
		t.Errorf("Heaviest = %v, want [b c]", got)
	}
	if Heaviest(nil, 3) != nil || Heaviest(map[string]float64{"a": 1}, 0) != nil {
		t.Error("nothing to rank is nothing")
	}
}

// The stagger must not shape the tail.
//
// Measured with the two rules treated alike: a 24-slot run finished its last 150
// cases six at a time, because every case left was in the heavy set and the cap
// allowed six. Eighteen slots idled for half an hour, rebuilding the long tail
// that ordering longest-first exists to prevent.
func TestTheStaggerYieldsWhenThereIsNothingElseToRun(t *testing.T) {
	// Four heavy cases, a cap of one, and nothing else in the queue.
	q := New([]string{"h1", "h2", "h3", "h4"}, 0)
	q.Policy(nil, NewHeavyCap([]string{"h1", "h2", "h3", "h4"}, 1))

	var got []string
	for i := 0; i < 4; i++ {
		tk, ok, again := q.claimOnce("slot"+strconv.Itoa(i), LaneAny)
		if again || !ok {
			t.Fatalf("slot %d idled through a tail of heavy cases: ok=%v again=%v", i, ok, again)
		}
		got = append(got, tk.Case)
	}
	if len(got) != 4 {
		t.Fatalf("four slots should be working, got %v", got)
	}
}

// And it still binds while there is other work, which is what it is for.
func TestTheStaggerBindsWhileOrdinaryWorkRemains(t *testing.T) {
	q := New([]string{"h1", "h2", "h3", "l1", "l2", "l3"}, 0)
	q.Policy(nil, NewHeavyCap([]string{"h1", "h2", "h3"}, 1))

	var heavy int
	for i := 0; i < 4; i++ {
		tk, ok, _ := q.claimOnce("slot"+strconv.Itoa(i), LaneAny)
		if !ok {
			t.Fatalf("claim %d refused with work left", i)
		}
		if strings.HasPrefix(tk.Case, "h") {
			heavy++
		}
	}
	if heavy != 1 {
		t.Errorf("the cap should hold at one heavy case while light ones remain, got %d", heavy)
	}
}

// A constraint never yields, tail or no tail: the ceiling is not a preference.
func TestTheConstraintDoesNotYieldAtTheTail(t *testing.T) {
	q := New([]string{"a", "b"}, 0)
	q.Policy(NewHeadroom("tmpfs", func() (int, int) { return 95, 100 }, 80), nil)

	// The first is handed out because nothing is running -- a rule that can
	// refuse every case is a rule that can stop the run.
	if _, ok, _ := q.claimOnce("slot0", LaneAny); !ok {
		t.Fatal("the first case must be handed out whatever the rules say")
	}
	// The second must wait, even though there is nothing else to run.
	if _, ok, again := q.claimOnce("slot1", LaneAny); ok || !again {
		t.Fatalf("a full ceiling must hold, tail or not: ok=%v again=%v", ok, again)
	}
}

// A gate with nothing to gate says nothing. This shipped the other way: a run
// with no scenario_ram_mb still printed "admission: always: start nothing new
// while the corpus tmpfs is over 80% full", naming a tmpfs that did not exist
// and a rule that could never fire, which reads as a protection the run has not
// got.
func TestAGateWithNoLimitDoesNotAnnounceItself(t *testing.T) {
	none := NewHeadroom("the corpus tmpfs", func() (int, int) { return 0, 0 }, 80)
	if d := none.Describe(); d != "" {
		t.Errorf("a headroom over no limit describes itself as %q", d)
	}
	if !none.Admit("a", []string{"b"}) {
		t.Error("a headroom over no limit refused a case")
	}

	// With nothing else configured the run is back to the order alone, and must
	// say that rather than an empty rule.
	if d := Describe(none, nil); d != "the plan's order alone" {
		t.Errorf("with only a limitless gate the run announces %q", d)
	}

	// Composed with a real rule it must not leave a dangling separator.
	cap := NewHeavyCap([]string{"h"}, 2)
	if d := Describe(none, cap); strings.Contains(d, "always: ,") || strings.HasPrefix(d, ", ") {
		t.Errorf("an empty rule left a separator behind: %q", d)
	}
	if got := All(none, cap).Describe(); got != cap.Describe() {
		t.Errorf("All kept the empty description: %q", got)
	}

	// And a real limit still announces itself.
	real := NewHeadroom("the corpus tmpfs", func() (int, int) { return 10, 100 }, 80)
	if d := real.Describe(); d == "" {
		t.Error("a headroom over a real limit says nothing")
	}
}
