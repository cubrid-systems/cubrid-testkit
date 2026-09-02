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

import "sync"

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
}

// New returns a queue over cases, in the order given.
func New(cases []string, maxRetry int) *Queue {
	return &Queue{
		cases:      cases,
		maxRetry:   maxRetry,
		retryCount: map[string]int{},
		queued:     map[string]bool{},
	}
}

// Total is the number of cases in the first pass.
func (q *Queue) Total() int { return len(q.cases) }

// Claim returns the next case, or reports ok false when there is no more work.
// A worker loops on it until ok is false.
func (q *Queue) Claim() (Ticket, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.stopped {
		return Ticket{}, false
	}
	if q.next < len(q.cases) {
		c := q.cases[q.next]
		q.next++
		return Ticket{Case: c}, true
	}
	// The first pass is done, because the one worker asking has completed
	// everything it was handed. Retries come now and not before.
	if len(q.retryQueue) > 0 {
		c := q.retryQueue[0]
		q.retryQueue = q.retryQueue[1:]
		delete(q.queued, c)
		return Ticket{Case: c, Retry: q.retryCount[c]}, true
	}
	return Ticket{}, false
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
	q.retryQueue = append(q.retryQueue, c)
	q.queued[c] = true
}

// Finished reports whether all work is done.
func (q *Queue) Finished() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.stopped || (q.next >= len(q.cases) && len(q.retryQueue) == 0)
}

// Stop ends the queue without finishing the remaining cases. It is how a
// cancelled context reaches a worker between claims.
func (q *Queue) Stop() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.stopped = true
}
