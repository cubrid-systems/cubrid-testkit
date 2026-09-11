package result

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The orders below are what JDK 8's Hashtable iterates for these keys, run on
// a real JVM: before a rehash, at the ninth key (11 buckets to 23), and after
// the second (23 to 47). CQT's summary files are listed in this order.
func TestJTableIteratesAsJDK8Does(t *testing.T) {
	keys := []string{"_01_fixed", "_02_xtests", "_03_full_mdb", "_04_full", "_05_err_x", "_06_fulltests", "_07_mc_dep", "_08_mc_ind", "Summary",
		"/r/y2026/m9/schedule_linux_medium_64bit_1114081778_11.5.0.2560-a7a1db8/medium/_01_fixed",
		"_09", "_10", "_11", "_12", "_13", "_14", "_15", "_16", "_17", "_18"}
	want := map[int]string{
		8:  "_01_fixed,_05_err_x,_03_full_mdb,_04_full,_02_xtests,_08_mc_ind,_07_mc_dep,_06_fulltests",
		9:  "Summary,_04_full,_07_mc_dep,_03_full_mdb,_02_xtests,_05_err_x,_01_fixed,_06_fulltests,_08_mc_ind",
		20: "_18,_17,_16,_15,_06_fulltests,_14,_13,_12,_11,_07_mc_dep,_10,_01_fixed,_02_xtests,_03_full_mdb,/r/y2026/m9/schedule_linux_medium_64bit_1114081778_11.5.0.2560-a7a1db8/medium/_01_fixed,Summary,_09,_08_mc_ind,_05_err_x,_04_full",
	}
	tb := newJTable[int]()
	for i, k := range keys {
		tb.put(k, i)
		if w, ok := want[i+1]; ok {
			if got := strings.Join(tb.keys(), ","); got != w {
				t.Errorf("after %d keys:\n got %s\nwant %s", i+1, got, w)
			}
		}
	}
	tb.put("_01_fixed", 99)
	if v, _ := tb.get("_01_fixed"); v != 99 || tb.count != len(keys) {
		t.Error("putting an existing key should replace its value in place")
	}
	if jhash("Summary") != -192987258 || jhash("été") != 227742 {
		t.Error("jhash is not String.hashCode")
	}
}

// Java's percent format rounds half-even on the exact binary value, and a
// float division comes first: 1/32 is exactly 3.125%, which is 3.12% there and
// 3.13% to anything that rounds half-up. The rest are progress lines from real
// runs.
func TestPercentIsCQTs(t *testing.T) {
	for _, c := range []struct {
		i, n int
		want string
	}{
		{1, 32, "3.12%"}, {5, 32, "15.62%"}, {29, 32, "90.62%"}, {10, 64, "15.62%"},
		{1, 975, "0.10%"}, {3, 975, "0.31%"}, {131, 975, "13.44%"}, {975, 975, "100.00%"},
		{16953, 17459, "97.10%"}, {16954, 17459, "97.11%"}, {16955, 17459, "97.11%"},
	} {
		if got := percent(c.i, c.n); got != c.want {
			t.Errorf("percent(%d, %d) = %s, want %s", c.i, c.n, got, c.want)
		}
	}
}

// A small run end to end: what a pass, a failure and a case with no answer
// leave behind. The byte-level check against CQT itself is
// TestTheRecordsAreCQTs, which needs a real run; this one keeps the shape from
// regressing without one.
func TestTheRecordsOfASmallRun(t *testing.T) {
	ctp := t.TempDir()
	corpus := filepath.Join(t.TempDir(), "cubrid-testcases", "sql") + "/"
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	pass := corpus + "_01_a/cases/pass.sql"
	fail := corpus + "_01_a/cases/fail.sql"
	lone := corpus + "_02_b/_01_c/cases/lone.sql"
	write(pass, "select 1;\n")
	write(fail, "select 2;\n")
	write(lone, "select 3;\n")
	write(corpus+"_01_a/answers/pass.answer", "1\n")
	write(corpus+"_01_a/answers/fail.answer", "3\n")

	var out, log bytes.Buffer
	clock := time.Date(2026, 9, 11, 14, 8, 17, 0, time.Local)
	r, err := OpenSQL(SQLRun{
		CTPHome: ctp, Type: "sql", Alias: "sql", Bits: "64", Build: "11.5.0.2562-1642235",
		Scenario: corpus, Mode: "release", XMLSummary: true,
		Cases:   []string{pass, fail, lone},
		Answers: map[string]string{pass: AnswerPath(pass), fail: AnswerPath(fail)},
		Serial:  true, Out: &out, Log: &log,
		Failure: func(string) string { return "[Query]\nselect 2;\n[Diff]\n-3\n+2" },
		Now:     func() time.Time { return clock },
		Rand:    func(int) int { return 7 },
	})
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(ctp, "sql", "result", "y2026", "m9", "schedule_linux_sql_64bit_111408177_11.5.0.2562-1642235")
	if r.Root() != root {
		t.Fatalf("result directory %s, want %s", r.Root(), root)
	}
	r.Begin([]string{"", ""})
	for i, c := range []SQLCase{
		{File: pass, Ran: true, OK: true, Ms: 12, Rendered: []byte("====\n1\n")},
		{File: fail, Ran: true, OK: false, Ms: 1500, Rendered: []byte("====\n2\n")},
		{File: lone},
	} {
		c.Index, c.Total, c.Start = i+1, 3, clock
		r.Starting(c)
		if err := r.Case(c); err != nil {
			t.Fatal(err)
		}
	}
	counts, err := r.End()
	if err != nil {
		t.Fatal(err)
	}
	if counts != (SQLCounts{Total: 3, Success: 1, Fail: 1, TotalTime: 1512}) {
		t.Errorf("counts %+v", counts)
	}

	wantOut := "Result Root Dir:" + root + "\n\n\n---------------execute begin--------------------\n" +
		"[14:08:17] Testing " + pass + " (1/3 33.33%) [OK]\n" +
		"[14:08:17] Testing " + fail + " (2/3 66.67%) [NOK]\n" +
		// No answer: the prefix, and not even a newline -- CQT's.
		"[14:08:17] Testing " + lone + " (3/3 100.00%)" +
		"---------------execute end  --------------------\n"
	if out.String() != wantOut {
		t.Errorf("stdout:\n%s\nwant:\n%s", out.String(), wantOut)
	}
	if log.String() != wantOut {
		t.Error("the log should hold what CQT printed, as run.sh's tee put it there")
	}

	read := func(path string) string {
		t.Helper()
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	if got := read(corpus + "_01_a/cases/pass.result"); got != "====\n1\n" {
		t.Errorf("the .result beside the case is %q", got)
	}
	if _, err := os.Stat(corpus + "_02_b/_01_c/cases/lone.result"); err == nil {
		t.Error("a case that did not run left a .result")
	}
	bottom := filepath.Join(root, "sql", "_01_a")
	for name, want := range map[string]string{"fail.result": "====\n2\n", "fail.sql": "select 2;\n", "fail.answer": "3\n"} {
		if got := read(filepath.Join(bottom, name)); got != want {
			t.Errorf("failure copy %s is %q, want %q", name, got, want)
		}
	}
	if _, err := os.Stat(filepath.Join(bottom, "pass.sql")); err == nil {
		t.Error("a passing case was copied into the result tree")
	}
	if st, _ := os.Stat(filepath.Join(bottom, "fail.result")); st.Mode().Perm()&0o100 == 0 {
		t.Error("CQT's files carry the owner's execute bit")
	}

	if got, want := read(filepath.Join(bottom, "summary_info")),
		"total:2\nsuccess:1\nfail:1\ntotalTime:1512ms\nSiteRunTimes:0times\n"+
			pass+":ok    12ms\n"+fail+":nok    1500ms\n"; got != want {
		t.Errorf("bottom summary_info:\n%s\nwant:\n%s", got, want)
	}
	// The one that did not run is counted and listed, with no verdict.
	if got := read(filepath.Join(root, "sql", "_02_b", "_01_c", "summary_info")); !strings.Contains(got, "total:1\nsuccess:0\nfail:0\n") ||
		!strings.Contains(got, lone+":    0ms\n") {
		t.Errorf("a case with no answer:\n%s", got)
	}
	if got := read(filepath.Join(root, "summary_info")); !strings.HasPrefix(got, "total:3\nsuccess:1\nfail:1\ntotalTime:1512ms\nSiteRunTimes:1times\nsql/:3    1    1\n") {
		t.Errorf("root summary_info:\n%s", got)
	}
	if got := read(filepath.Join(root, "summary.xml")); got != "<results>\n"+
		"  <scenario>\n     <case>sql/_01_a/cases/pass.sql</case>\n     <answer>sql/_01_a/answers/pass.answer</answer>\n     <elapsetime>12</elapsetime>\n     <result>success</result>\n  </scenario>\n"+
		"  <scenario>\n     <case>sql/_01_a/cases/fail.sql</case>\n     <answer>sql/_01_a/answers/fail.answer</answer>\n     <elapsetime>1500</elapsetime>\n     <result>fail</result>\n  </scenario>\n"+
		"</results>\n" {
		t.Errorf("summary.xml:\n%s", got)
	}
	info := read(filepath.Join(root, "main.info"))
	for _, want := range []string{"build:11.5.0.2562-1642235\nversion:64bit\nos:linux\ncategory:sql\nelapse_time:1512\n",
		"success:1\nfail:1\ntotal:3\nexecute_case:2\ntotalTime:1512\n", "end_time:2026-09-11 14:08:17\nresult_path:" + root + "\n"} {
		if !strings.Contains(info, want) {
			t.Errorf("main.info lacks %q:\n%s", want, info)
		}
	}
	junit := read(filepath.Join(root, "linux_sql_64bit.xml"))
	for _, want := range []string{
		`<testsuite name="linux_sql_64bit_release" tests="2" failures="1">`,
		`<testcase classname="linux_sql_64bit_release" name="sql/_01_a/cases/pass.sql" file="cubrid-testcases/sql/_01_a/cases/pass.sql" time="0.012"></testcase>`,
		"time=\"1.500\">\n      <failure message=\"unexpected result\"><![CDATA[[Query]\nselect 2;\n[Diff]\n-3\n+2]]></failure>\n    </testcase>",
	} {
		if !strings.Contains(junit, want) {
			t.Errorf("the JUnit report lacks %q:\n%s", want, junit)
		}
	}
	if si := read(filepath.Join(root, "summary.info")); !strings.HasPrefix(si, "<summary>\n  <childList>\n    <summary>\n      <parent reference=\"../../..\"/>") ||
		strings.HasSuffix(si, "\n") || !strings.Contains(si, "<notRunList>") {
		t.Errorf("summary.info is not XStream's shape:\n%s", si)
	}

	// What the status page shows when a case is clicked.
	if d := r.Describe(fail); !strings.Contains(d, "NOK in 1500 ms") || !strings.Contains(d, "first difference") ||
		!strings.Contains(d, "answer, from line 1:\n> 3") || !strings.Contains(d, "result, from line 1:\n> ====") {
		t.Errorf("a failed case's description should show where answer and result part:\n%s", d)
	}
	if d := r.Describe(pass); !strings.Contains(d, "OK in 12 ms") {
		t.Errorf("a passed case's description: %q", d)
	}
	if d := r.Describe(lone); !strings.Contains(d, "not run") {
		t.Errorf("a case with no answer should say it did not run: %q", d)
	}
	if d := r.Describe(corpus + "_01_a/cases/nope.sql"); d != "" {
		t.Errorf("a case the run does not have should describe as empty, got %q", d)
	}

	if err := r.MainInfoTail("CUBRID 11.5.0 (x)", "qa", "127.0.1.1"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(read(filepath.Join(root, "main.info")), "cubrid_rel:CUBRID 11.5.0 (x)\nuser:qa\nmachine:127.0.1.1\n") {
		t.Error("run.sh's three lines were not appended")
	}
}
