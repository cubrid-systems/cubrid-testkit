package perf

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/sandbox"
)

// Runner is one local run: a case, two builds, two clusters, a results
// directory. The session (M3) will hold the same thing for every pair.
type Runner struct {
	SuiteDir  string
	Suite     *Suite
	Out       string
	SessionID string
	Started   time.Time
	Builds    [2]string // target, reference

	CPUSet, ClientCPUSet string
	ClientImage          string
	CSBBin               string
	Repeats              int // 0 means the case's
	Keep                 bool

	Log io.Writer
}

func (r *Runner) logf(format string, args ...any) {
	fmt.Fprintf(r.Log, "%s %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
}

func (r *Runner) repeats(c *Case) int {
	if r.Repeats > 0 {
		return r.Repeats
	}
	return c.Repeats
}

func (r *Runner) pinning() string {
	switch {
	case r.CPUSet != "" && r.ClientCPUSet != "":
		return PinningCPUSet
	case r.CPUSet != "" || r.ClientCPUSet != "":
		return PinningPartial
	}
	return PinningNone
}

const runUsage = `usage: testkit perf run <case-id> --suite <dir> --build <target tree> --build <reference tree>
                        [--repeats N] [--out <dir>] [--cpuset <list>] [--client-cpuset <list>]
                        [--client-image <image>] [--keep]

Runs one case on two builds, the way a session would, and prints the ratio
on the last line of standard output. The first --build is the target, the
second the reference. --out defaults to ./perf-run-<time>/. --cpuset pins the
database nodes and --client-cpuset the clients (both empty: no pinning).
--client-image is the image for the client node (default localhost/perf-client:dev);
--keep leaves the two clusters standing for a look. Needs csb on PATH or in
$TESTKIT_CSB, and a container runtime it can use.`

// run is the verb (Spec §7.1, FR-28).
func runCmd(args []string, stdout, stderr io.Writer) int {
	r := &Runner{Log: stderr, Started: time.Now(), ClientImage: "localhost/perf-client:dev"}
	var builds []string
	var caseID string
	for i := 0; i < len(args); i++ {
		a := args[i]
		next := func() (string, bool) {
			if i+1 < len(args) {
				i++
				return args[i], true
			}
			return "", false
		}
		var v string
		var ok bool
		switch a {
		case "--suite":
			v, ok = next()
			r.SuiteDir = v
		case "--build":
			v, ok = next()
			builds = append(builds, v)
		case "--repeats":
			v, ok = next()
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				fmt.Fprintf(stderr, "testkit perf run: --repeats wants a positive integer, got %q\n", v)
				return ExitRefused
			}
			r.Repeats = n
		case "--out":
			v, ok = next()
			r.Out = v
		case "--cpuset":
			v, ok = next()
			r.CPUSet = v
		case "--client-cpuset":
			v, ok = next()
			r.ClientCPUSet = v
		case "--client-image":
			v, ok = next()
			r.ClientImage = v
		case "--keep":
			r.Keep, ok = true, true
		case "-h", "--help":
			fmt.Fprintln(stdout, runUsage)
			return ExitOK
		default:
			if strings.HasPrefix(a, "-") || caseID != "" {
				fmt.Fprintf(stderr, "testkit perf run: %q is not a flag\n%s\n", a, runUsage)
				return ExitRefused
			}
			caseID, ok = a, true
		}
		if !ok {
			fmt.Fprintf(stderr, "testkit perf run: %s wants a value\n%s\n", a, runUsage)
			return ExitRefused
		}
	}
	if caseID == "" || r.SuiteDir == "" || len(builds) != 2 {
		fmt.Fprintln(stderr, runUsage)
		return ExitRefused
	}
	for _, kv := range []struct{ k, v string }{{"--cpuset", r.CPUSet}, {"--client-cpuset", r.ClientCPUSet}} {
		if kv.v != "" && !cpusetRe.MatchString(kv.v) {
			fmt.Fprintf(stderr, "testkit perf run: %s wants a CPU list like 0-7,16-23, got %q\n", kv.k, kv.v)
			return ExitRefused
		}
	}
	copy(r.Builds[:], builds)
	for i, b := range r.Builds {
		abs, err := filepath.Abs(b)
		if err == nil {
			r.Builds[i] = abs
		}
		if !exists(filepath.Join(r.Builds[i], "bin", "cub_server")) {
			fmt.Fprintf(stderr, "testkit perf run: %s is not a CUBRID install tree (no bin/cub_server)\n", b)
			return ExitRefused
		}
	}
	r.SuiteDir = absolute(r.SuiteDir)
	// The one case and its fixture, not the whole suite: a broken manifest
	// elsewhere is validate's business, and must not stop a local run.
	module, name, _ := strings.Cut(caseID, ".")
	if !caseIDRe.MatchString(caseID) || !exists(filepath.Join(r.SuiteDir, "cases", module, name, "case.json")) {
		fmt.Fprintf(stderr, "testkit perf run: %s is not a case in %s\n", caseID, r.SuiteDir)
		return ExitRefused
	}
	c, err := ReadCase(filepath.Join(r.SuiteDir, "cases", module, name))
	if err != nil {
		for _, line := range problemLines(err) {
			fmt.Fprintln(stderr, line)
		}
		return ExitRefused
	}
	f, err := ReadFixture(filepath.Join(r.SuiteDir, "fixtures", c.Fixture.Name))
	if err != nil {
		for _, line := range problemLines(err) {
			fmt.Fprintln(stderr, line)
		}
		return ExitRefused
	}
	r.Suite = &Suite{Root: r.SuiteDir, Cases: []*Case{c}, Fixtures: map[string]*Fixture{f.Name: f}}
	r.SessionID = "perf-run-" + r.Started.Format("20060102-150405")
	if r.Out == "" {
		r.Out = "perf-run-" + r.Started.Format("20060102-150405")
	}
	r.Out = absolute(r.Out)
	if err := os.MkdirAll(r.Out, 0o755); err != nil {
		fmt.Fprintf(stderr, "testkit perf run: %v\n", err)
		return ExitEnvironment
	}
	r.CSBBin = os.Getenv(sandbox.BinEnv)
	if err := (&sandbox.CLI{Bin: r.CSBBin}).Available(context.Background()); err != nil {
		fmt.Fprintf(stderr, "testkit perf run: %v\n", err)
		return ExitEnvironment
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	code, line := r.runOne(ctx, c, f)
	if line != "" {
		fmt.Fprintln(stdout, line)
	}
	return code
}

// runOne is the run: both sides up with the fixture, the case measured,
// judged, written, the sides down.
func (r *Runner) runOne(ctx context.Context, c *Case, f *Fixture) (int, string) {
	pair := "local"
	pairDir := filepath.Join(r.Out, pair)
	session := RunSession{
		Schema: "perf-regression-session/1", ID: r.SessionID, Kind: "run", Started: r.Started, State: "incomplete",
		Host: hostname(), Interleave: "case", CPUSet: CPUSetPair{Server: r.CPUSet, Client: r.ClientCPUSet},
		Pinning: r.pinning(), Pair: pair, Cases: []string{c.ID},
	}
	session.Target = BuildRef{Build: r.Builds[0], Fingerprint: readFingerprint(r.Builds[0])}
	session.Reference = BuildRef{Build: r.Builds[1], Fingerprint: readFingerprint(r.Builds[1])}
	session.Target.Commit, session.Reference.Commit = session.Target.Fingerprint.Commit, session.Reference.Fingerprint.Commit
	if !SameFingerprint(session.Target.Fingerprint, session.Reference.Fingerprint) {
		session.Notes = append(session.Notes, "fingerprint_changed: the two builds differ in compiler, build type, flags or a third-party library")
	}
	writeSession := func(state, reason string) {
		session.State, session.ExitReason, session.Ended = state, reason, time.Now()
		if err := writeJSON(filepath.Join(r.Out, "session.json"), &session); err != nil {
			r.logf("session.json: %v", err)
		}
	}
	writeSession("incomplete", "")
	r.logf("run %s: %s on %s (target) vs %s (reference), repeats %d, out %s", r.SessionID, c.ID, r.Builds[0], r.Builds[1], r.repeats(c), r.Out)

	var sides []*Side
	defer func() {
		// The nodes' logs go with the cluster; keep them beside the results
		// first, whatever happened.
		for _, s := range sides {
			r.keepLogs(s, pairDir)
		}
		if r.Keep {
			for _, s := range sides {
				r.logf("kept %s (--keep); csb cluster destroy --cluster %s --purge when done", s.Name, s.Name)
			}
			return
		}
		for _, s := range sides {
			if err := r.destroySide(s); err != nil {
				session.Notes = append(session.Notes, "teardown: "+err.Error())
				writeSession(session.State, session.ExitReason)
			}
		}
	}()
	cases := []*Case{c}
	for _, side := range []struct{ role, suffix, build string }{{"target", "t", r.Builds[0]}, {"reference", "r", r.Builds[1]}} {
		s, err := r.createSide(ctx, side.role, side.suffix, side.build, cases)
		if s != nil {
			sides = append(sides, s)
			session.Clusters = append(session.Clusters, s.Name)
		}
		if err != nil {
			r.logf("%v", err)
			writeSession("incomplete", "cluster")
			return ExitEnvironment, ""
		}
		if raw, err := s.CLI.DescribeRaw(ctx); err == nil {
			_ = os.MkdirAll(filepath.Join(r.Out, "clusters"), 0o755)
			_ = os.WriteFile(filepath.Join(r.Out, "clusters", s.Name+".describe.json"), append(raw, '\n'), 0o644)
		}
		if err := r.buildFixture(ctx, s, f); err != nil {
			r.logf("%v", err)
			writeSession("incomplete", "fixture")
			return ExitEnvironment, ""
		}
	}
	t, ref := sides[0], sides[1]
	cr := r.runCase(ctx, c, f, t, ref)
	for _, p := range cr.Passes {
		s := t
		if p.Side == "reference" {
			s = ref
		}
		r.keepPassFiles(s, c, p.Phase, p.Rep, pairDir)
	}
	v := judge(c, cr, r.repeats(c))
	entry := caseEntry(c, cr, v)
	sidecar := Sidecar{
		Schema: "perf-regression/1", Session: r.SessionID, Pair: pair,
		Target: session.Target, Reference: session.Reference, Machine: session.Host,
		CPUSet: session.CPUSet, Pinning: session.Pinning, Interleave: "case", SessionValid: true,
		Cases: []CaseEntry{entry},
	}
	if err := writeJSON(filepath.Join(pairDir, "regression-case.json"), &sidecar); err != nil {
		// The sidecar is the result; a run that could not write it has no
		// result, whatever it printed.
		r.logf("sidecar: %v", err)
		writeSession("incomplete", "write")
		return ExitEnvironment, ""
	}
	if err := writeCasesCSV(filepath.Join(pairDir, "cases.csv"), r.SessionID, pair, sidecar.Cases, map[string]Verdict{c.ID: v}); err != nil {
		r.logf("cases.csv: %v", err)
	}
	counters := countersFile{Case: c.ID, Version: c.Version, Roles: []string{"server", "broker", "cas", "client"}, Passes: cr.Passes, PerOp: v.Counters}
	if err := writeJSON(filepath.Join(pairDir, c.ID, "counters.json"), &counters); err != nil {
		r.logf("counters.json: %v", err)
	}
	reason := "done"
	if errors.Is(ctx.Err(), context.Canceled) {
		reason = "signal"
	}
	writeSession(map[bool]string{true: "complete", false: "incomplete"}[reason == "done"], reason)
	if reason == "signal" {
		return ExitEnvironment, lastLine(c, v, entry)
	}
	return ExitOK, lastLine(c, v, entry)
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}
