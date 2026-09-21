package sandbox

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// WaitForReplication blocks until a row written on the master is readable on
// every slave, and returns how long that took.
//
// This is the wait that P1 is about, and it is deliberately the same shape as
// the two implementations that already exist rather than a third idea:
//
//   - CTP's own `wait_for_slave` (make_ha_upper.sh:74) creates a table on the
//     master, inserts 'replication finished', and polls the slave with
//     -tillcontains until it arrives.
//   - `ha_repl`'s Test.java writes a flag, polls the slave for GOOD-<id>, backs
//     off, and gives up at ha_sync_detect_timeout_in_ms.
//
// Both were arrived at independently and they agree, which is the argument for
// this being the right shape. What neither of them is, and what 244 of the 373
// HA cases use instead, is a sleep: measured on four cases, the wait costs about
// 1.3 seconds where the sleep it replaced cost 5 to 30, and no verdict changed
// (evidence/ha/p1-sleep-to-wait.md).
//
// The marker is a table of its own, created and dropped per call, so a case's
// own tables are untouched. It is visible to a query that lists the catalog
// while the wait is in flight, which is the one thing a caller has to know.
func (p *Pair) WaitForReplication(ctx context.Context, timeout time.Duration) (time.Duration, error) {
	if p.DB == "" {
		return 0, fmt.Errorf("sandbox: cluster %q named no database, so there is nothing to replicate", p.Cluster)
	}
	// The marker carries the attempt, so a wait cannot be satisfied by the
	// leftovers of the one before it -- which is the failure a fixed table name
	// invites, and the reason ha_repl numbers its flag.
	marker := fmt.Sprintf("tkrepl_%d", time.Now().UnixNano())
	master := p.MasterChannel()

	create := fmt.Sprintf(
		"csql -u dba -c \"CREATE TABLE %s(i INT PRIMARY KEY); INSERT INTO %s VALUES(1);\" %s",
		marker, marker, p.DB)
	if res, err := master.Run(ctx, create); err != nil || res.ExitCode != 0 {
		return 0, fmt.Errorf("sandbox: the master would not take the marker: %v: %s", err, tail(res.Stdout+res.Stderr))
	}
	// Dropped whatever happens below: a marker left behind is a table the next
	// case meets, and this package does not get to leave those.
	defer master.Run(context.Background(), fmt.Sprintf("csql -u dba -c \"DROP TABLE %s;\" %s", marker, p.DB))

	start := time.Now()
	deadline := start.Add(timeout)
	// A backoff rather than a tight loop: each probe is a csql connection, and
	// ha_repl grew the same thing (100 + index*10 ms) for the same reason.
	wait := 50 * time.Millisecond
	for i := range len(p.Slaves) {
		slave, err := p.SlaveChannel(i)
		if err != nil {
			return time.Since(start), err
		}
		read := fmt.Sprintf("csql -u dba -c \"SELECT i FROM %s;\" %s 2>&1", marker, p.DB)
		for {
			res, rerr := slave.Run(ctx, read)
			if rerr != nil {
				return time.Since(start), rerr
			}
			// One row, holding 1. A table that exists and is empty is replication
			// half-arrived, and counts as not arrived.
			if strings.Contains(res.Stdout, "1 rows selected") || strings.Contains(res.Stdout, "1 row selected") {
				break
			}
			if time.Now().After(deadline) {
				return time.Since(start), fmt.Errorf(
					"sandbox: %s did not see the master's marker within %s; last read:\n%s",
					p.Slaves[i], timeout, tail(res.Stdout))
			}
			select {
			case <-ctx.Done():
				return time.Since(start), ctx.Err()
			case <-time.After(wait):
			}
			if wait < time.Second {
				wait += 50 * time.Millisecond
			}
		}
	}
	return time.Since(start), nil
}

// SameOnBothNodes runs one query on the master and on every slave and reports
// the first node whose answer differs, or "" when they all agree.
//
// The oracle the HA shell corpus and ha_repl both arrived at independently: the
// pair disagreeing with itself. Neither needs an answer file to know it failed,
// which is the single best property either of them has
// (design/module-ha.md §2-2).
//
// It does not wait. A difference under write traffic is replication in flight
// rather than divergence, and the two cannot be told apart from here -- so the
// caller waits first, with WaitForReplication, and this reports what it finds.
// csb's own `repl diff` refuses to conflate them for the same reason, and is
// weaker than this besides: it compares row counts, and says so.
func (p *Pair) SameOnBothNodes(ctx context.Context, query string) (string, error) {
	read := fmt.Sprintf("csql -u dba -c %q %s 2>&1", query, p.DB)
	want, err := p.MasterChannel().Run(ctx, read)
	if err != nil {
		return "", err
	}
	for i, name := range p.Slaves {
		slave, serr := p.SlaveChannel(i)
		if serr != nil {
			return "", serr
		}
		got, rerr := slave.Run(ctx, read)
		if rerr != nil {
			return "", rerr
		}
		if normalise(got.Stdout) != normalise(want.Stdout) {
			return name, nil
		}
	}
	return "", nil
}

// normalise drops what two nodes may legitimately print differently.
//
// The first three are CTP's own `format_csql_output` (init.sh:1079), which the
// HA cases call before every comparison they make: the execution-time line, any
// line carrying a "(... sec)" timing, and "Committed.". Mirrored rather than
// invented, because a comparison that masks differently from the corpus is
// answering a different question than the corpus asks.
//
// The last two are not in that helper and are needed here: csql on these nodes
// opens with a connection notification carrying a timestamp and an EID, and a
// line naming its own pid and the host it reached. All three vary per node and
// per invocation and none of them is a row.
//
//	Time: 09/21/26 07:28:00.096 - NOTIFICATION *** file .../boot_cl.c, line 875 ...
//	Program 'csql' (pid 462) connected to database server 'tkha2' on the host 'localhost' (port 31523).
//
// Deliberately minimal. A general normaliser is ADR-009's, still unwritten, and
// guessing at its shape here would put a second one in the tree.
func normalise(s string) string {
	var keep []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimRight(line, " \t")
		t := strings.TrimSpace(line)
		switch {
		case strings.Contains(line, "SQL statement execution time"),
			secTiming.MatchString(line),
			strings.Contains(line, "Committed."),
			strings.HasPrefix(t, "Time:") && strings.Contains(line, "NOTIFICATION"),
			strings.HasPrefix(t, "Program 'csql'") && strings.Contains(line, "connected to"):
			continue
		}
		keep = append(keep, line)
	}
	return strings.TrimSpace(strings.Join(keep, "\n"))
}

// secTiming is format_csql_output's `/(.* sec)/d`.
var secTiming = regexp.MustCompile(`\(.* sec\)`)

// tail is the last few lines, for an error that has to carry evidence without
// carrying a screen of it.
func tail(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > 6 {
		lines = lines[len(lines)-6:]
	}
	return strings.Join(lines, "\n")
}
