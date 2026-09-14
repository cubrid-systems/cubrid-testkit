package sqlsuite

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/result"
)

// TestTheRecordsAreCQTs rebuilds a real CQT run's records from that run's own
// verdicts and times, and compares every file and every line of CQT's output,
// byte for byte, with what CQT wrote. It is the check that the records half of
// ADR-016's split is CQT's: discovery, N and the percentages, the Hashtable
// orders, the XStream and JUnit layouts, main.info.
//
// It needs a real run, so it runs only when pointed at one:
//
//	TESTKIT_CQT_REF   a copy of the run's result directory, modes kept (cp -a)
//	TESTKIT_CQT_OUT   the run's standard output, as captured
//
// The records are written where CQT wrote them, because the orders hash those
// very paths: run it with the run's result root behind a scratch mount, and
// the corpus behind an overlay, since a failed case's .result is written
// beside it. docs/evidence/sql/records.sh does both.
func TestTheRecordsAreCQTs(t *testing.T) {
	ref, capture := os.Getenv("TESTKIT_CQT_REF"), os.Getenv("TESTKIT_CQT_OUT")
	if ref == "" || capture == "" {
		t.Skip("not pointed at a real CQT run; set TESTKIT_CQT_REF and TESTKIT_CQT_OUT")
	}
	read := func(path string) string {
		t.Helper()
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	// What the run was, from its own records.
	m := regexp.MustCompile(`^schedule_linux_([a-z]+)_(\d+)bit_(\d{8})(\d{1,2})_(.+)$`).FindStringSubmatch(filepath.Base(ref))
	if m == nil {
		t.Fatalf("%s is not a CQT result directory name", filepath.Base(ref))
	}
	typ, bits, stamp, build := m[1], m[2], m[3], m[5]
	rnd, _ := strconv.Atoi(m[4])
	info := map[string]string{}
	for _, line := range strings.Split(read(filepath.Join(ref, "main.info")), "\n") {
		if k, v, ok := strings.Cut(line, ":"); ok {
			info[k] = v
		}
	}
	resultPath := info["result_path"]
	ctpHome := resultPath[:strings.Index(resultPath, "/sql/result/")]
	ym := regexp.MustCompile(`/y(\d+)/m(\d+)/`).FindStringSubmatch(resultPath)
	year, _ := strconv.Atoi(ym[1])
	month, _ := strconv.Atoi(ym[2])
	day, _ := strconv.Atoi(stamp[0:2])
	hh, _ := strconv.Atoi(stamp[2:4])
	mm, _ := strconv.Atoi(stamp[4:6])
	ss, _ := strconv.Atoi(stamp[6:8])
	started := time.Date(year, time.Month(month), day, hh, mm, ss, 0, time.Local)
	ended, err := time.ParseInLocation("2006-01-02 15:04:05", info["end_time"], time.Local)
	if err != nil {
		t.Fatal(err)
	}

	// Each case's verdict and time, from summary.xml; its start, from the
	// progress lines -- read across line breaks, as ADR-017 reads them.
	type rec struct {
		ms int64
		ok bool
	}
	verdicts := map[string]rec{}
	first := ""
	for _, x := range regexp.MustCompile(`<case>(.*?)</case>\s*<answer>.*?</answer>\s*<elapsetime>(\d+)</elapsetime>\s*<result>(\w+)</result>`).
		FindAllStringSubmatch(read(filepath.Join(ref, "summary.xml")), -1) {
		ms, _ := strconv.ParseInt(x[2], 10, 64)
		verdicts[x[1]] = rec{ms, x[3] == "success"}
		if first == "" {
			first = strings.TrimPrefix(x[1], typ+"/")
		}
	}
	out := read(capture)
	cqtOut := out[strings.Index(out, "Result Root Dir:"):]
	cqtOut = cqtOut[:strings.Index(cqtOut, "---------------execute end  --------------------\n")+len("---------------execute end  --------------------\n")]
	progress := regexp.MustCompile(`\[(\d\d):(\d\d):(\d\d)\] Testing (\S+) \(\d+/\d+ [\d.]+%\)`).FindAllStringSubmatch(cqtOut, -1)
	if len(progress) == 0 || first == "" {
		t.Fatal("no progress lines in the capture, or no records in summary.xml")
	}
	// The corpus root is the first case's path less the part summary.xml
	// records; every case in the corpus has an answer, so the first case ran.
	scenario, ok := strings.CutSuffix(progress[0][4], first)
	if !ok {
		t.Fatalf("the first case %s is not summary.xml's first record %s", progress[0][4], first)
	}

	// A failed case's text for the report is CQT's; here it is taken back out
	// of the report CQT wrote.
	failures := map[string]string{}
	for _, x := range regexp.MustCompile(`(?s)<testcase [^>]*name="([^"]*)"[^>]*>\s*<failure message="unexpected result"><!\[CDATA\[(.*?)\]\]></failure>`).
		FindAllStringSubmatch(read(filepath.Join(ref, "linux_"+typ+"_"+bits+"bit.xml")), -1) {
		failures[x[1]] = x[2]
	}

	st := &settings{scenario: scenario}
	cfg, err := readCQTConfig(ctpHome, "test_default.xml")
	if err != nil {
		t.Fatal(err)
	}
	cases, err := discover(st, cfg)
	if err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	calls := 0
	r, err := result.OpenSQL(result.SQLRun{
		CTPHome: ctpHome, Type: typ, Alias: info["category"], Bits: bits, Build: build,
		Scenario: scenario, Mode: "release", XMLSummary: cfg.xmlSummary,
		AnswerInSummary: cfg.answerInSummary, QueryPlanAll: cfg.queryPlanAll,
		Cases: cases.all, Answers: cases.answer, Serial: true, Out: &stdout,
		Failure: func(file string) string {
			return failures[file[strings.Index(file, "/cubrid-testcases/")+len("/cubrid-testcases/"):]]
		},
		Now: func() time.Time {
			calls++
			if calls == 1 {
				return started
			}
			return ended
		},
		Rand: func(int) int { return rnd },
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.Root() != resultPath {
		t.Fatalf("the result directory is %s, CQT's was %s", r.Root(), resultPath)
	}
	r.Begin([]string{"", ""})
	for i, file := range cases.all {
		c := result.SQLCase{File: file, Index: i + 1, Total: len(cases.all)}
		p := progress[i]
		if p[4] != file {
			t.Fatalf("case %d is %s here and %s in CQT's run", i+1, file, p[4])
		}
		h, _ := strconv.Atoi(p[1])
		mi, _ := strconv.Atoi(p[2])
		s, _ := strconv.Atoi(p[3])
		c.Start = time.Date(year, time.Month(month), day, h, mi, s, 0, time.Local)
		r.Starting(c)
		if _, has := cases.answer[file]; has {
			v := verdicts[typ+"/"+file[len(scenario):]]
			c.Ran, c.OK, c.Ms = true, v.ok, v.ms
			if !v.ok {
				bottom := filepath.Join(ref, typ, strings.TrimSuffix(filepath.Dir(file[len(scenario):]), "/cases"))
				c.Rendered = []byte(read(filepath.Join(bottom, strings.TrimSuffix(filepath.Base(file), ".sql")+".result")))
			}
		}
		if err := r.Case(c); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.End(); err != nil {
		t.Fatal(err)
	}
	if err := r.MainInfoTail(info["cubrid_rel"], info["user"], info["machine"]); err != nil {
		t.Fatal(err)
	}

	// The output, byte for byte, once the server's own words are taken out of
	// the capture: CTP's server shares CQT's standard output and once in a
	// while lands between the two halves of a progress line (ADR-017); this
	// test has no server.
	const server = "*** XASL generation failed ***\n"
	interleaved := strings.Count(cqtOut, server)
	if want := strings.ReplaceAll(cqtOut, server, ""); stdout.String() != want {
		t.Errorf("stdout differs %s", firstDifference(want, stdout.String()))
	}
	t.Logf("stdout: %d bytes, %d server message(s) taken out of the capture", len(cqtOut), interleaved)

	// Every file CQT wrote, and nothing it did not.
	compared := 0
	err = filepath.WalkDir(ref, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(ref, path)
		mine := filepath.Join(resultPath, rel)
		a, b := read(path), ""
		if data, err := os.ReadFile(mine); err == nil {
			b = string(data)
		} else {
			t.Errorf("%s: CQT wrote it, this did not", rel)
			return nil
		}
		compared++
		if a != b {
			t.Errorf("%s differs: %s", rel, firstDifference(a, b))
		}
		sa, _ := os.Stat(path)
		sb, _ := os.Stat(mine)
		if sa.Mode().Perm()&0o100 != sb.Mode().Perm()&0o100 {
			t.Errorf("%s: owner execute bit is %v in CQT's, %v here", rel, sa.Mode().Perm(), sb.Mode().Perm())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	filepath.WalkDir(resultPath, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(resultPath, path)
			if _, err := os.Stat(filepath.Join(ref, rel)); err != nil {
				t.Errorf("%s: written here, not by CQT", rel)
			}
		}
		return nil
	})
	t.Logf("%d cases, %d files compared", len(cases.all), compared)
}

func firstDifference(a, b string) string {
	i := 0
	for i < len(a) && i < len(b) && a[i] == b[i] {
		i++
	}
	lo := max(i-60, 0)
	return "at byte " + strconv.Itoa(i) + ":\n  CQT:  " + strconv.Quote(a[lo:min(i+60, len(a))]) +
		"\n  here: " + strconv.Quote(b[lo:min(i+60, len(b))])
}
