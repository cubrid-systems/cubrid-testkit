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
		fmt.Sprintf("printf ';checkpoint\\n' | csql --sysadm -u dba %s", db), 5*time.Minute); err != nil {
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

// waitVacuum waits until vacuum is idle with nothing left, or the bound
// passes. Num_vacuum_log_pages_to_vacuum and Num_vacuum_log_pages_vacuumed
// are accumulators -- pages of the log blocks handed to vacuum, and of those
// it has finished -- and the engine adds to them only while a statdump
// watcher is registered, so the caller keeps one running. Their difference
// is the backlog, give or take what was counted while no watcher was
// (blocks handed over before a restart and finished after it make it
// negative), so idle is two reads in a row with the difference at or below 0
// and vacuumed not moving (vacuumIdle). Records in the log block still being
// filled are in neither until the block is handed over. The note says when
// the bound hit; the caller decides what that means.
func (r *Runner) waitVacuum(ctx context.Context, s *Side, db string, bound time.Duration) (note string, err error) {
	deadline := time.Now().Add(bound)
	var prev vacuumCounts
	for first := true; ; first = false {
		res, err := r.exec(ctx, s.N1, fmt.Sprintf("cubrid statdump %s 2>/dev/null | grep -E '^Num_vacuum_log_pages_(to_vacuum|vacuumed) '", db), time.Minute)
		if err != nil {
			return "", err
		}
		cur := parseVacuumCounts(res.Stdout)
		if !first && vacuumIdle(prev, cur) {
			return "", nil
		}
		prev = cur
		if time.Now().After(deadline) {
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

// vacuumCounts is one statdump read of the two vacuum accumulators.
type vacuumCounts struct {
	toVacuum, vacuumed int64
	ok                 bool
}

func (v vacuumCounts) backlog() int64 { return v.toVacuum - v.vacuumed }

// parseVacuumCounts reads Num_vacuum_log_pages_to_vacuum and
// Num_vacuum_log_pages_vacuumed from statdump lines; ok only with both.
func parseVacuumCounts(statdump string) vacuumCounts {
	var v vacuumCounts
	var haveTo, haveDone bool
	for _, line := range strings.Split(statdump, "\n") {
		k, val, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		n, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
		if err != nil {
			continue
		}
		switch strings.TrimSpace(k) {
		case "Num_vacuum_log_pages_to_vacuum":
			v.toVacuum, haveTo = n, true
		case "Num_vacuum_log_pages_vacuumed":
			v.vacuumed, haveDone = n, true
		}
	}
	v.ok = haveTo && haveDone
	return v
}

// vacuumIdle says two reads in a row show vacuum caught up and not working:
// both readable, the backlog at or below 0 in both, vacuumed the same.
func vacuumIdle(prev, cur vacuumCounts) bool {
	return prev.ok && cur.ok && prev.backlog() <= 0 && cur.backlog() <= 0 && prev.vacuumed == cur.vacuumed
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
