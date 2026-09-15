package isolationsuite

import (
	"strings"
	"testing"
)

func TestSlotsAreTheDefaultAndTheMachineBoundsThem(t *testing.T) {
	unset := load(t, "scenario=/s\n")
	for _, c := range []struct {
		name          string
		cpus, availMB int
		want          int
		why           string
	}{
		{"a machine with room", 16, 16000, 4, "the default"},
		{"nothing known about the machine", 0, 0, 4, "the default"},
		{"two CPUs", 2, 16000, 2, "one for each CPU"},
		{"memory for three", 16, 2048 + 3*slotMB + 100, 3, "available memory"},
		{"memory for none", 16, 1000, 1, "available memory"},
	} {
		n, why := slotsFor(unset, c.cpus, c.availMB)
		if n != c.want || !strings.Contains(why, c.why) {
			t.Errorf("%s: %d slots (%q), want %d (%q)", c.name, n, why, c.want, c.why)
		}
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
		if n, _ := slotsFor(load(t, body), 2, 1000); n != want {
			t.Errorf("%q: %d slots, want %d", strings.TrimSpace(body), n, want)
		}
	}
}
