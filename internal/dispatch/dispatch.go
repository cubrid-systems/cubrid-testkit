// Package dispatch hands test cases out to workers, one at a time, and decides
// which failures get another attempt.
//
// The shape it reproduces is CTP's Dispatch (CUBRIDQA-1337): every case is tried
// once, and only when the whole first pass has finished does anything get retried.
// A worker that finishes case 3 early does not start retrying it while case 3000
// is still running. That ordering is the point -- a retry is meant to distinguish
// a flaky case from a broken build, and it can only do that once the build has
// had its full first pass.
//
// docs/design/module-shell.md 3-3.
package dispatch

import "sync"

// Ticket is one attempt at one case.
type Ticket struct {
	Case string
	// Retry is 0 for the first attempt, and n for the nth retry.
	Retry int
}

// IsRetry reports whether this ticket is a second or later attempt.
func (t Ticket) IsRetry() bool { return t.Retry > 0 }

// Queue is the single source of work for all workers. Safe for concurrent use.
type Queue struct {
	mu   sync.Mutex
	cond *sync.Cond

	cases    []string
	next     int
	maxRetry int

	// completed counts first-pass completions only. Retries do not advance it,
	// which is what keeps the two passes from overlapping.
	completed int

	retryQueue []string
	retryCount map[string]int
	queued     map[string]bool
	inFlight   map[string]bool

	finished bool
	stopped  bool
}

// New returns a queue over cases, in the order given.
func New(cases []string, maxRetry int) *Queue {
	q := &Queue{
		cases:      cases,
		maxRetry:   maxRetry,
		retryCount: map[string]int{},
		queued:     map[string]bool{},
		inFlight:   map[string]bool{},
	}
	q.cond = sync.NewCond(&q.mu)
	if len(cases) == 0 {
		q.finished = true
	}
	return q
}

// Total is the number of cases in the first pass.
func (q *Queue) Total() int { return len(q.cases) }

// Claim blocks until a case is available and returns it, or reports ok false when
// there is no more work. A worker loops on it until ok is false.
func (q *Queue) Claim() (Ticket, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for !q.finished && !q.stopped {
		if q.next < len(q.cases) {
			c := q.cases[q.next]
			q.next++
			return Ticket{Case: c}, true
		}

		// First pass handed out but not finished: wait rather than start retrying.
		if q.completed < len(q.cases) {
			q.cond.Wait()
			continue
		}

		if len(q.retryQueue) > 0 {
			c := q.retryQueue[0]
			q.retryQueue = q.retryQueue[1:]
			delete(q.queued, c)
			q.inFlight[c] = true
			return Ticket{Case: c, Retry: q.retryCount[c]}, true
		}

		// A retry still running may enqueue another one, so we are not done yet.
		if len(q.inFlight) > 0 {
			q.cond.Wait()
			continue
		}

		q.finished = true
		q.cond.Broadcast()
	}
	return Ticket{}, false
}

// Complete records the outcome of a ticket and reports whether the case will be
// tried again. A retried case produces no verdict yet, so the caller must not
// report it as finished.
//
// hasCore suppresses the retry: a case that dumped core did not fail flakily, it
// found something, and running it again would only overwrite the evidence.
func (q *Queue) Complete(t Ticket, success, hasCore bool) (retrying bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if t.IsRetry() {
		delete(q.inFlight, t.Case)
	} else {
		q.completed++
	}

	retrying = !success && !hasCore && t.Retry < q.maxRetry
	if retrying {
		q.enqueue(t.Case, t.Retry+1)
	} else {
		delete(q.retryCount, t.Case)
		delete(q.queued, t.Case)
	}

	if q.completed >= len(q.cases) && len(q.retryQueue) == 0 && len(q.inFlight) == 0 {
		q.finished = true
	}
	q.cond.Broadcast()
	return retrying
}

func (q *Queue) enqueue(c string, retry int) {
	if retry > q.maxRetry {
		return
	}
	q.retryCount[c] = retry
	if q.queued[c] || q.inFlight[c] {
		return
	}
	q.retryQueue = append(q.retryQueue, c)
	q.queued[c] = true
	q.finished = false
}

// Finished reports whether all work is done.
func (q *Queue) Finished() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.finished
}

// Stop releases every waiting worker without finishing the remaining cases. It is
// how a cancelled context reaches workers blocked in Claim.
func (q *Queue) Stop() {
	q.mu.Lock()
	q.stopped = true
	q.mu.Unlock()
	q.cond.Broadcast()
}
