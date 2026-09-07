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

	// EscalateAfter overrides the default grace period before a case that the
	// sweep did not free is ended outright. Negative turns it off.
	EscalateAfter time.Duration
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
	first := m.Worker.markTimedOut()

	res, err := runIn(ctx, m.Channel, KillScript(m.Local))
	cleaned := ""
	if err != nil {
		cleaned = "fail to reset processes: " + err.Error()
	} else {
		cleaned = res.Output()
	}

	action := fmt.Sprintf("[RESOLVE] %d + timeout (actual: %d seconds)\nCLEAN PROCESSES: \n%s",
		int(m.Timeout.Seconds()), int(elapsed.Seconds()), cleaned)
	m.Worker.Report.CaseMonitor(name, action, m.Worker.envIdentify())

	// Never on the same pass that resolved it: the sweep is owed at least one
	// poll interval to do what it is there for.
	if !first {
		m.escalate(name, elapsed)
	}
}

// EscalateAfter is how long the monitor keeps sweeping before it ends the case
// outright. Zero takes this default; negative disables escalation, which is
// CTP's behaviour exactly.
//
// CTP has none: resolveTimeout sweeps, marks the case failed, and trusts the
// case's own command to return once what it was waiting on is dead. That holds
// until a sweep misses the thing being waited on, and then the worker blocks
// for ever.
//
// The deviation is admitted because the gate depends on it: one case in 3,452
// behaving that way stops the run, and ADR-013 cannot judge a run that does not
// finish.
const EscalateAfter = 2 * time.Minute

// escalate ends a case that the sweep has not freed.
//
// The grace period is measured from the first resolve rather than from the
// timeout, because a sweep that is going to work generally works at once, and
// this way the number means what it says: how long the case is given after
// somebody tried to stop it. It is never reached on the pass that resolved,
// however short the grace: a sweep that has not been given one poll interval
// has not been given a chance.
func (m *Monitor) escalate(name string, elapsed time.Duration) {
	grace := m.EscalateAfter
	if grace == 0 {
		grace = EscalateAfter
	}
	if grace < 0 {
		return
	}
	resolved, abort := m.Worker.resolvedAt()
	if resolved.IsZero() || abort == nil {
		return
	}
	waited := time.Since(resolved)
	if waited < grace {
		return
	}

	// Cancelling the case's context kills its process group, so what the sweep
	// could not name dies with the shell that started it.
	abort()
	m.Worker.Report.CaseMonitor(name, fmt.Sprintf(
		"[ABANDON] the case did not end %d seconds after it was resolved (actual: %d seconds); its process group was killed",
		int(waited.Seconds()), int(elapsed.Seconds())), m.Worker.envIdentify())
}
