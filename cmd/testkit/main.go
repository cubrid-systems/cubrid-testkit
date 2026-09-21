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
	"github.com/cubrid-systems/cubrid-testkit/internal/status"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/casecheck"
	"github.com/cubrid-systems/cubrid-testkit/internal/cli"
	"github.com/cubrid-systems/cubrid-testkit/internal/conf"
	"github.com/cubrid-systems/cubrid-testkit/internal/contain"
	"github.com/cubrid-systems/cubrid-testkit/internal/ctl"
	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
	"github.com/cubrid-systems/cubrid-testkit/internal/registry"
	"github.com/cubrid-systems/cubrid-testkit/internal/result"
	"github.com/cubrid-systems/cubrid-testkit/internal/runner"
	"github.com/cubrid-systems/cubrid-testkit/internal/runner/hareplsuite"
	"github.com/cubrid-systems/cubrid-testkit/internal/runner/isolationsuite"
	"github.com/cubrid-systems/cubrid-testkit/internal/runner/legacy"
	"github.com/cubrid-systems/cubrid-testkit/internal/runner/shellsuite"
	"github.com/cubrid-systems/cubrid-testkit/internal/runner/sqlsuite"
	"github.com/cubrid-systems/cubrid-testkit/internal/runshell"
	"github.com/cubrid-systems/cubrid-testkit/internal/sizing"
)

// version is stamped at build time: -ldflags "-X main.version=..."
var version = "dev"

// Exit codes are frozen (docs/project/concept/external-surface-freeze.md §6-1).
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
	// The isolation controller is a leaf: runone.sh calls it once per case,
	// inside the slot the runner has already made, in place of ctltool's qactl
	// (ADR-019). It goes before containment because it is not a run.
	if len(os.Args) > 1 && os.Args[1] == "isolation-ctl" {
		os.Exit(isolationCtl(os.Args[2:]))
	}
	// And this is how it gets there: the runner asks a slot to install it, from
	// inside, because out here that path is still ctltool's own directory.
	if len(os.Args) > 1 && os.Args[1] == "isolation-ctl-install" {
		os.Exit(isolationCtlInstall(os.Args[2:]))
	}
	// sizing says what this machine would run at once, and on what evidence.
	if len(os.Args) > 1 && os.Args[1] == "sizing" {
		os.Exit(sizingCmd(os.Args[2:]))
	}
	// check-cases reads a corpus and reports the cases that cannot fail. It runs
	// no case, so it needs no engine, no database and no containment -- which is
	// why it goes here rather than through run().
	if len(os.Args) > 1 && os.Args[1] == "check-cases" {
		os.Exit(checkCasesCmd(os.Args[2:]))
	}

	// Containment happens before anything else or it happens to a process that
	// has already opened files and started goroutines. Enter re-executes this
	// program in namespaces of its own and returns the child's exit code; -1
	// means there was nothing to do, which is the default.
	if code := contain.Enter(); code >= 0 {
		os.Exit(code)
	}
	// PID 1 of that namespace becomes an init and forks the work: collecting
	// orphans is Wait4(-1), which would otherwise take os/exec's own children
	// out from under it. -1 again means this process is the one doing the work.
	if code := contain.Init(); code >= 0 {
		os.Exit(code)
	}
	if err := contain.Setup(); err != nil {
		fmt.Fprintf(os.Stderr, "testkit: %v\n", err)
		os.Exit(exitEnvironment)
	}

	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	// run-shell is the other CLI tree: one case, looped until it fails. CTP
	// shipped it as shell/init_path/run_shell.sh rather than as a ctp.sh task, and
	// keeping that separation is what stops "run the corpus" and "hound one case"
	// from growing into each other's options
	// (docs/project/concept/external-surface-freeze.md §1-4).
	if len(args) > 0 && args[0] == "run-shell" {
		return runShell(args[1:])
	}

	// replay is the third CLI tree, and it is new rather than inherited: CTP had
	// nothing like it. It plays a finished run back through the status page, at a
	// speed, from the feedback.log the run already wrote -- so it works on runs
	// that finished before any of this existed.
	if len(args) > 0 && args[0] == "replay" {
		return replay(args[1:])
	}

	// failures turns a finished run into the list of cases to try again. It is
	// the other half of testcase_from_file, and it is new rather than inherited.
	if len(args) > 0 && args[0] == "failures" {
		return failures(args[1:])
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

	// Each family runs here only when it is asked for. Taking over the task that
	// runs 3,452 cases -- or the one that runs 17,459 -- on the strength of unit
	// tests would be the wrong way round; the gates come off family by family as
	// the comparison against CTP is made (ADR-004, ADR-013, ADR-017).
	if native("shell") {
		reg.Register(shellsuite.NewShell())
	}
	if native("sql") {
		reg.Register(sqlsuite.New())
	}
	if native("isolation") {
		reg.Register(isolationsuite.New())
	}
	// ha_repl is behind the same switch and for a different reason: there is no
	// gate to clear, because there is nothing to clear it against. CTP reaches
	// its nodes over SSH and a sandbox node runs no sshd, so the two runners
	// cannot be pointed at the same pair and compared (ADR-022). What this one
	// produces is the pair's own verdict, and it says so.
	if native("ha_repl") {
		reg.Register(hareplsuite.New())
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
(docs/project/concept/migration-exclusions.md).
`

// runShell is the entry point for the looping single-case tool.
// isolationCtl is `qactl <db> <case.ctl> <client program>`: the three arguments
// runone.sh passes its controller (runone.sh:265). The probe it needs sits
// beside the client program unless TESTKIT_ISOLATION_PROBE says otherwise.
func isolationCtl(args []string) int {
	if len(args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: testkit isolation-ctl <db> <case.ctl> <client program>")
		return 2
	}
	probe := os.Getenv("TESTKIT_ISOLATION_PROBE")
	if probe == "" {
		probe = filepath.Join(filepath.Dir(args[2]), "qablocked")
	}
	return ctl.Run(ctl.Options{
		DB:     args[0],
		Case:   args[1],
		Client: args[2],
		Probe:  probe,
		Out:    os.Stdout,
		Err:    os.Stderr,
	})
}

// sizingCmd prints what a suite would run at once on this machine, and the runs
// it read to decide (ADR-020). It decides nothing and runs nothing.
//
//	testkit sizing [suite] [mode] [corpus] [lane]
//
// corpus is the scenario path a run would name, and lane where its slots would
// write; without them the newest run's are taken, which is the question most
// often asked -- what would the same run again get.
func sizingCmd(args []string) int {
	arg := func(i int) string {
		if len(args) > i {
			return strings.TrimSpace(args[i])
		}
		return ""
	}
	suite := sizing.Isolation
	if arg(0) != "" {
		suite = sizing.Suite(arg(0))
		if !slices.Contains(sizing.Suites, suite) {
			fmt.Fprintf(os.Stderr, "%s is not a suite. One of: %v\n", arg(0), sizing.Suites)
			return 2
		}
	}
	mode, warn := sizing.ModeOf(arg(1))
	if warn != "" {
		fmt.Fprintln(os.Stderr, warn)
	}
	machine := sizing.ThisMachine()
	avail := sizing.MemAvailableMB()
	eng := sizing.EngineFromConf(os.Getenv("CUBRID"))
	recs := sizing.Load(suite)
	corpus, lane := arg(2), arg(3)
	if corpus == "" && len(recs) > 0 {
		corpus = recs[0].Corpus
	}
	if lane == "" {
		lane = contain.Lane()
		if len(recs) > 0 {
			lane = recs[0].Lane
		}
	}

	fmt.Printf("machine   %s, %d processors, %d MB of memory, %d MB free now\n",
		machine.Host, machine.Cores, machine.MemTotalMB, avail)
	fmt.Printf("engine    data_buffer_size %d MB, log_buffer_size %d MB, from $CUBRID/conf/cubrid.conf\n",
		eng.DataBufferMB, eng.LogBufferMB)
	if len(recs) == 0 {
		fmt.Printf("runs      none at %s: this machine has not run %s yet\n", sizing.Path(suite), suite)
	} else {
		fmt.Printf("runs      %s, newest first\n", sizing.Path(suite))
		for _, r := range recs {
			fmt.Printf("          %s  %2d slots  %5d s wall  %5d MB a slot  %5d cases  longest %4d s  %s  %s\n",
				r.When, r.Slots, r.WallS, r.PerSlotMB, r.Cases, r.LongestUnitS, orNone(r.Lane), r.Corpus)
		}
	}
	fmt.Printf("sizing    %s on %s\n", orNone(corpus), orNone(lane))
	if suite == sizing.Shell {
		fmt.Printf("          a run also sets scenario_ram_mb aside from memory, and is bounded by its own case count,\n")
		fmt.Printf("          so what it takes can be lower than this\n")
	}
	for _, m := range []sizing.Mode{sizing.Conservative, sizing.Measured, sizing.Aggressive} {
		p := sizing.Slots(sizing.Input{Suite: suite, Mode: m, Engine: eng, AvailMB: avail, Corpus: corpus, Lane: lane,
			Records: recs})
		mark := " "
		if m == mode {
			mark = "*"
		}
		fmt.Printf("%s %-13s %2d slots -- %s\n", mark, m, p.Slots, p.Why)
	}
	return 0
}

func orNone(s string) string {
	if s == "" {
		return "unrecorded"
	}
	return s
}

// isolationCtlInstall makes one ctltool directory run this controller instead of
// ctltool's own (ADR-019 Consequence 4).
func isolationCtlInstall(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: testkit isolation-ctl-install <ctltool directory>")
		return 2
	}
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "testkit: %v\n", err)
		return 1
	}
	if err := ctl.Install(args[0], self); err != nil {
		fmt.Fprintf(os.Stderr, "testkit: %v\n", err)
		return 1
	}
	return 0
}

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
				"(docs/project/concept/migration-exclusions.md)\n", name)
			return exitPreflight
		}
	}
	for name, v := range excludedFlags {
		if *v {
			fmt.Fprintf(os.Stderr, "run-shell: --%s is QA operations and is not implemented here "+
				"(docs/project/concept/migration-exclusions.md)\n", name)
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
	// Fixed in axis T as a clear bug (docs/project/concept/external-surface-freeze.md).
	if res.Failed() {
		return exitPreflight
	}
	return exitOK
}

const replayUsage = `usage: replay [OPTION] <feedback.log | result directory>

Play a finished run back through the status page. The wall clock is compressed;
the durations reported are the ones the run really had, so the histogram, the
per-family totals and the finished table all say what actually happened.

The input is a run's feedback.log, or a directory holding one -- a result tree,
or its current_runtime_logs. Nothing was recorded for this: feedback.log already
carries the slot, the case, the verdict, the elapsed time and the wall clock each
case finished at, which is everything the page needs.

options:
  --speed N     times real time; default 60, so an hour plays in a minute
  --http ADDR   where to serve; "on" or a bare port are accepted, as in shell.conf
`

func replay(args []string) int {
	fs := flag.NewFlagSet("replay", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	speed := fs.Float64("speed", 60, "")
	addr := fs.String("http", "on", "")
	help := fs.Bool("h", false, "")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "replay: %v\n", err)
		fmt.Fprint(os.Stderr, replayUsage)
		return exitPreflight
	}
	if *help || fs.NArg() == 0 {
		fmt.Fprint(os.Stdout, replayUsage)
		return exitOK
	}

	path, err := findFeedback(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "replay: %v\n", err)
		return exitPreflight
	}
	events, err := status.ParseFeedbackFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "replay: %s: %v\n", path, err)
		return exitPreflight
	}
	where := status.Addr(*addr)
	if where == "" {
		where = status.DefaultAddr
	}
	stopPage, err := status.ReplayFrom(path, events, *speed, where, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "replay: %v\n", err)
		return exitEnvironment
	}
	defer stopPage()
	// The run is over but the page is the point, so it stays up until the user
	// stops looking at it.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	return exitOK
}

// findFeedback accepts the log itself, a result tree, or anything above one.
func findFeedback(arg string) (string, error) {
	info, err := os.Stat(arg)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return arg, nil
	}
	for _, try := range []string{
		"feedback.log",
		"current_runtime_logs/feedback.log",
		"shell/current_runtime_logs/feedback.log",
		"result/shell/current_runtime_logs/feedback.log",
	} {
		p := filepath.Join(arg, try)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("no feedback.log under %s", arg)
}

const failuresUsage = `usage: failures [OPTION] <feedback.log | result directory>

Print the cases a run failed, one a line, in the form testcase_from_file wants.
This is the loop a fix goes round: run the corpus, fix something, rebuild the
engine, and try the failures again -- which is minutes rather than the two hours
the whole corpus takes, and answers the question that was actually asked.

    testkit failures ~/CTP/result/shell > failed.txt
    # then, in the conf for the next run:
    #   testcase_from_file=/path/to/failed.txt

Paths are printed from the family segment down rather than in full, so the list
still selects the same cases when the corpus sits somewhere else. The engine may
be a different build; a case is the same case.

options:
  --full        print the whole path as the run recorded it
  --count       print only how many failed
`

func failures(args []string) int {
	fs := flag.NewFlagSet("failures", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	full := fs.Bool("full", false, "")
	count := fs.Bool("count", false, "")
	help := fs.Bool("h", false, "")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "failures: %v\n", err)
		fmt.Fprint(os.Stderr, failuresUsage)
		return exitPreflight
	}
	if *help || fs.NArg() == 0 {
		fmt.Fprint(os.Stdout, failuresUsage)
		return exitOK
	}
	path, err := findFeedback(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failures: %v\n", err)
		return exitPreflight
	}
	events, err := status.ParseFeedbackFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failures: %s: %v\n", path, err)
		return exitPreflight
	}

	// The last attempt wins. A retried case appears twice and the second
	// verdict is the run's; listing a case that ended up passing would send the
	// next run after something that is already fixed.
	last := map[string]status.Event{}
	for _, e := range events {
		if prev, seen := last[e.Case]; !seen || e.Start.After(prev.Start) {
			last[e.Case] = e
		}
	}
	var out []string
	for name, e := range last {
		if e.OK {
			continue
		}
		if *full {
			out = append(out, name)
		} else {
			out = append(out, caseFragment(name))
		}
	}
	sort.Strings(out)

	if *count {
		fmt.Println(len(out))
		return exitOK
	}
	if len(out) == 0 {
		fmt.Fprintln(os.Stderr, "failures: none -- every case in that run passed")
		return exitOK
	}
	fmt.Printf("# %d cases failed in %s\n", len(out), path)
	for _, c := range out {
		fmt.Println(c)
	}
	return exitOK
}

// caseFragment names a case from its family down, which is the part that does
// not change when the corpus moves. Same rule the status page names cases by.
func caseFragment(p string) string {
	parts := strings.Split(p, "/")
	for i, seg := range parts {
		if len(seg) > 3 && seg[0] == '_' && seg[1] >= '0' && seg[1] <= '9' {
			return strings.Join(parts[i:], "/")
		}
	}
	return p
}

// native says whether a family runs here rather than being handed to CTP.
//
// One switch, because this is one program: TESTKIT_NATIVE names the families,
// comma-separated, and "all" is every one of them.
//
//	TESTKIT_NATIVE=shell,sql
//
// The older spelling still works -- TESTKIT_NATIVE_SHELL=1, TESTKIT_NATIVE_SQL=1
// -- because it is what the shell category's documentation, the evidence
// scripts and every operator's shell history say. Either turns the same
// registration on; neither turns the other off.
func native(family string) bool {
	if os.Getenv("TESTKIT_NATIVE_"+strings.ToUpper(family)) == "1" {
		return true
	}
	for _, f := range strings.Split(os.Getenv("TESTKIT_NATIVE"), ",") {
		switch strings.ToLower(strings.TrimSpace(f)) {
		case family, "all":
			return true
		}
	}
	return false
}

// checkCasesCmd is `testkit check-cases <scenario> <init_path>`.
//
// Both paths are arguments rather than configuration. The corpus is often a
// checkout that no conf points at yet -- this is the check somebody runs on a
// branch of the cases before proposing it -- and the helper library is read
// rather than assumed for the reason ReadHelpers gives.
func checkCasesCmd(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: testkit check-cases <scenario> [<init_path>]")
		fmt.Fprintln(os.Stderr, "  init_path defaults to $CTP_HOME/shell/init_path")
		return 2
	}
	scenario := args[0]
	initPath := ""
	if len(args) > 1 {
		initPath = args[1]
	} else if home := os.Getenv("CTP_HOME"); home != "" {
		initPath = filepath.Join(home, "shell", "init_path")
	}
	if initPath == "" {
		fmt.Fprintln(os.Stderr, "testkit check-cases: no helper library. Pass one, or set CTP_HOME")
		return 2
	}
	h, err := casecheck.ReadHelpers(initPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "testkit check-cases: %v\n", err)
		return exitEnvironment
	}
	rep, err := casecheck.Walk(scenario, h)
	if err != nil {
		fmt.Fprintf(os.Stderr, "testkit check-cases: %v\n", err)
		return exitEnvironment
	}
	for _, f := range rep.Findings {
		fmt.Println(f)
	}
	fmt.Printf("\n%d case(s) read against %d helper(s), %d of which can report a failure\n",
		rep.Cases, len(h.All), len(h.Failing))
	fmt.Printf("cannot fail: %d   misspelt verdict: %d   self-comparison: %d\n",
		rep.Count(casecheck.RuleCannotFail),
		rep.Count(casecheck.RuleMisspeltVerdict),
		rep.Count(casecheck.RuleSelfComparison))
	// A finding is a defect in a case, so the command that found one says so in
	// its status: this is meant to be run from CI over a corpus branch.
	if len(rep.Findings) > 0 {
		return 1
	}
	return 0
}
