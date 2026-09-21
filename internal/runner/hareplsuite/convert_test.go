package hareplsuite

import "testing"

func TestAddPrimaryKeyPutsItOnTheFirstColumn(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{
			"one column",
			"create table t(i int)",
			"create table t(i int PRIMARY KEY)",
		},
		{
			"several columns",
			"create table t(i int, v varchar(10))",
			"create table t(i int PRIMARY KEY, v varchar(10))",
		},
		{
			// The comma inside numeric(10,2) is not the end of a column, and
			// this is the case CTP's position-comparing version cannot see.
			"a comma inside the type",
			"create table t(n numeric(10,2), v varchar(10))",
			"create table t(n numeric(10,2) PRIMARY KEY, v varchar(10))",
		},
		{
			"a column list over several lines",
			"create class tb(\n\tcol1 char(20),\n\tcol2 int\n)",
			"create class tb(\n\tcol1 char(20) PRIMARY KEY,\n\tcol2 int\n)",
		},
		{
			// A parenthesis and a comma inside a default literal are text.
			"a literal with punctuation in it",
			"create table t(v varchar(10) default 'a,b(c', i int)",
			"create table t(v varchar(10) default 'a,b(c' PRIMARY KEY, i int)",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, changed := AddPrimaryKey(c.in)
			if !changed {
				t.Fatalf("not converted: %q", c.in)
			}
			if got != c.want {
				t.Errorf("\n got: %q\nwant: %q", got, c.want)
			}
		})
	}
}

// The transform leaves alone what it cannot improve, and says so by returning
// false. A statement it changed when it should not have is a case that now
// fails for a reason the corpus did not write.
func TestAddPrimaryKeyLeavesTheseAlone(t *testing.T) {
	for _, in := range []string{
		"create table t(i int primary key, v varchar(10))",
		"create table t(i int, v varchar(10), PRIMARY KEY(i))",
		"create table t2 as select * from t1",
		"insert into t values(1)",
		"select * from t",
		"create view v as select * from t",
		"create index ix on t(i)",
	} {
		got, changed := AddPrimaryKey(in)
		if changed {
			t.Errorf("changed what it should not have: %q -> %q", in, got)
		}
		if got != in {
			t.Errorf("returned a different statement without saying so: %q", got)
		}
	}
}
