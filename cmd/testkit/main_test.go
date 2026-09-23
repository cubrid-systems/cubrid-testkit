package main

import (
	"strings"
	"testing"

	"github.com/cubrid-systems/cubrid-testkit/internal/cli"
)

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

// The two runners are hard to tell apart from their output, so the one about to
// run says which it is. A peer session debugged CTP's precondition check by
// reading this project's port of it for an hour; the port was not running.
func TestATaskHandedToCTPSaysSoAndNamesTheSwitch(t *testing.T) {
	t.Setenv("TESTKIT_NATIVE", "")
	t.Setenv("TESTKIT_NATIVE_SHELL", "")
	var b strings.Builder
	sayWhoRunsIt(&b, cli.Shell)
	got := b.String()
	if !strings.Contains(got, "CTP") || !strings.Contains(got, "TESTKIT_NATIVE=shell") {
		t.Errorf("a task going to CTP said %q, which does not name the switch", got)
	}

	// And it is quiet when the native runner is the one that will run.
	t.Setenv("TESTKIT_NATIVE", "shell")
	b.Reset()
	sayWhoRunsIt(&b, cli.Shell)
	if b.Len() != 0 {
		t.Errorf("said %q about a task that runs here", b.String())
	}

	// A task no native runner claims gets no advice either: there is no switch
	// to name, and a line that named one would send the reader after nothing.
	t.Setenv("TESTKIT_NATIVE", "")
	b.Reset()
	sayWhoRunsIt(&b, cli.JDBC)
	if b.Len() != 0 {
		t.Errorf("said %q about a task with no native runner", b.String())
	}
}
