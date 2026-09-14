package shellsuite

import (
	"strings"
	"testing"

	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
)

// A script that exits non-zero has failed, and runIn says so -- with the
// script's own reason, which exec.Result.Failure extracts.
func TestRunInTakesANonZeroExitAsAnError(t *testing.T) {
	res, err := runIn(t.Context(), exec.NewLocal(""), `echo partial; echo "the message" >&2; exit 3`)
	if err == nil || !strings.Contains(err.Error(), "exit 3: the message") {
		t.Fatalf("got %v, want the status and the reason", err)
	}
	if !strings.Contains(res.Output(), "partial") {
		t.Errorf("the output was lost with the failure: %q", res.Output())
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
