// Package feedback reports how a run is going.
//
// The events are axis T -- they describe the test run itself. Where they are
// *stored* is axis O, which is why there is no database backend here: FeedbackDB
// was excluded (docs/project/concept/migration-exclusions.md §1-5). Null and File remain.
//
// docs/project/design/contracts.md C5.
package feedback

import "time"

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

// Null discards everything. It is what feedback_type is set to anything other
// than "file" or "database" -- the default, when the key is absent, is File.
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
