package hareplsuite

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
func Watch(ctx context.Context, home *conf.Home, confPath, addr string, out io.Writer) error {
	cfg, err := home.Load(confPath)
	if err != nil {
		return fmt.Errorf("please confirm your conf file path: %w", err)
	}
	cluster := clusterOf(cfg.GetOr(ClusterKey, ""))
	if cluster == "" {
		return fmt.Errorf("no cluster: set %s in %s, or %s in the environment",
			ClusterKey, confPath, ClusterEnv)
	}
	scenario := cfg.GetOr("scenario", "")
	cases, cerr := Cases(scenario)
	if cerr != nil {
		return cerr
	}
	keepDir := cfg.GetOr("difference_dir", filepath.Join(filepath.Dir(confPath), "ha_repl_differences"))
	ledgerPath := filepath.Join(keepDir, LedgerFile)

	board := status.New(len(cases))
	where, stop, serr := board.Serve(addr)
	if serr != nil {
		return serr
	}
	defer stop()
	fmt.Fprintf(out, "watching %s\n  ledger  %s\n  cluster %s\n  page    http://%s/\n",
		scenario, ledgerPath, cluster, where)
	board.Lane(boardSlot, "pair")
	board.Setup([]status.Setting{
		{Group: "suite", Key: "task", Value: "ha_repl", Note: "watched from outside the run"},
		{Group: "suite", Key: "scenario", Value: scenario},
		{Group: "suite", Key: ClusterKey, Value: cluster},
		{Group: "suite", Key: "ledger", Value: ledgerPath,
			Note: "the run writes one line per case here, and this page reads it"},
	})

	cli := sandbox.Bind(cluster)
	static := status.Pair{Cluster: cluster}
	if info, aerr := cli.Artifact(ctx); aerr == nil {
		static.Backend, static.Image, static.Network, static.DB = info.Backend, info.Image, info.Network, info.DB
		static.Engine = info.Engine.Build
	}
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			board.Pair(samplePair(ctx, cli, static))
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()

	return follow(ctx, board, ledgerPath)
}

// follow reads the ledger from the beginning and then keeps reading.
//
// From the beginning because a watcher that attached at case 3,000 would draw
// a run that had done 327 of them. By polling rather than by inotify: the file
// is appended a few times a minute at most, the run flushes every line, and a
// poll cannot miss a write the way a watcher that has to re-register can.
func follow(ctx context.Context, board *status.Board, path string) error {
	var offset int64
	seen := map[string]bool{}
	for {
		size, err := sizeOf(path)
		switch {
		case err != nil:
			// The run may not have written its first verdict yet. That is a
			// state to wait in, not to fail on -- a watcher started in the
			// same breath as the run is the normal case.
		case size < offset:
			// The file shrank, so it is a different run: start over rather
			// than read the middle of a line.
			offset, seen = 0, map[string]bool{}
			fallthrough
		case size > offset:
			n, rerr := readFrom(path, offset, func(r Result) {
				if seen[r.Case] {
					return
				}
				seen[r.Case] = true
				board.Begin(boardSlot, r.Case)
				board.Record(boardSlot, r.Case, ok(r.Outcome), r.Took)
			})
			if rerr == nil {
				offset = n
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(500 * time.Millisecond):
		}
	}
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
