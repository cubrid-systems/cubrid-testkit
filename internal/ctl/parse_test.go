package ctl

import (
	"strings"
	"testing"
)

func TestStatements(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"one statement", "C1: select 1;", []string{"C1: select 1;"}},
		{
			"white space collapses and does not lead",
			"  C1:\n\tselect\n   1;",
			[]string{"C1: select 1;"},
		},
		{
			"two statements on one line, as 2,328 cases write them",
			"C2: drop table t1; drop table t2;",
			[]string{"C2: drop table t1;", "drop table t2;"},
		},
		{
			"a C comment is dropped",
			"/* Test Case: x\n * Author: y\n */\nMC: setup NUM_CLIENTS = 2;",
			[]string{"MC: setup NUM_CLIENTS = 2;"},
		},
		{
			"an SQL comment runs to the end of the line",
			"C1: select 1; -- why not\nC1: select 2;",
			[]string{"C1: select 1;", "C1: select 2;"},
		},
		{
			"a semicolon inside a string is not a terminator",
			"C1: insert into t values ('a;b');",
			[]string{"C1: insert into t values ('a;b');"},
		},
		{
			"a doubled quote stays inside the string",
			"C1: insert into t values ('it''s; here');",
			[]string{"C1: insert into t values ('it''s; here');"},
		},
		{
			"a double-quoted identifier may hold a semicolon",
			`C1: select "a;b" from t;`,
			[]string{`C1: select "a;b" from t;`},
		},
		{
			"the last statement need not be terminated",
			"C1: quit",
			[]string{"C1: quit"},
		},
		{
			"a division is not a comment",
			"C1: select 4/2;",
			[]string{"C1: select 4/2;"},
		},
		{
			"a minus is not a comment",
			"C1: select 4-2;",
			[]string{"C1: select 4-2;"},
		},
		{"nothing but white space is no statement", "\n  \n\t\n", nil},
		{"nothing but a comment is no statement", "/* just this */\n", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Statements(strings.NewReader(c.in))
			if len(got) != len(c.want) {
				t.Fatalf("got %d statements %q, want %d %q", len(got), got, len(c.want), c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("statement %d: got %q, want %q", i+1, got[i], c.want[i])
				}
			}
		})
	}
}

// A line longer than the 1024-byte fgets buffer counts as two lines, because
// the old parser counted fgets calls. The statement itself is unaffected.
func TestLinesCountsFgetsCalls(t *testing.T) {
	long := "C1: select '" + strings.Repeat("x", 1100) + "';\n"
	p := NewReader(strings.NewReader(long))
	if _, ok := p.Next(); !ok {
		t.Fatal("no statement")
	}
	if p.Lines() != 2 {
		t.Errorf("got %d lines, want 2", p.Lines())
	}
}
