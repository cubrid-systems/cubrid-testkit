package shellsuite

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/dispatch"
	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
	"github.com/cubrid-systems/cubrid-testkit/internal/feedback"
	"github.com/cubrid-systems/cubrid-testkit/internal/patch"
	"github.com/cubrid-systems/cubrid-testkit/internal/plan"
	"github.com/cubrid-systems/cubrid-testkit/internal/result"
	"github.com/cubrid-systems/cubrid-testkit/internal/status"
	"path/filepath"
)

// Worker runs cases on one instance, one at a time, until the queue is empty.
//
// One worker owns one channel. Two workers never share, because a worker spends
// most of its life blocked inside a case, and anything else that needs to reach
// the same machine while a case is running -- the timeout monitor, above all --
// needs a connection of its own.
type Worker struct {
	EnvID string
	// SlotID names this worker on the status page. Slots share an EnvID on
	// purpose -- so that a parallel run writes the files a serial one does --
	// which makes this the only thing that tells them apart.
	SlotID string
	// Board is where this worker says what it is doing, or nil when nobody asked
	// for a status page. Every method on it tolerates nil, so no call site here
	// has to check.
	Board *status.Board
	// LaneID is which pool of slots this worker belongs to: the fast lane's
	// corpus writes go to memory, the slow lane's to disk. dispatch.LaneAny when
	// lanes are off, which is every case the queue has.
	LaneID dispatch.Lane
	// Corpus is the overlay a run's writes go to, or nil when they go to disk.
	// A case's directory is reclaimed through it when the case retires, which is
	// why the worker and not the queue owns the call: the worker is the one that
	// knows a retry is not a retirement.
	Corpus *Corpus
	// Plan is where this worker records what each case took, so the next run can
	// hand the long ones out first. Nil when no plan was asked for.
	Plan    *plan.Record
	Channel exec.Channel
	Queue   *dispatch.Queue
	Sink    *result.Sink
	Report  feedback.Feedback
	Options CaseOptions
	// Patches are the corpus changes this run carries. Nil is the ordinary case.
	Patches *patch.Set
	// Logs keeps what a case wrote, so a failure can be diagnosed without running
	// the corpus again. Nil when case_logs is off, which is the default.
	Logs *CaseLogs

	// MaxRetry is only used to decide whether the retry count is printed at all:
	// CTP appended it whenever retries were configured, even to a case that failed
	// on its first attempt and was never retried.
	MaxRetry int

	// Local drops the sweep of the user's shell scripts, which would otherwise
	// kill the case that asked for the sweep.
	Local bool

	// Contained says the run has namespaces of its own, which changes what the
	// sweep selects on -- see KillScript.
	Contained bool

	// CheckDiskSpace runs the disk check before each case. CTP's version also took
	// two mail addresses and notified them; the notification is axis O and is
	// gone, the check is not.
	CheckDiskSpace bool
	ReserveDisk    string

	// current is the start of the case in flight, read by a Monitor on another
	// connection. Zero means no case is running.
	mu      sync.Mutex
	current time.Time
	// buf holds this case's log lines until it ends, so that slots do not
	// interleave one case's trace into another's.
	buf       []string
	buffering bool
	timedIn   string
	timeout   bool
	// abort ends the case in flight. It is the monitor's last resort, and it is
	// nil whenever no case is running.
	abort context.CancelFunc
	// resolved is when the monitor first declared this case over time. Zero means
	// it has not. The monitor measures its own grace period from here.
	resolved time.Time
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
		ticket, ok := w.Queue.ClaimFor(w.SlotID, w.LaneID)
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
			w.finish(ticket, Verdict{Items: []string{resultItem("NOK", err.Error())}}, "", 0)
			continue
		}

		w.Report.CaseStart(ticket.Case, w.envIdentify())
		w.Board.Begin(w.SlotID, ticket.Case)
		// Where this case writes its verdicts while it runs, so the page can
		// show them arriving. Cleared when it finishes: the file is about to be
		// reclaimed with the rest of the directory.
		w.Board.Live(ticket.Case, filepath.Join(c.Dir, c.Result))
		w.hold()
		w.log("[TESTCASE] " + ticket.Case)

		// The case gets a context of its own so the monitor has something to pull
		// when its sweep has not freed the case. Cancelling this one ends the
		// case; cancelling the run's ends everything.
		caseCtx, abort := context.WithCancel(ctx)
		start := time.Now()
		w.beginCase(start, ticket.Case, abort)
		items, console := w.runOne(caseCtx, c)
		elapsed := time.Since(start)
		w.endCase()
		abort()

		v := verdictOf(items)
		if w.tookTooLong() {
			v.Success = false
			v.Items = append(v.Items, resultItem("NOK", "timeout"))
		}
		// Here and nowhere else. RestoreScript runs at the *start* of a case, so
		// what this one wrote is still on the machine until the next case claims
		// this slot -- and the corpus overlay drops the case's own directory when
		// its directory retires. Before the verdict there is nothing to keep;
		// after either of those there is nothing left to copy.
		if note := w.Logs.Capture(ctx, w.Channel, ticket.Case, c.Dir, ticket.Retry+1, v.Success); note != "" {
			w.log(note)
		}
		w.finish(ticket, v, console, elapsed)
	}
}

// runOne is one attempt at one case: put the machine back to a known state, run
// the case, look for what it did not report, and read what it did.
func (w *Worker) runOne(ctx context.Context, c Case) (items []string, console string) {
	add := func(flag, msg string) { items = append(items, resultItem(flag, msg)) }

	w.quietly(ctx, KillScript(w.Local, w.Contained), "CLEAN PROCESSES:", "Fail to reset processes")
	w.quietly(ctx, RestoreScript(), "Reset CUBRID:", "Fail to reset CUBRID")
	if w.CheckDiskSpace {
		w.diskSpace(ctx)
	}

	// A compatibility patch, applied into the run's overlay rather than into the
	// corpus, so the checkout is unchanged and the change is visible. Refused
	// rather than skipped when it does not fit: the case has moved, and running
	// it unpatched would answer a question nobody asked.
	// c.Path and not c.Script: Script is only the part after cases/, and a patch
	// is keyed by the case's whole path.
	if pf := w.Patches.For(c.Path); pf != "" {
		// Two different failures, and they were being reported as one. A patch
		// that does not apply exits non-zero with err nil -- Run reports a
		// command's own status in the Result, and keeps err for not being able to
		// run it at all. Checking err alone therefore called a transport hiccup
		// "the case changed upstream", and would have called a patch that really
		// did not apply a success and run the case unpatched while the run
		// claimed it was patched.
		res, perr := probeIn(ctx, w.Channel, patch.ApplyScript(c.Dir, pf))
		switch {
		case perr != nil:
			w.Board.Refused(c.Path, pf)
			add("NOK", "the compatibility patch "+pf+" could not be run: "+perr.Error())
			return items, console
		case res.ExitCode == 127:
			// A third failure that was being reported as the second. Without
			// patch(1) the shell answers 127, which the branch below reads as
			// "the case changed upstream" -- so a machine missing a tool was
			// blamed on the corpus. Measured: the CI image ships no patch(1),
			// and six cases matched a patch, ran unpatched, and said the corpus
			// had moved.
			w.Board.Refused(c.Path, pf)
			add("NOK", "the compatibility patch "+pf+" could not be applied: "+
				"patch(1) is not on this machine's PATH")
			return items, console
		case res.ExitCode != 0:
			w.Board.Refused(c.Path, pf)
			add("NOK", "the compatibility patch "+pf+" does not apply to this case any more, "+
				"which usually means the case changed upstream: "+strings.TrimSpace(res.Output()))
			return items, console
		}
		w.log("[PATCH] applied " + pf)
		w.Board.Patched(c.Path, pf)
		w.Patches.Applied(c.Path, pf)
		// Put it back. Behind the overlay the writes go anyway when the
		// directory retires, but that is a property of how the run was
		// configured, and "does the corpus come out as it went in" must not
		// have "it depends" as its answer.
		defer func() {
			res, rerr := probeIn(context.Background(), w.Channel, patch.RevertScript(c.Dir, pf))
			why := ""
			if rerr != nil {
				why = rerr.Error()
			} else if res.ExitCode != 0 {
				why = strings.TrimSpace(res.Output())
			}
			if why != "" {
				w.log("[ERROR] the compatibility patch " + pf + " could not be reverted, so " +
					c.Dir + " is left patched: " + why)
			}
		}()
	}

	// A case's exit status is not its verdict -- the verdict is what it wrote to
	// its result file, and CTP never read the status either.
	res, err := probeIn(ctx, w.Channel, RunScript(c, w.Options))
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
		// cat fails on a result file that is not there, and that is an empty
		// answer rather than an error: it is what the retries below wait out,
		// and what a blank result is made of when they run out.
		res, err := probeIn(ctx, w.Channel, CollectScript(c))
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
//
// The console output goes to feedback and not to the worker log. CTP kept two
// buffers for exactly this: resultItemList, which workerLog receives line by
// line, and resultCont, which is that list plus -- for a case that failed -- the
// banner and the console output (Test.java). Putting the console in the items
// would write it to the worker log a second time, because runOne has already
// written it there as the case produced it.
func (w *Worker) finish(ticket dispatch.Ticket, v Verdict, console string, elapsed time.Duration) {
	for _, item := range v.Items {
		w.log(item)
	}

	retrying := w.Queue.Complete(ticket, v.Success, v.HasCore)
	ev := feedback.CaseStop{
		Case:       ticket.Case,
		EnvID:      w.envIdentify(),
		Success:    v.Success,
		Elapsed:    elapsed,
		ResultText: resultText(v, console),
		TimedOut:   w.timedOut(),
		HasCore:    v.HasCore,
		SkipType:   feedback.SkipTypeNo,
		RetryCount: ticket.Retry,
	}
	if retrying {
		w.Report.CaseStopRetry(ev)
		w.log("")
		w.flush()
		return
	}

	w.Board.Live(ticket.Case, "")
	ev.LastPassResultCont = ev.ResultText
	if c, err := Split(ticket.Case); err == nil {
		// The registry outlives the files unless it is told, and a name that
		// points at a reclaimed directory fails the next case that walks it.
		if w.Corpus.Retire(w.SlotID, c.Dir) {
			if _, err := runIn(context.Background(), w.Channel, PruneRegistryScript(c.Dir)); err != nil {
				w.log("[ERROR] cannot prune the database registry for " + c.Dir + ": " + err.Error())
			}
		}
	}
	w.Plan.Add(ticket.Case, elapsed)
	w.Board.End(w.SlotID, ticket.Case, v.Success)
	w.Report.CaseStop(ev)
	w.Sink.TestCase(ticket.Case, w.EnvID, v.Success, w.MaxRetry, ticket.Retry)
	if err := w.Sink.Finished(w.EnvID, ticket.Case); err != nil {
		w.log("[ERROR] cannot record the case as finished: " + err.Error())
	}
	w.log("")
	w.flush()
}

// consoleBanner separates a failing case's result lines from the console output
// appended after them. It is F1: it goes into feedback.log and test-shell.xml.
const consoleBanner = "============================= CONSOLE OUTPUT ============================="

// resultText is what feedback receives: the result items, and for a case that
// failed, the console output after the banner. The banner follows a failure
// whether or not the case said anything, which is what CTP did.
func resultText(v Verdict, console string) string {
	text := strings.Join(v.Items, "\n")
	if v.Success {
		return text
	}
	if text != "" {
		text += "\n"
	}
	return text + consoleBanner + "\n" + console
}

// quietly runs a script whose output belongs in the worker log and whose failure
// is not the case's fault.
// quietly runs a housekeeping script and logs what it said.
//
// Both failures, not one. Run reports a command's own non-zero exit in the
// Result and keeps err for not being able to run it at all, so checking err
// alone sees a script that refused as a script that succeeded -- which is how
// the reset's own "$CUBRID is not a CUBRID installation" guard came to do
// nothing: it exits 1 and explains itself on stderr, and neither reached a log.
//
// Stderr is logged on failure for the same reason. Output() is stdout, because
// that is what the frozen logs contain; an explanation the runner discards is
// an explanation nobody has.
func (w *Worker) quietly(ctx context.Context, script, label, onError string) {
	res, err := probeIn(ctx, w.Channel, script)
	switch {
	case err != nil:
		w.log("[ERROR] " + onError + " (" + err.Error() + ")")
	case res.ExitCode != 0:
		why := strings.TrimSpace(res.Stderr)
		if why == "" {
			why = strings.TrimSpace(res.Output())
		}
		w.log("[ERROR] " + onError + " (exit " + strconv.Itoa(res.ExitCode) + "): " + why)
	default:
		w.log("[INFO] " + label + " " + res.Output())
	}
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

// log collects a line, and while a case is running it collects rather than
// writes. The case's whole trace goes out at once in flush, because slots make
// the worker log something more than one worker appends to.
func (w *Worker) log(line string) {
	w.mu.Lock()
	if w.buffering {
		w.buf = append(w.buf, line)
		w.mu.Unlock()
		return
	}
	w.mu.Unlock()
	if err := w.Sink.Worker(w.EnvID, line); err != nil {
		// The worker log is where a failure is explained. Losing it is worth
		// saying out loud, but not worth abandoning the run for.
		fmt.Printf("[ERROR] cannot write the worker log for %s: %v\n", w.EnvID, err)
	}
}

// hold starts collecting this case's lines, and flush writes them as one block.
func (w *Worker) hold() {
	w.mu.Lock()
	w.buffering = true
	w.buf = w.buf[:0]
	w.mu.Unlock()
}

func (w *Worker) flush() {
	w.mu.Lock()
	lines := append([]string(nil), w.buf...)
	w.buf = w.buf[:0]
	w.buffering = false
	w.mu.Unlock()
	if len(lines) == 0 {
		return
	}
	if err := w.Sink.WorkerLines(w.EnvID, lines); err != nil {
		fmt.Printf("[ERROR] cannot write the worker log for %s: %v\n", w.EnvID, err)
	}
}

func (w *Worker) beginCase(at time.Time, name string, abort context.CancelFunc) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.current, w.timedIn, w.timeout = at, name, false
	w.abort, w.resolved = abort, time.Time{}
}

func (w *Worker) endCase() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.current, w.abort, w.resolved = time.Time{}, nil, time.Time{}
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

// markTimedOut records that the case in flight ran past its timeout, and says
// whether this is the first time. It deliberately leaves `current` alone.
//
// It used to clear it, which stopped the monitor after one pass -- runningSince
// returned zero and every later check gave up before doing anything. CTP does not
// do that: TestMonitor.resolveTimeout leaves test.startTime set, so it resolves
// again every three seconds for as long as the case keeps running, and its
// feedback carries one entry per pass. Clearing the field was a divergence with
// no reason behind it.
func (w *Worker) markTimedOut() (first bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	first = !w.timeout
	w.timeout = true
	if first {
		w.resolved = time.Now()
	}
	return first
}

// resolvedAt reports when the monitor first resolved the case in flight, and the
// function that ends it. Both are zero when nothing is running or nothing has
// timed out.
func (w *Worker) resolvedAt() (time.Time, context.CancelFunc) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.resolved, w.abort
}
