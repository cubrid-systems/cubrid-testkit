package cli

import "testing"

// The command line is a frozen surface, so these tests are the surface written
// down. A change that breaks one of them is a change to the contract, not a
// refactor.

func TestSeveralTasksRunInOrder(t *testing.T) {
	inv, err := Parse([]string{"sql", "medium", "shell"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := []Task{SQL, Medium, Shell}
	if len(inv.Tasks) != len(want) {
		t.Fatalf("got %v, want %v", inv.Tasks, want)
	}
	for i := range want {
		if inv.Tasks[i] != want[i] {
			t.Errorf("position %d: got %q, want %q", i, inv.Tasks[i], want[i])
		}
	}
}

func TestTaskNamesAreCaseInsensitive(t *testing.T) {
	// CTP upper-cases the argument before the enum lookup, so every casing works.
	for _, arg := range []string{"shell", "SHELL", "Shell", "sHeLl"} {
		inv, err := Parse([]string{arg})
		if err != nil {
			t.Fatalf("%q: %v", arg, err)
		}
		if len(inv.Tasks) != 1 || inv.Tasks[0] != Shell {
			t.Errorf("%q: got %v, want [shell]", arg, inv.Tasks)
		}
	}
}

func TestUnknownTaskDoesNotStopTheOthers(t *testing.T) {
	// An unrecognised name prints help and is skipped. The tasks around it still
	// run, and the exit code is unaffected.
	inv, err := Parse([]string{"shell", "nosuchtask", "sql"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(inv.Tasks) != 2 || inv.Tasks[0] != Shell || inv.Tasks[1] != SQL {
		t.Errorf("tasks: got %v, want [shell sql]", inv.Tasks)
	}
	if len(inv.Unknown) != 1 || inv.Unknown[0] != "nosuchtask" {
		t.Errorf("unknown: got %v, want [nosuchtask]", inv.Unknown)
	}
}

func TestRetiredNamesStayTasks(t *testing.T) {
	// CTP accepted these and did nothing. They are kept as tasks so the runner can
	// say they are retired, rather than being reported as a misspelling.
	inv, err := Parse([]string{"tpcc"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(inv.Tasks) != 1 || !IsRetired(inv.Tasks[0]) {
		t.Errorf("got tasks %v unknown %v, want tpcc as a retired task", inv.Tasks, inv.Unknown)
	}
}

func TestConfigForms(t *testing.T) {
	for _, args := range [][]string{
		{"-c", "/tmp/x.conf", "shell"},
		{"--config", "/tmp/x.conf", "shell"},
		{"--config=/tmp/x.conf", "shell"},
		{"shell", "-c", "/tmp/x.conf"},
	} {
		inv, err := Parse(args)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if inv.ConfigPath != "/tmp/x.conf" {
			t.Errorf("%v: got %q", args, inv.ConfigPath)
		}
		if len(inv.Tasks) != 1 || inv.Tasks[0] != Shell {
			t.Errorf("%v: tasks %v", args, inv.Tasks)
		}
	}
}

func TestConfigWithoutValueIsAnError(t *testing.T) {
	if _, err := Parse([]string{"shell", "-c"}); err == nil {
		t.Error("want an error when -c has no value")
	}
}

func TestWebConsoleTakesAnAction(t *testing.T) {
	inv, err := Parse([]string{"webconsole", "start"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if inv.WebConsoleAction != "start" {
		t.Errorf("action: got %q, want start", inv.WebConsoleAction)
	}
	if len(inv.Unknown) != 0 {
		t.Errorf("start should not be reported unknown, got %v", inv.Unknown)
	}
}

func TestRqgFallsBackToShellConfig(t *testing.T) {
	// rqg runs the shell path with a category flag; its config is shell.conf.
	if got := RQG.Suite(); got != "shell" {
		t.Errorf("rqg suite: got %q, want shell", got)
	}
	if got := Isolation.Suite(); got != "isolation" {
		t.Errorf("isolation suite: got %q, want isolation", got)
	}
}
