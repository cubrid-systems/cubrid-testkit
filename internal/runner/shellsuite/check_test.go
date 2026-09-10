package shellsuite

import (
	"bytes"
	"strings"
	"testing"

	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
)

func checker(t *testing.T) (*CheckRequirement, *bytes.Buffer) {
	t.Helper()
	out := &bytes.Buffer{}
	return &CheckRequirement{
		EnvID: "local", Title: "local", Protocol: "ssh",
		Channel: exec.NewLocal(""), Out: out,
	}, out
}

// CTP asked "which <cmd>" and looked for the string "no <cmd>" in the output,
// which is csh's wording. On a bash machine `which` prints nothing and returns
// non-zero, so every missing command reported PASS -- the check that exists to
// catch a misconfigured machine could not catch the one thing it was most likely
// to find. It was watched happening: check_local.log said dos2unix PASS on a
// machine without dos2unix, and the run then failed on "command not found".
func TestAMissingCommandFails(t *testing.T) {
	c, out := checker(t)
	c.checkCommand(t.Context(), "a-command-no-machine-has")

	if !c.failed {
		t.Error("a missing command was reported as present")
	}
	if !strings.Contains(out.String(), "Not found executable") {
		t.Errorf("got %q", out.String())
	}
}

func TestAPresentCommandPasses(t *testing.T) {
	c, out := checker(t)
	c.checkCommand(t.Context(), "cat")

	if c.failed {
		t.Errorf("cat was reported missing: %q", out.String())
	}
	if !strings.Contains(out.String(), "PASS") {
		t.Errorf("got %q", out.String())
	}
}

// dos2unix is not required. init.sh strips CRLF before diffing an answer file,
// which matters when one was edited on Windows -- and Windows is out of scope.
// None of the 51 answer files in the shell corpus has CRLF, so requiring the
// command fails a machine for a reason that cannot arise.
func TestDos2unixIsNotRequired(t *testing.T) {
	for _, cmd := range checkedCommands {
		if cmd == "dos2unix" {
			t.Fatal("dos2unix is back in the required list; Windows is out of scope (ADR-003 revision)")
		}
	}
}

// expect is required, and it is not CTP's. 216 cases run it and the corpus ships
// 236 .exp scripts, so it carries more of the corpus than three of the commands
// CTP does check: wget is wanted by 2 cases, tar by 10, kill by 149.
//
// It earns its place by how it fails rather than by how often it is used. A
// machine without expect does not stop: the case runs, the .exp script does not,
// and the case greps a log that was never written. bug_bts_6159 and bug_bts_8456
// both end at `cnt=0` that way, and a plain NOK is what the operator sees --
// indistinguishable from a wrong answer. One line at the start of the run is the
// whole point of this list.
func TestExpectIsRequired(t *testing.T) {
	for _, cmd := range checkedCommands {
		if cmd == "expect" {
			return
		}
	}
	t.Error("expect is not in the required list; 216 cases run it and fail silently without it")
}
