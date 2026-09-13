// Package sqlsuite runs the sql and medium tasks: CTP's sql/bin/run.sh and the
// CQT it starts, with CQT's per-case execution kept as it is (ADR-016) and
// everything around it moved here -- and onto shell's slots, so that the two
// suites that run a corpus against one prepared database can run it in
// parallel.
//
// docs/design/module-sql.md.
package sqlsuite

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
	"github.com/cubrid-systems/cubrid-testkit/internal/conf"
	"github.com/cubrid-systems/cubrid-testkit/internal/contain"
	"github.com/cubrid-systems/cubrid-testkit/internal/dispatch"
	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
	"github.com/cubrid-systems/cubrid-testkit/internal/patch"
	"github.com/cubrid-systems/cubrid-testkit/internal/result"
	"github.com/cubrid-systems/cubrid-testkit/internal/runner"
	"github.com/cubrid-systems/cubrid-testkit/internal/runner/legacy"
	"github.com/cubrid-systems/cubrid-testkit/internal/status"
)

// SQL runs the sql and medium tasks.
type SQL struct{}

// New returns the runner for sql and medium.
func New() *SQL { return &SQL{} }

func (*SQL) Tasks() []cli.Task { return []cli.Task{cli.SQL, cli.Medium} }

// exitSetup is run.sh's `exit 1`: no build, no configuration, no corpus. A run
// that found failing cases exits 0, as CTP's does (external-surface-freeze.md
// §6-1).
const exitSetup = 1

func setupFailed(format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	fmt.Println(msg)
	return &runner.ExitError{Code: exitSetup, Err: fmt.Errorf("%s", msg)}
}

// delegated says whether this run is one sqlsuite hands back to CTP: an
// interactive session, or a run under valgrind. Neither is a corpus run, and
// both are CTP's until they have a design of their own (module-sql.md §2-1).
func delegated(req runner.Request, ini *conf.Ini) bool {
	if req.Interactive {
		return true
	}
	if ini == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(ini.GetOr("sql", "enable_memory_leak", ""))) {
	case "yes", "true", "y", "on", "1":
		return true
	}
	return false
}

func (s *SQL) Validate(req runner.Request) error {
	ini, _ := req.Home.LoadIni(req.ConfigPath)
	if delegated(req, ini) {
		return legacy.New(req.Task).Validate(req)
	}
	if st, err := os.Stat(os.Getenv("CUBRID")); os.Getenv("CUBRID") == "" || err != nil || !st.IsDir() {
		return setupFailed("please make sure your build is installed")
	}
	if ini == nil {
		return setupFailed("please confirm your conf file path!")
	}
	if os.Getenv("JAVA_HOME") == "" {
		return setupFailed("JAVA_HOME is not set, and CQT needs a JDK")
	}
	return nil
}

// Run is run.sh, stage by stage.
func (s *SQL) Run(ctx context.Context, req runner.Request) error {
	ini, err := req.Home.LoadIni(req.ConfigPath)
	if err != nil {
		return setupFailed("please confirm your conf file path!")
	}
	if delegated(req, ini) {
		return legacy.New(req.Task).Run(ctx, req)
	}

	// ---- do_init ----------------------------------------------------------
	// Its status is not read: cubrid_rel can exit non-zero having printed a good
	// banner, and run.sh reads the banner. parseEngine says whether it is one.
	rel, err := machine{}.Channel().Run(ctx, "cubrid_rel")
	if err != nil {
		return setupFailed("please make sure your build is installed (cubrid_rel: %v)", err)
	}
	e, err := parseEngine(rel.Output())
	if err != nil {
		return setupFailed("please make sure your build is installed (%v)", err)
	}
	st, err := newSettings(req.Home, req.ConfigPath, string(req.Task), ini, e)
	if err != nil {
		return setupFailed("%v", err)
	}
	logFile := st.logName(e, time.Now().Unix())
	for _, d := range []string{filepath.Dir(logFile), filepath.Join(st.ctpHome, "sql", "result")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	log, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer log.Close()

	// ---- do_clean, do_configure, do_create_db ------------------------------
	// Once, here, in the runner's own namespaces, before any slot exists. The
	// database they make is at $CUBRID/databases/<db>, so every slot's
	// install overlay has it as its lower layer: prepared once, copied never,
	// and at the same path in every slot, so nothing in _vinf or
	// databases.txt needs rewriting.
	here := machine{}
	began := time.Now()
	for _, calls := range []string{"do_clean", "do_configure", "do_create_db"} {
		res, err := stage(ctx, here.Channel(), st, e, logFile, calls)
		os.Stdout.WriteString(res.Output())
		if err != nil {
			return setupFailed("%s: %v", calls, err)
		}
	}
	// Timings go to standard error: standard output is CTP's.
	fmt.Fprintf(os.Stderr, "[INFO] database prepared in %s\n", since(began))

	classDir, err := compileExecutor(ctx, st.ctpHome)
	if err != nil {
		return setupFailed("%v", err)
	}
	defer os.RemoveAll(classDir)

	cfg, err := readCQTConfig(st.ctpHome, st.jdbcConfig)
	if err != nil {
		return setupFailed("CQT's configuration %s: %v", st.jdbcConfig, err)
	}
	cases, err := discover(st, cfg)
	if err != nil {
		return setupFailed("%v", err)
	}
	if len(cases.all) == 0 {
		// CQT exits 1 from inside discovery here, having printed nothing.
		return setupFailed("No Results!! please confirm your scenario path include valid case script(the current scenairo path:%s)", st.scenario)
	}

	// ---- patches ----------------------------------------------------------
	patches, err := patch.Load(ini.GetOr("sql", "case_patch_dir", ""), st.scenario, ".sql", cases.all)
	if err != nil {
		return setupFailed("%v", err)
	}
	for _, line := range patches.Describe() {
		fmt.Println(line)
	}
	if err := applyPatches(ctx, here.Channel(), patches, cases.all); err != nil {
		return setupFailed("%v", err)
	}
	defer putPatchesBack(here.Channel(), patches, cases.all)

	// ---- places -------------------------------------------------------------
	// As shell does it: contained, every worker gets a slot, the only one
	// included; not contained and serial, the one worker runs on the machine,
	// as CTP did.
	n := max(ini.Int("sql", "parallel_slots", 1), 1)
	var places []place
	closeSlots := func() {}
	if n > 1 || contain.Active() {
		opened, closeAll, err := contain.OpenSlots(n, nil)
		if err != nil {
			return setupFailed("%v", err)
		}
		closeSlots = sync.OnceFunc(closeAll)
		defer closeSlots()
		for _, sl := range opened {
			places = append(places, sl)
		}
	} else {
		places = []place{here}
	}

	// ---- do_test ----------------------------------------------------------
	// The run's own context, which the executors are started under: an
	// executor that dies ends the run, and cancelling this is what stops the
	// others rather than waiting for the cases they have in flight.
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var executors []*jdbc
	var rec *result.SQL
	rec, err = result.OpenSQL(result.SQLRun{
		CTPHome:         st.ctpHome,
		Type:            st.category,
		Alias:           st.alias,
		Bits:            e.bits,
		Build:           build(req.Home, st.ctpHome),
		Scenario:        st.scenario,
		Mode:            junitMode(rel.Output()),
		XMLSummary:      cfg.xmlSummary,
		AnswerInSummary: cfg.answerInSummary,
		QueryPlanAll:    cfg.queryPlanAll,
		Cases:           cases.all,
		Answers:         cases.answer,
		Serial:          len(places) == 1,
		Out:             os.Stdout,
		Log:             log,
		Failure: func(caseFile string) string {
			// Asked during End, after every slot's loop has returned.
			for _, x := range executors {
				if x != nil {
					return x.Failure(caseFile)
				}
			}
			return ""
		},
	})
	if err != nil {
		return err
	}
	// What this run patched, beside the records it is about to write. Written
	// now rather than at the end: a run that dies still says which of its
	// verdicts are about a patched case.
	if err := patches.Report(rec.Root()); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] cannot record what was patched: %v\n", err)
	}

	board, stopBoard, err := openBoard(ini, st, e, cases, len(places))
	if err != nil {
		// The page is for watching the run; a run is not stopped for want of it.
		fmt.Fprintf(os.Stderr, "[WARN] no status page: %v\n", err)
		board, stopBoard = nil, func() {}
	}
	defer stopBoard()
	// What a click on a case shows: a sql case leaves a rendering and an answer,
	// not a feedback.log.
	board.DetailFunc(rec.Describe)
	// A verdict from patched source is a claim about the patched case, not about
	// the corpus, and the page has to say so wherever it shows the verdict. The
	// patches went in before the first case, so every one of them is known here.
	for _, c := range cases.all {
		if pf := patches.For(c); pf != "" {
			board.Patched(c, pf)
		}
	}

	queue := dispatch.New(cases.all, 0)
	if len(places) > 1 {
		queue.Affinity()
	}
	w := &work{cases: cases, rec: rec, board: board, total: len(cases.all)}

	// Each place starts its own server and broker, on the ports the
	// configuration names -- the same ports in every slot, which the network
	// namespace allows -- then its own executor, and takes cases from the moment
	// that executor is ready.
	//
	// The servers start one slot at a time. A server's first write to a volume
	// copies the whole file into its slot's upper layer, a gigabyte for either
	// database, and eight of them at once on one disk took 2m43s each and
	// finished together, where one alone takes seconds. One at a time, the first
	// slot is working within a minute and the rest join as they come up; their
	// executors, which want CPU rather than the disk, still start side by side.
	executors = make([]*jdbc, len(places))
	closeExecutors := sync.OnceFunc(func() {
		for _, x := range executors {
			if x != nil {
				x.Close()
			}
		}
	})
	defer closeExecutors()
	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		begin    sync.Once
		startErr []error
		errs     = make([]error, len(places))
	)
	// Servers come up one at a time, unless the slots' overlays are volatile. A
	// server's first write copies its volumes up into the slot's layer, 1.1 GB a
	// slot, and an overlay that syncs ends each copy with an fsync of it: eight
	// at once on one SATA SSD, with the executors' old scan, were 9.5 minutes
	// before the first case, and one at a time 75 s. A volatile overlay does not
	// sync, and what is left of a start is waiting -- for the heartbeat to make
	// the server active, for the broker -- which the slots can do together.
	together := contain.Volatile()
	after := make(chan struct{})
	close(after) // the first slot waits for nobody
	for i, p := range places {
		served := make(chan struct{})
		wg.Add(1)
		go func(i int, p place, after <-chan struct{}, served chan struct{}) {
			defer wg.Done()
			doneServing := sync.OnceFunc(func() { close(served) })
			defer doneServing()
			name := placeName(i, len(places))
			select {
			case <-after:
			case <-runCtx.Done():
				return
			}
			// Servers come up a slot at a time, so a small corpus can be done
			// before the later slots' turn comes; starting one then would only
			// make the run wait for a server nobody will use.
			if i > 0 && queue.Drained() {
				return
			}
			t0 := time.Now()
			res, err := stage(runCtx, p.Channel(), st, e, logFile, "do_serve")
			doneServing()
			// CTP starts one server and prints what that says. The others' say
			// the same and are noise, unless they failed.
			if i == 0 {
				mu.Lock()
				os.Stdout.WriteString(res.Output())
				mu.Unlock()
			}
			t1 := time.Now()
			var x *jdbc
			if err == nil {
				x, err = startJDBC(runCtx, p, st, e, classDir, rec.Root(), javaOptions(len(places)), rec.Say)
			}
			if err == nil && x.cases != len(cases.all) {
				// CQT's discovery and this runner's disagree, and every count in
				// the result would be wrong. Not a warning.
				x.Close()
				err = fmt.Errorf("CQT found %d cases and this runner %d", x.cases, len(cases.all))
			}
			if err != nil {
				mu.Lock()
				startErr = append(startErr, fmt.Errorf("%s: %w", name, err))
				mu.Unlock()
				if i > 0 {
					fmt.Fprintf(os.Stderr, "[WARN] %s did not come up: %v\n%s", name, err, res.Output())
				}
				return
			}
			fmt.Fprintf(os.Stderr, "[INFO] %s: server and broker up in %s, executor ready in %s\n",
				name, t1.Sub(t0).Round(time.Second), since(t1))
			mu.Lock()
			executors[i] = x
			mu.Unlock()
			begin.Do(func() { rec.Begin(x.startup) })
			if err := w.loop(runCtx, name, p, queue, x); err != nil {
				errs[i] = err
				// An executor that is gone is a run that cannot finish as CTP's
				// would have -- CQT's own loop ends on the same thing.
				cancel()
				queue.Stop()
			}
		}(i, p, after, served)
		if !together {
			after = served
		}
	}
	wg.Wait()
	if len(startErr) == len(places) {
		// Nothing ran, so there is nothing to keep: a result directory here
		// would be a run that never happened.
		rec.Discard()
		return setupFailed("%v", startErr[0])
	}
	// The executor that died, not the slots it took down with it.
	var first error
	for _, err := range errs {
		if err != nil && (first == nil || errors.Is(err, errExecutorGone) && !errors.Is(first, errExecutorGone)) {
			first = err
		}
	}
	if first != nil {
		return first
	}
	summary, err := rec.End()
	if err != nil {
		return err
	}
	closeExecutors()

	// ---- do_summary_and_clean ---------------------------------------------
	hostIP, _ := here.Channel().Run(ctx, "hostname -i")
	if err := rec.MainInfoTail(e.rel, runningUser(), strings.TrimSpace(hostIP.Output())); err != nil {
		return err
	}
	rec.Summary(summary, logFile)

	cores, err := findCores(ctx, here, places, st, e, logFile, rec.Root())
	if err != nil {
		return err
	}
	for _, c := range cores {
		fmt.Println("CORE_FILE:" + c)
	}
	// The slots go before the clean: do_clean kills every cub process this
	// namespace can see, which includes the slots', and deletes the database
	// their overlays are still mounted over. run.sh cleans after CQT has
	// exited, too.
	closeSlots()
	if len(cores) > 0 {
		if err := rec.TestError(); err != nil {
			return err
		}
	} else {
		res, err := stage(ctx, here.Channel(), st, e, logFile, "do_clean")
		os.Stdout.WriteString(res.Output())
		if err != nil {
			return err
		}
	}
	fmt.Print("-----------------------\nTesting End!\n-----------------------\n")
	return nil
}

// applyPatches puts the run's patches into the corpus, and putPatchesBack takes
// them out again.
//
// Before the first case, and not per case as shell does it. The slots share one
// corpus -- sql has no per-slot overlay over it -- so a patch applied while
// another slot is reading the same tree is a race. And every executor reads
// every case at its start, to work out where CQT's server-message flag stands
// for each (ADR-016): a patch applied after that is a patch the prediction never
// saw.
//
// A patch that does not apply stops the run. It means the case has moved, and
// running it unpatched would answer a question nobody asked.
func applyPatches(ctx context.Context, ch exec.Channel, p *patch.Set, cases []string) error {
	for _, c := range cases {
		pf := p.For(c)
		if pf == "" {
			continue
		}
		res, err := exec.Check(ch.Run(ctx, patch.ApplyScript(caseDirOf(c), pf)))
		if err != nil {
			return fmt.Errorf("%s does not apply to %s: %w: %s", pf, c, err, strings.TrimSpace(res.Output()))
		}
		p.Applied(c, pf)
	}
	return nil
}

// putPatchesBack is best effort and says what it could not do: the run is over,
// and a corpus left patched is a corpus the next run reads from git.
func putPatchesBack(ch exec.Channel, p *patch.Set, cases []string) {
	for _, c := range cases {
		pf := p.For(c)
		if pf == "" {
			continue
		}
		if res, err := exec.Check(ch.Run(context.Background(), patch.RevertScript(caseDirOf(c), pf))); err != nil {
			fmt.Fprintf(os.Stderr, "[WARN] %s is still patched with %s: %v: %s\n",
				c, pf, err, strings.TrimSpace(res.Output()))
		}
	}
}

// caseDirOf is the directory a patch applies in: the one holding cases/ and
// answers/, so a single patch can change a case and its answer together.
func caseDirOf(caseFile string) string {
	return filepath.Dir(filepath.Dir(caseFile))
}

// findCores is do_summary_and_clean's search, done where each core can be: the
// cores run.sh looks for are under $CUBRID and CTP_HOME, and in a slot
// $CUBRID is the slot's own and goes with the slot. So CTP_HOME is searched
// once, here, and each slot's $CUBRID from inside it; a core found in a slot is
// copied into the result tree before the slot closes, under a name that does
// not start with "core" -- the next run's clean deletes every core* under
// CTP_HOME, and the result tree is under it.
func findCores(ctx context.Context, here place, places []place, st *settings, e engine, logFile, root string) ([]string, error) {
	type search struct {
		p    place
		name string
		dirs string
	}
	var searches []search
	if _, slotted := places[0].(*contain.Slot); slotted {
		searches = append(searches, search{here, "", `"$CTP_HOME"`})
		for i, p := range places {
			searches = append(searches, search{p, placeName(i, len(places)), `"$CUBRID"`})
		}
	} else {
		searches = append(searches, search{here, "", `"$CUBRID" "$CTP_HOME"`})
	}
	seen := map[string]bool{}
	var cores []string
	for _, sr := range searches {
		res, err := stage(ctx, sr.p.Channel(), st, e, logFile, "find_cores "+sr.dirs)
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(strings.TrimSpace(res.Output()), "\n") {
			path, ok := strings.CutPrefix(line, "CORE_FILE:")
			if !ok || seen[sr.name+"\x00"+path] {
				continue
			}
			seen[sr.name+"\x00"+path] = true
			if sr.name != "" {
				keep := filepath.Join(root, "slot_cores", sr.name+"_"+strings.ReplaceAll(strings.TrimPrefix(path, "/"), "/", "_"))
				if _, err := exec.Check(sr.p.Channel().Run(ctx, "mkdir -p "+shQuote(filepath.Dir(keep))+" && cp "+shQuote(path)+" "+shQuote(keep))); err != nil {
					fmt.Fprintf(os.Stderr, "[WARN] cannot keep %s from %s: %v\n", path, sr.name, err)
				} else {
					fmt.Fprintf(os.Stderr, "[INFO] %s's %s is kept as %s\n", sr.name, path, keep)
				}
			}
			if !seen[path] {
				seen[path] = true
				cores = append(cores, path)
			}
		}
	}
	return cores, nil
}

// runningUser is who ran the test, for main.info's user line, which run.sh
// writes from $USER. Contained, $USER is root -- the namespace's truth, which
// the cases need -- and the account is the one the runner was started as.
func runningUser() string {
	if u := os.Getenv(contain.OuterUserEnv); u != "" {
		return u
	}
	return os.Getenv("USER")
}

// openBoard is the status page, when status_http asks for one: off by
// default, and never on standard output, which is CTP's and is compared
// (ADR-003). A second run finds the default port taken and moves along to a
// free one, as shell's does; an address the operator pinned is not moved.
func openBoard(ini *conf.Ini, st *settings, e engine, cases *caseSet, slots int) (*status.Board, func(), error) {
	addr := status.Addr(ini.GetOr("sql", "status_http", ""))
	if addr == "" {
		return nil, func() {}, nil
	}
	board := status.New(len(cases.all))
	board.Expect(cases.all, nil, slots)
	where, stop, err := board.Serve(addr)
	if err != nil && addr == status.DefaultAddr {
		for try := 1; try <= 16 && err != nil; try++ {
			where, stop, err = board.Serve(status.NearDefault(try))
		}
	}
	if err != nil {
		return nil, nil, err
	}
	fmt.Fprintf(os.Stderr, "[INFO] status page at http://%s/\n", where)
	board.Watch(os.Getenv("CUBRID"), "", 0)
	// Where a slot's writes go, named as the page groups by it. sql has no
	// memory lane -- shell's tmpfs ceiling is its own -- so this says the disk
	// and whether its syncs were taken out of the way.
	lane := "disk"
	if contain.Volatile() {
		lane = "disk, volatile"
	}
	for i := 0; i < slots; i++ {
		board.Lane(placeName(i, slots), lane)
	}
	notRun := len(cases.all) - len(cases.answer)
	board.Setup([]status.Setting{
		{Group: "suite", Key: "task", Value: st.category},
		{Group: "suite", Key: "parallel_slots", Value: fmt.Sprint(slots), Default: "1", Note: "cases at once; a directory stays on one slot"},
		{Group: "suite", Key: "executor", Value: "jdbc", Note: "CQT's own parser and renderer (ADR-016)"},
		{Group: "suite", Key: "db", Value: st.dbName + " " + st.dbCharset},
		{Group: "suite", Key: "jdbc_config_file", Value: st.jdbcConfig, Default: "test_default.xml"},
		{Group: "suite", Key: "cases without an answer", Value: fmt.Sprint(notRun), Default: "0", Note: "counted as failures, never run"},
		{Group: "engine", Key: "build", Value: e.ver},
		{Group: "environment", Key: contain.Env, Value: yesNo(contain.Active()),
			Note: "the run's own namespaces; slots need them"},
		{Group: "environment", Key: contain.SlotRootEnv, Value: orUnset(os.Getenv(contain.SlotRootEnv)),
			Default: "/var/tmp/testkit-slots", Note: "where a slot's writes land"},
		{Group: "environment", Key: contain.SlotVolatileEnv, Value: yesNo(contain.Volatile()), Default: "no",
			Note: "a sync on a slot's layer returns having done nothing"},
		{Group: "environment", Key: "case_patch_dir", Value: orUnset(ini.GetOr("sql", "case_patch_dir", "")),
			Default: "off", Note: "verdicts from a patched case are about the patch"},
	})
	return board, stop, nil
}

func since(t time.Time) time.Duration { return time.Since(t).Round(time.Second) }

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func orUnset(s string) string {
	if strings.TrimSpace(s) == "" {
		return "unset"
	}
	return s
}

// placeName names a place in messages: the slot's label, or the machine.
func placeName(i, n int) string {
	if n == 1 && !contain.Active() {
		return "this machine"
	}
	return fmt.Sprintf("slot%d", i)
}

// build is what CQT names the build: local.properties' dbversion and
// dbbuildnumber, which do_configure has just written from cubrid_rel. A file
// that is not there gives "." -- getValue answers "" for a missing key.
func build(home *conf.Home, ctpHome string) string {
	props, err := home.Load(filepath.Join(ctpHome, "sql", "configuration", "local.properties"))
	if err != nil {
		return "."
	}
	return strings.TrimSpace(props.GetOr("dbversion", "")) + "." + strings.TrimSpace(props.GetOr("dbbuildnumber", ""))
}

// junitMode is JunitXmlWriter's last word for the suite: debug or release, from
// cubrid_rel; neither means no report.
func junitMode(rel string) string {
	switch l := strings.ToLower(rel); {
	case strings.Contains(l, "debug"):
		return "debug"
	case strings.Contains(l, "release"):
		return "release"
	}
	return ""
}
