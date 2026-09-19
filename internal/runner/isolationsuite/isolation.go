// Package isolationsuite runs the isolation task: CTP's isolation module, with
// every case still executed by runone.sh and ctltool exactly as CTP ships them
// (ADR-007), and everything around a case -- discovery, the queue, slots, the
// verdict and every record -- here.
//
// docs/project/design/module-isolation.md.
package isolationsuite

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/cli"
	"github.com/cubrid-systems/cubrid-testkit/internal/contain"
	"github.com/cubrid-systems/cubrid-testkit/internal/dispatch"
	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
	"github.com/cubrid-systems/cubrid-testkit/internal/feedback"
	"github.com/cubrid-systems/cubrid-testkit/internal/patch"
	"github.com/cubrid-systems/cubrid-testkit/internal/result"
	"github.com/cubrid-systems/cubrid-testkit/internal/runner"
	"github.com/cubrid-systems/cubrid-testkit/internal/runner/shellsuite"
	"github.com/cubrid-systems/cubrid-testkit/internal/sizing"
	"github.com/cubrid-systems/cubrid-testkit/internal/topology"
)

// Isolation runs the isolation task.
type Isolation struct{}

// New returns the runner for the isolation task.
func New() *Isolation { return &Isolation{} }

func (*Isolation) Tasks() []cli.Task { return []cli.Task{cli.Isolation} }

// exitEnvironment is what CTP's System.exit(-1) becomes: a run that could not
// start, as opposed to a run that found failures.
const exitEnvironment = 255

func quit(format string, args ...any) error {
	fmt.Printf("[ERROR]: "+format+"\n", args...)
	fmt.Println("QUIT")
	return &runner.ExitError{Code: exitEnvironment, Err: fmt.Errorf(format, args...)}
}

// resultCategory is where the run directory goes: result/isolation, whatever
// test_category says. Context calls setLogDir("isolation") and reads the key only
// for what feedback prints.
const resultCategory = "isolation"

// Validate refuses what cannot be run here.
func (*Isolation) Validate(req runner.Request) error {
	cfg, err := req.Home.Load(req.ConfigPath)
	if err != nil {
		return quit("cannot read the configuration %s: %v", req.ConfigPath, err)
	}
	if strings.TrimSpace(cfg.GetOr("scenario", "")) == "" {
		return quit("The parameter 'scenario' must be set correctly in %s !", req.ConfigPath)
	}
	if cfg.Bool("testcase_update_yn", false) {
		return quit("testcase_update_yn=yes asks for a case update, which this runner does not do " +
			"(docs/project/concept/migration-exclusions.md). Update the corpus separately, then run with " +
			"testcase_update_yn=no")
	}
	// Not an option, even for one slot. runone.sh kills by user -- every cub,
	// sleep, qactl and qacsql the user owns -- so on a machine where the same user
	// does anything else, an uncontained run kills it (ADR-007).
	if !contain.Active() {
		return quit("isolation cases run in namespaces of their own: runone.sh kills every cub, sleep, "+
			"qactl and qacsql the user owns (docs/project/adr/ADR-007-isolation-executor.md). Set %s=1, "+
			"or run the task through CTP", contain.Env)
	}
	return nil
}

// Run is Main.exec and TestFactory.execute.
func (r *Isolation) Run(ctx context.Context, req runner.Request) error {
	cfg, err := req.Home.Load(req.ConfigPath)
	if err != nil {
		return quit("cannot read the configuration %s: %v", req.ConfigPath, err)
	}
	category := cfg.GetOr("test_category", "isolation")
	continueMode := cfg.Bool("test_continue_yn", false)

	configured, err := topology.From(cfg)
	if err != nil {
		return quit("%v", err)
	}
	if len(configured) > 0 {
		names := make([]string, 0, len(configured))
		for _, inst := range configured {
			names = append(names, inst.EnvID())
		}
		return quit("this runner runs isolation on the machine it is started on, and the configuration names %s. "+
			"Run it there, or run the task through CTP", strings.Join(names, ", "))
	}
	machine := topology.Local(cfg)
	envID := machine.EnvID()
	fmt.Printf("Available Env: [%s]\n", envID)

	if url := strings.TrimSpace(cfg.GetOr("cubrid_download_url", "")); url != "" {
		fmt.Printf("[WARN] cubrid_download_url is set but installing builds is not this runner's job "+
			"(docs/project/concept/migration-exclusions.md 1-4a). Testing the build that is installed; "+
			"compare the build number below against %s\n", url)
	}

	here := &exec.Local{}
	info, err := run(ctx, here, shellScript(versionCommand))
	buildID := shellsuite.BuildID(output(info))
	if err != nil || buildID == "" {
		return quit("Please confirm your build installation for local test!")
	}
	bits := shellsuite.BuildBits(output(info))

	scenario, err := resolveScenario(ctx, here, strings.TrimSpace(cfg.GetOr("scenario", "")))
	if err != nil {
		// CTP printed this and returned, so the task ends with 0.
		fmt.Println("[ERROR]" + err.Error())
		return nil
	}
	fmt.Printf("Build Id: %s\n", buildID)
	fmt.Printf("Build Bits: %s\n", bits)

	sink, err := result.Open(req.Home, resultCategory, continueMode)
	if err != nil {
		return err
	}
	defer sink.Close()
	if !continueMode {
		// TestFactory.execute empties the run directory first.
		if err := emptyDir(sink.Dir()); err != nil {
			return err
		}
	}

	var report feedback.Feedback = feedback.Null{}
	switch t := strings.TrimSpace(cfg.GetOr("feedback_type", "file")); {
	case strings.EqualFold(t, "file"), strings.EqualFold(t, "database"):
		if strings.EqualFold(t, "database") {
			fmt.Printf("[WARN] feedback_type=%s asks for a database this runner does not write to; "+
				"the events are kept in %s instead\n", t, sink.Dir())
		}
		f, err := feedback.OpenIsolation(sink.Dir(), category, os.Stdout, continueMode)
		if err != nil {
			return err
		}
		defer f.Close()
		report = f
	default:
		fmt.Printf("[WARN] feedback_type=%s is not a backend, so this run keeps no feedback\n", t)
	}

	snapshot := func() error {
		return sink.Snapshot(cfg, map[string]string{"AUTO_BUILD_ID": buildID, "AUTO_BUILD_BITS": bits})
	}
	check := func() error {
		fmt.Println("BEGIN TO CHECK: ")
		log, err := sink.Check(envID)
		if err != nil {
			return err
		}
		c := &checker{ch: here, title: envID, out: os.Stdout, log: log,
			logPath: filepath.Join(sink.Dir(), "check_"+envID+".log")}
		if !c.check(ctx, scenario) {
			fmt.Println("CHECK RESULT: FAIL")
			fmt.Println("QUIT")
			return &runner.ExitError{Code: exitEnvironment, Err: errors.New("the machine check failed")}
		}
		fmt.Println("CHECK RESULT: PASS")
		return nil
	}
	// TestCaseGithub.update, less what it is for: it upgrades CTP (upgrade.sh, which
	// also prints the whole environment to standard output) and pulls the cases,
	// and both are excluded. The one thing in it a run depends on -- the execute
	// bit on ctltool's scripts -- is done where each slot is prepared (deploy).
	updated := func() {
		fmt.Println("============= UPDATE TEST CASES ==================")
		fmt.Println("SKIP TEST CASE UPDATE!")
		fmt.Println("DONE")
	}

	var cases, skipped []string
	if continueMode {
		if err := snapshot(); err != nil {
			return err
		}
		report.TaskContinue()
		fmt.Println("============= FETCH TEST CASES ==================")
		if cases, err = sink.Remaining(); err != nil {
			return err
		}
		if len(cases) == 0 {
			fmt.Println("NO TEST CASE TO TEST WITH CONTINUE MODE!")
			return nil
		}
		if err := check(); err != nil {
			return err
		}
		updated()
	} else {
		if err := snapshot(); err != nil {
			return err
		}
		if err := check(); err != nil {
			return err
		}
		report.TaskStart(cfg.GetOr("cubrid_download_url", ""))
		updated()
		fmt.Println("============= FETCH TEST CASES ==================")
		found, err := discover(ctx, here, scenario)
		if err != nil {
			return quit("%v", err)
		}
		var entries []string
		if file := strings.TrimSpace(cfg.GetOr("testcase_exclude_from_file", "")); file != "" {
			if entries, err = exclusions(ctx, here, file); err != nil {
				return quit("%v", err)
			}
		}
		cases, skipped = exclude(found, entries)
		for _, c := range skipped {
			fmt.Println("Excluded File: " + c)
		}
		if err := sink.All(cases); err != nil {
			return err
		}
		if len(cases) == 0 {
			fmt.Println("NO TEST CASE TO TEST!")
			return nil
		}
	}

	report.TotalTestCase(len(cases), 0, len(skipped))
	for _, c := range skipped {
		report.CaseStop(feedback.CaseStop{Case: c, Elapsed: -time.Millisecond, SkipType: feedback.SkipTypeByTemp})
	}
	fmt.Printf("The Number of Test Case : %d\n", len(cases))

	// ---- patches --------------------------------------------------------------
	// The corpus changes this run carries and does not own, as sql does it:
	// applied once before the first case and reverted at the end, never per case.
	// Two reasons, and either alone would decide it. The slots share the tree --
	// with scenario_disk each has a layer over it, and a patch applied after a
	// layer exists lands in one slot's upper directory instead of the tree every
	// slot reads. And a `.ctl` is read by the controller when the case starts, so
	// there is no moment inside a case at which patching it would mean anything.
	//
	// Before the deploy, so that a patch that will not apply costs the second it
	// takes to find out rather than the databases the deploy has already made.
	corpus := scenario
	if !filepath.IsAbs(corpus) {
		corpus = filepath.Join(os.Getenv("HOME"), corpus)
	}
	// A case is recorded relative to $HOME when the corpus is under it, which is
	// CTP's form and what the dispatch files keep (cases.go, resolveScenario). A
	// patch is found by where the file is, so the lookup is done on absolute
	// paths and the records keep the name they had.
	absCases := make([]string, len(cases))
	for i, c := range cases {
		absCases[i] = c
		if !filepath.IsAbs(c) {
			absCases[i] = filepath.Join(os.Getenv("HOME"), c)
		}
	}
	patches, err := patch.Load(cfg.GetOr("case_patch_dir", ""), corpus, ".ctl", absCases)
	if err != nil {
		return quit("%v", err)
	}
	for _, line := range patches.Describe() {
		fmt.Println(line)
	}
	if err := applyPatches(ctx, here, patches, absCases); err != nil {
		return quit("%v", err)
	}
	// Registered before the slots are opened, so it runs after they are closed:
	// reverting under a live overlay would change the tree a slot is reading.
	defer putPatchesBack(here, patches, absCases)
	// Beside the records it is about to write, rather than at the end: a run that
	// dies still says which of its verdicts are about a patched case.
	if err := patches.Report(sink.Dir()); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] cannot record what was patched: %v\n", err)
	}

	// ---- slots ----------------------------------------------------------------
	// Every slot gets ctltool's directory behind an overlay of its own: runone.sh
	// runs `make clean qactl qacsql` there, and keeps .test.log, runone.log and
	// timeout.log in it as its working directory. Two slots in one directory
	// would rebuild each other's binaries and read each other's attempts.
	//
	// The corpus goes behind one only with scenario_disk, as for shell: without
	// it runone.sh writes result/ and <name>.result into the cases tree, as under
	// CTP, which is also what a comparison with CTP reads.
	plan := slotsFor(cfg, scenario, len(cases), sizing.MemAvailableMB(), sizing.Load(sizing.Isolation))
	n := plan.Slots
	fmt.Fprintf(os.Stderr, "[INFO] %d slot(s): %s\n", n, plan.Why)
	// From before the slots open, so that the fall in available memory is theirs.
	meter := sizing.StartMeter(sampleEvery, nil)
	defer meter.Stop()
	ctltool := filepath.Join(req.Home.Path, "isolation", "ctltool")
	onDisk := cfg.Bool("scenario_disk", false)
	guards := guardsFor(os.Getenv("HOME"), os.Getenv("CUBRID"))
	if guards.cubridLog != "" {
		fmt.Fprintf(os.Stderr, "[INFO] %s is not the log of the install under test, and runone.sh empties it "+
			"before every case; each slot sees an empty directory there instead\n", guards.cubridLog)
	}
	slots, closeSlots, err := contain.OpenSlots(n, func(i int, s *contain.Slot) error {
		if err := s.NS.Overlay(ctltool, filepath.Join(s.Dir, "ctltool")); err != nil {
			return err
		}
		// The controller is testkit's only where it is asked for. ADR-019 is a
		// draft and its gate has not been run, so until then this is what
		// TESTKIT_NATIVE_SHELL was before ADR-013's: one switch, off.
		if wantOwnController() {
			if err := installController(ctx, s, ctltool); err != nil {
				return err
			}
		}
		if err := guards.mount(s); err != nil {
			return err
		}
		if onDisk {
			return s.NS.Overlay(corpus, filepath.Join(s.Dir, "scenario"))
		}
		return nil
	})
	if err != nil {
		return quit("%v", err)
	}
	closeSlots = sync.OnceFunc(closeSlots)
	defer closeSlots()
	if onDisk {
		fmt.Printf("[INFO] the corpus is read-only for this run; each slot's writes go to a layer of its own on disk\n")
	}

	// ---- deploy ---------------------------------------------------------------
	// In every slot, because the install each slot sees is its own overlay: the
	// line Deploy appends to cubrid.conf lands there and goes with the slot. The
	// worker log gets the first slot's account of it, which is what a serial run
	// writes.
	fmt.Println("============= DEPLOY ==================")
	logs := make([][]string, len(slots))
	errs := make([]error, len(slots))
	var wg sync.WaitGroup
	for i, s := range slots {
		wg.Add(1)
		go func() {
			defer wg.Done()
			logs[i], errs[i] = deploy(ctx, s.Channel(), machine, buildID)
		}()
	}
	wg.Wait()
	if err := sink.WorkerLines(envID, logs[0]); err != nil {
		return err
	}
	for i, err := range errs {
		if err != nil {
			return quit("%s: deploy: %v", slots[i].Label, err)
		}
	}
	fmt.Println("DONE")

	// ---- test -----------------------------------------------------------------
	fmt.Println("============= TEST ==================")
	queue := dispatch.New(cases, 0)
	opts := optionsOf(cfg)
	board, stopBoard := openBoard(cfg, cases, slots, onDisk, filepath.Join(sink.Dir(), "feedback.log"), buildID, plan.Why)
	defer stopBoard()
	// A verdict from patched source is a claim about the patched case and not
	// about the corpus, so every place the page shows a verdict shows that too.
	// The patches went in before the slots, so every one of them is known here.
	for i, c := range cases {
		if pf := patches.For(absCases[i]); pf != "" {
			board.Patched(c, pf)
		}
	}
	for i, s := range slots {
		w := &worker{slot: s.Label, envID: envID, ch: s.Channel(), queue: queue, sink: sink, report: report,
			opts: opts, board: board, meter: meter, ctltool: ctltool,
			crashes: map[string]bool{}, cores: map[string]bool{}, fatals: map[string]int{}}
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = w.run(ctx)
		}()
	}
	fmt.Println("STARTED")
	wg.Wait()

	report.TaskStop()
	fmt.Println("TEST COMPLETE")
	if !continueMode && ctx.Err() == nil && errors.Join(errs...) == nil {
		record(meter, cfg, scenario, len(slots))
	}
	// Before the slots go, because their directories go with them.
	for _, s := range slots {
		kept, err := keepBackups(filepath.Join(s.Dir, "error_backup"), guards.errorBackup, s.Label)
		for _, k := range kept {
			fmt.Fprintf(os.Stderr, "[INFO] %s: core backup kept at %s\n", s.Label, k)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] %s: %v\n", s.Label, err)
		}
	}
	closeSlots()

	// CTP packs the run directory at the end, whatever the verdicts were. Failing
	// to pack it does not fail the run: the results are already on disk.
	if _, err := sink.BackupIsolation(buildID, bits, 0, time.Now()); err != nil {
		fmt.Printf("[ERROR] cannot pack the run directory: %v\n", err)
	}
	return errors.Join(errs...)
}

// emptyDir is CommonUtils.cleanFilesByDirectory for the run directory. The path
// is the sink's, derived from CTP_HOME, never a value from the configuration.
func emptyDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// wantOwnController reports whether this run executes cases with testkit's own
// controller instead of ctltool's qactl (ADR-019).
func wantOwnController() bool { return os.Getenv("TESTKIT_ISOLATION_CTL") == "1" }

// installController puts the controller into one slot's ctltool, which is an
// overlay of its own -- so the shim, the probe's source and the changed Makefile
// rule are the slot's, and go with it. It has to run inside the slot: from out
// here the same path is still ctltool's own directory.
func installController(ctx context.Context, s *contain.Slot, ctltool string) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("the controller cannot be installed: %w", err)
	}
	script := isolationScript(fmt.Sprintf("%s isolation-ctl-install %s || echo INSTALL_FAILED",
		shellQuote(self), shellQuote(ctltool)))
	res, err := run(ctx, s.Channel(), script)
	if err != nil {
		return fmt.Errorf("installing the controller in %s: %w", s.Label, err)
	}
	if out := output(res); strings.Contains(out, "INSTALL_FAILED") {
		return fmt.Errorf("installing the controller in %s:\n%s", s.Label, out)
	}
	return nil
}

// shellQuote is single quoting, for a path that goes into a script.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
