package shellsuite

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
)

// A plug-in exactly as the contract describes it: four functions, and execute
// reports through IS_SUCC rather than through an exit code.
const samplePlugin = `
init() {
    echo "initialising"
    export PREPARED=yes
}

list() {
    echo alpha
    echo beta
    echo gamma
}

execute() {
    local testcase=$1
    echo "running $testcase (prepared=$PREPARED)"
    if [ "$testcase" = "beta" ]; then
        IS_SUCC=false
    else
        IS_SUCC=true
    fi
}

finish() {
    echo "cleaning up"
}
`

func newPlugin(t *testing.T, body string) *plugin {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, "shell", "local")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mytype.sh"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return &plugin{
		ch:       exec.NewLocal(home, "CTP_HOME="+home),
		testType: "mytype",
	}
}

func TestPluginRunsTheFourFunctions(t *testing.T) {
	p := newPlugin(t, samplePlugin)
	ctx := context.Background()

	out, err := p.Init(ctx)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if !strings.Contains(out, "initialising") {
		t.Errorf("init output: %q", out)
	}

	cases, err := p.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(cases) != 3 || cases[0] != "alpha" || cases[2] != "gamma" {
		t.Errorf("list: %v", cases)
	}

	if _, err := p.Finish(ctx); err != nil {
		t.Fatalf("finish: %v", err)
	}
}

func TestVerdictComesFromIsSuccNotTheExitCode(t *testing.T) {
	// This is the part of the contract that surprises people. The plug-in sets a
	// variable; it does not exit non-zero. A case that "fails" leaves the shell
	// perfectly happy.
	p := newPlugin(t, samplePlugin)
	ctx := context.Background()

	if _, ok, err := p.Execute(ctx, "alpha"); err != nil || !ok {
		t.Errorf("alpha: ok=%v err=%v, want a pass", ok, err)
	}
	if _, ok, err := p.Execute(ctx, "beta"); err != nil || ok {
		t.Errorf("beta: ok=%v err=%v, want a failure", ok, err)
	}
}

func TestExecuteSeesWhatInitExported(t *testing.T) {
	// init and execute run in the same shell, which is why the plug-in may be four
	// functions rather than four scripts.
	p := newPlugin(t, samplePlugin)
	out, _, err := p.Execute(context.Background(), "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "prepared=") {
		t.Fatalf("output: %q", out)
	}
}

func TestMarkersAreStrippedFromOutput(t *testing.T) {
	// Everything from GPROPSTART on is protocol, not the plug-in's output. A
	// caller that logged it would be logging the mechanism.
	p := newPlugin(t, samplePlugin)
	out, _, err := p.Execute(context.Background(), "alpha")
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{propStart, propPrefix, propEnd} {
		if strings.Contains(out, marker) {
			t.Errorf("output still carries %q: %q", marker, out)
		}
	}
}

func TestAnUnsetVerdictIsAFailure(t *testing.T) {
	// A plug-in that forgets to set IS_SUCC has not passed. Reading silence as
	// success is how a broken plug-in reports a green run.
	p := newPlugin(t, `
init() { :; }
list() { echo only; }
execute() { echo "did something"; }
finish() { :; }
`)
	if _, ok, err := p.Execute(context.Background(), "only"); err != nil || ok {
		t.Errorf("ok=%v err=%v, want a failure", ok, err)
	}
}

func TestShippedPluginSyntaxWorks(t *testing.T) {
	// shell/local/unittest.sh -- the one plug-in CTP ships -- declares its
	// functions as `function name { }`. That is bash and ksh syntax; dash rejects
	// it outright. This test is here so that a later change from bash back to sh
	// fails loudly rather than only on machines where /bin/sh is dash.
	p := newPlugin(t, `
function init {
	return
}
function list {
	echo one
}
function execute {
	echo "ran $1"
	IS_SUCC=true
}
function finish {
	return
}
`)
	ctx := context.Background()
	if _, err := p.Init(ctx); err != nil {
		t.Fatalf("init: %v", err)
	}
	cases, err := p.List(ctx)
	if err != nil || len(cases) != 1 || cases[0] != "one" {
		t.Fatalf("list: %v %v", cases, err)
	}
	out, ok, err := p.Execute(ctx, "one")
	if err != nil || !ok {
		t.Fatalf("execute: ok=%v err=%v out=%q", ok, err, out)
	}
}
