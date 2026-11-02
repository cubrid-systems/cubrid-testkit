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
// which the first fail_counter read does not see. A client's script (-i)
// prints $STATE/clientlog, or waits for good under $HANG. The stress tables'
// counts answer per node from heap.<table>.<host>, pk.<table>.<host> and
// orphans.<host>, 0 when absent, and whether 'k' and 'k ' compare equal from
// trailing.<host>; the table $DIFFER names reads differently on each node,
// under $STALL the slave never sees a marker, and under $NOMARKER the master
// refuses to make one.
const pairCSQL = `#!/bin/sh
sql=""; tx=0; script=""; host=""
for a in "$@"; do
  case "$a" in --no-auto-commit) tx=1 ;; *SELECT*|*INSERT*|*DELETE*|*UPDATE*|*CREATE*|*DROP*|*SHOW*) sql="$a" ;; *@*) host=${a#*@} ;; esac
done
prev=""
for a in "$@"; do [ "$prev" = "-i" ] && script=$a; prev=$a; done
if [ -n "$script" ]; then
  [ -n "$HANG" ] && exec sleep 300
  [ -f "$STATE/clientlog" ] && cat "$STATE/clientlog"
  exit 0
fi
if [ $tx = 1 ]; then
  echo "$sql" >> "$STATE/txlog"
  if [ -n "$CRASH" ]; then case "$sql" in *"$CRASH"*) exit 137 ;; esac; fi
  echo "1 row affected."; exit 0
fi
per() { [ -f "$STATE/$1" ] && cat "$STATE/$1" || echo 0; }
case "$sql" in
  "SELECT CAST('k' AS VARCHAR(4))"*) printf '\n     %s\n' "$(per trailing.$host)"; exit 0 ;;
  "SELECT COUNT(*) FROM s_child c WHERE NOT EXISTS"*) printf '\n     %s\n' "$(per orphans.$host)"; exit 0 ;;
  "SELECT COUNT(*) FROM s_"*"NVL"*) t=$(echo "$sql" | awk '{ print $4 }'); printf '\n     %s\n' "$(per heap.$t.$host)"; exit 0 ;;
  "SELECT id FROM s_"*|"SELECT p FROM s_"*|"SELECT c FROM s_"*)
    t=$(echo "$sql" | awk '{ print $4 }'); n=$(per pk.$t.$host); i=1
    while [ $i -le $n ]; do printf '     %s\n' $i; i=$((i + 1)); done; exit 0 ;;
esac
case "$sql" in
  "SELECT COUNT(*) FROM ballast_"*)
    c=$(cat "$STATE/count")
    [ "$c" = ERR ] && { echo "ERROR: Failed to connect to database server" >&2; exit 1; }
    printf '\n     %s\n' "$c" ;;
  "SELECT (SELECT"*) [ -f "$STATE/look" ] && printf '\n     %s\n' "$(cat "$STATE/look")" ;;
  "SELECT fail_counter"*)
    if [ -f "$STATE/failread" ]; then v=$(cat "$STATE/fail"); else v=0; touch "$STATE/failread"; fi
    printf '\n     %s\n' "$v" ;;
  "SELECT i FROM tkrepl_"*) [ -z "$STALL" ] && echo "1 row selected." ;;
  "CREATE TABLE tkrepl_"*) [ -n "$NOMARKER" ] && { echo "ERROR: A server would not take a write."; exit 1; } ;;
  "SELECT * FROM "*|"SELECT c, COUNT(*)"*)
    printf '=== <Result of SELECT Command in Line 1> ===\n\n  1\n\n1 row selected.\n'
    if [ -n "$DIFFER" ]; then case "$sql" in *" $DIFFER "*) echo "$host" ;; esac; fi ;;
esac
exit 0
`

// fakePair is a Runner on a pair of fakes, and the directory that holds the
// slave's answers: the count of a round's ballast starts at count.
func fakePair(t *testing.T, count string) (*Runner, string, *bytes.Buffer) {
	t.Helper()
	state := fakeEnv(t, count)
	cli := &sandbox.CLI{Cluster: "fake"}
	pair, err := sandbox.Describe(context.Background(), cli, "")
	if err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	r := &Runner{Pair: pair, Client: sandbox.NewNode(cli, "c1"), Rounds: 3, Rows: 100, Log: &log,
		WitnessWait: time.Second, StallFor: 3 * time.Second, NoReadFor: 2 * time.Second, BarrierFor: 2 * time.Second}
	return r, state, &log
}

// fakeEnv makes this test's csb the test binary and its csql the pair fake,
// and returns the fake's state directory.
func fakeEnv(t *testing.T, count string) string {
	t.Helper()
	bin, state := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "csql"), []byte(pairCSQL), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(fakeCSBEnv, bin)
	t.Setenv("STATE", state)
	t.Setenv("CRASH", "")
	t.Setenv(sandbox.BinEnv, os.Args[0])
	setState(t, state, "count", count)
	setState(t, state, "fail", "0")
	return state
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
