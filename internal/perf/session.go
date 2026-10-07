package perf

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/sandbox"
)

// Session is the weekly run (Design §5.1): every pair of perf.conf and every
// active registration, each gated by a canary A/A between its two clusters,
// then the cases A/B with the idle side paused, one fixture server up per
// side, re-measurement when the pairs disagree, and the files Spec §7.6–7.8
// describe. It is one bench-client run; the lease is checked, not taken.
type Session struct {
	Runner
	Conf     *Conf
	Pairs    []*SessionPair
	Clock    clock
	DryRun   bool
	Only     string
	OnlyPair string

	doc       *SessionDoc
	leaseFile string
	tracking  bool     // whether the lease is this run's to lose
	runs      string   // the runs root, for the previous sessions
	left      []string // registrations left out: "<branch>: <reason>"
}

// SessionDoc is session.json (Design §4.2).
type SessionDoc struct {
	Schema      string         `json:"schema"`
	ID          string         `json:"id"`
	Kind        string         `json:"kind"`
	Started     time.Time      `json:"started"`
	Ended       time.Time      `json:"ended"`
	State       string         `json:"state"`
	Host        string         `json:"host"`
	BenchMode   BenchMode      `json:"bench_mode"`
	Interleave  string         `json:"interleave"`
	CPUSet      CPUSetPair     `json:"cpuset"`
	Pinning     string         `json:"pinning"`
	ClientImage ClientImage    `json:"client_image"`
	Pairs       []*SessionPair `json:"pairs"`
	// Valid says whether every pair that ran passed its canaries; Flags is
	// their flag count. Both are what the hub's dashboard reads first.
	Valid      bool           `json:"valid"`
	Flags      int            `json:"flags"`
	BudgetS    int            `json:"budget_s"`
	Deadline   *time.Time     `json:"deadline,omitempty"`
	ExitReason string         `json:"exit_reason"`
	Guard      GuardReport    `json:"guard"`
	Preflight  map[string]any `json:"preflight"`
	Previous   string         `json:"previous_session,omitempty"`
	Notes      []string       `json:"notes,omitempty"`
}

const sessionUsage = `usage: testkit perf session -c <perf.conf> [--dry-run] [--only <case-glob>] [--pair <name>]
                            [--out <dir>] [--deadline <RFC3339>] [--id <run-id>] [--keep]

Runs the weekly session perf.conf describes: every pair, canaries first, then
the cases A/B. Results go to $REPORTS_DIR (what bench-client exports), else
--out, else ./perf-session-<time>/. --dry-run resolves the builds, checks the
host and writes session.json without creating a cluster. --only narrows the
cases, --pair the pairs. The deadline is $PERF_DEADLINE or --deadline; the
session stops at a pair or case boundary when it or the budget comes first.`

// sessionCmd is the verb.
func sessionCmd(args []string, stdout, stderr io.Writer) int {
	s := &Session{Runner: Runner{Log: stderr, Started: time.Now(), PauseIdle: true}}
	var confPath, deadline, id string
	for i := 0; i < len(args); i++ {
		a := args[i]
		next := func() (string, bool) {
			if i+1 < len(args) {
				i++
				return args[i], true
			}
			return "", false
		}
		ok := true
		switch a {
		case "-c":
			confPath, ok = next()
		case "--dry-run":
			s.DryRun = true
		case "--only":
			s.Only, ok = next()
		case "--pair":
			s.OnlyPair, ok = next()
		case "--out":
			s.Out, ok = next()
		case "--deadline":
			deadline, ok = next()
		case "--id":
			id, ok = next()
		case "--keep":
			s.Keep = true
		case "-h", "--help":
			fmt.Fprintln(stdout, sessionUsage)
			return ExitOK
		default:
			fmt.Fprintf(stderr, "testkit perf session: %q is not a flag\n%s\n", a, sessionUsage)
			return ExitRefused
		}
		if !ok {
			fmt.Fprintf(stderr, "testkit perf session: %s wants a value\n%s\n", a, sessionUsage)
			return ExitRefused
		}
	}
	if confPath == "" {
		fmt.Fprintln(stderr, sessionUsage)
		return ExitRefused
	}
	if deadline == "" {
		deadline = os.Getenv("PERF_DEADLINE")
	}
	var dl time.Time
	if deadline != "" {
		t, err := time.Parse(time.RFC3339, deadline)
		if err != nil {
			fmt.Fprintf(stderr, "testkit perf session: deadline %q is not RFC3339\n", deadline)
			return ExitRefused
		}
		dl = t
	}
	// The results directory decides the id: bench-client stages a run at
	// <root>/runs/<id> and exports it as REPORTS_DIR.
	if s.Out == "" {
		s.Out = os.Getenv("REPORTS_DIR")
	}
	if s.Out == "" {
		s.Out = "perf-session-" + s.Started.Format("20060102-150405")
	}
	s.Out = absolute(s.Out)
	switch {
	case id != "":
		s.SessionID = id
	case os.Getenv("REPORTS_DIR") != "":
		s.SessionID = filepath.Base(s.Out)
	default:
		s.SessionID = s.Started.Format("20060102") + "_perf-session"
	}
	return s.run(confPath, dl)
}

func (s *Session) note(format string, args ...any) {
	s.doc.Notes = append(s.doc.Notes, fmt.Sprintf(format, args...))
	s.logf(format, args...)
}

// setState settles the document's state and totals without writing it.
func (s *Session) setState(state, reason string) {
	s.doc.State, s.doc.ExitReason, s.doc.Ended = state, reason, time.Now()
	ran, valid, flags := 0, true, 0
	for _, p := range s.doc.Pairs {
		if p.Skipped != "" {
			continue
		}
		ran++
		valid = valid && p.Valid
		flags += p.Flags
	}
	s.doc.Valid, s.doc.Flags = ran > 0 && valid, flags
}

func (s *Session) writeDoc(state, reason string) {
	s.setState(state, reason)
	if err := writeJSON(filepath.Join(s.Out, "session.json"), s.doc); err != nil {
		s.logf("session.json: %v", err)
	}
}

// finish is the end of every path that got as far as a results directory:
// the summary (which reads the state), then session.json.
func (s *Session) finish(state, reason string) {
	s.setState(state, reason)
	if err := s.summary(); err != nil {
		s.note("summary: %v", err)
	}
	s.writeDoc(state, reason)
}

// run is Session(conf) of Design §5.1.
func (s *Session) run(confPath string, deadline time.Time) int {
	// 1. Everything read and validated before anything is touched: exit 2.
	c, err := ReadConf(confPath)
	if err != nil {
		for _, line := range problemLines(err) {
			fmt.Fprintln(s.Log, line)
		}
		return ExitRefused
	}
	s.Conf = c
	s.SuiteDir = absolute(c.Suite)
	suite, err := LoadSuite(s.SuiteDir)
	if err != nil {
		for _, line := range problemLines(err) {
			fmt.Fprintln(s.Log, line)
		}
		return ExitRefused
	}
	s.Suite = suite
	var canaries []*Case
	for _, id := range c.Canaries {
		cc := suite.Case(id)
		if cc == nil {
			fmt.Fprintf(s.Log, "%s: canary %s is not a case in %s\n", confPath, id, c.Suite)
			return ExitRefused
		}
		canaries = append(canaries, cc)
	}
	var active []Branch
	if c.Branches != "" {
		all, err := ReadBranches(c.Branches)
		if err != nil {
			for _, line := range problemLines(err) {
				fmt.Fprintln(s.Log, line)
			}
			return ExitRefused
		}
		active, s.left = Active(all, c.BranchesMax, s.Started)
	}
	s.CPUSet, s.ClientCPUSet, s.ClientImage, s.CSBBin = c.CPUSetServer, c.CPUSetClient, c.ClientImage, c.CSB
	// csb, sandbox.Home() and the work directories all go by $CSB_HOME; the
	// conf's value is the one that counts.
	if c.CSBHome != "" {
		os.Setenv("CSB_HOME", absolute(c.CSBHome))
	}
	s.runs = runsRoot()
	s.leaseFile = filepath.Join(s.runs, ".lease.json")

	s.doc = &SessionDoc{
		Schema: "perf-regression-session/1", ID: s.SessionID, Kind: "session", Started: s.Started, State: "incomplete",
		Host: hostname(), Interleave: c.Interleave, CPUSet: CPUSetPair{Server: s.CPUSet, Client: s.ClientCPUSet},
		Pinning: s.pinning(), BudgetS: c.SessionBudgetS, Preflight: map[string]any{}, Pairs: []*SessionPair{},
		Guard: GuardReport{Before: []string{}, Cleaned: []string{}, Left: []string{}, OtherUsers: []string{}, InContainers: []string{}},
	}
	s.Clock = clock{End: s.Started.Add(time.Duration(c.SessionBudgetS) * time.Second)}
	if !deadline.IsZero() {
		s.doc.Deadline = &deadline
		if deadline.Before(s.Clock.End) {
			s.Clock.End = deadline
		}
	}
	for _, l := range s.left {
		s.note("registration left out: %s", l)
	}

	// 2. The builds: builds.json, or install trees named outright (FR-1.1).
	var manifest *BuildsManifest
	stale := false
	if c.BuildsManifest != "" {
		if m, err := readBuilds(c.BuildsManifest); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				s.note("builds manifest: %v", err)
			} else {
				s.note("no builds manifest at %s; only pairs that name install trees can run", c.BuildsManifest)
			}
		} else {
			manifest = m
			if stale = m.stale(s.Started); stale {
				s.note("builds manifest %s was written %s, more than %s before this session: every pair is skipped (L9)", c.BuildsManifest, m.Written, buildsMaxAge)
			}
		}
	}
	s.Pairs = pairsFrom(c, active, manifest, stale)
	if s.OnlyPair != "" {
		var kept []*SessionPair
		for _, p := range s.Pairs {
			if p.Name == s.OnlyPair {
				kept = append(kept, p)
			}
		}
		if len(kept) == 0 {
			fmt.Fprintf(s.Log, "testkit perf session: --pair %s is not a pair of %s or a registered branch\n", s.OnlyPair, confPath)
			return ExitRefused
		}
		s.Pairs = kept
	}
	s.doc.Pairs = s.Pairs
	for _, p := range s.Pairs {
		if p.Skipped != "" {
			continue
		}
		p.fingerprints()
		if d := p.debugBuild(); d != "" {
			// Spec §12: a Debug build is not measured. A conf pair is the
			// session's reason to exist and refuses it; a registration is
			// one pair among others and is skipped by itself.
			if p.branch == nil {
				fmt.Fprintf(s.Log, "testkit perf session: pair %s: %s is a Debug build; only Release builds are measured (Spec §12)\n", p.Name, d)
				return ExitRefused
			}
			p.Skipped = "debug build: " + d
			continue
		}
		p.selected = selectCases(suite, p.branch, s.Only)
		// FR-2: against the newest earlier session that measured this pair.
		if pp, id := previousPair(s.runs, s.SessionID, p.Name); pp != nil {
			if s.doc.Previous == "" {
				s.doc.Previous = id
			}
			var diffs []string
			if d := fingerprintDiff(pp.Target.Fingerprint, p.Target.Fingerprint); d != "" {
				diffs = append(diffs, "target: "+d)
			}
			if d := fingerprintDiff(pp.Reference.Fingerprint, p.Reference.Fingerprint); d != "" {
				diffs = append(diffs, "reference: "+d)
			}
			if len(diffs) > 0 {
				p.FingerprintChanged = true
				p.FingerprintNote = fmt.Sprintf("since %s: %s", id, strings.Join(diffs, "; "))
			}
		}
	}
	if err := os.MkdirAll(s.Out, 0o755); err != nil {
		fmt.Fprintf(s.Log, "testkit perf session: %v\n", err)
		return ExitEnvironment
	}
	runnable := 0
	for _, p := range s.Pairs {
		if p.Skipped == "" {
			runnable++
		}
	}
	if runnable == 0 {
		// Nothing to run is a session that ends well (L9, Design §8): the
		// summary says why, and no host check turns it into a failure.
		s.note("no pair can run; the session ends without a measurement")
		s.plan(canaries)
		s.finish("complete", "nothing to run")
		return ExitOK
	}

	// 3. The host (Design §7, §11; Spec FR-6.1, FR-6.2 stage 1).
	s.doc.BenchMode = benchModeState()
	if s.doc.BenchMode.Boost != nil && *s.doc.BenchMode.Boost {
		s.note("CPU boost is on; the session continues (FR-6.1)")
	}
	code := s.preflight()
	if code != ExitOK && !s.DryRun {
		s.finish("incomplete", "preflight")
		return code
	}
	guard, gerr := guardBefore(filepath.Join(s.Out, "guard"), !s.DryRun)
	s.doc.Guard = guard
	if gerr != nil {
		s.note("guard: %v", gerr)
		if !s.DryRun {
			s.finish("incomplete", "guard")
			return ExitEnvironment
		}
	} else if len(guard.Cleaned) > 0 {
		s.note("guard: stopped %d server process(es) outside the clusters", len(guard.Cleaned))
	}
	if len(guard.OtherUsers) > 0 {
		s.note("guard: %d server process(es) of other users on this host could not be stopped; the session continues beside them", len(guard.OtherUsers))
	}
	if len(guard.InContainers) > 0 {
		s.note("guard: %d server process(es) in containers that are not pf- clusters; left alone", len(guard.InContainers))
	}
	if !s.DryRun {
		s.tracking = leaseHeldBy(s.leaseFile, s.SessionID)
		if !s.tracking {
			s.note("no lease names run %s at %s; the lease is not checked between cases (local session)", s.SessionID, s.leaseFile)
		}
	}
	if s.CPUSet != "" {
		if cores, err := parseCPUList(s.CPUSet); err == nil {
			s.Guard = &hostGuard{cores: cores, threshold: contaminationCores}
		}
	}

	// 4. The plan, said once; a dry run ends here (Spec §13 A1).
	s.plan(canaries)
	if s.DryRun {
		s.writeDoc("complete", "dry-run")
		return ExitOK
	}
	s.writeDoc("incomplete", "")
	if err := s.cleanupStale(); err != nil {
		s.note("cleanup of earlier pf- clusters: %v", err)
	}

	// 5. The pairs. A pair counts as run once its sidecar is on disk,
	// whether or not the session's clock stopped it part-way (FR-5).
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	reason := "done"
	attempted, ran := 0, 0
	for _, p := range s.Pairs {
		if p.Skipped != "" {
			s.logf("pair %s: skipped (%s)", p.Name, p.Skipped)
			continue
		}
		if reason != "done" {
			p.Skipped = reason
			continue
		}
		if bound := pairBound(canaries, p.selected, p.Overlap != nil); !s.Clock.fits(time.Now(), bound) {
			p.Skipped = "budget"
			s.logf("pair %s: skipped, %s left and the pair's bound is %s", p.Name, s.Clock.left(time.Now()).Round(time.Minute), bound.Round(time.Minute))
			continue
		}
		attempted++
		if why := s.runPair(ctx, p, canaries); why != "" {
			reason = why
		}
		if pairRan(p) {
			ran++
		}
		s.writeDoc("incomplete", "")
	}
	if reason == "done" && ctx.Err() != nil {
		reason = "signal"
	}

	// 6. What a reader gets (§5.10).
	state, code := sessionExit(reason, attempted, ran)
	s.finish(state, reason)
	return code
}

// pairRan says whether a pair counts as run: its sidecar is on disk and it
// was not skipped for a reason of its own (a cluster that never stood up).
func pairRan(p *SessionPair) bool {
	return p.written && p.Skipped == ""
}

// sessionExit is Spec §7.1's exit code: 0 when the session ended, invalid
// pairs and a budget stop included (FR-5); 3 when a signal or the lost
// lease ended it (FR-6), or when every pair that was tried failed to stand
// up (Design §5.1).
func sessionExit(reason string, attempted, ran int) (state string, code int) {
	switch {
	case reason == "signal" || reason == "lease":
		return "incomplete", ExitEnvironment
	case reason != "budget" && attempted > 0 && ran == 0:
		return "complete", ExitEnvironment
	}
	return "complete", ExitOK
}

// preflight is Design §11's checks that stop a session before a cluster:
// csb, the client image, the disk, the memory. What is only a note (the
// page-cache drop) is recorded and the session goes on.
func (s *Session) preflight() int {
	pf := s.doc.Preflight
	code := ExitOK
	if err := (&sandbox.CLI{Bin: s.CSBBin}).Available(context.Background()); err != nil {
		pf["csb"] = err.Error()
		s.note("csb: %v", err)
		code = ExitEnvironment
	} else {
		pf["csb"] = "ok"
	}
	digest := imageDigest(s.ClientImage)
	s.doc.ClientImage = ClientImage{Name: s.ClientImage, Digest: digest}
	if digest == "" {
		pf["client_image"] = "missing"
		s.note("client image %s is not on this host", s.ClientImage)
		code = ExitEnvironment
	} else {
		pf["client_image"] = "ok"
	}
	if free, err := diskFree(sandbox.Home()); err == nil {
		gb := float64(free) / (1 << 30)
		pf["disk_free_gb"] = math.Round(gb*10) / 10
		if s.Conf.DiskMinGB > 0 && gb < float64(s.Conf.DiskMinGB) {
			s.note("%.0f GB free under %s, less than disk_min_gb=%d", gb, sandbox.Home(), s.Conf.DiskMinGB)
			code = ExitEnvironment
		}
	}
	if avail, err := memAvailable(); err == nil {
		pf["mem_available_gb"] = math.Round(float64(avail)/(1<<30)*10) / 10
		if s.Conf.MemoryCap != "" {
			if cap, err := parseSize(s.Conf.MemoryCap); err == nil && avail < cap {
				s.note("MemAvailable %.1f GB is below memory_cap=%s", float64(avail)/(1<<30), s.Conf.MemoryCap)
				code = ExitEnvironment
			}
		}
	}
	drop := dropCachesInstalled
	if _, err := os.Stat(drop); err != nil {
		drop = filepath.Join(s.SuiteDir, "scripts", "page_cache_drop.sh")
	}
	if out, err := osExecOutput("sudo", "-n", "-l", drop); err != nil {
		pf["drop_caches"] = "sudo -n refuses " + drop + ": " + tail(out, 120)
		s.note("page-cache drop: sudo -n -l %s failed; cold cases will be null(cache_drop)", drop)
	} else {
		pf["drop_caches"] = "ok"
	}
	return code
}

// plan says what the session is about to do, on the log, so the dry run and
// the real one read the same.
func (s *Session) plan(canaries []*Case) {
	s.logf("session %s: out %s, budget %s, ends by %s", s.SessionID, s.Out, (time.Duration(s.Conf.SessionBudgetS) * time.Second).String(), s.Clock.End.Format(time.RFC3339))
	ids := make([]string, len(canaries))
	for i, c := range canaries {
		ids[i] = c.ID
	}
	s.logf("canaries: %s (tolerance %.3f)", strings.Join(ids, " "), s.Conf.CanaryTolerance)
	for _, p := range s.Pairs {
		if p.Skipped != "" {
			s.logf("pair %s: skipped (%s)", p.Name, p.Skipped)
			continue
		}
		s.logf("pair %s: target %s (%s) vs reference %s (%s)", p.Name, p.Target.Build, short(p.Target.Commit), p.Reference.Build, short(p.Reference.Commit))
		if p.Overlap != nil {
			s.logf("pair %s: overlap %s (%s)", p.Name, p.Overlap.Build, short(p.Overlap.Commit))
		}
		if p.FingerprintChanged {
			s.logf("pair %s: fingerprint changed %s", p.Name, p.FingerprintNote)
		}
		order := shuffled(p.selected, seedFrom(s.SessionID+"/"+p.Name))
		var names []string
		for _, g := range confGroups(order) {
			var in []string
			for _, c := range g.Cases {
				in = append(in, c.ID)
			}
			names = append(names, fmt.Sprintf("[%s]", strings.Join(in, " ")))
		}
		s.logf("pair %s: %d case(s) in %d conf group(s), bound %s: %s", p.Name, len(order), len(confGroups(order)), pairBound(canaries, order, p.Overlap != nil).Round(time.Minute), strings.Join(names, " "))
	}
}

func short(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}
	return orUnknown(commit)
}

// cleanupStale removes what an earlier session left (Design §8): every
// cluster whose name starts with pf-.
func (s *Session) cleanupStale() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cli := &sandbox.CLI{Bin: s.CSBBin}
	all, err := cli.Clusters(ctx)
	if err != nil {
		return err
	}
	for _, cl := range all {
		if !strings.HasPrefix(cl.Name, "pf-") {
			continue
		}
		c := &sandbox.CLI{Bin: s.CSBBin, Cluster: cl.Name}
		if _, err := c.DestroyPurge(ctx); err != nil {
			s.logf("stale cluster %s: %v", cl.Name, err)
			continue
		}
		s.logf("removed stale cluster %s", cl.Name)
	}
	return nil
}

// leaseHeld is L7's check at every case boundary.
func (s *Session) leaseHeld() bool {
	if !s.tracking {
		return true
	}
	return leaseHeldBy(s.leaseFile, s.SessionID)
}

// sides is the clusters a pair has up, by suffix.
type sides struct {
	t, r, o *Side
}

func (ss *sides) all() []*Side {
	var out []*Side
	for _, s := range []*Side{ss.t, ss.r, ss.o} {
		if s != nil {
			out = append(out, s)
		}
	}
	return out
}

// frozen names the first side that could not be unpaused, or "".
func (ss *sides) frozen() string {
	for _, s := range ss.all() {
		if s.frozen != nil {
			return fmt.Sprintf("cluster: %s could not be unpaused: %v", s.Role, s.frozen)
		}
	}
	return ""
}

// runPair is one pair of Design §5.1: the canary gate, then the cases. It
// returns the reason the session must stop ("budget", "lease", "signal"),
// or "" when the session goes on to the next pair -- which includes a pair
// that could not stand its clusters up (p.Skipped says so) and a pair that
// ran part-way (p.written says the sidecar is there).
func (s *Session) runPair(ctx context.Context, p *SessionPair, canaries []*Case) string {
	start := time.Now()
	pairDir := filepath.Join(s.Out, p.Name)
	canaryDir := filepath.Join(pairDir, "canary")
	overlapDir := filepath.Join(pairDir, "overlap")
	s.logf("pair %s: start", p.Name)
	ss := &sides{}
	defer func() {
		p.Seconds = int(time.Since(start).Seconds())
		for _, side := range ss.all() {
			s.keepLogs(side, pairDir)
			if s.Keep {
				s.logf("kept %s (--keep)", side.Name)
				continue
			}
			if err := s.destroySide(side); err != nil {
				s.note("pair %s: teardown: %v", p.Name, err)
			}
		}
	}()
	ctxStop := func() string {
		if ctx.Err() != nil {
			return "signal"
		}
		return ""
	}
	// The cases that share a conf key share a cluster; a cluster is created
	// with every case of its key so it serves the canaries and the cases alike.
	byKey := map[string][]*Case{}
	for _, c := range append(append([]*Case{}, canaries...), p.selected...) {
		k := confKey(c)
		if !containsCase(byKey[k], c) {
			byKey[k] = append(byKey[k], c)
		}
	}
	// ensure has a side up on the build for the conf key, recreating it when
	// the key changed; dir is where its describe artifact and logs go.
	ensure := func(slot **Side, role, suffix, build, key, dir string) error {
		if *slot != nil && (*slot).confKey == key {
			return nil
		}
		if *slot != nil {
			s.keepLogs(*slot, dir)
			if err := s.destroySide(*slot); err != nil {
				return err
			}
			*slot = nil
		}
		side, err := s.createSide(ctx, role, suffix, build, byKey[key])
		if side != nil {
			*slot = side
			if !containsStr(p.Clusters, side.Name) {
				p.Clusters = append(p.Clusters, side.Name)
			}
			if raw, derr := side.CLI.DescribeRaw(ctx); derr == nil {
				_ = os.MkdirAll(filepath.Join(dir, "clusters"), 0o755)
				_ = os.WriteFile(filepath.Join(dir, "clusters", side.Name+".describe.json"), append(raw, '\n'), 0o644)
			}
		}
		if err != nil {
			return err
		}
		return s.buildFixtures(ctx, side, s.Suite, byKey[key])
	}

	// Canaries: the target's cluster stood up on the reference build, an A/A
	// across the two clusters (FR-3, D9).
	var canaryEntries []CaseEntry
	canaryVerdicts := map[string]Verdict{}
	stop := ""
	for _, g := range confGroups(canaries) {
		err := ensure(&ss.r, "reference", "r", p.Reference.Build, g.Key, pairDir)
		if err == nil {
			err = ensure(&ss.t, "target", "t", p.Reference.Build, g.Key, canaryDir)
		}
		if err != nil {
			s.note("pair %s: cluster: %v", p.Name, err)
			if stop = ctxStop(); stop == "" {
				p.Skipped = "cluster: " + err.Error()
			}
			break
		}
		res, st := s.runGroup(ctx, p, g.Cases, ss.t, ss.r, nil, canaryDir, true)
		for _, cr := range res {
			canaryEntries = append(canaryEntries, cr.entry)
			canaryVerdicts[cr.entry.ID] = cr.verdict
			p.Canaries = append(p.Canaries, canaryOf(cr, s.Conf.CanaryTolerance))
		}
		if st != "" {
			stop = st
			break
		}
		if why := ss.frozen(); why != "" {
			s.note("pair %s: %s", p.Name, why)
			p.Skipped = why
			break
		}
	}
	p.Valid = stop == "" && p.Skipped == "" && len(p.Canaries) == len(canaries)
	for _, cn := range p.Canaries {
		p.Valid = p.Valid && cn.OK
	}
	s.writeCanarySidecar(p, canaryDir, canaryEntries, canaryVerdicts)
	if ss.t != nil {
		s.keepLogs(ss.t, canaryDir)
		if err := s.destroySide(ss.t); err != nil {
			s.note("pair %s: teardown of the canary cluster: %v", p.Name, err)
		}
		ss.t = nil
	}
	if stop != "" || p.Skipped != "" {
		// The session stopped, or the clusters failed, before a case ran:
		// the cases are on record as skipped for that reason, the pair is
		// not "invalid" -- nothing judged it.
		why := stop
		if why == "" {
			why = p.Skipped
		}
		if stop != "" {
			p.Skipped = stop
		}
		var entries []CaseEntry
		for _, c := range p.selected {
			entries = append(entries, skippedEntry(c, why))
		}
		p.CasesSkipped = len(entries)
		p.entries = entries
		s.writeSidecar(p, p.Name, pairDir, p.Reference, false, entries, map[string]Verdict{})
		return stop
	}
	if !p.Valid {
		why := "canary outside the tolerance"
		for _, cn := range p.Canaries {
			if !cn.OK {
				why = fmt.Sprintf("canary %s: %s", cn.ID, cn.Reason)
				break
			}
		}
		s.note("pair %s: invalid (%s); its %d case(s) are skipped", p.Name, why, len(p.selected))
		var entries []CaseEntry
		for _, c := range p.selected {
			entries = append(entries, skippedEntry(c, why))
		}
		p.CasesSkipped = len(entries)
		p.entries = entries
		s.writeSidecar(p, p.Name, pairDir, p.Reference, false, entries, map[string]Verdict{})
		p.written = true
		return ""
	}

	// The cases, A/B, in this session's order, by conf group. A group whose
	// clusters cannot be stood up skips its cases and the ones after it;
	// what was judged before stays on record.
	order := shuffled(p.selected, seedFrom(s.SessionID+"/"+p.Name))
	var entries, overlapEntries []CaseEntry
	verdicts, overlapVerdicts := map[string]Verdict{}, map[string]Verdict{}
	broken := ""
	skipGroup := func(g confGroup, why string) {
		for _, c := range g.Cases {
			entries = append(entries, skippedEntry(c, why))
			if p.Overlap != nil {
				overlapEntries = append(overlapEntries, skippedEntry(c, why))
			}
		}
	}
	for _, g := range confGroups(order) {
		if stop != "" || broken != "" {
			skipGroup(g, firstNonEmptyStr(stop, broken))
			continue
		}
		if why := s.groupMemory(g, p.Overlap != nil); why != "" {
			s.note("pair %s: conf group %q: %s", p.Name, g.Key, why)
			skipGroup(g, why)
			continue
		}
		var err error
		if err = ensure(&ss.t, "target", "t", p.Target.Build, g.Key, pairDir); err == nil {
			if err = ensure(&ss.r, "reference", "r", p.Reference.Build, g.Key, pairDir); err == nil && p.Overlap != nil {
				err = ensure(&ss.o, "overlap", "o", p.Overlap.Build, g.Key, overlapDir)
			}
		}
		if err != nil {
			s.note("pair %s: cluster: %v", p.Name, err)
			if ctx.Err() != nil {
				stop = "signal"
			} else {
				broken = "cluster: " + err.Error()
			}
			skipGroup(g, firstNonEmptyStr(stop, broken))
			continue
		}
		res, st := s.runGroup(ctx, p, g.Cases, ss.t, ss.r, []*Side{ss.o}, pairDir, false)
		for _, cr := range res {
			entries = append(entries, cr.entry)
			verdicts[cr.entry.ID] = cr.verdict
		}
		if ss.o != nil && (st != "" || ss.frozen() != "") {
			for _, c := range g.Cases {
				overlapEntries = append(overlapEntries, skippedEntry(c, firstNonEmptyStr(st, ss.frozen())))
			}
		}
		if st == "" && ss.o != nil && ss.frozen() == "" {
			// FR-29: the same target against the second reference, with the
			// first reference idle -- its fixture servers down as well.
			if err := s.stopOthers(ctx, ss.r, nil); err != nil {
				s.note("pair %s: stopping the reference's servers for the overlap: %v", p.Name, err)
			}
			ores, ost := s.runGroup(ctx, p, g.Cases, ss.t, ss.o, []*Side{ss.r}, overlapDir, false)
			for _, cr := range ores {
				overlapEntries = append(overlapEntries, cr.entry)
				overlapVerdicts[cr.entry.ID] = cr.verdict
			}
			st = ost
		}
		if st != "" {
			stop = st
		} else if why := ss.frozen(); why != "" {
			s.note("pair %s: %s", p.Name, why)
			broken = why
		}
	}
	for _, e := range entries {
		switch e.Status {
		case StatusOK:
			p.CasesRun++
			if e.Flag != FlagNone {
				p.Flags++
			}
		case StatusNull:
			p.CasesRun++
			p.CasesNull++
		default:
			p.CasesSkipped++
		}
	}
	p.entries, p.overlapEntries = entries, overlapEntries
	s.writeSidecar(p, p.Name, pairDir, p.Reference, true, entries, verdicts)
	if p.Overlap != nil {
		s.writeSidecar(p, p.Name+"+overlap", overlapDir, *p.Overlap, true, overlapEntries, overlapVerdicts)
	}
	p.written = true
	if broken != "" && p.CasesRun == 0 {
		p.Skipped = broken
	}
	s.logf("pair %s: done in %s: %d run, %d null, %d skipped, %d flag(s)", p.Name, time.Since(start).Round(time.Second), p.CasesRun, p.CasesNull, p.CasesSkipped, p.Flags)
	return stop
}

// groupMemory is Design §7's check: the group's data_buffer_size on every
// side at once against memory_cap.
func (s *Session) groupMemory(g confGroup, overlap bool) string {
	if s.Conf.MemoryCap == "" || len(g.Cases) == 0 {
		return ""
	}
	capBytes, err := parseSize(s.Conf.MemoryCap)
	if err != nil {
		return ""
	}
	dbs := g.Cases[0].Conf["data_buffer_size"]
	if dbs == "" {
		return ""
	}
	per, err := parseSize(dbs)
	if err != nil {
		return ""
	}
	sides := int64(2)
	if overlap {
		sides = 3
	}
	if per*sides > capBytes {
		return fmt.Sprintf("memory_cap: %d × data_buffer_size=%s exceeds %s", sides, dbs, s.Conf.MemoryCap)
	}
	return ""
}

type caseRun struct {
	entry   CaseEntry
	verdict Verdict
}

// runGroup is Design §5.1's runGroup: the cases in order, each with the
// lease and the budget checked first, the fixture servers switched, the
// case measured (the idle sides paused), re-measured when the pairs
// disagree (FR-20.1), judged and written. It returns what it has and the
// session-level reason it stopped, if it did; a side that could not be
// unpaused stops the group with the rest of its cases skipped, and the
// caller reads the sides' frozen marks.
func (s *Session) runGroup(ctx context.Context, p *SessionPair, cases []*Case, t, ref *Side, idle []*Side, dir string, canary bool) ([]caseRun, string) {
	var out []caseRun
	stop, broken := "", ""
	for i, c := range cases {
		if stop == "" && broken == "" {
			switch {
			case ctx.Err() != nil:
				stop = "signal"
			case !s.leaseHeld():
				s.note("pair %s: the lease is no longer run %s's; %d case(s) not run (L7)", p.Name, s.SessionID, len(cases)-i)
				stop = "lease"
			case !s.Clock.fits(time.Now(), caseBound(c)):
				s.note("pair %s: %s left, %s needs %s; the rest is skipped (FR-5)", p.Name, s.Clock.left(time.Now()).Round(time.Minute), c.ID, caseBound(c).Round(time.Minute))
				stop = "budget"
			}
		}
		if stop != "" || broken != "" {
			out = append(out, caseRun{entry: skippedEntry(c, firstNonEmptyStr(stop, broken)), verdict: Verdict{Status: StatusSkipped, Flag: FlagNone}})
			continue
		}
		f := s.Suite.Fixtures[c.Fixture.Name]
		switchErr := error(nil)
		for _, side := range []*Side{t, ref} {
			if err := s.startFixtureServer(ctx, side, f); err != nil {
				switchErr = err
				break
			}
			if err := s.stopOthers(ctx, side, f); err != nil {
				switchErr = err
				break
			}
		}
		if switchErr != nil {
			s.note("pair %s: %s: fixture server: %v", p.Name, c.ID, switchErr)
			if ctx.Err() != nil {
				stop = "signal"
			}
			out = append(out, caseRun{entry: skippedEntry(c, "cluster: "+switchErr.Error()), verdict: Verdict{Status: StatusSkipped, Flag: FlagNone}})
			continue
		}
		cr := s.runCase(ctx, c, f, t, ref, idle...)
		reps := s.repeats(c)
		v := judge(c, cr, reps)
		if !canary && v.Status == StatusOK && v.Ratio != nil && !v.Confirmed && outsideTolerance(*v.Ratio, c.Tolerance) {
			if s.Clock.fits(time.Now(), remeasureBound(c)) && ctx.Err() == nil && t.frozen == nil && ref.frozen == nil {
				s.logf("%s: ratio %.4f outside the tolerance without agreement; re-measuring %d pairs (FR-20.1)", c.ID, *v.Ratio, remeasurePairs)
				s.measurePasses(ctx, cr, c, f, t, ref, reps+1, reps+remeasurePairs, idle...)
				reps += remeasurePairs
				v = judge(c, cr, reps)
			} else {
				v.Why += "; no budget to re-measure"
			}
		}
		for _, ps := range cr.Passes {
			side := t
			if ps.Side == ref.Role {
				side = ref
			}
			s.keepPassFiles(side, c, ps.Phase, ps.Rep, dir)
		}
		counters := countersFile{Case: c.ID, Version: c.Version, Roles: []string{"server", "broker", "cas", "client"}, Passes: cr.Passes, PerOp: v.Counters}
		if err := writeJSON(filepath.Join(dir, c.ID, "counters.json"), &counters); err != nil {
			s.logf("counters.json: %v", err)
		}
		e := caseEntry(c, cr, v)
		if canary {
			// A canary is an A/A: its verdict is the pair's validity, read
			// by canaryOf with canary_tolerance, never a flag on the case.
			e.Flag, e.Tolerance = FlagNone, s.Conf.CanaryTolerance
			if v.Flag != FlagNone && e.Reason == "" {
				e.Reason = "A/A"
			}
			s.logf("canary %s: ratio %s status %s", c.ID, fmtPtr(v.Ratio), v.Status)
		} else {
			s.logf("%s: ratio %s flag %s status %s (%d pairs, confirmed %t)", c.ID, fmtPtr(v.Ratio), v.Flag, v.Status, len(v.Pairs), v.Confirmed)
		}
		out = append(out, caseRun{entry: e, verdict: v})
		for _, side := range append([]*Side{t, ref}, idle...) {
			if side != nil && side.frozen != nil {
				broken = fmt.Sprintf("cluster: %s could not be unpaused: %v", side.Role, side.frozen)
				s.note("pair %s: %s; %d case(s) not run", p.Name, broken, len(cases)-i-1)
				break
			}
		}
	}
	return out, stop
}

func outsideTolerance(ratio, tol float64) bool {
	return ratio > 1+tol || ratio < 1-tol
}

// canaryOf is FR-3's reading of a canary's verdict: ok when measured and
// within canary_tolerance of 1.
func canaryOf(cr caseRun, tol float64) CanaryResult {
	cn := CanaryResult{ID: cr.entry.ID, Ratio: cr.verdict.Ratio, Tolerance: tol, Status: cr.verdict.Status}
	switch {
	case cr.verdict.Status != StatusOK || cr.verdict.Ratio == nil:
		cn.Reason = "status " + cr.verdict.Status
		if cr.entry.Reason != "" {
			cn.Reason += ": " + cr.entry.Reason
		}
	case math.Abs(*cr.verdict.Ratio-1) > tol:
		cn.Reason = fmt.Sprintf("ratio %.4f is outside 1 ± %.3f", *cr.verdict.Ratio, tol)
	default:
		cn.OK = true
	}
	return cn
}

func skippedEntry(c *Case, reason string) CaseEntry {
	return CaseEntry{
		ID: c.ID, Version: c.Version, Metric: c.Metric, Op: c.Op,
		Target: SideValues{Unit: metricUnit(c.Metric), Values: []*float64{}}, Reference: SideValues{Unit: metricUnit(c.Metric), Values: []*float64{}},
		Pairs: []float64{}, Tolerance: c.Tolerance, Flag: FlagNone, Counters: map[string]counterPair{}, Status: StatusSkipped, Reason: reason,
	}
}

func (s *Session) writeSidecar(p *SessionPair, pairName, dir string, reference BuildRef, valid bool, entries []CaseEntry, verdicts map[string]Verdict) {
	if entries == nil {
		entries = []CaseEntry{}
	}
	sc := Sidecar{
		Schema: "perf-regression/1", Session: s.SessionID, Pair: pairName,
		Target: p.Target, Reference: reference, Machine: s.doc.Host,
		CPUSet: s.doc.CPUSet, Pinning: s.doc.Pinning, Interleave: s.doc.Interleave, ClientImage: s.doc.ClientImage,
		SessionValid: valid, Cases: entries,
	}
	if err := writeJSON(filepath.Join(dir, "regression-case.json"), &sc); err != nil {
		s.note("pair %s: sidecar: %v", pairName, err)
	}
	if err := writeCasesCSV(filepath.Join(dir, "cases.csv"), s.SessionID, pairName, entries, verdicts); err != nil {
		s.logf("cases.csv: %v", err)
	}
}

// writeCanarySidecar is the canaries' own sidecar: pair "canary", both
// sides the reference build, so cbingest posts the ratio alone (§4.3).
func (s *Session) writeCanarySidecar(p *SessionPair, dir string, entries []CaseEntry, verdicts map[string]Verdict) {
	if entries == nil {
		entries = []CaseEntry{}
	}
	valid := len(entries) > 0
	for _, cn := range p.Canaries {
		valid = valid && cn.OK
	}
	sc := Sidecar{
		Schema: "perf-regression/1", Session: s.SessionID, Pair: "canary", CanaryOf: p.Name,
		Target: p.Reference, Reference: p.Reference, Machine: s.doc.Host,
		CPUSet: s.doc.CPUSet, Pinning: s.doc.Pinning, Interleave: s.doc.Interleave, ClientImage: s.doc.ClientImage,
		SessionValid: valid, Cases: entries,
	}
	if err := writeJSON(filepath.Join(dir, "regression-case.json"), &sc); err != nil {
		s.note("pair %s: canary sidecar: %v", p.Name, err)
	}
	if err := writeCasesCSV(filepath.Join(dir, "cases.csv"), s.SessionID, "canary", entries, verdicts); err != nil {
		s.logf("cases.csv: %v", err)
	}
}

func containsCase(list []*Case, c *Case) bool {
	for _, x := range list {
		if x == c {
			return true
		}
	}
	return false
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// sortedKeys is for deterministic output of a map.
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
