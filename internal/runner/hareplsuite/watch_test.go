package hareplsuite

import (
	"strings"
	"testing"
	"time"
)

// A page that stops advancing reads exactly like a run that is slow, and the
// difference is the one the watcher exists to show: a pair died once and an
// hour of a run went into wait_timeout with nobody looking.
func TestTheWatcherSaysWhenItsSourceHasStopped(t *testing.T) {
	now := time.Now()
	for _, c := range []struct {
		name       string
		done, tot  int
		since      time.Time
		wantSaying bool
	}{
		{"still writing", 36, 39, now, false},
		{"slow, but inside the bound", 36, 39, now.Add(-2 * time.Minute), false},
		{"gone quiet, unfinished", 36, 39, now.Add(-10 * time.Minute), true},
		// A finished run has nothing more to write, and calling that a fault
		// would report success as one.
		{"finished, and quiet for that reason", 39, 39, now.Add(-10 * time.Minute), false},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := quiet(c.done, c.tot, c.since)
			if saying := got != ""; saying != c.wantSaying {
				t.Fatalf("quiet(%d, %d, %s) = %q", c.done, c.tot, time.Since(c.since).Round(time.Second), got)
			}
			if c.wantSaying && !strings.Contains(got, "36 of 39") {
				t.Errorf("the sentence does not say how far it got: %q", got)
			}
		})
	}
}
