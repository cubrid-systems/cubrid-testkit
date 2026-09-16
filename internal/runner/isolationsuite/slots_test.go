package isolationsuite

import (
	"strings"
	"testing"

	"github.com/cubrid-systems/cubrid-testkit/internal/contain"
	"github.com/cubrid-systems/cubrid-testkit/internal/sizing"
)

// With nothing written, the machine decides; with nothing known about it, one.
func TestAnUnsetSlotCountIsSized(t *testing.T) {
	t.Setenv("CUBRID", "")
	unset := load(t, "scenario=/s\n")
	if p := slotsFor(unset, "/s", 100, 0, nil); p.Slots != 1 || !strings.Contains(p.Why, "unknown") {
		t.Errorf("unknown memory: %d slots (%q), want one", p.Slots, p.Why)
	}
	if p := slotsFor(unset, "/s", 100, 1000, nil); p.Slots != 1 || !strings.Contains(p.Why, "floor") {
		t.Errorf("no memory to speak of: %d slots (%q), want one", p.Slots, p.Why)
	}
	if p := slotsFor(unset, "/s", 100, 2048+1750*2, nil); p.Slots != 2 || !strings.Contains(p.Why, "shipped") {
		t.Errorf("memory for two at the shipped figure: %d slots (%q)", p.Slots, p.Why)
	}
}

// parallel reaches the decision, and a word it does not know is said out loud.
func TestParallelIsReadFromTheConfiguration(t *testing.T) {
	t.Setenv("CUBRID", "")
	recs := []sizing.Record{{Corpus: "/s", Lane: contain.Lane(), Slots: 8, Cases: 100, PerSlotMB: 100}}
	const avail = 2048 + 115*2
	measured := slotsFor(load(t, "scenario=/s\n"), "/s", 100, avail, recs)
	careful := slotsFor(load(t, "scenario=/s\nparallel=conservative\n"), "/s", 100, avail, recs)
	if careful.Slots >= measured.Slots || !strings.Contains(careful.Why, "conservative") {
		t.Errorf("conservative: %d slots (%q) against measured %d", careful.Slots, careful.Why, measured.Slots)
	}
	typo := slotsFor(load(t, "scenario=/s\nparallel=fast\n"), "/s", 100, avail, recs)
	if typo.Slots != measured.Slots || !strings.Contains(typo.Why, `parallel="fast"`) {
		t.Errorf("an unknown word: %d slots (%q), want measured's %d and a warning", typo.Slots, typo.Why, measured.Slots)
	}
}

// The buffers a slot runs with are the configuration's when it writes them.
func TestTheConfigurationsBuffersAreTheSlots(t *testing.T) {
	t.Setenv("CUBRID", "")
	e := engineOf(load(t, "scenario=/s\ndefault.cubrid.data_buffer_size=128M\ndefault.cubrid.log_buffer_size=64M\n"))
	if e.DataBufferMB != 128 || e.LogBufferMB != 64 {
		t.Errorf("got %+v, want 128 and 64", e)
	}
}

// Whoever writes parallel_slots has decided, including to run serially as CTP
// does, and including more slots than the machine is sized for.
func TestAWrittenSlotCountIsUsedAsWritten(t *testing.T) {
	for body, want := range map[string]int{
		"parallel_slots=1\n":  1,
		"parallel_slots=12\n": 12,
		"parallel_slots=0\n":  1,
		"parallel_slots=x\n":  1,
	} {
		if p := slotsFor(load(t, body), "/s", 100, 1000, nil); p.Slots != want {
			t.Errorf("%q: %d slots, want %d", strings.TrimSpace(body), p.Slots, want)
		}
	}
}

// A case's longest time is evidence about the corpus only when runone.sh ran the
// controller once; a retried case measures the timeout.
func TestFirstAttempt(t *testing.T) {
	one := "+ START=1\n+ elapse=66000\n+ echo 'elapse: 66000'\nflag: OK\n"
	three := "+ START=1\n+ elapse=100431\n+ echo 'elapse: 100431'\n+ elapse=100427\n+ elapse=250\nflag: OK\n"
	if !firstAttempt(one) {
		t.Error("one attempt should be a first attempt")
	}
	if firstAttempt(three) {
		t.Error("three attempts should not be")
	}
}
