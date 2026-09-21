package casecheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// library writes a helper directory shaped like CTP's: write_nok is the root,
// compare_result_between_files reaches it, and a wait prints its own NOK line
// without calling anything -- which ha_common.sh really does.
func library(t *testing.T) *Helpers {
	t.Helper()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "init.sh"), `
function write_ok
{
  echo "OK"
}

function write_nok
{
  echo "----------------- $case_no : NOK"
}

function compare_result_between_files
{
  diff $1 $2
  if [ $? -ne 0 ]; then
     write_nok
  else
     write_ok
  fi
}

format_csql_output()
{
  sed -i 's/x//' $1
}
`)
	write(t, filepath.Join(dir, "ha_common.sh"), `
function wait_for_active
{
   if [ $count -eq 0 ]; then
      echo "NOK: db server is not active after long time"
   fi
}
`)
	h, err := ReadHelpers(dir)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func rules(fs []Finding) string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Rule)
	}
	return strings.Join(out, ",")
}

// A helper is a failure path when it reaches write_nok, and the reach is
// transitive: 169 HA cases get their verdict only through
// compare_result_between_files, and a check that did not follow the call would
// report every one of them as unable to fail.
func TestAHelperThatReachesWriteNokIsAFailurePath(t *testing.T) {
	h := library(t)
	for _, want := range []string{"write_nok", "compare_result_between_files", "wait_for_active"} {
		if !h.Failing[want] {
			t.Errorf("%s is not counted as a failure path", want)
		}
	}
	for _, not := range []string{"format_csql_output"} {
		if h.Failing[not] {
			t.Errorf("%s reaches no failure and is counted as one", not)
		}
	}
	if !h.All["format_csql_output"] {
		t.Error("the name() form of a declaration was not read")
	}
}

// The one this package exists for. `wirte_nok` in an else branch is a command
// that does not exist, so the branch reporting a failure reports nothing.
func TestAMisspeltVerdictCallIsReported(t *testing.T) {
	h := library(t)
	got := h.Check("x.sh", `
if [ -s a.log ]; then
	write_ok
else
	wirte_nok
fi
`)
	// Two findings, and both are true: the typo, and the consequence -- this
	// case's only route to NOK was that line, so it cannot fail at all. In the
	// real corpus the two come apart, because a case usually has other checks
	// and only one dead branch.
	if rules(got) != RuleMisspeltVerdict+","+RuleCannotFail {
		t.Fatalf("got %v", got)
	}
	if !strings.Contains(got[0].Detail, "write_nok") {
		t.Errorf("the finding does not name what was meant: %s", got[0].Detail)
	}
}

// And when the case has another check, the typo stands alone: the branch is
// dead but the case can still report a failure. Every one of the seven found in
// the shell corpus is this shape.
func TestADeadBranchInACaseThatCanStillFail(t *testing.T) {
	h := library(t)
	got := h.Check("x.sh", `
compare_result_between_files a.result a.answer
if [ -s b.log ]; then
	write_ok
else
	wirte_nok
fi
`)
	if rules(got) != RuleMisspeltVerdict {
		t.Fatalf("got %v", got)
	}
}

// An assignment is not a call. `db_stats=` was reported before command position
// was taken into account, and db_status is a real helper one edit away.
func TestAnAssignmentIsNotACall(t *testing.T) {
	h := library(t)
	h.All["db_status"] = true
	h.Failing["db_status"] = true
	got := h.Check("x.sh", "db_stats=`cub_commdb -P | grep bbb | wc -l`\nwrite_nok\n")
	if len(got) != 0 {
		t.Fatalf("an assignment was read as a call: %v", got)
	}
}

// Nor is a file name. `expect exec_csql.exp` was reported for the same reason,
// against the helper exec_sql.
func TestAFileNameIsNotACall(t *testing.T) {
	h := library(t)
	h.All["exec_sql"] = true
	h.Failing["exec_sql"] = true
	got := h.Check("x.sh", "expect exec_csql.exp $db_name > result.log\nwrite_nok\n")
	if len(got) != 0 {
		t.Fatalf("a file name was read as a call: %v", got)
	}
}

// A comparison of something against itself cannot differ, so the check it looks
// like is not being made. Three of these are in the shell corpus, each beside
// sibling lines that compare a result against an answer.
func TestAComparisonWithItselfIsReported(t *testing.T) {
	h := library(t)
	got := h.Check("x.sh", "compare_result_between_files plan.result plan.result\n")
	if rules(got) != RuleSelfComparison {
		t.Fatalf("got %v", got)
	}
	ok := h.Check("x.sh", "compare_result_between_files plan.result plan.expect\n")
	if len(ok) != 0 {
		t.Fatalf("a real comparison was reported: %v", ok)
	}
}

// A case with no route to NOK reports OK or reports nothing, and either way the
// slot it occupies is believed to be covered.
func TestACaseWithNoFailurePathIsReported(t *testing.T) {
	h := library(t)
	got := h.Check("x.sh", "csql -u dba -c 'select 1' demodb\nwrite_ok\n")
	if rules(got) != RuleCannotFail {
		t.Fatalf("got %v", got)
	}
}

// Three ways to have a failure path, and all three count: the helper, a helper
// that reaches it, and the case printing its own NOK line -- which the runner's
// substring rule catches just the same.
func TestEveryRouteToNokCounts(t *testing.T) {
	h := library(t)
	for name, body := range map[string]string{
		"direct":     "write_nok\n",
		"transitive": "compare_result_between_files a b\n",
		"printed":    `echo "NOK: it did not happen" >> $result_file` + "\n",
	} {
		if got := h.Check("x.sh", body); len(got) != 0 {
			t.Errorf("%s: reported a case that can fail: %v", name, got)
		}
	}
}

// A comment is not code. A verdict call inside one is not a failure path, and
// counting it would hide exactly the case this package is looking for.
func TestACommentedOutVerdictIsNotAFailurePath(t *testing.T) {
	h := library(t)
	got := h.Check("x.sh", "csql -u dba -c 'select 1' demodb\n# write_nok\nwrite_ok\n")
	if rules(got) != RuleCannotFail {
		t.Fatalf("got %v", got)
	}
}

// A case may define its own verdict wrapper, and then the wrapper is the failure
// path and calling it is not a typo.
func TestACaseCanDefineItsOwnVerdict(t *testing.T) {
	h := library(t)
	got := h.Check("x.sh", "my_fail() {\n  write_nok\n}\nmy_fail\n")
	if len(got) != 0 {
		t.Fatalf("a case's own function was reported: %v", got)
	}
}

// Walk applies CTP's discovery rule, so the helper scripts that sit beside a
// case are not themselves checked -- they cannot fail, truthfully and uselessly.
func TestWalkChecksCasesAndNotTheHelpersBesideThem(t *testing.T) {
	h := library(t)
	root := t.TempDir()
	caseDir := filepath.Join(root, "_01_utility", "mycase", "cases")
	if err := os.MkdirAll(caseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(caseDir, "mycase.sh"), "wirte_nok\n")
	write(t, filepath.Join(caseDir, "helper.sh"), "echo just a helper\n")

	rep, err := Walk(root, h)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Cases != 1 {
		t.Fatalf("read %d cases, want 1", rep.Cases)
	}
	if rep.Count(RuleMisspeltVerdict) != 1 {
		t.Errorf("findings: %v", rep.Findings)
	}
	for _, f := range rep.Findings {
		if strings.Contains(f.Case, "helper.sh") {
			t.Errorf("a helper beside the case was checked: %v", f)
		}
	}
}

func TestDamerauCountsATranspositionAsOne(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"write_nok", "write_nok", 0},
		{"wirte_nok", "write_nok", 1}, // the typo people actually make
		{"write_no", "write_nok", 1},
		{"write_ok", "write_nok", 1},
		{"compare_result_between_file", "compare_result_between_files", 1},
		{"csql", "write_nok", 3},
	} {
		if got := damerau(c.a, c.b); got != c.want {
			t.Errorf("damerau(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
