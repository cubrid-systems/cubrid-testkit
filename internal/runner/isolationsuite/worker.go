package isolationsuite

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/coredump"
	"github.com/cubrid-systems/cubrid-testkit/internal/dispatch"
	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
	"github.com/cubrid-systems/cubrid-testkit/internal/feedback"
	"github.com/cubrid-systems/cubrid-testkit/internal/result"
	"github.com/cubrid-systems/cubrid-testkit/internal/runner/shellsuite"
	"github.com/cubrid-systems/cubrid-testkit/internal/sizing"
	"github.com/cubrid-systems/cubrid-testkit/internal/status"
	"github.com/cubrid-systems/cubrid-testkit/internal/topology"
)

// worker is Test.runAll for one slot: take a case, hand it to runone.sh, judge
// what came back, write it down.
//
// There is no retry here and no timeout monitor. runone.sh retries inside itself
// (-r), and its timeout3.sh bounds the one step that can hang on the engine; CTP
// commented its monitor out for that reason (TestFactory.java:191-205).
type worker struct {
	slot   string
	envID  string
	ch     exec.Channel
	queue  *dispatch.Queue
	sink   *result.Sink
	report feedback.Feedback
	opts   options
	// board is the status page, or nil; every method on it tolerates nil.
	board *status.Board
	// crashes, cores and fatals are what this slot has already been seen to
	// leave. Nothing sweeps them between cases, so what counts against a case is
	// what is new since the one before it.
	crashes map[string]bool
	cores   map[string]bool
	fatals  map[string]int
	// ctltool is the directory runone.sh works in, which is where a client's core
	// lands; CTP looked there, in $CUBRID and in the case's own directory.
	ctltool string
	// meter takes what the next run on this machine is sized by, or is nil.
	meter *sizing.Meter
}

// envIdentify is what feedback records a case against: the env and the title
// IsolationHelper gives a local machine.
func (w *worker) envIdentify() string { return "EnvId=" + w.envID + "[local]" }

func (w *worker) run(ctx context.Context) error {
	w.sink.EnvStart(w.envID)
	for {
		ticket, ok := w.queue.ClaimFor(w.slot, dispatch.LaneAny)
		if !ok {
			break
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		w.one(ctx, ticket)
	}
	w.stopService(ctx)
	w.report.EnvStop(w.envID)
	w.sink.EnvStop(w.envID)
	return nil
}

func (w *worker) one(ctx context.Context, ticket dispatch.Ticket) {
	tc := ticket.Case
	start := time.Now()
	w.report.CaseStart(tc, w.envIdentify())
	w.board.Begin(w.slot, tc)
	w.board.Live(tc, liveResult(tc))

	// One block per case: with more than one slot the worker log has more than one
	// writer, and a case's trace must not be cut into by another's.
	lines := []string{"[TESTCASE] " + tc}
	var v verdict
	first := false
	res, err := run(ctx, w.ch, runoneScript(tc, w.opts))
	if err != nil {
		v = verdict{items: []string{item("NOK", "Runtime error ("+err.Error()+")")}}
	} else {
		out := output(res)
		lines = append(lines, out)
		v = judge(out)
		first = firstAttempt(out)
	}
	// What a dying server leaves, looked for here rather than by runone.sh: its
	// own crash report, which CTP's check cannot see at all, and the core file
	// and FATAL ERROR that CTP's check would have found at the price of copying
	// the whole install into ~/error_backup (ADR-021). Asked of every case,
	// passing or failing.
	w.checkTheWreckage(ctx, tc, &v)
	// Taken where CTP took it, before the diff.
	elapsed := time.Since(start)
	if w.meter != nil {
		w.meter.Case(tc, tc, elapsed, v.ok && first)
	}
	lines = append(lines, v.items...)

	diff := ""
	if !v.ok {
		diff = w.diff(ctx, tc)
	}
	w.board.Live(tc, "")
	w.board.End(w.slot, tc, v.ok)
	w.report.CaseStop(feedback.CaseStop{
		Case:       tc,
		EnvID:      w.envIdentify(),
		Success:    v.ok,
		Elapsed:    elapsed,
		ResultText: resultText(v, diff),
		HasCore:    v.hasCore,
		SkipType:   feedback.SkipTypeNo,
	})
	// No retry count on the console, ever, in this module (Test.java:135).
	w.sink.TestCase(tc, w.envID, v.ok, 0, 0)
	lines = append(lines, "")
	if err := w.sink.WorkerLines(w.envID, lines); err != nil {
		fmt.Printf("[ERROR] cannot write the worker log for %s: %v\n", w.envID, err)
	}
	if err := w.sink.Finished(w.envID, tc); err != nil {
		fmt.Printf("[ERROR] cannot record %s as finished: %v\n", tc, err)
	}
	w.queue.Complete(ticket, v.ok, v.hasCore)
}

// checkTheWreckage fails the case if this slot's install has anything new to say
// about a server that died: a crash report, a core file, or FATAL ERROR in the
// log. The report is kept whole and a core is kept as its stack -- a core is
// gigabytes and goes with the slot, and the stack is what a reader needs.
func (w *worker) checkTheWreckage(ctx context.Context, tc string, v *verdict) {
	cubrid := os.Getenv("CUBRID")
	dir := filepath.Join(w.sink.Dir(), "crash")

	for _, c := range coredump.Crashes(ctx, w.ch, cubrid, w.crashes) {
		v.ok, v.hasCore = false, true
		v.items = append(v.items, item("NOK", "found crash report "+c.String()))
		if kept, err := coredump.Keep(ctx, w.ch, c, dir, tc); err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] %s: %v\n", w.slot, err)
		} else {
			fmt.Fprintf(os.Stderr, "[WARN] %s: the server left a crash report while %s ran: %s\n", w.slot, tc, kept)
		}
	}

	for _, core := range coredump.Cores(ctx, w.ch, w.cores, cubrid, w.ctltool, filepath.Dir(tc)) {
		v.ok, v.hasCore = false, true
		v.items = append(v.items, item("NOK", "found core file "+core))
		fmt.Fprintf(os.Stderr, "[WARN] %s: %s left a core at %s\n", w.slot, tc, core)
		if kept, err := coredump.KeepStack(ctx, w.ch, core, dir, tc); err != nil {
			fmt.Fprintf(os.Stderr, "[WARN] %s: no stack for %s: %v\n", w.slot, core, err)
		} else {
			fmt.Fprintf(os.Stderr, "[INFO] %s: its stack is at %s\n", w.slot, kept)
		}
	}

	for _, f := range coredump.Fatals(ctx, w.ch, cubrid, w.fatals) {
		v.ok, v.hasCore = false, true
		v.items = append(v.items, item("NOK", "found fatal error in "+f))
		fmt.Fprintf(os.Stderr, "[WARN] %s: %s left a fatal error in %s\n", w.slot, tc, f)
	}
}

func (w *worker) diff(ctx context.Context, tc string) string {
	res, err := run(ctx, w.ch, diffScript(tc))
	out := output(res)
	if err != nil {
		out = "DIFF ERROR: " + err.Error()
	}
	return crlf(out)
}

// stopService is Test.stopCUBRIDService, which ends every worker. The closing
// bracket is doubled in CTP's line, and is here too.
func (w *worker) stopService(ctx context.Context) {
	lines := []string{"Stop service for " + w.envIdentify() + "]"}
	res, err := run(ctx, w.ch, isolationScript("cubrid service stop"))
	if err != nil {
		lines = append(lines, "[ERROR] "+err.Error())
	} else {
		lines = append(lines, output(res))
	}
	if err := w.sink.WorkerLines(w.envID, lines); err != nil {
		fmt.Printf("[ERROR] cannot write the worker log for %s: %v\n", w.envID, err)
	}
}

// deploy is Deploy for one slot (DeployOneNode.java): clear the processes, skip
// the installation, append the inquire_on_exit line, write the configured
// parameters. It returns what CTP wrote to the worker log.
//
// One thing is not reproduced. When the install step's output said "[ERROR]" or
// "No such file", CTP slept five seconds and tried again, forever. Here it stops
// the run and says why.
func deploy(ctx context.Context, ch exec.Channel, machine *topology.Instance, buildID string) ([]string, error) {
	var log []string

	// ctltool's scripts are committed without their execute bit (100644), and
	// runone.sh runs timeout3.sh as a program: without this every case fails on
	// "Permission denied". CTP sets the bit on every run in TestCaseGithub.update
	// -- the step that also pulls cases, which is excluded, and upgrades CTP, which
	// is excluded -- so that one line is kept and moved here. In a slot it lands
	// in the slot's overlay of the ctltool directory, and the tree keeps its modes.
	res, err := run(ctx, ch, isolationScript("cd ${CTP_HOME}/isolation/ctltool", "chmod u+x *.sh 2>&1"))
	if err == nil && output(res) != "" {
		err = fmt.Errorf("%s", output(res))
	}
	if err != nil {
		return log, fmt.Errorf("make ctltool's scripts executable: %w", err)
	}

	res, err = run(ctx, ch, isolationScript(killScript()))
	if err != nil {
		return log, fmt.Errorf("clean processes: %w", err)
	}
	log = append(log, output(res))

	res, err = run(ctx, ch, installScript(buildID))
	if err != nil {
		return log, fmt.Errorf("install step: %w", err)
	}
	out := output(res)
	if strings.Contains(out, "[ERROR]") || strings.Contains(out, "No such file") {
		return log, fmt.Errorf("the install step failed, where CTP would retry it forever: %s", strings.TrimSpace(out))
	}
	// Log.print and then Log.println: one line.
	log = append(log, "Skip build installation since cubrid_download_url is not configured!!"+out)

	// The emptiness test is shell runner's, which counts brokercommon where CTP's
	// did not; see shellsuite.ConfigureScript.
	if s := shellsuite.ConfigureScript(machine); s != "" {
		res, err := run(ctx, ch, shellScript(s))
		if err != nil {
			return log, fmt.Errorf("configure: %w", err)
		}
		log = append(log, output(res))
	}
	return log, nil
}
