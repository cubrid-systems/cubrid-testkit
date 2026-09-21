package hareplsuite

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
)

// A statement cost about a second when each one was its own call, and a case
// is a few dozen statements. Measured on this pair:
//
//	podman exec true                        0.16 s
//	csb node exec true                      0.57 s
//	csb node exec + csql, one statement     1.05 s
//	csb node exec + csql, ten statements    1.57 s   (0.16 s each)
//
// So the cost is the call and the csql it starts, not the statement -- and ten
// statements in one call is 6.7x. Batching is therefore not an optimisation
// applied to a working design; it is the design, and the per-statement version
// was the prototype that proved the oracle.
//
// What makes it safe is two things csql does. It **continues past an error**,
// so a batch behaves as a sequence of calls did rather than stopping at the
// first refusal. And it labels every result with the line the statement
// started on -- `=== <Result of SELECT Command in Line 3> ===` -- so a batch's
// output can be taken apart again and each answer attributed to the statement
// that produced it.

var resultMarker = regexp.MustCompile(`^=== <Result of [^>]*Command in Line (\d+)> ===`)

// batch is a group of statements sent in one call, and where each began.
type batch struct {
	script string
	// line is the 1-based line each statement starts on inside the script,
	// which is what csql's result marker reports.
	line []int
}

// newBatch lays statements out one after another, recording where each begins.
func newBatch(stmts []string) batch {
	var b strings.Builder
	lines := make([]int, len(stmts))
	at := 1
	for i, s := range stmts {
		lines[i] = at
		b.WriteString(s)
		b.WriteString(";\n")
		at += strings.Count(s, "\n") + 1
	}
	return batch{script: b.String(), line: lines}
}

// run sends the batch to a node and returns each statement's result block,
// indexed the same way the statements were given.
//
// A statement with no block produced no result: it was not a query, or the
// engine refused it. Both are the caller's to interpret, and neither is an
// error here.
func (b batch) run(ctx context.Context, ch exec.Channel, db string) ([]string, int, error) {
	if b.script == "" {
		return nil, 0, nil
	}
	script := fmt.Sprintf("csql -u dba %s <<'__TESTKIT_SQL__'\n%s__TESTKIT_SQL__", db, b.script)
	res, err := ch.Run(ctx, script)
	if err != nil {
		return nil, 0, err
	}
	byLine := splitResults(res.Stdout)
	out := make([]string, len(b.line))
	for i, ln := range b.line {
		out[i] = byLine[ln]
	}
	return out, res.ExitCode, nil
}

// runRaw sends the batch and returns the node's whole output, for a caller
// that is not attributing results to statements -- a run of writes, whose
// only question is whether anything was refused.
func (b batch) runRaw(ctx context.Context, ch exec.Channel, db string) (string, int, error) {
	if b.script == "" {
		return "", 0, nil
	}
	script := fmt.Sprintf("csql -u dba %s <<'__TESTKIT_SQL__'\n%s__TESTKIT_SQL__", db, b.script)
	res, err := ch.Run(ctx, script)
	if err != nil {
		return "", 0, err
	}
	return res.Stdout + res.Stderr, res.ExitCode, nil
}

// splitResults takes csql's output apart at its result markers.
func splitResults(out string) map[int]string {
	blocks := map[int]string{}
	cur, curLine := []string{}, -1
	flush := func() {
		if curLine > 0 {
			blocks[curLine] = strings.Join(cur, "\n")
		}
		cur, curLine = nil, -1
	}
	for _, line := range strings.Split(out, "\n") {
		if m := resultMarker.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			flush()
			n, _ := strconv.Atoi(m[1])
			curLine = n
			continue
		}
		if curLine > 0 {
			cur = append(cur, line)
		}
	}
	flush()
	return blocks
}

// segment is a run of statements of one kind: all writes, or all reads.
//
// The boundary is where the oracle has to act. Everything up to a read can go
// in one call; the wait sits between the writes and the reads that follow
// them, and nowhere else.
type segment struct {
	read  bool
	stmts []string
	// idx maps back to the case's own statement numbering, for reporting.
	idx []int
}

// segments groups a case's statements into alternating runs.
func segments(stmts []string) []segment {
	var out []segment
	for i, s := range stmts {
		if isDirective(s) {
			continue
		}
		r := IsRead(s)
		if n := len(out); n > 0 && out[n-1].read == r {
			out[n-1].stmts = append(out[n-1].stmts, s)
			out[n-1].idx = append(out[n-1].idx, i)
			continue
		}
		out = append(out, segment{read: r, stmts: []string{s}, idx: []int{i}})
	}
	return out
}
