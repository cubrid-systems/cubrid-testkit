package haapply

import (
	"os/exec"
	"strings"
	"testing"
)

func TestShellQuoteSurvivesTheQuotesInTheSQL(t *testing.T) {
	for _, s := range []string{
		"INSERT INTO t VALUES ('Key5');",
		"INSERT INTO t VALUES (TIMESTAMPTZ'2020-01-01 00:00:05 +00:00');",
		"it's $HOME and `date` and \"x\"",
	} {
		out, err := exec.Command("sh", "-c", "printf %s "+shellQuote(s)).Output()
		if err != nil {
			t.Fatal(err)
		}
		if string(out) != s {
			t.Errorf("sh read %q back as %q", s, out)
		}
	}
}

func TestFirstNumberReadsAValueOutOfCsqlOutput(t *testing.T) {
	out := "\n=== <Result of SELECT Command in Line 1> ===\n\n    count(*)\n=============\n           7\n\n1 row selected.\n"
	if got := firstNumber(out); got != "7" {
		t.Errorf("firstNumber = %q, want 7", got)
	}
	if got := firstNumber("count(*)\n\n"); got != "" {
		t.Errorf("no value should read as empty, got %q", got)
	}
	if got := firstNumber("   -3  \n   9\n"); got != "-3" {
		t.Errorf("firstNumber = %q, want -3", got)
	}
}

// SHOW WRITESET STATUS in csql's line mode, as the prototype read it.
func TestParseWritesetReadsTheFourCounters(t *testing.T) {
	out := `
<00001> Conflict_write_count    : 12
        Conflict_ref_count      : 3
        Commit_fallback_count   : 1
        History_clear_count     : 2
        Fallback_statement_count: 1
`
	w := parseWriteset(out)
	if w != (Writeset{Write: 12, Ref: 3, Fallback: 1, Clear: 2}) {
		t.Errorf("parseWriteset = %+v", w)
	}
}

func TestNormaliseDropsWhatDiffersBetweenTwoRunsOfOneQuery(t *testing.T) {
	a := "=== <Result> ===\n  1  a\n2 rows selected. (0.001 sec) Committed. (0.0 sec)\nTime: 10/30/26 12:00:00.1 - NOTIFICATION x\nProgram 'csql' (pid 12) connected\n"
	b := "=== <Result> ===\n  1  a\n2 rows selected. (0.007 sec) Committed. (0.0 sec)\n"
	if normalise(a) != normalise(b) {
		t.Errorf("normalise left a difference:\n%q\n%q", normalise(a), normalise(b))
	}
}

// A login profile may print before the script does; the tags are found anyway.
func TestParseTagsSkipsWhatIsNotATag(t *testing.T) {
	out := "Welcome to the node\nlast login: today\nt1=ok\nt2=refused\nt2err=ERROR: Operation would have caused one or more unique constraint violations.\n"
	tags := parseTags(out)
	if tags["t1"] != "ok" || tags["t2"] != "refused" || !strings.HasPrefix(tags["t2err"], "ERROR:") {
		t.Errorf("tags = %v", tags)
	}
	if _, ok := tags["last login: today"]; ok {
		t.Errorf("a line with a space in its key was taken as a tag: %v", tags)
	}
}
