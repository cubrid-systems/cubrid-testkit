package main

import "testing"

// One program, one switch -- and the spelling every operator already has in
// their shell history still works.
func TestWhichFamiliesRunHere(t *testing.T) {
	for _, c := range []struct {
		name          string
		native        string
		shellOld      string
		sqlOld        string
		shell, sqlFam bool
	}{
		{name: "nothing set: CTP runs everything"},
		{name: "one family", native: "sql", sqlFam: true},
		{name: "both, comma-separated", native: "shell,sql", shell: true, sqlFam: true},
		{name: "spaces and case", native: " Shell , SQL ", shell: true, sqlFam: true},
		{name: "all", native: "all", shell: true, sqlFam: true},
		{name: "the older spelling", shellOld: "1", shell: true},
		{name: "the older spelling, sql", sqlOld: "1", sqlFam: true},
		{name: "one of each", native: "sql", shellOld: "1", shell: true, sqlFam: true},
		{name: "a family nobody named", native: "medium"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("TESTKIT_NATIVE", c.native)
			t.Setenv("TESTKIT_NATIVE_SHELL", c.shellOld)
			t.Setenv("TESTKIT_NATIVE_SQL", c.sqlOld)
			if got := native("shell"); got != c.shell {
				t.Errorf("shell runs here = %v, want %v", got, c.shell)
			}
			if got := native("sql"); got != c.sqlFam {
				t.Errorf("sql runs here = %v, want %v", got, c.sqlFam)
			}
		})
	}
}
