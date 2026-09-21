package hareplsuite

import (
	"strings"
	"testing"
)

func TestStatementsSplitsOnTerminatorsAndNotInsideLiterals(t *testing.T) {
	src := `-- a comment with a ; in it
create table t(i int primary key, v varchar(10));
insert into t values(1, 'a;b');
/* a block
   comment; with one too */
select * from t;`
	got := Statements(src)
	if len(got) != 3 {
		t.Fatalf("want 3 statements, got %d: %q", len(got), got)
	}
	if !strings.Contains(got[1], "'a;b'") {
		t.Errorf("a semicolon inside a literal is not a terminator: %q", got[1])
	}
	for _, s := range got {
		if strings.Contains(s, "a comment with") || strings.Contains(s, "block") {
			t.Errorf("a comment reached a statement: %q", s)
		}
	}
}

// The splitter refuses a block body rather than cutting it into fragments. It
// is the difference between a case this suite skips and names, and a case it
// runs as nonsense and reports a verdict about.
func TestSplittableRefusesABodyWhoseSemicolonsAreNotTerminators(t *testing.T) {
	for _, src := range []string{
		"CREATE PROCEDURE p AS BEGIN null; END;",
		"create or replace function f() return int as begin return 1; end;",
		"CREATE TRIGGER tr BEFORE INSERT ON t EXECUTE print 'x';",
	} {
		if Splittable(src) {
			t.Errorf("split was allowed on a block body: %q", src)
		}
	}
	if !Splittable("create table t(i int); insert into t values(1);") {
		t.Error("an ordinary case was refused")
	}
}

func TestReadAndWriteAreDecidedTheSafeWayRound(t *testing.T) {
	reads := []string{"select * from t", "SELECT 1", "show tables", "values (1)"}
	for _, s := range reads {
		if !IsRead(s) {
			t.Errorf("not recognised as a read: %q", s)
		}
		if IsWrite(s) {
			t.Errorf("a read must not count as a write: %q", s)
		}
	}
	// A write is anything that is not a read and not csql's own vocabulary.
	// Over-claiming costs a wait; under-claiming compares a slave that was
	// never given the chance to catch up.
	writes := []string{"insert into t values(1)", "create table t(i int)", "drop table t", "call p()"}
	for _, s := range writes {
		if !IsWrite(s) {
			t.Errorf("not recognised as a write: %q", s)
		}
	}
	for _, s := range []string{"--+ holdcas on", "autocommit off", "commit", "rollback work"} {
		if IsWrite(s) {
			t.Errorf("csql's own vocabulary reaches no node's data: %q", s)
		}
	}
}

// The keyless-table check is name matching, and the direction it errs in is
// the whole of its correctness: a missed table turns CUBRID's HA design into
// a reported defect of this build.
func TestMentionsAnyMatchesWholeNamesOnly(t *testing.T) {
	bare := []string{"track", "t1"}
	for _, s := range []string{
		"select * from track",
		"select * from db_index where class_name in ('track')",
		"SELECT * FROM TRACK ORDER BY 1",
		"select * from t1 order by 1",
	} {
		if !mentionsAny(s, bare) {
			t.Errorf("a keyless table was not seen in: %q", s)
		}
	}
	for _, s := range []string{
		"select * from tracklist",
		"select * from my_track",
		"select * from t12",
		"select * from album",
	} {
		if mentionsAny(s, bare) {
			t.Errorf("a different table was taken for a keyless one: %q", s)
		}
	}
	if mentionsAny("select 1", nil) {
		t.Error("nothing is keyless, so nothing mentions one")
	}
}

func TestNormaliseDropsWhatTwoNodesMaySayDifferently(t *testing.T) {
	master := `
Time: 09/21/26 10:36:58.062 - NOTIFICATION *** file boot_cl.c, line 875  CODE = -971
Program 'csql' (pid 11381) connected to database server 'pmha' on the host 'localhost' (port 31523).

=== <Result of SELECT Command in Line 1> ===

            i  v
===================================
            1  'a'

1 row selected. (0.001000 sec) Committed. (0.000000 sec)
`
	slave := strings.NewReplacer(
		"10:36:58.062", "10:36:58.551",
		"pid 11381", "pid 13117",
		"(0.001000 sec)", "(0.000000 sec)",
	).Replace(master)

	if Normalise(master) != Normalise(slave) {
		t.Errorf("a timestamp, a pid and a timing made two identical answers differ:\n%s\n---\n%s",
			Normalise(master), Normalise(slave))
	}
	if strings.Contains(Normalise(master), "1  'a'") == false {
		t.Error("the row itself was normalised away, which would make every comparison pass")
	}
}

// A table may be called ERROR. Dropping every line with that word in it was
// how a view over one came to look like a replication finding.
func TestParseTableListKeepsATableCalledERROR(t *testing.T) {
	out := `
Time: 09/21/26 10:36:58.062 - NOTIFICATION *** file boot_cl.c, line 875
Program 'csql' (pid 11381) connected to database server 'pmha' on the host 'localhost' (port 31523).
  ERROR
  NESTED
  album
1 row selected. (0.001000 sec) Committed. (0.000000 sec)
`
	got := parseTableList(out)
	want := []string{"ERROR", "NESTED", "album"}
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("want %v, got %v", want, got)
			break
		}
	}
	if len(parseTableList("ERROR: before ';' Syntax error\n")) != 0 {
		t.Error("a csql diagnostic was taken for a table name")
	}
}
