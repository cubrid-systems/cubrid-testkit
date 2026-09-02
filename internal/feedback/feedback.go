// Package feedback reports how a run is going.
//
// The events are axis T -- they describe the test run itself. Where they are
// *stored* is axis O, which is why there is no database backend here: FeedbackDB
// was excluded (docs/concept/migration-exclusions.md §1-5). Null and File remain.
//
// docs/design/contracts.md C5.
package feedback

import (
	"fmt"
	"io"
	"time"
)

// SkipType says why a case did not run. The constants keep CTP's names because
// they travel into stored feedback.
type SkipType int

const (
	SkipTypeNo SkipType = iota
	SkipTypeByMacro
	SkipTypeByTemp
)

// CaseStop carries the outcome of one case.
type CaseStop struct {
	Case       string
	EnvID      string
	Success    bool
	Elapsed    time.Duration
	ResultText string
	// LastPassResultCont and RetryCount exist only in the shell module. The
	// isolation module's event has neither, and that asymmetry is preserved.
	LastPassResultCont string
	TimedOut           bool
	HasCore            bool
	SkipType           SkipType
	RetryCount         int
}

// Feedback receives the events of a run.
type Feedback interface {
	TaskStart(buildURL string)
	TaskContinue()
	TaskStop()
	TotalTestCase(total, macroSkipped, tempSkipped int)
	CaseStart(name, envID string)
	CaseStop(ev CaseStop)
	CaseStopRetry(ev CaseStop) // shell only

	// CaseMonitor records something that happened to a case while it was still
	// running -- in practice, a timeout being resolved out from under it.
	CaseMonitor(name, action, envID string)

	EnvStop(envID string)
}

// Null discards everything. It is the default, and it is what feedback_type=null
// selected.
type Null struct{}

func (Null) TaskStart(string)                   {}
func (Null) TaskContinue()                      {}
func (Null) TaskStop()                          {}
func (Null) TotalTestCase(int, int, int)        {}
func (Null) CaseStart(string, string)           {}
func (Null) CaseStop(CaseStop)                  {}
func (Null) CaseStopRetry(CaseStop)             {}
func (Null) CaseMonitor(string, string, string) {}
func (Null) EnvStop(string)                     {}

// File is feedback_type=file.
//
// It writes to its own file and, for two of the events, to standard output as
// well -- FeedbackFile.setTotalTestCase prints the same two lines twice, once to
// each. Those two lines are therefore part of the frozen console output even
// though they come from a feedback backend rather than from the runner.
type File struct {
	Category string
	Out      io.Writer // standard output
	Log      io.Writer // the feedback file; may be nil
}

func (f *File) emit(format string, args ...any) {
	if f.Log != nil {
		fmt.Fprintf(f.Log, format+"\n", args...)
	}
	if f.Out != nil {
		fmt.Fprintf(f.Out, format+"\n", args...)
	}
}

func (f *File) TaskStart(string) {}
func (f *File) TaskContinue()    {}
func (f *File) TaskStop()        {}

// TotalTestCase prints the two lines CTP prints once the case list is known.
func (f *File) TotalTestCase(total, macroSkipped, tempSkipped int) {
	f.emit("Test Category:%s", f.Category)
	f.emit("The Number of Test Cases: %d (macro skipped: %d, bug skipped: %d)", total, macroSkipped, tempSkipped)
}

func (f *File) CaseStart(string, string) {}
func (f *File) CaseStop(CaseStop)        {}
func (f *File) CaseStopRetry(CaseStop)   {}

// CaseMonitor writes one line to the feedback file and nowhere else.
func (f *File) CaseMonitor(name, action, envID string) {
	if f.Log != nil {
		fmt.Fprintf(f.Log, "%s %s %s\n", action, name, envID)
	}
}

func (f *File) EnvStop(string) {}
