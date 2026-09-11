package shellsuite

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/cli"
	"github.com/cubrid-systems/cubrid-testkit/internal/conf"
	"github.com/cubrid-systems/cubrid-testkit/internal/contain"
	"github.com/cubrid-systems/cubrid-testkit/internal/dispatch"
	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
	"github.com/cubrid-systems/cubrid-testkit/internal/feedback"
	"github.com/cubrid-systems/cubrid-testkit/internal/plan"
	"github.com/cubrid-systems/cubrid-testkit/internal/result"
	"github.com/cubrid-systems/cubrid-testkit/internal/runner"
	"github.com/cubrid-systems/cubrid-testkit/internal/status"
	"github.com/cubrid-systems/cubrid-testkit/internal/topology"
)

// Shell runs the shell task, and rqg, which is the same machinery under a
// different category.
type Shell struct {
	// Channels opens the two channels the machine needs. It is a field because the
	// choice of machine is the one thing that varies between a local run and a
	// remote one, and because a test cannot be allowed to open the real ones: the
	// reset that runs before every case would kill the test.
	Channels func(*topology.Instance) (worker, monitor exec.Channel, err error)
}

// NewShell returns the runner for the shell and rqg tasks.
// NewShell leaves Channels nil on purpose. Run falls back to openChannels when
// it is, and "nil" is then what distinguishes the real opener from one a caller
// supplied -- which is what decides whether slots may be opened underneath it.
func NewShell() *Shell { return &Shell{} }

func (s *Shell) Tasks() []cli.Task { return []cli.Task{cli.Shell, cli.RQG} }

// exitEnvironment is what CTP's System.exit(-1) becomes: a run that could not
// start, as opposed to a run that found failures.
const exitEnvironment = 255

func quit(format string, args ...any) error {
	fmt.Printf("[ERROR]: "+format+"\n", args...)
	fmt.Println("QUIT")
	return &runner.ExitError{Code: exitEnvironment, Err: fmt.Errorf(format, args...)}
}

// Validate refuses a configuration that cannot describe a run, and refuses one
// that asks for something excluded on the axis split.
func (s *Shell) Validate(req runner.Request) error {
	if req.Config == nil {
		return quit("no configuration file")
	}
	if strings.TrimSpace(req.Config.GetOr("scenario", "")) == "" {
		return quit("The parameter 'scenario' must be set correctly in %s !", req.ConfigPath)
	}

	if _, err := topology.From(req.Config); err != nil {
		return quit("%v", err)
	}

	// Updating the case corpus is an operations decision and is not done here.
	// Asked for and silently skipped, it would mean passing on stale cases and
	// believing the result -- so it fails instead.
	if req.Config.Bool("testcase_update_yn", false) {
		return quit("testcase_update_yn=yes asks for a case update, which this runner does not do " +
			"(docs/concept/migration-exclusions.md). Update the corpus separately, then run with " +
			"testcase_update_yn=no")
	}
	return nil
}

// Run is the whole task: work out what to test, put the machines in a known
// state, hand the cases out, and say what happened.
func (s *Shell) Run(ctx context.Context, req runner.Request) error {
	cfg := req.Config
	category := categoryFor(req, string(req.Task))
	continueMode := cfg.Bool("test_continue_yn", false)

	configured, err := topology.From(cfg)
	if err != nil {
		return quit("%v", err)
	}
	machine, extra := oneMachine(cfg, configured)
	local := machine.IsLocal()

	// CTP printed the whole instance list here. It still prints what the
	// configuration named, so a run that mentions eight machines does not quietly
	// look like a run that mentioned one.
	names := make([]string, 0, len(configured))
	for _, inst := range configured {
		names = append(names, inst.EnvID())
	}
	if len(names) == 0 {
		names = []string{machine.EnvID()}
	}
	fmt.Printf("Available Env: [%s]\n", strings.Join(names, ", "))
	fmt.Printf("Continue Mode: %v\n", continueMode)

	for _, name := range extra {
		fmt.Printf("[WARN] %s is configured but will not be used: this runner runs on one machine "+
			"(ADR-014). Running everything on %s; to use %s as well, run a second runner against it\n",
			name, machine.EnvID(), name)
	}

	if url := strings.TrimSpace(cfg.GetOr("cubrid_download_url", "")); url != "" {
		fmt.Printf("[WARN] cubrid_download_url is set but installing builds is not this runner's job "+
			"(docs/concept/migration-exclusions.md 1-4a). Testing the build that is installed; "+
			"compare the build number below against %s\n", url)
	}

	sink, err := result.Open(req.Home, category, continueMode)
	if err != nil {
		return err
	}
	defer sink.Close()

	report, err := feedback.Open(sink.Dir(), category, os.Stdout, continueMode, os.Getenv("MSG_ID"))
	if err != nil {
		return err
	}
	defer report.Close()

	opener := s.Channels
	if opener == nil {
		opener = openChannels
	}
	worker, monitor, err := opener(machine)
	if err != nil {
		return quit("%v", err)
	}
	defer worker.Close()
	defer monitor.Close()

	// One slot is the path there has always been: the pair opened above, used
	// exactly as it always was, with no namespace anywhere.
	//
	// More than one and *every* slot needs a namespace, the first included.
	// Leaving the first outside looks like it preserves the old behaviour and
	// does the opposite: the reset matches process names with `ps -e`, and a
	// worker outside the namespaces sees into all of them, so its sweep kills
	// every other slot's server. Measured -- four slots produced a `ps -e -f`
	// listing three keepers, two other slots' cases and another slot's
	// cub_server, and then killed them.
	// The corpus goes behind an overlay whose upper layer is memory, before the
	// slots exist so that they inherit one rather than each mounting its own.
	//
	// It answers two things with one mechanism. A case creates its database in
	// its own directory and not all of them delete it, and nothing puts the tree
	// back -- 105 MB in the repository is 20 GB on this machine, and a case that
	// finds a database it did not create behaves differently from one that does
	// not. And the writes are the run's bottleneck: at eight slots the wall clock
	// stopped improving, because 217 cases at 708 MB each is 150 GB against a
	// disk that writes 332 MB/s.
	//
	// Clean is then structural rather than a step. 105 MB is cheap to copy, but a
	// copy is something that can be skipped and a mount is a property of how the
	// run is mounted.

	// Lanes need three things known before a slot exists: the durations, the
	// slot count, and whether the corpus is behind an overlay at all -- so the
	// plan is read here rather than where the case list arrives.
	planPath := strings.TrimSpace(cfg.GetOr("case_plan", ""))
	var known map[string]time.Duration
	if planPath != "" {
		k, err := plan.Read(planPath)
		if err != nil {
			return quit("cannot read case_plan %s: %v", planPath, err)
		}
		known = k
	}
	// Seeded with what is already known, so an interrupted run refreshes the
	// cases it reached instead of forgetting the ones it did not.
	record := plan.Continue(known)

	// Footprints are the other half of what the corpus costs, and they live in
	// their own file because they are keyed by directory rather than by case: a
	// directory is what a slot owns, what a reclaim drops, and what a lane holds.
	sizePath := strings.TrimSpace(cfg.GetOr("case_sizes", ""))
	var heldMB map[string]int
	if sizePath != "" {
		h, err := plan.ReadSizes(sizePath)
		if err != nil {
			return quit("cannot read case_sizes %s: %v", sizePath, err)
		}
		heldMB = h
	}
	sizes := plan.ContinueSizes(heldMB)

	slowSecs := cfg.Int("lane_slow_secs", 0)
	slowMB := cfg.Int("lane_slow_mb", 0)
	slowMBps := cfg.Int("lane_slow_mbps", 0)
	lanes := slowSecs > 0 || slowMB > 0 || slowMBps > 0
	slots := cfg.Int("parallel_slots", 1)
	ramMB := cfg.Int("scenario_ram_mb", 0)
	if lanes && ramMB <= 0 {
		return quit("lanes need scenario_ram_mb: a fast lane is a lane whose writes go to memory")
	}
	if slowMB > 0 && sizePath == "" {
		return quit("lane_slow_mb needs case_sizes: the footprints it thresholds are measured by a run, not guessed")
	}
	if slowMBps > 0 && (sizePath == "" || planPath == "") {
		return quit("lane_slow_mbps needs case_sizes and case_plan: a rate is megabytes over seconds and both halves are measured by a run, not guessed")
	}
	onDisk := cfg.Bool("scenario_disk", false)
	if err := checkScenarioDisk(onDisk, ramMB, cfg.GetOr("scenario", ""), cfg.GetOr("testcase_workspace_dir", "")); err != nil {
		return quit("%v", err)
	}

	var corpus *Corpus
	var split laneSplit
	if ramMB > 0 {
		if lanes {
			// Decided here, before a slot exists, because a slot's lane decides
			// where its overlay's upper layer goes. The plan's own keys are the
			// case list it needs: a plan is the previous run's case list with a
			// duration against each entry, so no discovery is required to know
			// which directories are slow.
			measured := make([]string, 0, len(known))
			for c := range known {
				measured = append(measured, c)
			}
			sp, err := planLanes(measured, known, heldMB, slowSecs, slowMB, slowMBps, slots)
			if err != nil {
				return quit("%v", err)
			}
			if !sp.on() {
				return quit("no directory reaches the lane threshold; nothing would run in the slow lane")
			}
			split = sp
		}
		c, err := OpenCorpus(cfg.GetOr("scenario", ""), ramMB, lanes)
		if err != nil {
			return quit("%v", err)
		}
		corpus = c
		defer corpus.Close()
		if lanes {
			fmt.Printf("[INFO] the corpus is read-only for this run; the fast lane's writes go to %d MB of memory and the slow lane's to disk\n", ramMB)
		} else {
			fmt.Printf("[INFO] the corpus is read-only for this run; its writes go to %d MB of memory\n", ramMB)
		}
	}
	if onDisk {
		how := ""
		if contain.Volatile() {
			how = ", mounted volatile"
		}
		fmt.Printf("[INFO] the corpus is read-only for this run; each slot's writes go to a layer of its own on disk%s\n", how)
	}

	// Every worker gets a namespace, including the only one.
	//
	// This used to start at two, and one worker ran on the bare machine. That
	// made a serial run *less* isolated than a parallel one, which is backwards
	// and it cost a measurement: three _01_sqlx cases were recorded at 182, 183
	// and 183 seconds in a serial arm and are 8.8, 12.4 and 38.4 on develop.
	// With no network namespace the run shared port 1523 with leftover masters
	// and another session's server, and three cases at almost exactly the same
	// number looked like a fixed timeout and were contention.
	//
	// A network namespace is most of what this buys a single worker: the run
	// stops competing for the shipped ports with whatever else is on the
	// machine. The private /dev/shm, the per-slot $CUBRID overlay and the short
	// CUBRID_TMP come with it, and they are the same arrangement a two-slot run
	// has been verified against -- 0 verdicts differ, measured.
	// A caller that supplies its own channels is controlling how commands run,
	// and slots would build their own and ignore it -- which is how the whole-task
	// test lost the guard that intercepts the destructive reset. Slots are for the
	// real opener.
	// Slots build their own channels and would ignore an injected one, so they
	// are only for the real opener. Setting Channels is how a caller says it is
	// controlling how commands run -- the whole-task test does it to intercept
	// the destructive reset.
	own := s.Channels == nil
	pairs := []channelPair{{worker: worker, monitor: monitor, close: func() {}}}
	if own && (slots > 1 || contain.Active()) {
		slotted, closeSlots, err := openSlots(slots, corpus, cfg.GetOr("scenario", ""), split, onDisk)
		if err != nil {
			return quit("%v", err)
		}
		defer closeSlots()
		pairs = slotted
	}

	buildInfo, err := runIn(ctx, worker, versionScript)
	if err != nil {
		return quit("Please confirm your build installation for local test! (%v)", err)
	}
	buildID := BuildID(buildInfo.Output())
	if buildID == "" {
		return quit("Please confirm your build installation for local test!")
	}
	bits := BuildBits(buildInfo.Output())
	fmt.Printf("Build Number: %s\n", buildID)

	if err := sink.Snapshot(cfg, map[string]string{
		"AUTO_TEST_VERSION": buildID,
		"AUTO_TEST_BITS":    bits,
	}); err != nil {
		return err
	}

	if continueMode {
		report.TaskContinue()
	} else {
		report.TaskStart(cfg.GetOr("cubrid_download_url", ""))
	}

	// ---- requirements ----------------------------------------------------
	// A machine that cannot run cases should say so once, here, rather than
	// through 3,452 identical failures.
	envID := machine.EnvID()
	checkLog, err := sink.Check(envID)
	if err != nil {
		return err
	}
	title := envID
	if !machine.IsLocal() {
		title = machine.SSH().User + "@" + machine.SSH().Host + ":" + machine.SSH().Port
	}
	(&CheckRequirement{
		EnvID:       envID,
		Title:       title,
		Protocol:    cfg.GetOr("service_protocol_type", "ssh"),
		Channel:     worker,
		Scenario:    strings.TrimSpace(cfg.GetOr("scenario", "")),
		ExcludeFile: strings.TrimSpace(cfg.GetOr("testcase_exclude_from_file", "")),
		Out:         os.Stdout,
		Log:         checkLog,
	}).Check(ctx)

	// CTP's Log constructor creates its file whether or not anything is ever
	// written to it, so every run leaves a monitor_<envId>.log behind -- usually
	// empty, always present.
	if err := sink.Monitor(envID, ""); err != nil {
		return err
	}

	// ---- workspace -------------------------------------------------------
	fmt.Println("============= UPDATE TEST CASES ==================")
	workspace, err := s.prepareWorkspace(ctx, worker, cfg, local)
	if err != nil {
		return quit("%v", err)
	}
	fmt.Println("DONE")

	// ---- case list -------------------------------------------------------
	fmt.Println("============= FETCH TEST CASES ==================")
	cases, macroSkipped, tempSkipped, err := s.caseList(ctx, worker, sink, cfg, workspace, continueMode)
	if err != nil {
		return quit("%v", err)
	}
	if len(cases) == 0 {
		if continueMode {
			fmt.Println("NO TEST CASE TO TEST WITH CONTINUE MODE!")
		} else {
			fmt.Println("NO TEST CASE TO TEST!")
		}
		return nil
	}

	report.TotalTestCase(len(cases), len(macroSkipped), len(tempSkipped))
	if !continueMode {
		s.recordSkipped(report, macroSkipped, feedback.SkipTypeByMacro)
		s.recordSkipped(report, tempSkipped, feedback.SkipTypeByTemp)
	}
	fmt.Printf("The Number of Test Case : %d\n", len(cases))
	// What to reclaim, and when: a directory's writes go when its last case
	// retires, which is what keeps the ceiling a size instead of a rate.
	corpus.Plan(cases)

	// ---- order ------------------------------------------------------------
	// Longest case first, from what a previous run on this machine measured.
	// Off unless case_plan names a file, because it changes the order cases are
	// handed out in -- and a run that did not ask for that keeps the corpus
	// order it has always had.
	if planPath != "" {
		if len(known) > 0 {
			cases = plan.Order(cases, known)
			fmt.Printf("[INFO] cases ordered longest-first from %s (%d of %d measured)\n",
				planPath, len(known), len(cases))
		} else {
			fmt.Printf("[INFO] no durations in %s yet; this run will write them\n", planPath)
		}
	}

	// ---- deploy ----------------------------------------------------------
	fmt.Println("============= DEPLOY ==================")
	if err := s.deploy(ctx, machine, worker, sink); err != nil {
		return quit("%v", err)
	}
	fmt.Println("DONE")

	// ---- test ------------------------------------------------------------
	fmt.Println("============= TEST ==================")
	queue := dispatch.New(cases, cfg.Int("testcase_retry_num", 0))

	// The order proposes and a policy disposes -- see internal/dispatch/policy.go.
	//
	// Two rules, answering different halves of the same failure. At 24 slots the
	// corpus tmpfs went from empty to 25,584 MB of 25,600 in under three minutes
	// with zero cases finished: the order puts the heaviest cases first, so every
	// slot started one at once. A ceiling rule cannot prevent that -- the ceiling
	// is empty at the moment it happens -- so the cap on concurrent heavy cases
	// exists for the start, and the ceiling rule for everything after it.
	//
	// Both are defaults with knobs rather than settings with defaults: a run that
	// has the data should be protected without being asked.
	heavyMax := cfg.Int("heavy_in_flight_max", (slots+3)/4)
	highWater := cfg.Int("scenario_ram_high_water", 80)
	var heavy []string
	if len(known) > 0 && heavyMax > 0 {
		secs := make(map[string]float64, len(known))
		for c, d := range known {
			secs[c] = d.Seconds()
		}
		// The heaviest tenth: enough to cover the head of the queue, few enough
		// that the cap does not hold back the ordinary long cases.
		heavy = dispatch.Heaviest(secs, len(known)/10)
	}
	// The ceiling is a constraint: nothing may cross it. The heavy cap is a
	// preference about order: it stops N slots starting N heavy cases at once,
	// and it must not be allowed to shape the tail, where taking a heavy case is
	// the only thing left to do.
	// The machine has to be able to hold the whole ceiling, not the gate.
	//
	// The gate stops *new* cases at highWater percent of scenario_ram_mb; the
	// cases already running are free to fill the rest, and they do -- a 24-slot
	// run with the gate at 80% finished with 22,528 MB of its 22,528 MB ceiling
	// in use. So the number to compare against the machine is the ceiling
	// itself, plus what the run's own processes take, and the first version of
	// this check compared the gate instead. It stayed silent for a run that
	// systemd-oomd then killed at 2,951 of 3,204 cases, with nothing in the
	// run's own log to say why.
	//
	// perSlotMB is the floor a server measures at this suite's settings -- 157
	// MB that no parameter reaches, plus a quarter of the buffers. It is a
	// reserve rather than a prediction: a case's own working set is larger and
	// is not knowable here. tools/sizing.sh computes the figure properly.
	const perSlotMB = 175
	if _, limit := corpus.Usage(); limit > 0 {
		reserve := slots * perSlotMB
		if avail := memAvailableMB(); avail > 0 && limit+reserve > avail {
			fmt.Printf("[WARN] scenario_ram_mb=%d plus about %d MB for %d slots' servers is more "+
				"than the %d MB this machine has available.\n", limit, reserve, slots, avail)
			fmt.Printf("[WARN]   The gate at %d%% holds back new cases only; the ones already "+
				"running can fill the ceiling, and a run that reaches it meets the OOM killer "+
				"rather than the gate.\n", highWater)
			if room := avail - reserve; room > 0 {
				fmt.Printf("[WARN]   A ceiling at or below %d MB fits. tools/sizing.sh sizes it "+
					"from the machine.\n", room)
			}
		}
	}
	hard := dispatch.NewHeadroom("the corpus tmpfs", corpus.Usage, highWater)
	soft := dispatch.NewHeavyCap(heavy, heavyMax)
	if hard != nil || soft != nil {
		queue.Policy(hard, soft)
		fmt.Println("[INFO] admission: " + dispatch.Describe(hard, soft))
	}
	if split.on() {
		// A directory this corpus has and the plan did not mention takes the
		// fast lane, which planLanes already decided by omission.
		queue.Assign(split.byDir)
		fmt.Println(split.describe(slowSecs, slowMB))
		for _, line := range split.slowest(known, heldMB, cases, 5) {
			fmt.Printf("[INFO]   slow lane: %s\n", line)
		}
	}

	// The page is off unless a port is named. It is not on standard output on
	// purpose: what the runner prints there is frozen (ADR-003) and the
	// comparison reads it, so a screen drawn over it would be drawn over the
	// evidence.
	var board *status.Board
	if addr := status.Addr(cfg.GetOr("status_http", "")); addr != "" {
		board = status.New(len(cases))
		// The page's "remaining" is a guess unless the run has a plan, and with the
		// longest cases first the guess opens at its worst: two cases into this
		// corpus the rate said 82 hours where the plan says 2.2.
		board.Expect(cases, known, slots)
		// A second run on the same machine would find the default port taken, and
		// killing the run over the page it was only asked to serve is the wrong
		// trade -- so the default moves along until it finds a free one and says
		// where it landed. An address the operator pinned is not moved: they
		// asked for that one.
		where, stop, err := board.Serve(addr)
		if err != nil && addr == status.DefaultAddr {
			for try := 1; try <= 16 && err != nil; try++ {
				where, stop, err = board.Serve(status.NearDefault(try))
			}
		}
		if err != nil {
			return quit("%v", err)
		}
		defer stop()
		fmt.Printf("[INFO] status page at http://%s/\n", where)
		// The machine panel reports the two places that matter to this run
		// rather than the root filesystem.
		board.Watch(os.Getenv("CUBRID"), corpus.Ram(), cfg.Int("scenario_ram_mb", 0))
		// Where the page finds what a case actually did. It is the file the run
		// is already writing, so a click costs a scan and nothing is recorded
		// twice.
		board.Detail(filepath.Join(sink.Dir(), "feedback.log"))
		// What the run was told to do, which the verdicts do not say and which
		// changes what they mean: an engine default that is not the engine's, and
		// switches that decide how faithful the run is.
		board.Setup(describeSetup(cfg, slots, ramMB, slowSecs, slowMB, planPath, sizePath))
		// The template cache is CTP's, turned on with an environment variable and
		// keeping its own store, so the page reads that store rather than asking
		// the shell to report. Off unless the run asked for a cache, and then the
		// panel is absent rather than empty.
		if os.Getenv("CTP_DB_TEMPLATE_CACHE") == "1" {
			board.WatchTemplates(templateStore(), templateCapMB())
		}
	}

	// The registry is inherited, and a run that inherits state it did not make is
	// a run whose failures are not all its own. Said before the first case rather
	// than found afterwards in a case log.
	if stale := staleDatabases(cfg.GetOr("scenario", "")); len(stale) > 0 {
		fmt.Printf("[WARN] $CUBRID_DATABASES holds %d database(s) from an earlier run. "+
			"Anything that walks databases.txt -- make_tz -g extend, for one -- will try to use them:\n", len(stale))
		for i, s := range stale {
			if i == 5 {
				fmt.Printf("[WARN]   ... and %d more\n", len(stale)-5)
				break
			}
			fmt.Printf("[WARN]   %s\n", s)
		}
	}

	// A corpus changed before it ran is the first thing a reader of the verdicts
	// has to know, so it is said here and repeated per case and on the page.
	patches, perr := LoadPatches(cfg.GetOr("case_patch_dir", ""), cfg.GetOr("scenario", ""), cases)
	if perr != nil {
		return quit("%v", perr)
	}
	for _, line := range patches.Describe() {
		fmt.Println(line)
	}

	// What a case leaves behind, kept where a later reader can find it. Off by
	// default: it is new, and a run that has never asked for it should not start
	// writing megabytes it did not ask for.
	logs, lerr := NewCaseLogs(sink.Dir(), cfg.GetOr("scenario", ""), cfg.GetOr("case_logs", ""), cfg.Int("case_logs_max_mb", 0))
	if lerr != nil {
		return quit("%v", lerr)
	}
	if logs != nil {
		fmt.Printf("[INFO] case logs under %s\n", logs.Dir())
	}

	fmt.Println("STARTED")
	err = s.test(ctx, machine, pairs, queue, sink, report, cfg, buildID, bits, local, board, corpus, record, split, patches, logs)

	if planPath != "" {
		if werr := record.Write(planPath); werr != nil {
			fmt.Printf("[ERROR] cannot write case_plan %s: %v\n", planPath, werr)
		}
	}
	if line := logs.Summary(); line != "" {
		fmt.Println(line)
	}
	// A patch that matched and never ran leaves the verdicts describing the
	// unpatched corpus while the run's opening line says otherwise. Say which.
	if missed := patches.Unapplied(); len(missed) > 0 {
		fmt.Printf("[ERROR] %d of %d case(s) matched a compatibility patch and did not run against one. "+
			"Their verdicts are about the corpus as it is, not as the patch leaves it.\n",
			len(missed), patches.Count())
		for i, c := range missed {
			if i == 10 {
				fmt.Printf("[ERROR]   ... and %d more\n", len(missed)-i)
				break
			}
			fmt.Println("[ERROR]   " + c)
		}
	}
	if werr := patches.Report(sink.Dir()); werr != nil {
		fmt.Printf("[ERROR] cannot record which cases were patched: %v\n", werr)
	}
	if sizePath != "" {
		sizes.Merge(corpus.Held())
		if werr := sizes.Write(sizePath); werr != nil {
			fmt.Printf("[ERROR] cannot write case_sizes %s: %v\n", sizePath, werr)
		}
	}

	report.TaskStop()

	// CTP packs the run directory unconditionally, at the end, whatever the
	// verdicts were. Failing to pack it does not fail the run: the results are
	// already on disk, and the archive is a convenience for carrying them off.
	if _, backupErr := sink.Backup(buildID, bits, 0, time.Now()); backupErr != nil {
		fmt.Printf("[ERROR] cannot pack the run directory: %v\n", backupErr)
	}

	fmt.Println("TEST COMPLETE")
	return err
}

// oneMachine picks the machine this run uses, and names the ones it will not.
//
// A configuration that names none describes this machine, which is what CTP did
// too. A configuration that names several describes a fleet, and a fleet belongs
// to the operations layer -- so the first is used and the rest are reported.
// Leaving them out costs throughput, not correctness, which is why this warns
// instead of failing (ADR-014, migration-exclusions.md 2a).
func oneMachine(cfg *conf.Config, configured []*topology.Instance) (*topology.Instance, []string) {
	if len(configured) == 0 {
		return topology.Local(cfg), nil
	}
	var extra []string
	for _, inst := range configured[1:] {
		extra = append(extra, inst.EnvID())
	}
	return configured[0], extra
}

// openSlots opens n places to run a case, each in namespaces of its own -- see
// contain.OpenSlots -- and gives each the two channels a worker needs.
//
// With lanes, the corpus overlay is each slot's rather than the run's, and
// where its upper layer sits is what the lane means: memory for the fast lane,
// disk for the slow one. Mounted inside the slot, because mounted once before
// the slots there is only one place for it to be.
//
// With scenario_disk every slot has one, its upper in the slot's own directory
// on the slot root, so that TESTKIT_SLOT_VOLATILE reaches what a shell case
// writes most: the databases it creates in its own directory. Without an
// overlay a case writes them into the corpus itself, as under CTP, and no
// switch on the slots can touch that.
func openSlots(n int, corpus *Corpus, corpusDir string, split laneSplit, onDisk bool) ([]channelPair, func(), error) {
	var mount func(int, *contain.Slot) error
	if onDisk {
		mount = func(i int, s *contain.Slot) error {
			return s.NS.Overlay(corpusDir, filepath.Join(s.Dir, "scenario"))
		}
	}
	if split.on() {
		mount = func(i int, s *contain.Slot) error {
			if corpus == nil {
				return fmt.Errorf("lanes need a corpus overlay; scenario_ram_mb is unset")
			}
			onRAM := split.laneOf(i) == dispatch.LaneFast
			upperRoot, err := corpus.Slot(s.Label, onRAM, func(script string) error {
				out, err := s.NS.Channel("").Run(context.Background(), script)
				if err != nil {
					return fmt.Errorf("%s: %w: %s", s.Label, err, strings.TrimSpace(out.Output()))
				}
				return nil
			})
			if err != nil {
				return err
			}
			return s.NS.Overlay(corpusDir, upperRoot)
		}
	}
	slots, closeAll, err := contain.OpenSlots(n, mount)
	if err != nil {
		return nil, nil, err
	}
	pairs := make([]channelPair, len(slots))
	for i, s := range slots {
		// The monitor needs a channel of its own into the same namespace: it has
		// to reach the machine while the case is holding the worker's.
		pairs[i] = channelPair{worker: s.Channel(), monitor: s.Channel(), close: func() {}}
	}
	return pairs, closeAll, nil
}

// checkScenarioDisk refuses what scenario_disk cannot do.
//
// It is one of two places for the corpus's writes, scenario_ram_mb being the
// other. It is mounted per slot, so it needs slots, which need a contained
// runner. And it needs the cases to run in scenario: a separate workspace is
// filled from outside after the slots have mounted their overlays over
// scenario, and the cases would run there instead, on no overlay at all.
func checkScenarioDisk(on bool, ramMB int, scenario, workspace string) error {
	if !on {
		return nil
	}
	scenario, workspace = strings.TrimSpace(scenario), strings.TrimSpace(workspace)
	switch {
	case ramMB > 0:
		return fmt.Errorf("scenario_disk and scenario_ram_mb are two places for the corpus's writes; set one of them")
	case scenario == "":
		return fmt.Errorf("scenario_disk needs scenario to be set")
	case workspace != "" && workspace != scenario:
		return fmt.Errorf("scenario_disk puts scenario behind an overlay, and the cases would run in testcase_workspace_dir instead; unset one of them")
	case !contain.Active():
		return fmt.Errorf("scenario_disk needs the runner contained; set %s=1", contain.Env)
	}
	return nil
}

// openChannels opens the two channels the machine needs.
//
// Two, not one. A worker spends most of its life blocked inside a case, and the
// timeout monitor has to reach the same machine while that is happening.
func openChannels(inst *topology.Instance) (worker, monitor exec.Channel, err error) {
	if inst.IsLocal() {
		// CTP reached even the local machine through SSHConnect, so a local run
		// gets the profile just as a remote one does.
		return &exec.Local{SourceProfile: true}, &exec.Local{SourceProfile: true}, nil
	}
	ssh := inst.SSH()
	if ssh.Host == "" {
		return nil, nil, fmt.Errorf("instance %s has no ssh.host", inst.EnvID())
	}
	open := func() exec.Channel {
		return exec.NewSSH(exec.SSHConfig{
			Host: ssh.Host, Port: ssh.Port, User: ssh.User, Password: ssh.Password,
		})
	}
	return open(), open(), nil
}

// prepareWorkspace materialises the case tree the run will read from.
//
// CTP did this inside TestCaseGithub.update, wrapped around a git pull. The pull
// is excluded, but the copy is not: cases dirty their own directories, and the
// dispatcher searches the workspace rather than the scenario. Without the copy
// there would be nothing to find.
func (s *Shell) prepareWorkspace(ctx context.Context, ch exec.Channel, cfg *conf.Config, local bool) (string, error) {
	scenario := strings.TrimSpace(cfg.GetOr("scenario", ""))
	workspace := strings.TrimSpace(cfg.GetOr("testcase_workspace_dir", ""))

	out, err := runIn(ctx, ch, KillScript(local, contain.Active()))
	if err != nil {
		return "", err
	}
	fmt.Println("CLEAN PROCESSES:")
	fmt.Println(out.Output())

	if workspace != "" && workspace != scenario {
		// This empties the workspace, so it is checked before it is a command
		// rather than after it has been one.
		if err := safeToEmpty("testcase_workspace_dir", workspace); err != nil {
			return "", err
		}
		w, sc := shQuote(workspace), shQuote(scenario)
		script := strings.Join([]string{
			"mkdir -p " + w,
			"rm -rf " + w + "/*",
			"cp -r " + sc + "/* " + w,
		}, "\n")
		// cp walks the corpus as discovery does, and meets the same directories
		// a run as root left behind. What did not come across is said, and the
		// run goes on with what did -- which is what CTP did, without saying.
		res, err := probeIn(ctx, ch, script)
		if err != nil {
			return "", err
		}
		if res.ExitCode != 0 {
			fmt.Printf("[WARN] the scenario was not copied whole into %s: %v\n", workspace, res.Failure())
		}
	} else {
		workspace = scenario
	}

	fmt.Println("SKIP TEST CASES UPDATE")
	return workspace, nil
}

// caseList produces the cases to run, either by discovery or, when resuming, by
// subtracting what previous workers finished.
func (s *Shell) caseList(ctx context.Context, ch exec.Channel, sink *result.Sink, cfg *conf.Config,
	workspace string, continueMode bool) (cases, macroSkipped, tempSkipped []string, err error) {

	if continueMode {
		cases, err = sink.Remaining()
		return cases, nil, nil, err
	}

	cases, err = Discover(ctx, ch, workspace)
	if err != nil {
		return nil, nil, nil, err
	}

	if key := strings.TrimSpace(cfg.GetOr("testcase_exclude_by_macro", "")); key != "" {
		// grep's 1 is "no case names the macro", which is an answer. Its 2 is a
		// file it could not read, and a skip list built without that file would
		// run the cases the macro exists to keep out.
		out, runErr := probeIn(ctx, ch, fmt.Sprintf("grep %q `%s`", key, findAll(workspace)))
		if runErr == nil && out.ExitCode != 0 && out.ExitCode != 1 {
			runErr = fmt.Errorf("testcase_exclude_by_macro %s: %w", key, out.Failure())
		}
		if runErr != nil {
			return nil, nil, nil, runErr
		}
		cases, macroSkipped = Remove(cases, ParseSkipped(out.Output(), key))
		for _, c := range macroSkipped {
			fmt.Println("Skipped File(macro): " + c)
		}
	}

	// Only these, when a run is a second attempt at what failed. It comes first
	// because the two exclusions still apply on top: a case excluded upstream
	// stays excluded even if it is on the list.
	if file := strings.TrimSpace(cfg.GetOr("testcase_from_file", "")); file != "" {
		out, runErr := runIn(ctx, ch, "cat "+file)
		if runErr != nil {
			return nil, nil, nil, runErr
		}
		patterns := ParseExcluded(out.Output())
		if len(patterns) == 0 {
			return nil, nil, nil, fmt.Errorf("testcase_from_file %s names no case", file)
		}
		var missed []string
		cases, missed = Include(cases, patterns)
		fmt.Println("****************************************")
		fmt.Printf("# OF SELECTED = %d, from %d patterns\n", len(cases), len(patterns))
		fmt.Println("****************************************")
		_ = missed
		if len(cases) == 0 {
			return nil, nil, nil, fmt.Errorf("testcase_from_file %s selected no case in this corpus", file)
		}
	}

	// More than one file, comma-separated, because the reasons are not one
	// reason. The corpus's own daily_regression list is upstream's judgement
	// about a case; a list of cases this machine cannot run is a fact about the
	// machine, and it has to be readable and deletable on its own -- see
	// exclusions/README.md. CTP took a single path and that still works.
	if files := ExcludeFiles(cfg.GetOr("testcase_exclude_from_file", "")); len(files) > 0 {
		var patterns []string
		for _, file := range files {
			// Strict, and not what CTP did: it read nothing from a file that was
			// not there and ran every case the file was meant to keep out.
			out, runErr := runIn(ctx, ch, "cat "+shQuote(file))
			if runErr != nil {
				return nil, nil, nil, runErr
			}
			from := ParseExcluded(out.Output())
			if len(files) > 1 {
				fmt.Printf("[INFO] %d exclusion(s) from %s\n", len(from), file)
			}
			patterns = append(patterns, from...)
		}
		// The two lines around the count are CTP's and are frozen
		// (docs/concept/external-surface-freeze.md); the count is the total.
		fmt.Println("****************************************")
		fmt.Printf("# OF EXCLUDED = %d\n", len(patterns))
		fmt.Println("****************************************")
		cases, tempSkipped = Exclude(cases, patterns)
		for _, c := range tempSkipped {
			fmt.Println("Skipped File(Temp): " + c)
		}
	}

	if err := sink.All(cases); err != nil {
		return nil, nil, nil, err
	}
	return cases, macroSkipped, tempSkipped, nil
}

// memAvailableMB is what the kernel says can be handed out without swapping,
// which is the number the tmpfs gate has to stay under. Zero when it cannot be
// read, and every caller treats that as "do not judge".
func memAvailableMB() int {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.HasPrefix(line, "MemAvailable:") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 2 {
			return 0
		}
		kb, err := strconv.Atoi(f[1])
		if err != nil {
			return 0
		}
		return kb / 1024
	}
	return 0
}

func (s *Shell) recordSkipped(report feedback.Feedback, cases []string, kind feedback.SkipType) {
	for _, c := range cases {
		report.CaseStop(feedback.CaseStop{Case: c, Elapsed: -time.Millisecond, SkipType: kind})
	}
}

// deploy configures the machine and takes the snapshot every case is restored
// from. It prepares the engine under test; it does not provision anything
// (ADR-014). Installing a build is not part of it either.
func (s *Shell) deploy(ctx context.Context, machine *topology.Instance,
	ch exec.Channel, sink *result.Sink) error {

	log := func(line string) { sink.Worker(machine.EnvID(), line) }

	if script := ConfigureScript(machine); script != "" {
		out, err := runIn(ctx, ch, script)
		if err != nil {
			return fmt.Errorf("%s: configure: %w", machine.EnvID(), err)
		}
		log(out.Output())
	}
	out, err := runIn(ctx, ch, SnapshotScript())
	if err != nil {
		return fmt.Errorf("%s: snapshot: %w", machine.EnvID(), err)
	}
	log(out.Output())
	return nil
}

// test runs the cases and waits for the queue to drain.
//
// One machine means one worker: a case restores the whole CUBRID install before
// it runs, so two cases cannot share a machine even if two workers could share a
// queue.
//
// The monitor gets its own goroutine and its own channel, because the worker is
// blocked inside a case for as long as the case takes and the timeout has to
// reach the machine anyway.
// test runs the cases and waits for the queue to drain.
//
// The pairs are one per slot, and one pair is the behaviour there has always
// been. Every slot works the same shared queue and reports under the same
// EnvID: a parallel run then writes the same files, with the same membership,
// as a serial one -- in a different order, which ADR-013 already sorts away.
// Giving each slot its own EnvID would have split dispatch_tc_FIN_<env> and
// made the two runs incomparable, which is the evidence B-T3 exists to produce.
func (s *Shell) test(ctx context.Context, machine *topology.Instance,
	pairs []channelPair, queue *dispatch.Queue,
	sink *result.Sink, report feedback.Feedback, cfg *conf.Config,
	buildID, bits string, local bool, board *status.Board, corpus *Corpus,
	record *plan.Record, split laneSplit, patches *Patches, logs *CaseLogs) error {

	var wg sync.WaitGroup
	errs := make([]error, len(pairs))
	for i, pair := range pairs {
		wg.Add(1)
		go func(i int, pair channelPair) {
			defer wg.Done()
			errs[i] = s.oneWorker(ctx, machine, pair, queue, sink, report, cfg,
				buildID, bits, local, board, corpus, record, split.laneOf(i),
				fmt.Sprintf("slot%d", i), patches, logs)
		}(i, pair)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// channelPair is what one slot needs: a channel for the case and a second one
// for the monitor, because the monitor has to be able to talk to the machine
// while the case is holding the first.
type channelPair struct {
	worker, monitor exec.Channel
	close           func()
}

func (s *Shell) oneWorker(ctx context.Context, machine *topology.Instance,
	pair channelPair, queue *dispatch.Queue,
	sink *result.Sink, report feedback.Feedback, cfg *conf.Config,
	buildID, bits string, local bool, board *status.Board, corpus *Corpus,
	record *plan.Record, lane dispatch.Lane, slotID string, patches *Patches, logs *CaseLogs) error {

	workerCh, monitorCh := pair.worker, pair.monitor
	// Which lane this slot is in: where its corpus writes land. With lanes off
	// every slot is in the same one, named for where the run's writes go, which
	// is the honest reading of what it is doing.
	name := lane.String()
	if name == "" {
		name = "disk"
		if corpus != nil {
			name = "tmpfs"
		}
	}
	board.Lane(slotID, name)
	ssh := machine.SSH()
	w := &Worker{
		EnvID:     machine.EnvID(),
		SlotID:    slotID,
		Board:     board,
		Patches:   patches,
		Logs:      logs,
		Corpus:    corpus,
		Plan:      record,
		LaneID:    lane,
		Contained: contain.Active(),
		Channel:   workerCh,
		Queue:     queue,
		Sink:      sink,
		Report:    report,
		Options: CaseOptions{
			Bits:                 bits,
			BigSpaceDir:          cfg.GetOr("large_space_dir", ""),
			Charset:              cfg.GetOr("cubrid_db_charset", "en_US"),
			IgnoreCoresByKeyword: cfg.GetOr("ignore_core_by_keywords", ""),
			SSHHost:              ssh.Host,
			SSHPort:              ssh.Port,
			SSHUser:              ssh.User,
			BuildID:              buildID,
		},
		MaxRetry:       cfg.Int("testcase_retry_num", 0),
		Local:          local,
		CheckDiskSpace: cfg.Bool("enable_check_disk_space_yn", false),
		ReserveDisk:    cfg.GetOr("reserve_disk_space_size", "2G"),
	}

	monitorCtx, stopMonitor := context.WithCancel(ctx)
	defer stopMonitor()
	go (&Monitor{
		Worker:    w,
		Channel:   monitorCh,
		Timeout:   time.Duration(cfg.Int("testcase_timeout_in_secs", 0)) * time.Second,
		Local:     local,
		Contained: contain.Active(),
	}).Watch(monitorCtx)

	return w.Run(ctx)
}

// templateStore and templateCapMB read where CTP's database-template cache keeps
// its store and how large it is allowed to be. The defaults are init.sh's.
func templateStore() string {
	if d := strings.TrimSpace(os.Getenv("CTP_DB_TEMPLATE_DIR")); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ctp_db_templates")
}

func templateCapMB() int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv("CTP_DB_TEMPLATE_MAX_MB"))); err == nil && v > 0 {
		return v
	}
	return 10240
}
