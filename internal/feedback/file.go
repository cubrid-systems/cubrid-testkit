package feedback

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/result"
)

// File is feedback_type=file, and it is the default -- a run that says nothing
// about feedback gets this one.
//
// It writes four files into the run directory and, for a handful of lines, to
// standard output as well. Those lines are part of the frozen console output even
// though they come from a feedback backend rather than from the runner, which is
// not where anyone would look for them.
//
//	feedback.log         one line per event, plus each case's result text
//	test_status.data     the five counters, rewritten after every case
//	current_task_id      the task id, which is always 0 without the scheduler
//	test-<category>.xml  JUnit XML, for whatever reads it downstream
type File struct {
	Category string
	Out      io.Writer

	// MsgID comes from the MSG_ID environment variable, which the excluded
	// scheduler used to set. Standalone it is empty, and the line that reports it
	// ends in a space. Carried through because it costs nothing and because a
	// scheduler-driven run is still meant to be readable.
	MsgID string

	dir       string // empty means nothing is written to disk
	continued bool
	// isolation is the isolation module's FeedbackFile, which is a different
	// class from shell's and says a few things differently -- see OpenIsolation.
	isolation bool

	mu    sync.Mutex
	log   *os.File
	xml   *junit
	start time.Time
	stats Stats
}

// Stats are the five counters CTP keeps in test_status.data.
type Stats struct {
	Total    int
	Executed int
	Success  int
	Fail     int
	Skip     int
}

// statusFile is the name of the counters file. It is read back by a resumed run,
// so its format is Java Properties and stays that way.
const statusFile = "test_status.data"

// Console is a File that only produces the lines that go to standard output. It
// is what unittest wants: the two counts, and no run directory to write into.
func Console(category string, out io.Writer) *File {
	return &File{Category: category, Out: out}
}

// Open prepares the four files in dir. A continued run appends to the log and
// picks the counters up where they were left.
func Open(dir, category string, out io.Writer, continueMode bool, msgID string) (*File, error) {
	f := &File{Category: category, Out: out, MsgID: msgID, dir: dir, continued: continueMode}
	if err := f.openLog(); err != nil {
		return nil, err
	}

	x, err := openJUnit(filepath.Join(dir, "test-"+category+".xml"), category)
	if err != nil {
		f.log.Close()
		return nil, err
	}
	f.xml = x
	return f, nil
}

// OpenIsolation is Open for the isolation task.
//
// CTP's isolation module has a FeedbackFile of its own
// (isolation/impl/FeedbackFile.java), and it is not shell's. It writes two of
// the four files -- feedback.log and test_status.data, with no current_task_id
// and no JUnit report -- and it says four things differently: no task id or MSG id
// when the task starts, no colon and no retry count after [OK] and [NOK], and a
// console that says "The category:" where the log says "Test Category:". All of
// it is in the run directory a comparison reads, so all of it is reproduced.
func OpenIsolation(dir, category string, out io.Writer, continueMode bool) (*File, error) {
	f := &File{Category: category, Out: out, dir: dir, continued: continueMode, isolation: true}
	if err := f.openLog(); err != nil {
		return nil, err
	}
	return f, nil
}

// openLog opens feedback.log. A continued run appends to it and picks the
// counters up where they were left.
func (f *File) openLog() error {
	flags := os.O_CREATE | os.O_WRONLY
	if f.continued {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	log, err := os.OpenFile(filepath.Join(f.dir, "feedback.log"), flags, 0o644)
	if err != nil {
		return fmt.Errorf("feedback log: %w", err)
	}
	f.log = log

	if f.continued {
		if err := f.readStats(); err != nil {
			f.log.Close()
			return err
		}
	}
	return nil
}

// println writes to the feedback log only. Nil entries are skipped, which is how
// CTP's variadic println let callers pass an absent result.
func (f *File) println(lines ...string) {
	if f.log == nil {
		return
	}
	for _, l := range lines {
		fmt.Fprintln(f.log, l)
	}
}

// emit writes to both the feedback log and standard output.
func (f *File) emit(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	f.println(line)
	if f.Out != nil {
		fmt.Fprintln(f.Out, line)
	}
}

func (f *File) TaskStart(string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.start = time.Now()
	if f.isolation {
		f.println("[TASK START] Current Time is " + javaDate(time.Now()))
		return
	}
	f.writeTaskID(0)
	f.println("[Task Id] is 0")
	// Java concatenated a null MsgID into the string, so a run without the
	// scheduler says "is null" rather than trailing off.
	f.println("[TASK START] Current Time is " + javaDate(time.Now()) + ", start MSG Id is " + orNull(f.MsgID))
}

func (f *File) TaskContinue() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.start = time.Now()
	if !f.isolation {
		f.println("[Task Id] is 0")
	}
	f.println("[TASK CONTINUE] Current Time is " + javaDate(time.Now()))
}

// TotalTestCase records the size of the run. A continued run already knows, and
// says nothing -- which is why the two console lines below are absent when a run
// is resumed.
func (f *File) TotalTestCase(total, macroSkipped, tempSkipped int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.continued {
		return
	}
	f.stats.Total = total
	f.stats.Skip = macroSkipped + tempSkipped

	if f.isolation {
		f.println("Test Category:" + f.Category)
		if f.Out != nil {
			fmt.Fprintln(f.Out, "The category:"+f.Category)
		}
	} else {
		f.emit("Test Category:%s", f.Category)
	}
	f.emit("The Number of Test Cases: %d (macro skipped: %d, bug skipped: %d)",
		total, macroSkipped, tempSkipped)

	f.xml.suiteStart(f.stats.Total, f.stats.Skip)
	f.update()
}

func (f *File) CaseStart(string, string) {}

// CaseStop records a verdict.
//
// The head of the line is CTP's, including the shape of the failing form: the
// retry flag is used as a label there, "[NOK]: TRY-> = 2", and not as the prefix
// the console uses. Two spellings of the same thing, both frozen.
func (f *File) CaseStop(ev CaseStop) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var head string
	switch ev.SkipType {
	case SkipTypeNo:
		if ev.Success {
			head = "[OK]: "
			if f.isolation {
				head = "[OK]"
			}
			f.stats.Success++
			f.xml.testCase(ev.Case, ev.EnvID, ev.Elapsed, "", "", "")
		} else {
			head = "[NOK]: " + result.RetryFlag + " = " + strconv.Itoa(ev.RetryCount)
			if f.isolation {
				head = "[NOK]"
			}
			f.stats.Fail++
			f.xml.testCase(ev.Case, ev.EnvID, ev.Elapsed, "failure", "Test failed", ev.ResultText)
		}
	case SkipTypeByMacro:
		head = "[SKIP_BY_MACRO]"
		f.xml.testCase(ev.Case, ev.EnvID, 0, "skipped", "Skipped by macro", "")
	case SkipTypeByTemp:
		head = "[SKIP_BY_BUG]"
		f.xml.testCase(ev.Case, ev.EnvID, 0, "skipped", "Skipped by bug", "")
	default:
		head = "[UNKNOWN]"
		f.xml.testCase(ev.Case, ev.EnvID, ev.Elapsed, "error", "Unknown test status", "")
	}

	f.println(head+" "+ev.Case+" "+millis(ev.Elapsed)+" "+ev.EnvID, ev.ResultText, "")
	f.update()
}

// CaseStopRetry records an attempt that is going back into the queue. It is not a
// verdict: the counters do not move and nothing reaches the XML, or a case that
// eventually passed would be reported as a failure too.
func (f *File) CaseStopRetry(ev CaseStop) {
	f.mu.Lock()
	defer f.mu.Unlock()

	head := "[NOK]: "
	if ev.Success {
		head = "[OK]: "
	}
	f.println(head+" "+ev.Case+" "+millis(ev.Elapsed)+" "+ev.EnvID,
		ev.ResultText,
		" ("+result.RetryFlag+" = "+strconv.Itoa(ev.RetryCount)+")")
}

func (f *File) CaseMonitor(name, action, envID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.println(action + " " + name + " " + envID)
}

func (f *File) EnvStop(string) {}

// TaskStop prints the summary, which is the part of a run anyone actually reads.
func (f *File) TaskStop() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.emit("============= PRINT SUMMARY ==================")
	f.emit("Test Category:%s", f.Category)
	f.emit("Total Case:%d", f.stats.Total)
	f.emit("Total Execution Case:%d", f.stats.Executed)
	f.emit("Total Success Case:%d", f.stats.Success)
	f.emit("Total Fail Case:%d", f.stats.Fail)
	f.emit("Total Skip Case:%d", f.stats.Skip)
	if f.Out != nil {
		fmt.Fprintln(f.Out)
	}

	elapsed := time.Duration(0)
	if !f.start.IsZero() {
		elapsed = time.Since(f.start)
	}
	f.println("[TEST STOP] Current Time is "+javaDate(time.Now()),
		"Elapse Time:"+millis(elapsed))
}

// Stats returns the counters as they stand.
func (f *File) Stats() Stats {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stats
}

// Close finishes the XML and releases the log.
func (f *File) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	var firstErr error
	if f.xml != nil {
		if err := f.xml.close(); err != nil {
			firstErr = err
		}
		f.xml = nil
	}
	if f.log != nil {
		if err := f.log.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		f.log = nil
	}
	return firstErr
}

// update recomputes the derived counters and rewrites test_status.data.
//
// Executed and Total are derived rather than counted: a resumed run adds to
// whatever the previous one left, so counting would double.
func (f *File) update() {
	f.stats.Executed = f.stats.Success + f.stats.Fail
	f.stats.Total = f.stats.Executed + f.stats.Skip
	if f.dir == "" {
		return
	}
	if err := f.writeStats(); err != nil {
		msg := "[ERROR]: Update test status into " + filepath.Join(f.dir, statusFile) + " fail!"
		f.println(msg)
		if f.Out != nil {
			fmt.Fprintln(f.Out, msg)
		}
	}
}

// statusKeys are written in this order. CTP used Properties.store, which emits
// hash order and a timestamp comment, so the file was different on every run for
// reasons that had nothing to do with the run. Sorted keys and no timestamp make
// it comparable; it stays readable as Java Properties, which is what a resumed
// run needs.
var statusKeys = []string{
	"total_case_count",
	"total_executed_case_count",
	"total_success_case_count",
	"total_fail_case_count",
	"total_skip_case_count",
}

func (f *File) writeStats() error {
	values := map[string]int{
		"total_case_count":          f.stats.Total,
		"total_executed_case_count": f.stats.Executed,
		"total_success_case_count":  f.stats.Success,
		"total_fail_case_count":     f.stats.Fail,
		"total_skip_case_count":     f.stats.Skip,
	}
	var b strings.Builder
	for _, k := range statusKeys {
		fmt.Fprintf(&b, "%s=%d\n", k, values[k])
	}
	return os.WriteFile(filepath.Join(f.dir, statusFile), []byte(b.String()), 0o644)
}

func (f *File) readStats() error {
	body, err := os.ReadFile(filepath.Join(f.dir, statusFile))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s: %w", statusFile, err)
	}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || !slices.Contains(statusKeys, strings.TrimSpace(key)) {
			continue
		}
		switch strings.TrimSpace(key) {
		case "total_case_count":
			f.stats.Total = n
		case "total_executed_case_count":
			f.stats.Executed = n
		case "total_success_case_count":
			f.stats.Success = n
		case "total_fail_case_count":
			f.stats.Fail = n
		case "total_skip_case_count":
			f.stats.Skip = n
		}
	}
	return nil
}

func (f *File) writeTaskID(id int) {
	if f.dir == "" {
		return
	}
	os.WriteFile(filepath.Join(f.dir, "current_task_id"), []byte(strconv.Itoa(id)+"\n"), 0o644)
}

// orNull renders an absent value the way Java's string concatenation does. It
// looks like a bug and is not: "null" is what these lines have always said, and a
// reader who greps for it should keep finding it.
func orNull(s string) string {
	if s == "" {
		return "null"
	}
	return s
}

// millis renders a duration the way a Java long of milliseconds prints.
func millis(d time.Duration) string { return strconv.FormatInt(d.Milliseconds(), 10) }

func javaDate(t time.Time) string { return t.Format("Mon Jan 02 15:04:05 MST 2006") }
