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

	var results []Result
	lastDir := ""
	for _, path := range cases {
		if ctx.Err() != nil {
			break
		}
		// The directory is the unit whose cases may rely on each other, so the
		// database is cleared when the directory changes and never inside one.
		if dir := filepath.Dir(path); dir != lastDir {
			if derr := DropAll(ctx, pair); derr != nil {
				fmt.Printf("  ! could not clear the database before %s: %v\n", dir, derr)
			}
			lastDir = dir
		}
		sql, rerr := os.ReadFile(path)
		if rerr != nil {
			results = append(results, Result{Case: path, Outcome: CaseFailed, Detail: rerr.Error()})
			continue
		}
		rel, _ := filepath.Rel(scenario, path)
		r := RunCase(ctx, pair, rel, string(sql), wait, keepDir)
		results = append(results, r)
		fmt.Printf("  %-15s %s%s\n", r.Outcome, rel, detailSuffix(r))
	}

	report(results)
	return nil
}

func detailSuffix(r Result) string {
	switch r.Outcome {
	case Differ:
		return "  <- " + r.Differing + " on " + r.Node
	case Unreplicatable:
		return "  <- " + strings.Join(r.Tables, ", ")
	case WaitTimeout, CaseFailed:
		return "  <- " + r.Detail
	}
	return ""
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
	for _, o := range []Outcome{Same, Differ, Unordered, Unreplicatable, NoData, Skipped, WaitTimeout, CaseFailed} {
		if by[o] > 0 {
			fmt.Printf("  %-15s %d\n", o, by[o])
		}
	}
	var stmts, cmp int
	for _, r := range results {
		stmts += r.Statements
		cmp += r.Compared
	}
	var skipped, unordered int
	for _, r := range results {
		skipped += r.Unreplicated
		unordered += r.Unordered
	}
	fmt.Printf("  %d statement(s), %d read(s) compared, %d skipped for want of a primary key, %d unordered\n",
		stmts, cmp, skipped, unordered)
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
