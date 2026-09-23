package hareplsuite

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/conf"
	"github.com/cubrid-systems/cubrid-testkit/internal/sandbox"
	"github.com/cubrid-systems/cubrid-testkit/internal/status"
)

// Watch serves the page for a run that is already going, from outside it.
//
// # Why this exists
//
// The page is served by the run's own process, from memory, so a run started
// without `status_http` cannot be looked at afterwards -- and the moment you
// want to look is never the moment you started. The first answer was to
// restart the run with the page on, which is only tolerable because the run is
// resumable, and it is still the wrong answer: it asks the measurement to
// accommodate the instrument.
//
// It does not have to. The run already writes everything the page counts, one
// line per case, flushed as it goes, for its own resumability -- and
// `replay.go` established the shape of the answer for the shell suite: "there
// is no recorder, a replay reads what the run already wrote". This is that,
// following a file that is still growing rather than one that stopped.
//
// So nothing is asked of the run. It needs no flag, no port, no signal and no
// restart; it does not know it is being watched. Two people can watch the same
// run from two machines, and a run that has already finished shows its result
// the same way.
//
// It also answers a different question from the run's own page, and both are
// worth having. The ledger is the corpus's progress across every restart --
// 1,011 of 3,327 when measured -- and the run's own board is what this process
// has judged since it started -- 401 of the 2,717 it has left. A resumed run
// makes those two numbers diverge on purpose.
//
// # What it reads, and what it asks
//
// The ledger gives the verdicts and their durations. A line written before the
// duration column existed reads as zero, so a watcher over an old ledger draws
// every one of those cases in the first bucket of the histogram; the counts and
// the verdicts are right and the shape fills in as the run writes new lines. Everything else on the
// page that is about the *pair* -- roles, lag, fail_counter, the faults in
// force -- is not in any file, because it is a property of the cluster and not
// of the run, so the watcher asks csb for it directly. That is the same
// reading the run's own page makes, from the same place, which is why the two
// cannot disagree.
// Watch serves one page for one or more runs.
//
// # Why more than one
//
// A corpus sharded across several pairs is several processes, each with a page
// of its own on a port of its own -- which is several places to look for the
// one shard that died, and the reason the pair panel exists is that nobody was
// looking when one did. So a watcher takes as many runs as it is given, or
// finds them itself, and draws them side by side: a lane and a pair panel each.
//
// Each run keeps its own place in its own ledger. One shard being rebuilt
// because it re-judged a case does not disturb the others' counts, and a shard
// whose ledger has gone quiet says so about itself rather than about the page.
func Watch(ctx context.Context, home *conf.Home, confPaths []string, addr string, out io.Writer) error {
	if len(confPaths) == 0 {
		return fmt.Errorf("nothing to watch: name a conf with -c, or start a run for this to find")
	}
	var sources []*source
	total := 0
	for _, confPath := range confPaths {
		src, err := openSource(ctx, home, confPath)
		if err != nil {
			return err
		}
		sources = append(sources, src)
		total += src.total
	}

	board := status.New(total)
	where, stop, serr := board.Serve(addr)
	if serr != nil {
		return serr
	}
	defer stop()

	fmt.Fprintf(out, "watching %d run(s)\n  page    http://%s/\n", len(sources), where)
	settings := []status.Setting{
		{Group: "suite", Key: "task", Value: "ha_repl", Note: "watched from outside the run"},
		{Group: "page", Key: "runs", Value: fmt.Sprint(len(sources)),
			Note: "each writes one line per case to its own ledger, and this page reads them"},
	}
	for _, src := range sources {
		fmt.Fprintf(out, "  %-8s %s\n           %s\n", src.label, src.scenario, src.ledger)
		board.Lane(src.label, "pair "+src.cluster)
		settings = append(settings,
			status.Setting{Group: src.label, Key: ClusterKey, Value: src.cluster},
			status.Setting{Group: src.label, Key: "scenario", Value: src.scenario},
			status.Setting{Group: src.label, Key: "ledger", Value: src.ledger})
	}
	board.Setup(settings)

	for _, src := range sources {
		go src.samplePairs(ctx, board)
	}
	return followAll(ctx, board, sources)
}

// source is one run: its lane, its ledger, and the pair it measures against.
type source struct {
	label    string
	cluster  string
	scenario string
	ledger   string
	total    int

	cli    *sandbox.CLI
	static status.Pair

	// Where this run's ledger has been read to, and what it said, so that a
	// case judged twice can be replaced rather than counted twice.
	offset int64
	order  []Result
	seen   map[string]int
	grew   time.Time
}

// openSource reads one run's conf and works out everything the page needs from
// it. It does not touch the run.
func openSource(ctx context.Context, home *conf.Home, confPath string) (*source, error) {
	cfg, err := home.Load(confPath)
	if err != nil {
		return nil, fmt.Errorf("please confirm your conf file path (%s): %w", confPath, err)
	}
	cluster := clusterOf(cfg.GetOr(ClusterKey, ""))
	if cluster == "" {
		return nil, fmt.Errorf("no cluster: set %s in %s, or %s in the environment",
			ClusterKey, confPath, ClusterEnv)
	}
	scenario := cfg.GetOr("scenario", "")
	cases, cerr := Cases(scenario)
	if cerr != nil {
		return nil, cerr
	}
	keepDir := cfg.GetOr("difference_dir", filepath.Join(filepath.Dir(confPath), "ha_repl_differences"))

	src := &source{
		// The cluster names the lane, because that is what the operator is
		// looking for when a shard misbehaves. Two runs against one cluster
		// would collide here, and that is a configuration worth colliding on:
		// they would also be resetting the same database under each other.
		label:    cluster,
		cluster:  cluster,
		scenario: scenario,
		ledger:   filepath.Join(keepDir, LedgerFile),
		total:    len(cases),
		cli:      sandbox.Bind(cluster),
		static:   status.Pair{Cluster: cluster},
		seen:     map[string]int{},
		grew:     time.Now(),
	}
	if info, aerr := src.cli.Artifact(ctx); aerr == nil {
		src.static.Backend, src.static.Image = info.Backend, info.Image
		src.static.Network, src.static.DB = info.Network, info.DB
		src.static.Engine = info.Engine.Build
	}
	return src, nil
}

// samplePairs keeps this run's pair panel current.
func (s *source) samplePairs(ctx context.Context, board *status.Board) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		board.Pair(samplePair(ctx, s.cli, s.static))
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// followAll reads every run's ledger from the beginning and then keeps reading.
//
// From the beginning because a watcher that attached at case 3,000 would draw a
// run that had done 327 of them, which is a lie about the run rather than a gap
// in the page.
//
// # Why a re-judged case rebuilds everything
//
// A board can be told about a case, not told to forget one. A resumed run
// re-runs what came back `wait_timeout`, and the second answer is the one that
// counts -- so when a line replaces an earlier one, the board is reset and
// every run's lines are laid down again in order. That costs nothing but a
// redraw, and it keeps the counts honest without the board learning what a
// retry is.
func followAll(ctx context.Context, board *status.Board, sources []*source) error {
	for {
		redraw := false
		for _, s := range sources {
			if s.poll(board) {
				redraw = true
			}
		}
		if redraw {
			board.Reset()
			for _, s := range sources {
				for _, r := range s.order {
					board.Begin(s.label, r.Case)
					board.Record(s.label, r.Case, ok(r.Outcome), r.Took)
				}
			}
		}
		board.Note(notes(sources))
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// poll reads what this run has written since the last look, and reports whether
// the whole board has to be laid down again.
func (s *source) poll(board *status.Board) bool {
	size, err := sizeOf(s.ledger)
	switch {
	case err != nil:
		// The run may not have written its first verdict yet. That is a state
		// to wait in, not to fail on -- a watcher started in the same breath as
		// the run is the normal case.
		return false
	case size < s.offset:
		// The file shrank, so it is a different run: start over rather than
		// read the middle of a line.
		s.offset, s.order, s.seen = 0, nil, map[string]int{}
		return true
	case size == s.offset:
		return false
	}
	replaced := false
	n, rerr := readFrom(s.ledger, s.offset, func(r Result) {
		at, already := s.seen[r.Case]
		if !already {
			s.seen[r.Case] = len(s.order)
			s.order = append(s.order, r)
			board.Begin(s.label, r.Case)
			board.Record(s.label, r.Case, ok(r.Outcome), r.Took)
			return
		}
		s.order[at] = r
		replaced = true
	})
	if rerr == nil {
		if n != s.offset {
			s.grew = time.Now()
		}
		s.offset = n
	}
	return replaced
}

// notes is what the page says about its sources, naming the run when there is
// more than one: "quiet" about eight shards is only useful if it says which.
func notes(sources []*source) string {
	var said []string
	for _, s := range sources {
		if line := quiet(len(s.order), s.total, s.grew); line != "" {
			if len(sources) > 1 {
				line = s.label + ": " + line
			}
			said = append(said, line)
		}
	}
	return strings.Join(said, " · ")
}

// quietFor is how long a ledger may go without a new verdict before the page
// says so.
//
// Longer than any case this corpus has: the wait bound alone is a minute, and
// a case that hits it is slow rather than gone. Short enough that a run killed
// at lunchtime is not still drawing a live-looking page at two.
const quietFor = 3 * time.Minute

// quiet is the sentence the page needs when the ledger stops growing.
//
// A finished run needs none -- the board already says finished, and a watcher
// that added "nothing new" to it would be reporting success as a fault. An
// unfinished one that has gone quiet is the case worth a sentence, because a
// page that merely stops advancing reads exactly like a run that is slow, and
// that is the failure this whole panel was built after: a pair died and an
// hour of a run went into wait_timeout with nobody looking.
func quiet(done, total int, since time.Time) string {
	if done >= total || time.Since(since) < quietFor {
		return ""
	}
	return fmt.Sprintf("no verdict in %s -- the run may have stopped, or be stuck on one case; "+
		"%d of %d judged", time.Since(since).Round(time.Second), done, total)
}

func sizeOf(path string) (int64, error) {
	st, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return st.Size(), nil
}

// readFrom reads whole lines from an offset and returns where the last one
// ended, so a line still being written is read next time instead of in halves.
func readFrom(path string, offset int64, each func(Result)) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return offset, err
	}
	defer f.Close()
	if _, serr := f.Seek(offset, io.SeekStart); serr != nil {
		return offset, serr
	}
	rest, rerr := io.ReadAll(f)
	if rerr != nil {
		return offset, rerr
	}
	consumed := int64(0)
	for {
		i := indexByte(rest[consumed:], '\n')
		if i < 0 {
			break
		}
		line := string(rest[consumed : consumed+int64(i)])
		consumed += int64(i) + 1
		if r, okLine := parseLedgerLine(line); okLine {
			each(r)
		}
	}
	return offset + consumed, nil
}

func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}
