package perf

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// dbName is the database a fixture lives in: one per fixture, so a restore
// touches only that fixture's volumes (Spec §7.4).
func dbName(f *Fixture) string { return "perf_" + f.Name }

// hostDB is the host path of a side's /db.
func (s *Side) hostDB() string { return filepath.Join(s.Work, s.Name+"-n1", "db") }

// dbDir is a fixture database's own directory under /db, as createdb is told
// to use for its volumes and its logs (--file-path, --log-path). One
// directory per database, because the engine names a database's files by
// prefix and two fixtures whose names share a prefix (narrow, narrow_1m)
// would otherwise be one glob -- a restore of one deleting the other's
// volumes while its server is up.
func dbDir(db string) string { return "/db/" + db }

func (s *Side) hostDBDir(db string) string { return filepath.Join(s.hostDB(), db) }

// snapDir is where a fixture's volumes are kept between restores: a reflink
// copy on the same filesystem, so taking and restoring it is metadata work
// (M0: 0.012 s and 0.13 s for 5 GB on the hub's XFS).
func (s *Side) snapDir(db string) string { return filepath.Join(s.Work, "perf", "snap", db) }

// buildFixture makes the fixture's database on a side: createdb with the
// size the manifest gives, the schema, the loader, then a snapshot of the
// volumes with the server stopped, then the server back up with a statdump
// watcher on it. Done once per side per fixture, before any pass (Design §5.4).
func (r *Runner) buildFixture(ctx context.Context, s *Side, f *Fixture) error {
	db := dbName(f)
	r.logf("%s: fixture %s -> %s (%d rows, %s)", s.Role, f.Name, db, f.Rows, f.Size)
	dir := dbDir(db)
	log := func(step string) string { return fmt.Sprintf("%s/log/%s-%s.log", workPerf, step, db) }
	if err := r.mustExecLog(ctx, s, "createdb", log("createdb"),
		fmt.Sprintf("mkdir -p %s && cd %s && cubrid createdb --db-volume-size=%s --file-path=%s --log-path=%s %s en_US.utf8",
			dir, dir, f.Size, dir, dir, db), 10*time.Minute); err != nil {
		return err
	}
	if err := r.serverStart(ctx, s, db); err != nil {
		return err
	}
	// The vacuum statistics the wait below reads count only while a watcher
	// is registered, so one runs from before the load; serverStop ends it.
	if err := r.watcherStart(ctx, s, db); err != nil {
		return err
	}
	fdir := fmt.Sprintf("%s/fixtures/%s", workPerf, f.Name)
	if err := r.mustExecLog(ctx, s, "schema", log("schema"),
		fmt.Sprintf("csql -u dba %s -i %s/%s", db, fdir, shellJoin([]string{f.SchemaSQL})), 5*time.Minute); err != nil {
		return err
	}
	if err := r.mustExecLog(ctx, s, "load", log("load"),
		fmt.Sprintf("cd %s && bash %s/%s %s %d", fdir, fdir, shellJoin([]string{f.Load}), db, f.Rows), 30*time.Minute); err != nil {
		return err
	}
	// The snapshot is the state every pass starts from, so it must not carry
	// the loader's vacuum backlog or dirty pages: both would be paid inside
	// the first passes, and only on the side that happened to get them.
	if note, err := r.waitVacuum(ctx, s, db, 10*time.Minute); err != nil {
		return fmt.Errorf("%s: vacuum after load: %w", s.Role, err)
	} else if note != "" {
		r.logf("%s: %s", s.Role, note)
	}
	if err := r.mustExecLog(ctx, s, "checkpoint", log("checkpoint"),
		checkpointRequest(db), 5*time.Minute); err != nil {
		return err
	}
	if err := r.serverStop(ctx, s, db); err != nil {
		return err
	}
	if err := snapshot(s.hostDBDir(db), s.snapDir(db)); err != nil {
		return fmt.Errorf("%s: snapshot of %s: %w", s.Role, db, err)
	}
	if err := r.serverStart(ctx, s, db); err != nil {
		return err
	}
	if !s.paramsTried {
		r.recordParams(ctx, s, db)
	}
	return r.watcherStart(ctx, s, db)
}

// reset puts the fixture back the way the case assumes it, before every pass,
// the first one included (FR-13).
func (r *Runner) reset(ctx context.Context, s *Side, c *Case, f *Fixture) error {
	db := dbName(f)
	switch f.Reset {
	case "none":
		return nil
	case "truncate_and_reload":
		// The case's reset.sql when it has one (a read-only case has none),
		// then the loader, which is a no-op on a table that still holds its
		// rows and refills one that was truncated.
		fdir := fmt.Sprintf("%s/fixtures/%s", workPerf, f.Name)
		if exists(filepath.Join(c.Dir, "reset.sql")) {
			if err := r.mustExecLog(ctx, s, "reset.sql", fmt.Sprintf("%s/log/reset-%s.log", workPerf, c.ID),
				fmt.Sprintf("csql -u dba %s -i %s/cases/%s/reset.sql", db, workPerf, c.ID), 10*time.Minute); err != nil {
				return err
			}
		}
		return r.mustExecLog(ctx, s, "reload", fmt.Sprintf("%s/log/reload-%s.log", workPerf, db),
			fmt.Sprintf("cd %s && bash %s/%s %s %d", fdir, fdir, shellJoin([]string{f.Load}), db, f.Rows), 30*time.Minute)
	case ResetRestoreSnapshot:
		if err := r.serverStop(ctx, s, db); err != nil {
			return err
		}
		if err := restore(s.hostDBDir(db), s.snapDir(db)); err != nil {
			return fmt.Errorf("%s: restore of %s: %w", s.Role, db, err)
		}
		if err := r.serverStart(ctx, s, db); err != nil {
			return err
		}
		return r.watcherStart(ctx, s, db)
	}
	return fmt.Errorf("fixture %s: reset %q is not one this runner knows", f.Name, f.Reset)
}

// snapshot copies a database directory's contents out. Plain copies, not
// reflinks: a live volume that shared extents with its snapshot would pay
// XFS copy-on-write on the first write to each block, inside the measured
// window and on the hub only (M0 measured reflinks at 0.13 s and plain
// copies at 0.8 s for 5 GB; the difference is not worth a bias).
func snapshot(dbDir, snap string) error {
	if err := os.RemoveAll(snap); err != nil {
		return err
	}
	if err := os.MkdirAll(snap, 0o755); err != nil {
		return err
	}
	entries, err := dirEntries(dbDir)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return fmt.Errorf("nothing under %s", dbDir)
	}
	return reflinkCopy(entries, snap)
}

// restore puts the snapshot back: the database directory's contents are
// removed and the snapshot's copied in, and the directory itself is never
// removed -- it sits under a bind mount, and a directory made anew in its
// place is not what the container holds (Design §5.4, D10; M0 confirmed
// both ways).
func restore(dbDir, snap string) error {
	snapped, err := dirEntries(snap)
	if err != nil || len(snapped) == 0 {
		return fmt.Errorf("no snapshot under %s", snap)
	}
	entries, err := dirEntries(dbDir)
	if err != nil {
		return err
	}
	for _, f := range entries {
		if err := os.RemoveAll(f); err != nil {
			return err
		}
	}
	return reflinkCopy(snapped, dbDir)
}

func dirEntries(dir string) ([]string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(ents))
	for _, e := range ents {
		out = append(out, filepath.Join(dir, e.Name()))
	}
	return out, nil
}

func reflinkCopy(src []string, dstDir string) error {
	args := append([]string{"-a", "--reflink=never"}, src...)
	args = append(args, dstDir+string(os.PathSeparator))
	if out, err := exec.Command("cp", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("cp: %v: %s", err, out)
	}
	return exec.Command("sync").Run()
}

// waitVacuum waits until vacuum is idle, or the bound passes.
// Num_vacuum_log_pages_to_vacuum and Num_vacuum_log_pages_vacuumed are
// accumulators -- pages of the log blocks handed to vacuum, and of those it
// has finished -- and the engine adds to them only while a statdump watcher
// is registered, so each read also checks that the watcher is up and re-arms
// it when it is not (a dead watcher freezes both, and a frozen 0/0 reads as
// idle). Their difference is the backlog, give or take what was counted while
// no watcher was: blocks handed over before a restart and finished after it
// make it negative, and a negative offset can hide new backlog, so idle is
// the difference at or below 0 with vacuumed not moving over one interval
// when the difference is 0, two when it is below (vacuumIdle). Records in the
// log block still being filled are in neither until the block is handed over,
// so up to one block can still be vacuumed after the wait. The note says when
// the bound hit; the caller decides what that means.
func (r *Runner) waitVacuum(ctx context.Context, s *Side, db string, bound time.Duration) (note string, err error) {
	deadline := time.Now().Add(bound)
	var reads []vacuumCounts
	rearmed := false
	for {
		res, err := r.exec(ctx, s.N1, fmt.Sprintf("bash %s/scripts/statdump_window.sh status %s >/dev/null 2>&1 && echo watcher=up; "+
			"cubrid statdump %s 2>/dev/null | grep -E '^Num_vacuum_log_pages_(to_vacuum|vacuumed) '", workPerf, db, db), time.Minute)
		if err != nil {
			return "", err
		}
		if !strings.Contains(res.Stdout, "watcher=up") {
			if err := r.watcherStart(ctx, s, db); err != nil {
				return "", err
			}
			// What was counted before is not comparable with what comes now.
			reads, rearmed = nil, true
		} else {
			reads = append(reads, parseVacuumCounts(res.Stdout))
			if vacuumIdle(reads) {
				if rearmed {
					return "statdump watcher was down during the vacuum wait and was re-armed", nil
				}
				return "", nil
			}
		}
		if time.Now().After(deadline) {
			var cur vacuumCounts
			if len(reads) > 0 {
				cur = reads[len(reads)-1]
			}
			switch {
			case !cur.ok:
				return fmt.Sprintf("vacuum statistics unreadable after %s", bound), nil
			case cur.backlog() > 0:
				return fmt.Sprintf("vacuum still had %d log pages to do after %s", cur.backlog(), bound), nil
			default:
				return fmt.Sprintf("vacuum was still working after %s", bound), nil
			}
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// checkpointRequest is csql's ;checkpoint session command: only a --sysadm
// csql may run it, and only from its input, not -c (which is SQL). There is
// no checkpoint utility in 11.x. It only wakes the checkpoint daemon and
// returns; a wake-up while the daemon is busy is dropped (waiter::wakeup).
func checkpointRequest(db string) string {
	return fmt.Sprintf("printf ';checkpoint\\n' | csql --sysadm -u dba %s", db)
}

// checkpoint takes a checkpoint and waits until it has ended, or the bound
// passes: a checkpoint left running ended inside the window that followed
// (txn.commit_group_64 on 19 passes in 20, 2026-10-30). The waiting is
// checkpointWait's; this gives it the statdump read, with the watcher's
// status in the same exec (the counters count only while one is registered),
// the request and the re-arm, and logs how long it took.
func (r *Runner) checkpoint(ctx context.Context, s *Side, db string, bound time.Duration) (note string, err error) {
	w := checkpointWait{
		read: func() (checkpointCounts, bool, error) {
			res, err := r.exec(ctx, s.N1, fmt.Sprintf("bash %s/scripts/statdump_window.sh status %s >/dev/null 2>&1 && echo watcher=up; "+
				"cubrid statdump %s 2>/dev/null | grep -E '^Num_log_(start|end)_checkpoints '", workPerf, db, db), time.Minute)
			if err != nil {
				return checkpointCounts{}, false, err
			}
			return parseCheckpointCounts(res.Stdout), strings.Contains(res.Stdout, "watcher=up"), nil
		},
		ask: func() error {
			return r.mustExec(ctx, s, "checkpoint", fmt.Sprintf("%s > %s/log/checkpoint-%s.log 2>&1", checkpointRequest(db), workPerf, db), 5*time.Minute)
		},
		rearm: func() error { return r.watcherStart(ctx, s, db) },
		bound: bound, poll: time.Second, reask: 5 * time.Second, running: 2 * time.Minute, unreadable: 10,
		now: time.Now, sleep: sleepCtx,
	}
	// A lasting offset is waited out once per server start, not every pass:
	// the counters start over with the server (serverStart forgets it).
	offset := s.ckptOffset[db]
	w.offset = &offset
	start := time.Now()
	note, err = w.run(ctx)
	if s.ckptOffset == nil {
		s.ckptOffset = map[string]int64{}
	}
	s.ckptOffset[db] = offset
	r.logf("%s %s: checkpoint before the window took %s", s.Role, db, time.Since(start).Round(time.Millisecond))
	return note, err
}

// checkpointWait waits for one checkpoint asked for after a baseline read of
// the start and end counters. Done is a checkpoint started since the
// baseline and as many ended as started since (checkpointDone): deltas, not
// the counts, because the two can be apart for good -- a failed checkpoint
// counts a start and no end, and either misses what ran while no watcher
// was registered.
//   - A checkpoint running at the baseline (more started than ended, past
//     the offset already known) is waited for up to running; one that does
//     not end by then is a lasting offset, said in a note and kept in
//     offset for the next waits on that server, and the baseline is taken
//     as it is.
//   - With none started since the request after reask, the request was
//     dropped by one running then, and it asks again -- never in the last
//     poll before the bound, where a new one would run into the window.
//   - A watcher found down is re-armed, and the baseline and the request
//     are taken again: what ran meanwhile was not counted. One that cannot
//     be re-armed reads as unreadable.
//   - An unreadable baseline: asked for, not waited for. unreadable reads
//     in a row: the wait ends. Both with a note.
type checkpointWait struct {
	read                        func() (c checkpointCounts, watcherUp bool, err error)
	ask, rearm                  func() error
	bound, poll, reask, running time.Duration
	unreadable                  int
	offset                      *int64 // started minus ended known to last on this server
	now                         func() time.Time
	sleep                       func(context.Context, time.Duration) error
}

func (w checkpointWait) run(ctx context.Context) (string, error) {
	deadline := w.now().Add(w.bound)
	var notes []string
	say := func(note string) string {
		notes = append(notes, note)
		return strings.Join(notes, "; ")
	}
	misses := 0
	rearmFailed := false
	// next reads the counters with the watcher up, re-arming it when it
	// is down; ok false is an unreadable read, which a watcher that cannot
	// be re-armed also gives (the request still goes out).
	next := func() (c checkpointCounts, rearmed bool, err error) {
		c, up, err := w.read()
		if err != nil || up {
			return c, false, err
		}
		if err := w.rearm(); err != nil {
			if !rearmFailed {
				rearmFailed = true
				say("statdump watcher down and not re-armed: " + err.Error())
			}
			return checkpointCounts{}, false, nil
		}
		say("statdump watcher was down during the checkpoint wait and was re-armed")
		c, _, err = w.read()
		return c, true, err
	}
	base, _, err := next()
	if err != nil {
		return "", err
	}
	if !base.ok {
		if err := w.ask(); err != nil {
			return "", err
		}
		return say("checkpoint counters unreadable; the checkpoint before the window was asked for and not waited for"), nil
	}
	// The offset only shrinks: a read with fewer started past ended was
	// taken with nothing running, so a larger one stored was a checkpoint
	// that ran past running, not a lasting gap.
	shrink := func(c checkpointCounts) {
		if d := c.started - c.ended; c.ok && d < *w.offset {
			*w.offset = d
		}
	}
	if stored := *w.offset; stored != 0 {
		if shrink(base); *w.offset == stored {
			say(fmt.Sprintf("checkpoint counter offset %d (started minus ended) stored by an earlier wait on this server applied", stored))
		} else {
			say(fmt.Sprintf("checkpoint counter offset %d stored by an earlier wait on this server dropped to %d", stored, *w.offset))
		}
	} else {
		shrink(base)
	}
	ahead := func(c checkpointCounts) bool { return c.started-c.ended > *w.offset }
	if ahead(base) {
		until := w.now().Add(w.running)
		for ahead(base) && w.now().Before(until) {
			if err := w.sleep(ctx, w.poll); err != nil {
				return "", err
			}
			c, _, err := next()
			if err != nil {
				return "", err
			}
			if c.ok {
				base = c
			}
		}
		if ahead(base) {
			*w.offset = base.started - base.ended
			say(fmt.Sprintf("checkpoint counters %d started, %d ended for %s: taken as an offset", base.started, base.ended, w.running))
		}
	}
	if err := w.ask(); err != nil {
		return "", err
	}
	asked := w.now()
	var c checkpointCounts
	for {
		if err := w.sleep(ctx, w.poll); err != nil {
			return "", err
		}
		var rearmed bool
		if c, rearmed, err = next(); err != nil {
			return "", err
		}
		if c.ok && !rearmed {
			shrink(c)
			// A checkpoint running at the baseline ended (one taken as an
			// offset, or one the offset hid): what started since is
			// measured from here.
			if c.started == base.started && c.ended > base.ended {
				base = c
			}
		}
		switch {
		case rearmed && c.ok:
			base = c
			if err := w.ask(); err != nil {
				return "", err
			}
			asked = w.now()
		case !c.ok:
			if misses++; misses >= w.unreadable {
				return say(fmt.Sprintf("checkpoint counters unreadable %d times in a row; stopped waiting", misses)), nil
			}
		case checkpointDone(base, c):
			return strings.Join(notes, "; "), nil
		default:
			misses = 0
			if c.started == base.started && w.now().Sub(asked) >= w.reask && deadline.Sub(w.now()) > w.poll {
				if err := w.ask(); err != nil {
					return "", err
				}
				asked = w.now()
			}
		}
		if w.now().After(deadline) {
			if !c.ok {
				return say(fmt.Sprintf("the checkpoint before the window had not ended after %s (counters unreadable)", w.bound)), nil
			}
			return say(fmt.Sprintf("the checkpoint before the window had not ended after %s (started %d -> %d, ended %d -> %d)", w.bound, base.started, c.started, base.ended, c.ended)), nil
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// checkpointCounts is one statdump read of the checkpoint counters.
type checkpointCounts struct {
	started, ended int64
	ok             bool
}

// checkpointDone says a checkpoint started since base and as many ended as
// started since.
func checkpointDone(base, now checkpointCounts) bool {
	return base.ok && now.ok && now.started > base.started && now.ended-base.ended >= now.started-base.started
}

func parseCheckpointCounts(statdump string) checkpointCounts {
	n, ok := statCounts(statdump, "Num_log_start_checkpoints", "Num_log_end_checkpoints")
	return checkpointCounts{started: n[0], ended: n[1], ok: ok}
}

// vacuumCounts is one statdump read of the two vacuum accumulators.
type vacuumCounts struct {
	toVacuum, vacuumed int64
	ok                 bool
}

func (v vacuumCounts) backlog() int64 { return v.toVacuum - v.vacuumed }

func parseVacuumCounts(statdump string) vacuumCounts {
	n, ok := statCounts(statdump, "Num_vacuum_log_pages_to_vacuum", "Num_vacuum_log_pages_vacuumed")
	return vacuumCounts{toVacuum: n[0], vacuumed: n[1], ok: ok}
}

// statCounts reads the named statistics from statdump lines, in the order
// named; ok only with all of them.
func statCounts(statdump string, names ...string) ([]int64, bool) {
	out := make([]int64, len(names))
	have := 0
	for _, line := range strings.Split(statdump, "\n") {
		k, val, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		n, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
		if err != nil {
			continue
		}
		for i, name := range names {
			if strings.TrimSpace(k) == name {
				out[i] = n
				have |= 1 << i
			}
		}
	}
	return out, have == 1<<len(names)-1
}

// vacuumIdle says the last reads, oldest first, show vacuum caught up and
// not working: every one readable with the backlog at or below 0 and the same
// vacuumed, over two reads when the backlog reads 0 and three when it reads
// below 0 (an offset from before a restart, which can hide new blocks).
func vacuumIdle(reads []vacuumCounts) bool {
	n := len(reads)
	if n < 2 {
		return false
	}
	need := 2
	if reads[n-1].backlog() < 0 {
		need = 3
	}
	if n < need {
		return false
	}
	last := reads[n-1]
	for _, r := range reads[n-need:] {
		if !r.ok || r.backlog() > 0 || r.vacuumed != last.vacuumed {
			return false
		}
	}
	return true
}

func firstNonEmptyStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (r *Runner) mustExec(ctx context.Context, s *Side, what, script string, timeout time.Duration) error {
	res, err := r.exec(ctx, s.N1, script, timeout)
	if err != nil {
		return fmt.Errorf("%s: %s: %w", s.Role, what, err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("%s: %s: exit %d: %s", s.Role, what, res.ExitCode, tail(res.Stderr+"\n"+res.Stdout, 400))
	}
	return nil
}

// mustExecLog is mustExec with the step's output in a file in the node, and
// that file's tail in the error when the step fails: the file is what says
// why, and it goes with the cluster.
func (r *Runner) mustExecLog(ctx context.Context, s *Side, what, log, script string, timeout time.Duration) error {
	return r.mustExec(ctx, s, what, fmt.Sprintf("{ %s; } > %s 2>&1; rc=$?; [ $rc -eq 0 ] || tail -20 %s >&2; exit $rc", script, log, log), timeout)
}
