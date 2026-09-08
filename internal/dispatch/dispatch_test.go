package dispatch

import (
	"strings"
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

func cs(dirs ...string) []string {
	var out []string
	for _, d := range dirs {
		out = append(out, "/x/"+d+"/cases/"+d+".sh")
	}
	return out
}

// Lanes off is the old queue, and every existing test above says so. This says
// the new entry point agrees.
func TestClaimForWithoutLanesIsTheOldQueue(t *testing.T) {
	q := New(cs("a", "b", "c"), 0)
	for _, want := range cs("a", "b", "c") {
		tk, ok := q.ClaimFor("slot0", LaneSlow)
		if !ok || tk.Case != want {
			t.Fatalf("got %q %v, want %q -- an unassigned queue must ignore the lane", tk.Case, ok, want)
		}
		q.Complete(tk, true, false)
	}
	if !q.Finished() {
		t.Error("the queue is not finished after every case was claimed")
	}
}

// A slow-lane slot must never be handed a fast-lane case: its writes go to disk
// and the case's siblings, if any, are on another slot's overlay.
func TestALaneOnlyGetsItsOwnCases(t *testing.T) {
	cases := cs("short1", "long1", "short2", "long2")
	q := New(cases, 0)
	q.Assign(map[string]Lane{
		"/x/long1/cases": LaneSlow,
		"/x/long2/cases": LaneSlow,
	})
	if !q.Lanes() {
		t.Fatal("Assign did not turn lanes on")
	}
	for i := 0; i < 2; i++ {
		tk, ok := q.ClaimFor("slow0", LaneSlow)
		if !ok || !strings.Contains(tk.Case, "long") {
			t.Fatalf("the slow lane was handed %q", tk.Case)
		}
		q.Complete(tk, true, false)
	}
	if tk, ok := q.ClaimFor("slow0", LaneSlow); ok {
		t.Errorf("the slow lane got a third case: %q", tk.Case)
	}
	if q.Finished() {
		t.Error("the queue is finished with the fast lane's cases unclaimed")
	}
	for i := 0; i < 2; i++ {
		tk, ok := q.ClaimFor("fast0", LaneFast)
		if !ok || !strings.Contains(tk.Case, "short") {
			t.Fatalf("the fast lane was handed %q", tk.Case)
		}
		q.Complete(tk, true, false)
	}
	if !q.Finished() {
		t.Error("every case was claimed and the queue says it is not finished")
	}
}

// A directory the assignment does not mention takes the fast lane. An
// unmeasured case is probably short, and if it is not it costs the ceiling
// rather than correctness.
func TestAnUnassignedDirectoryIsFast(t *testing.T) {
	q := New(cs("known", "unknown"), 0)
	q.Assign(map[string]Lane{"/x/known/cases": LaneSlow})
	if got := q.LaneOfCase("/x/unknown/cases/unknown.sh"); got != LaneFast {
		t.Errorf("an unassigned directory is in lane %v, want fast", got)
	}
	if got := q.LaneOfCase("/x/known/cases/known.sh"); got != LaneSlow {
		t.Errorf("an assigned directory is in lane %v, want slow", got)
	}
}

// The reason affinity exists: 15 of the 217 directories hold more than one
// case, each slot's corpus writes go to an overlay of its own, and a case that
// set a database up for its sibling would find it gone.
func TestADirectorysCasesAllGoToOneSlot(t *testing.T) {
	cases := []string{
		"/x/multi/cases/a.sh",
		"/x/other/cases/other.sh",
		"/x/multi/cases/b.sh",
		"/x/multi/cases/c.sh",
	}
	q := New(cases, 0)
	q.Assign(map[string]Lane{"/x/multi/cases": LaneFast})

	first, ok := q.ClaimFor("slot0", LaneFast)
	if !ok || first.Case != "/x/multi/cases/a.sh" {
		t.Fatalf("first claim was %q", first.Case)
	}
	q.Complete(first, true, false)
	// The other slot must not be able to take a sibling, even though one is
	// next in the list.
	tk, ok := q.ClaimFor("slot1", LaneFast)
	if !ok || tk.Case != "/x/other/cases/other.sh" {
		t.Fatalf("slot1 was handed %q; the siblings should be held for slot0", tk.Case)
	}
	q.Complete(tk, true, false)
	if _, ok := q.ClaimFor("slot1", LaneFast); ok {
		t.Error("slot1 got a case from slot0's directory")
	}
	for _, want := range []string{"/x/multi/cases/b.sh", "/x/multi/cases/c.sh"} {
		tk, ok := q.ClaimFor("slot0", LaneFast)
		if !ok || tk.Case != want {
			t.Fatalf("slot0 got %q, want its held sibling %q", tk.Case, want)
		}
		q.Complete(tk, true, false)
	}
	if !q.Finished() {
		t.Error("all four cases were claimed and the queue says it is not finished")
	}
}

// A retry has to go back to the slot that owns the directory, because that is
// where the case's writes are. Retrying it elsewhere would run it against a
// pristine corpus, which is a different test from the one that failed.
func TestARetryGoesBackToTheOwningSlot(t *testing.T) {
	q := New(cs("a", "b"), 1)
	q.Assign(map[string]Lane{"/x/a/cases": LaneFast, "/x/b/cases": LaneFast})

	a, _ := q.ClaimFor("slot0", LaneFast)
	b, _ := q.ClaimFor("slot1", LaneFast)
	if !q.Complete(a, false, false) {
		t.Fatal("a failed case with a retry left was not retried")
	}
	q.Complete(b, true, false)

	if tk, ok := q.ClaimFor("slot1", LaneFast); ok {
		t.Errorf("slot1 was handed slot0's retry: %q", tk.Case)
	}
	tk, ok := q.ClaimFor("slot0", LaneFast)
	if !ok || tk.Case != a.Case || tk.Retry != 1 {
		t.Errorf("slot0 got %q retry=%d, want its own case back as retry 1", tk.Case, tk.Retry)
	}
}

// Held cases are work outstanding: a run that reported itself finished while a
// slot still had siblings waiting would stop with cases unrun.
func TestFinishedCountsHeldCases(t *testing.T) {
	q := New([]string{"/x/m/cases/a.sh", "/x/m/cases/b.sh"}, 0)
	q.Assign(map[string]Lane{"/x/m/cases": LaneFast})
	tk, _ := q.ClaimFor("slot0", LaneFast)
	q.Complete(tk, true, false)
	if q.Finished() {
		t.Fatal("the queue says it is finished with a sibling still held")
	}
	tk, ok := q.ClaimFor("slot0", LaneFast)
	if !ok || tk.Case != "/x/m/cases/b.sh" {
		t.Fatalf("the held sibling was not handed back: %q", tk.Case)
	}
	q.Complete(tk, true, false)
	if !q.Finished() {
		t.Error("the queue is not finished after both cases ran")
	}
}

func TestDirOf(t *testing.T) {
	for in, want := range map[string]string{
		"/x/a/cases/a.sh":           "/x/a/cases",
		"/x/a/cases/sub/a.sh":       "/x/a/cases",
		"/deep/_01/x/cases/x.sh":    "/deep/_01/x/cases",
		"nocasesegment.sh":          "nocasesegment.sh",
		"/x/foo_cases/cases/foo.sh": "/x/foo_cases/cases",
	} {
		if got := dirOf(in); got != want {
			t.Errorf("dirOf(%q) = %q, want %q", in, got, want)
		}
	}
}

// A retry exists to tell a flaky case from a broken build, and a case that
// failed because eight slots were competing for memory will fail again if it is
// retried while they still are. So a retry is handed out only when nothing else
// is running.
func TestRetriesWaitForTheMachineToGoQuiet(t *testing.T) {
	q := New(cs("a", "b"), 1)

	a, _ := q.Claim()
	b, _ := q.Claim()

	// a fails while b is still running. Its retry is queued...
	if !q.Complete(a, false, false) {
		t.Fatal("a failing case with a retry left was not retried")
	}
	// ...and must not be handed out yet: b is still going.
	if tk, ok := q.Claim(); ok {
		t.Errorf("a retry was handed out while a case was still running: %q", tk.Case)
	}
	if q.Finished() {
		t.Error("the queue reports finished with a case running and a retry pending")
	}

	// b finishes. Now the machine is quiet and the retry can go.
	q.Complete(b, true, false)
	tk, ok := q.Claim()
	if !ok || tk.Case != a.Case || tk.Retry != 1 {
		t.Fatalf("the retry was not handed out once the machine went quiet: %q %v", tk.Case, ok)
	}
	// And only one at a time: nothing else while the retry runs.
	if _, ok := q.Claim(); ok {
		t.Error("a second case was handed out during the retry pass")
	}
	q.Complete(tk, true, false)
	if !q.Finished() {
		t.Error("the queue is not finished after the retry succeeded")
	}
}

// A worker that finds the first pass empty while others are busy simply stops.
// The last one standing drains the retries, and it is always there: whoever
// completes the final case is the one that enqueued the last failure.
func TestTheLastWorkerDrainsTheRetries(t *testing.T) {
	q := New(cs("a", "b", "c"), 1)
	a, _ := q.ClaimFor("slot0", LaneAny)
	b, _ := q.ClaimFor("slot1", LaneAny)
	c, _ := q.ClaimFor("slot2", LaneAny)

	q.Complete(a, false, false) // fails, queued for retry
	// slot0 asks again: the first pass is empty and two cases are running.
	if _, ok := q.ClaimFor("slot0", LaneAny); ok {
		t.Error("slot0 took a retry while slot1 and slot2 were running")
	}
	q.Complete(b, true, false)
	if _, ok := q.ClaimFor("slot1", LaneAny); ok {
		t.Error("slot1 took a retry while slot2 was running")
	}
	// slot2 finishes last, so it is the one that drains.
	q.Complete(c, true, false)
	tk, ok := q.ClaimFor("slot2", LaneAny)
	if !ok || tk.Case != a.Case {
		t.Fatalf("the last worker did not get the retry: %q %v", tk.Case, ok)
	}
	q.Complete(tk, true, false)
	if !q.Finished() {
		t.Error("the queue is not finished after the last retry")
	}
}

// With lanes, a retry goes back to the slot that owns the directory -- but a
// slot that has already stopped asking would hold it for ever.
func TestARetryOwnedByADepartedSlotGoesBackToTheQueue(t *testing.T) {
	q := New(cs("a", "b"), 1)
	q.Assign(map[string]Lane{"/x/a/cases": LaneFast, "/x/b/cases": LaneFast})

	a, _ := q.ClaimFor("slot0", LaneFast)
	b, _ := q.ClaimFor("slot1", LaneFast)
	q.Complete(a, true, false)
	// slot0 has nothing left and stops asking.
	if _, ok := q.ClaimFor("slot0", LaneFast); ok {
		t.Fatal("slot0 was given work while slot1 was running")
	}
	// Now b fails, and b's directory is owned by slot1 -- but suppose the owner
	// had left: the general queue has to take it.
	q.Complete(b, false, false)
	tk, ok := q.ClaimFor("slot1", LaneFast)
	if !ok || tk.Case != b.Case {
		t.Fatalf("the retry did not come back: %q %v", tk.Case, ok)
	}
	q.Complete(tk, true, false)
	if !q.Finished() {
		t.Error("the queue is not finished")
	}
}
