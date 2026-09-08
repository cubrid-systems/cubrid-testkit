package status

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Replaying a finished run on the same page.
//
// The page answers "which slot is stuck and on what" while a run is going, and
// after it there is nothing to look at but a log. But a run already records
// everything the page needs, in feedback.log, which is a frozen surface every
// runner writes:
//
//	[OK]:  /path/_02_sqlx_init/x/cases/x.sh 5048 EnvId=local[slot0]
//	01:42:55----/path/_02_sqlx_init/x/cases--- time=5
//
// The verdict line carries the slot, the case and the elapsed milliseconds; the
// line after it carries the wall clock the case finished at. Subtracting gives
// the start, and with starts and durations for every case the whole run can be
// played back -- including which slots were busy together, which is the part a
// log cannot show.
//
// So there is no recorder. A replay reads what the run already wrote, which
// means it works on runs that finished before this existed.

// Event is one case as the log recorded it.
type Event struct {
	Slot  string
	Case  string
	OK    bool
	Start time.Time
	Took  time.Duration
}

var (
	// Both verdict shapes: [OK] has the path first, [NOK] puts "TRY-> = n" in
	// front of it.
	reVerdict = regexp.MustCompile(`^\[(OK|NOK)\]:.*?(\S+\.sh) (\d+) EnvId=\S*?\[?([a-zA-Z0-9_]*)\]?\s*$`)
	reStamp   = regexp.MustCompile(`^(\d{2}):(\d{2}):(\d{2})----(\S+?)--- time=`)
)

// ParseFeedback reads the events out of a feedback.log.
//
// A verdict is paired with the next timestamp naming its own directory, because
// that is how the file is written: the block for a case ends with its stamp.
// Anything unpaired is dropped rather than guessed at -- a replay that invents a
// start time would draw a picture of a run that did not happen.
func ParseFeedback(r io.Reader) ([]Event, error) {
	type pending struct {
		slot, path string
		ok         bool
		took       time.Duration
	}
	var out []Event
	var wait []pending
	var last time.Time
	day := 0

	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for s.Scan() {
		line := s.Text()
		if m := reVerdict.FindStringSubmatch(line); m != nil {
			ms, _ := strconv.Atoi(m[3])
			slot := m[4]
			if slot == "" {
				slot = "slot0"
			}
			wait = append(wait, pending{slot: slot, path: m[2], ok: m[1] == "OK",
				took: time.Duration(ms) * time.Millisecond})
			continue
		}
		m := reStamp.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		h, _ := strconv.Atoi(m[1])
		mi, _ := strconv.Atoi(m[2])
		sec, _ := strconv.Atoi(m[3])
		dir := m[4]
		// The stamp has no date. A run that crosses midnight goes backwards, and
		// a day is added when it does.
		at := time.Date(2000, 1, 1+day, h, mi, sec, 0, time.UTC)
		if !last.IsZero() && at.Before(last.Add(-6*time.Hour)) {
			day++
			at = at.AddDate(0, 0, 1)
		}
		last = at
		for i, p := range wait {
			if strings.HasPrefix(p.path, dir+"/") {
				out = append(out, Event{Slot: p.slot, Case: p.path, OK: p.ok,
					Start: at.Add(-p.took), Took: p.took})
				wait = append(wait[:i:i], wait[i+1:]...)
				break
			}
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no cases found: is this a feedback.log?")
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Start.Equal(out[j].Start) {
			return out[i].Start.Before(out[j].Start)
		}
		return out[i].Case < out[j].Case
	})
	return out, nil
}

// ParseFeedbackFile is ParseFeedback over a path.
func ParseFeedbackFile(path string) ([]Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseFeedback(f)
}

// Replay serves the page and plays the events through it at speed times real
// time, returning when the last one has been played.
//
// The durations the page reports are the real ones -- the histogram, the family
// totals and the finished table all say what the run actually took. Only the
// wall clock is compressed, which is the point: a two-hour run is watchable in
// two minutes, and what it shows is the shape of the run, where the slots went
// idle and which family owned the time.
// It returns the way to take the page down. The page outlives the playback on
// purpose -- the run is over and the page is the thing being looked at, so
// stopping the server when the last case plays would close the window just as it
// became worth reading.
func Replay(events []Event, speed float64, addr string, out io.Writer) (stop func(), err error) {
	if speed <= 0 {
		speed = 1
	}
	b := New(len(events))
	b.replaying = true
	where, stop, err := b.Serve(addr)
	if err != nil {
		return func() {}, err
	}
	fmt.Fprintf(out, "[INFO] replaying %d cases at %gx on http://%s/\n", len(events), speed, where)

	slots := map[string]bool{}
	for _, e := range events {
		slots[e.Slot] = true
	}
	names := make([]string, 0, len(slots))
	for s := range slots {
		names = append(names, s)
	}
	sort.Slice(names, func(i, j int) bool { return slotLess(names[i], names[j]) })
	for _, s := range names {
		b.Lane(s, "replay")
	}

	// One ordered list of moments, so a case beginning and another ending at the
	// same instant happen in the order the run had them.
	type moment struct {
		at    time.Duration
		begin bool
		ev    Event
	}
	origin := events[0].Start
	var ms []moment
	for _, e := range events {
		ms = append(ms, moment{at: e.Start.Sub(origin), begin: true, ev: e})
		ms = append(ms, moment{at: e.Start.Add(e.Took).Sub(origin), ev: e})
	}
	sort.SliceStable(ms, func(i, j int) bool { return ms[i].at < ms[j].at })

	started := time.Now()
	for _, m := range ms {
		target := time.Duration(float64(m.at) / speed)
		if d := target - time.Since(started); d > 0 {
			time.Sleep(d)
		}
		if m.begin {
			b.Begin(m.ev.Slot, m.ev.Case)
		} else {
			b.endWith(m.ev.Slot, m.ev.Case, m.ev.OK, m.ev.Took)
		}
	}
	fmt.Fprintf(out, "[INFO] replay complete; the page stays up until interrupted\n")
	return stop, nil
}
