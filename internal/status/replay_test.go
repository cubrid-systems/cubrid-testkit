package status

import (
	"net/url"
	"strconv"
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

// The playback is a position on the run's own clock, not a loop through a list,
// which is what lets it be scrubbed. Going forward folds in more moments; going
// back rebuilds, because a board is an accumulation and there is nothing to
// subtract.
func TestAReplayCanBeScrubbed(t *testing.T) {
	ev, err := ParseFeedback(strings.NewReader(feedbackSample))
	if err != nil {
		t.Fatal(err)
	}
	stop, err := Replay(ev, 1, "127.0.0.1:0", &strings.Builder{})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	b := lastBoard
	rp := b.replay
	seek := func(sec float64) view {
		rp.control(url.Values{"seek": {strconv.FormatFloat(sec, 'f', -1, 64)}})
		return b.snapshot()
	}

	// At the very start nothing has happened.
	if v := seek(0); v.Done != 0 {
		t.Errorf("at the start %d cases are done", v.Done)
	}
	// At the end everything has.
	end := seek(1e9)
	if end.Done != 3 {
		t.Fatalf("at the end %d of 3 are done", end.Done)
	}
	// And back to the start again -- the part that needs a rebuild rather than
	// an undo.
	if v := seek(0); v.Done != 0 || len(v.Recent) != 0 {
		t.Errorf("scrubbing back left %d done and %d in the tail", v.Done, len(v.Recent))
	}
	// Forward once more gives the same answer as the first time: seeking is not
	// allowed to accumulate.
	if v := seek(1e9); v.Done != end.Done || v.OK != end.OK {
		t.Errorf("a second pass gave %d/%d, the first gave %d/%d", v.Done, v.OK, end.Done, end.OK)
	}

	// The knobs report where they are, so the page can draw them there.
	rp.control(url.Values{"speed": {"60"}, "paused": {"1"}})
	got := b.snapshot().Replay
	if got == nil || got.Speed != 60 || !got.Paused {
		t.Errorf("the controls did not take: %+v", got)
	}
	// Nonsense is ignored rather than obeyed.
	rp.control(url.Values{"speed": {"-5"}})
	if s := b.snapshot().Replay.Speed; s != 60 {
		t.Errorf("a negative speed was accepted: %v", s)
	}
}
