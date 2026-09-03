// Package result writes the frozen output.
//
// Every byte anything outside this program can see is produced here, and nowhere
// else. That is deliberate: the freeze is a property of one layer, so it can be
// verified by comparing this layer's output and nothing more, and internals can be
// rewritten freely as long as they do not reach around it.
//
// Sink is a struct rather than an interface for the same reason. There is one
// surface, so there is nothing to abstract over, and an abstraction would make it
// impossible to answer "where does this marker come from" by reading.
//
// Every literal here was taken from CTP's source, not from the analysis notes.
// Three of them contradicted the notes: see the comments on TestCase and Open.
package result

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/cubrid-systems/cubrid-testkit/internal/conf"
)

// RetryFlag is what CTP prints before the retry count on a failed case.
//
// It is "TRY->", not "retry: ". Constants.RETRY_FLAG in the shell module.
const RetryFlag = "TRY->"

// CurrentRuntimeLogs is the fixed name of the directory a run writes into.
//
// There is no timestamp in the path. Context builds it as
// getToolHome() + "/result/" + category + "/current_runtime_logs", and
// jdbc/bin/run.sh spells the same thing out literally.
const CurrentRuntimeLogs = "current_runtime_logs"

// Sink owns the output of one run.
type Sink struct {
	dir    string
	root   string // result/<category>, the directory that holds dir
	stdout io.Writer

	mu      sync.Mutex
	workers map[string]*os.File // test_<envId>.log
	monitor map[string]*os.File // monitor_<envId>.log
	checks  map[string]*os.File // check_<envId>.log
	fin     map[string]*os.File // dispatch_tc_FIN_<envId>.txt
	append  bool                // continue mode reopens rather than truncates
}

// Open prepares the run directory: <CTP_HOME>/result/<category>/current_runtime_logs
//
// category is the task's result grouping, which is the task name for most tasks.
func Open(home *conf.Home, category string, continueMode bool) (*Sink, error) {
	dir := filepath.Join(home.Path, "result", category, CurrentRuntimeLogs)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("result directory %s: %w", dir, err)
	}
	return &Sink{
		dir:     dir,
		root:    filepath.Dir(dir),
		stdout:  os.Stdout,
		workers: map[string]*os.File{},
		monitor: map[string]*os.File{},
		checks:  map[string]*os.File{},
		fin:     map[string]*os.File{},
		append:  continueMode,
	}, nil
}

// Dir is the directory this run writes into.
func (s *Sink) Dir() string { return s.dir }

// EnvStart and EnvStop bracket one environment's work.
func (s *Sink) EnvStart(envID string) { fmt.Fprintf(s.stdout, "[ENV START] %s\n", envID) }
func (s *Sink) EnvStop(envID string)  { fmt.Fprintf(s.stdout, "[ENV STOP] %s\n", envID) }

// TestCase reports one case.
//
//	[TESTCASE] <name> EnvId=<env> [OK]
//	[TESTCASE] <name> EnvId=<env> [NOK]
//	[TESTCASE] <name> EnvId=<env> [NOK], TRY-><n>
//
// The retry suffix hangs on two conditions, and the second is easy to get wrong:
// the case has to have failed, and retries have to be *configured*. CTP tests
// maxRetryCount != 0, not whether a retry actually happened -- so a first-attempt
// failure prints ", TRY->0" when testcase_retry_num is set. The suffix never
// appears on a pass, and never in the isolation module at all.
func (s *Sink) TestCase(name, envID string, ok bool, maxRetryCount, retryCount int) {
	if ok {
		fmt.Fprintf(s.stdout, "[TESTCASE] %s EnvId=%s [OK]\n", name, envID)
		return
	}
	if maxRetryCount != 0 {
		fmt.Fprintf(s.stdout, "[TESTCASE] %s EnvId=%s [NOK], %s%d\n", name, envID, RetryFlag, retryCount)
		return
	}
	fmt.Fprintf(s.stdout, "[TESTCASE] %s EnvId=%s [NOK]\n", name, envID)
}

// Core announces a core file. Test cases in the frozen testcases repositories grep
// for this line, so it is F1 with a consumer that can be named.
func (s *Sink) Core(path string) { fmt.Fprintf(s.stdout, "CORE_FILE:%s\n", path) }

// Worker appends to test_<envId>.log, the per-environment detail log.
func (s *Sink) Worker(envID, line string) error {
	f, err := s.file(s.workers, "test_"+envID+".log", true)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(f, line)
	return err
}

// Check returns the writer for check_<envId>.log.
func (s *Sink) Check(envID string) (io.Writer, error) {
	return s.file(s.checks, "check_"+envID+".log", s.append)
}

// Monitor appends to monitor_<envId>.log.
//
// The file is usually empty. CTP created it whether or not anything was written,
// because its Log constructor creates the file eagerly, so an empty
// monitor_<envId>.log is part of every result directory -- a frozen result file
// the specification's list did not have.
func (s *Sink) Monitor(envID, line string) error {
	f, err := s.file(s.monitor, "monitor_"+envID+".log", s.append)
	if err != nil {
		return err
	}
	if line == "" {
		return nil
	}
	_, err = fmt.Fprintln(f, line)
	return err
}

// Finished records a completed case in dispatch_tc_FIN_<envId>.txt.
//
// The format is the contract for resuming: continue mode takes the cases in
// dispatch_tc_ALL.txt minus the union of these files, so one case per line and
// nothing else.
func (s *Sink) Finished(envID, name string) error {
	f, err := s.file(s.fin, "dispatch_tc_FIN_"+envID+".txt", s.append)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(f, name)
	return err
}

// Remaining is the case list a resumed run has left: everything in
// dispatch_tc_ALL.txt that no environment recorded as finished.
//
// A case that was still being retried when the run stopped is not in any FIN
// file, so it comes back -- which is right, because it never reached a verdict.
func (s *Sink) Remaining() ([]string, error) {
	all, err := readLines(filepath.Join(s.dir, "dispatch_tc_ALL.txt"))
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", s.dir, err)
	}
	done := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "dispatch_tc_FIN") || !strings.HasSuffix(name, ".txt") {
			continue
		}
		lines, err := readLines(filepath.Join(s.dir, name))
		if err != nil {
			return nil, err
		}
		for _, l := range lines {
			done[l] = true
		}
	}

	var left []string
	for _, c := range all {
		if !done[c] {
			left = append(left, c)
		}
	}
	return left, nil
}

func readLines(path string) ([]string, error) {
	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var out []string
	for _, l := range strings.Split(string(body), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out, nil
}

// All writes dispatch_tc_ALL.txt, the full pool for this run.
func (s *Sink) All(names []string) error {
	return s.write("dispatch_tc_ALL.txt", func(w io.Writer) error {
		for _, n := range names {
			if _, err := fmt.Fprintln(w, n); err != nil {
				return err
			}
		}
		return nil
	})
}

// Snapshot writes main_snapshot.properties: the configuration as it was when the
// run started, plus whatever the run resolved for itself, such as the build id.
func (s *Sink) Snapshot(cfg *conf.Config, resolved map[string]string) error {
	return s.write("main_snapshot.properties", func(w io.Writer) error {
		for _, k := range cfg.Keys() {
			v, _ := cfg.Get(k)
			if _, err := fmt.Fprintf(w, "%s=%s\n", k, v); err != nil {
				return err
			}
		}
		extra := make([]string, 0, len(resolved))
		for k := range resolved {
			extra = append(extra, k)
		}
		sort.Strings(extra)
		for _, k := range extra {
			if _, err := fmt.Fprintf(w, "%s=%s\n", k, resolved[k]); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Sink) write(name string, body func(io.Writer) error) error {
	path := filepath.Join(s.dir, name)
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	if err := body(w); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return w.Flush()
}

func (s *Sink) file(cache map[string]*os.File, name string, appendMode bool) (*os.File, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f, ok := cache[name]; ok {
		return f, nil
	}
	flags := os.O_CREATE | os.O_WRONLY
	if appendMode {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	path := filepath.Join(s.dir, name)
	f, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	cache[name] = f
	return f, nil
}

// Close releases the per-environment files.
func (s *Sink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var firstErr error
	for _, cache := range []map[string]*os.File{s.workers, s.monitor, s.checks, s.fin} {
		for _, f := range cache {
			if err := f.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	s.workers = map[string]*os.File{}
	s.monitor = map[string]*os.File{}
	s.checks = map[string]*os.File{}
	s.fin = map[string]*os.File{}
	return firstErr
}

// ---------------------------------------------------------------------------
// unittest
//
// The unittest task prints nothing like the shell family. It has step headings
// and its own case line, with a one-based index and [SUCC]/[FAIL] rather than
// [OK]/[NOK]. GeneralLocalTest.start is the source for every literal below,
// including the trailing space after each heading and the leading space before
// the verdict, which lands on the same line as the case name.
//
// None of this was in the Phase 0 notes, which described only the shell markers.
// ---------------------------------------------------------------------------

// Step prints one of the four headings: Init, List, Execute, Finish.
func (s *Sink) Step(name string) { fmt.Fprintf(s.stdout, "=> %s Step: \n", name) }

// Raw prints a block of the plug-in's own output, as the step handlers do.
func (s *Sink) Raw(text string) { fmt.Fprintln(s.stdout, text) }

// Blank prints the empty line that separates sections.
func (s *Sink) Blank() { fmt.Fprintln(s.stdout) }

// UnitCaseStart prints the case line without a verdict. The verdict arrives on
// the same line, so this deliberately does not end it.
func (s *Sink) UnitCaseStart(index int, name string) {
	fmt.Fprintf(s.stdout, "[TESTCASE-%d] %s", index, name)
}

// UnitCaseVerdict closes the line UnitCaseStart opened.
func (s *Sink) UnitCaseVerdict(ok bool) {
	if ok {
		fmt.Fprintln(s.stdout, " [SUCC]")
		return
	}
	fmt.Fprintln(s.stdout, " [FAIL]")
}

// NoCases reports an empty list, which CTP treats as a finished run rather than
// an error: it returns from start() and the exit code stays 0.
func (s *Sink) NoCases() { fmt.Fprintln(s.stdout, "[ERROR] Not found any test cases.") }
