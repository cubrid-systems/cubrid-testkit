package haapply

import "testing"

func TestAPatternIsMissedWhenT2WasSeenFirstOrTheNodesDiffer(t *testing.T) {
	cases := []struct {
		name string
		o    Outcome
	}{
		{"seen first", Outcome{Pattern: "pk_update", Rounds: 20, Ahead: 1, Final: "same"}},
		{"differ", Outcome{Pattern: "pk_update", Rounds: 20, Final: "differ:t_pk_update"}},
		{"apply failed", Outcome{Pattern: "pk_update", Rounds: 20, Final: "same", FailDelta: 2}},
		{"fail counter moved down", Outcome{Pattern: "pk_update", Rounds: 20, Final: "same", FailDelta: -1}},
		{"stalled", Outcome{Pattern: "pk_update", Rounds: 4, Final: "stalled", Stalled: true}},
	}
	for _, c := range cases {
		got := Judge([]Outcome{{Pattern: "ctl", Rounds: 20, Ahead: 19, Final: "same"}, c.o})
		if got[1] != Missed {
			t.Errorf("%s: verdict %q, want %q", c.name, got[1], Missed)
		}
	}
}

// A pair that never reorders proves nothing by holding: the control is how a run
// knows it could have seen a miss (ADR-EXT-012 3-3).
func TestHeldIsInconclusiveWhenTheControlNeverWentFirst(t *testing.T) {
	ok := Outcome{Pattern: "pk_update", Rounds: 20, Final: "same"}
	got := Judge([]Outcome{{Pattern: "ctl", Rounds: 20, Ahead: 0, Final: "same"}, ok})
	if got[0] != ControlStill || got[1] != Inconclusive {
		t.Errorf("verdicts %v, want [%s %s]", got, ControlStill, Inconclusive)
	}
	got = Judge([]Outcome{{Pattern: "ctl", Rounds: 20, Ahead: 3, Final: "same"}, ok})
	if got[0] != ControlOvertook || got[1] != Held {
		t.Errorf("verdicts %v, want [%s %s]", got, ControlOvertook, Held)
	}
}

// A miss is a miss whatever the control did: T2 seen first is direct evidence.
func TestAMissStandsWithoutTheControl(t *testing.T) {
	got := Judge([]Outcome{{Pattern: "ctl", Rounds: 20, Final: "same"},
		{Pattern: "fp_negzero", Rounds: 20, Ahead: 4, Final: "same"}})
	if got[1] != Missed {
		t.Errorf("verdict %q, want %q", got[1], Missed)
	}
}

// A round whose write the master refused asks nothing, so a pattern with one
// cannot be held.
func TestARefusedWriteMakesHeldInconclusive(t *testing.T) {
	got := Judge([]Outcome{{Pattern: "ctl", Rounds: 20, Ahead: 20, Final: "same"},
		{Pattern: "uk_move", Rounds: 20, Final: "same", Refused: 1}})
	if got[1] != Inconclusive {
		t.Errorf("verdict %q, want %q", got[1], Inconclusive)
	}
}

// The control shares nothing, so its going first is expected; but a control that
// leaves the nodes different has found something else wrong and says so.
func TestAControlThatLeavesTheNodesDifferentIsMissed(t *testing.T) {
	got := Judge([]Outcome{{Pattern: "ctl", Rounds: 20, Ahead: 20, Final: "differ:t_ctl"}})
	if got[0] != Missed {
		t.Errorf("verdict %q, want %q", got[0], Missed)
	}
}

// A witness that did not arrive in time is a slow slave unless something else
// went wrong: T2 waiting for T1 is what a held dependency looks like.
func TestAWitnessThatNeverArrivedAloneIsInconclusive(t *testing.T) {
	got := Judge([]Outcome{{Pattern: "ctl", Rounds: 20, Ahead: 20, Final: "same"},
		{Pattern: "pk_update", Rounds: 20, Never: 1, Final: "same"}})
	if got[1] != Inconclusive {
		t.Errorf("verdict %q, want %q", got[1], Inconclusive)
	}
}
