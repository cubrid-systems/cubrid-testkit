package coredump

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
)

// Crash is the report a server writes about its own death: when it takes a
// signal, `crash_handler` prints the call stack to
// $CUBRID/log/coredump/<program>_<when>.coredump and the process ends.
//
// It is not a core file, and that is the point. CTP looks for files named
// `core.*` and for FATAL ERROR in the log (runone.sh's checkCoreAndFatalError,
// CQT's getCoreFiles, shell's do_check_more_errors). Where
// /proc/sys/kernel/core_pattern hands cores to a crash handler such as apport
// there is no `core.*` to find, and a SIGSEGV writes no FATAL ERROR line, so a
// server that died is reported as an ordinary diff -- which is how
// isolation's reorganization_select_01 was recorded as a failing case for as
// long as this project has records, while it was crashing every time
// (evidence/isolation-always-failing.md §1, upstream CBRD-27407).
type Crash struct {
	// Path is where the engine left it, inside the run's own namespace.
	Path string
	// Program is the report's first line: "cub_server ctldb".
	Program string
	// Where is the first frame below the crash handler, as
	// "qdata_save_agg_hentry_to_list at query_aggregate.cpp:2974".
	Where string
}

// String is what a verdict line says about it.
func (c Crash) String() string {
	s := filepath.Base(c.Path)
	if c.Program != "" {
		s += " (" + c.Program
		if c.Where != "" {
			s += ", " + c.Where
		}
		s += ")"
	}
	return s
}

// Crashes are the reports under cubridDir that seen does not already hold. A
// slot's install is its own overlay at the same path, so cubridDir is what the
// runner knows as $CUBRID and the command runs in the place that owns it; seen
// carries across the cases of one worker, because nothing sweeps the directory
// between them.
//
// The path is passed rather than left to the shell: these commands do not run
// behind the profile the suites' own scripts use, so $CUBRID would be empty
// there and the find would search from the root.
//
// Reading the machine is never a reason to fail a case that otherwise passed, so
// an unreadable directory is no crash rather than an error.
func Crashes(ctx context.Context, ch exec.Channel, cubridDir string, seen map[string]bool) []Crash {
	if strings.TrimSpace(cubridDir) == "" {
		return nil
	}
	dir := filepath.Join(cubridDir, "log", "coredump")
	res, err := ch.Run(ctx, `find `+shellQuote(dir)+` -type f -name '*.coredump' 2>/dev/null | sort`)
	if err != nil {
		return nil
	}
	var found []Crash
	for _, path := range strings.Split(strings.TrimSpace(res.Output()), "\n") {
		path = strings.TrimSpace(path)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		c := Crash{Path: path}
		if head, err := ch.Run(ctx, "head -40 "+shellQuote(path)); err == nil {
			c.Program, c.Where = readHead(head.Output())
		}
		found = append(found, c)
	}
	return found
}

// Keep copies a report out of the place that holds it, into dir, and returns
// where it landed. The name carries the case it belongs to, because one
// directory holds the reports of a whole run.
//
// It is read through the channel rather than with Get: a slot's install is an
// overlay inside that slot's mount namespace, so the path exists only in there
// -- and the whole reason to copy it now is that the overlay is thrown away with
// the slot. A report is a few kilobytes of text.
func Keep(ctx context.Context, ch exec.Channel, c Crash, dir, caseName string) (string, error) {
	res, err := ch.Run(ctx, "cat "+shellQuote(c.Path))
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", c.Path, err)
	}
	if strings.TrimSpace(res.Output()) == "" {
		return "", fmt.Errorf("reading %s: it is empty", c.Path)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := strings.TrimSuffix(filepath.Base(caseName), filepath.Ext(caseName))
	local := filepath.Join(dir, name+"."+filepath.Base(c.Path))
	if err := os.WriteFile(local, []byte(res.Output()), 0o644); err != nil {
		return "", err
	}
	return local, nil
}

// handlers are the frames of the crash handler itself, which every report
// begins with and none of which says where the server died.
var handlers = []string{
	"er_dump_call_stack", "er_print_crash_callstack", "crash_handler", "__restore_rt",
	"print_output::~print_output", "??",
}

// readHead reads the program and the first frame that is not the handler's.
func readHead(out string) (program, where string) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "process info :"):
			program = strings.TrimSpace(strings.TrimPrefix(line, "process info :"))
		case where == "" && strings.HasPrefix(line, "["):
			if w, ok := frameOf(line); ok {
				where = w
			}
		}
	}
	return program, where
}

// frameOf reads one "[16] 0x…: function(args) at /path/file.cpp:2974" line, and
// says no to the crash handler's own frames.
func frameOf(line string) (string, bool) {
	_, rest, ok := strings.Cut(line, ": ")
	if !ok {
		return "", false
	}
	at := strings.LastIndex(rest, " at ")
	if at < 0 {
		return "", false
	}
	fn, place := rest[:at], rest[at+len(" at "):]
	// The arguments are the C++ signature and say nothing a reader needs here.
	if i := strings.Index(fn, "("); i > 0 {
		fn = fn[:i]
	}
	fn = strings.TrimSpace(fn)
	for _, h := range handlers {
		if strings.HasPrefix(fn, h) {
			return "", false
		}
	}
	if fn == "" {
		return "", false
	}
	return fn + " at " + filepath.Base(strings.TrimSpace(place)), true
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// Cores are the core files under the directories given that seen does not
// already hold: CTP's own pattern (`core.*`, less `core.log`), which is what
// runone.sh's check looked for before this runner took the check over
// (ADR-021).
//
// Where a machine writes cores, they are worth a stack and not a copy: Analyze
// reads one in the place that holds it, and the stack goes to the results. The
// core itself stays where it fell, and goes with the slot.
func Cores(ctx context.Context, ch exec.Channel, seen map[string]bool, dirs ...string) []string {
	var quoted []string
	for _, d := range dirs {
		if strings.TrimSpace(d) != "" {
			quoted = append(quoted, shellQuote(d))
		}
	}
	if len(quoted) == 0 {
		return nil
	}
	res, err := ch.Run(ctx, "find "+strings.Join(quoted, " ")+` -name 'core.*' -type f 2>/dev/null | sort`)
	if err != nil {
		return nil
	}
	var found []string
	for _, path := range strings.Split(strings.TrimSpace(res.Output()), "\n") {
		if path = strings.TrimSpace(path); path == "" || seen[path] {
			continue
		}
		// core.log is the engine's log, and CTP's check excluded it by name.
		if filepath.Base(path) == "core.log" {
			continue
		}
		seen[path] = true
		found = append(found, path)
	}
	return found
}

// Fatals are the log files under cubridDir/log holding more "FATAL ERROR" lines
// than counts has recorded, and the number they have gained. A log is appended
// to and never emptied between cases, so what makes this a finding about this
// case is the increase and not the total -- where CTP's check fired again for
// every case that followed one (runone.sh:189).
func Fatals(ctx context.Context, ch exec.Channel, cubridDir string, counts map[string]int) []string {
	if strings.TrimSpace(cubridDir) == "" {
		return nil
	}
	res, err := ch.Run(ctx, "grep -rc 'FATAL ERROR' "+shellQuote(filepath.Join(cubridDir, "log"))+" 2>/dev/null")
	if err != nil {
		return nil
	}
	var found []string
	for _, line := range strings.Split(strings.TrimSpace(res.Output()), "\n") {
		path, n, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		now, err := strconv.Atoi(strings.TrimSpace(n))
		if err != nil || now <= counts[path] {
			continue
		}
		gained := now - counts[path]
		counts[path] = now
		found = append(found, fmt.Sprintf("%s (%d line(s))", filepath.Base(path), gained))
	}
	return found
}

// KeepStack writes a core's stack into dir, named for the case and the core, and
// returns where it landed. The core stays where it fell: it is gigabytes, it
// belongs to a slot that is about to go, and what a reader needs from it is the
// stack. gdb runs in the place that holds the core, for the same reason.
func KeepStack(ctx context.Context, ch exec.Channel, core, dir, caseName string) (string, error) {
	s, err := Analyze(ctx, ch, core)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := strings.TrimSuffix(filepath.Base(caseName), filepath.Ext(caseName))
	local := filepath.Join(dir, name+"."+filepath.Base(core)+".stack")
	f, err := os.Create(local)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := Report(f, filepath.Dir(core), []Stack{s}); err != nil {
		return "", err
	}
	return local, nil
}
