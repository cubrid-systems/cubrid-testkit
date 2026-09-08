package status

import (
	"strings"
	"testing"
	"time"
)

// The replay reads what a run already wrote. This is feedback.log's real shape:
// a verdict line carrying the slot, the case and the elapsed milliseconds, and
// the line after it carrying the wall clock the case finished at.
const feedbackSample = `[OK]:  /x/shell/_02_sqlx_init/a/cases/a.sh 5048 EnvId=local[slot0]
a-1 : OK
01:42:55----/x/shell/_02_sqlx_init/a/cases--- time=5
[NOK]: TRY-> = 0 /x/shell/_06_issues/b/cases/b.sh 12000 EnvId=local[slot3]
b-1 : NOK it did not work
01:42:58----/x/shell/_06_issues/b/cases--- time=12
[OK]:  /x/shell/_38_csql/c/cases/c.sh 1500 EnvId=local[slot0]
01:43:10----/x/shell/_38_csql/c/cases--- time=2
`

func TestParseFeedbackRecoversTheRun(t *testing.T) {
	ev, err := ParseFeedback(strings.NewReader(feedbackSample))
	if err != nil {
		t.Fatal(err)
	}
	if len(ev) != 3 {
		t.Fatalf("recovered %d cases of 3: %+v", len(ev), ev)
	}
	// In start order, which is what makes the playback show the run's shape.
	if !strings.HasSuffix(ev[0].Case, "/b.sh") {
		t.Errorf("events are not in start order: first is %s", ev[0].Case)
	}
	byCase := map[string]Event{}
	for _, e := range ev {
		byCase[e.Case[strings.LastIndex(e.Case, "/")+1:]] = e
	}
	a := byCase["a.sh"]
	if a.Slot != "slot0" || !a.OK || a.Took != 5048*time.Millisecond {
		t.Errorf("a.sh came back as %+v", a)
	}
	b := byCase["b.sh"]
	if b.Slot != "slot3" || b.OK {
		t.Errorf("a failing case on slot3 came back as %+v", b)
	}
	// Start is the finish minus the elapsed, which is what lets the playback
	// show two slots busy at once.
	if got := a.Start.Add(a.Took); got.Format("15:04:05") != "01:42:55" {
		t.Errorf("a.sh finishes at %s, want 01:42:55", got.Format("15:04:05"))
	}
	if !b.Start.Before(a.Start) {
		t.Error("b.sh started at 01:42:46 and a.sh at 01:42:50; the order is wrong")
	}
}

// A run that crosses midnight goes backwards in a file that records no date.
func TestParseFeedbackCrossesMidnight(t *testing.T) {
	ev, err := ParseFeedback(strings.NewReader(
		"[OK]:  /x/a/cases/a.sh 1000 EnvId=local[slot0]\n" +
			"23:59:30----/x/a/cases--- time=1\n" +
			"[OK]:  /x/b/cases/b.sh 1000 EnvId=local[slot0]\n" +
			"00:00:30----/x/b/cases--- time=1\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(ev) != 2 {
		t.Fatalf("recovered %d of 2", len(ev))
	}
	if gap := ev[1].Start.Sub(ev[0].Start); gap < 0 || gap > 2*time.Minute {
		t.Errorf("the two cases are %v apart; midnight was not handled", gap)
	}
}

// A verdict with no matching stamp is dropped rather than guessed at: a replay
// that invents a start time draws a run that did not happen.
func TestAnUnpairedVerdictIsDropped(t *testing.T) {
	_, err := ParseFeedback(strings.NewReader(
		"[OK]:  /x/a/cases/a.sh 1000 EnvId=local[slot0]\n"))
	if err == nil {
		t.Error("a file with no timestamps produced events")
	}
}

// A file that is not a feedback.log says so instead of serving an empty page.
func TestNotAFeedbackLog(t *testing.T) {
	if _, err := ParseFeedback(strings.NewReader("hello\nworld\n")); err == nil {
		t.Error("arbitrary text was accepted as a run")
	}
}

// The point of the whole thing: the wall clock is compressed and the durations
// are not.
func TestReplayKeepsTheRealDurations(t *testing.T) {
	ev, err := ParseFeedback(strings.NewReader(feedbackSample))
	if err != nil {
		t.Fatal(err)
	}
	// 15 seconds of run at 1000x is milliseconds of test.
	started := time.Now()
	stop, err := Replay(ev, 1000, "127.0.0.1:0", &strings.Builder{})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if took := time.Since(started); took > 5*time.Second {
		t.Errorf("a 15-second run at 1000x took %v", took)
	}
}
