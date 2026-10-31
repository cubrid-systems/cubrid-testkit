package haapply

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/sandbox"
)

// fakeCSBEnv makes the test binary a stand-in for csb: it answers `cluster
// describe` with one pair, and runs every `node exec` script here with sh, the
// directory the variable names first on PATH. Every node is this host.
const fakeCSBEnv = "HAAPPLY_FAKE_CSB"

func TestMain(m *testing.M) {
	if dir := os.Getenv(fakeCSBEnv); dir != "" {
		os.Exit(fakeCSB(os.Args[1:], dir))
	}
	os.Exit(m.Run())
}

func fakeCSB(args []string, dir string) int {
	reply := func(ok bool, data any) {
		b, _ := json.Marshal(map[string]any{"schema": sandbox.Schema, "ok": ok, "data": data})
		fmt.Println(string(b))
	}
	switch {
	case len(args) >= 2 && args[0] == "cluster" && args[1] == "describe":
		if os.Getenv("FAKE_DESCRIBE_FAIL") != "" {
			reply(false, map[string]any{})
			return 1
		}
		reply(true, map[string]string{"format": "ctp",
			"conf": "env.instance1.master.ssh.host=m1\nenv.instance1.slave.ssh.host=s1\nenv.instance1.ha.ha_db_list=db\n"})
	case len(args) >= 2 && args[0] == "node" && args[1] == "exec":
		node, script := "", ""
		for i, a := range args {
			if a == "--json" && i+1 < len(args) {
				node = args[i+1]
			}
			if a == "--" && i+1 < len(args) {
				script = args[i+1]
				break
			}
		}
		cmd := exec.Command("sh", "-c", script)
		cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"))
		var out, errOut bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errOut
		code := 0
		if err := cmd.Run(); err != nil {
			var ee *exec.ExitError
			if !errors.As(err, &ee) {
				return 1
			}
			code = ee.ExitCode()
		}
		reply(true, map[string]any{node: map[string]any{"exit": code, "stdout": out.String(), "stderr": errOut.String()}})
	default:
		reply(true, map[string]any{})
	}
	return 0
}

// pairCSQL is csql for a pair on one host. A write goes to $STATE/txlog
// ($CRASH in it: the csql dies without a word); the slave's reads answer from
// $STATE: count (ERR: no connection), look (absent: no witness yet), and fail,
// which the first fail_counter read does not see.
const pairCSQL = `#!/bin/sh
sql=""; tx=0
for a in "$@"; do
  case "$a" in --no-auto-commit) tx=1 ;; *SELECT*|*INSERT*|*DELETE*|*UPDATE*|*CREATE*|*DROP*|*SHOW*) sql="$a" ;; esac
done
if [ $tx = 1 ]; then
  echo "$sql" >> "$STATE/txlog"
  if [ -n "$CRASH" ]; then case "$sql" in *"$CRASH"*) exit 137 ;; esac; fi
  echo "1 row affected."; exit 0
fi
case "$sql" in
  "SELECT COUNT(*) FROM ballast_"*)
    c=$(cat "$STATE/count")
    [ "$c" = ERR ] && { echo "ERROR: Failed to connect to database server" >&2; exit 1; }
    printf '\n     %s\n' "$c" ;;
  "SELECT (SELECT"*) [ -f "$STATE/look" ] && printf '\n     %s\n' "$(cat "$STATE/look")" ;;
  "SELECT fail_counter"*)
    if [ -f "$STATE/failread" ]; then v=$(cat "$STATE/fail"); else v=0; touch "$STATE/failread"; fi
    printf '\n     %s\n' "$v" ;;
  "SELECT i FROM tkrepl_"*) echo "1 row selected." ;;
  "SELECT * FROM "*) printf '=== <Result of SELECT Command in Line 1> ===\n\n  1\n\n1 row selected.\n' ;;
esac
exit 0
`

// fakePair is a Runner on a pair of fakes, and the directory that holds the
// slave's answers: the count of a round's ballast starts at count.
func fakePair(t *testing.T, count string) (*Runner, string, *bytes.Buffer) {
	t.Helper()
	bin, state := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "csql"), []byte(pairCSQL), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(fakeCSBEnv, bin)
	t.Setenv("STATE", state)
	t.Setenv("CRASH", "")
	setState(t, state, "count", count)
	setState(t, state, "fail", "0")
	cli := &sandbox.CLI{Bin: os.Args[0], Cluster: "fake"}
	pair, err := sandbox.Describe(context.Background(), cli, "")
	if err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	r := &Runner{Pair: pair, Client: sandbox.NewNode(cli, "c1"), Rounds: 3, Rows: 100, Log: &log,
		WitnessWait: time.Second, StallFor: 3 * time.Second, NoReadFor: 2 * time.Second}
	return r, state, &log
}

func setState(t *testing.T, state, name, v string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(state, name), []byte(v), 0o644); err != nil {
		t.Fatal(err)
	}
}

// later writes a state file after d, as the slave moving on would.
func later(d time.Duration, state, name, v string) {
	go func() {
		time.Sleep(d)
		os.WriteFile(filepath.Join(state, name), []byte(v), 0o644)
	}()
}
