package shellsuite

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/dispatch"
	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
	"github.com/cubrid-systems/cubrid-testkit/internal/feedback"
	"github.com/cubrid-systems/cubrid-testkit/internal/result"
)

// Worker runs cases on one instance, one at a time, until the queue is empty.
//
// One worker owns one channel. Two workers never share, because a worker spends
// most of its life blocked inside a case, and anything else that needs to reach
// the same machine while a case is running -- the timeout monitor, above all --
// needs a connection of its own.
type Worker struct {
	EnvID   string
	Channel exec.Channel
	Queue   *dispatch.Queue
	Sink    *result.Sink
	Report  feedback.Feedback
	Options CaseOptions

	// MaxRetry is only used to decide whether the retry count is printed at all:
	// CTP appended it whenever retries were configured, even to a case that failed
	// on its first attempt and was never retried.
	MaxRetry int

	// Local drops the sweep of the user's shell scripts, which would otherwise
	// kill the case that asked for the sweep.
	Local bool

	// CheckDiskSpace runs the disk check before each case. CTP's version also took
	// two mail addresses and notified them; the notification is axis O and is
	// gone, the check is not.
	CheckDiskSpace bool
	ReserveDisk    string

	// current is the start of the case in flight, read by a Monitor on another
	// connection. Zero means no case is running.
	mu      sync.Mutex
	current time.Time
	timedIn string
	timeout bool
}

// envIdentify is the string feedback records a case against.
func (w *Worker) envIdentify() string {
	return fmt.Sprintf("EnvId=%s[%s]", w.EnvID, w.Channel.Describe())
}

// Run is the worker loop.
func (w *Worker) Run(ctx context.Context) error {
	w.Sink.EnvStart(w.EnvID)
	defer func() {
		w.Report.EnvStop(w.EnvID)
		w.Sink.EnvStop(w.EnvID)
	}()

	for {
		ticket, ok := w.Queue.Claim()
		if !ok {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		c, err := Split(ticket.Case)
		if err != nil {
			// Discovery cannot produce such a path, so this is a case list edited
			// by hand or a continue file from another run. Fail it rather than
			// dropping it silently.
			w.finish(ticket, Verdict{Items: []string{resultItem("NOK", err.Error())}}, 0)
			continue
		}

		w.Report.CaseStart(ticket.Case, w.envIdentify())
		w.log("[TESTCASE] " + ticket.Case)

		start := time.Now()
		w.beginCase(start, ticket.Case)
		items, console := w.runOne(ctx, c)
		elapsed := time.Since(start)
		w.endCase()

		v := verdictOf(items)
		if w.tookTooLong() {
			v.Success = false
			v.Items = append(v.Items, resultItem("NOK", "timeout"))
		}
		if !v.Success {
			v.Items = append(v.Items,
				"============================= CONSOLE OUTPUT =============================",
				console)
		}
		w.finish(ticket, v, elapsed)
	}
}

// runOne is one attempt at one case: put the machine back to a known state, run
// the case, look for what it did not report, and read what it did.
func (w *Worker) runOne(ctx context.Context, c Case) (items []string, console string) {
	add := func(flag, msg string) { items = append(items, resultItem(flag, msg)) }

	w.quietly(ctx, KillScript(w.Local), "CLEAN PROCESSES:", "Fail to reset processes")
	w.quietly(ctx, RestoreScript(), "Reset CUBRID:", "Fail to reset CUBRID")
	if w.CheckDiskSpace {
		w.diskSpace(ctx)
	}

	res, err := runIn(ctx, w.Channel, RunScript(c, w.Options))
	if err != nil {
		add("NOK", "Runtime error ("+err.Error()+")")
		return items, console
	}
	console = res.Output()
	w.log(console)

	// do_check_more_errors appends what it finds to the case's own result file, so
	// on this host its stdout is deliberately not read.
	if _, err := runIn(ctx, w.Channel, FinalCheckScript(c, w.Options)); err != nil {
		add("NOK", "Runtime error. Fail to check more errors on "+w.Channel.Describe()+": "+err.Error())
	}

	collected, err := w.collect(ctx, c)
	if err != nil {
		add("NOK", "Runtime error ("+err.Error()+")")
		return items, console
	}
	return append(items, collected...), console
}

// collect reads the result file, giving a case whose last write has not landed a
// few seconds to finish before calling the result blank.
func (w *Worker) collect(ctx context.Context, c Case) ([]string, error) {
	var text string
	for attempt := range collectAttempts {
		res, err := runIn(ctx, w.Channel, CollectScript(c))
		if err != nil {
			return nil, err
		}
		text = res.Output()
		if strings.TrimSpace(text) != "" {
			break
		}
		if attempt == collectAttempts-1 {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(collectPause):
		}
	}

	if strings.TrimSpace(text) == "" {
		return blankResultItems(c, time.Now()), nil
	}

	var items []string
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) != "" {
			items = append(items, line)
		}
	}
	return items, nil
}

// finish records the outcome and decides whether it is a verdict yet. A case
// going back for a retry has produced no verdict: it is not printed, and it is
// not written to the finished list, or a resumed run would skip it.
func (w *Worker) finish(ticket dispatch.Ticket, v Verdict, elapsed time.Duration) {
	for _, item := range v.Items {
		w.log(item)
	}

	retrying := w.Queue.Complete(ticket, v.Success, v.HasCore)
	ev := feedback.CaseStop{
		Case:       ticket.Case,
		EnvID:      w.envIdentify(),
		Success:    v.Success,
		Elapsed:    elapsed,
		ResultText: strings.Join(v.Items, "\n"),
		TimedOut:   w.timedOut(),
		HasCore:    v.HasCore,
		SkipType:   feedback.SkipTypeNo,
		RetryCount: ticket.Retry,
	}
	if retrying {
		w.Report.CaseStopRetry(ev)
		w.log("")
		return
	}

	ev.LastPassResultCont = ev.ResultText
	w.Report.CaseStop(ev)
	w.Sink.TestCase(ticket.Case, w.EnvID, v.Success, w.MaxRetry, ticket.Retry)
	if err := w.Sink.Finished(w.EnvID, ticket.Case); err != nil {
		w.log("[ERROR] cannot record the case as finished: " + err.Error())
	}
	w.log("")
}

// quietly runs a script whose output belongs in the worker log and whose failure
// is not the case's fault.
func (w *Worker) quietly(ctx context.Context, script, label, onError string) {
	res, err := runIn(ctx, w.Channel, script)
	if err != nil {
		w.log("[ERROR] " + onError + " (" + err.Error() + ")")
		return
	}
	w.log("[INFO] " + label + " " + res.Output())
}

// diskSpace calls the deployed check. CTP passed it two mail addresses and let it
// notify; that path is axis O and is not here. A failing check is logged, which
// is what CTP did with it too -- it never stopped a run.
func (w *Worker) diskSpace(ctx context.Context) {
	script := strings.Join([]string{
		"source ${init_path}/../../common/script/util_common.sh",
		fmt.Sprintf("check_disk_space `df -P $HOME | grep -v Filesystem | awk '{print $1}'` %s \"\" \"\"", w.ReserveDisk),
	}, "\n")
	start := time.Now()
	if _, err := runIn(ctx, w.Channel, script); err != nil {
		w.log("[FAIL] Check disk space FAIL on " + w.Channel.Describe())
		w.log(err.Error())
		return
	}
	w.log(fmt.Sprintf("[INFO] Check disk space PASS on %s(elapse: %d seconds)",
		w.Channel.Describe(), int(time.Since(start).Seconds())))
}

func (w *Worker) log(line string) {
	if err := w.Sink.Worker(w.EnvID, line); err != nil {
		// The worker log is where a failure is explained. Losing it is worth
		// saying out loud, but not worth abandoning the run for.
		fmt.Printf("[ERROR] cannot write the worker log for %s: %v\n", w.EnvID, err)
	}
}

func (w *Worker) beginCase(at time.Time, name string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.current, w.timedIn, w.timeout = at, name, false
}

func (w *Worker) endCase() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.current = time.Time{}
}

func (w *Worker) tookTooLong() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.timeout
}

func (w *Worker) timedOut() bool { return w.tookTooLong() }

// runningSince reports when the case in flight started, and its name. A zero time
// means nothing is running.
func (w *Worker) runningSince() (time.Time, string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.current, w.timedIn
}

func (w *Worker) markTimedOut() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.timeout = true
	w.current = time.Time{}
}
