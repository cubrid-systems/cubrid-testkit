package feedback

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

// junit writes test-<category>.xml.
//
// The file is JUnit XML, which is what makes a run's results readable by
// something other than a person -- and the one output here that is consumed by a
// machine rather than compared against a previous run. It is graded F2: the same
// elements and attributes, not the same bytes, because CTP produced it through
// IndentingXMLStreamWriter and matching that library's whitespace exactly would
// be reproducing an accident.
//
// docs/project/concept/external-surface-freeze.md.
type junit struct {
	f        *os.File
	w        *bufio.Writer
	category string

	// suiteOpen is what CTP got wrong on a resumed run. writeTestSuiteStart was
	// only reached from setTotalTestCase, which returns early in continue mode --
	// so <testsuite> was never opened, cases were written as direct children of
	// <testsuites>, and the close wrote one end-element too many. That throws, and
	// because the flush and the close sit after it in the same block, the file was
	// left short. Here the suite is opened on first use, whichever call gets there
	// first, so the document is well-formed in both modes.
	suiteOpen bool
	closed    bool
}

func openJUnit(path, category string) (*junit, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("test report: %w", err)
	}
	j := &junit{f: f, w: bufio.NewWriter(f), category: category}
	fmt.Fprint(j.w, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<testsuites>\n")
	return j, nil
}

// suiteStart opens the suite element with the counts known at the time, which is
// before any case has run. CTP wrote them there too, so they describe the pool
// rather than the outcome.
func (j *junit) suiteStart(tests, skipped int) {
	if j == nil || j.suiteOpen {
		return
	}
	j.suiteOpen = true
	fmt.Fprintf(j.w, "  <testsuite name=%s tests=%s skipped=%s timestamp=%s>\n",
		attr(j.category), attr(fmt.Sprint(tests)), attr(fmt.Sprint(skipped)),
		attr(javaDate(time.Now())))
}

// testCase writes one case. An empty outcome is a pass, which in JUnit is a
// testcase element with nothing inside it.
func (j *junit) testCase(name, envID string, elapsed time.Duration, outcome, message, details string) {
	if j == nil {
		return
	}
	j.suiteStart(0, 0)

	fmt.Fprintf(j.w, "    <testcase classname=%s name=%s file=%s time=%s",
		attr(caseURL(name, j.category)), attr(caseRelative(name, j.category)),
		attr(name), attr(fmt.Sprintf("%g", elapsed.Seconds())))
	if outcome == "" {
		fmt.Fprint(j.w, "/>\n")
		return
	}
	fmt.Fprint(j.w, ">\n")

	fmt.Fprintf(j.w, "      <%s message=%s", outcome, attr(message))
	switch outcome {
	case "failure":
		fmt.Fprintf(j.w, " type=%s", attr("TestFailure"))
	case "error":
		fmt.Fprintf(j.w, " type=%s", attr("UnknownStatus"))
	}
	if strings.TrimSpace(details) == "" {
		fmt.Fprintf(j.w, "/>\n    </testcase>\n")
		return
	}
	fmt.Fprint(j.w, ">")
	fmt.Fprintf(j.w, "<![CDATA[\nEnvIdentify: %s\nHostname: %s\n%s\n]]>",
		envID, orNull(os.Getenv("HOSTNAME")), cdataSafe(details))
	fmt.Fprintf(j.w, "</%s>\n    </testcase>\n", outcome)
}

func (j *junit) close() error {
	if j == nil || j.closed {
		return nil
	}
	j.closed = true
	if j.suiteOpen {
		fmt.Fprint(j.w, "  </testsuite>\n")
	}
	fmt.Fprint(j.w, "</testsuites>\n")
	if err := j.w.Flush(); err != nil {
		j.f.Close()
		return err
	}
	return j.f.Close()
}

// attr renders an attribute value with quotes and the five escapes.
func attr(v string) string {
	r := strings.NewReplacer(
		"&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;",
		"\n", "&#10;", "\r", "&#13;", "\t", "&#9;",
	)
	return `"` + r.Replace(v) + `"`
}

// cdataSafe breaks up the one sequence a CDATA section cannot contain. A case's
// output is arbitrary text and has no reason to avoid "]]>".
func cdataSafe(s string) string { return strings.ReplaceAll(s, "]]>", "]]]]><![CDATA[>") }

// caseURL is the classname attribute, which CTP filled with a GitHub link rather
// than a class name. Whatever reads the report gets a clickable path to the case.
func caseURL(testCase, category string) string {
	rel, ok := relativeToCategory(testCase, category)
	if !ok {
		return testCase
	}
	return "https://github.com/CUBRID/cubrid-testcases-private-ex/blob/develop/" + rel
}

func caseRelative(testCase, category string) string {
	rel, ok := relativeToCategory(testCase, category)
	if !ok {
		return testCase
	}
	return rel
}

// relativeToCategory trims everything before the category directory, so a case
// reads the same whatever absolute path it was found under.
func relativeToCategory(testCase, category string) (string, bool) {
	if testCase == "" || category == "" {
		return testCase, false
	}
	if i := strings.Index(testCase, "/"+category+"/"); i != -1 {
		return testCase[i+1:], true
	}
	if strings.HasPrefix(testCase, category+"/") {
		return testCase, true
	}
	return testCase, false
}
