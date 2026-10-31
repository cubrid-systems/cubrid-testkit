// Package haapply is `testkit ha-apply`, the check that a parallel HA applier
// applies conflicting transactions in their order (ADR-EXT-012).
package haapply

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/sandbox"
)

const usage = `usage:
  testkit ha-apply list
  testkit ha-apply pairs (--build DIR | --cluster NAME) [options] [pattern...]
  testkit ha-apply stress (--build DIR | --cluster NAME) [options]

options of both:
  --build DIR      stand an HA pair up from this install tree, and take it down after
  --cluster NAME   use this csb pair instead; it needs a client node c1
  --workers N      ha_apply_worker_count of a pair it stands up (default 4)
  --history N      ha_writeset_history_size of a pair it stands up (default: the engine's)
  --ledger FILE    where the ledger goes (default ha-apply-<verb>-<time>.tsv); stress
                   keeps the clients' files and any differing reads in FILE minus .tsv, plus .d
  --keep           keep the pair it stood up

pairs options:
  --rounds N       rounds per pattern, at most 999 (default 20)
  --rows N         ballast rows ahead of T1's last write (default 20000)

stress options:
  --clients N      clients at once, at most 21 (default 8)
  --txns N         transactions a client, at most 9999 and fewer with 21 clients (default 200)
  --keys N         the key space, at most 2147483647 (default 20)
  --seed N         names every client's SQL, 0 to 999999 (default 1)
  --ops LIST       operations 0-9 to draw from (default 0,1,2,3,4,5,6,7,8,9)
  --ballast N      rows a slow transaction inserts first, at most 10000 (default 3000)
  --timeout D      how long the clients may take before they are stopped, 1s at least (default 30m)
  --compare-only   no workload: compare the tables an earlier run left (--cluster only)
`

// maxRounds keeps every pattern's keys apart: tz0/tz9 write a round as
// minutes and seconds under an hour, and fk_repoint's second parent is r+1000.
const maxRounds = 999

// barrierFor is how long the slave may take to apply a marker.
const barrierFor = 5 * time.Minute

// Main is the subcommand: exit 0 when the run finished, whatever its
// verdicts; 1 when it could not run; 2 for a command it cannot read.
func Main(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "list":
		return list(stdout)
	case "pairs":
		return pairs(args[1:], stdout, stderr)
	case "stress":
		return stress(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	}
	fmt.Fprintf(stderr, "ha-apply: unknown verb %q\n%s", args[0], usage)
	return 2
}

func list(stdout io.Writer) int {
	tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "pattern\tshares")
	for _, p := range Catalog {
		fmt.Fprintf(tw, "%s\t%s\n", p.Name, p.Shares)
	}
	tw.Flush()
	return 0
}

// pairOpts is how a verb gets its pair: one it stands up from a build, or the
// one --cluster names.
type pairOpts struct {
	build, cluster, ledger string
	workers, history       int
	keep                   bool
}

func pairFlags(fs *flag.FlagSet, o *pairOpts) {
	fs.StringVar(&o.build, "build", "", "")
	fs.StringVar(&o.cluster, "cluster", "", "")
	fs.StringVar(&o.ledger, "ledger", "", "")
	fs.IntVar(&o.workers, "workers", 4, "")
	fs.IntVar(&o.history, "history", 0, "")
	fs.BoolVar(&o.keep, "keep", false, "")
}

func (o *pairOpts) check(fs *flag.FlagSet, verb string) error {
	switch {
	case (o.build == "") == (o.cluster == ""):
		return fmt.Errorf("give one of --build and --cluster")
	case o.workers < 1 || o.history < 0:
		return fmt.Errorf("--workers must be at least 1 and --history not negative")
	case o.cluster != "" && (o.keep || isSet(fs, "history") || isSet(fs, "workers")):
		return fmt.Errorf("--workers, --history and --keep are for a pair this run stands up, not --cluster")
	}
	if o.ledger == "" {
		o.ledger = "ha-apply-" + verb + "-" + time.Now().Format("20060102-150405") + ".tsv"
	}
	return nil
}

type pairsArgs struct {
	pairOpts
	rounds, rows int
	patterns     []Pattern
}

func parsePairs(args []string, stderr io.Writer) (*pairsArgs, error) {
	fs := flag.NewFlagSet("ha-apply pairs", flag.ContinueOnError)
	fs.SetOutput(stderr)
	a := &pairsArgs{}
	pairFlags(fs, &a.pairOpts)
	fs.IntVar(&a.rounds, "rounds", 20, "")
	fs.IntVar(&a.rows, "rows", 20000, "")
	fs.Usage = func() { fmt.Fprint(stderr, usage) }
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if err := a.check(fs, "pairs"); err != nil {
		return nil, err
	}
	if a.rounds < 1 || a.rounds > maxRounds || a.rows < 1 {
		return nil, fmt.Errorf("--rounds must be 1 to %d and --rows at least 1", maxRounds)
	}
	pats, err := Select(fs.Args())
	if err != nil {
		return nil, err
	}
	a.patterns = pats
	return a, nil
}

func isSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) { set = set || f.Name == name })
	return set
}

// ledgerFile opens a ledger before any pair is touched: a path that cannot be
// written should not cost a cluster create. drop is for a run that fails
// before it measures anything: it leaves no empty ledger of its own, and a
// path that was there before it is left there.
func ledgerFile(path string) (f *os.File, drop func(), err error) {
	_, statErr := os.Stat(path)
	created := errors.Is(statErr, os.ErrNotExist)
	if f, err = os.Create(path); err != nil {
		return nil, nil, err
	}
	return f, func() {
		f.Close()
		if created {
			os.Remove(path)
		}
	}, nil
}

// session is the pair a verb runs on, with its client node.
type session struct {
	cli    *sandbox.CLI
	pair   *sandbox.Pair
	client *sandbox.Node
}

// openPair stands the pair up, or takes the one --cluster names, and checks
// that its client node has csql. The teardown it returns removes a pair it
// stood up, unless --keep; it is never nil, and a pair that was half made
// still goes.
func openPair(ctx context.Context, stop func(), o pairOpts, verb string, stderr io.Writer) (*session, func(), error) {
	cli := &sandbox.CLI{Cluster: o.cluster}
	teardown := func() {}
	if o.build != "" {
		// Random, so two runs started together never share a pair: csb adopts a
		// running cluster of the same name.
		cli.Cluster = "hap" + strings.TrimPrefix(sandbox.NewRunName(time.Now()), "tk")
		hidden := []string{fmt.Sprintf("ha_apply_worker_count=%d", o.workers)}
		if o.history > 0 {
			hidden = append(hidden, fmt.Sprintf("ha_writeset_history_size=%d", o.history))
		}
		fmt.Fprintf(stderr, "ha-apply: standing up %s from %s (%s)\n", cli.Cluster, o.build, strings.Join(hidden, " "))
		if !o.keep {
			teardown = func() {
				// A second interrupt should reach a teardown that hangs, so the
				// signal goes back to its default first; the run's own context
				// may be cancelled by now, and the pair still goes.
				stop()
				if _, derr := cli.DestroyPurge(context.Background()); derr != nil {
					fmt.Fprintf(stderr, "ha-apply: %s was not removed: %v\n", cli.Cluster, derr)
				}
			}
		}
		if err := cli.CreateWith(ctx, sandbox.CreateOptions{
			Preset: "ha", Build: o.build, Clients: 1, SetHidden: hidden,
			Labels: map[string]string{"testkit.ha-apply": verb}, Timeout: 15 * time.Minute,
		}); err != nil {
			return nil, teardown, fmt.Errorf("cluster create: %w", err)
		}
	}
	pair, err := sandbox.Describe(ctx, cli, "")
	if err != nil {
		return nil, teardown, err
	}
	client := sandbox.NewNode(cli, "c1")
	if res, err := client.Run(ctx, "command -v csql"); err != nil {
		return nil, teardown, fmt.Errorf("%s: client node c1: %w", cli.Cluster, err)
	} else if res.ExitCode != 0 {
		return nil, teardown, fmt.Errorf("%s has no csql on a client node c1 (exit %d); csb cluster create --clients 1 makes one",
			cli.Cluster, res.ExitCode)
	}
	return &session{cli: cli, pair: pair, client: client}, teardown, nil
}

// meta is the run's identity, which a ledger's first line carries: the pair,
// its build, and the applier's settings as the slave's conf has them.
func (s *session) meta(ctx context.Context) []string {
	return []string{
		"cluster=" + s.cli.Cluster, "build=" + buildID(ctx, s.pair),
		"ha_apply_worker_count=" + configured(ctx, s.pair, "ha_apply_worker_count"),
		"ha_writeset_history_size=" + configured(ctx, s.pair, "ha_writeset_history_size"),
	}
}

func pairs(args []string, stdout, stderr io.Writer) int {
	a, err := parsePairs(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(stderr, "ha-apply pairs: %v\n", err)
		return 2
	}
	f, drop, err := ledgerFile(a.ledger)
	if err != nil {
		fmt.Fprintf(stderr, "ha-apply: %v\n", err)
		return 1
	}
	headWritten := false
	defer func() {
		if !headWritten {
			drop()
		}
	}()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	s, teardown, err := openPair(ctx, stop, a.pairOpts, "pairs", stderr)
	defer teardown()
	if err != nil {
		fmt.Fprintf(stderr, "ha-apply: %v\n", err)
		return 1
	}

	meta := append(s.meta(ctx), fmt.Sprintf("rounds=%d", a.rounds), fmt.Sprintf("rows=%d", a.rows))
	fmt.Fprintf(stdout, "# %s\n", strings.Join(meta, " "))
	ledgerHead(f, meta)

	headWritten = true
	r := &Runner{Pair: s.pair, Client: s.client, Rounds: a.rounds, Rows: a.rows, Log: stderr,
		WitnessWait: 30 * time.Second, StallFor: stallFor(a.rows), NoReadFor: time.Minute, BarrierFor: barrierFor}
	var outs []Outcome
	code := 0
	for i, p := range a.patterns {
		o, err := r.Pattern(ctx, p)
		if err != nil {
			fmt.Fprintf(stderr, "ha-apply: %s: %v\n", p.Name, err)
			code = 1
			break
		}
		outs = append(outs, o)
		v := Judge(outs)[len(outs)-1]
		fmt.Fprintln(stdout, line(o, v))
		// A row as it is known: the control runs first, so no later pattern
		// changes a verdict already written.
		ledgerRow(f, o, v)
		if o.Stalled && i < len(a.patterns)-1 {
			// Every later pattern starts with a wait on this slave.
			fmt.Fprintf(stderr, "ha-apply: %s: the slave stopped applying, so the patterns after it are not run\n", p.Name)
			code = 1
			break
		}
	}
	if cerr := f.Close(); cerr != nil {
		fmt.Fprintf(stderr, "ha-apply: ledger %s: %v\n", a.ledger, cerr)
		code = 1
	}
	fmt.Fprintln(stdout, summary(outs))
	fmt.Fprintf(stderr, "ha-apply: ledger %s\n", a.ledger)
	return code
}

type stressArgs struct {
	pairOpts
	cfg         StressConfig
	compareOnly bool
}

// The ballast keys of a run are client×10^8 + transaction×10^4 + row, in an
// INT, and a transaction's rows stay under the next transaction's.
const (
	maxClients = 21
	maxTxns    = 9999
	maxBallast = 10000
	maxSeed    = 999999
)

var workloadFlags = []string{"clients", "txns", "keys", "seed", "ops", "ballast", "timeout"}

func parseStress(args []string, stderr io.Writer) (*stressArgs, error) {
	fs := flag.NewFlagSet("ha-apply stress", flag.ContinueOnError)
	fs.SetOutput(stderr)
	a := &stressArgs{}
	pairFlags(fs, &a.pairOpts)
	c := &a.cfg
	fs.IntVar(&c.Clients, "clients", 8, "")
	fs.IntVar(&c.Txns, "txns", 200, "")
	fs.IntVar(&c.Keys, "keys", 20, "")
	fs.IntVar(&c.Seed, "seed", 1, "")
	ops := fs.String("ops", "0,1,2,3,4,5,6,7,8,9", "")
	fs.IntVar(&c.Ballast, "ballast", 3000, "")
	fs.DurationVar(&c.Timeout, "timeout", 30*time.Minute, "")
	fs.BoolVar(&a.compareOnly, "compare-only", false, "")
	fs.Usage = func() { fmt.Fprint(stderr, usage) }
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() > 0 {
		return nil, fmt.Errorf("stress takes no patterns: %s", strings.Join(fs.Args(), " "))
	}
	if err := a.check(fs, "stress"); err != nil {
		return nil, err
	}
	if a.compareOnly {
		if a.build != "" {
			return nil, fmt.Errorf("--compare-only reads the tables a pair already has; give --cluster")
		}
		for _, name := range workloadFlags {
			if isSet(fs, name) {
				return nil, fmt.Errorf("--%s is for a run, not --compare-only", name)
			}
		}
		return a, nil
	}
	switch {
	case c.Clients < 1 || c.Clients > maxClients:
		return nil, fmt.Errorf("--clients must be 1 to %d", maxClients)
	case c.Txns < 1 || c.Txns > maxTxns:
		return nil, fmt.Errorf("--txns must be 1 to %d", maxTxns)
	case c.Ballast < 1 || c.Ballast > maxBallast:
		return nil, fmt.Errorf("--ballast must be 1 to %d", maxBallast)
	case c.Clients*100000000+c.Txns*10000+c.Ballast > math.MaxInt32:
		return nil, fmt.Errorf("--clients %d with --txns %d names ballast keys past INT", c.Clients, c.Txns)
	case c.Keys < 1 || c.Keys > math.MaxInt32:
		// A key is an INT, and the shell's awk prints it as one.
		return nil, fmt.Errorf("--keys must be 1 to %d", math.MaxInt32)
	case c.Seed < 0 || c.Seed > maxSeed:
		return nil, fmt.Errorf("--seed must be 0 to %d", maxSeed)
	case c.Timeout < time.Second:
		// The clients' script counts whole seconds.
		return nil, fmt.Errorf("--timeout must be 1s at least")
	}
	for _, s := range strings.Split(*ops, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || n < 0 || n > 9 {
			return nil, fmt.Errorf("--ops is a comma-separated list of 0 to 9, not %q", *ops)
		}
		c.Ops = append(c.Ops, n)
	}
	return a, nil
}

func stress(args []string, stdout, stderr io.Writer) int {
	a, err := parseStress(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(stderr, "ha-apply stress: %v\n", err)
		return 2
	}
	// The run's files go beside the ledger; an earlier run's there would be
	// taken for this one's, and they are not this run's to remove.
	save := strings.TrimSuffix(a.ledger, ".tsv") + ".d"
	if entries, err := os.ReadDir(save); err == nil && len(entries) > 0 {
		fmt.Fprintf(stderr, "ha-apply: %s already holds files; give another --ledger\n", save)
		return 1
	}
	f, drop, err := ledgerFile(a.ledger)
	if err != nil {
		fmt.Fprintf(stderr, "ha-apply: %v\n", err)
		return 1
	}
	headWritten := false
	defer func() {
		if !headWritten {
			drop()
		}
	}()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	s, teardown, err := openPair(ctx, stop, a.pairOpts, "stress", stderr)
	defer teardown()
	if err != nil {
		fmt.Fprintf(stderr, "ha-apply: %v\n", err)
		return 1
	}

	c := a.cfg
	meta := s.meta(ctx)
	if a.compareOnly {
		meta = append(meta, "mode=compare-only")
	} else {
		ops := make([]string, len(c.Ops))
		for i, n := range c.Ops {
			ops[i] = strconv.Itoa(n)
		}
		meta = append(meta, "mode=run", fmt.Sprintf("clients=%d", c.Clients), fmt.Sprintf("txns=%d", c.Txns),
			fmt.Sprintf("keys=%d", c.Keys), "ops="+strings.Join(ops, ","), fmt.Sprintf("ballast=%d", c.Ballast),
			fmt.Sprintf("seed=%d", c.Seed), "timeout="+c.Timeout.String())
	}
	fmt.Fprintf(stdout, "# %s\n", strings.Join(meta, " "))
	stressHead(f, meta)
	headWritten = true

	// The client node's /work is this host's work directory of the cluster.
	dir := filepath.Join("ha-apply", fmt.Sprintf("stress-%d", c.Seed))
	c.Host = filepath.Join(sandbox.WorkDir(s.cli.Cluster), dir)
	c.Node = "/work/" + filepath.ToSlash(dir)
	c.Save = save
	r := &Runner{Pair: s.pair, Client: s.client, Log: stderr, BarrierFor: barrierFor}
	code := 0
	o, err := r.Stress(ctx, c, a.compareOnly)
	if err != nil {
		fmt.Fprintf(stderr, "ha-apply: stress: %v\n", err)
		code = 1
	} else {
		v := JudgeStress(o)
		fmt.Fprintln(stdout, stressLine(o, v))
		stressRow(f, o, v)
	}
	if cerr := f.Close(); cerr != nil {
		fmt.Fprintf(stderr, "ha-apply: ledger %s: %v\n", a.ledger, cerr)
		code = 1
	}
	if _, err := os.Stat(c.Save); err == nil {
		fmt.Fprintf(stderr, "ha-apply: ledger %s, files %s\n", a.ledger, c.Save)
	} else {
		fmt.Fprintf(stderr, "ha-apply: ledger %s\n", a.ledger)
	}
	return code
}

// stallFor is how long the slave may take to apply a round's T1 before it
// counts as stopped: two minutes for every 20,000 rows of ballast, two at
// least. At the default a round on #8052's new head, T1 on the slave included,
// averaged 2.3 to 5.5 seconds a run in the M1 runs; what this is for is a slave
// that waits for good (evidence/ha/fk-index-insert-loses-the-victims-error.md).
func stallFor(rows int) time.Duration {
	return time.Duration((rows+19999)/20000) * 2 * time.Minute
}

// configured is a key of the slave's cubrid_ha.conf as written there,
// "unset" when the file does not set it (the engine's default: one worker,
// and its own history size), or "unknown" when the file cannot be read.
func configured(ctx context.Context, p *sandbox.Pair, key string) string {
	ch, err := p.SlaveChannel(0)
	if err != nil {
		return "unknown"
	}
	res, err := ch.Run(ctx, fmt.Sprintf(`f="$CUBRID/conf/cubrid_ha.conf"; [ -r "$f" ] || { echo unreadable; exit 0; }
grep -E '^[[:space:]]*%s[[:space:]]*=' "$f" | tail -1`, key))
	if err != nil || strings.TrimSpace(res.Stdout) == "unreadable" {
		return "unknown"
	}
	_, v, ok := strings.Cut(strings.TrimSpace(res.Stdout), "=")
	if !ok {
		return "unset"
	}
	return strings.TrimSpace(v)
}

var relID = regexp.MustCompile(`\(([0-9.]+-[0-9a-f]+)\)`)

func buildID(ctx context.Context, p *sandbox.Pair) string {
	res, err := p.MasterChannel().Run(ctx, "cubrid_rel")
	if err != nil {
		return "unknown"
	}
	if m := relID.FindStringSubmatch(res.Stdout); m != nil {
		return m[1]
	}
	return "unknown"
}

func line(o Outcome, verdict string) string {
	return fmt.Sprintf("%-18s %-24s ahead=%d/%d never=%d refused=%d final=%s fail=%d write=%d ref=%d fallback=%d clear=%d",
		o.Pattern, verdict, o.Ahead, o.Rounds, o.Never, o.Refused, o.Final, o.FailDelta, o.Write, o.Ref, o.Fallback, o.Clear)
}

func summary(outs []Outcome) string {
	count := map[string]int{}
	for _, v := range Judge(outs) {
		count[v]++
	}
	return fmt.Sprintf("patterns=%d held=%d missed=%d inconclusive=%d control=%s",
		len(outs), count[Held], count[Missed], count[Inconclusive], controlOf(outs))
}

func controlOf(outs []Outcome) string {
	for _, o := range outs {
		if o.Pattern == "ctl" {
			return fmt.Sprintf("%d/%d", o.Ahead, o.Rounds)
		}
	}
	return "absent"
}

// The ledger (ADR-EXT-012 3-6): the run's identity, a header, then a row a
// pattern with its verdict, written as each pattern ends.
func ledgerHead(w io.Writer, meta []string) {
	fmt.Fprintf(w, "# %s\n", strings.Join(meta, " "))
	fmt.Fprintln(w, "pattern\trounds\tt2_ahead\tt2_never_seen\trefused\tfinal\tfail_counter_delta\tconflict_write\tconflict_ref\tfallback\tclear\tverdict")
}

func ledgerRow(w io.Writer, o Outcome, verdict string) {
	fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%d\t%s\t%d\t%d\t%d\t%d\t%d\t%s\n",
		o.Pattern, o.Rounds, o.Ahead, o.Never, o.Refused, o.Final, o.FailDelta, o.Write, o.Ref, o.Fallback, o.Clear, verdict)
}
