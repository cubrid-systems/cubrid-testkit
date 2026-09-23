package hareplsuite

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The ledger is the run's verdicts, written as they happen.
//
// # Why a run needs one
//
// A run over `_01_object` is 3,327 cases and better than an hour, and an hour
// is long enough to be interrupted by something that has nothing to do with
// the suite. Twice on 2026-09-22 every CUBRID process on the host stopped in
// the same millisecond -- both sandbox clusters and the host's own install --
// once 1,228 cases in and once seventeen seconds in. Everything judged before
// that was in the terminal and nowhere else.
//
// So each verdict is appended and flushed as it is reached. That makes the
// run resumable, and it makes an interrupted run readable even when it is not
// resumed, which is the more important of the two.
//
// # What it deliberately does not do
//
// Carry a case's counters. A resumed case contributes its outcome to the tally
// and nothing else -- not its statements, not its reads compared. A file that
// tried to carry all of it would be a second format to keep in step with
// Result, and the run says plainly how many of its cases came from the ledger
// so the counters can be read for what they are.
type ledger struct {
	path string
	f    *os.File
}

// LedgerFile is the name beside the differences.
const LedgerFile = "verdicts.tsv"

func openLedger(dir string) (*ledger, error) {
	if dir == "" {
		return &ledger{}, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return &ledger{}, err
	}
	path := filepath.Join(dir, LedgerFile)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return &ledger{path: path}, err
	}
	return &ledger{path: path, f: f}, nil
}

// Write appends one verdict and flushes it. A verdict held in a buffer when the
// process is killed is a verdict that was not kept.
//
// The duration goes last, not third, so that a file written before this column
// existed still reads: three fields is case, outcome, detail, and four is the
// same with the milliseconds on the end.
func (l *ledger) Write(r Result, took time.Duration) {
	if l == nil || l.f == nil {
		return
	}
	detail := strings.ReplaceAll(r.Detail, "\t", " ")
	detail = strings.ReplaceAll(detail, "\n", " ")
	fmt.Fprintf(l.f, "%s\t%s\t%s\t%d\n", r.Case, r.Outcome, detail, took.Milliseconds())
	l.f.Sync()
}

func (l *ledger) Close() {
	if l != nil && l.f != nil {
		l.f.Close()
	}
}

// Judged reads back the cases a previous run decided.
//
// `wait_timeout` and `case_failed` are left out on purpose: they are verdicts
// about the run and not about the case, and an interruption produces nothing
// else. Resuming them would carry the interruption's own damage into the
// resumed run's tally and call it a measurement.
func (l *ledger) Judged() map[string]Result {
	out := map[string]Result{}
	if l == nil || l.path == "" {
		return out
	}
	f, err := os.Open(l.path)
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		r, ok := parseLedgerLine(sc.Text())
		if !ok {
			continue
		}
		if r.Outcome == WaitTimeout || r.Outcome == CaseFailed {
			delete(out, r.Case)
			continue
		}
		out[r.Case] = r
	}
	return out
}

// parseLedgerLine reads one line back, in either shape.
func parseLedgerLine(line string) (Result, bool) {
	fields := strings.Split(line, "\t")
	if len(fields) < 2 || fields[0] == "" || fields[1] == "" {
		return Result{}, false
	}
	r := Result{Case: fields[0], Outcome: Outcome(fields[1])}
	if len(fields) >= 3 {
		r.Detail = fields[2]
	}
	if len(fields) >= 4 {
		if ms, err := strconv.ParseInt(fields[3], 10, 64); err == nil {
			r.Took = time.Duration(ms) * time.Millisecond
		}
	}
	return r, true
}
