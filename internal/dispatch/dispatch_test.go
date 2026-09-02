package dispatch

import "testing"

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

// One worker, every case once, in order. The queue used to hold a condition
// variable so several workers on several machines could share it; with one
// machine there is one worker (ADR-014), and this is what is left to check.
func TestEveryCaseIsHandedOutOnceInOrder(t *testing.T) {
	cases := []string{"a", "b", "c", "d"}
	q := New(cases, 0)

	var got []string
	for {
		tk, ok := q.Claim()
		if !ok {
			break
		}
		got = append(got, tk.Case)
		q.Complete(tk, true, false)
	}

	if len(got) != len(cases) {
		t.Fatalf("got %v, want %v", got, cases)
	}
	for i := range cases {
		if got[i] != cases[i] {
			t.Fatalf("got %v, want %v", got, cases)
		}
	}
	if !q.Finished() {
		t.Error("queue drained but not marked finished")
	}
}

// Claim does not block any more, so Stop is not a wake-up: it is how a cancelled
// context ends the run between one case and the next.
func TestStopEndsTheQueue(t *testing.T) {
	q := New([]string{"a", "b", "c"}, 0)

	tk, ok := q.Claim()
	if !ok {
		t.Fatal("nothing to claim")
	}
	q.Complete(tk, true, false)

	q.Stop()

	if _, ok := q.Claim(); ok {
		t.Error("Claim returned a ticket after Stop")
	}
	if !q.Finished() {
		t.Error("a stopped queue does not report itself finished")
	}
}

// Stop arrives from the goroutine watching the context while the worker is
// between claims, so those two do race and the mutex still has to hold. Run
// under -race.
func TestStopIsSafeAlongsideAWorker(t *testing.T) {
	q := New([]string{"a", "b", "c", "d", "e"}, 1)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			tk, ok := q.Claim()
			if !ok {
				return
			}
			q.Complete(tk, false, false)
		}
	}()
	q.Stop()
	<-done
}
