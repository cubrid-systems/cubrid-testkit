package shellsuite

import (
	"context"
	"fmt"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
)

// monitorPoll is how often a worker is checked. CTP used three seconds, which is
// fine for a timeout measured in minutes and cheap enough to leave alone.
const monitorPoll = 3 * time.Second

// Monitor stops a case that has run too long.
//
// It does not cancel the command. Cancelling would close the channel and leave
// whatever the case started still running on the far machine -- a cub_server
// holding a port, a csql holding a lock -- and the next case would fail for
// reasons that have nothing to do with it. Instead the monitor kills the
// processes the case is waiting on, over its own connection, and the case's
// command returns on its own because there is nothing left to wait for.
//
// That is why a Monitor needs a channel of its own: the worker's is busy being
// the case.
type Monitor struct {
	Worker  *Worker
	Channel exec.Channel

	// Timeout is per case. Zero or negative disables the monitor, which is what a
	// missing testcase_timeout_in_secs meant.
	Timeout time.Duration

	// Local matches the worker's setting, so the kill has the same reach.
	Local bool
}

// Watch polls until the context ends.
func (m *Monitor) Watch(ctx context.Context) {
	if m.Timeout <= 0 {
		return
	}
	ticker := time.NewTicker(monitorPoll)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.check(ctx)
		}
	}
}

func (m *Monitor) check(ctx context.Context) {
	since, name := m.Worker.runningSince()
	if since.IsZero() {
		return
	}
	elapsed := time.Since(since)
	if elapsed < m.Timeout {
		return
	}

	// Mark first. The kill makes the worker's command return, and if the flag
	// arrived afterwards the case could be recorded as a pass on its way out.
	m.Worker.markTimedOut()

	res, err := m.Channel.Run(ctx, KillScript(m.Local))
	cleaned := ""
	if err != nil {
		cleaned = "fail to reset processes: " + err.Error()
	} else {
		cleaned = res.Combined()
	}

	action := fmt.Sprintf("[RESOLVE] %d + timeout (actual: %d seconds)\nCLEAN PROCESSES: \n%s",
		int(m.Timeout.Seconds()), int(elapsed.Seconds()), cleaned)
	m.Worker.Report.CaseMonitor(name, action, m.Worker.envIdentify())
}
