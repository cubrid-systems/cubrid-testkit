// Package status shows what a run is doing while it does it.
//
// Not on standard output. What the runner prints there is a frozen surface --
// ADR-003 -- and the comparison that proves this system equivalent reads it, so
// a screen drawn over it would be a screen drawn over the evidence. A page
// served on a port touches none of it.
//
// It exists because of slots. One worker's progress is legible in the log it
// writes; ten workers' is not, and the question that matters during a run --
// which slot is stuck, and on what -- has no answer in a stream of finished
// cases.
package status

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// DefaultAddr is where the page goes when the configuration asks for one
// without saying where.
//
// In the private range, 49152-65535, so it cannot collide with a registered
// service, and not 8080, which everything else on a developer's machine is
// already using. The digits are CUBRID's own 1523 with a 5 in front, which is
// the only reason this number rather than another.
const DefaultAddr = "127.0.0.1:51523"

// NearDefault is the nth port after the default, for a machine already running a
// run. Two runs on one machine is a normal thing to want -- a long one and a
// quick check of one family -- and the page is not worth failing over.
func NearDefault(n int) string {
	host, port, ok := strings.Cut(DefaultAddr, ":")
	if !ok {
		return DefaultAddr
	}
	base, err := strconv.Atoi(port)
	if err != nil {
		return DefaultAddr
	}
	return host + ":" + strconv.Itoa(base+n)
}

// Addr reads what the configuration said. A bare "on" takes DefaultAddr, a bare
// port takes every interface, and anything else is passed through as written.
//
// Loopback by default rather than every interface: a QA machine's run should not
// become a page the rest of the network can read because someone turned it on.
func Addr(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "":
		return ""
	case "on", "yes", "true", "1":
		return DefaultAddr
	}
	v = strings.TrimSpace(v)
	if !strings.Contains(v, ":") {
		return ":" + v
	}
	return v
}

// Board is what the workers report to and the page reads from.
//
// A run must not fail because nobody was watching it, so every method here is
// safe on a nil Board: the runner passes one when a port was asked for and nil
// when it was not, and no call site has to know which.
type Board struct {
	mu      sync.Mutex
	started time.Time
	total   int
	running map[string]inflight
	done    int
	ok      int
	recent  []finished
	// failed is every case that failed, not the failures among the last few.
	// A run of 217 with 56 failures is a run where the list of failures is the
	// thing being watched, and filtering a recency window would answer a
	// different question.
	failed []finished
	// buckets counts completions per bucketSpan, so the page can show whether
	// the run is keeping pace. With slots that is the question the numbers alone
	// do not answer: ten workers finishing nothing looks like one worker on a
	// long case until you see the rate go flat.
	buckets []int
	bucket0 time.Time
	// hist counts completions by how long they took, and byFamily and bySlot
	// aggregate them. All three are counters rather than lists: a corpus of
	// 3,452 cases should not be held twice so that a page can add it up.
	// corpusDir and ramDir are what the machine panel reports free space for,
	// set by the runner when it knows them.
	corpusDir string
	ramDir    string
	ramCap    int
	// sampler reads the machine on its own ticker, because CPU and disk are
	// counters and a rate needs two readings.
	sampler *sampler

	hist []int
	// histSecs is the same buckets weighted by time rather than by count, and
	// the two together are the lane decision. Counting cases says the corpus is
	// mostly short cases; counting seconds says a handful of long ones own the
	// run -- over _01_utility the 17 cases longer than 30 seconds are 8% of the
	// cases and 41% of the case-seconds. A lane split is a threshold on this
	// panel, and without the second series the panel cannot show where to put
	// it.
	histSecs []int
	byFamily map[string]*tally
	bySlot   map[string]*tally
	// laneOf is where a slot's writes go, and byLane adds the cases up by it.
	// One lane today -- every slot's corpus writes go to the same place -- which
	// is why the panel reads "tmpfs 8 slots" rather than a split. It is here
	// because the split is the next thing (B-T13) and because even undivided it
	// answers a question the other panels do not: what fraction of the run's
	// case-seconds is holding memory.
	laneOf map[string]string
	byLane map[string]*tally
	// templates is the database-template cache, when a run uses one. Nil when it
	// does not, which is every run that leaves CTP_DB_TEMPLATE_CACHE off.
	templates *templates
	// replaying says this board is playing a finished run back rather than
	// watching one happen, and the page says so -- an old run and a live one look
	// identical otherwise, and mistaking the first for the second is the kind of
	// error that costs an afternoon.
	replaying bool
	// detail is where this run's feedback.log is, which is what lets a finished
	// case be clicked. Nil when nobody said.
	detail *detail
	// replay is the playback's position, when this board is one. It is what the
	// controls move.
	replay *replayer
	// workLeft is the planned seconds of the cases still to come, and slots is
	// how many run at once. Together they are what is left; zero means no plan
	// was given and the rate has to do.
	workLeft time.Duration
	slots    int
	// tookSum is how long the finished cases took, summed. Its mean is what
	// stands in for a case the plan does not know, and it is a mean of case
	// durations rather than of wall clock per case, which is the distinction
	// that keeps a stall from being read as evidence that everything is slow.
	tookSum time.Duration
	// planned is what each case took last time, so finishing one can take the
	// right amount off workLeft. typical stands in for a case the plan does not
	// mention.
	planned map[string]time.Duration
	typical time.Duration
	// setup is how the run was configured, recorded once before the first case.
	setup setupBox
	// live is where each running case writes its verdicts, so the page can show
	// them as they arrive rather than only once the case is over.
	live liveFiles
	// patched maps a case to the compatibility patch it ran against, and refused
	// to the patch that would not apply.
	patched map[string]string
	refused map[string]string
	// replayAt is the knobs' state as the replayer last published it.
	//
	// Published rather than asked for. The replayer takes its own lock and then
	// the board's -- seeking calls Begin and End -- so a snapshot that held the
	// board's lock and then reached for the replayer's would deadlock the two
	// against each other. One direction only: replayer, then board.
	replayAt *replayView
}

// reset empties the board, which is what seeking backwards in a replay needs: a
// board is an accumulation and there is nothing to subtract from it.
func (b *Board) reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.running = map[string]inflight{}
	b.done, b.ok = 0, 0
	b.recent, b.failed = nil, nil
	b.buckets, b.bucket0 = nil, time.Time{}
	b.hist = make([]int, len(histEdges)+1)
	b.histSecs = make([]int, len(histEdges)+1)
	b.byFamily = map[string]*tally{}
	// The lanes and the slots are the run's shape rather than its progress, so
	// they are rebuilt empty but keep their membership: a slot that existed at
	// the end existed at the start.
	// From laneOf, which every slot registers itself in when it opens -- bySlot
	// only exists once a case has finished, so using it would show no slots at
	// all for the first minutes of a run.
	for slot := range b.laneOf {
		b.bySlot[slot] = &tally{}
	}
	for lane := range b.byLane {
		b.byLane[lane] = &tally{}
	}
	b.started = time.Now()
}

// tally is what is known about a group of cases without keeping the cases.
type tally struct {
	Done int
	OK   int
	Secs int
	Max  int
}

// histEdges are the upper bounds of the duration buckets, in seconds. Chosen
// from the corpus rather than round numbers: the median is 11 s, the fixed cost
// of createdb, start, stop and delete is about 2.6, and the longest case
// measured is 227 -- so the interesting detail is between 2 and 30, and above
// 120 all that matters is that something is there.
var histEdges = []int{2, 5, 10, 20, 30, 60, 120, 300}

// bucketSpan is the resolution of the rate. Ten seconds is short enough that a
// stall shows within one screen refresh and long enough that a single case
// finishing does not read as a spike.
const bucketSpan = 10 * time.Second

// bucketMax bounds the series at half an hour of history.
const bucketMax = 180

type inflight struct {
	Case  string
	Since time.Time
}

type finished struct {
	Slot string
	Case string
	OK   bool
	Took time.Duration
	At   time.Time
}

// recentMax is how much of the run's tail the page keeps. Enough to see what
// just happened; the log is where the run is actually recorded.
const recentMax = 40

// failedMax bounds the failure list. Above this the page is not the tool for
// the job -- a run failing a thousand cases is read in the result tree -- but
// it is far enough above a bad day that the list stays complete on one.
const failedMax = 500

func New(total int) *Board {
	return &Board{started: time.Now(), total: total, running: map[string]inflight{},
		hist:     make([]int, len(histEdges)+1),
		histSecs: make([]int, len(histEdges)+1),
		byFamily: map[string]*tally{}, bySlot: map[string]*tally{},
		laneOf: map[string]string{}, byLane: map[string]*tally{}}
}

// Lane says where a slot's corpus writes go, so the page can group by it.
//
// Called once per slot when the run opens it. A slot with no lane is still a
// slot: the panel leaves it out rather than inventing a name for it.
func (b *Board) Lane(slot, lane string) {
	if b == nil || lane == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.laneOf[slot] = lane
	if b.byLane[lane] == nil {
		b.byLane[lane] = &tally{}
	}
	// And the slot itself, so it is in the by-slot table from the start.
	//
	// The table used to be built only from cases that had finished, which hid
	// exactly the slots worth looking at: the slow lane's three slots were on
	// 195, 166 and 125-second cases, so for the first three minutes of a run the
	// panel showed five slots of eight and said nothing about the other three.
	// The same shape of bug as a disk-free row that disappears when it reaches
	// zero.
	if b.bySlot[slot] == nil {
		b.bySlot[slot] = &tally{}
	}
}

// Patched records that a case ran against a compatibility patch, and which one.
// A verdict from patched source is a claim about the patched case, not about the
// corpus, and every place the page shows the verdict has to show that too --
// including which patch, because "patched" without a name is a caveat the reader
// cannot follow up.
func (b *Board) Patched(name, patch string) {
	if b == nil || name == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.patched == nil {
		b.patched = map[string]string{}
	}
	b.patched[name] = patch
}

// Refused records that a patch would not apply, so the case ran as neither the
// corpus nor the patch has it. That is a third state and the one most worth
// seeing: without it the page shows an ordinary failure and the reader has no
// way to tell that the run's own tooling is what went wrong.
func (b *Board) Refused(name, patch string) {
	if b == nil || name == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.refused == nil {
		b.refused = map[string]string{}
	}
	b.refused[name] = patch
}

// WasPatched reports the patch a case ran against, or "".
func (b *Board) WasPatched(name string) string {
	if b == nil {
		return ""
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.patched[name]
}

// Expect tells the board what the run is expected to cost, from the plan an
// earlier run wrote. Without it the page falls back to the rate so far.
func (b *Board) Expect(cases []string, planned map[string]time.Duration, slots int) {
	if b == nil || slots < 1 || len(planned) == 0 {
		return
	}
	// The median rather than the mean: this corpus's mean is more than twice its
	// median, and a case nobody measured is far more likely to be an ordinary
	// one than one of the sixty-six that own two fifths of the run.
	known := make([]time.Duration, 0, len(planned))
	for _, d := range planned {
		known = append(known, d)
	}
	sort.Slice(known, func(i, j int) bool { return known[i] < known[j] })
	typical := known[len(known)/2]
	if len(known)%2 == 0 {
		typical = (known[len(known)/2-1] + typical) / 2
	}

	var total time.Duration
	for _, c := range cases {
		if d, ok := planned[c]; ok {
			total += d
		} else {
			total += typical
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.planned, b.typical, b.workLeft, b.slots = planned, typical, total, slots
}

func (b *Board) Begin(slot, name string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.running[slot] = inflight{Case: name, Since: time.Now()}
}

func (b *Board) End(slot, name string, ok bool) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	took := time.Duration(0)
	if in, live := b.running[slot]; live {
		took = time.Since(in.Since)
	}
	b.end(slot, name, ok, took)
}

// endWith is End with a duration supplied rather than measured, which is what a
// replay needs: the wall clock is compressed but the durations reported are the
// ones the run really had.
func (b *Board) endWith(slot, name string, ok bool, took time.Duration) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.end(slot, name, ok, took)
}

func (b *Board) end(slot, name string, ok bool, took time.Duration) {
	if b.slots > 0 {
		expect, known := b.planned[name]
		if !known {
			expect = b.typical
		}
		if b.workLeft -= expect; b.workLeft < 0 {
			b.workLeft = 0
		}
	}
	delete(b.running, slot)
	b.done++
	b.tookSum += took
	if ok {
		b.ok++
	}
	b.recent = append(b.recent, finished{Slot: slot, Case: name, OK: ok, Took: took, At: time.Now()})
	if len(b.recent) > recentMax {
		b.recent = b.recent[len(b.recent)-recentMax:]
	}
	if !ok {
		b.failed = append(b.failed, finished{Slot: slot, Case: name, Took: took, At: time.Now()})
		if len(b.failed) > failedMax {
			b.failed = b.failed[len(b.failed)-failedMax:]
		}
	}
	secs := int(took.Seconds())
	bucket := bucketOf(secs)
	b.hist[bucket]++
	b.histSecs[bucket] += secs
	groups := map[string]map[string]*tally{familyOf(name): b.byFamily, slot: b.bySlot}
	if lane := b.laneOf[slot]; lane != "" {
		groups[lane] = b.byLane
	}
	for key, m := range groups {
		t := m[key]
		if t == nil {
			t = &tally{}
			m[key] = t
		}
		t.Done++
		t.Secs += secs
		if ok {
			t.OK++
		}
		if secs > t.Max {
			t.Max = secs
		}
	}
	b.count(time.Now())
}

func bucketOf(secs int) int {
	for i, e := range histEdges {
		if secs < e {
			return i
		}
	}
	return len(histEdges)
}

// familyOf is the corpus's own grouping: the leading path segments named the way
// the families are, _NN_something.
//
// All of them, not the first. The corpus nests two deep in places -- _06_issues
// holds _10_2h, _11_1h, _11_2h and twenty more -- and _06_issues is half of
// everything, so grouping by the first segment produced one row covering half
// the run and said nothing about where inside it the time went. Ninety-nine
// groups is a longer table and a useful one; the panel is sorted slowest-first,
// so the rows that matter are at the top.
//
// Consecutive from the start, which is what stops it descending into a case: in
// _06_issues/_14_1h/bug_bts_13649/_01_show_log_header/_01_basic_log the last two
// look like families and are directories inside one case, and the run stops at
// bug_bts_13649 because that segment does not match.
func familyOf(path string) string {
	parts := strings.Split(path, "/")
	first := -1
	for i, seg := range parts {
		if isFamilySegment(seg) {
			first = i
			break
		}
	}
	if first < 0 {
		return "(other)"
	}
	last := first
	for last+1 < len(parts) && isFamilySegment(parts[last+1]) {
		last++
	}
	return strings.Join(parts[first:last+1], "/")
}

func isFamilySegment(seg string) bool {
	return len(seg) > 3 && seg[0] == '_' && seg[1] >= '0' && seg[1] <= '9'
}

// count puts one completion in its bucket, filling the empty buckets in
// between: a gap has to appear in the series as zeroes, or a stall would draw
// as a straight line from before it to after.
func (b *Board) count(at time.Time) {
	if b.bucket0.IsZero() {
		b.bucket0 = b.started
	}
	want := int(at.Sub(b.bucket0)/bucketSpan) + 1
	for len(b.buckets) < want {
		b.buckets = append(b.buckets, 0)
	}
	b.buckets[want-1]++
	if len(b.buckets) > bucketMax {
		drop := len(b.buckets) - bucketMax
		b.buckets = b.buckets[drop:]
		b.bucket0 = b.bucket0.Add(time.Duration(drop) * bucketSpan)
	}
}

type view struct {
	Total    int         `json:"total"`
	Done     int         `json:"done"`
	OK       int         `json:"ok"`
	NOK      int         `json:"nok"`
	Elapsed  int         `json:"elapsed"`
	Remain   int         `json:"remain"`
	Slots    []slotView  `json:"slots"`
	Recent   []doneView  `json:"recent"`
	Failed   []doneView  `json:"failed"`
	Rate     []int       `json:"rate"`
	RateSpan int         `json:"rateSpan"`
	Hist     []int       `json:"hist"`
	HistEdge []int       `json:"histEdge"`
	HistSecs []int       `json:"histSecs"`
	Family   []groupView `json:"family"`
	Slot     []groupView `json:"slot"`
	Lanes    []laneView  `json:"lanes"`
	// Setup is how the run was configured. Static, and sent with every snapshot
	// because a reader who opens the page an hour in needs it too.
	Setup []Setting `json:"setup,omitempty"`
	// NPatched is how many finished cases ran against a compatibility patch, and
	// NRefused how many had one that would not apply.
	NPatched int `json:"npatched,omitempty"`
	NRefused int `json:"nrefused,omitempty"`
	// Templates is nil unless the run uses the database-template cache, and the
	// page leaves the panel out when it is.
	Templates *templateView `json:"templates,omitempty"`
	Machine   machineView   `json:"machine"`
	Finished  bool          `json:"finished"`
	Replaying bool          `json:"replaying,omitempty"`
	Replay    *replayView   `json:"replay,omitempty"`
}

type slotView struct {
	Slot string `json:"slot"`
	Lane string `json:"lane"`
	Case string `json:"case"`
	Held int    `json:"held"`
	// Plan is what the case took last time, so a case that is running far past
	// it is visible as it happens. Observed: a six-second case held a slot for
	// 325 seconds and looked, on a table that lists only busy slots, exactly
	// like a slot that was never given anything.
	Plan int `json:"plan,omitempty"`
}

// laneView is one lane added up. Share is the fraction of the run's case-seconds
// that ran in it, which is the number a lane split exists to change: a lane on
// memory that holds 5% of the seconds is memory spent where it does not pay.
type laneView struct {
	Name string `json:"name"`
	// Slots is which slots are in this lane, compacted into ranges. A count
	// alone does not answer the question the panel is read for -- when a slot is
	// stuck, which lane is it in.
	Slots  string `json:"slots"`
	NSlots int    `json:"nslots"`
	Done   int    `json:"done"`
	NOK    int    `json:"nok"`
	Secs   int    `json:"secs"`
	Share  int    `json:"share"`
}

// groupView is a family or a slot, added up.
type groupView struct {
	Name string `json:"name"`
	Lane string `json:"lane,omitempty"`
	Done int    `json:"done"`
	OK   int    `json:"ok"`
	NOK  int    `json:"nok"`
	Secs int    `json:"secs"`
	Max  int    `json:"max"`
}

type doneView struct {
	Slot string `json:"slot"`
	Case string `json:"case"`
	OK   bool   `json:"ok"`
	Took int    `json:"took"`
	// Patch is the compatibility patch this case ran against, and Refused says
	// the patch would not apply -- so the case ran as neither the corpus nor the
	// patch has it. Both are different claims from an ordinary verdict, and
	// naming the patch matters: "patched" without a name is a caveat the reader
	// cannot follow up.
	Patch   string `json:"patch,omitempty"`
	Refused bool   `json:"refused,omitempty"`
}

func (b *Board) snapshot() view {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(b.started)
	v := view{
		Total: b.total, Done: b.done, OK: b.ok, NOK: b.done - b.ok,
		Elapsed:   int(elapsed.Seconds()),
		Finished:  b.done >= b.total && len(b.running) == 0,
		Replaying: b.replaying,
	}
	// What is left, from what the run already knows it is.
	//
	// The rate so far is a bad estimate and ordering made it worse rather than
	// better: with the longest cases first, the opening rate is the worst the run
	// will ever have, and two cases into a 3,444-case run the page said 82 hours
	// where the answer was 2.2. The old comment claimed ordering was the fix for
	// this. It inverted the bias instead.
	//
	// A run with a plan does not have to guess. It knows what every case took
	// last time, so what is left is the planned seconds of the cases still to
	// come, divided by the slots that will run them -- which is the same
	// arithmetic the whole schedule already lands within 2% of.
	//
	// A run without one has to average, and the average has to be of case
	// durations, not of wall clock per case. Dividing elapsed by done looks like
	// the same thing and is not: while nothing finishes, its numerator keeps
	// growing and its denominator does not, so the estimate climbs at
	// (total-done)/done seconds per second. Measured on a live 22-case run
	// stalled at 13 done, the page added 14 s of remaining work for every 20 s
	// of clock -- elapsed and remaining rising together, which is the one thing
	// a countdown must never do.
	//
	// Either way the cases in flight have already paid part of their bill, so
	// take off what they have spent. That is what makes the number fall between
	// case ends rather than sit still and then jump.
	per, left := time.Duration(0), time.Duration(0)
	switch {
	case b.slots > 0 && b.workLeft > 0:
		per, left = b.typical, b.workLeft
	case b.done > 0 && b.total > b.done:
		per = b.tookSum / time.Duration(b.done)
		left = per * time.Duration(b.total-b.done)
	}
	if left > 0 {
		for _, in := range b.running {
			spent := now.Sub(in.Since)
			if expect, known := b.planned[in.Case]; known && expect > 0 {
				per = expect
			}
			if per > 0 && spent > per {
				spent = per // a case past its estimate owes nothing more we can name
			}
			if left -= spent; left <= 0 {
				left = 0
				break
			}
		}
		slots := b.slots
		if slots < 1 {
			slots = 1
		}
		v.Remain = int((left / time.Duration(slots)).Seconds())
	}
	// Every slot the run has, not only the busy ones. A table of busy slots
	// makes a slot between cases indistinguishable from a slot that is stuck,
	// and the same slots stay listed while the others come and go -- which reads
	// as "those slots never get anything" when it is the opposite.
	// Every slot the run has: the ones that registered a lane when they opened,
	// and any that are running without having done so. bySlot is not the source
	// -- it only exists once a case has finished, so it would show nothing at all
	// for the first minutes of a run.
	seen := make(map[string]bool, len(b.laneOf)+len(b.running))
	for slot := range b.laneOf {
		seen[slot] = true
	}
	for slot := range b.running {
		seen[slot] = true
	}
	for slot := range seen {
		in, busy := b.running[slot]
		row := slotView{Slot: slot, Lane: b.laneOf[slot]}
		if busy {
			row.Case = in.Case
			row.Held = int(time.Since(in.Since).Seconds())
			row.Plan = int(b.planned[in.Case].Seconds())
		}
		v.Slots = append(v.Slots, row)
	}
	sort.Slice(v.Slots, func(i, j int) bool { return slotLess(v.Slots[i].Slot, v.Slots[j].Slot) })
	// The series runs to now, not to the last completion, so a stall is visible
	// as it happens rather than only once something finishes.
	b.count(time.Now())
	b.buckets[len(b.buckets)-1]--
	for i := len(b.failed) - 1; i >= 0; i-- {
		f := b.failed[i]
		v.Failed = append(v.Failed, doneView{
			Slot: f.Slot, Case: f.Case, OK: false, Took: int(f.Took.Seconds()),
			Patch: b.patchOf(f.Case), Refused: b.refused[f.Case] != "",
		})
	}
	v.Rate = append([]int(nil), b.buckets...)
	v.RateSpan = int(bucketSpan.Seconds())
	v.Hist = append([]int(nil), b.hist...)
	v.HistSecs = append([]int(nil), b.histSecs...)
	v.HistEdge = append([]int(nil), histEdges...)
	v.Family = groups(b.byFamily)
	v.Slot = groups(b.bySlot)
	// By name, not by seconds: the lanes are contiguous blocks of slots, so
	// name order groups them and a slot does not move as it works. The family
	// table wants the slowest first; this one wants to sit still.
	sort.Slice(v.Slot, func(i, j int) bool { return slotLess(v.Slot[i].Name, v.Slot[j].Name) })
	for i := range v.Slot {
		v.Slot[i].Lane = b.laneOf[v.Slot[i].Name]
	}
	v.Lanes = b.lanes()
	v.Setup = b.setupRows()
	v.NPatched = len(b.patched)
	v.NRefused = len(b.refused)
	v.Templates = b.templates.snapshot()
	v.Replay = b.replayAt
	v.Machine = b.sampler.snapshot()
	for i := len(b.recent) - 1; i >= 0; i-- {
		f := b.recent[i]
		v.Recent = append(v.Recent, doneView{
			Slot: f.Slot, Case: f.Case, OK: f.OK, Took: int(f.Took.Seconds()),
			Patch: b.patchOf(f.Case), Refused: b.refused[f.Case] != "",
		})
	}
	return v
}

// Open starts the page, and moves along when the default port is taken.
//
// Both suites want the same behaviour and had the same loop: a second run on
// one machine should find a free port rather than fail, while an address the
// operator pinned is theirs and is not second-guessed.
func (b *Board) Open(addr string) (string, func(), error) {
	where, stop, err := b.Serve(addr)
	if err != nil && addr == DefaultAddr {
		for try := 1; try <= 16 && err != nil; try++ {
			where, stop, err = b.Serve(NearDefault(try))
		}
	}
	return where, stop, err
}

// Serve starts the page and returns the address it is on and a way to stop it.
//
// The listener is opened before returning, so a port already in use is an error
// the caller sees rather than a page that silently never appears.
func (b *Board) Serve(addr string) (string, func(), error) {
	if b == nil {
		return "", func() {}, nil
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", nil, fmt.Errorf("status page: %w", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/case", b.serveDetail)
	mux.HandleFunc("/live", b.serveLive)
	// The replay's knobs. A GET so the page can drive it with fetch and nothing
	// else has to exist.
	mux.HandleFunc("/replay", func(w http.ResponseWriter, r *http.Request) {
		b.mu.Lock()
		rp := b.replay
		b.mu.Unlock()
		if rp == nil {
			http.Error(w, "this board is not a replay", http.StatusNotFound)
			return
		}
		rp.control(r.URL.Query())
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rp.view())
	})
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(b.snapshot())
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	return ln.Addr().String(), func() { _ = srv.Close() }, nil
}

// Watch tells the board where the run's disk and memory actually are.
//
// diskDir must not be the corpus directory once that has an overlay on it: a
// statfs there answers for the upper layer, which is the tmpfs, so the panel
// reported the same filesystem twice and called one of them disk. It is the
// install instead, which is on the disk the run actually writes to.
func (b *Board) Watch(diskDir, ramDir string, ramCapMB int) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.corpusDir, b.ramDir, b.ramCap = diskDir, ramDir, ramCapMB
	b.sampler = newSampler(diskDir, ramDir, ramCapMB)
}

// groups sorts the tallies slowest-first: the question is which family or slot
// is costing the run, and an alphabetical list does not answer it.
func groups(m map[string]*tally) []groupView {
	out := make([]groupView, 0, len(m))
	for name, t := range m {
		out = append(out, groupView{
			Name: name, Done: t.Done, OK: t.OK, NOK: t.Done - t.OK,
			Secs: t.Secs, Max: t.Max,
		})
	}
	// Slowest first, and by name where that ties. sort.Slice is not stable, so
	// without the second key equal rows swap places on every poll and a table
	// that is refreshed once a second never sits still.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Secs != out[j].Secs {
			return out[i].Secs > out[j].Secs
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func freeMB(dir string) int {
	if dir == "" {
		return 0
	}
	var st syscall.Statfs_t
	if syscall.Statfs(dir, &st) != nil {
		return 0
	}
	return int(uint64(st.Bavail) * uint64(st.Bsize) / (1 << 20))
}

func usedMB(dir string) int {
	if dir == "" {
		return 0
	}
	var st syscall.Statfs_t
	if syscall.Statfs(dir, &st) != nil {
		return 0
	}
	return int((uint64(st.Blocks) - uint64(st.Bavail)) * uint64(st.Bsize) / (1 << 20))
}

// lanes adds the run up by lane, with each lane's share of the case-seconds.
//
// Ordered by share, largest first, and by name where that ties -- the same
// reason the family table needs a second key: the page refreshes once a second
// and sort.Slice is not stable, so tied rows would swap places on every poll.
func (b *Board) lanes() []laneView {
	if len(b.byLane) == 0 {
		return nil
	}
	members := map[string][]string{}
	for slot, lane := range b.laneOf {
		members[lane] = append(members[lane], slot)
	}
	total := 0
	for _, t := range b.byLane {
		total += t.Secs
	}
	out := make([]laneView, 0, len(b.byLane))
	for name, t := range b.byLane {
		share := 0
		if total > 0 {
			share = t.Secs * 100 / total
		}
		out = append(out, laneView{
			Name: name, Slots: compactSlots(members[name]), NSlots: len(members[name]),
			Done: t.Done, NOK: t.Done - t.OK, Secs: t.Secs, Share: share,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Secs != out[j].Secs {
			return out[i].Secs > out[j].Secs
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// slotLess orders slot0, slot1, ... slot10 the way a reader expects rather than
// the way strings sort, where slot10 comes before slot2.
func slotLess(a, b string) bool {
	na, oka := slotNum(a)
	nb, okb := slotNum(b)
	if oka && okb && na != nb {
		return na < nb
	}
	return a < b
}

func slotNum(s string) (int, bool) {
	i := len(s)
	for i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		i--
	}
	if i == len(s) {
		return 0, false
	}
	n, err := strconv.Atoi(s[i:])
	return n, err == nil
}

// compactSlots turns slot0,slot1,slot2,slot5 into "slot0-slot2, slot5".
//
// Sixteen slot names on one line is a line nobody reads, and the lanes are
// contiguous by construction -- the fast slots are the first ones -- so almost
// always this is one range a lane.
func compactSlots(names []string) string {
	if len(names) == 0 {
		return ""
	}
	sorted := append([]string(nil), names...)
	sort.Slice(sorted, func(i, j int) bool { return slotLess(sorted[i], sorted[j]) })

	var parts []string
	run := 0 // index in sorted where the current run started
	flush := func(end int) {
		if end == run {
			parts = append(parts, sorted[run])
			return
		}
		parts = append(parts, sorted[run]+"-"+sorted[end])
	}
	for i := 1; i <= len(sorted); i++ {
		contiguous := false
		if i < len(sorted) {
			a, oka := slotNum(sorted[i-1])
			b, okb := slotNum(sorted[i])
			contiguous = oka && okb && b == a+1
		}
		if !contiguous {
			flush(i - 1)
			run = i
		}
	}
	return strings.Join(parts, ", ")
}
