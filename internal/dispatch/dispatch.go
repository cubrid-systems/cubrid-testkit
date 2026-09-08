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
// docs/design/module-shell.md 3-3, 4a.
package dispatch

import (
	"strings"
	"sync"
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
		return "ram"
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
func (q *Queue) ClaimFor(slot string, lane Lane) (Ticket, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.stopped {
		return Ticket{}, false
	}
	if held := q.held[slot]; len(held) > 0 {
		c := held[0]
		q.held[slot] = held[1:]
		retry := q.retryCount[c]
		delete(q.queued, c)
		return Ticket{Case: c, Retry: retry}, true
	}
	if !q.lanes {
		if q.next < len(q.cases) {
			c := q.cases[q.next]
			q.next++
			return Ticket{Case: c}, true
		}
	} else if c, ok := q.scan(slot, lane); ok {
		return Ticket{Case: c}, true
	}
	// The first pass is done for this claimant. Retries come now and not before.
	for i, c := range q.retryQueue {
		if q.lanes && !q.laneMatch(c, lane) {
			continue
		}
		q.retryQueue = append(q.retryQueue[:i:i], q.retryQueue[i+1:]...)
		delete(q.queued, c)
		return Ticket{Case: c, Retry: q.retryCount[c]}, true
	}
	return Ticket{}, false
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
		if owner, ok := q.owner[dirOf(c)]; ok {
			q.held[owner] = append(q.held[owner], c)
			return
		}
	}
	q.retryQueue = append(q.retryQueue, c)
}

// Finished reports whether all work is done.
func (q *Queue) Finished() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.stopped {
		return true
	}
	if len(q.retryQueue) > 0 {
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
