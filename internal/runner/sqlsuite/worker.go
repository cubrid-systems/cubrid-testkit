package sqlsuite

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/coredump"
	"github.com/cubrid-systems/cubrid-testkit/internal/dispatch"
	"github.com/cubrid-systems/cubrid-testkit/internal/result"
	"github.com/cubrid-systems/cubrid-testkit/internal/status"
)

// work is what every slot's loop shares: the cases, where their verdicts go,
// and the count that numbers the progress lines.
type work struct {
	cases *caseSet
	rec   *result.SQL
	// board is the status page, or nil; every method on it tolerates nil.
	board *status.Board
	total int
	// started numbers cases in the order they start, which is what CQT's
	// "(i/N p%)" counts. A parallel run prints its lines as cases finish, so
	// the numbers arrive out of order; each one still says where its case
	// started.
	started atomic.Int64
}

// loop runs cases in one place until the queue is empty. An error is the
// executor gone, which ends the run; a case that fails is a verdict and not an
// error.
func (w *work) loop(ctx context.Context, name string, p place, q *dispatch.Queue, x Executor) error {
	cores := map[string]bool{}
	for {
		t, ok := q.ClaimFor(name, dispatch.LaneAny)
		if !ok {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		c := result.SQLCase{
			File:  t.Case,
			Index: int(w.started.Add(1)),
			Total: w.total,
			Start: time.Now(),
		}
		w.rec.Starting(c)
		w.board.Begin(name, t.Case)
		// A running sql case writes nothing until it ends -- the executor renders
		// the whole case at once -- so what the page shows for one is what it
		// is running: its statements.
		w.board.Live(t.Case, t.Case)
		// No answer, no run: CQT sets shouldRun false and its loop counts the
		// case as failed without executing it (ConsoleBO.build, execute).
		if answer, has := w.cases.answer[t.Case]; has {
			c.Ran = true
			rendered, ms, err := x.Run(ctx, t.Case)
			switch {
			case errors.Is(err, errExecutorGone):
				q.Complete(t, true, false)
				return fmt.Errorf("%s: %w", name, err)
			case err != nil:
				c.Err = err
			default:
				want, rerr := os.ReadFile(answer)
				if rerr != nil {
					c.Err = rerr
					break
				}
				c.Rendered, c.Ms = rendered, ms
				c.OK = matches(rendered, want)
			}
			if !c.OK || c.Err != nil {
				if found := newCores(ctx, p, cores); len(found) > 0 {
					c.HasCore = true
					w.coreErr(ctx, p, t.Case, found)
				}
			}
		}
		if err := w.rec.Case(c); err != nil {
			q.Complete(t, true, false)
			return err
		}
		w.board.Live(t.Case, "")
		w.board.End(name, t.Case, c.Ran && c.OK && c.Err == nil)
		// No retries: CQT has none, and the queue is only here for slots.
		q.Complete(t, true, false)
	}
}

// newCores is CQT's search after a failed case (CommonFileUtile.getCoreFiles):
// files under $CUBRID named CORE.<digits> in any case, that this place has not
// already reported. It runs in the place because a slot's $CUBRID is its own.
func newCores(ctx context.Context, p place, seen map[string]bool) []string {
	res, err := p.Channel().Run(ctx, `find "$CUBRID" -type f -iname 'core.*' 2>/dev/null`)
	if err != nil {
		return nil
	}
	var found []string
	for _, path := range strings.Split(strings.TrimSpace(res.Output()), "\n") {
		if path == "" || seen[path] {
			continue
		}
		_, suffix, _ := strings.Cut(filepath.Base(path), ".")
		// commons-lang 2.1's isNumeric, which says yes to the empty string.
		if strings.Trim(suffix, "0123456789") != "" {
			continue
		}
		seen[path] = true
		found = append(found, path)
	}
	return found
}

// coreErr is CQT's <case>.err (ConsoleBO.saveCoreCallStackFile): gdb's stack
// for each core the case left, written beside the case's failure copies.
//
// CQT writes them all after its last case. Here they are written as the case
// ends, because the core, the $CUBRID holding it and the program that dumped
// it are a slot's, and the slot is gone by the end of the run. The analysis
// runs in that slot for the same reason.
//
// Everything it can go wrong on is a warning and not a failure: a core is
// already the worst news the run has, and losing its stack is not a reason to
// lose the verdicts after it.
func (w *work) coreErr(ctx context.Context, p place, caseFile string, cores []string) {
	dir := w.rec.CaseResultDir(caseFile)
	if dir == "" {
		return
	}
	var stacks []coredump.Stack
	for _, core := range cores {
		s, err := coredump.Analyze(ctx, p.Channel(), core)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[WARN] no stack for %s: %v\n", core, err)
			continue
		}
		stacks = append(stacks, s)
	}
	if len(stacks) == 0 {
		return
	}
	// CORE_DIR is where the cores were left: the backup directory the
	// environment names, under this run's id, or $CUBRID.
	where := os.Getenv("CUBRID")
	if backup := os.Getenv("CORE_BACKUP_DIR"); backup != "" {
		where = filepath.Join(backup, w.rec.TestID())
	}
	path := filepath.Join(dir, strings.TrimSuffix(filepath.Base(caseFile), ".sql")+".err")
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] cannot write %s: %v\n", path, err)
		return
	}
	defer f.Close()
	if err := coredump.Report(f, where, stacks); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] cannot write %s: %v\n", path, err)
	}
}
