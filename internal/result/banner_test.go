package result

import (
	"bytes"
	"testing"
	"time"
)

func TestDispatcherBanner(t *testing.T) {
	// CTP.java's task loop. The blank line before the rule is part of it, and the
	// task name is upper-cased.
	at := time.Date(2026, 9, 2, 21, 3, 13, 0, time.FixedZone("KST", 9*3600))

	var b bytes.Buffer
	TaskBanner(&b, "unittest", at)
	TaskStarted(&b, "unittest", at)
	want := "\n" +
		"====================================== UNITTEST ==========================================\n" +
		"[UNITTEST] TEST STARTED (Wed Sep 02 21:03:13 KST 2026)\n" +
		"\n"
	if b.String() != want {
		t.Errorf("got:\n%q\nwant:\n%q", b.String(), want)
	}
}

func TestElapsedTruncates(t *testing.T) {
	// CTP divides milliseconds by 1000.0 and casts to long, so 1.9 seconds is 1.
	at := time.Date(2026, 9, 2, 21, 5, 44, 0, time.FixedZone("KST", 9*3600))
	var b bytes.Buffer
	TaskEnded(&b, "shell", at, 1900*time.Millisecond)
	want := "[SHELL] TEST END (Wed Sep 02 21:05:44 KST 2026)\n" +
		"[SHELL] ELAPSE TIME: 1 seconds\n"
	if b.String() != want {
		t.Errorf("got:\n%q\nwant:\n%q", b.String(), want)
	}
}

func TestUnitTestMarkers(t *testing.T) {
	// unittest prints nothing like the shell family: step headings, a one-based
	// index, and [SUCC]/[FAIL] rather than [OK]/[NOK]. The verdict lands on the
	// same line as the case name.
	s, out := newSink(t)
	s.Step("Execute")
	s.UnitCaseStart(1, "alpha")
	s.UnitCaseVerdict(true)
	s.UnitCaseStart(2, "beta")
	s.UnitCaseVerdict(false)

	want := "=> Execute Step: \n" +
		"[TESTCASE-1] alpha [SUCC]\n" +
		"[TESTCASE-2] beta [FAIL]\n"
	if got := out.String(); got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}
