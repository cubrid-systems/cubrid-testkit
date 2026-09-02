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
	stdout io.Writer

	mu      sync.Mutex
	workers map[string]*os.File // test_<envId>.log
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
		stdout:  os.Stdout,
		workers: map[string]*os.File{},
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
	for _, cache := range []map[string]*os.File{s.workers, s.fin} {
		for _, f := range cache {
			if err := f.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	s.workers = map[string]*os.File{}
	s.fin = map[string]*os.File{}
	return firstErr
}
