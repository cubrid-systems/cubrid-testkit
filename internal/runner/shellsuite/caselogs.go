package shellsuite

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
)

// CaseLogs keeps what a case wrote, so that a failure can be diagnosed without
// running the corpus again.
//
// The motivating run: nineteen of twenty-two cases reported `cubrid server
// start: fail`, and the server's own .err file saying why was inside an overlay
// that had already been dropped. The verdict survived and the reason did not.
//
// What is kept is decided by the *file class*, not by the verdict, and the cheap
// class is kept for cases that passed:
//
//   - CTP greps the server log for "Internal Error" only while writing a NOK, so
//     a case that passed with one in its log says nothing about it.
//   - A case that fails intermittently cannot be diagnosed from its failures
//     alone. The run that passed is half the comparison.
//
// Measured on this corpus: a case's server logs are about 5 KB, which is 15 MB
// over 3,444 cases -- a rounding error against a tmpfs measured in gigabytes, so
// there is no policy worth arguing about. The broker's SQL log is 99.6% of the
// log tree and is megabytes a case, which is why it is in the failure tier and
// why that tier has a budget.
//
// What is deliberately *not* copied is the install. CTP's own snapshot copies
// $CUBRID whole -- 748.8 MB per failing case on the build this was written
// against -- and it is the same 748.8 MB every time, because a run has one
// build. Only what the case changed is worth keeping, and the run already knows
// what that is: everything under $CUBRID/log is the case's, since the install is
// restored from a pristine snapshot before every case.
type CaseLogs struct {
	// dir is <CTP_HOME>/result/<category>/case-logs. Empty means off.
	dir string
	// scenario is stripped from a case path so that the tree under dir is the
	// corpus's own shape. Keeping the absolute path instead buries every capture
	// under a copy of wherever the corpus happened to be checked out.
	scenario string
	// all keeps the cheap tier for cases that passed, not only for failures.
	all bool
	// budget is what the whole run may spend, in MB. Zero is no limit.
	budget int

	mu    sync.Mutex
	spent int64 // bytes
	// stopped is set once the budget is gone, so the run says so once rather
	// than for every case afterwards.
	stopped bool
	kept    int
}

// NewCaseLogs reads the two keys. mode is off, fail or all; anything else is an
// error rather than a silent default, because a misspelt mode that quietly means
// "off" is a run that discovers at the end that it kept nothing.
func NewCaseLogs(resultDir, scenario, mode string, budgetMB int) (*CaseLogs, error) {
	switch mode {
	case "", "off":
		return nil, nil
	case "fail", "all":
	default:
		return nil, fmt.Errorf("case_logs is %q: it must be off, fail or all", mode)
	}
	if resultDir == "" {
		return nil, fmt.Errorf("case_logs needs a result directory")
	}
	// Beside current_runtime_logs and not inside it: that tree is the frozen
	// surface (ADR docs/project/concept/external-surface-freeze.md) and a directory
	// nobody expects there is a change to it.
	return &CaseLogs{
		dir:      filepath.Join(filepath.Dir(resultDir), "case-logs"),
		scenario: scenario,
		all:      mode == "all",
		budget:   budgetMB,
	}, nil
}

// Dir is where captures go, for the line the run prints.
func (l *CaseLogs) Dir() string {
	if l == nil {
		return ""
	}
	return l.dir
}

// wants reports whether this verdict is worth a capture.
func (l *CaseLogs) wants(ok bool) bool {
	if l == nil {
		return false
	}
	return !ok || l.all
}

// spend takes n bytes off the budget and reports whether the capture may stand.
// A capture that would cross the budget is still kept -- refusing it after it has
// been written would leave the run having paid for it and thrown it away -- but
// it is the last one.
func (l *CaseLogs) spend(n int64) (overBudget bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.spent += n
	l.kept++
	if l.budget > 0 && l.spent >= int64(l.budget)<<20 && !l.stopped {
		l.stopped = true
		return true
	}
	return false
}

func (l *CaseLogs) done() bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.stopped
}

// Summary is what the run says on the way out, or "" when nothing was kept.
func (l *CaseLogs) Summary() string {
	if l == nil {
		return ""
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.kept == 0 {
		return ""
	}
	s := fmt.Sprintf("[INFO] case logs: %d kept, %d MB, under %s", l.kept, l.spent>>20, l.dir)
	if l.stopped {
		s += fmt.Sprintf(" (stopped at the %d MB budget)", l.budget)
	}
	return s
}

// destOf is where one case's capture goes: the case's own path under case-logs,
// so that nothing has to be allocated and nothing can collide.
//
// CTP names a snapshot AUTO_<build>_<datetime>, which is unique when one case
// runs at a time and stops being so at eight -- several land in the same second
// and the name does not say which case is in which. A case path is unique by
// construction.
func destOf(root, scenario, casePath string, attempt int) string {
	// The case as the corpus names it: the scenario off the front, and the
	// script name off the end, since the case directory is what identifies it
	// and <name>/cases/<name>.sh repeats itself.
	rel := casePath
	if scenario != "" {
		if r, err := filepath.Rel(scenario, casePath); err == nil && !strings.HasPrefix(r, "..") {
			rel = r
		}
	}
	rel = strings.TrimPrefix(rel, "/")
	if dir, file := filepath.Split(rel); filepath.Base(filepath.Dir(dir)) != "" && strings.HasSuffix(file, ".sh") {
		rel = filepath.Clean(dir)
	}
	d := filepath.Join(root, rel)
	if attempt > 1 {
		d = filepath.Join(d, fmt.Sprintf("try%d", attempt))
	}
	return d
}

// CaptureScript copies what one case left into dest.
//
// Through the case's own channel, and by path rather than by reading the slot's
// overlay upper layer from outside: it is the same files either way, and this
// works identically whether the run has overlays, one slot, or neither. The
// destination is under CTP_HOME, which no slot overlays, so a copy made inside a
// slot lands on the real filesystem and survives the slot.
//
// Copy and never move: the case is still the source of truth for its own
// verdict, and a diagnostic that consumes what it is diagnosing is worse than no
// diagnostic.
//
// It ends with `du` so the caller can charge the budget for what was actually
// written rather than for what it hoped to write.
func CaptureScript(dest, caseDir string, failed bool) string {
	var b strings.Builder
	// A capture that cannot start says so rather than exiting quietly. Swallowing
	// it is how this went unnoticed for a run: the summary said one case was
	// kept and the directory was empty, and there was nothing to read either way.
	fmt.Fprintf(&b, "mkdir -p %q/server || { echo \"testkit-capture-failed: mkdir %s\"; exit 0; }\n", dest, dest)
	// Tier 1, always. A pristine install's log directory is empty, so everything
	// here is this case's.
	fmt.Fprintf(&b, "cp -p ${CUBRID}/log/server/* %q/server/ 2>/dev/null\n", dest)
	fmt.Fprintf(&b, "cp -p ${CUBRID}/log/*.err %q/ 2>/dev/null\n", dest)
	fmt.Fprintf(&b, "cp -p ${CUBRID}/log/cubrid_utility.log %q/ 2>/dev/null\n", dest)
	fmt.Fprintf(&b, "cp -rp ${CUBRID}/log/pl %q/ 2>/dev/null\n", dest)
	// What the case itself wrote, which is often the only thing that says what
	// happened. tran_info runs `cubrid loaddb ... >load.log 2>&1` in the
	// background and fails when it cannot observe it; load.log is where loaddb
	// said why, and without this there is nothing to read.
	//
	// Kept for a case that passed as well as one that failed, because comparing
	// the two is how an intermittent case is diagnosed and a passing run is half
	// of that comparison. Measured while trying to redesign two expect scripts:
	// they pass on one machine and fail on another, and the passing exp.log --
	// 1,979 bytes -- was the missing half. It is cheap, unlike the broker log
	// below, so it follows the mode rather than the verdict.
	//
	// The case's own directory, not the corpus around it: everything here goes
	// to the corpus overlay and is dropped when the directory retires, so it is
	// this or nothing. Databases are excluded by name -- they are the tier
	// above, and one of them can be larger than every log put together.
	if caseDir != "" {
		fmt.Fprintf(&b, "mkdir -p %q/case || { echo \"testkit-capture-failed: mkdir %s/case\"; exit 0; }\n", dest, dest)
		fmt.Fprintf(&b, "find %q -maxdepth 1 -type f \\( -name '*.log' -o -name '*.err' -o "+
			"-name '*.out' -o -name '*.result' -o -name '*.diff' -o -name 'core*' -prune \\) "+
			"-size -8M -exec cp -p {} %q/case/ \\; 2>/dev/null\n", caseDir, dest)
	}
	if failed {
		// The expensive tier, and the only thing in it. The broker's SQL log is
		// 99.6% of the log tree and megabytes a case -- one case in a
		// twenty-two-case run wrote 36 MB of the run's 55 -- so it is the one
		// thing kept only for a failure. cubrid.conf comes with it: it is how a
		// case that changed a parameter explains itself.
		fmt.Fprintf(&b, "cp -rp ${CUBRID}/log/broker %q/ 2>/dev/null\n", dest)
		fmt.Fprintf(&b, "cp -p ${CUBRID}/conf/cubrid.conf %q/ 2>/dev/null\n", dest)
	}
	fmt.Fprintf(&b, "du -sk %q 2>/dev/null | cut -f1\n", dest)
	return b.String()
}

// Capture keeps one case's logs. It reports what it could not do rather than
// returning an error, because a capture that fails must not fail the case.
func (l *CaseLogs) Capture(ctx context.Context, ch exec.Channel, casePath, caseDir string, attempt int, ok bool) string {
	if !l.wants(ok) || l.done() {
		return ""
	}
	dest := destOf(l.dir, l.scenario, casePath, attempt)
	res, err := runIn(ctx, ch, CaptureScript(dest, caseDir, !ok))
	if err != nil {
		return "[WARN] case logs: " + casePath + ": " + err.Error()
	}
	out := strings.TrimSpace(res.Output())
	if i := strings.Index(out, "testkit-capture-failed:"); i >= 0 {
		line := out[i:]
		if j := strings.IndexByte(line, '\n'); j >= 0 {
			line = line[:j]
		}
		return "[WARN] case logs: " + casePath + ": " + line
	}
	var kb int64
	fmt.Sscanf(lastLine(out), "%d", &kb)
	if l.spend(kb << 10) {
		return fmt.Sprintf("[WARN] case logs: the %d MB budget is gone; nothing more will be kept.", l.budget)
	}
	return ""
}

// lastLine is the du figure at the end of the capture, which is the only line
// the script is meant to produce. Anything before it is a message, and a message
// means something did not go to plan.
func lastLine(s string) string {
	if i := strings.LastIndexByte(strings.TrimRight(s, "\n"), '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}
