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

// A block body is kept whole rather than cut into fragments, and a construct
// that merely looks like one is not. The first version refused every case
// whose text contained "create trigger", which skipped fifteen of 131 cases
// that split perfectly well -- a trigger without a body is one statement.
func TestStatementsKeepsABlockBodyWholeAndSplitsWhatIsNotOne(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want int
	}{
		{"a procedure body", "CREATE PROCEDURE p AS BEGIN null; END;", 1},
		{"a function body", "create or replace function f() return int as begin return 1; end;", 1},
		{"a trigger with no body", "CREATE TRIGGER tr BEFORE INSERT ON t EXECUTE print 'x';", 1},
		{"a trigger, then a statement",
			"create trigger t1 after insert on a execute insert into b values (obj.c1);\nselect 1;", 2},
		// END IF closes an IF, not the block. Counting it would end the
		// procedure early and run its tail as a statement of its own.
		{"END IF inside a body", "create procedure p as begin if x then null; end if; end;\nselect 2;", 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Statements(c.src)
			if len(got) != c.want {
				t.Fatalf("want %d statement(s), got %d: %q", c.want, len(got), got)
			}
			if !Splittable(c.src) {
				t.Errorf("refused a case it read correctly: %q", c.src)
			}
		})
	}
}

// What is left to refuse is a block whose END never came: the last statement
// would be the rest of the file, and a verdict about that is a verdict about
// nothing.
func TestSplittableRefusesAnUnclosedBlock(t *testing.T) {
	if Splittable("create procedure p as begin null;") {
		t.Error("a block with no END was accepted")
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
//
// The one place that direction was wrong is a name in quotes, and the case
// below used to be listed here as a table that must be seen. It is not a
// table; it is a value. A read of `db_index` or `db_class` filtered by the
// name of a keyless table reads the catalog, which replicates as DDL, and the
// rows that do not replicate are never touched. Skipping it cost a finding:
// `_02_class/_003_auto_increment/cubridsus-965.sql` changes a class's owner by
// a method call that does not replicate, its first read sees exactly that, and
// the case reported `same` because that read was skipped for the quoted name
// -- while the same run found the divergence stranded on the slave a moment
// later (evidence/ha/class-owner-change-not-replicated.md).
func TestMentionsAnyMatchesWholeNamesOnly(t *testing.T) {
	bare := []string{"track", "t1"}
	for _, s := range []string{
		"select * from track",
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
		// A value, not a table: the catalog is what is being read.
		"select * from db_index where class_name in ('track')",
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

// The catalog qualifies a view with its owner and a case does not, so a check
// that matches only the catalog's spelling misses the view entirely.
func TestBothSpellingsCoversTheQualifiedAndTheBareName(t *testing.T) {
	got := bothSpellings("u1.v1")
	if len(got) != 2 || got[0] != "u1.v1" || got[1] != "v1" {
		t.Errorf("want [u1.v1 v1], got %v", got)
	}
	if got := bothSpellings("v1"); len(got) != 1 || got[0] != "v1" {
		t.Errorf("an unqualified name has one spelling, got %v", got)
	}
	if got := bothSpellings("u1."); len(got) != 1 {
		t.Errorf("a trailing dot is not a second name, got %v", got)
	}
}

// A serial's next value is a write wearing a SELECT. Comparing one across the
// pair asks a standby to take a write, which it will not, so the master's
// answer and an empty one are reported as a difference that is not one.
func TestASelectThatMovesASerialIsNotARead(t *testing.T) {
	for _, stmt := range []string{
		"SELECT serial_next_value(ser1, 1) FROM db_root",
		"select se1.next_value from db_root",
		"SELECT cnf_col1.current_value,cnf_col1.next_value  from cnf_1",
	} {
		if IsRead(stmt) {
			t.Errorf("compared across the pair: %q", stmt)
		}
		if !IsWrite(stmt) {
			t.Errorf("not waited for: %q", stmt)
		}
	}
	// The value replication carried is still worth comparing.
	if !IsRead("select se1.current_value from db_root") {
		t.Error("current_value on its own should still be compared")
	}
}

// A keyless table's name inside a string literal is a value. Skipping a
// catalog read for it throws away the read that would have caught the
// divergence -- measured on cubridsus-965, which reported `same` over one.
func TestAQuotedNameIsNotATableReference(t *testing.T) {
	keyless := []string{"xxx"}
	skipped := "select class_name, owner_name from db_class where class_name='xxx'"
	if mentionsAny(skipped, keyless) {
		t.Errorf("a catalog read was skipped for a quoted value: %q", skipped)
	}
	real := "select * from xxx"
	if !mentionsAny(real, keyless) {
		t.Errorf("a read of the table itself was not skipped: %q", real)
	}
	// The name has to be the whole word, quoted or not.
	if mentionsAny("select * from xxx_ai_a", keyless) {
		t.Error("matched a longer name")
	}
}
