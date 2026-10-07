package perf

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	osexec "os/exec"

	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
	"github.com/cubrid-systems/cubrid-testkit/internal/sandbox"
)

// Side is one of the two clusters a comparison runs on: the target build or
// the reference build, each a single database node and one client node.
type Side struct {
	Role  string // target or reference
	Name  string // the cluster name, pf-<yyyymmdd>-<hex4>-{t,r}
	Build string
	CLI   *sandbox.CLI
	N1    *sandbox.Node
	C1    *sandbox.Node
	// Work is the host path of the cluster's /work; perf/ under it is this
	// runner's, and every file a pass writes lands there first.
	Work string
	// serial numbers the statdump watcher files per database, because a
	// restarted watcher with the same -o path would overwrite the series.
	watchers map[string]int
	startN   map[string]int
	// running is the databases whose server is up; built the fixtures made
	// on this side. The session keeps one fixture server up per side (§5.1).
	running map[string]bool
	built   map[string]bool
	// confKey is the conf group the cluster was created for; a group with
	// another key needs another cluster.
	confKey string
	// containers is what the runtime calls this cluster's containers, for
	// pause; cgroup is the server node's, for guard stage 3.
	containers []string
	cgroup     string
}

// Pinning says how the server and the client were kept apart: cpuset when the
// runtime enforced it, none when nothing was asked (a local run).
const (
	PinningCPUSet  = "cpuset"
	PinningPartial = "partial" // only one of the two sets was given
	PinningNone    = "none"
)

// ClusterName is pf-<yyyymmdd>-<hex4>-<suffix>: csb's rule is ^[a-z][a-z0-9-]{0,30}$,
// so no underscore and no branch name, and the hash is of the session id so
// two sessions on one day do not collide (Design §5.3).
func ClusterName(sessionID string, day time.Time, suffix string) string {
	return fmt.Sprintf("pf-%s-%s-%s", day.Format("20060102"), Hex4(sessionID), suffix)
}

// Hex4 is the first four hex digits of the session id's hash.
func Hex4(sessionID string) string {
	sum := sha1.Sum([]byte(sessionID))
	return hex.EncodeToString(sum[:])[:4]
}

// workPerf is where this runner's files live inside every node.
const workPerf = "/work/perf"

// seedDB is the database csb makes when it creates the cluster. It is not a
// fixture and its server is stopped right after; it is named here because
// csb's default is the cluster name, and a database named pf-<date>-<hex>-t
// is one createdb will not make (the engine builds its log names from it).
const seedDB = "seed"

// brokerCAS is the CAS count the broker is pinned to, so that no CAS is
// spawned inside a measured window (sandbox --broker-set, S6): the largest
// client count among the cases the cluster serves (Spec §7.2 clients), and
// one when none of them goes through the broker.
func brokerCAS(cases []*Case) int {
	n := 1
	for _, c := range cases {
		if c.Driver == "jdbc" && c.Clients > n {
			n = c.Clients
		}
	}
	return n
}

// createSide stands a side up for the cases it will run: the union of their
// cubrid.conf overrides, the pinned CAS count, the pinning, the client image,
// and then the work directory staged and the clients compiled.
func (r *Runner) createSide(ctx context.Context, role, suffix, build string, cases []*Case) (*Side, error) {
	s := &Side{
		Role: role, Name: ClusterName(r.SessionID, r.Started, suffix), Build: build,
		watchers: map[string]int{}, startN: map[string]int{}, running: map[string]bool{}, built: map[string]bool{},
		confKey: groupKey(cases),
	}
	s.CLI = &sandbox.CLI{Bin: r.CSBBin, Cluster: s.Name}
	s.N1 = sandbox.NewNode(s.CLI, "n1")
	s.C1 = sandbox.NewNode(s.CLI, "c1")
	s.Work = sandbox.WorkDir(s.Name)

	// A cluster of this name left by an earlier session would be resumed by
	// create, not replaced; it goes first (Design §7).
	if removed, err := s.CLI.DestroyPurge(ctx); err == nil && len(removed) > 0 {
		r.logf("%s: removed what was left of an earlier %s: %s", s.Role, s.Name, strings.Join(removed, " "))
	}
	set := confUnion(cases)
	opts := sandbox.CreateOptions{
		Preset: "single", Build: build, DB: seedDB, Clients: 1, ClientImage: r.ClientImage, WithBroker: true,
		CPUSet: r.CPUSet, ClientCPUSet: r.ClientCPUSet,
		Set: set,
		BrokerSet: []string{
			fmt.Sprintf("MIN_NUM_APPL_SERVER=%d", brokerCAS(cases)), fmt.Sprintf("MAX_NUM_APPL_SERVER=%d", brokerCAS(cases)),
			"SQL_LOG=OFF",
		},
		Labels:  map[string]string{"perf.session": r.SessionID, "perf.role": role},
		Timeout: 10 * time.Minute,
	}
	r.logf("%s: creating %s from %s (--set %s)", role, s.Name, build, strings.Join(set, " "))
	// On failure the side is returned with the error: csb may have left the
	// containers of a cluster that did not reach serving, and the caller's
	// teardown has to know their name.
	if err := s.CLI.CreateWith(ctx, opts); err != nil {
		return s, fmt.Errorf("%s: cluster create: %w", role, err)
	}
	// The work directory first: every node-side step from here logs into it.
	if err := r.stageWork(s, cases); err != nil {
		return s, err
	}
	// csb's own database is not a fixture; its server would be an idle
	// neighbour of every measured pass, and collect_l0.sh would count it. Stop
	// it, and know that it stopped.
	if err := r.serverStop(ctx, s, seedDB); err != nil {
		return s, err
	}
	if err := r.compileClients(ctx, s, cases); err != nil {
		return s, err
	}
	if r.Guard != nil {
		if cg, err := containerCgroup(s.Name + "-n1"); err != nil {
			r.logf("%s: guard: %v (foreign CPU will count this cluster's)", s.Role, err)
		} else {
			s.cgroup = cg
			r.Guard.cgroups = append(r.Guard.cgroups, cg)
		}
	}
	return s, nil
}

// groupKey is confKey over a set of cases that share a cluster.
func groupKey(cases []*Case) string {
	if len(cases) == 0 {
		return ""
	}
	return confKey(cases[0])
}

// pauseSide freezes every container of a cluster (Design §5.1): the runtime's
// pause, by the cluster label, because csb has no verb for it (§13 11).
func (r *Runner) pauseSide(s *Side) error {
	return r.runtimeOnCluster(s, "pause")
}

// unpauseSide thaws them. A container that is not paused is not an error:
// the runtime says so and the state is the one wanted.
func (r *Runner) unpauseSide(s *Side) error {
	return r.runtimeOnCluster(s, "unpause")
}

func (r *Runner) runtimeOnCluster(s *Side, verb string) error {
	if len(s.containers) == 0 {
		names, err := clusterContainers(s.Name)
		if err != nil {
			return err
		}
		if len(names) == 0 {
			return fmt.Errorf("%s: no container carries csb.cluster=%s", s.Role, s.Name)
		}
		s.containers = names
	}
	var failed []string
	for _, name := range s.containers {
		out, err := osexec.Command(runtimeBin(), verb, name).CombinedOutput()
		if err != nil {
			msg := strings.TrimSpace(string(out))
			if verb == "unpause" && strings.Contains(msg, "not paused") {
				continue
			}
			if verb == "pause" && strings.Contains(msg, "already paused") {
				continue
			}
			failed = append(failed, name+": "+tail(msg, 160))
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("%s %s: %s", runtimeBin(), verb, strings.Join(failed, "; "))
	}
	return nil
}

// buildFixtures makes every fixture the cases need that this side does not
// have yet, and leaves their servers down: the session keeps up only the
// server of the case it is running (§5.1).
func (r *Runner) buildFixtures(ctx context.Context, s *Side, suite *Suite, cases []*Case) error {
	for _, name := range fixturesOf(cases) {
		if s.built[name] {
			continue
		}
		f := suite.Fixtures[name]
		if f == nil {
			return fmt.Errorf("%s: fixture %s is not in the suite", s.Role, name)
		}
		if err := r.buildFixture(ctx, s, f); err != nil {
			return err
		}
		s.built[name] = true
		if err := r.serverStop(ctx, s, dbName(f)); err != nil {
			return err
		}
	}
	return nil
}

// startFixtureServer brings the case's fixture server up with its watcher
// when it is not up already.
func (r *Runner) startFixtureServer(ctx context.Context, s *Side, f *Fixture) error {
	db := dbName(f)
	if s.running[db] {
		return nil
	}
	if err := r.serverStart(ctx, s, db); err != nil {
		return err
	}
	return r.watcherStart(ctx, s, db)
}

// stopOthers takes down every other fixture server on the side, watcher
// first (serverStop does that), so their daemons and data_buffer_size are
// out of the way (§5.1, §7 memory).
func (r *Runner) stopOthers(ctx context.Context, s *Side, f *Fixture) error {
	keep := dbName(f)
	dbs := make([]string, 0, len(s.running))
	for db := range s.running {
		if db != keep {
			dbs = append(dbs, db)
		}
	}
	sort.Strings(dbs)
	for _, db := range dbs {
		if err := r.serverStop(ctx, s, db); err != nil {
			return err
		}
	}
	return nil
}

// confUnion is every case's cubrid.conf override, key=value, sorted; the
// cases that share a cluster share a conf group, so two values for one key is
// a refusal before anything is built.
func confUnion(cases []*Case) []string {
	seen := map[string]string{}
	for _, c := range cases {
		for k, v := range c.Conf {
			if prev, ok := seen[k]; ok && prev != v {
				// Caught by groupCases before we get here; keep the first.
				continue
			}
			seen[k] = v
		}
	}
	out := make([]string, 0, len(seen))
	for k, v := range seen {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}

// stageWork copies what the nodes will read into <work>/perf: the scripts,
// every client source (the common class with them), each case's reset.sql,
// each fixture's files. /work is bind-mounted into every node, so one copy
// serves the server node and the client node (Design §5.3).
func (r *Runner) stageWork(s *Side, cases []*Case) error {
	root := filepath.Join(s.Work, "perf")
	for _, d := range []string{"scripts", "src", "cases", "fixtures", "log", "out", "statdump", "tmp", "classes", "snap"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			return err
		}
	}
	if err := copyTree(filepath.Join(r.SuiteDir, "scripts"), filepath.Join(root, "scripts")); err != nil {
		return fmt.Errorf("staging scripts: %w", err)
	}
	if err := copyGlob(filepath.Join(r.SuiteDir, "scripts", "common", "*.java"), filepath.Join(root, "src")); err != nil {
		return err
	}
	fixtures := map[string]bool{}
	seen := map[string]string{}
	for _, c := range cases {
		fixtures[c.Fixture.Name] = true
		// Every case's sources share one directory, so one name in two
		// cases would be one of them running the other's program.
		srcs, _ := filepath.Glob(filepath.Join(c.Dir, "src", "*"))
		for _, f := range srcs {
			base := filepath.Base(f)
			if other, dup := seen[base]; dup {
				return fmt.Errorf("source %s is in both %s and %s; client sources share one directory", base, other, c.ID)
			}
			seen[base] = c.ID
		}
		if err := copyGlob(filepath.Join(c.Dir, "src", "*"), filepath.Join(root, "src")); err != nil {
			return err
		}
		if reset := filepath.Join(c.Dir, "reset.sql"); exists(reset) {
			dst := filepath.Join(root, "cases", c.ID)
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
			if err := copyFile(reset, filepath.Join(dst, "reset.sql")); err != nil {
				return err
			}
		}
	}
	for f := range fixtures {
		if err := copyTree(filepath.Join(r.SuiteDir, "fixtures", f), filepath.Join(root, "fixtures", f)); err != nil {
			return fmt.Errorf("staging fixture %s: %w", f, err)
		}
	}
	return nil
}

// compileClients builds the clients inside the client node, where the
// toolchain is: the Java ones against the engine's own JDBC driver, a C one
// (cdc-api, src/<bin>.c) against libcubridcs in the bind-mounted tree.
func (r *Runner) compileClients(ctx context.Context, s *Side, cases []*Case) error {
	java := false
	var cBins []string
	for _, c := range cases {
		switch c.Driver {
		case "jdbc":
			java = true
		case "cdc-api":
			cBins = append(cBins, c.CDC.Bin)
		}
	}
	if java {
		script := `set -e; ` + jdbcJar + `; ` +
			`[ -n "$jar" ] || { echo "no JDBC driver under /opt/cubrid-ro/jdbc" >&2; exit 3; }; ` +
			`javac -encoding UTF-8 -cp "$jar" -d ` + workPerf + `/classes ` + workPerf + `/src/*.java`
		res, err := r.exec(ctx, s.C1, script, 3*time.Minute)
		if err != nil {
			return fmt.Errorf("%s: compiling clients: %w", s.Role, err)
		}
		if res.ExitCode != 0 {
			return fmt.Errorf("%s: javac failed (exit %d): %s", s.Role, res.ExitCode, tail(res.Stderr+res.Stdout, 800))
		}
	}
	for _, bin := range cBins {
		script := fmt.Sprintf(`set -e; mkdir -p %s/bin && gcc -O2 -o %s/bin/%s %s/src/%s.c -I/opt/cubrid-ro/include -L/opt/cubrid-ro/lib -lcubridcs -Wl,-rpath,/opt/cubrid-ro/lib`,
			workPerf, workPerf, shellJoin([]string{bin}), workPerf, shellJoin([]string{bin}))
		res, err := r.exec(ctx, s.C1, script, 3*time.Minute)
		if err != nil {
			return fmt.Errorf("%s: compiling %s: %w", s.Role, bin, err)
		}
		if res.ExitCode != 0 {
			return fmt.Errorf("%s: gcc failed for %s (exit %d): %s", s.Role, bin, res.ExitCode, tail(res.Stderr+res.Stdout, 800))
		}
	}
	return nil
}

// jdbcJar sets $jar to the engine's JDBC driver. The install tree ships the
// driver beside its -javadoc and -sources jars, and a glob sorted by name
// puts -javadoc first -- which compiles (java.sql is all the clients import)
// and then fails at run time with ClassNotFoundException.
const jdbcJar = `jar=$(ls /opt/cubrid-ro/jdbc/cubrid_jdbc.jar /opt/cubrid-ro/jdbc/cubrid-jdbc-*.jar 2>/dev/null | grep -v -e -javadoc -e -sources | head -1)`

// serverStart starts a database's server with its output in a file: a pipe
// would be inherited by the daemon and the exec would wait on it until the
// timeout (csb T8; seen again on the hub in M0).
func (r *Runner) serverStart(ctx context.Context, s *Side, db string) error {
	s.startN[db]++
	log := fmt.Sprintf("%s/log/start-%s-%d.log", workPerf, db, s.startN[db])
	script := fmt.Sprintf("cubrid server start %s > %s 2>&1; rc=$?; tail -3 %s; exit $rc", db, log, log)
	res, err := r.exec(ctx, s.N1, script, 5*time.Minute)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("%s: cubrid server start %s: exit %d: %s", s.Role, db, res.ExitCode, tail(res.Stdout, 300))
	}
	s.running[db] = true
	return nil
}

// serverStop stops a database's server and waits until the process is gone.
// A server that is not running is already stopped -- a pass after a failed
// start must not fail again on the stop. The statdump watcher goes first:
// it is a client of the server, and a watcher still attached when the next
// start comes refuses the new one (Design §5.4).
func (r *Runner) serverStop(ctx context.Context, s *Side, db string) error {
	if _, err := r.exec(ctx, s.N1, fmt.Sprintf("bash %s/scripts/statdump_window.sh stop %s >/dev/null 2>&1; true", workPerf, shellJoin([]string{db})), time.Minute); err != nil {
		return err
	}
	script := fmt.Sprintf(`pgrep -f '^cub_server %s$' >/dev/null || exit 0; cubrid server stop %s > %s/log/stop-%s.log 2>&1; rc=$?; `+
		`for i in $(seq 1 60); do pgrep -f '^cub_server %s$' >/dev/null || exit $rc; sleep 1; done; echo 'still running' >&2; exit 1`,
		db, db, workPerf, db, db)
	res, err := r.exec(ctx, s.N1, script, 3*time.Minute)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("%s: cubrid server stop %s: exit %d: %s", s.Role, db, res.ExitCode, tail(res.Stderr, 300))
	}
	delete(s.running, db)
	return nil
}

// exec runs a script on a node with a bound on both clocks and returns the
// result; a non-zero exit is the caller's to read (Design §5.3: data.<node>.exit).
func (r *Runner) exec(ctx context.Context, n *sandbox.Node, script string, timeout time.Duration) (exec.Result, error) {
	res, err := n.RunFor(ctx, script, timeout)
	if err != nil {
		return res, fmt.Errorf("%s: %w", n.Describe(), err)
	}
	return res, nil
}

// destroySide takes a side down. It is called from a defer with its own
// context, because the one the run had may already be cancelled by the
// signal that is the reason for the teardown.
func (r *Runner) destroySide(s *Side) error {
	if s == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	// Purged, not just destroyed: the describe artifact is already in the
	// results directory, and a record left behind is what the next session's
	// "remove what is left" would otherwise have to deal with.
	if _, err := s.CLI.DestroyPurge(ctx); err != nil {
		r.logf("%s: destroy %s: %v", s.Role, s.Name, err)
		return fmt.Errorf("%s %s: %w", s.Role, s.Name, err)
	}
	r.logf("%s: destroyed %s", s.Role, s.Name)
	return nil
}

// keepLogs copies the node-side logs (server start/stop, createdb, the
// loader, the watchers) to the results directory, because /work goes with
// the cluster and a failure's cause is in exactly those files.
func (r *Runner) keepLogs(s *Side, pairDir string) {
	if s == nil {
		return
	}
	for _, d := range []string{"log", "statdump"} {
		src := filepath.Join(s.Work, "perf", d)
		if !exists(src) {
			continue
		}
		if err := copyTree(src, filepath.Join(pairDir, "logs", s.Role, d)); err != nil {
			r.logf("keeping %s: %v", src, err)
		}
	}
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyGlob(pattern, dstDir string) error {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}
	for _, m := range matches {
		if st, err := os.Stat(m); err != nil || st.IsDir() {
			continue
		}
		if err := copyFile(m, filepath.Join(dstDir, filepath.Base(m))); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	st, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, b, st.Mode().Perm()|0o600)
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return "…" + s[len(s)-n:]
	}
	return s
}

var errNotOK = errors.New("exit != 0")
