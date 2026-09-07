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
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/cli"
	"github.com/cubrid-systems/cubrid-testkit/internal/conf"
	"github.com/cubrid-systems/cubrid-testkit/internal/contain"
	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
	"github.com/cubrid-systems/cubrid-testkit/internal/registry"
	"github.com/cubrid-systems/cubrid-testkit/internal/result"
	"github.com/cubrid-systems/cubrid-testkit/internal/runner"
	"github.com/cubrid-systems/cubrid-testkit/internal/runner/legacy"
	"github.com/cubrid-systems/cubrid-testkit/internal/runner/shellsuite"
	"github.com/cubrid-systems/cubrid-testkit/internal/runshell"
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
	// Containment happens before anything else or it happens to a process that
	// has already opened files and started goroutines. Enter re-executes this
	// program in namespaces of its own and returns the child's exit code; -1
	// means there was nothing to do, which is the default.
	if code := contain.Enter(); code >= 0 {
		os.Exit(code)
	}
	if err := contain.Setup(); err != nil {
		fmt.Fprintf(os.Stderr, "testkit: %v\n", err)
		os.Exit(exitEnvironment)
	}
	contain.Reap()

	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	// run-shell is the other CLI tree: one case, looped until it fails. CTP
	// shipped it as shell/init_path/run_shell.sh rather than as a ctp.sh task, and
	// keeping that separation is what stops "run the corpus" and "hound one case"
	// from growing into each other's options
	// (docs/concept/external-surface-freeze.md §1-4).
	if len(args) > 0 && args[0] == "run-shell" {
		return runShell(args[1:])
	}

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
	reg.Register(shellsuite.NewUnitTest())

	// The shell runner is complete but has never been compared against CTP on a
	// real machine, and taking over the task that runs 3,452 cases on the strength
	// of unit tests would be the wrong way round. It is opt-in until
	// docs/evidence/regression-shell.md exists; then this gate comes off and the
	// registration below becomes unconditional, which is the whole mechanism of
	// the migration (ADR-004, ADR-013).
	if os.Getenv("TESTKIT_NATIVE_SHELL") == "1" {
		reg.Register(shellsuite.NewShell())
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	for _, task := range inv.Tasks {
		// The dispatcher's banner comes before the name is resolved, so even a task
		// that turns out to be retired gets one. CTP prints it that way.
		start := time.Now()
		result.TaskBanner(os.Stdout, string(task), start)

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
		// A missing file is not fatal here. CTP passed a null configuration to the
		// unittest entry point, and the legacy path hands the path back to CTP,
		// which reports its own absence in its own words.
		if cfg, err := home.Load(req.ConfigPath); err == nil {
			req.Config = cfg
		}
		if task == cli.WebConsole && inv.WebConsoleAction != "" {
			req.Extra = []string{inv.WebConsoleAction}
		}

		result.TaskStarted(os.Stdout, string(task), start)

		if err := rn.Validate(req); err != nil {
			return report(err, exitPreflight)
		}
		if err := rn.Run(ctx, req); err != nil {
			return report(err, exitEnvironment)
		}

		end := time.Now()
		result.TaskEnded(os.Stdout, string(task), end, end.Sub(start))
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

// runShellUsage is what -h prints. CTP used commons-cli's HelpFormatter under the
// heading "run_shell [OPTION]"; the seven options it listed and this does not are
// QA operations, and asking for one now says so rather than being ignored.
const runShellUsage = `usage: run-shell [OPTION] [testcase]

Run one test case, repeatedly, until it fails. The testcase argument may name the
case directory, its cases/ subdirectory, or a file in either; it defaults to the
working directory.

    --loop                    keep running until a failure is checked
    --maxloop <n>             stop after n loops
    --maxtime <seconds>       stop after n seconds
    --extend-script <file>    source it and call "verify <dir> <name>.result"
                              instead of reading the result file
    --prompt-continue <bool>  answer the continue prompt without a terminal
 -h,--help                    this

Touching a file named STOP in the case directory ends the loop after the attempt
in flight.

--update-build, --next-build-url, --enable-report, --report-cron, --mailto,
--mailcc and --issue are QA operations and are not implemented here
(docs/concept/migration-exclusions.md).
`

// runShell is the entry point for the looping single-case tool.
func runShell(args []string) int {
	fs := flag.NewFlagSet("run-shell", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var (
		loop           = fs.Bool("loop", false, "")
		maxLoop        = fs.Int("maxloop", 0, "")
		maxTime        = fs.Int("maxtime", 0, "")
		extendScript   = fs.String("extend-script", "", "")
		promptContinue = fs.String("prompt-continue", "", "")
		help           = fs.Bool("help", false, "")
		helpShort      = fs.Bool("h", false, "")
	)
	// The excluded options are accepted and refused rather than rejected as
	// unknown, so that an operator who used them gets told why instead of being
	// told they made a typo.
	excluded := map[string]*string{}
	for _, name := range []string{"next-build-url", "report-cron", "mailto", "mailcc", "issue"} {
		excluded[name] = fs.String(name, "", "")
	}
	excludedFlags := map[string]*bool{}
	for _, name := range []string{"update-build", "enable-report"} {
		excludedFlags[name] = fs.Bool(name, false, "")
	}

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprint(os.Stdout, runShellUsage)
		return exitPreflight
	}
	if *help || *helpShort {
		fmt.Fprint(os.Stdout, runShellUsage)
		return exitOK
	}
	for name, v := range excluded {
		if *v != "" {
			fmt.Fprintf(os.Stderr, "run-shell: --%s is QA operations and is not implemented here "+
				"(docs/concept/migration-exclusions.md)\n", name)
			return exitPreflight
		}
	}
	for name, v := range excludedFlags {
		if *v {
			fmt.Fprintf(os.Stderr, "run-shell: --%s is QA operations and is not implemented here "+
				"(docs/concept/migration-exclusions.md)\n", name)
			return exitPreflight
		}
	}

	opts := runshell.Options{
		Loop:         *loop,
		MaxLoop:      *maxLoop,
		MaxTime:      time.Duration(*maxTime) * time.Second,
		ExtendScript: *extendScript,
	}
	if *promptContinue != "" {
		yes := strings.EqualFold(*promptContinue, "true") || strings.EqualFold(*promptContinue, "y")
		opts.PromptContinue = &yes
	}

	arg := ""
	if fs.NArg() > 0 {
		arg = fs.Arg(0)
	}
	c, err := runshell.Locate(arg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprint(os.Stdout, runShellUsage)
		return exitPreflight
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ch := &exec.Local{SourceProfile: true}
	defer ch.Close()

	meta, err := runshell.ReadMeta(ctx, ch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return exitEnvironment
	}

	r := &runshell.Run{Case: c, Options: opts, Channel: ch, Meta: meta, Out: os.Stdout, In: os.Stdin}
	res := r.Go(ctx)

	// CTP ended with System.exit(0) unconditionally, so a run that printed
	// QUIT(NOK) still reported success and "run_shell.sh ... && ..." passed.
	// Fixed in axis T as a clear bug (docs/concept/external-surface-freeze.md).
	if res.Failed() {
		return exitPreflight
	}
	return exitOK
}
