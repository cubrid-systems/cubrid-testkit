package shellsuite

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/cli"
	"github.com/cubrid-systems/cubrid-testkit/internal/conf"
	"github.com/cubrid-systems/cubrid-testkit/internal/dispatch"
	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
	"github.com/cubrid-systems/cubrid-testkit/internal/feedback"
	"github.com/cubrid-systems/cubrid-testkit/internal/result"
	"github.com/cubrid-systems/cubrid-testkit/internal/runner"
	"github.com/cubrid-systems/cubrid-testkit/internal/topology"
)

// Shell runs the shell task, and rqg, which is the same machinery under a
// different category.
type Shell struct {
	// Channels opens the two channels each instance needs. It is a field because
	// the choice of machine is the one thing that varies between a local run and a
	// remote one, and because a test cannot be allowed to open the real ones: the
	// reset that runs before every case would kill the test.
	Channels func([]*topology.Instance) (workers, monitors map[string]exec.Channel, err error)
}

// NewShell returns the runner for the shell and rqg tasks.
func NewShell() *Shell { return &Shell{Channels: openChannels} }

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

	instances, err := topology.From(cfg)
	if err != nil {
		return quit("%v", err)
	}
	// No configured machine means this one.
	local := len(instances) == 0
	if local {
		instances = []*topology.Instance{topology.Local(cfg)}
	}
	envIDs := make([]string, 0, len(instances))
	for _, inst := range instances {
		envIDs = append(envIDs, inst.EnvID())
	}
	fmt.Printf("Available Env: [%s]\n", strings.Join(envIDs, ", "))
	fmt.Printf("Continue Mode: %v\n", continueMode)

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

	// One channel per instance for the workers, and a second for each monitor.
	opener := s.Channels
	if opener == nil {
		opener = openChannels
	}
	channels, monitorChannels, err := opener(instances)
	if err != nil {
		return quit("%v", err)
	}
	defer func() {
		for _, ch := range channels {
			ch.Close()
		}
		for _, ch := range monitorChannels {
			ch.Close()
		}
	}()

	first := channels[instances[0].EnvID()]
	buildInfo, err := runIn(ctx, first, versionScript)
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
	for _, inst := range instances {
		envID := inst.EnvID()
		logFile, err := sink.Check(envID)
		if err != nil {
			return err
		}
		title := envID
		if !inst.IsLocal() {
			title = inst.SSH().User + "@" + inst.SSH().Host + ":" + inst.SSH().Port
		}
		check := &CheckRequirement{
			EnvID:       envID,
			Title:       title,
			Protocol:    cfg.GetOr("service_protocol_type", "ssh"),
			Channel:     channels[envID],
			Scenario:    strings.TrimSpace(cfg.GetOr("scenario", "")),
			ExcludeFile: strings.TrimSpace(cfg.GetOr("testcase_exclude_from_file", "")),
			Out:         os.Stdout,
			Log:         logFile,
		}
		check.Check(ctx)

		// CTP's Log constructor creates its file whether or not anything is ever
		// written to it, so every run leaves a monitor_<envId>.log behind -- usually
		// empty, always present.
		if err := sink.Monitor(envID, ""); err != nil {
			return err
		}
	}

	// ---- workspace -------------------------------------------------------
	fmt.Println("============= UPDATE TEST CASES ==================")
	workspace, err := s.prepareWorkspace(ctx, channels, cfg, local)
	if err != nil {
		return quit("%v", err)
	}
	fmt.Println("DONE")

	// ---- case list -------------------------------------------------------
	fmt.Println("============= FETCH TEST CASES ==================")
	cases, macroSkipped, tempSkipped, err := s.caseList(ctx, first, sink, cfg, workspace, continueMode)
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

	// ---- deploy ----------------------------------------------------------
	fmt.Println("============= DEPLOY ==================")
	if err := s.deploy(ctx, instances, channels, sink); err != nil {
		return quit("%v", err)
	}
	fmt.Println("DONE")

	// ---- test ------------------------------------------------------------
	fmt.Println("============= TEST ==================")
	queue := dispatch.New(cases, cfg.Int("testcase_retry_num", 0))
	err = s.test(ctx, instances, channels, monitorChannels, queue, sink, report, cfg, buildID, bits, local)

	report.TaskStop()
	fmt.Println("TEST COMPLETE")
	return err
}

// openChannels opens the two channels each instance needs. A worker spends most
// of its life inside a case, so the monitor that has to interrupt it cannot
// share.
//
// No instances means a local run, and then both channels are this machine.
func openChannels(instances []*topology.Instance) (workers, monitors map[string]exec.Channel, err error) {
	workers = map[string]exec.Channel{}
	monitors = map[string]exec.Channel{}

	for _, inst := range instances {
		if inst.IsLocal() {
			// CTP reached even the local machine through SSHConnect, so a local run
			// gets the profile just as a remote one does.
			workers[inst.EnvID()] = &exec.Local{SourceProfile: true}
			monitors[inst.EnvID()] = &exec.Local{SourceProfile: true}
			continue
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
		workers[inst.EnvID()] = open()
		monitors[inst.EnvID()] = open()
	}
	return workers, monitors, nil
}

// prepareWorkspace materialises the case tree the run will read from.
//
// CTP did this inside TestCaseGithub.update, wrapped around a git pull. The pull
// is excluded, but the copy is not: cases dirty their own directories, and the
// dispatcher searches the workspace rather than the scenario. Without the copy
// there would be nothing to find.
func (s *Shell) prepareWorkspace(ctx context.Context, channels map[string]exec.Channel, cfg *conf.Config, local bool) (string, error) {
	scenario := strings.TrimSpace(cfg.GetOr("scenario", ""))
	workspace := strings.TrimSpace(cfg.GetOr("testcase_workspace_dir", ""))
	if workspace == "" || workspace == scenario {
		for envID, ch := range channels {
			out, err := runIn(ctx, ch, KillScript(local))
			if err != nil {
				return "", fmt.Errorf("%s: %w", envID, err)
			}
			fmt.Println("CLEAN PROCESSES:")
			fmt.Println(out.Output())
		}
		fmt.Println("SKIP TEST CASES UPDATE")
		return scenario, nil
	}

	script := strings.Join([]string{
		"mkdir -p " + workspace,
		"rm -rf " + workspace + "/*",
		"cp -r " + scenario + "/* " + workspace,
	}, "\n")

	var wg sync.WaitGroup
	errs := make([]error, 0, len(channels))
	var mu sync.Mutex
	for envID, ch := range channels {
		wg.Add(1)
		go func(envID string, ch exec.Channel) {
			defer wg.Done()
			if _, err := runIn(ctx, ch, KillScript(local)); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s: %w", envID, err))
				mu.Unlock()
				return
			}
			if _, err := runIn(ctx, ch, script); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s: %w", envID, err))
				mu.Unlock()
			}
		}(envID, ch)
	}
	wg.Wait()
	fmt.Println("SKIP TEST CASES UPDATE")
	return workspace, errors.Join(errs...)
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
		out, runErr := runIn(ctx, ch, fmt.Sprintf("grep %q `%s`", key, findAll(workspace)))
		if runErr != nil {
			return nil, nil, nil, runErr
		}
		cases, macroSkipped = Remove(cases, ParseSkipped(out.Output(), key))
		for _, c := range macroSkipped {
			fmt.Println("Skipped File(macro): " + c)
		}
	}

	if file := strings.TrimSpace(cfg.GetOr("testcase_exclude_from_file", "")); file != "" {
		out, runErr := runIn(ctx, ch, "cat "+file)
		if runErr != nil {
			return nil, nil, nil, runErr
		}
		patterns := ParseExcluded(out.Output())
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

func (s *Shell) recordSkipped(report feedback.Feedback, cases []string, kind feedback.SkipType) {
	for _, c := range cases {
		report.CaseStop(feedback.CaseStop{Case: c, Elapsed: -time.Millisecond, SkipType: kind})
	}
}

// deploy configures each instance and takes the snapshot every case is restored
// from. Installing a build is not part of it.
func (s *Shell) deploy(ctx context.Context, instances []*topology.Instance,
	channels map[string]exec.Channel, sink *result.Sink) error {

	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error
	for _, inst := range instances {
		wg.Add(1)
		go func(inst *topology.Instance) {
			defer wg.Done()
			ch := channels[inst.EnvID()]
			log := func(line string) { sink.Worker(inst.EnvID(), line) }

			if script := ConfigureScript(inst); script != "" {
				out, err := runIn(ctx, ch, script)
				if err != nil {
					mu.Lock()
					errs = append(errs, fmt.Errorf("%s: configure: %w", inst.EnvID(), err))
					mu.Unlock()
					return
				}
				log(out.Output())
			}
			out, err := runIn(ctx, ch, SnapshotScript())
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s: snapshot: %w", inst.EnvID(), err))
				mu.Unlock()
				return
			}
			log(out.Output())
		}(inst)
	}
	wg.Wait()
	return errors.Join(errs...)
}

// test starts one worker and one monitor per instance and waits for the queue to
// drain.
func (s *Shell) test(ctx context.Context, instances []*topology.Instance,
	channels, monitorChannels map[string]exec.Channel, queue *dispatch.Queue,
	sink *result.Sink, report feedback.Feedback, cfg *conf.Config, buildID, bits string, local bool) error {

	timeout := time.Duration(cfg.Int("testcase_timeout_in_secs", 0)) * time.Second
	maxRetry := cfg.Int("testcase_retry_num", 0)

	// A cancelled context has to reach workers blocked waiting for the first pass
	// to finish, and Claim does not take one.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			queue.Stop()
		case <-stop:
		}
	}()

	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error
	for _, inst := range instances {
		envID := inst.EnvID()
		ssh := inst.SSH()
		w := &Worker{
			EnvID:   envID,
			Channel: channels[envID],
			Queue:   queue,
			Sink:    sink,
			Report:  report,
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
			MaxRetry:       maxRetry,
			Local:          local,
			CheckDiskSpace: cfg.Bool("enable_check_disk_space_yn", false),
			ReserveDisk:    cfg.GetOr("reserve_disk_space_size", "2G"),
		}

		monitorCtx, stopMonitor := context.WithCancel(ctx)
		m := &Monitor{Worker: w, Channel: monitorChannels[envID], Timeout: timeout, Local: local}
		go m.Watch(monitorCtx)

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer stopMonitor()
			if err := w.Run(ctx); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s: %w", envID, err))
				mu.Unlock()
			}
		}()
	}
	// CTP printed this once the workers were launched, not once they were done,
	// so it lands before the first [ENV START].
	fmt.Println("STARTED")

	wg.Wait()
	return errors.Join(errs...)
}
