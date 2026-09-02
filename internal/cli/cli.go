// Package cli parses the command line ctp.sh has always accepted.
//
// The grammar is frozen at F1 (docs/concept/external-surface-freeze.md §1-1):
//
//	ctp.sh <task>... [-c <conf>] [--interactive] [-h] [-v]
//
// Three details are easy to lose and each is part of the contract:
//
//   - several tasks may be named at once, and they run in the order given;
//   - task names are matched case-insensitively;
//   - a name that is not a task prints help and skips that task only. It is not
//     an error, and the tasks after it still run.
package cli

import (
	"fmt"
	"io"
	"strings"
)

// Invocation is one parsed command line.
type Invocation struct {
	Tasks       []Task   // in the order given, duplicates preserved
	Unknown     []string // names that matched no task, in the order given
	ConfigPath  string   // -c / --config; empty means fall back per task
	Interactive bool     // --interactive; only the sql family reads it
	Help        bool
	Version     bool

	// WebConsoleAction is the second positional argument when the first task is
	// webconsole: `ctp.sh webconsole start|stop`.
	WebConsoleAction string
}

// Parse reads the argument list. It returns an error only for a malformed line --
// an option that needs a value and did not get one. Unknown task names are not
// errors; they land in Unknown.
func Parse(args []string) (*Invocation, error) {
	inv := &Invocation{}
	var positional []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-c" || arg == "--config":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("%s requires a path", arg)
			}
			i++
			inv.ConfigPath = args[i]
		case strings.HasPrefix(arg, "--config="):
			inv.ConfigPath = strings.TrimPrefix(arg, "--config=")
		case arg == "--interactive":
			inv.Interactive = true
		case arg == "-h" || arg == "--help":
			inv.Help = true
		case arg == "-v" || arg == "--version":
			inv.Version = true
		default:
			positional = append(positional, arg)
		}
	}

	for _, p := range positional {
		if t, ok := ParseTask(p); ok {
			inv.Tasks = append(inv.Tasks, t)
			continue
		}
		// A retired name is still a name CTP knew. Keep it as a task so the
		// runner layer can say so, rather than reporting it as a typo.
		if t := Task(strings.ToLower(p)); IsRetired(t) {
			inv.Tasks = append(inv.Tasks, t)
			continue
		}
		inv.Unknown = append(inv.Unknown, p)
	}

	// `ctp.sh webconsole start` -- the action is the argument after the task, and
	// it is not itself a task, so it will have landed in Unknown.
	if len(inv.Tasks) > 0 && inv.Tasks[0] == WebConsole && len(inv.Unknown) > 0 {
		inv.WebConsoleAction = inv.Unknown[0]
		inv.Unknown = inv.Unknown[1:]
	}

	return inv, nil
}

// Usage writes the help text. CTP prints help both for -h and for an unrecognised
// task name, so this is reachable from two places.
func Usage(w io.Writer) {
	fmt.Fprintln(w, "Usage: ctp.sh <task>... [-c <conf>] [--interactive] [-h] [-v]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Tasks:")
	for _, t := range Active {
		fmt.Fprintf(w, "  %s\n", t)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Options:")
	fmt.Fprintln(w, "  -c, --config <path>   configuration file")
	fmt.Fprintln(w, "                        without it, $CTP_HOME/conf/<suite>.conf")
	fmt.Fprintln(w, "      --interactive     interactive run (sql family only)")
	fmt.Fprintln(w, "  -h, --help            this text")
	fmt.Fprintln(w, "  -v, --version         version")
}
