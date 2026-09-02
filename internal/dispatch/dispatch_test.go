package dispatch

import (
	"sync"
	"testing"
)

func drain(t *testing.T, q *Queue, outcome func(Ticket) (success, core bool)) []Ticket {
	t.Helper()
	var seen []Ticket
	for {
		tk, ok := q.Claim()
		if !ok {
			return seen
		}
		seen = append(seen, tk)
		s, c := outcome(tk)
		q.Complete(tk, s, c)
	}
}

// The whole reason retries are queued rather than run immediately: a retry is
// meant to tell a flaky case apart from a broken build, and it can only do that
// after the build has had one clean pass at everything.
func TestFirstPassCompletesBeforeAnyRetry(t *testing.T) {
	q := New([]string{"a", "b", "c"}, 1)

	var order []string
	seen := drain(t, q, func(tk Ticket) (bool, bool) {
		if tk.IsRetry() {
			order = append(order, "retry:"+tk.Case)
		} else {
			order = append(order, "first:"+tk.Case)
		}
		return tk.Case != "a", false // "a" always fails
	})

	want := []string{"first:a", "first:b", "first:c", "retry:a"}
	if len(order) != len(want) {
		t.Fatalf("got %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("attempt %d: got %q, want %q (full: %v)", i, order[i], want[i], order)
		}
	}
	if seen[3].Retry != 1 {
		t.Errorf("retry ticket carries Retry=%d, want 1", seen[3].Retry)
	}
}

// A case that dumped core did not fail flakily, it found something. Running it
// again would overwrite the core we came for.
func TestACoreSuppressesTheRetry(t *testing.T) {
	q := New([]string{"crasher"}, 3)

	tk, ok := q.Claim()
	if !ok {
		t.Fatal("nothing to claim")
	}
	if retrying := q.Complete(tk, false, true); retrying {
		t.Error("a failure with a core was queued for retry")
	}
	if _, ok := q.Claim(); ok {
		t.Error("claimed again after a core; the case should be done")
	}
}

func TestRetriesStopAtMaxRetry(t *testing.T) {
	for _, max := range []int{0, 1, 2, 5} {
		q := New([]string{"always-fails"}, max)
		attempts := len(drain(t, q, func(Ticket) (bool, bool) { return false, false }))
		if want := max + 1; attempts != want {
			t.Errorf("maxRetry=%d: %d attempts, want %d", max, attempts, want)
		}
	}
}

func TestAnEmptyQueueIsFinishedImmediately(t *testing.T) {
	q := New(nil, 3)
	if !q.Finished() {
		t.Error("an empty queue is not finished")
	}
	if _, ok := q.Claim(); ok {
		t.Error("claimed a case from an empty queue")
	}
}

// Run under -race: every case goes to exactly one worker, and the queue drains.
func TestConcurrentWorkersEachTakeACaseOnce(t *testing.T) {
	cases := make([]string, 200)
	for i := range cases {
		cases[i] = string(rune('a'+i%26)) + string(rune('0'+i/26))
	}
	q := New(cases, 1)

	var mu sync.Mutex
	firstPass := map[string]int{}

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				tk, ok := q.Claim()
				if !ok {
					return
				}
				mu.Lock()
				if !tk.IsRetry() {
					firstPass[tk.Case]++
				}
				mu.Unlock()
				q.Complete(tk, true, false)
			}
		}()
	}
	wg.Wait()

	if len(firstPass) != len(cases) {
		t.Fatalf("%d distinct cases ran, want %d", len(firstPass), len(cases))
	}
	for c, n := range firstPass {
		if n != 1 {
			t.Errorf("case %q ran %d times in the first pass", c, n)
		}
	}
	if !q.Finished() {
		t.Error("queue drained but not marked finished")
	}
}

func TestStopReleasesWaitingWorkers(t *testing.T) {
	q := New([]string{"a", "b"}, 0)

	// Claim "a" and never complete it, so the second worker blocks waiting for
	// the first pass to finish.
	if _, ok := q.Claim(); !ok {
		t.Fatal("nothing to claim")
	}
	if _, ok := q.Claim(); !ok {
		t.Fatal("nothing to claim")
	}

	released := make(chan bool, 1)
	go func() {
		_, ok := q.Claim()
		released <- ok
	}()

	q.Stop()
	if ok := <-released; ok {
		t.Error("Claim returned a ticket after Stop")
	}
}
