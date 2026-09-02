package feedback

import (
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func open(t *testing.T, continueMode bool) (*File, string, *bytes.Buffer) {
	t.Helper()
	dir := t.TempDir()
	out := &bytes.Buffer{}
	f, err := Open(dir, "shell", out, continueMode, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f, dir, out
}

func read(t *testing.T, dir, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func pass(name string) CaseStop {
	return CaseStop{Case: name, EnvID: "EnvId=env1[host]", Success: true, Elapsed: 1500 * time.Millisecond}
}

func fail(name string, retry int) CaseStop {
	return CaseStop{
		Case: name, EnvID: "EnvId=env1[host]", Success: false,
		Elapsed: 2 * time.Second, ResultText: " : NOK it did not work", RetryCount: retry,
	}
}

func TestTheSummaryIsWhatAnyoneActuallyReads(t *testing.T) {
	f, _, out := open(t, false)

	f.TaskStart("")
	f.TotalTestCase(3, 1, 0)
	f.CaseStop(pass("shell/a/cases/a.sh"))
	f.CaseStop(fail("shell/b/cases/b.sh", 0))
	f.CaseStop(CaseStop{Case: "shell/c/cases/c.sh", SkipType: SkipTypeByMacro, Elapsed: -1})
	f.TaskStop()

	got := out.String()
	for _, want := range []string{
		"Test Category:shell",
		"The Number of Test Cases: 3 (macro skipped: 1, bug skipped: 0)",
		"============= PRINT SUMMARY ==================",
		"Total Case:3",
		"Total Execution Case:2",
		"Total Success Case:1",
		"Total Fail Case:1",
		"Total Skip Case:1",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// The feedback log spells the retry flag as a label, "[NOK]: TRY-> = 2", while
// the console spells it as a prefix, "[NOK], TRY->2". Two spellings of the same
// thing, both frozen, and only one of them is where anyone looks.
func TestTheFeedbackLogSpellsTheRetryFlagItsOwnWay(t *testing.T) {
	f, dir, _ := open(t, false)
	f.TaskStart("")
	f.CaseStop(fail("shell/b/cases/b.sh", 2))
	f.Close()

	log := read(t, dir, "feedback.log")
	if !strings.Contains(log, "[NOK]: TRY-> = 2 shell/b/cases/b.sh 2000 EnvId=env1[host]") {
		t.Errorf("the failing line is not what CTP wrote:\n%s", log)
	}
}

func TestAPassingCaseLineCarriesItsElapsedMilliseconds(t *testing.T) {
	f, dir, _ := open(t, false)
	f.TaskStart("")
	f.CaseStop(pass("shell/a/cases/a.sh"))
	f.Close()

	if want := "[OK]:  shell/a/cases/a.sh 1500 EnvId=env1[host]"; !strings.Contains(read(t, dir, "feedback.log"), want) {
		t.Errorf("missing %q in:\n%s", want, read(t, dir, "feedback.log"))
	}
}

// A retry is not a verdict. It appears in the log so a reader can see the case
// was tried, and it must not move a counter or the run's totals stop adding up.
func TestARetryMovesNoCounter(t *testing.T) {
	f, _, _ := open(t, false)
	f.TaskStart("")
	f.TotalTestCase(1, 0, 0)
	f.CaseStopRetry(fail("shell/b/cases/b.sh", 1))

	if s := f.Stats(); s.Fail != 0 || s.Executed != 0 {
		t.Errorf("a retry moved the counters: %+v", s)
	}
	f.CaseStop(pass("shell/b/cases/b.sh"))
	if s := f.Stats(); s.Success != 1 || s.Fail != 0 {
		t.Errorf("the eventual pass was not counted alone: %+v", s)
	}
}

func TestTheCountersAreReadableAsProperties(t *testing.T) {
	f, dir, _ := open(t, false)
	f.TaskStart("")
	f.TotalTestCase(2, 0, 0)
	f.CaseStop(pass("shell/a/cases/a.sh"))

	data := read(t, dir, statusFile)
	for _, want := range []string{
		"total_case_count=1",
		"total_executed_case_count=1",
		"total_success_case_count=1",
		"total_fail_case_count=0",
		"total_skip_case_count=0",
	} {
		if !strings.Contains(data, want) {
			t.Errorf("missing %q in:\n%s", want, data)
		}
	}
}

// CTP wrote this file with Properties.store, which stamps the current time into a
// comment and emits keys in hash order -- so it differed on every run for reasons
// that had nothing to do with the run.
func TestTheCountersFileIsTheSameTwice(t *testing.T) {
	write := func() string {
		f, dir, _ := open(t, false)
		f.TaskStart("")
		f.TotalTestCase(1, 0, 0)
		f.CaseStop(pass("shell/a/cases/a.sh"))
		f.Close()
		return read(t, dir, statusFile)
	}
	if first, second := write(), write(); first != second {
		t.Errorf("two identical runs produced different counters:\n%q\nvs\n%q", first, second)
	}
}

func TestAResumedRunPicksTheCountersUp(t *testing.T) {
	dir := t.TempDir()

	first, err := Open(dir, "shell", nil, false, "")
	if err != nil {
		t.Fatal(err)
	}
	first.TaskStart("")
	first.TotalTestCase(2, 0, 0)
	first.CaseStop(pass("shell/a/cases/a.sh"))
	first.Close()

	second, err := Open(dir, "shell", nil, true, "")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if got := second.Stats().Success; got != 1 {
		t.Fatalf("the resumed run starts at %d successes, want 1", got)
	}

	second.TaskContinue()
	second.CaseStop(pass("shell/b/cases/b.sh"))
	if got := second.Stats().Success; got != 2 {
		t.Errorf("after the resumed run: %d successes, want 2", got)
	}
}

// setTotalTestCase returns early on a resumed run, so the two counts are absent
// from a resumed run's console. That is CTP's behaviour and it is why a resumed
// run looks like it started in the middle.
func TestAResumedRunDoesNotReprintTheCounts(t *testing.T) {
	f, _, out := open(t, true)
	f.TotalTestCase(5, 0, 0)
	if strings.Contains(out.String(), "The Number of Test Cases") {
		t.Errorf("a resumed run reprinted the counts:\n%s", out.String())
	}
}

// ---------------------------------------------------------------------------
// The XML report.

func TestTheReportIsWellFormedJUnit(t *testing.T) {
	f, dir, _ := open(t, false)
	f.TaskStart("")
	f.TotalTestCase(3, 1, 0)
	f.CaseStop(pass("shell/a/cases/a.sh"))
	f.CaseStop(fail("shell/b/cases/b.sh", 0))
	f.CaseStop(CaseStop{Case: "shell/c/cases/c.sh", SkipType: SkipTypeByMacro})
	f.TaskStop()
	f.Close()

	body := read(t, dir, "test-shell.xml")

	var suites struct {
		Suites []struct {
			Name  string `xml:"name,attr"`
			Tests string `xml:"tests,attr"`
			Cases []struct {
				Classname string `xml:"classname,attr"`
				Name      string `xml:"name,attr"`
				File      string `xml:"file,attr"`
				Failure   *struct {
					Message string `xml:"message,attr"`
					Type    string `xml:"type,attr"`
					Text    string `xml:",chardata"`
				} `xml:"failure"`
				Skipped *struct {
					Message string `xml:"message,attr"`
				} `xml:"skipped"`
			} `xml:"testcase"`
		} `xml:"testsuite"`
	}
	if err := xml.Unmarshal([]byte(body), &suites); err != nil {
		t.Fatalf("the report is not parseable XML: %v\n%s", err, body)
	}
	if len(suites.Suites) != 1 {
		t.Fatalf("got %d suites, want 1:\n%s", len(suites.Suites), body)
	}
	s := suites.Suites[0]
	if s.Name != "shell" {
		t.Errorf("suite name = %q", s.Name)
	}
	if len(s.Cases) != 3 {
		t.Fatalf("got %d cases, want 3:\n%s", len(s.Cases), body)
	}
	if s.Cases[1].Failure == nil || s.Cases[1].Failure.Type != "TestFailure" {
		t.Errorf("the failing case has no failure element:\n%s", body)
	}
	if !strings.Contains(s.Cases[1].Failure.Text, "it did not work") {
		t.Errorf("the failure detail is missing:\n%s", body)
	}
	if s.Cases[2].Skipped == nil {
		t.Errorf("the skipped case has no skipped element:\n%s", body)
	}
	if want := "https://github.com/CUBRID/cubrid-testcases-private-ex/blob/develop/shell/a/cases/a.sh"; s.Cases[0].Classname != want {
		t.Errorf("classname = %q, want %q", s.Cases[0].Classname, want)
	}
}

// CTP's resumed runs produced a broken report: writeTestSuiteStart was reached
// only from setTotalTestCase, which returns early in continue mode, so no
// <testsuite> was ever opened -- and then the close wrote one end element too
// many, threw, and skipped its own flush. Opening the suite on first use makes
// both modes produce a document that parses.
func TestAResumedRunStillProducesAParseableReport(t *testing.T) {
	f, dir, _ := open(t, true)
	f.TaskContinue()
	f.TotalTestCase(5, 0, 0) // returns early, as CTP's did
	f.CaseStop(pass("shell/a/cases/a.sh"))
	f.TaskStop()
	f.Close()

	body := read(t, dir, "test-shell.xml")
	var v any
	if err := xml.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("a resumed run's report does not parse: %v\n%s", err, body)
	}
	if !strings.Contains(body, "<testsuite ") {
		t.Errorf("the case was written outside any suite:\n%s", body)
	}
}

func TestCaseOutputCannotBreakOutOfTheCDATASection(t *testing.T) {
	f, dir, _ := open(t, false)
	f.TaskStart("")
	f.TotalTestCase(1, 0, 0)
	f.CaseStop(CaseStop{
		Case: "shell/x/cases/x.sh", Success: false,
		ResultText: `a case printed ]]> and then <injected/> & "quotes"`,
	})
	f.Close()

	body := read(t, dir, "test-shell.xml")
	var v any
	if err := xml.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("hostile case output broke the report: %v\n%s", err, body)
	}
}

func TestAnUnknownPathKeepsItsFullName(t *testing.T) {
	if got := caseRelative("/somewhere/else/x.sh", "shell"); got != "/somewhere/else/x.sh" {
		t.Errorf("got %q", got)
	}
	if got := caseRelative("/home/qa/cubrid-testcases-private-ex/shell/a/cases/a.sh", "shell"); got != "shell/a/cases/a.sh" {
		t.Errorf("got %q", got)
	}
}

func TestConsoleWritesNoFiles(t *testing.T) {
	out := &bytes.Buffer{}
	f := Console("unittest", out)
	f.TaskStart("")
	f.TotalTestCase(2, 0, 0)
	f.CaseStop(pass("a"))
	f.TaskStop()

	if !strings.Contains(out.String(), "Test Category:unittest") {
		t.Errorf("the console lines are missing:\n%s", out.String())
	}
}
