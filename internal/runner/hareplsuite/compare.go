package hareplsuite

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cubrid-systems/cubrid-testkit/internal/sandbox"
)

// compareRead runs one read on every node and returns the first node that
// answered differently, with both answers.
//
// sandbox.SameOnBothNodes answers the same question and returns only the node
// name, which is the right shape for a caller that is deciding something. It
// is the wrong shape for this one: a difference here is the suite's entire
// output, and a difference nobody can look at is a verdict nobody can act on.
// The first run of this suite reported a differing catalog read and the
// difference could not be examined at all, which is what this exists to fix.
func compareRead(ctx context.Context, p *sandbox.Pair, stmt string) (node, want, got string, err error) {
	read := fmt.Sprintf("csql -u dba -c %q %s 2>&1", stmt, p.DB)
	m, err := p.MasterChannel().Run(ctx, read)
	if err != nil {
		return "", "", "", err
	}
	for i, name := range p.Slaves {
		sc, serr := p.SlaveChannel(i)
		if serr != nil {
			return "", "", "", serr
		}
		s, rerr := sc.Run(ctx, read)
		if rerr != nil {
			return "", "", "", rerr
		}
		if Normalise(s.Stdout) != Normalise(m.Stdout) {
			return name, m.Stdout, s.Stdout, nil
		}
	}
	return "", "", "", nil
}

// Normalise drops what two nodes may legitimately print differently.
//
// The same three lines CTP's own `format_csql_output` drops, plus csql's
// connection notification, which carries a timestamp and a pid. Kept here
// rather than reaching into internal/sandbox because that copy is unexported
// and duplicating four rules is cheaper than exporting a normaliser two
// packages would then have to agree about (ADR-009's general one is still
// unwritten).
func Normalise(s string) string {
	var keep []string
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		switch {
		case t == "":
		case strings.HasPrefix(t, "Time:"):
		case strings.HasPrefix(t, "Program 'csql'"):
		case strings.Contains(t, " sec)"):
		case strings.HasPrefix(t, "Committed."):
		default:
			keep = append(keep, strings.TrimRight(line, " \t"))
		}
	}
	return strings.Join(keep, "\n")
}

// keepDifference writes both answers where a reader can diff them, and returns
// the directory it used.
func keepDifference(dir, caseName, stmt, master, slave, slaveNode string) (string, error) {
	safe := strings.NewReplacer("/", "~", " ", "_").Replace(strings.TrimSuffix(caseName, ".sql"))
	out := filepath.Join(dir, safe)
	if err := os.MkdirAll(out, 0o755); err != nil {
		return "", err
	}
	files := map[string]string{
		"statement.sql":    stmt + ";\n",
		"master.out":       master,
		slaveNode + ".out": slave,
		"normalised.diff": "--- master (normalised)\n" + Normalise(master) +
			"\n--- " + slaveNode + " (normalised)\n" + Normalise(slave) + "\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(out, name), []byte(body), 0o644); err != nil {
			return "", err
		}
	}
	return out, nil
}
