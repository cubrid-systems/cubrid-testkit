package shellsuite

import (
	"strings"
	"testing"

	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
)

// A script that exits non-zero has failed, and runIn says so. The channel keeps
// err for not being able to run a script at all, and every caller that checked
// err alone took a failed script for one that worked.
func TestRunInTakesANonZeroExitAsAnError(t *testing.T) {
	res, err := runIn(t.Context(), exec.NewLocal(""),
		`echo partial; echo "the message" >&2; echo "a line between" >&2; echo "the last word" >&2; exit 3`)
	if err == nil {
		t.Fatal("a script that exited 3 was reported as a success")
	}
	for _, want := range []string{"exit 3", "the message", "the last word"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not say %q: %v", want, err)
		}
	}
	// Bounded: a cp refused a thousand files says so a thousand times.
	if strings.Contains(err.Error(), "a line between") {
		t.Errorf("the error carries all of stderr rather than its first and last lines: %v", err)
	}
	// What it printed before it failed is still the caller's to read.
	if !strings.Contains(res.Output(), "partial") {
		t.Errorf("the output was lost with the failure: %q", res.Output())
	}
}

// A script that reports on stdout, as check_disk_space does, still gets its
// reason into the error -- the last thing it printed.
func TestRunInFallsBackToTheLastLineAScriptPrinted(t *testing.T) {
	_, err := runIn(t.Context(), exec.NewLocal(""), `echo first; echo "Usage: nothing like this"; exit 1`)
	if err == nil || !strings.Contains(err.Error(), "exit 1: Usage: nothing like this") {
		t.Errorf("got %v, want the status and the last line", err)
	}
}

// probeIn is the opt-out: the status is an answer, and it is handed back as one.
func TestProbeInHandsTheStatusBack(t *testing.T) {
	res, err := probeIn(t.Context(), exec.NewLocal(""), `echo answer; exit 1`)
	if err != nil {
		t.Fatalf("a probe's status was turned into an error: %v", err)
	}
	if res.ExitCode != 1 || !strings.Contains(res.Output(), "answer") {
		t.Errorf("got exit %d and %q, want exit 1 and the answer", res.ExitCode, res.Output())
	}
}
