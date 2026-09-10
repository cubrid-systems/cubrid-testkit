package shellsuite

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// staleDatabases names the databases the registry already holds whose files are
// not in this run's scenario.
//
// $CUBRID_DATABASES/databases.txt is the registry every case's createdb writes
// into, and each slot gets an overlay of it so that two slots creating a
// database of the same name do not collide -- which matters more than it sounds:
// the corpus makes 4,056 createdb calls using 115 distinct names, and "testdb"
// alone is used by 74 different cases.
//
// The overlay isolates the writes. What it does not isolate is the lower layer,
// which is the directory as the machine left it, so every slot inherits whatever
// an earlier run registered and never removed. That is not merely untidy:
// make_tz -g extend iterates every entry in databases.txt and runs
// `cubrid gen_tz -g extend` against each, so one database left over from a run
// three weeks ago fails all 38 cases in this corpus that rebuild timezones.
// Observed exactly that -- an entry pointing into a scenario tree this run does
// not use, and every timezone case reporting "make_tz failed!!!".
//
// Reported rather than deleted: the registry is the machine's, a run does not
// own what it did not create, and an operator who is told can clear it in one
// line.
func staleDatabases(scenario string) []string {
	dir := os.Getenv("CUBRID_DATABASES")
	if dir == "" || scenario == "" {
		return nil
	}
	f, err := os.Open(filepath.Join(dir, "databases.txt"))
	if err != nil {
		return nil
	}
	defer f.Close()

	root, err := filepath.Abs(scenario)
	if err != nil {
		return nil
	}
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name, path := fields[0], fields[1]
		// Under the scenario is this run's; anywhere else belongs to a run that
		// is over. A relative path cannot be judged and is left alone.
		if !filepath.IsAbs(path) || strings.HasPrefix(path, root+string(filepath.Separator)) || path == root {
			continue
		}
		out = append(out, fmt.Sprintf("%s at %s", name, path))
	}
	return out
}

// PruneRegistryScript removes from this slot's registry every database whose
// files were under dir.
//
// The reclaim and the registry have to move together. A case creates a database
// in its own directory, createdb writes the name and that path into
// $CUBRID_DATABASES/databases.txt, and the case does not always delete it --
// which upstream can live with, because upstream leaves the files where they
// are. This runner reclaims the directory when its last case retires, so the
// entry outlives the database it names.
//
// Then the next case in this slot that walks the registry fails on it. Observed
// on a full-corpus run: db25452 registered at a path this runner had already
// reclaimed, in the same slot, with 38 timezone cases still to come -- and
// make_tz -g extend runs `cubrid gen_tz -g extend` against every name it finds.
//
// Written through the slot's own channel, because databases.txt is behind a
// per-slot overlay: this slot's copy is the only one that has the entry, and the
// only one that should lose it.
func PruneRegistryScript(dir string) string {
	// awk rather than sed: the path is data, and a case directory holds every
	// character sed would treat as syntax.
	return `f="${CUBRID_DATABASES:-}/databases.txt"; [ -n "${CUBRID_DATABASES:-}" ] && [ -f "$f" ] || exit 0
awk -v d=` + shellQuote(dir) + ` '
  /^[[:space:]]*#/ { print; next }
  NF < 2          { print; next }
  $2 == d || index($2, d "/") == 1 { next }
  { print }
' "$f" > "$f.tkprune" && mv "$f.tkprune" "$f"`
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
