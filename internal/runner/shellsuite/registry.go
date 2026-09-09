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
