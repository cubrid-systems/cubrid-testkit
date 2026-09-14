package result

import (
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
)

// The sql family's records: what CQT printed and wrote around each case, and
// what run.sh added after it. The spec every byte here is held to is CQT's
// source (CTP develop a1bec87), and it was checked by an independent emulator
// against five real runs, byte for byte (docs/project/evidence/sql-baseline.md).
//
// Where Java's collections leak into the bytes -- Hashtable iteration and a sort
// whose comparator treats a null name as equal to everything -- they are
// emulated rather than tidied, for the reason jtable gives.

// SQLRun is one run of the sql family, as its records need it: what CQT was
// told, and the cases it found.
type SQLRun struct {
	CTPHome string
	Type    string // CQT's testType: sql or medium
	Alias   string // its typeAlias: test_category
	Bits    string // "64" or "32"
	// Build is local.properties' dbversion "." dbbuildnumber, which run.sh
	// writes from cubrid_rel before CQT starts.
	Build string
	// Scenario is the corpus root exactly as CQT was given it, trailing slash
	// included: every relative path in the records is measured from its length.
	Scenario string
	// Mode is "release" or "debug", the JUnit suite's last word; "" writes no
	// report, as CQT does when cubrid_rel says neither.
	Mode string
	// XMLSummary is need_xml_summary: summary.xml, main.info and the failure
	// copies. AnswerInSummary is need_answer_in_summary. QueryPlanAll is
	// System.xml's queryPlan.
	XMLSummary, AnswerInSummary, QueryPlanAll bool
	// Cases is CQT's discovery order. Answers is the answer each is judged
	// against; a case without one is not run.
	Cases   []string
	Answers map[string]string
	// Serial prints a progress line in two halves around its case, as CQT
	// does. With parallel slots a line is printed whole when its case ends.
	Serial bool
	// Out is the run's standard output; Log is CQT's log, which run.sh tees
	// CQT's output into.
	Out, Log io.Writer
	// Failure is a failed case's text for the JUnit report, from CQT's own
	// builder: the lockstep statement diff needs CQT's statement parser.
	Failure func(caseFile string) string
	// Now and Rand name the result directory; nil means the clock and
	// math/rand.
	Now  func() time.Time
	Rand func(n int) int
}

// SQLCase is one case's outcome, handed to the records as it finishes.
type SQLCase struct {
	File  string
	Index int       // 1-based, in the order cases started
	Total int       // CQT's N
	Start time.Time // when it started, for the progress line
	// Ran is false for a case with no answer.
	Ran bool
	OK  bool
	Ms  int64
	// Rendered is what the executor produced; nil when it produced nothing.
	Rendered []byte
	// Err is the executor failing on this case -- CQT itself, not a statement.
	Err     error
	HasCore bool
}

// SQLCounts are the root summary's numbers, which run.sh prints.
type SQLCounts struct {
	Total, Success, Fail int
	TotalTime            int64
}

// SQL writes one run's records.
type SQL struct {
	run    SQLRun
	testID string
	dir    string

	// mu guards the records; outMu the output, on its own so that an
	// executor's words (Say) never wait on End, which holds mu while it asks
	// an executor for failure texts -- an executor whose stderr pipe filled
	// behind a waiting Say would never answer.
	mu    sync.Mutex
	outMu sync.Mutex
	cases map[string]*sqlCase
	cat   *catNode
	xml   *os.File
}

type sqlCase struct {
	file, answer, name, caseDir, bottom string
	shouldRun, ran, ok, hasCore         bool
	ms                                  int64
}

// catNode is one level of CQT's category map: a Hashtable whose keys are
// directory names and, once the level has a summary, "Summary".
type catNode struct {
	keys    *jtable[*catNode]
	summary *summary
}

const summaryKey = "Summary"

type summary struct {
	bottom               bool
	resultDir, catPath   string
	total, success, fail int
	totalTime            int64
	siteRunTimes         int
	cases                []*sqlCase
	children             *jtable[*summary]
	info                 *summaryInfo
}

// OpenSQL names the result directory, makes the directories CQT makes before
// its first case, and opens summary.xml. It prints nothing.
func OpenSQL(run SQLRun) (*SQL, error) {
	now := time.Now
	if run.Now != nil {
		now = run.Now
	}
	rnd := rand.Intn
	if run.Rand != nil {
		rnd = run.Rand
	}
	t := now()
	// TestUtil.getTestId and getResultDir: the month is not padded, and the
	// random suffix is 0..98, not padded either.
	s := &SQL{run: run, cases: map[string]*sqlCase{}}
	s.testID = fmt.Sprintf("schedule_linux_%s_%sbit_%s%d_%s", run.Type, run.Bits, t.Format("02150405"), rnd(99), run.Build)
	s.dir = filepath.Join(run.CTPHome, "sql", "result", fmt.Sprintf("y%d", t.Year()), fmt.Sprintf("m%d", int(t.Month())), s.testID)
	s.build()

	for _, c := range run.Cases {
		if err := os.MkdirAll(s.cases[c].bottom, 0o775); err != nil {
			return nil, err
		}
	}
	if run.XMLSummary {
		f, err := createCQTFile(filepath.Join(s.dir, "summary.xml"), false)
		if err != nil {
			return nil, err
		}
		if _, err := f.WriteString("<results>\n"); err != nil {
			f.Close()
			return nil, err
		}
		s.xml = f
	}
	return s, nil
}

// Root is the result directory.
func (s *SQL) Root() string { return s.dir }

// TestID names this run, as CQT's TestUtil.getTestId does.
func (s *SQL) TestID() string { return s.testID }

// CaseResultDir is CaseResult.getResultDir: the directory a failed case's
// copies go in, which is also where its core stack belongs. It is "" for a
// path that is not a case of this run.
func (s *SQL) CaseResultDir(caseFile string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rc := s.cases[caseFile]; rc != nil {
		return rc.bottom
	}
	return ""
}

// Discard removes a result directory whose run never started: a run whose
// servers or executors could not be brought up has nothing to record, and a
// half-made tree would read as a run that happened.
func (s *SQL) Discard() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.xml != nil {
		s.xml.Close()
		s.xml = nil
	}
	os.RemoveAll(s.dir)
}

// build is ConsoleBO.build: a case record for every case, and the category map
// with a bottom summary for every directory that holds cases.
func (s *SQL) build() {
	s.cat = &catNode{keys: newJTable[*catNode]()}
	summaries := map[string]*summary{}
	for _, file := range s.run.Cases {
		if _, dup := s.cases[file]; dup {
			continue
		}
		rel := file[min(len(s.run.Scenario), len(file)):]
		c := &sqlCase{
			file:    file,
			name:    caseName(file),
			caseDir: filepath.Dir(file),
			bottom:  cutCases(s.dir + "/" + s.run.Type + "/" + rel),
		}
		c.answer, c.shouldRun = s.run.Answers[file]
		if !c.shouldRun {
			c.answer = answerPath(file)
		}
		s.cases[file] = c

		catPath := cutCases(s.run.Type + "/" + rel)
		node := s.cat
		for _, k := range strings.Split(catPath, "/") {
			next, ok := node.keys.get(k)
			if !ok {
				next = &catNode{keys: newJTable[*catNode]()}
				node.keys.put(k, next)
			}
			node = next
		}
		if node.summary == nil {
			node.summary = &summary{bottom: true, resultDir: c.bottom, catPath: catPath}
			node.keys.put(summaryKey, nil)
			summaries[c.bottom] = node.summary
		}
		summaries[c.bottom].cases = append(summaries[c.bottom].cases, c)
	}
}

// cutCases is getTestCatPath's and getResultDir's shape: everything from the
// last "/cases" on goes, and a trailing slash with it.
func cutCases(p string) string {
	if i := strings.LastIndex(p, "/cases"); i >= 0 {
		p = p[:i]
	}
	return strings.TrimSuffix(p, "/")
}

// caseName is FileUtil.getFileName: the base name, cut at its first dot.
func caseName(file string) string {
	name := filepath.Base(file)
	if i := strings.IndexByte(name, '.'); i >= 0 {
		name = name[:i]
	}
	return name
}

// answerPath is TestUtil.getAnswerFile: the last "cases" in the directory
// becomes "answers" -- a substring, not a path component -- and the name gets
// ".answer".
func answerPath(file string) string {
	dir := filepath.Dir(file)
	if i := strings.LastIndex(dir, "cases"); i >= 0 {
		dir = dir[:i] + "answers"
	}
	return dir + "/" + caseName(file) + ".answer"
}

// AnswerPath is the answer CQT looks for first, before any run_mode variant.
func AnswerPath(file string) string { return answerPath(file) }

// Begin prints what CQT prints before its first case: the result directory,
// whatever CQT's own startup printed (startup, in order -- the database's
// <script> element prints two empty lines there), and the marker.
func (s *SQL) Begin(startup []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.print("Result Root Dir:" + s.dir + "\n")
	for _, line := range startup {
		s.print(line + "\n")
	}
	s.print("---------------execute begin--------------------\n")
}

// Say prints a line CQT printed while it ran: an executor's own output.
func (s *SQL) Say(line string) {
	s.print(line + "\n")
}

func (s *SQL) print(text string) {
	s.outMu.Lock()
	defer s.outMu.Unlock()
	io.WriteString(s.run.Out, text)
	if s.run.Log != nil {
		io.WriteString(s.run.Log, text)
	}
}

// progress is the first half of CQT's progress line.
func progress(c SQLCase) string {
	return "[" + c.Start.Format("15:04:05") + "] Testing " + c.File +
		" (" + strconv.Itoa(c.Index) + "/" + strconv.Itoa(c.Total) + " " + percent(c.Index, c.Total) + ")"
}

// percent is ConsoleBO's completeRatio: a float division and multiply, handed
// to a percent format with two decimals as a double over 100, and rounded
// half-even on the exact binary value -- which strconv's correctly rounded
// formatting is. Checked against Java for every i <= N <= 3000.
func percent(i, n int) string {
	r := float32(float32(i) / float32(n))
	r = float32(r * 100)
	p := (float64(r) / 100.0) * 100.0
	return strconv.FormatFloat(p, 'f', 2, 64) + "%"
}

// Starting is a case about to run. A serial run prints the first half of its
// progress line now and flushes it, as CQT does, so that whatever the server
// says during the case lands where it lands in CTP's output.
func (s *SQL) Starting(c SQLCase) {
	if !s.run.Serial {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.print(progress(c))
}

// Case records a finished case: its .result beside it, what a failure leaves
// in the result tree, its summary.xml record, and the rest of its progress
// line.
func (s *SQL) Case(c SQLCase) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rc := s.cases[c.File]
	if rc == nil {
		return fmt.Errorf("%s is not a case of this run", c.File)
	}
	head := ""
	if !s.run.Serial {
		head = progress(c)
	}
	if !rc.shouldRun {
		// CQT prints the prefix and nothing else -- not even the newline, so
		// the next case's line runs on from it. Only a serial run can do that.
		if !s.run.Serial {
			s.print(head + "\n")
		}
		return nil
	}
	rc.ran, rc.ok, rc.ms, rc.hasCore = true, c.OK && c.Err == nil, c.Ms, c.HasCore

	if c.Rendered != nil {
		// Best effort, as FileUtil.writeToFile is: it makes the file writable
		// first and swallows what goes wrong. A read-only corpus loses its
		// .result files and not its run.
		path := filepath.Join(rc.caseDir, rc.name+".result")
		if st, err := os.Stat(path); err == nil {
			os.Chmod(path, st.Mode().Perm()|0o200)
		}
		if err := writeCQTFile(path, c.Rendered); err != nil {
			fmt.Fprintf(os.Stderr, "[WARN] cannot write %s: %v\n", path, err)
		}
	}
	if !rc.ok && c.Rendered != nil {
		if err := writeCQTFile(filepath.Join(rc.bottom, rc.name+".result"), c.Rendered); err != nil {
			return err
		}
	}
	if s.run.XMLSummary {
		if !rc.ok {
			for from, to := range map[string]string{rc.file: rc.name + ".sql", rc.answer: rc.name + ".answer"} {
				if err := copyPlain(from, filepath.Join(rc.bottom, to)); err != nil {
					return err
				}
			}
		}
		verdict := "success"
		if !rc.ok {
			verdict = "fail"
		}
		root := len(s.run.Scenario)
		rec := "  <scenario>\n" +
			"     <case>" + s.run.Type + "/" + rc.file[min(root, len(rc.file)):] + "</case>\n" +
			"     <answer>" + s.run.Type + "/" + rc.answer[min(root, len(rc.answer)):] + "</answer>\n" +
			"     <elapsetime>" + strconv.FormatInt(rc.ms, 10) + "</elapsetime>\n" +
			"     <result>" + verdict + "</result>\n" +
			"  </scenario>\n"
		if _, err := s.xml.WriteString(rec); err != nil {
			return err
		}
	}

	tail := " [OK]\n"
	if !rc.ok {
		tail = " [NOK]\n"
	}
	s.print(head + tail)
	if c.Err != nil {
		// Where CQT's own loop would have ended the run, this one ends the
		// case, and says why.
		s.print("[ERROR] " + c.File + ": " + c.Err.Error() + "\n")
	}
	return nil
}

// End is everything CQT does after its last case: the summary files for every
// directory, the end marker, the JUnit report, the footer of summary.xml and
// main.info. It returns the numbers run.sh prints.
func (s *SQL) End() (SQLCounts, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	root := s.makeSummary()
	if root == nil {
		return SQLCounts{}, fmt.Errorf("no summary: the run had no cases")
	}
	s.makeSummaryInfo(root)
	if err := s.saveResultSummary(s.cat); err != nil {
		return SQLCounts{}, err
	}
	s.print("---------------execute end  --------------------\n")

	if s.run.Mode != "" {
		if err := s.writeJUnit(root); err != nil {
			return SQLCounts{}, err
		}
	}
	if s.xml != nil {
		if _, err := s.xml.WriteString("</results>\n"); err != nil {
			return SQLCounts{}, err
		}
		if err := s.xml.Close(); err != nil {
			return SQLCounts{}, err
		}
		s.xml = nil
	}
	counts := SQLCounts{Total: root.total, Success: root.success, Fail: root.fail, TotalTime: root.totalTime}
	if s.run.XMLSummary {
		info := "build:" + s.run.Build + "\n" +
			"version:" + s.run.Bits + "bit\n" +
			"os:linux\n" +
			"category:" + s.run.Alias + "\n" +
			"elapse_time:" + strconv.FormatInt(root.totalTime, 10) + "\n" +
			"success:" + strconv.Itoa(root.success) + "\n" +
			"fail:" + strconv.Itoa(root.fail) + "\n" +
			"total:" + strconv.Itoa(root.total) + "\n" +
			"execute_case:" + strconv.Itoa(root.success+root.fail) + "\n" +
			"totalTime:" + strconv.FormatInt(root.totalTime, 10) + "\n" +
			"end_time:" + s.now().Format("2006-01-02 15:04:05") + "\n" +
			"result_path:" + s.dir + "\n"
		if err := writeCQTFile(filepath.Join(s.dir, "main.info"), []byte(info)); err != nil {
			return SQLCounts{}, err
		}
	}
	return counts, nil
}

func (s *SQL) now() time.Time {
	if s.run.Now != nil {
		return s.run.Now()
	}
	return time.Now()
}

// Describe is what the status page shows for a finished case: its verdict and
// time, where its answer and its rendering are, and for one that failed, the
// first line where they part -- blank lines aside, since the comparison takes
// line breaks out. "" for a case that has not finished.
func (s *SQL) Describe(file string) string {
	s.mu.Lock()
	c := s.cases[file]
	var ran, ok, shouldRun bool
	var ms int64
	if c != nil {
		ran, ok, shouldRun, ms = c.ran, c.ok, c.shouldRun, c.ms
	}
	s.mu.Unlock()
	if c == nil || (!ran && shouldRun) {
		return ""
	}
	result := filepath.Join(c.caseDir, c.name+".result")
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\nanswer: %s\nresult: %s\n\n", file, c.answer, result)
	switch {
	case !shouldRun:
		b.WriteString("not run: it has no answer, and CQT counts that as a failure\n")
		return b.String()
	case ok:
		fmt.Fprintf(&b, "OK in %d ms\n", ms)
		return b.String()
	}
	fmt.Fprintf(&b, "NOK in %d ms\n", ms)
	want, err1 := os.ReadFile(c.answer)
	got, err2 := os.ReadFile(result)
	if err1 != nil || err2 != nil {
		fmt.Fprintf(&b, "\n(cannot read both: %v %v)\n", err1, err2)
		return b.String()
	}
	a, r := nonBlankLines(string(want)), nonBlankLines(string(got))
	i := 0
	for i < len(a) && i < len(r) && a[i].text == r[i].text {
		i++
	}
	if i == len(a) && i == len(r) {
		b.WriteString("\nthe two differ only in line breaks, which CQT removes before comparing\n")
		return b.String()
	}
	show := func(name string, lines []numbered) {
		fmt.Fprintf(&b, "\n%s, from line %d:\n", name, lineOf(lines, i))
		for j := max(i-3, 0); j < min(i+8, len(lines)); j++ {
			mark := "  "
			if j == i {
				mark = "> "
			}
			fmt.Fprintf(&b, "%s%s\n", mark, lines[j].text)
		}
	}
	b.WriteString("\nfirst difference:\n")
	show("answer", a)
	show("result", r)
	return b.String()
}

type numbered struct {
	n    int
	text string
}

func nonBlankLines(s string) []numbered {
	var out []numbered
	for i, l := range strings.Split(strings.ReplaceAll(s, "\r", ""), "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, numbered{i + 1, l})
		}
	}
	return out
}

func lineOf(lines []numbered, i int) int {
	if i < len(lines) {
		return lines[i].n
	}
	if len(lines) > 0 {
		return lines[len(lines)-1].n + 1
	}
	return 1
}

// MainInfoTail is what run.sh appends to main.info after CQT exits.
func (s *SQL) MainInfoTail(rel, user, machine string) error {
	return appendFile(filepath.Join(s.dir, "main.info"),
		"cubrid_rel:"+rel+"\nuser:"+user+"\nmachine:"+machine+"\n")
}

// TestError is run.sh's mark on a run that left cores.
func (s *SQL) TestError() error {
	return appendFile(filepath.Join(s.dir, "summary_info"), "test_error=Y\n")
}

// Summary prints run.sh's block after CQT: from main.info's numbers, which
// are the same ones End returns.
func (s *SQL) Summary(c SQLCounts, logFile string) {
	s.outMu.Lock()
	defer s.outMu.Unlock()
	fmt.Fprintf(s.run.Out, "\n-----------------------\nFail:%d\nSuccess:%d\nTotal:%d\nElapse Time:%d\nTest Log:%s\nTest Result Directory:%s\n-----------------------\n\n",
		c.Fail, c.Success, c.Total, c.TotalTime, logFile, s.dir)
}

// ---- TestUtil.makeSummary ----------------------------------------------------

// makeSummary is TestUtil.makeSummary, including the path vector it shares
// across its recursion: a directory's result path is rebuilt from the tokens
// of the last bottom it walked through, which is why a non-bottom's summary is
// only as good as the paths below it.
func (s *SQL) makeSummary() *summary {
	var dirPath []string
	var last *summary
	var walk func(n *catNode)
	walk = func(n *catNode) {
		if n.summary != nil && n.summary.bottom {
			dirPath = dirPath[:0]
			sm := n.summary
			last = sm
			for _, c := range sm.cases {
				sm.total++
				if !c.ran {
					continue
				}
				if c.ok {
					sm.success++
				} else {
					sm.fail++
				}
				sm.totalTime += c.ms
			}
			for _, tok := range strings.Split(sm.resultDir, "/") {
				if tok != "" {
					dirPath = append(dirPath, tok)
				}
			}
			return
		}
		for _, k := range n.keys.keys() {
			dirPath = append(dirPath, k)
			child, _ := n.keys.get(k)
			walk(child)
		}
		if len(dirPath) > 0 {
			dirPath = dirPath[:len(dirPath)-1]
		}
		resultDir := "/" + strings.Join(dirPath, "/")
		if !strings.Contains(resultDir, s.testID) {
			return
		}
		sm := &summary{resultDir: resultDir, children: newJTable[*summary]()}
		n.summary = sm
		n.keys.put(summaryKey, nil)
		if len(dirPath) > 0 {
			sm.siteRunTimes = 1
			if strings.EqualFold(dirPath[len(dirPath)-1], "site") {
				sm.siteRunTimes = 0
			}
		}
		sm.catPath = cutCases(strings.TrimPrefix(resultDir[min(len(s.dir), len(resultDir)):], "/"))
		for _, k := range n.keys.keys() {
			if k == summaryKey {
				continue
			}
			child, _ := n.keys.get(k)
			ch := child.summary
			if ch == nil {
				continue
			}
			sm.total += ch.total
			sm.success += ch.success
			sm.fail += ch.fail
			sm.totalTime += ch.totalTime
			sm.children.put(ch.resultDir, ch)
			sm.cases = append(sm.cases, ch.cases...)
		}
		last = sm
	}
	walk(s.cat)
	return last
}

// childSummaries is a summary's children in its Hashtable's order, which is
// the order summary_info lists them and the JUnit report walks them.
func (sm *summary) childSummaries() []*summary {
	var out []*summary
	for _, k := range sm.children.keys() {
		ch, _ := sm.children.get(k)
		out = append(out, ch)
	}
	return out
}

// text is Summary.toString, the whole of a summary_info.
func (sm *summary) text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "total:%d\nsuccess:%d\nfail:%d\ntotalTime:%dms\nSiteRunTimes:%dtimes\n",
		sm.total, sm.success, sm.fail, sm.totalTime, sm.siteRunTimes)
	if id := os.Getenv("MSG_ID"); id != "" {
		fmt.Fprintf(&b, "msg_id:%s\n", id)
	}
	if sm.bottom {
		for _, c := range sm.cases {
			verdict := ""
			if c.ran {
				verdict = "nok"
				if c.ok {
					verdict = "ok"
				}
			}
			fmt.Fprintf(&b, "%s:%s    %dms\n", c.file, verdict, c.ms)
		}
		return b.String()
	}
	for _, ch := range sm.childSummaries() {
		fmt.Fprintf(&b, "%s/:%d    %d    %d\n", ch.catPath, ch.total, ch.success, ch.fail)
	}
	return b.String()
}

// ---- TestUtil.makeSummaryInfo and saveResultSummary -------------------------

type summaryInfo struct {
	parent                     *summaryInfo
	children                   []*summaryInfo
	resultDir, catPath, name   string
	named                      bool
	total, site, success, fail int
	totalTime                  int64
	version                    string
	ok, nok, notRun            []*sqlCase
}

// makeSummaryInfo is TestUtil.makeSummaryInfo.
func (s *SQL) makeSummaryInfo(root *summary) {
	var walk func(n *catNode, parent *summaryInfo) *summaryInfo
	walk = func(n *catNode, parent *summaryInfo) *summaryInfo {
		si := &summaryInfo{}
		if parent == nil {
			si.name, si.named = afterSecondUnderscore(s.testID), true
			si.version = "Main"
			if s.run.Bits == "32" {
				si.version = "32bits"
			}
			si.copyFrom(root)
		} else {
			si.parent = parent
			parent.children = append(parent.children, si)
		}
		for _, k := range n.keys.keys() {
			if k != summaryKey {
				child, _ := n.keys.get(k)
				walk(child, si)
				continue
			}
			sm := n.summary
			sm.info = si
			if !si.named {
				si.name, si.named = sm.catPath[strings.LastIndex(sm.catPath, "/")+1:], true
			}
			si.copyFrom(sm)
			if sm.bottom {
				for _, c := range sm.cases {
					switch {
					case !c.ran:
						si.notRun = append(si.notRun, c)
					case c.ok:
						si.ok = append(si.ok, c)
					default:
						si.nok = append(si.nok, c)
					}
				}
			}
		}
		return si
	}
	walk(s.cat, nil)
}

func (si *summaryInfo) copyFrom(sm *summary) {
	si.resultDir, si.catPath, si.site = sm.resultDir, sm.catPath, sm.siteRunTimes
	si.total, si.success, si.fail, si.totalTime = sm.total, sm.success, sm.fail, sm.totalTime
}

func afterSecondUnderscore(id string) string {
	for range 2 {
		if i := strings.IndexByte(id, '_'); i >= 0 {
			id = id[i+1:]
		}
	}
	return id
}

// childList is SummaryInfo's children as XStream finds them. addChild sorts
// after every append, and the child just appended has no name yet; compareTo
// says a null name equals everything, so it stays where it was put. Nothing
// sorts after the last append. So: every child but the last, sorted by name,
// then the last -- checked against a port of JDK 8's TimSort on 2,770 real
// files.
func (si *summaryInfo) childList() []*summaryInfo {
	n := len(si.children)
	if n < 2 {
		return si.children
	}
	out := append([]*summaryInfo(nil), si.children[:n-1]...)
	sort.SliceStable(out, func(i, j int) bool { return javaLess(out[i].name, out[j].name) })
	return append(out, si.children[n-1])
}

// javaLess is String.compareTo: UTF-16 code units.
func javaLess(a, b string) bool {
	ua, ub := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	for i := 0; i < len(ua) && i < len(ub); i++ {
		if ua[i] != ub[i] {
			return ua[i] < ub[i]
		}
	}
	return len(ua) < len(ub)
}

// saveResultSummary is TestUtil.saveResultSummary: a summary_info and a
// summary.info for every directory, walked in the category map's order. The
// alias rewrite is CQT's and permanent, which is why the order matters: the
// testType node is written before the root, so the root's file shows the
// alias too.
func (s *SQL) saveResultSummary(n *catNode) error {
	for _, k := range n.keys.keys() {
		if k != summaryKey {
			child, _ := n.keys.get(k)
			if err := s.saveResultSummary(child); err != nil {
				return err
			}
			continue
		}
		sm := n.summary
		if err := writeCQTFile(filepath.Join(sm.resultDir, "summary_info"), []byte(sm.text())); err != nil {
			return err
		}
		si := sm.info
		if strings.EqualFold(s.run.Type, si.catPath) && !strings.EqualFold(s.run.Type, s.run.Alias) {
			si.catPath = s.run.Alias
		}
		var b strings.Builder
		s.xstream(&b, si, 0, true)
		if err := writeCQTFile(filepath.Join(si.resultDir, "summary.info"), []byte(strings.TrimSuffix(b.String(), "\n"))); err != nil {
			return err
		}
	}
	return nil
}

// xstream is XStream 1.2.2's PrettyPrintWriter over a SummaryInfo: two spaces
// a level, fields in declaration order, nulls left out, an empty collection as
// <tag/>. The only references in the graph are child-to-parent, and from a
// parent element the parent object is always three levels up.
func (s *SQL) xstream(b *strings.Builder, si *summaryInfo, depth int, top bool) {
	p, q := strings.Repeat("  ", depth), strings.Repeat("  ", depth+1)
	b.WriteString(p + "<summary>\n")
	if !top && si.parent != nil {
		b.WriteString(q + `<parent reference="../../.."/>` + "\n")
	}
	if kids := si.childList(); len(kids) == 0 {
		b.WriteString(q + "<childList/>\n")
	} else {
		b.WriteString(q + "<childList>\n")
		for _, c := range kids {
			s.xstream(b, c, depth+2, false)
		}
		b.WriteString(q + "</childList>\n")
	}
	b.WriteString(q + "<resultDir>" + xmlText(si.resultDir) + "</resultDir>\n")
	b.WriteString(q + "<catPath>" + xmlText(si.catPath) + "</catPath>\n")
	b.WriteString(q + "<name>" + xmlText(si.name) + "</name>\n")
	fmt.Fprintf(b, "%s<totalCount>%d</totalCount>\n%s<siteRunTimes>%d</siteRunTimes>\n%s<successCount>%d</successCount>\n%s<failCount>%d</failCount>\n%s<totalTime>%d</totalTime>\n%s<type>0</type>\n",
		q, si.total, q, si.site, q, si.success, q, si.fail, q, si.totalTime, q)
	if si.version != "" {
		b.WriteString(q + "<version>" + xmlText(si.version) + "</version>\n")
	}
	s.caseList(b, "okList", si.ok, depth+1)
	s.caseList(b, "nokList", si.nok, depth+1)
	s.caseList(b, "notRunList", si.notRun, depth+1)
	b.WriteString(p + "</summary>\n")
}

func (s *SQL) caseList(b *strings.Builder, tag string, cases []*sqlCase, depth int) {
	p, q, r := strings.Repeat("  ", depth), strings.Repeat("  ", depth+1), strings.Repeat("  ", depth+2)
	if len(cases) == 0 {
		b.WriteString(p + "<" + tag + "/>\n")
		return
	}
	b.WriteString(p + "<" + tag + ">\n")
	for _, c := range cases {
		b.WriteString(q + "<caseresult>\n")
		if c.ran {
			// A fresh CaseResult with six fields copied: hasAnswer and
			// printQueryPlan keep their defaults, which is why every entry
			// says false.
			fmt.Fprintf(b, "%s<siteRunTimes>1</siteRunTimes>\n%s<printQueryPlan>false</printQueryPlan>\n%s<type>0</type>\n%s<totalTime>%d</totalTime>\n",
				r, r, r, r, c.ms)
			b.WriteString(r + "<caseFile>" + xmlText(c.file) + "</caseFile>\n")
			if s.run.AnswerInSummary {
				b.WriteString(r + "<answerFile>" + xmlText(c.answer) + "</answerFile>\n")
			}
			fmt.Fprintf(b, "%s<hasAnswer>false</hasAnswer>\n%s<isSuccessFul>%t</isSuccessFul>\n%s<hasCore>%t</hasCore>\n%s<shouldRun>true</shouldRun>\n",
				r, r, c.ok, r, c.hasCore, r)
		} else {
			// The original CaseResult, with every field build() set.
			fmt.Fprintf(b, "%s<siteRunTimes>1</siteRunTimes>\n%s<printQueryPlan>%t</printQueryPlan>\n%s<type>0</type>\n%s<totalTime>0</totalTime>\n",
				r, r, s.printsQueryPlan(c.file), r, r)
			b.WriteString(r + "<caseDir>" + xmlText(c.caseDir) + "</caseDir>\n")
			b.WriteString(r + "<caseFile>" + xmlText(c.file) + "</caseFile>\n")
			b.WriteString(r + "<caseName>" + xmlText(c.name) + "</caseName>\n")
			b.WriteString(r + "<answerFile>" + xmlText(c.answer) + "</answerFile>\n")
			b.WriteString(r + "<hasAnswer>false</hasAnswer>\n")
			b.WriteString(r + "<resultDir>" + xmlText(c.bottom) + "</resultDir>\n")
			fmt.Fprintf(b, "%s<isSuccessFul>true</isSuccessFul>\n%s<hasCore>false</hasCore>\n%s<shouldRun>false</shouldRun>\n", r, r, r)
		}
		b.WriteString(q + "</caseresult>\n")
	}
	b.WriteString(p + "</" + tag + ">\n")
}

// printsQueryPlan is TestUtil.isPrintQueryPlan.
func (s *SQL) printsQueryPlan(file string) bool {
	if s.run.QueryPlanAll {
		return true
	}
	dot := strings.LastIndex(file, ".")
	if dot < 0 {
		return false
	}
	_, err := os.Stat(file[:dot] + ".queryPlan")
	return err == nil
}

// xmlText is PrettyPrintWriter.writeText's escaping.
func xmlText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		case '\r':
			b.WriteString("&#x0D;")
		case 0:
			b.WriteString("&#x0;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ---- JunitXmlWriter ----------------------------------------------------------

var junitAnchors = []string{"cubrid-testcases-private-ex", "cubrid-testcases-private", "cubrid-testcases"}

// writeJUnit is JunitXmlWriter.write: every case that ran, walking the
// summaries as makeSummary linked them.
func (s *SQL) writeJUnit(root *summary) error {
	var cases []*sqlCase
	var walk func(sm *summary)
	walk = func(sm *summary) {
		if sm.bottom {
			for _, c := range sm.cases {
				if c.ran {
					cases = append(cases, c)
				}
			}
			return
		}
		for _, ch := range sm.childSummaries() {
			walk(ch)
		}
	}
	walk(root)

	target := "linux_" + s.run.Type + "_" + s.run.Bits + "bit"
	suite := target + "_" + s.run.Mode
	failures := 0
	for _, c := range cases {
		if !c.ok {
			failures++
		}
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n<testsuites>\n")
	fmt.Fprintf(&b, `  <testsuite name="%s" tests="%d" failures="%d">`, xmlAttr(suite), len(cases), failures)
	for _, c := range cases {
		name, file := c.file, c.file
		for _, anchor := range junitAnchors {
			if i := strings.Index(c.file, "/"+anchor+"/"); i >= 0 {
				name = c.file[i+len(anchor)+2:]
				file = anchor + "/" + name
				break
			}
		}
		fmt.Fprintf(&b, "\n    <testcase classname=\"%s\" name=\"%s\" file=\"%s\" time=\"%d.%03d\">",
			xmlAttr(suite), xmlAttr(name), xmlAttr(file), c.ms/1000, c.ms%1000)
		if c.ok {
			b.WriteString("</testcase>")
			continue
		}
		b.WriteString("\n      <failure message=\"unexpected result\">")
		if s.run.Failure != nil {
			if payload := s.run.Failure(c.file); payload != "" {
				// Written raw, "]]>" and all, because that is what CQT writes.
				// It hands the text to XMLStreamWriter.writeCData, and JDK 8's
				// writer does not break the one sequence a CDATA section cannot
				// contain -- measured: writeCData("before ]]> after") comes out
				// as <![CDATA[before ]]> after]]>, which ends the section early
				// and leaves the report invalid XML.
				//
				// So a failing case whose output contains "]]>" produces a
				// report no parser will read, in CTP and here alike. Splitting
				// it (feedback/junit.go does, for shell's own report, which is
				// not compared) would be a better report and a different file,
				// and this one is compared byte for byte. It belongs upstream,
				// in CQT (module-sql.md §6).
				b.WriteString("<![CDATA[" + payload + "]]>")
			}
		}
		b.WriteString("</failure>\n    </testcase>")
	}
	if len(cases) > 0 {
		b.WriteString("\n  ")
	}
	b.WriteString("</testsuite>\n</testsuites>")
	f, err := os.Create(filepath.Join(s.dir, target+".xml"))
	if err != nil {
		return err
	}
	if _, err := f.WriteString(b.String()); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// xmlAttr is the JDK stream writer's attribute escaping.
func xmlAttr(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

// ---- files as CQT writes them --------------------------------------------------

// writeCQTFile is FileUtil.writeToFile: UTF-8, truncating, and the owner's
// execute bit, which setExecutable(true) adds to everything CQT writes.
func writeCQTFile(path string, data []byte) error {
	f, err := createCQTFile(path, true)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func createCQTFile(path string, truncate bool) (*os.File, error) {
	flags := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	if truncate {
		flags = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	}
	f, err := os.OpenFile(path, flags, 0o666)
	if err != nil {
		return nil, err
	}
	if st, err := f.Stat(); err == nil {
		f.Chmod(st.Mode().Perm() | 0o300)
	}
	return f, nil
}

// copyPlain is FileUtil's channel copy: the bytes, and the mode a new file
// gets.
func copyPlain(from, to string) error {
	data, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return os.WriteFile(to, data, 0o666)
}

func appendFile(path, text string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(text); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
