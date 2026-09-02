package cli

import "strings"

// Task is a name ctp.sh accepts as a positional argument. The set is frozen:
// see docs/concept/external-surface-freeze.md §1-2.
type Task string

// The fourteen tasks that do something. Order matters only for help output.
const (
	SQL        Task = "sql"
	Medium     Task = "medium"
	KCC        Task = "kcc"
	Neis05     Task = "neis05"
	Neis08     Task = "neis08"
	SQLByCCI   Task = "sql_by_cci"
	Shell      Task = "shell"
	RQG        Task = "rqg"
	UnitTest   Task = "unittest"
	Isolation  Task = "isolation"
	HARepl     Task = "ha_repl"
	CDCRepl    Task = "cdc_repl"
	JDBC       Task = "jdbc"
	WebConsole Task = "webconsole"
)

// Active lists the tasks that dispatch somewhere, in the order CTP declares them.
var Active = []Task{
	SQL, Medium, KCC, Neis05, Neis08, SQLByCCI,
	Shell, RQG, UnitTest,
	Isolation, HARepl, CDCRepl,
	JDBC, WebConsole,
}

// Retired names CTP still accepts and silently ignores: they are declared in
// ComponentEnum but no branch handles them, so asking for one does nothing at all
// and says nothing about it.
//
// We do not reproduce the silence. NG7 forbids carrying a dead surface across as a
// quiet no-op, and analysis/_overview/orphan-enums.md recommends removing these
// after a grace period. Until ADR-005 settles the policy, asking for one of these
// prints that it is retired and moves on to the next task -- which is the same
// observable outcome as before (nothing runs, exit code unaffected) with the
// silence removed.
var Retired = []Task{"cci", "dots", "nbd", "sysbench", "tpcc", "tpcw", "ycsb"}

// ParseTask maps a positional argument to a task. CTP upper-cases the argument
// before looking it up in the enum, so matching is case-insensitive.
func ParseTask(arg string) (Task, bool) {
	name := Task(strings.ToLower(strings.TrimSpace(arg)))
	for _, t := range Active {
		if t == name {
			return t, true
		}
	}
	return name, false
}

// IsRetired reports whether the name is one CTP accepted and ignored.
func IsRetired(t Task) bool {
	for _, r := range Retired {
		if r == t {
			return true
		}
	}
	return false
}

// Suite is the config basename a task falls back to when -c is absent:
// $CTP_HOME/conf/<suite>.conf. Most tasks use their own name; rqg and unittest
// are the exceptions, and webconsole takes no config of its own here.
func (t Task) Suite() string {
	switch t {
	case RQG:
		return "shell"
	default:
		return string(t)
	}
}
