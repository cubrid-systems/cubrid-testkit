package perf

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/sandbox"
)

// clientRecord is the one JSON line a client prints last (Spec §7.3).
type clientRecord struct {
	Ops       float64             `json:"ops"`
	WarmOps   *float64            `json:"warm_ops"` // what the warm-up did; the server-side snapshots bracket it
	ElapsedNs float64             `json:"elapsed_ns"`
	P50Ns     *float64            `json:"p50_ns"`
	P99Ns     *float64            `json:"p99_ns"`
	Errors    float64             `json:"errors"`
	Self      map[string]*float64 `json:"self"` // a counter the client could not read is null (FR-16)
}

// passOut is where one pass's files are, in the node and on the host.
type passOut struct {
	InNode string
	Host   string
}

func (r *Runner) passOut(s *Side, c *Case, phase string, k int) passOut {
	prefix := "rep"
	if phase != "measure" {
		prefix = "warm"
	}
	rel := filepath.Join("out", c.ID, s.Role, fmt.Sprintf("%s%d", prefix, k))
	return passOut{InNode: workPerf + "/" + filepath.ToSlash(rel), Host: filepath.Join(s.Work, "perf", rel)}
}

// Exit codes coreutils' timeout gives a command it had to end: 124 after
// TERM, 137 when it came to KILL.
const (
	exitTimedOut = 124
	exitKilled   = 137
)

// runClient runs the case's program for one pass and returns what it
// reported. A pass that ran past its budget is killed from inside the node
// by the pid the client wrote, and comes back as null with the reason
// (Design §5.5.2). rc is the program's exit code; reason is empty when the
// record is usable.
func (r *Runner) runClient(ctx context.Context, s *Side, c *Case, f *Fixture, phase string, k int) (rec *clientRecord, rc int, reason string, err error) {
	out := r.passOut(s, c, phase, k)
	if err := os.MkdirAll(out.Host, 0o755); err != nil {
		return nil, -1, "", err
	}
	for _, stale := range []string{"client.out", "client.err", "rc", "pid"} {
		_ = os.Remove(filepath.Join(out.Host, stale))
	}
	env := fmt.Sprintf("PERF_PHASE=%s PERF_DB=%s PERF_HOST=%s-n1 PERF_PORT=33000 PERF_OUT=%s PERF_REPEAT=%d PERF_WARM_S=%d PERF_OPS=%d",
		phase, dbName(f), s.Name, out.InNode, k, c.WarmS, utilityOps(c))
	var node *sandbox.Node
	var cmd string
	switch c.Driver {
	case "jdbc":
		node = s.C1
		cmd = fmt.Sprintf(`%s; cd %s && env %s java -Xmx1g -cp %s/classes:$jar %s %s`,
			jdbcJar, workPerf, env, workPerf, c.JDBC.Main, shellJoin(c.JDBC.Args))
	case "utility":
		node = s.N1
		cmd = fmt.Sprintf(`cd %s && env %s bash %s/scripts/run_utility.sh %s`, workPerf, env, workPerf, shellJoin(c.Utility.Argv))
	case "cdc-api":
		node = s.C1
		cmd = fmt.Sprintf(`cd %s && env %s %s/bin/%s %s`, workPerf, env, workPerf, c.CDC.Bin, shellJoin(c.CDC.Args))
	default:
		return nil, -1, "", fmt.Errorf("driver %q", c.Driver)
	}
	// The budget is enforced where the process is: coreutils' timeout in
	// the node ends the client at budget_s (+ the warming it was given) and
	// says so in its exit code, and setsid puts the client in a group of its
	// own so a kill from outside reaches a utility's children too. The exec's
	// own bounds are longer, so they fire only when the node itself is gone.
	budget := time.Duration(c.BudgetS)*time.Second + time.Duration(c.WarmS)*time.Second
	script := fmt.Sprintf("setsid -w timeout -k 5 %d bash -c %s > %s/client.out 2> %s/client.err; rc=$?; echo $rc > %s/rc; exit $rc",
		int(budget.Seconds()), shellJoin([]string{cmd}), out.InNode, out.InNode, out.InNode)
	res, runErr := node.RunFor(ctx, script, budget+60*time.Second)
	if runErr != nil {
		r.killClient(node, out.InNode)
		if errors.Is(runErr, sandbox.ErrTimedOut) {
			return nil, -1, "budget", nil
		}
		return nil, -1, "", fmt.Errorf("%s: %w", node.Describe(), runErr)
	}
	rc = res.ExitCode
	switch {
	case rc == exitTimedOut || rc == exitKilled:
		return nil, rc, "budget", nil
	case rc < 0:
		// csb's own clock ended the exec; the client may still be there.
		r.killClient(node, out.InNode)
		return nil, rc, "budget", nil
	}
	b, err := os.ReadFile(filepath.Join(out.Host, "client.out"))
	if err != nil {
		return nil, rc, "no client.out", nil
	}
	rec, perr := parseClientRecord(b)
	switch {
	case rc != 0:
		return rec, rc, fmt.Sprintf("exit %d", rc), nil
	case perr != nil:
		return nil, rc, "no JSON record: " + perr.Error(), nil
	case rec.Errors > 0:
		return rec, rc, fmt.Sprintf("errors=%d", int(rec.Errors)), nil
	case rec.Ops <= 0 || rec.ElapsedNs <= 0:
		return rec, rc, "ops or elapsed_ns is zero", nil
	}
	return rec, rc, "", nil
}

// killClient ends a client that outlived its budget, from inside the node:
// the exec that started it is gone, and the process is not (Design §5.5.2).
// The whole process group goes, by the pid the client wrote: a utility's
// children are in it, and so is the client itself.
func (r *Runner) killClient(node *sandbox.Node, outInNode string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	script := fmt.Sprintf(`p=$(cat %s/pid 2>/dev/null); if [ -n "$p" ]; then g=$(ps -o pgid= -p "$p" 2>/dev/null | tr -d ' '); `+
		`kill -TERM -- -"${g:-$p}" 2>/dev/null; sleep 2; kill -KILL -- -"${g:-$p}" 2>/dev/null; fi; true`, outInNode)
	if _, err := node.RunFor(ctx, script, 30*time.Second); err != nil {
		r.logf("%s: killing the client: %v", node.Describe(), err)
	}
}

// parseClientRecord takes the last non-empty line of the client's stdout.
func parseClientRecord(out []byte) (*clientRecord, error) {
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "{") {
			return nil, fmt.Errorf("last line is not JSON: %s", tail(line, 120))
		}
		var rec clientRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, err
		}
		return &rec, nil
	}
	return nil, errors.New("empty output")
}

func utilityOps(c *Case) int {
	if c.Utility != nil {
		return c.Utility.Ops
	}
	return 1
}

// shellJoin quotes arguments for the node's shell.
func shellJoin(args []string) string {
	q := make([]string, len(args))
	for i, a := range args {
		q[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return strings.Join(q, " ")
}
