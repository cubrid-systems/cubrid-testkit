package feedback

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// The lines below are the ones CTP's isolation FeedbackFile wrote in a real run
// (docs/project/evidence/isolation-baseline.md §2), with the times masked.
func TestIsolationSaysItsOwnLines(t *testing.T) {
	dir := t.TempDir()
	out := &bytes.Buffer{}
	f, err := OpenIsolation(dir, "isolation", out, false)
	if err != nil {
		t.Fatal(err)
	}

	f.TaskStart("")
	f.TotalTestCase(2, 0, 1)
	f.CaseStop(CaseStop{Case: "/s/a/skipped.ctl", Elapsed: -time.Millisecond, SkipType: SkipTypeByTemp})
	f.CaseStop(CaseStop{Case: "/s/a/pass.ctl", EnvID: "EnvId=local[local]", Success: true, Elapsed: 595 * time.Millisecond})
	f.CaseStop(CaseStop{Case: "/s/a/fail.ctl", EnvID: "EnvId=local[local]", Elapsed: 2746 * time.Millisecond,
		ResultText: "=== D I F F ===\n| a\t| b\r\n"})
	f.TaskStop()
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	got := read(t, dir, "feedback.log")
	got = regexp.MustCompile(`Current Time is .*`).ReplaceAllString(got, "Current Time is <date>")
	got = regexp.MustCompile(`Elapse Time:\d+`).ReplaceAllString(got, "Elapse Time:<ms>")
	want := strings.Join([]string{
		"[TASK START] Current Time is <date>",
		"Test Category:isolation",
		"The Number of Test Cases: 2 (macro skipped: 0, bug skipped: 1)",
		"[SKIP_BY_BUG] /s/a/skipped.ctl -1 ",
		"",
		"",
		"[OK] /s/a/pass.ctl 595 EnvId=local[local]",
		"",
		"",
		"[NOK] /s/a/fail.ctl 2746 EnvId=local[local]",
		"=== D I F F ===",
		"| a\t| b\r",
		"",
		"",
		"============= PRINT SUMMARY ==================",
		"Test Category:isolation",
		"Total Case:3",
		"Total Execution Case:2",
		"Total Success Case:1",
		"Total Fail Case:1",
		"Total Skip Case:1",
		"[TEST STOP] Current Time is <date>",
		"Elapse Time:<ms>",
		"",
	}, "\n")
	if got != want {
		t.Errorf("feedback.log:\n%s\nwant:\n%s", got, want)
	}

	// The console says "The category:" where the log says "Test Category:".
	if !strings.Contains(out.String(), "The category:isolation\nThe Number of Test Cases: 2 (macro skipped: 0, bug skipped: 1)\n") {
		t.Errorf("console:\n%s", out.String())
	}

	// Two files, not four.
	for _, name := range []string{"current_task_id", "test-isolation.xml"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s was written; the isolation module never wrote it", name)
		}
	}
	if status := read(t, dir, "test_status.data"); !strings.Contains(status, "total_fail_case_count=1\n") {
		t.Errorf("test_status.data:\n%s", status)
	}
}

func TestIsolationContinuesWithoutATaskID(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "test_status.data"),
		[]byte("total_success_case_count=4\ntotal_fail_case_count=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := OpenIsolation(dir, "isolation", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	f.TaskContinue()
	f.Close()

	got := read(t, dir, "feedback.log")
	if strings.Contains(got, "[Task Id]") || !strings.HasPrefix(got, "[TASK CONTINUE] Current Time is ") {
		t.Errorf("feedback.log:\n%s", got)
	}
	if s := f.Stats(); s.Success != 4 || s.Fail != 1 {
		t.Errorf("the counters were not picked up: %+v", s)
	}
}
