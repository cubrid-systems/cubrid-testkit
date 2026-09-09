package dispatch

import (
	"strings"
	"testing"
)

// The order puts the heaviest cases first, so N slots start N of them at once.
// Measured on the full corpus at 24 slots: the tmpfs went from empty to
// 25,584 MB of 25,600 in under three minutes with zero cases finished.
func TestHeavyCasesDoNotFillEverySlotAtTheStart(t *testing.T) {
	cases := []string{"h1", "h2", "h3", "h4", "l1", "l2", "l3", "l4"}
	q := New(cases, 0)
	q.Policy(NewHeavyCap([]string{"h1", "h2", "h3", "h4"}, 2))

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
	q.Policy(NewHeavyCap([]string{"h1", "h2"}, 1))

	first, ok := q.Claim()
	if !ok {
		t.Fatal("the first case was refused")
	}
	if _, ok := q.Claim(); ok {
		t.Fatal("the second heavy case should be held while the first runs")
	}
	q.Complete(first, true, false)
	if _, ok := q.Claim(); !ok {
		t.Fatal("with nothing running the queue must hand out a case whatever the policy says")
	}
}

// Feedback, for everything after the start. Late by construction: it can only
// stop the next case, never the ones already running.
func TestHeadroomStopsNewCasesWhenTheCeilingIsFull(t *testing.T) {
	used, limit := 0, 1000
	q := New([]string{"a", "b", "c"}, 0)
	q.Policy(NewHeadroom("the corpus tmpfs", func() (int, int) { return used, limit }, 80))

	if _, ok := q.Claim(); !ok {
		t.Fatal("an empty ceiling should admit")
	}
	used = 850
	if _, ok := q.Claim(); ok {
		t.Fatal("a ceiling over 80% should not admit another case")
	}
	used = 100
	if _, ok := q.Claim(); !ok {
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
