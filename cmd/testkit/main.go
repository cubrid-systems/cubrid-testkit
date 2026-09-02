// Command testkit runs CUBRID's functional tests.
//
// It is a drop-in for bin/ctp.sh: same arguments, same task names, same exit
// codes. Tasks that have been rewritten run here; the rest are handed back to the
// original CTP, and from outside there is no way to tell which is which.
//
// Right now nothing has been rewritten, so every task takes the second path. That
// is the point of this first step -- the command line, the config lookup and the
// exit codes can be proven against the old behaviour before any test-running code
// is touched.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/cubrid-systems/cubrid-testkit/internal/cli"
	"github.com/cubrid-systems/cubrid-testkit/internal/conf"
	"github.com/cubrid-systems/cubrid-testkit/internal/registry"
	"github.com/cubrid-systems/cubrid-testkit/internal/runner"
	"github.com/cubrid-systems/cubrid-testkit/internal/runner/legacy"
)

// version is stamped at build time: -ldflags "-X main.version=..."
var version = "dev"

// Exit codes are frozen (docs/concept/external-surface-freeze.md §6-1).
const (
	exitOK = 0
	// exitPreflight is what bin/ctp.sh returned when JAVA_HOME was unset. It now
	// covers pre-flight environment failures generally, graded F2.
	exitPreflight = 1
	// exitEnvironment is System.exit(-1) as the shell sees it. A Go program has to
	// say 255 for the caller to see the same byte.
	exitEnvironment = 255
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	inv, err := cli.Parse(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "testkit: %v\n", err)
		cli.Usage(os.Stderr)
		return exitPreflight
	}

	if inv.Version {
		fmt.Printf("cubrid-testkit %s\n", version)
		return exitOK
	}
	if inv.Help || (len(inv.Tasks) == 0 && len(inv.Unknown) == 0) {
		cli.Usage(os.Stdout)
		return exitOK
	}

	// An unrecognised name prints help and is skipped. It is not an error and it
	// does not stop the tasks around it -- CTP behaved this way and something may
	// depend on it.
	for _, u := range inv.Unknown {
		fmt.Fprintf(os.Stderr, "testkit: unknown task %q\n", u)
		cli.Usage(os.Stderr)
	}
	if len(inv.Tasks) == 0 {
		return exitOK
	}

	home, err := conf.FindHome()
	if err != nil {
		fmt.Fprintf(os.Stderr, "testkit: %v\n", err)
		return exitPreflight
	}

	reg := registry.New()
	// Legacy claims everything. A native runner registered after this one takes
	// over the tasks it names, and that is the whole mechanism of the migration.
	reg.Register(legacy.New(cli.Active...))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	for _, task := range inv.Tasks {
		if cli.IsRetired(task) {
			// CTP accepted these and did nothing, without saying so. The outcome is
			// unchanged -- nothing runs, the exit code is untouched -- but the
			// silence is gone. Disposal policy is ADR-005, still open.
			fmt.Fprintf(os.Stderr, "testkit: task %q was retired; nothing to run\n", task)
			continue
		}

		rn, ok := reg.Lookup(task)
		if !ok {
			fmt.Fprintf(os.Stderr, "testkit: no runner for %q\n", task)
			continue
		}

		req := runner.Request{
			Task:        task,
			Home:        home,
			ConfigPath:  home.ConfigFor(task.Suite(), inv.ConfigPath),
			Interactive: inv.Interactive,
		}
		if task == cli.WebConsole && inv.WebConsoleAction != "" {
			req.Extra = []string{inv.WebConsoleAction}
		}

		if err := rn.Validate(req); err != nil {
			return report(err, exitPreflight)
		}
		if err := rn.Run(ctx, req); err != nil {
			return report(err, exitEnvironment)
		}
	}
	return exitOK
}

// report prints the failure and returns the exit code it carries, falling back to
// fallbackCode when the error does not name one.
func report(err error, fallbackCode int) int {
	fmt.Fprintf(os.Stderr, "testkit: %v\n", err)
	var exitErr *runner.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	return fallbackCode
}
