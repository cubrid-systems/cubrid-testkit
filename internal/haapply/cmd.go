// Package haapply is `testkit ha-apply`, the check that a parallel HA applier
// applies conflicting transactions in their order (ADR-EXT-012).
package haapply

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/sandbox"
)

const usage = `usage:
  testkit ha-apply list
  testkit ha-apply pairs (--build DIR | --cluster NAME) [options] [pattern...]

pairs options:
  --build DIR      stand an HA pair up from this install tree, and take it down after
  --cluster NAME   use this csb pair instead; it needs a client node c1
  --workers N      ha_apply_worker_count of a pair it stands up (default 4)
  --history N      ha_writeset_history_size of a pair it stands up (default: the engine's)
  --rounds N       rounds per pattern, at most 999 (default 20)
  --rows N         ballast rows ahead of T1's last write (default 20000)
  --ledger FILE    where the ledger goes (default ha-apply-pairs-<time>.tsv)
  --keep           keep the pair it stood up
`

// maxRounds keeps every pattern's keys apart: tz0/tz9 write a round as
// minutes and seconds under an hour, and fk_repoint's second parent is r+1000.
const maxRounds = 999

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

type pairsArgs struct {
	build, cluster, ledger string
	workers, history       int
	rounds, rows           int
	keep                   bool
	patterns               []Pattern
}

func parsePairs(args []string, stderr io.Writer) (*pairsArgs, error) {
	fs := flag.NewFlagSet("ha-apply pairs", flag.ContinueOnError)
	fs.SetOutput(stderr)
	a := &pairsArgs{}
	fs.StringVar(&a.build, "build", "", "")
	fs.StringVar(&a.cluster, "cluster", "", "")
	fs.StringVar(&a.ledger, "ledger", "", "")
	fs.IntVar(&a.workers, "workers", 4, "")
	fs.IntVar(&a.history, "history", 0, "")
	fs.IntVar(&a.rounds, "rounds", 20, "")
	fs.IntVar(&a.rows, "rows", 20000, "")
	fs.BoolVar(&a.keep, "keep", false, "")
	fs.Usage = func() { fmt.Fprint(stderr, usage) }
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	switch {
	case (a.build == "") == (a.cluster == ""):
		return nil, fmt.Errorf("give one of --build and --cluster")
	case a.rounds < 1 || a.rounds > maxRounds || a.rows < 1:
		return nil, fmt.Errorf("--rounds must be 1 to %d and --rows at least 1", maxRounds)
	case a.workers < 1 || a.history < 0:
		return nil, fmt.Errorf("--workers must be at least 1 and --history not negative")
	case a.cluster != "" && (a.keep || isSet(fs, "history") || isSet(fs, "workers")):
		return nil, fmt.Errorf("--workers, --history and --keep are for a pair this run stands up, not --cluster")
	}
	pats, err := Select(fs.Args())
	if err != nil {
		return nil, err
	}
	a.patterns = pats
	if a.ledger == "" {
		a.ledger = "ha-apply-pairs-" + time.Now().Format("20060102-150405") + ".tsv"
	}
	return a, nil
}

func isSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) { set = set || f.Name == name })
	return set
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
	// The ledger first: a path that cannot be written should not cost a
	// cluster create.
	_, statErr := os.Stat(a.ledger)
	created := errors.Is(statErr, os.ErrNotExist)
	f, err := os.Create(a.ledger)
	if err != nil {
		fmt.Fprintf(stderr, "ha-apply: %v\n", err)
		return 1
	}
	// A run that fails before it measures anything leaves no empty ledger of
	// its own; a path that was there before it is left there.
	headWritten := false
	defer func() {
		if !headWritten {
			f.Close()
			if created {
				os.Remove(a.ledger)
			}
		}
	}()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cli := &sandbox.CLI{Cluster: a.cluster}
	if a.build != "" {
		// Random, so two runs started together never share a pair: csb adopts a
		// running cluster of the same name.
		cli.Cluster = "hap" + strings.TrimPrefix(sandbox.NewRunName(time.Now()), "tk")
		hidden := []string{fmt.Sprintf("ha_apply_worker_count=%d", a.workers)}
		if a.history > 0 {
			hidden = append(hidden, fmt.Sprintf("ha_writeset_history_size=%d", a.history))
		}
		fmt.Fprintf(stderr, "ha-apply: standing up %s from %s (%s)\n", cli.Cluster, a.build, strings.Join(hidden, " "))
		err := cli.CreateWith(ctx, sandbox.CreateOptions{
			Preset: "ha", Build: a.build, Clients: 1, SetHidden: hidden,
			Labels: map[string]string{"testkit.ha-apply": "pairs"}, Timeout: 15 * time.Minute,
		})
		if !a.keep {
			defer func() {
				// A second interrupt should reach a teardown that hangs, so the
				// signal goes back to its default first; the run's own context
				// may be cancelled by now, and the pair still goes.
				stop()
				if _, derr := cli.DestroyPurge(context.Background()); derr != nil {
					fmt.Fprintf(stderr, "ha-apply: %s was not removed: %v\n", cli.Cluster, derr)
				}
			}()
		}
		if err != nil {
			fmt.Fprintf(stderr, "ha-apply: cluster create: %v\n", err)
			return 1
		}
	}
	pair, err := sandbox.Describe(ctx, cli, "")
	if err != nil {
		fmt.Fprintf(stderr, "ha-apply: %v\n", err)
		return 1
	}
	client := sandbox.NewNode(cli, "c1")
	if res, err := client.Run(ctx, "command -v csql"); err != nil {
		fmt.Fprintf(stderr, "ha-apply: %s: client node c1: %v\n", cli.Cluster, err)
		return 1
	} else if res.ExitCode != 0 {
		fmt.Fprintf(stderr, "ha-apply: %s has no csql on a client node c1 (exit %d); csb cluster create --clients 1 makes one\n",
			cli.Cluster, res.ExitCode)
		return 1
	}

	meta := []string{
		"cluster=" + cli.Cluster, "build=" + buildID(ctx, pair),
		"ha_apply_worker_count=" + configured(ctx, pair, "ha_apply_worker_count"),
		"ha_writeset_history_size=" + configured(ctx, pair, "ha_writeset_history_size"),
		fmt.Sprintf("rounds=%d", a.rounds), fmt.Sprintf("rows=%d", a.rows),
	}
	fmt.Fprintf(stdout, "# %s\n", strings.Join(meta, " "))
	ledgerHead(f, meta)

	headWritten = true
	r := &Runner{Pair: pair, Client: client, Rounds: a.rounds, Rows: a.rows, Log: stderr,
		WitnessWait: 30 * time.Second, StallFor: stallFor(a.rows), NoReadFor: time.Minute}
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
