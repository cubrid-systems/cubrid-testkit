package hareplsuite

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cubrid-systems/cubrid-testkit/internal/cli"
)

// Finding the runs that are going, so a watcher does not have to be told.
//
// # Why this is not a flag
//
// A sharded run is eight processes, and asking the operator to list eight conf
// paths to watch them is asking them to keep a record the machine already has.
// Each run was started with `-c <conf>`, and that argument is still in its
// command line for as long as it is running: the machine knows what is going
// on, and a watcher can read it.
//
// # What it deliberately does not do
//
// Guess. Only a process whose own argv says `ha_repl` and names a conf is
// returned, and only when that conf still exists to be read. A watcher that
// invented a run would draw a lane that never fills, which is worse than a
// watcher that draws nothing.

// Running returns the conf paths of the ha_repl runs on this machine, in a
// stable order and without duplicates.
//
// It reads /proc, so it sees this user's processes and the containers' -- which
// is right: a rootless sandbox node's processes are this user's too, and a run
// driving one is a run.
func Running() []string {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, e := range entries {
		pid := e.Name()
		if pid == "" || pid[0] < '0' || pid[0] > '9' {
			continue
		}
		argv, rerr := os.ReadFile(filepath.Join("/proc", pid, "cmdline"))
		if rerr != nil {
			continue
		}
		conf, ok := confOf(strings.Split(string(argv), "\x00"))
		if !ok || seen[conf] {
			continue
		}
		if _, serr := os.Stat(conf); serr != nil {
			continue
		}
		seen[conf] = true
		out = append(out, conf)
	}
	sort.Strings(out)
	return out
}

// confOf reads one process's argv and reports the conf an ha_repl run was given.
//
// The shape is `testkit ha_repl -c <path>`: the program's own name ends in
// `testkit`, one argument is the task, and the conf follows `-c` or `--config`
// (or is joined to it with `=`, which flag accepts and operators use).
func confOf(argv []string) (string, bool) {
	if len(argv) == 0 || !strings.HasSuffix(strings.TrimSuffix(argv[0], "\x00"), "testkit") {
		return "", false
	}
	task, conf := false, ""
	for i := 0; i < len(argv); i++ {
		a := strings.TrimSuffix(argv[i], "\x00")
		switch {
		case a == string(cli.HARepl):
			task = true
		case a == "-c" || a == "--config":
			if i+1 < len(argv) {
				conf = strings.TrimSuffix(argv[i+1], "\x00")
			}
		case strings.HasPrefix(a, "-c="):
			conf = strings.TrimPrefix(a, "-c=")
		case strings.HasPrefix(a, "--config="):
			conf = strings.TrimPrefix(a, "--config=")
		}
	}
	if !task || conf == "" {
		return "", false
	}
	abs, err := filepath.Abs(conf)
	if err != nil {
		return conf, true
	}
	return abs, true
}
