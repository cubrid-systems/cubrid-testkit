package hareplsuite

import (
	"strings"
	"testing"
)

// The line a statement starts on is how its answer is found again in a
// batch's output, so a multi-line statement has to advance the count by its
// own height. Getting this wrong attributes one statement's answer to
// another, which is worse than being slow.
func TestBatchRecordsWhereEachStatementBegins(t *testing.T) {
	b := newBatch([]string{
		"select 1",
		"create table t(\n  i int,\n  v varchar(10)\n)",
		"select 2",
	})
	want := []int{1, 2, 6}
	if len(b.line) != len(want) {
		t.Fatalf("want %d lines, got %v", len(want), b.line)
	}
	for i := range want {
		if b.line[i] != want[i] {
			t.Fatalf("want %v, got %v\nscript:\n%s", want, b.line, b.script)
		}
	}
	// The script csql is given must actually put them there.
	lines := strings.Split(b.script, "\n")
	if !strings.HasPrefix(lines[b.line[2]-1], "select 2") {
		t.Errorf("line %d is %q, not the third statement", b.line[2], lines[b.line[2]-1])
	}
}

func TestSplitResultsAttributesEachAnswerToItsLine(t *testing.T) {
	out := `
=== <Result of SELECT Command in Line 1> ===
            1
1 row selected.
=== <Result of SELECT Command in Line 3> ===
            2
1 row selected.
`
	got := splitResults(out)
	if len(got) != 2 {
		t.Fatalf("want two blocks, got %d: %v", len(got), got)
	}
	if !strings.Contains(got[1], "1") || strings.Contains(got[1], "2") {
		t.Errorf("line 1's block is wrong: %q", got[1])
	}
	if !strings.Contains(got[3], "2") {
		t.Errorf("line 3's block is wrong: %q", got[3])
	}
	if _, ok := got[2]; ok {
		t.Error("a statement that produced no result must have no block")
	}
}

// A statement the engine refuses produces no result block, and csql carries
// on. That is what makes a batch behave as a sequence of calls did.
func TestAStatementWithNoResultHasNoBlock(t *testing.T) {
	b := newBatch([]string{"select 1", "select * from nope", "select 2"})
	blocks := splitResults("=== <Result of SELECT Command in Line 1> ===\n  1\n" +
		"=== <Result of SELECT Command in Line 3> ===\n  2\n")
	var got []string
	for _, ln := range b.line {
		got = append(got, blocks[ln])
	}
	if strings.TrimSpace(got[1]) != "" {
		t.Errorf("the refused statement was given an answer: %q", got[1])
	}
	if !strings.Contains(got[0], "1") || !strings.Contains(got[2], "2") {
		t.Errorf("the other two lost theirs: %v", got)
	}
}

// The grouping the oracle needs is the grouping that is fast: everything up to
// a read goes in one call, and the wait sits on the boundary.
func TestSegmentsAlternateBetweenWritesAndReads(t *testing.T) {
	segs := segments([]string{
		"--+ holdcas on",
		"create table t(i int primary key)",
		"insert into t values(1)",
		"select * from t",
		"insert into t values(2)",
		"select * from t",
		"--+ holdcas off",
	})
	if len(segs) != 4 {
		t.Fatalf("want 4 segments, got %d: %+v", len(segs), segs)
	}
	for i, want := range []bool{false, true, false, true} {
		if segs[i].read != want {
			t.Errorf("segment %d: read=%v, want %v", i, segs[i].read, want)
		}
	}
	if len(segs[0].stmts) != 2 {
		t.Errorf("the two writes did not batch together: %v", segs[0].stmts)
	}
	// A csql directive reaches no node's data and belongs in no segment.
	for _, s := range segs {
		for _, st := range s.stmts {
			if strings.HasPrefix(st, "--+") || strings.HasPrefix(st, "autocommit") {
				t.Errorf("a directive was sent to a node: %q", st)
			}
		}
	}
}

// A prelude shifts every statement's line, and the line is how its answer is
// found again. Off by one here attributes the prelude's own output to the
// case's first read.
func TestAPreludeShiftsTheLinesAndIsNotIndexed(t *testing.T) {
	b := newBatchWith(
		[]string{"call login ('u1') on class db_user"},
		[]string{"select 1", "select 2"},
	)
	if len(b.line) != 2 {
		t.Fatalf("the prelude must not be indexed: %v", b.line)
	}
	if b.line[0] != 2 || b.line[1] != 3 {
		t.Fatalf("want lines [2 3], got %v\nscript:\n%s", b.line, b.script)
	}
	lines := strings.Split(b.script, "\n")
	if !strings.HasPrefix(lines[b.line[0]-1], "select 1") {
		t.Errorf("line %d is %q", b.line[0], lines[b.line[0]-1])
	}
}

// Session state dies with the csql process, so what has to be replayed is
// what the session held -- and only that. Replaying a write would run it
// twice.
func TestIsSessionStatementNamesOnlyWhatTheSessionHolds(t *testing.T) {
	for _, s := range []string{
		"call login ('u1') on class db_user",
		"CALL LOGIN ('dba') ON CLASS db_user",
		"set system parameters 'create_table_reuseoid=no'",
	} {
		if !IsSessionStatement(s) {
			t.Errorf("not recognised as session state: %q", s)
		}
	}
	for _, s := range []string{
		"insert into t values(1)",
		"select * from t",
		"create table t(i int)",
		"call change_trigger_owner ('t', 'u1') on class db_root",
	} {
		if IsSessionStatement(s) {
			t.Errorf("a statement that changes the database was taken for session state: %q", s)
		}
	}
}
