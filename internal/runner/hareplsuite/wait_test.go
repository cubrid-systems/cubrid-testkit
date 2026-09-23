package hareplsuite

import (
	"strings"
	"testing"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/conf"
	"github.com/cubrid-systems/cubrid-testkit/internal/sandbox"
)

func waitConf(t *testing.T, lines ...string) *conf.Config {
	t.Helper()
	c, err := conf.ParseText("wait.conf", strings.Join(lines, "\n"), "")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// CTP's conf key is ha_sync_detect_timeout_in_secs and it is in seconds. A conf
// written for CTP has to be read, because conf keys are F3 on the frozen
// surface -- and this runner read a key CTP never defines, so a CTP conf
// setting the real one was read as nothing at all.
func TestCTPsKeyIsReadAndIsInSeconds(t *testing.T) {
	if got := resolveWait(waitConf(t, "ha_sync_detect_timeout_in_secs=300")); got != 5*time.Minute {
		t.Errorf("300 seconds became %v, want 5m", got)
	}
}

// The spelling this runner invented still works, in milliseconds, because confs
// written against it exist and silently changing what they mean would be the
// same defect pointed the other way.
func TestTheOlderSpellingStillWorksInMilliseconds(t *testing.T) {
	if got := resolveWait(waitConf(t, "ha_sync_detect_timeout_in_ms=90000")); got != 90*time.Second {
		t.Errorf("90000 ms became %v, want 1m30s", got)
	}
}

// CTP's key wins where both are set: it is the one a conf written for CTP
// carries, and this runner is the one that has to bend.
func TestCTPsKeyWinsOverTheOlderOne(t *testing.T) {
	c := waitConf(t, "ha_sync_detect_timeout_in_secs=300", "ha_sync_detect_timeout_in_ms=1000")
	if got := resolveWait(c); got != 5*time.Minute {
		t.Errorf("got %v, want CTP's 5m rather than the other key's 1s", got)
	}
}

// The default is CTP's, not one chosen here: Constants.java:46, 600 * 1000.
// Sixty seconds -- a tenth of it -- is what turned _09_partition cases into
// wait_timeout.
func TestTheDefaultIsCTPs(t *testing.T) {
	if got := resolveWait(waitConf(t, "scenario=/tmp")); got != 10*time.Minute {
		t.Errorf("the default is %v, want CTP's 10m", got)
	}
}

// A probe of the slave is a read and must be bounded like one. The channel a
// Pair hands out carries the case bound, because everything it hands out runs
// case SQL -- but a one-row marker read is not that, and at the case bound a
// slave that stopped answering would hold the wait for ten minutes while the
// deadline it is measured against is smaller.
func TestAProbeCannotOutliveTheWaitItLivesInside(t *testing.T) {
	if sandbox.ProbeTimeout >= WaitDefault {
		t.Errorf("a probe is bounded at %v and the default wait is %v; a probe must be the smaller",
			sandbox.ProbeTimeout, WaitDefault)
	}
	if sandbox.ProbeTimeout >= sandbox.CaseTimeout {
		t.Errorf("a probe is bounded at %v, which is not smaller than the case bound %v",
			sandbox.ProbeTimeout, sandbox.CaseTimeout)
	}
}
