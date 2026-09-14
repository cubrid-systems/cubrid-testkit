// Package dispatch hands test cases out, one at a time, and decides which
// failures get another attempt.
//
// The ordering it guarantees is CTP's (CUBRIDQA-1337): every case is tried once,
// and only when the whole first pass is done does anything get retried. A worker
// that finishes case 3 early does not start retrying it while case 3000 is still
// running. That is the point -- a retry is meant to tell a flaky case apart from
// a broken build, and it can only do that once the build has had its full first
// pass.
//
// CTP enforced the ordering with a condition variable, because several workers on
// several machines shared one queue and a worker that ran out of first-pass cases
// had to wait for the others. This runner uses one machine and therefore one
// worker (ADR-014), so the ordering is not enforced at all: it falls out of
// handing the list out in order and only then looking at the retries.
//
// docs/project/design/module-shell.md 3-3, 4a.
package dispatch

import (
	"strings"
	"sync"
	"time"
)

// Lane is which pool of slots a case runs in.
//
// The pools differ in where their corpus writes land: the fast lane's go to
// memory, the slow lane's to disk. Which lane a case belongs in is decided by
// how long it runs, because a case holds its database for as long as it takes
// and the fixed cost memory removes is about 0.2 s -- measured, createdb 0.24 s
// on tmpfs against 0.04 s for a copy. A five-second case gets 40% of itself
// back; a 195-second case gets 1% and holds the memory for three minutes.
//
// LaneAny is what a claimant asks for when lanes are off, and it takes whatever
// is next. It is the zero value so that a queue nobody assigned lanes to
// behaves as it always did.
type Lane int

const (
	LaneAny Lane = iota
	LaneFast
	LaneSlow
)

func (l Lane) String() string {
	switch l {
	case LaneFast:
		return "tmpfs"
	case LaneSlow:
		return "disk"
	}
	return ""
}

// Ticket is one attempt at one case.
type Ticket struct {
	Case string
	// Retry is 0 for the first attempt, and n for the nth retry.
	Retry int
}

// IsRetry reports whether this ticket is a second or later attempt.
func (t Ticket) IsRetry() bool { return t.Retry > 0 }

// Queue is the source of work.
//
// Claim never blocks: with one worker there is nothing to wait for. The mutex is
// there because Stop arrives from whichever goroutine is watching the context,
// not because two workers compete.
type Queue struct {
	mu sync.Mutex

	cases    []string
	next     int
	maxRetry int

	retryQueue []string
	retryCount map[string]int
	queued     map[string]bool

	stopped bool

	// inFlight is how many cases are running right now, across every slot. A
	// retry is only handed out when it is zero, which is what makes the retry
	// pass quiet.
	inFlight int
	// departed are the slots that have stopped asking. A retry owned by one of
	// them has to go back to the general queue or nobody would ever run it.
	departed map[string]bool

	// hard is a constraint the run must not cross; soft is a preference about
	// order. Both refuse cases, and the difference only shows at the tail: when
	// nothing the soft rule likes is left, a slot should take what remains rather
	// than idle, and when nothing the hard rule allows is left, it must wait.
	//
	// Measured, with the two treated alike: a 24-slot run finished its last 150
	// cases six at a time because every one of them was in the heavy set, and
	// eighteen slots sat idle for half an hour. Longest-first exists to prevent
	// exactly that tail; the stagger rule had rebuilt it.
	hard Policy
	soft Policy
	// running is what is in flight, which is what a policy is given to judge
	// against. A slice rather than a count because a policy asks about the cases
	// and not only how many there are.
	running []string

	// --- lanes and slot affinity -----------------------------------------
	//
	// With lanes, a slot's corpus writes go to an overlay of its own, so two
	// cases in one directory that land on two slots do not see each other's
	// work. 15 of this family's 217 directories hold more than one case, and a
	// case that set a database up for its sibling would find it gone -- so a
	// directory belongs to whichever slot claimed it first, and its remaining
	// cases are held for that slot rather than offered to the queue.
	//
	// It costs almost no scheduling freedom: 202 of the 217 directories hold
	// exactly one case, so for those "held" is empty and the queue behaves as a
	// shared queue does.
	laneOf map[string]Lane     // by directory
	owner  map[string]string   // directory -> the slot that took it
	held   map[string][]string // slot -> cases kept for it
	taken  []bool              // cases claimed or held, by index
	cursor map[Lane]int        // where each lane resumed scanning
	lanes  bool
}

// dirOf is the cases/ directory a case runs in, which is the unit a lane and a
// slot are assigned to. It is the same segment shellsuite.Split finds, kept
// here as a string operation so that dispatch does not depend on the suite.
func dirOf(casePath string) string {
	if at := strings.LastIndex(casePath, "/cases/"); at >= 0 {
		return casePath[:at+len("/cases")]
	}
	return casePath
}

// New returns a queue over cases, in the order given.
func New(cases []string, maxRetry int) *Queue {
	return &Queue{
		cases:      cases,
		maxRetry:   maxRetry,
		retryCount: map[string]int{},
		queued:     map[string]bool{},
		laneOf:     map[string]Lane{},
		departed:   map[string]bool{},
		owner:      map[string]string{},
		held:       map[string][]string{},
		taken:      make([]bool, len(cases)),
		cursor:     map[Lane]int{},
	}
}

// Assign puts each directory in a lane and turns lanes on.
//
// By directory and not by case, because a directory is the unit a slot owns:
// see the comment on Queue. A directory the map does not mention takes the fast
// lane, which is the safe default -- a case whose duration nobody measured is
// probably short, and if it is not it costs the fast lane's ceiling rather than
// correctness.
func (q *Queue) Assign(byDir map[string]Lane) {
	if len(byDir) == 0 {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for dir, lane := range byDir {
		q.laneOf[dir] = lane
	}
	q.lanes = true
}

// Affinity keeps each directory on the slot that claimed its first case, with
// no lanes: the half of Assign a runner needs when its slots differ in nothing
// but what their earlier cases left behind.
//
// sql's cases share a database per slot, and a case may lean on what an earlier
// case in its directory created -- which CTP always ran first, on the same
// connection, because it ran everything in order. Held for one slot, a
// directory still runs that way.
func (q *Queue) Affinity() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.lanes = true
}

// Lanes reports whether lanes are on.
func (q *Queue) Lanes() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.lanes
}

// LaneOfCase is which lane a case belongs to, for the caller that has to place
// its writes before it runs.
func (q *Queue) LaneOfCase(casePath string) Lane {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.lanes {
		return LaneAny
	}
	if l, ok := q.laneOf[dirOf(casePath)]; ok {
		return l
	}
	return LaneFast
}

// Total is the number of cases in the first pass.
func (q *Queue) Total() int { return len(q.cases) }

// Claim returns the next case, or reports ok false when there is no more work.
// A worker loops on it until ok is false.
func (q *Queue) Claim() (Ticket, bool) { return q.ClaimFor("", LaneAny) }

// ClaimFor returns the next case for one slot in one lane.
//
// Three things happen in order, and the order is the whole of it. A case held
// for this slot comes first, because it is a sibling of one this slot already
// ran and its state is in this slot's overlay. Then the first pass, skipping
// what belongs to the other lane. Then retries, which go back to the slot that
// owns the directory for the same reason.
// admissionPoll is how long a claimant waits before asking again when the policy
// refuses everything. Short enough that a freed slot is not idle for long, long
// enough that a refused claimant is not a spin loop.
const admissionPoll = 200 * time.Millisecond

// Policy sets the admission rules: hard is a constraint, soft a preference.
// Either may be nil.
func (q *Queue) Policy(hard, soft Policy) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.hard, q.soft = hard, soft
}

// admissible is the first waiting case the policy will admit, and where it sits.
//
// Scanning forward rather than waiting: the order's next case may be one the
// machine cannot take yet, and the slot asking is free now. A later case that
// the policy admits is a case run instead of a slot idle, and the order is a
// preference rather than a contract.
//
// When the policy admits nothing, the next case is handed out anyway if nothing
// is running. A policy that can refuse every case is a policy that can stop the
// run, and no admission rule is worth that.
func (q *Queue) admissible() (string, int, bool) {
	if q.next >= len(q.cases) {
		return "", 0, false
	}
	// What both rules allow, in the order's own preference.
	if c, at, ok := q.scanPolicy(q.hard, q.soft); ok {
		return c, at, ok
	}
	// Nothing the preference likes remains -- which is the tail, and only the
	// tail: while ordinary work is left the scan above finds some. Take what the
	// constraint allows rather than leave slots idle through it.
	if c, at, ok := q.scanPolicy(q.hard, nil); ok {
		return c, at, ok
	}
	// The constraint refuses everything. Wait -- unless nothing at all is
	// running, because a rule that can refuse every case is a rule that can stop
	// the run.
	if len(q.running) == 0 {
		return q.cases[q.next], q.next, true
	}
	return "", 0, false
}

// scanPolicy is the first case in the remaining window that every given policy
// admits. A nil policy admits everything.
func (q *Queue) scanPolicy(ps ...Policy) (string, int, bool) {
	for i := q.next; i < len(q.cases); i++ {
		ok := true
		for _, p := range ps {
			if p != nil && !p.Admit(q.cases[i], q.running) {
				ok = false
				break
			}
		}
		if ok {
			return q.cases[i], i, true
		}
	}
	return "", 0, false
}

// take removes the case at i, keeping the rest in order.
func (q *Queue) take(i int) {
	if i == q.next {
		q.next++
		return
	}
	copy(q.cases[q.next+1:i+1], q.cases[q.next:i])
	q.next++
}

func (q *Queue) start(c string) {
	q.inFlight++
	q.running = append(q.running, c)
}

func (q *Queue) stop(c string) {
	for i, r := range q.running {
		if r == c {
			q.running = append(q.running[:i], q.running[i+1:]...)
			return
		}
	}
}

// ClaimFor hands a case to a slot, waiting while the policy will not admit one.
//
// false means the run is over for this claimant, and only that. It used to mean
// only "nothing left", and adding a policy quietly gave it a second meaning --
// "not right now" -- which the worker read as the first and closed the slot for
// good. Ten of twenty-four slots died in the first seconds of a run that way.
// So a refusal waits here rather than travelling.
func (q *Queue) ClaimFor(slot string, lane Lane) (Ticket, bool) {
	for {
		t, ok, again := q.claimOnce(slot, lane)
		if !again {
			return t, ok
		}
		time.Sleep(admissionPoll)
	}
}

// claimOnce is one attempt. again reports that the policy refused while work
// remains, which is a wait rather than an answer.
func (q *Queue) claimOnce(slot string, lane Lane) (t Ticket, ok, again bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.stopped {
		return Ticket{}, false, false
	}
	if held := q.held[slot]; len(held) > 0 {
		c := held[0]
		q.held[slot] = held[1:]
		retry := q.retryCount[c]
		delete(q.queued, c)
		q.inFlight++
		return Ticket{Case: c, Retry: retry}, true, false
	}
	if !q.lanes {
		if c, at, ok := q.admissible(); ok {
			q.take(at)
			q.start(c)
			return Ticket{Case: c}, true, false
		}
		// Cases remain and the policy will not have them yet. Wait, rather than
		// tell the claimant the run is over.
		if q.next < len(q.cases) {
			return Ticket{}, false, true
		}
	} else if c, ok := q.scan(slot, lane); ok {
		q.start(c)
		return Ticket{Case: c}, true, false
	}
	// The first pass is done for this claimant, so retries come now -- but only
	// when nothing else is running.
	//
	// A retry exists to tell a flaky case from a broken build, and a case that
	// failed because eight slots were competing for memory, disk or the ceiling
	// will fail again if it is retried while they still are. Waiting for the
	// machine to go quiet is what makes the second attempt mean something
	// different from the first.
	//
	// It costs nothing to arrange: a worker that finds the first pass empty while
	// others are still busy simply stops, and the last one standing drains the
	// retries alone. Whoever completes the final case is the one that enqueued
	// the last failure, so it is always there to pick it up.
	if q.inFlight == 0 {
		for i, c := range q.retryQueue {
			if q.lanes && !q.laneMatch(c, lane) {
				continue
			}
			q.retryQueue = append(q.retryQueue[:i:i], q.retryQueue[i+1:]...)
			delete(q.queued, c)
			q.inFlight++
			return Ticket{Case: c, Retry: q.retryCount[c]}, true, false
		}
	}
	q.departed[slot] = true
	return Ticket{}, false, false
}

func (q *Queue) laneMatch(casePath string, want Lane) bool {
	if want == LaneAny {
		return true
	}
	have, ok := q.laneOf[dirOf(casePath)]
	if !ok {
		have = LaneFast
	}
	return have == want
}

// scan takes the first unclaimed case in this lane and, with it, the rest of its
// directory -- held for this slot, so no other slot can be handed a sibling.
func (q *Queue) scan(slot string, lane Lane) (string, bool) {
	for i := q.cursor[lane]; i < len(q.cases); i++ {
		if q.taken[i] || !q.laneMatch(q.cases[i], lane) {
			continue
		}
		c := q.cases[i]
		q.taken[i] = true
		q.cursor[lane] = i + 1
		dir := dirOf(c)
		q.owner[dir] = slot
		for j := i + 1; j < len(q.cases); j++ {
			if !q.taken[j] && dirOf(q.cases[j]) == dir {
				q.taken[j] = true
				q.held[slot] = append(q.held[slot], q.cases[j])
			}
		}
		return c, true
	}
	q.cursor[lane] = len(q.cases)
	return "", false
}

// Complete records the outcome of a ticket and reports whether the case will be
// tried again. A retried case has produced no verdict yet, so the caller must not
// report it as finished.
//
// hasCore suppresses the retry: a case that dumped core did not fail flakily, it
// found something, and running it again would only overwrite the evidence.
func (q *Queue) Complete(t Ticket, success, hasCore bool) (retrying bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.inFlight > 0 {
		q.inFlight--
	}
	q.stop(t.Case)
	retrying = !success && !hasCore && t.Retry < q.maxRetry
	if retrying {
		q.enqueue(t.Case, t.Retry+1)
	} else {
		delete(q.retryCount, t.Case)
		delete(q.queued, t.Case)
	}
	return retrying
}

func (q *Queue) enqueue(c string, retry int) {
	if retry > q.maxRetry {
		return
	}
	q.retryCount[c] = retry
	if q.queued[c] {
		return
	}
	q.queued[c] = true
	// A retry goes back to the slot that owns the directory, because that is
	// where the case's writes are. Sending it anywhere else would retry it
	// against a pristine corpus, which is a different test from the one that
	// failed.
	if q.lanes {
		// A retry goes to the slot that owns the directory -- unless that slot
		// has already stopped asking, in which case holding it for that slot
		// would mean nobody ever runs it.
		if owner, ok := q.owner[dirOf(c)]; ok && !q.departed[owner] {
			q.held[owner] = append(q.held[owner], c)
			return
		}
	}
	q.retryQueue = append(q.retryQueue, c)
}

// Drained reports whether a claimant arriving now would find nothing to take:
// no first-pass case left unclaimed and no retry waiting. Cases still running,
// or held for the slot that owns their directory, are not a newcomer's -- so a
// runner still bringing a slot up can stop, rather than start one to find the
// run already over.
func (q *Queue) Drained() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.stopped {
		return true
	}
	if len(q.retryQueue) > 0 {
		return false
	}
	if !q.lanes {
		return q.next >= len(q.cases)
	}
	for _, t := range q.taken {
		if !t {
			return false
		}
	}
	return true
}

// Finished reports whether all work is done.
func (q *Queue) Finished() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.stopped {
		return true
	}
	if len(q.retryQueue) > 0 || q.inFlight > 0 {
		return false
	}
	for _, held := range q.held {
		if len(held) > 0 {
			return false
		}
	}
	if !q.lanes {
		return q.next >= len(q.cases)
	}
	for _, t := range q.taken {
		if !t {
			return false
		}
	}
	return true
}

// Stop ends the queue without finishing the remaining cases. It is how a
// cancelled context reaches a worker between claims.
func (q *Queue) Stop() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.stopped = true
}
