package hareplsuite

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/cli"
	"github.com/cubrid-systems/cubrid-testkit/internal/runner"
	"github.com/cubrid-systems/cubrid-testkit/internal/sandbox"
	"github.com/cubrid-systems/cubrid-testkit/internal/status"
)

// HARepl runs the ha_repl task against a sandbox cluster.
type HARepl struct{}

// New returns the runner for ha_repl.
func New() *HARepl { return &HARepl{} }

func (*HARepl) Tasks() []cli.Task { return []cli.Task{cli.HARepl} }

// exitSetup is the frozen "could not start" code. A run that found differences
// still exits 0, as every other suite here does
// (external-surface-freeze.md §6-1).
const exitSetup = 1

func setupFailed(format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	fmt.Println(msg)
	return &runner.ExitError{Code: exitSetup, Err: fmt.Errorf("%s", msg)}
}

// ClusterKey names the cluster in the configuration file. ClusterEnv overrides
// it, and is the spelling internal/sandbox's own live tests already use.
const (
	ClusterKey = "sandbox_cluster"
	ClusterEnv = "TESTKIT_CSB_CLUSTER"
	// WaitKey is CTP's own name for the bound on the replication wait, reused
	// rather than reinvented: it is the frozen surface (ADR-003) and it already
	// means exactly this.
	WaitKey = "ha_sync_detect_timeout_in_ms"
	// ResetKey is when the database is emptied: before every case, which is
	// what makes a run reproducible, or once per directory, which is the sql
	// corpus's own contract that a directory is the unit whose cases may rely
	// on each other. Per case by default -- a suite whose answer depends on
	// what the previous case left cannot be used to judge anything, and the
	// cases that genuinely share state are rare enough to be named.
	ResetKey = "reset"
	// AddKeyKey turns on the conversion: a CREATE TABLE with no primary key
	// gets a column of its own to be one, and the case's positional INSERTs
	// are rewritten to name their columns around it. Off by default -- the
	// unconverted run is the baseline the conversion has to be measured
	// against.
	AddKeyKey = "add_primary_key"
	// ResumeKey picks up where a run stopped. Off by default: a run that
	// silently continued someone else's would be the reproducibility problem
	// this suite spent a week removing, in a new place.
	//
	// It exists because a run here is long enough to be interrupted by things
	// that have nothing to do with it. Twice on 2026-09-22 every CUBRID
	// process on the host stopped in the same millisecond -- both sandbox
	// clusters and the host's own -- 1,228 cases into 3,327, and the second
	// time seventeen seconds in. A rootless container's processes are the
	// invoking user's, so anything that stops CUBRID by walking the process
	// table reaches inside (evidence/ha/where-this-stands.md §5).
	//
	// A case whose verdict was about the run rather than about the case --
	// `wait_timeout`, `case_failed` -- is never resumed. Those are exactly the
	// ones the interruption produced.
	ResumeKey = "resume"
	// StatusKey is the page, off unless asked for, and never on standard
	// output -- what the runner prints there is the frozen surface the
	// equivalence comparison reads (ADR-003).
	StatusKey = "status_http"
)

func clusterOf(cfgCluster string) string {
	if env := strings.TrimSpace(os.Getenv(ClusterEnv)); env != "" {
		return env
	}
	return strings.TrimSpace(cfgCluster)
}

func (s *HARepl) Validate(req runner.Request) error {
	cfg, err := req.Home.Load(req.ConfigPath)
	if err != nil {
		return setupFailed("please confirm your conf file path!")
	}
	if clusterOf(cfg.GetOr(ClusterKey, "")) == "" {
		return setupFailed("no cluster: set %s in %s, or %s in the environment",
			ClusterKey, req.ConfigPath, ClusterEnv)
	}
	scenario := cfg.GetOr("scenario", "")
	if st, serr := os.Stat(scenario); scenario == "" || serr != nil || !st.IsDir() {
		return setupFailed("scenario is not a directory: %q", scenario)
	}
	return nil
}

func (s *HARepl) Run(ctx context.Context, req runner.Request) error {
	cfg, err := req.Home.Load(req.ConfigPath)
	if err != nil {
		return setupFailed("please confirm your conf file path!")
	}
	scenario := cfg.GetOr("scenario", "")
	wait := time.Duration(cfg.Int(WaitKey, 60000)) * time.Millisecond
	// Off by default: the unconverted run is the baseline the conversion has
	// to be measured against, and a switch that is on by default hides it.
	addKey := cfg.Bool(AddKeyKey, false)
	resetEvery := strings.ToLower(strings.TrimSpace(cfg.GetOr(ResetKey, "case")))

	names := clustersOf(cfg.GetOr(ClusterKey, ""))
	if len(names) == 0 {
		return setupFailed("no cluster: set %s in %s, or %s in the environment",
			ClusterKey, req.ConfigPath, ClusterEnv)
	}
	shards := make([]*runShard, 0, len(names))
	for _, name := range names {
		c := sandbox.Bind(name)
		if aerr := c.Available(ctx); aerr != nil {
			return setupFailed("cluster %q cannot be reached: %v", name, aerr)
		}
		pair, perr := sandbox.Describe(ctx, c, req.Home.Path)
		if perr != nil {
			return setupFailed("cluster %q did not describe itself as a pair: %v", name, perr)
		}
		if pair.DB == "" {
			return setupFailed("cluster %q names no database, so there is nothing to replicate", name)
		}
		shards = append(shards, &runShard{cluster: name, cli: c, pair: pair})
	}

	cases, cerr := Cases(scenario)
	if cerr != nil {
		return setupFailed("%v", cerr)
	}
	if len(cases) == 0 {
		return setupFailed("no case under %s", scenario)
	}
	deal(shards, scenario, cases)

	fmt.Printf("ha_repl: %d case(s) from %s\n", len(cases), scenario)
	for _, sh := range shards {
		for _, line := range pairBanner(ctx, sh.cli, sh.cluster, sh.pair) {
			fmt.Println(line)
		}
		if len(shards) > 1 {
			fmt.Printf("           %d case(s)\n", len(sh.cases))
		}
	}

	// Where a difference is kept so it can be read rather than believed.
	keepDir := cfg.GetOr("difference_dir", filepath.Join(filepath.Dir(req.ConfigPath), "ha_repl_differences"))

	// The verdicts, one line each, written as they happen so that a run which
	// is interrupted still has everything it had judged.
	ledger, lerr := openLedger(keepDir)
	if lerr != nil {
		fmt.Printf("  ! could not keep the verdicts: %v\n", lerr)
	}
	defer ledger.Close()
	var done map[string]Result
	if cfg.Bool(ResumeKey, false) {
		done = ledger.Judged()
		if len(done) > 0 {
			fmt.Printf("  resuming: %d case(s) already judged\n", len(done))
		}
	}

	// The page is opened here rather than above, because what it counts is the
	// cases this run will judge. A resumed case is skipped without being begun
	// or ended, so counting it in the total leaves the page permanently short
	// -- 14 done of 3,327 on a run that had already judged 582.
	toJudge := 0
	for _, path := range cases {
		rel, _ := filepath.Rel(scenario, path)
		if _, already := done[rel]; !already {
			toJudge++
		}
	}
	board, closeBoard := openBoard(ctx, cfg, shards, scenario, wait, addKey, resetEvery, toJudge)
	defer closeBoard()

	run := &shardedRun{
		scenario: scenario, keepDir: keepDir, wait: wait, addKey: addKey,
		resetEvery: resetEvery, done: done, ledger: ledger, board: board,
		prefix: len(shards) > 1,
	}
	var wg sync.WaitGroup
	for _, sh := range shards {
		wg.Add(1)
		go func(sh *runShard) {
			defer wg.Done()
			run.shard(ctx, sh)
		}(sh)
	}
	wg.Wait()

	if keepDir != "" {
		if err := keepEmptyAgreements(keepDir, run.results); err != nil {
			fmt.Printf("  ! could not record the empty agreements: %v\n", err)
		}
		if err := keepStranded(keepDir, run.stranded); err != nil {
			fmt.Printf("  ! could not record what a slave was left holding: %v\n", err)
		}
	}
	report(run.results)
	if len(shards) > 1 {
		reportShards(shards, run.results)
	}
	if run.resumed > 0 {
		fmt.Printf("  %d of those were resumed from an earlier run, and their statements are not in the counts above\n", run.resumed)
	}
	if run.leftovers > 0 {
		fmt.Printf("  %d case(s) started from a state a reset could not clear\n", run.leftovers)
	}
	reportStranded(run.stranded, keepDir)
	return nil
}

// runShard is one pair and the cases dealt to it.
type runShard struct {
	cluster string
	cli     *sandbox.CLI
	pair    *sandbox.Pair
	cases   []string
}

// shardedRun is what every shard shares: where the output goes, and the tally.
//
// Everything a case touches is its own shard's -- its pair, its database, its
// reset -- so the only sharing is the record. It is behind one lock because
// interleaved lines in a ledger are the failure this suite has already had
// once, in test_<env>.log, and the cost is a mutex held for one write.
type shardedRun struct {
	scenario, keepDir, resetEvery string
	wait                          time.Duration
	addKey                        bool
	done                          map[string]Result
	ledger                        *ledger
	board                         *status.Board
	prefix                        bool

	mu        sync.Mutex
	results   []Result
	stranded  []strandedAt
	resumed   int
	leftovers int
}

// say prints one line, naming the shard when there is more than one. Two shards
// printing a case at once would otherwise produce a line belonging to neither.
func (r *shardedRun) say(sh *runShard, format string, a ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.prefix {
		fmt.Printf("[%s] ", sh.cluster)
	}
	fmt.Printf(format, a...)
}

// shard runs the cases dealt to one pair. It is the whole of what a run used to
// be, and a run with one cluster still is exactly this, once.
func (r *shardedRun) shard(ctx context.Context, sh *runShard) {
	lastDir, lastCase := "", "the state the run started from"
	for _, path := range sh.cases {
		if ctx.Err() != nil {
			return
		}
		dir := filepath.Dir(path)
		rel, _ := filepath.Rel(r.scenario, path)
		if prior, already := r.done[rel]; already {
			// Nothing ran, so there is nothing to reset and nothing a slave
			// could have been left holding by it.
			r.mu.Lock()
			r.results = append(r.results, prior)
			r.resumed++
			r.mu.Unlock()
			lastDir, lastCase = dir, rel
			continue
		}
		if r.resetEvery == "case" || dir != lastDir {
			out, rerr := Reset(ctx, sh.pair)
			if rerr != nil {
				r.say(sh, "  ! could not reset the database: %v\n", rerr)
			}
			if len(out.Left) > 0 {
				r.say(sh, "  ! %s\n", ResetNote(out.Left))
				r.mu.Lock()
				r.leftovers++
				r.mu.Unlock()
			}
			// A slave holding what the master does not is a finding and not
			// housekeeping: something the master did did not arrive, or
			// arrived and could not be undone by name. It is named, attributed
			// to the case that left it, and written down -- the repair keeps
			// the next case honest, the record keeps this one.
			for _, st := range out.Stranded {
				r.say(sh, "  ! %s, left by %s\n", st.String(), lastCase)
				r.mu.Lock()
				r.stranded = append(r.stranded, strandedAt{Case: lastCase, S: st})
				r.mu.Unlock()
			}
		}
		lastDir = dir
		sql, rerr := os.ReadFile(path)
		if rerr != nil {
			r.mu.Lock()
			r.results = append(r.results, Result{Case: path, Outcome: CaseFailed, Detail: rerr.Error()})
			r.mu.Unlock()
			continue
		}
		r.board.Begin(sh.cluster, rel)
		began := time.Now()
		res := RunCase(ctx, sh.pair, rel, string(sql), r.wait, r.keepDir, r.addKey)
		res.Took = time.Since(began)
		r.board.End(sh.cluster, rel, ok(res.Outcome))
		r.mu.Lock()
		r.results = append(r.results, res)
		r.mu.Unlock()
		r.ledger.Write(res, res.Took)
		lastCase = rel
		r.say(sh, "  %-15s %s%s\n", res.Outcome, rel, detailSuffix(res))
	}
}

// clustersOf reads the clusters a run drives.
//
// One name is a run as it has always been. Several, comma-separated, is the
// same corpus over several pairs at once -- which is a thing this runner does
// itself rather than a thing an operator does by starting it eight times. The
// front end is one test at a time by design: a result tree takes one run's lock
// (internal/result/lock.go), and eight processes would be eight runs arguing
// over one record. Inside one process there is one record, one page and one
// ledger, and the pairs are the only thing that is eight.
func clustersOf(cfgValue string) []string {
	raw := cfgValue
	if env := strings.TrimSpace(os.Getenv(ClusterEnv)); env != "" {
		raw = env
	}
	var out []string
	seen := map[string]bool{}
	for _, name := range strings.Split(raw, ",") {
		if name = strings.TrimSpace(name); name != "" && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

// deal spreads the corpus over the shards, a directory at a time.
//
// By directory and not by case, because the sql corpus's own contract is that a
// directory is the unit whose cases may rely on each other -- `reset=dir` says
// so outright, and even under `reset=case` the cases of one directory share
// names and shapes. Splitting a directory across two pairs would have one
// shard's `create table t1` meet another's leftovers on a different machine,
// which is a difference the run would report as the engine's.
//
// Round robin over directories sorted by size, largest first, so that one
// directory of 1,500 cases does not become one shard's whole afternoon while
// the other seven finish.
func deal(shards []*runShard, scenario string, cases []string) {
	if len(shards) == 1 {
		shards[0].cases = cases
		return
	}
	byDir := map[string][]string{}
	var order []string
	for _, path := range cases {
		dir := filepath.Dir(path)
		if _, seen := byDir[dir]; !seen {
			order = append(order, dir)
		}
		byDir[dir] = append(byDir[dir], path)
	}
	sort.SliceStable(order, func(i, j int) bool {
		return len(byDir[order[i]]) > len(byDir[order[j]])
	})
	for _, dir := range order {
		// The shard with the fewest cases takes the next directory, which is a
		// better balance than strict rotation when the directories differ by a
		// factor of a hundred, as this corpus's do.
		at := 0
		for i, sh := range shards {
			if len(sh.cases) < len(shards[at].cases) {
				at = i
			}
		}
		shards[at].cases = append(shards[at].cases, byDir[dir]...)
	}
}

// reportShards says what each pair did, because a run that is eight runs in a
// coat has to be readable as eight: a shard that judged nothing, or judged
// everything as wait_timeout, is a pair that died rather than a corpus that is
// clean.
func reportShards(shards []*runShard, results []Result) {
	byCase := map[string]Outcome{}
	for _, r := range results {
		byCase[r.Case] = r.Outcome
	}
	fmt.Println("\n  per pair:")
	for _, sh := range shards {
		counts := map[Outcome]int{}
		for _, path := range sh.cases {
			counts[byCase[filepath.Base(path)]]++
		}
		var parts []string
		for _, o := range []Outcome{Same, Differ, Replicating, Unreplicatable, NoData, Skipped, WaitTimeout, CaseFailed} {
			if counts[o] > 0 {
				parts = append(parts, fmt.Sprintf("%s %d", o, counts[o]))
			}
		}
		fmt.Printf("    %-10s %3d case(s)  %s\n", sh.cluster, len(sh.cases), strings.Join(parts, ", "))
	}
}

// strandedAt is one stranded object and the case that left it.
type strandedAt struct {
	Case string
	S    Stranded
}

// reportStranded says how often the pair diverged in a way the master's own
// reset could not undo, which is a different number from `differ` and worth
// its own line: `differ` is a case's reads disagreeing, this is the database
// underneath every case after it.
func reportStranded(stranded []strandedAt, keepDir string) {
	if len(stranded) == 0 {
		return
	}
	repaired, cases := 0, map[string]bool{}
	for _, s := range stranded {
		if s.S.Repaired {
			repaired++
		}
		cases[s.Case] = true
	}
	fmt.Printf("\n  %d object(s) were left on a slave by %d case(s), %d of them repaired\n",
		len(stranded), len(cases), repaired)
	for _, s := range stranded {
		fmt.Printf("    %-46s %s\n", s.Case, s.S.String())
	}
	if keepDir != "" {
		fmt.Printf("    written to %s\n", filepath.Join(keepDir, strandedFile))
	}
}

const strandedFile = "slave-stranded.tsv"

// keepStranded writes the record a reader needs after the run: which case left
// what on which node, and whether it was taken off again.
func keepStranded(dir string, stranded []strandedAt) error {
	if len(stranded) == 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString("case\tnode\tkind\tname\trepaired\twhy\n")
	for _, s := range stranded {
		fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%t\t%s\n",
			s.Case, s.S.Node, strings.ToLower(s.S.Word), s.S.Name, s.S.Repaired, s.S.Why)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, strandedFile), []byte(b.String()), 0o644)
}

// pairBanner says what the run is pointed at, one node to a line.
//
// The old one-liner was `master -> slave1,slave2, N case(s) from <path>`, and it
// had three problems that are one problem: everything on one line. A second
// slave pushed the scenario off the edge, the arrow said which way replication
// goes and nothing about whether either node was up, and the names carried the
// roles only by convention -- `-n1` is a master because it usually is.
//
// So: the count and the corpus on their own line, then one line per node, with
// **what it was built as first and what HA currently calls it second**. Those
// are different questions and they disagree exactly when something interesting
// has happened -- after a failover the node built as the master is `standby`.
//
// The HA state is asked for once, here. A cluster that will not answer is not a
// reason to refuse the run: the state is left out and the rest is printed,
// because the run's own oracle does not depend on it.
func pairBanner(ctx context.Context, c *sandbox.CLI, cluster string, pair *sandbox.Pair) []string {
	state := map[string]sandbox.NodeHealth{}
	if nodes, err := c.HAStatus(ctx); err == nil {
		for _, n := range nodes {
			state[n.Name] = n
		}
	}
	line := func(built, name string) string {
		s := fmt.Sprintf("  %-7s %-16s", built, name)
		n, known := state[name]
		switch {
		case !known:
			return s + "(state unread)"
		case !n.Live:
			return s + "(not live)"
		default:
			return s + "(" + n.Role + ", " + n.ServerState + ")"
		}
	}
	out := []string{fmt.Sprintf("  pair %s, database %s", cluster, pair.DB)}
	out = append(out, line("master", pair.Master))
	for _, slave := range pair.Slaves {
		out = append(out, line("slave", slave))
	}
	return out
}

// ok is what the page counts as a pass. A difference is the finding this suite
// exists for and still counts against, because the page's question is "is
// anything wrong", not "did the suite work".
func ok(o Outcome) bool {
	return o != Differ && o != WaitTimeout && o != CaseFailed
}

func detailSuffix(r Result) string {
	switch r.Outcome {
	case Differ:
		return "  <- " + r.Differing + " on " + r.Node
	case Unreplicatable:
		return "  <- " + strings.Join(r.Tables, ", ")
	case WaitTimeout, CaseFailed, SessionDiffers:
		return "  <- " + r.Detail
	}
	return ""
}

// keepEmptyAgreements writes down which reads agreed about nothing.
func keepEmptyAgreements(dir string, results []Result) error {
	var b strings.Builder
	for _, r := range results {
		for _, stmt := range r.Empty {
			fmt.Fprintf(&b, "%s\t%s\n", r.Case, stmt)
		}
	}
	if b.Len() == 0 {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "empty-agreements.tsv"), []byte(b.String()), 0o644)
}

// report prints the tally. There is no comparison against CTP here and none is
// implied: this suite's verdict is the pair's own, so the summary is what the
// pair said and not how closely it agreed with another runner.
func report(results []Result) {
	by := map[Outcome]int{}
	var waited time.Duration
	for _, r := range results {
		by[r.Outcome]++
		waited += r.Waited
	}
	fmt.Println()
	fmt.Printf("ha_repl: %d case(s)\n", len(results))
	for _, o := range []Outcome{Same, Differ, Replicating, Unreplicatable, SessionDiffers, NoData, Skipped, WaitTimeout, CaseFailed} {
		if by[o] > 0 {
			fmt.Printf("  %-15s %d\n", o, by[o])
		}
	}
	var stmts, cmp int
	for _, r := range results {
		stmts += r.Statements
		cmp += r.Compared
	}
	var skipped, unordered, converted, failed, empty, objs, kdup, knull, expected, sessions int
	for _, r := range results {
		sessions += r.SessionDiffered
		skipped += r.Unreplicated
		unordered += r.Unordered
		converted += r.Converted
		failed += r.WriteFailed
		empty += r.EmptyAgreement
		objs += r.ObjectDomain
		kdup += r.KeyDuplicate
		knull += r.KeyNull
		expected += r.EmptyExpected
	}
	fmt.Printf("  %d statement(s), %d read(s) compared, %d skipped for want of a primary key, %d agreeing only as a set\n",
		stmts, cmp, skipped, unordered)
	if converted > 0 {
		fmt.Printf("  %d statement(s) rewritten to carry a generated primary key\n", converted)
	}
	if objs > 0 {
		fmt.Printf("  %d read(s) skipped for an object-domain column, whose reference is not replicated\n", objs)
	}
	if sessions > 0 {
		fmt.Printf("  %d read(s) skipped because the two nodes could not be put in the same session\n", sessions)
	}
	fmt.Printf("  %d write(s) the engine refused", failed)
	if kdup+knull > 0 {
		fmt.Printf(" (%d on a generated primary key, %d on a NOT NULL constraint)", kdup, knull)
	}
	fmt.Println()
	fmt.Printf("  %d of the agreeing read(s) returned no rows on the master either", empty)
	if empty > 0 {
		fmt.Printf(" (%d in a case that was never going to show a row)", expected)
	}
	fmt.Println()
	fmt.Printf("  waited      %s in total, never slept\n", waited.Round(time.Millisecond))
	if by[Differ] > 0 {
		fmt.Println("\nthe pair disagreed with itself on:")
		for _, r := range results {
			if r.Outcome == Differ {
				fmt.Printf("  %s  %s on %s\n", r.Case, r.Differing, r.Node)
				if r.Kept != "" {
					fmt.Printf("      both answers: %s\n", r.Kept)
				}
			}
		}
	}
}

// Cases finds every case under a scenario, in the sql corpus's layout:
// <dir>/cases/<name>.sql, with answers/ and everything else left alone.
func Cases(scenario string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(scenario, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // an unreadable directory is skipped, as shell's discovery does
		}
		if d.IsDir() || !strings.HasSuffix(path, ".sql") {
			return nil
		}
		if filepath.Base(filepath.Dir(path)) != "cases" {
			return nil
		}
		out = append(out, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", scenario, err)
	}
	sort.Strings(out)
	return out, nil
}
