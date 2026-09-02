package result

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cubrid-systems/cubrid-testkit/internal/conf"
)

func newSink(t *testing.T) (*Sink, *bytes.Buffer) {
	t.Helper()
	h := &conf.Home{Path: t.TempDir()}
	s, err := Open(h, "shell", false)
	if err != nil {
		t.Fatal(err)
	}
	buf := &bytes.Buffer{}
	s.stdout = buf
	t.Cleanup(func() { s.Close() })
	return s, buf
}

// The markers are byte-for-byte contracts. These are the bytes.

func TestMarkers(t *testing.T) {
	s, out := newSink(t)
	s.EnvStart("env1")
	s.TestCase("a/b/c.sh", "env1", true, 0, 0)
	s.TestCase("a/b/d.sh", "env1", false, 0, 0)
	s.Core("/tmp/core.1234")
	s.EnvStop("env1")

	want := strings.Join([]string{
		"[ENV START] env1",
		"[TESTCASE] a/b/c.sh EnvId=env1 [OK]",
		"[TESTCASE] a/b/d.sh EnvId=env1 [NOK]",
		"CORE_FILE:/tmp/core.1234",
		"[ENV STOP] env1",
		"",
	}, "\n")
	if got := out.String(); got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestRetrySuffixHangsOnConfiguration(t *testing.T) {
	// CTP tests maxRetryCount != 0, not whether a retry happened. So a first
	// failure prints TRY->0 as soon as testcase_retry_num is set, and prints
	// nothing when it is not.
	s, out := newSink(t)
	s.TestCase("x.sh", "env2", false, 3, 0) // retries configured, none used yet
	s.TestCase("y.sh", "env2", false, 3, 2) // second retry
	s.TestCase("z.sh", "env2", false, 0, 0) // retries not configured
	s.TestCase("w.sh", "env2", true, 3, 1)  // a pass never carries the suffix

	want := strings.Join([]string{
		"[TESTCASE] x.sh EnvId=env2 [NOK], TRY->0",
		"[TESTCASE] y.sh EnvId=env2 [NOK], TRY->2",
		"[TESTCASE] z.sh EnvId=env2 [NOK]",
		"[TESTCASE] w.sh EnvId=env2 [OK]",
		"",
	}, "\n")
	if got := out.String(); got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestResultDirectoryHasNoTimestamp(t *testing.T) {
	// Context builds it as result/<category>/current_runtime_logs. A fixed name --
	// nothing in the path varies between runs.
	h := &conf.Home{Path: t.TempDir()}
	s, err := Open(h, "shell", false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	want := filepath.Join(h.Path, "result", "shell", "current_runtime_logs")
	if s.Dir() != want {
		t.Errorf("got %q want %q", s.Dir(), want)
	}
	if fi, err := os.Stat(want); err != nil || !fi.IsDir() {
		t.Errorf("directory was not created: %v", err)
	}
}

func TestDispatchFilesAreTheResumeContract(t *testing.T) {
	s, _ := newSink(t)
	if err := s.All([]string{"a.sh", "b.sh", "c.sh"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Finished("env1", "a.sh"); err != nil {
		t.Fatal(err)
	}
	if err := s.Finished("env2", "b.sh"); err != nil {
		t.Fatal(err)
	}
	s.Close()

	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(s.Dir(), name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return string(b)
	}
	if got := read("dispatch_tc_ALL.txt"); got != "a.sh\nb.sh\nc.sh\n" {
		t.Errorf("ALL: %q", got)
	}
	// One case per line and nothing else: continue mode takes ALL minus the union
	// of these, so any decoration here would break resuming.
	if got := read("dispatch_tc_FIN_env1.txt"); got != "a.sh\n" {
		t.Errorf("FIN env1: %q", got)
	}
	if got := read("dispatch_tc_FIN_env2.txt"); got != "b.sh\n" {
		t.Errorf("FIN env2: %q", got)
	}
}

func TestWorkerLogIsPerEnvironment(t *testing.T) {
	s, _ := newSink(t)
	if err := s.Worker("env1", "first"); err != nil {
		t.Fatal(err)
	}
	if err := s.Worker("env1", "second"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	b, err := os.ReadFile(filepath.Join(s.Dir(), "test_env1.log"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "first\nsecond\n" {
		t.Errorf("got %q", string(b))
	}
}
