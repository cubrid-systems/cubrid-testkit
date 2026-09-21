package hareplsuite

import (
	"strings"
	"testing"
)

func TestConversionAddsAKeyNoDataCanViolate(t *testing.T) {
	c := NewConversion()
	got, changed := c.Apply("create table t(i int, v varchar(10))")
	if !changed {
		t.Fatal("a table with no primary key was left without one")
	}
	want := "create table t(i int, v varchar(10), " + KeyColumn + " INT AUTO_INCREMENT PRIMARY KEY)"
	if got != want {
		t.Errorf("\n got: %q\nwant: %q", got, want)
	}
}

// The column list is read off the CREATE, so a comma inside a type is not the
// end of a column and a table-level constraint is not one at all.
func TestConversionReadsTheColumnNames(t *testing.T) {
	cases := []struct {
		name, create string
		want         []string
	}{
		{"simple", "create table t(i int, v varchar(10))", []string{"i", "v"}},
		{"comma in a type", "create table t(n numeric(10,2), v varchar(10))", []string{"n", "v"}},
		{"a table-level constraint", "create table t(a int, b int, FOREIGN KEY (a) REFERENCES u(x))", []string{"a", "b"}},
		{"over several lines", "create class tb(\n col1 char(20),\n col2 int\n)", []string{"col1", "col2"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := NewConversion()
			if _, ok := c.Apply(tc.create); !ok {
				t.Fatalf("not converted: %q", tc.create)
			}
			got := c.cols["t"]
			if got == nil {
				got = c.cols["tb"]
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("want %v, got %v", tc.want, got)
			}
		})
	}
}

// The extra column breaks a positional INSERT, which is most of the corpus.
// That objection is answered rather than accepted: the names are written in.
func TestConversionNamesTheColumnsOfAPositionalInsert(t *testing.T) {
	c := NewConversion()
	c.Apply("create table t(c1 int, c2 varchar(10))")

	got, changed := c.Apply("insert into t values(1, 'a')")
	if !changed {
		t.Fatal("a positional insert was left to break on the extra column")
	}
	if got != "insert into t (c1, c2) values(1, 'a')" {
		t.Errorf("got %q", got)
	}

	if got, _ := c.Apply("insert into t select * from u"); got != "insert into t (c1, c2) select * from u" {
		t.Errorf("insert-select: got %q", got)
	}
}

// The corpus nests them, and both tables may have been converted.
func TestConversionReachesANestedInsert(t *testing.T) {
	c := NewConversion()
	c.Apply("create class person2(name varchar(20), age integer)")
	c.Apply("create class employees(empno integer, attr person2)")

	got, changed := c.Apply("insert into employees values(1001, (insert into person2 values ('xxx', 21)))")
	if !changed {
		t.Fatal("not converted")
	}
	if !strings.Contains(got, "insert into employees (empno, attr) values") {
		t.Errorf("the outer insert was not named: %q", got)
	}
	if !strings.Contains(got, "insert into person2 (name, age) values") {
		t.Errorf("the inner insert was not named: %q", got)
	}
}

func TestConversionLeavesTheseAlone(t *testing.T) {
	c := NewConversion()
	c.Apply("create table t(c1 int, c2 int)")
	for _, in := range []string{
		"create table u(i int primary key, v varchar(10))",
		"create table v2 as select * from t",
		"insert into t(c1) values(1)", // already names its columns
		"insert into t set c1 = 1",    // names them another way
		"insert into other values(1)", // a table this conversion did not touch
		"select * from t",
		"create view w as select * from t",
	} {
		got, changed := c.Apply(in)
		if changed {
			t.Errorf("changed what it should not have: %q -> %q", in, got)
		}
	}
}

// A subclass inherits the key the conversion gave its parent, so it must not
// be given one of its own -- and its positional inserts carry the parent's
// columns first, which only the parent's entry can say.
func TestConversionHandlesASubclass(t *testing.T) {
	c := NewConversion()
	c.Apply("create class t1 (name varchar(20), age integer)")

	got, changed := c.Apply("create class sub_t1 as subclass of t1(gender char(1))")
	if changed {
		t.Errorf("a subclass must not be given a second key: %q", got)
	}

	ins, changed := c.Apply("insert into sub_t1 values('Sun', 26, 'f')")
	if !changed {
		t.Fatal("the subclass's positional insert was left to break on the inherited key")
	}
	if ins != "insert into sub_t1 (name, age, gender) values('Sun', 26, 'f')" {
		t.Errorf("got %q", ins)
	}
}

func TestSubclassOfNamesTheParent(t *testing.T) {
	if got := subclassOf("create class s as subclass of p(g char(1))"); got != "p" {
		t.Errorf("got %q", got)
	}
	if got := subclassOf("create table t(i int)"); got != "" {
		t.Errorf("an ordinary table has no parent, got %q", got)
	}
}
