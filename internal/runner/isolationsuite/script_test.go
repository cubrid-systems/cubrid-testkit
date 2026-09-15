package isolationsuite

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cubrid-systems/cubrid-testkit/internal/conf"
	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
)

func TestTheRunoneLineIsCTPsSpacesIncluded(t *testing.T) {
	o := options{retries: 4, timeout: "300", client: "qacsql", backupCore: true}
	got := runoneScript("/s/_01/a/b.ctl", o)
	if want := "cd $ctlpath\nsh runone.sh  -r 5 /s/_01/a/b.ctl 300 qacsql 2>&1\n"; !strings.HasSuffix(got, want) {
		t.Errorf("script ends:\n%s\nwant:\n%s", tail(got, 2), want)
	}
	for _, want := range []string{
		"export ctlpath=${CTP_HOME}/isolation/ctltool\nexport PATH=${ctlpath}:$PATH\n",
		"ulimit -c unlimited\nexport TEST_ID=0\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("script lacks %q", want)
		}
	}

	o.backupCore = false
	got = runoneScript("cubrid-testcases/isolation/b.ctl", o)
	if want := "sh runone.sh  -n  -r 5 $HOME/cubrid-testcases/isolation/b.ctl 300 qacsql 2>&1\n"; !strings.HasSuffix(got, want) {
		t.Errorf("script ends:\n%s\nwant:\n%s", tail(got, 1), want)
	}
}

func TestTheLastArgumentIsAClientProgramNotADatabase(t *testing.T) {
	for in, want := range map[string]string{
		"": "qacsql", "cubrid": "qacsql", " CUBRID ": "qacsql", "mysql": "qamysql", "oracle": "null",
	} {
		if got := clientFor(in); got != want {
			t.Errorf("cubrid_testdb_name=%q gives %q, want %q", in, got, want)
		}
	}
}

func TestOptionsAreReadAsContextReadThem(t *testing.T) {
	defaults := load(t, "")
	if o := optionsOf(defaults); o.retries != 0 || o.timeout != "2147483647" || o.client != "qacsql" || !o.backupCore {
		t.Errorf("defaults = %+v", o)
	}
	set := load(t, "testcase_retry_num=4\ntestcase_timeout_in_secs=300\nbackup_core_file_yn=no\ncubrid_testdb_name=mysql\n")
	if o := optionsOf(set); o.retries != 4 || o.timeout != "300" || o.client != "qamysql" || o.backupCore {
		t.Errorf("configured = %+v", o)
	}
	if o := optionsOf(load(t, "testcase_retry_num=-3\n")); o.retries != 0 {
		t.Errorf("a negative retry count gave %d, want 0", o.retries)
	}
}

func TestTheDiffIsTheBaseAnswerAgainstTheNormalizedResult(t *testing.T) {
	got := diffScript("/s/_01/a/select.ctl_01.ctl")
	// ".ctl" is removed wherever it is in the name, as CommonUtils.replace did.
	want := "cd \ntouch /s/_01/a/result/select_01.log\nmkdir -p /s/_01/a/result/\n" +
		"diff -a -y -W 185 /s/_01/a/answer/select_01.answer /s/_01/a/result/select_01.log\n"
	if !strings.HasSuffix(got, want) {
		t.Errorf("script ends:\n%s\nwant:\n%s", tail(got, 4), want)
	}
}

func TestInquireOnExitIsForTenAndLater(t *testing.T) {
	if !strings.Contains(installScript("11.5.0.2574-f1ae86f"), inquireOnExit) {
		t.Error("an 11.5 build did not get the line")
	}
	for _, id := range []string{"9.3.0.0206", "not-a-build"} {
		if strings.Contains(installScript(id), inquireOnExit) {
			t.Errorf("%s got the line", id)
		}
	}
}

func TestTheKillScriptIsConstantsLinKillProcess(t *testing.T) {
	got := killScript()
	want := "cubrid service stop\n" +
		"ps -u $USER -f| grep -v grep | grep cub_admin | awk '{print $2}' | xargs -i kill -9 {} \n" +
		"kill -9 `ps -u $USER -f| grep -v grep | grep cub_admin | awk '{print $2}'`\n\n"
	if !strings.HasPrefix(got, want) || strings.Count(got, "kill -9 `") != 3 {
		t.Errorf("got:\n%s", got)
	}
}

// What CTP read back is standard output between the markers, trimmed as Java
// trims: the profile's noise before the start and standard error after the end
// are both gone, and so is the trailing newline a worker log would otherwise
// turn into a blank line.
func TestOutputIsWhatCTPReadBack(t *testing.T) {
	res := exec.Result{
		Stdout: "profile noise\nALL_STARTED\n  a\nb\n\nALL_COMPLETED\n",
		Stderr: "kill: not enough arguments\n",
	}
	if got := output(res); got != "a\nb" {
		t.Errorf("output = %q, want %q", got, "a\nb")
	}
	// A non-breaking space is not something String.trim removes.
	if got := output(exec.Result{Stdout: " x\t\n"}); got != " x" {
		t.Errorf("output = %q", got)
	}
	// And the script carries the markers in a form its own text cannot print.
	s := isolationScript("echo hi")
	if strings.Contains(s, startFlag) || strings.Contains(s, compFlag) {
		t.Errorf("the script contains a marker verbatim:\n%s", s)
	}
}

func load(t *testing.T, body string) *conf.Config {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "isolation.conf")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := (&conf.Home{Path: dir}).Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func tail(s string, n int) string {
	lines := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
