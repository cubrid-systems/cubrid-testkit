package hareplsuite

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/cli"
	"github.com/cubrid-systems/cubrid-testkit/internal/runner"
	"github.com/cubrid-systems/cubrid-testkit/internal/sandbox"
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
	name := clusterOf(cfg.GetOr(ClusterKey, ""))
	scenario := cfg.GetOr("scenario", "")
	wait := time.Duration(cfg.Int(WaitKey, 60000)) * time.Millisecond
	// Off by default: the unconverted run is the baseline the conversion has
	// to be measured against, and a switch that is on by default hides it.
	addKey := cfg.Bool(AddKeyKey, false)
	resetEvery := strings.ToLower(strings.TrimSpace(cfg.GetOr(ResetKey, "case")))

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

	cases, cerr := Cases(scenario)
	if cerr != nil {
		return setupFailed("%v", cerr)
	}
	if len(cases) == 0 {
		return setupFailed("no case under %s", scenario)
	}

	fmt.Printf("ha_repl: %s -> %s, %d case(s) from %s\n",
		pair.Master, strings.Join(pair.Slaves, ","), len(cases), scenario)

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
	board, closeBoard := openBoard(ctx, cfg, c, pair, name, scenario, wait, addKey, resetEvery, toJudge)
	defer closeBoard()

	var results []Result
	var stranded []strandedAt
	resumed := 0
	lastDir, lastCase := "", "the state the run started from"
	leftovers := 0
	for _, path := range cases {
		if ctx.Err() != nil {
			break
		}
		dir := filepath.Dir(path)
		rel0, _ := filepath.Rel(scenario, path)
		if _, already := done[rel0]; already {
			// Nothing ran, so there is nothing to reset and nothing a slave
			// could have been left holding by it.
			results = append(results, done[rel0])
			resumed++
			lastDir, lastCase = dir, rel0
			continue
		}
		if resetEvery == "case" || dir != lastDir {
			out, rerr := Reset(ctx, pair)
			if rerr != nil {
				fmt.Printf("  ! could not reset the database: %v\n", rerr)
			}
			if len(out.Left) > 0 {
				fmt.Printf("  ! %s\n", ResetNote(out.Left))
				leftovers++
			}
			// A slave holding what the master does not is a finding and not
			// housekeeping: something the master did did not arrive, or
			// arrived and could not be undone by name. It is named, attributed
			// to the case that left it, and written down -- the repair keeps
			// the next case honest, the record keeps this one.
			for _, s := range out.Stranded {
				fmt.Printf("  ! %s, left by %s\n", s.String(), lastCase)
				stranded = append(stranded, strandedAt{Case: lastCase, S: s})
			}
		}
		lastDir = dir
		sql, rerr := os.ReadFile(path)
		if rerr != nil {
			results = append(results, Result{Case: path, Outcome: CaseFailed, Detail: rerr.Error()})
			continue
		}
		rel, _ := filepath.Rel(scenario, path)
		board.Begin(boardSlot, rel)
		r := RunCase(ctx, pair, rel, string(sql), wait, keepDir, addKey)
		board.End(boardSlot, rel, r.Outcome != Differ && r.Outcome != WaitTimeout && r.Outcome != CaseFailed)
		results = append(results, r)
		ledger.Write(r)
		lastCase = rel
		fmt.Printf("  %-15s %s%s\n", r.Outcome, rel, detailSuffix(r))
	}

	if keepDir != "" {
		if err := keepEmptyAgreements(keepDir, results); err != nil {
			fmt.Printf("  ! could not record the empty agreements: %v\n", err)
		}
		if err := keepStranded(keepDir, stranded); err != nil {
			fmt.Printf("  ! could not record what a slave was left holding: %v\n", err)
		}
	}
	report(results)
	if resumed > 0 {
		fmt.Printf("  %d of those were resumed from an earlier run, and their statements are not in the counts above\n", resumed)
	}
	if leftovers > 0 {
		fmt.Printf("  %d case(s) started from a state a reset could not clear\n", leftovers)
	}
	reportStranded(stranded, keepDir)
	return nil
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
