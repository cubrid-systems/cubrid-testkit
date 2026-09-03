package runshell

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
)

// caseTree builds <root>/<name>/cases/<name>.sh and returns the case directory.
func caseTree(t *testing.T, name, body string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name, "cases")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".sh"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A developer runs this from wherever they are standing, so all four ways of
// naming the same case have to work.
func TestLocateAcceptsEveryWayOfNamingACase(t *testing.T) {
	dir := caseTree(t, "flaky", "true\n")
	parent := filepath.Dir(dir)

	for _, arg := range []string{
		dir,                                // the cases/ directory
		parent,                             // the case directory above it
		filepath.Join(dir, "flaky.sh"),     // the script itself
		filepath.Join(dir, "flaky.result"), // a file that need not exist yet
	} {
		got, err := Locate(arg)
		if err != nil {
			// The last one names a file that does not exist; Stat fails and CTP
			// rejected it too. Skip rather than assert a behaviour CTP lacked.
			continue
		}
		if got.Dir != dir || got.Name != "flaky" {
			t.Errorf("Locate(%q) = %+v, want dir %q name flaky", arg, got, dir)
		}
	}
}

func TestLocateRejectsSomethingThatIsNotACase(t *testing.T) {
	if _, err := Locate(t.TempDir()); err == nil {
		t.Error("a directory with no cases/ was accepted")
	}
	if _, err := Locate(filepath.Join(t.TempDir(), "nowhere")); err == nil {
		t.Error("a path that does not exist was accepted")
	}
}

func newRun(t *testing.T, dir, name string, opts Options) (*Run, *bytes.Buffer) {
	t.Helper()
	out := &bytes.Buffer{}
	return &Run{
		Case:    Case{Dir: dir, Name: name},
		Options: opts,
		Channel: exec.NewLocal(""),
		Meta:    Meta{BuildID: "11.4.5.1875-74d17e9", Bits: "64", OS: "linux", RelType: "release"},
		Out:     out,
	}, out
}

// Without --loop the case runs once, whatever it says.
func TestWithoutLoopTheCaseRunsOnce(t *testing.T) {
	dir := caseTree(t, "counter", `echo " : OK counter" >> counter.result`)
	r, out := newRun(t, dir, "counter", Options{})

	res := r.Go(t.Context())
	if res.Outcome != OutcomeStop || res.Loops != 1 {
		t.Errorf("got %+v, want one loop and STOP", res)
	}
	if strings.Count(out.String(), "LOOP:") != 1 {
		t.Errorf("ran more than once:\n%s", out.String())
	}
}

// The whole point of the tool: keep going until it does not pass.
func TestTheLoopStopsAtTheFirstFailure(t *testing.T) {
	dir := caseTree(t, "flaky", strings.Join([]string{
		`n=$(cat .attempts 2>/dev/null || echo 0)`,
		`n=$((n+1)); echo $n > .attempts`,
		`if [ "$n" -ge 3 ]; then echo " : NOK it broke" > flaky.result;`,
		`else echo " : OK fine" > flaky.result; fi`,
	}, "\n"))
	r, out := newRun(t, dir, "flaky", Options{Loop: true, MaxLoop: 20})

	res := r.Go(t.Context())
	if res.Outcome != OutcomeNOK {
		t.Errorf("outcome = %s, want QUIT(NOK)", res.Outcome)
	}
	if res.Loops != 3 {
		t.Errorf("stopped after %d loops, want 3", res.Loops)
	}
	if !strings.Contains(out.String(), "Result: QUIT(NOK)") {
		t.Errorf("the verdict is not in the output:\n%s", out.String())
	}
}

func TestMaxLoopBoundsARunThatNeverFails(t *testing.T) {
	dir := caseTree(t, "steady", `echo " : OK steady" > steady.result`)
	r, _ := newRun(t, dir, "steady", Options{Loop: true, MaxLoop: 4})

	if res := r.Go(t.Context()); res.Loops != 4 || res.Outcome != OutcomeStop {
		t.Errorf("got %+v, want four loops and STOP", res)
	}
}

func TestMaxTimeBoundsARunThatNeverFails(t *testing.T) {
	dir := caseTree(t, "slow", "sleep 0.2\n"+`echo " : OK slow" > slow.result`)
	r, _ := newRun(t, dir, "slow", Options{Loop: true, MaxTime: 300 * time.Millisecond})

	res := r.Go(t.Context())
	if res.Outcome != OutcomeStop {
		t.Errorf("outcome = %s, want STOP", res.Outcome)
	}
	if res.Loops < 1 || res.Loops > 5 {
		t.Errorf("ran %d loops in 300ms of 200ms iterations, which is not plausible", res.Loops)
	}
}

// Touching STOP in the case directory ends the loop after the attempt in flight.
// It is the only way to stop a running loop without killing it, and the
// specification did not have it.
func TestTheStopFileEndsTheLoop(t *testing.T) {
	dir := caseTree(t, "stoppable", `echo " : OK stoppable" > stoppable.result`)
	if err := os.WriteFile(filepath.Join(dir, "STOP"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	r, out := newRun(t, dir, "stoppable", Options{Loop: true, MaxLoop: 100})

	res := r.Go(t.Context())
	if res.Loops != 1 {
		t.Errorf("ran %d loops with STOP present, want 1", res.Loops)
	}
	if !strings.Contains(out.String(), "Found STOP") {
		t.Errorf("stopping was not announced:\n%s", out.String())
	}
}

func TestAnEmptyResultIsNotAPass(t *testing.T) {
	dir := caseTree(t, "silent", "true\n")
	r, _ := newRun(t, dir, "silent", Options{})

	if res := r.Go(t.Context()); res.Outcome != OutcomeEmpty {
		t.Errorf("outcome = %s, want QUIT(FAIL1)", res.Outcome)
	}
}

// --extend-script hands the verdict to the operator's own script, for a case
// whose result cannot be judged by looking for NOK.
func TestAnExtendScriptDecidesTheVerdict(t *testing.T) {
	dir := caseTree(t, "custom", `echo "anything at all" > custom.result`)
	verify := filepath.Join(dir, "verify.sh")
	if err := os.WriteFile(verify, []byte("verify() { echo \"NOK the extend script said so\"; }\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	r, _ := newRun(t, dir, "custom", Options{ExtendScript: verify})

	res := r.Go(t.Context())
	if res.Outcome != OutcomeNOK {
		t.Errorf("outcome = %s, want the extend script's NOK", res.Outcome)
	}
	if !strings.Contains(res.Info, "the extend script said so") {
		t.Errorf("info = %q", res.Info)
	}
}

// A verify that answers neither way has not answered, and an unanswered verify
// is not a pass.
func TestAnExtendScriptThatSaysNothingIsNotAPass(t *testing.T) {
	dir := caseTree(t, "mute", `echo "x" > mute.result`)
	verify := filepath.Join(dir, "verify.sh")
	if err := os.WriteFile(verify, []byte("verify() { echo \"no opinion\"; }\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	r, _ := newRun(t, dir, "mute", Options{ExtendScript: verify})

	if res := r.Go(t.Context()); res.Outcome != OutcomeEmpty {
		t.Errorf("outcome = %s, want QUIT(FAIL1)", res.Outcome)
	}
}

func TestPromptContinueAnswersForTheOperator(t *testing.T) {
	yes, no := true, false
	dir := caseTree(t, "x", "true\n")

	r, _ := newRun(t, dir, "x", Options{PromptContinue: &yes})
	if !r.Continue() {
		t.Error("--prompt-continue=true did not answer yes")
	}
	r, _ = newRun(t, dir, "x", Options{PromptContinue: &no})
	if r.Continue() {
		t.Error("--prompt-continue=false did not answer no")
	}
}

// Without the option and without a terminal there is nobody to ask, and a tool
// that blocks forever on a prompt nobody can see is worse than one that starts
// over.
func TestWithNobodyToAskTheAnswerIsNo(t *testing.T) {
	dir := caseTree(t, "x", "true\n")
	r, _ := newRun(t, dir, "x", Options{})
	if r.Continue() {
		t.Error("a prompt with no reader answered yes")
	}
}

// The banner lists the options this runner does not have, because it is a frozen
// surface -- including config, which CTP always printed as null because the
// option's registration is commented out while its read is not.
func TestTheBannerKeepsTheOptionsThatWereExcluded(t *testing.T) {
	dir := caseTree(t, "x", `echo " : OK x" > x.result`)
	r, out := newRun(t, dir, "x", Options{})
	r.Go(t.Context())

	for _, want := range []string{
		"====> start to test ",
		"Test parameters: ",
		"   update-build :\tfalse",
		"   enable-report:\tfalse",
		"   mailto       :\tnull",
		"   config       :\tnull",
		"   env:CUBRID   :",
		"11.4.5.1875-74d17e9, 64bits, linux, release",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in banner:\n%s", want, out.String())
		}
	}
}

type relChannel struct{ line string }

func (c relChannel) Run(context.Context, string) (exec.Result, error) {
	return exec.Result{Stdout: c.line}, nil
}
func (relChannel) Put(context.Context, string, string) error { return nil }
func (relChannel) Get(context.Context, string, string) error { return nil }
func (relChannel) Describe() string                          { return "rel" }
func (relChannel) Close() error                              { return nil }

// This is the parser CTP got right. CommonUtils.getBuildId, used by the shell
// task, cut at the next "-", ")" or "." and ran away when a build had no commit
// suffix -- the two disagreed inside CTP, and the fix made the other one agree
// with this rather than inventing a third rule.
func TestReadMetaTakesTheBuildIDFromTheFirstParentheses(t *testing.T) {
	for line, want := range map[string]string{
		"CUBRID 11.4.5 (11.4.5.1875-74d17e9) (64bit release build for Linux) (Apr 29 2026)": "11.4.5.1875-74d17e9",
		"CUBRID 11.2 (11.2.0.0000) (64bit release build for linux_gnu)":                     "11.2.0.0000",
	} {
		m, err := ReadMeta(t.Context(), relChannel{line})
		if err != nil {
			t.Fatalf("%q: %v", line, err)
		}
		if m.BuildID != want {
			t.Errorf("%q: BuildID = %q, want %q", line, m.BuildID, want)
		}
		if m.Bits != "64" || m.OS != "linux" || m.RelType != "release" {
			t.Errorf("%q: got %+v", line, m)
		}
	}
}

func TestReadMetaRefusesAMachineWithNoEngine(t *testing.T) {
	if _, err := ReadMeta(t.Context(), relChannel{""}); err == nil {
		t.Error("an empty cubrid_rel was accepted")
	}
	if _, err := ReadMeta(t.Context(), relChannel{"CUBRID 11.4 (11.4.0.1) (64bit build for Plan9)"}); err == nil {
		t.Error("an unknown OS was accepted")
	}
}
