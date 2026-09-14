package isolationsuite

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
)

// fakeChannel answers a script by what it ends with, so a test can say what the
// machine would have printed without running anything on it.
type fakeChannel func(script string) string

func (f fakeChannel) Run(_ context.Context, script string) (exec.Result, error) {
	return exec.Result{Stdout: f(script)}, nil
}
func (fakeChannel) Put(context.Context, string, string) error { return nil }
func (fakeChannel) Get(context.Context, string, string) error { return nil }
func (fakeChannel) Describe() string                          { return "local" }
func (fakeChannel) Close() error                              { return nil }

func TestAnExclusionTakesOneCaseNotEveryMatch(t *testing.T) {
	cases := []string{"/s/_01/a/x_01.ctl", "/s/_01/a/x_02.ctl", "/s/_02/a/x_01.ctl"}
	kept, skipped := exclude(cases, []string{"_01/a/x_0", "nowhere.ctl"})
	if want := []string{"/s/_01/a/x_01.ctl"}; !slices.Equal(skipped, want) {
		t.Errorf("skipped = %q, want %q", skipped, want)
	}
	if want := []string{"/s/_01/a/x_02.ctl", "/s/_02/a/x_01.ctl"}; !slices.Equal(kept, want) {
		t.Errorf("kept = %q, want %q", kept, want)
	}
}

// The match is on the line as written, so a carriage return or a trailing space
// makes an entry that excludes nothing -- in CTP and here.
func TestAnExclusionEntryIsTheLineAsWritten(t *testing.T) {
	got := exclusionEntries("# a comment\n  -- another\n\n_01/a/x.ctl\n_02/b/y.ctl \r\n")
	if want := []string{"_01/a/x.ctl", "_02/b/y.ctl \r"}; !slices.Equal(got, want) {
		t.Fatalf("entries = %q, want %q", got, want)
	}
	if _, skipped := exclude([]string{"/s/_02/b/y.ctl"}, got[1:]); len(skipped) != 0 {
		t.Errorf("an entry with a carriage return excluded %q", skipped)
	}
}

func TestAScenarioUnderHomeBecomesRelativeToIt(t *testing.T) {
	machine := func(dir string) fakeChannel {
		return func(script string) string {
			if strings.Contains(script, "\necho $(cd $HOME; pwd)\n") {
				return "/home/qa\n"
			}
			return dir + "\n"
		}
	}
	for _, c := range []struct{ dir, root, want string }{
		{"/home/qa/cubrid-testcases/isolation", "~/cubrid-testcases/isolation", "cubrid-testcases/isolation"},
		{"/home/qa", "$HOME", "."},
		{"/data/cases/isolation", "/data/cases/isolation/", "/data/cases/isolation/"},
	} {
		got, err := resolveScenario(context.Background(), machine(c.dir), c.root)
		if err != nil || got != c.want {
			t.Errorf("scenario %s at %s = %q, %v; want %q", c.root, c.dir, got, err, c.want)
		}
	}
	if _, err := resolveScenario(context.Background(), machine(dirNotFound+"\n/home/qa"), "/nope"); err == nil ||
		!strings.Contains(err.Error(), "The directory in 'scenario' does not exist") {
		t.Errorf("a missing scenario gave %v", err)
	}
}

func TestCasesAreSorted(t *testing.T) {
	ch := fakeChannel(func(string) string { return "/s/b.ctl\n\n/s/a.ctl\n" })
	got, err := discover(context.Background(), ch, "/s")
	if err != nil || !slices.Equal(got, []string{"/s/a.ctl", "/s/b.ctl"}) {
		t.Errorf("cases = %q, %v", got, err)
	}
}

// check_local.log as CTP's sample run wrote it (isolation-baseline.md §2).
func TestTheCheckLogIsCTPs(t *testing.T) {
	ch := fakeChannel(func(script string) string {
		switch {
		case strings.Contains(script, "\nwhich wget 2>&1 \n"):
			return "which: no wget in (/usr/bin)\n"
		case strings.Contains(script, "\nwhich "):
			return "/usr/bin/x\n"
		case strings.Contains(script, "\nif [ -d "):
			return "PASS\n"
		default:
			return "/set\n"
		}
	})
	var log, out bytes.Buffer
	c := &checker{ch: ch, title: "local", out: &out, log: &log, logPath: "/r/check_local.log"}
	if c.check(context.Background(), "/data/regr-iso/sample") {
		t.Error("a machine without wget passed")
	}
	want := "=================== Check local============================\n" +
		"==> Check ssh connection ...... PASS\n" +
		"==> Check variable 'JAVA_HOME' ...... PASS\n" +
		"==> Check variable 'CTP_HOME' ...... PASS\n" +
		"==> Check variable 'CUBRID' ...... PASS\n" +
		"==> Check command 'java' ...... PASS\n" +
		"==> Check command 'diff' ...... PASS\n" +
		"==> Check command 'wget' ...... Result: FAIL. Not found executable wget\n" +
		"==> Check command 'find' ...... PASS\n" +
		"==> Check command 'cat' ...... PASS\n" +
		"==> Check directory '/data/regr-iso/sample' ...... PASS\n" +
		"==> Check directory '${CTP_HOME}/isolation/ctltool' ...... PASS\n" +
		"Log: /r/check_local.log\n" +
		"\n"
	if log.String() != want {
		t.Errorf("check_local.log:\n%s\nwant:\n%s", log.String(), want)
	}
	if out.String() != log.String() {
		t.Error("standard output and the log differ; the log CTP used echoes")
	}
}
