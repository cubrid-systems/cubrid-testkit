package coredump

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
