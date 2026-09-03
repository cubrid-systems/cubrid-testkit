package shellsuite

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
)

// checkedVariables, checkedCommands and checkedDirectories are what a machine has
// to have before a case is worth running on it. The list is CTP's, in CTP's
// order, and it is a better answer than a deployment guide: it is executable, it
// runs on the machine in question, and it says which one failed.
var (
	checkedVariables = []string{"HOME", "USER", "JAVA_HOME", "CTP_HOME", "init_path", "CUBRID"}
	// dos2unix is CTP's and is not here. init.sh strips CRLF from the two files it
	// is about to diff, which matters when an answer file was edited on Windows --
	// and Windows is out of scope (ADR-003 revision). Of the 51 answer files in the
	// shell corpus, none has CRLF, so on Linux the command is dead weight and
	// requiring it fails a machine for a reason that cannot arise.
	checkedCommands    = []string{"java", "javac", "diff", "wget", "find", "cat", "kill", "tar"}
	checkedDirectories = []string{"${CTP_HOME}/bin", "${CTP_HOME}/common/script"}
)

// CheckRequirement reports whether a machine is set up to run cases, writing the
// same check_<envId>.log CTP wrote -- to the file and to standard output, because
// the log it uses echoes.
type CheckRequirement struct {
	EnvID    string
	Title    string // how the machine is named in the log; "local" for a local run
	Protocol string // service_protocol_type, which is "ssh" even for a local run
	Channel  exec.Channel
	Scenario string
	// ExcludeFile is checked only when the configuration names one.
	ExcludeFile string

	// Out receives the same lines as the log.
	Out io.Writer
	// Log is check_<envId>.log; may be nil.
	Log io.Writer

	failed bool
}

// Check runs every requirement and reports whether all of them passed.
func (c *CheckRequirement) Check(ctx context.Context) bool {
	c.failed = false
	c.line("=================== Check %s============================", c.Title)

	// CTP checked the connection by opening it. Reaching here means it opened.
	c.print("==> Check connection(%s) ", c.Protocol)
	c.print("...... PASS")
	c.line("")

	for _, v := range checkedVariables {
		c.print("==> Check variable '%s' ", v)
		out, err := runIn(ctx, c.Channel, "echo $"+strings.TrimSpace(v))
		switch {
		case err != nil:
			c.fail("...... FAIL: %v", err)
		case strings.TrimSpace(out.Output()) == "":
			c.fail("...... FAIL. Please set %s.", v)
		default:
			c.print("...... PASS")
		}
		c.line("")
	}

	for _, cmd := range checkedCommands {
		c.print("==> Check command '%s' ", cmd)
		out, err := runIn(ctx, c.Channel, "which "+cmd+" 2>&1 ")
		switch {
		case err != nil:
			c.fail("...... FAIL: %v", err)
		// CTP looked for csh's "no <cmd> in ..." wording rather than for an exit
		// code, so a shell that says anything else reports a pass. Reproduced: the
		// alternative is failing runs on machines CTP passes.
		case strings.Contains(out.Output(), "no "+cmd):
			c.fail("...... Result: FAIL. Not found executable %s", cmd)
		default:
			c.print("...... PASS")
		}
		c.line("")
	}

	for _, dir := range checkedDirectories {
		c.checkPath(ctx, "directory", "-d", dir)
	}
	if c.Scenario != "" {
		c.checkPath(ctx, "directory", "-d", c.Scenario)
	}
	if c.ExcludeFile != "" {
		c.checkPath(ctx, "file", "-f", c.ExcludeFile)
	}

	c.line("")
	return !c.failed
}

// checkCommand asks the machine whether it has a command.
//
// CTP asked by running "which <cmd>" and looking for the string "no <cmd>" in
// the output. That is csh's wording. On a bash machine `which` prints nothing
// and returns non-zero, the string is absent, and **every missing command
// reports PASS** -- so the check that exists to catch a misconfigured machine
// could not catch the one thing it was most likely to find. It was watched
// happening: check_local.log said "Check command 'dos2unix' ...... PASS" on a
// machine without dos2unix, and the run then failed four times on
// "dos2unix: command not found" (docs/evidence/regression-shell.md).
//
// The verdict is computed on the far side rather than from the channel's exit
// code, because the exit code is not available: the SSH channel closes its frame
// with an echo, so a remote script's status is always the echo's. The same shape
// is what checkDirectory and checkFile already use.
func (c *CheckRequirement) checkCommand(ctx context.Context, cmd string) {
	c.print("==> Check command '%s' ", cmd)
	out, err := runIn(ctx, c.Channel,
		fmt.Sprintf("if which %s >/dev/null 2>&1; then echo PASS; else echo FAIL; fi", cmd))
	switch {
	case err != nil:
		c.fail("...... FAIL: %v", err)
	case strings.Contains(out.Output(), "PASS"):
		c.print("...... PASS")
	default:
		c.fail("...... Result: FAIL. Not found executable %s", cmd)
	}
	c.line("")
}

func (c *CheckRequirement) checkPath(ctx context.Context, kind, test, path string) {
	c.print("==> Check %s '%s' ", kind, path)
	out, err := runIn(ctx, c.Channel,
		fmt.Sprintf(`if [ %s "%s" ]; then echo PASS; else echo FAIL; fi`, test, path))
	switch {
	case err != nil:
		c.fail("...... FAIL: %v", err)
	case strings.Contains(out.Output(), "PASS"):
		c.print("...... PASS")
	default:
		c.fail("...... FAIL. Not found")
	}
	c.line("")
}

func (c *CheckRequirement) print(format string, args ...any) {
	s := fmt.Sprintf(format, args...)
	if c.Log != nil {
		fmt.Fprint(c.Log, s)
	}
	if c.Out != nil {
		fmt.Fprint(c.Out, s)
	}
}

func (c *CheckRequirement) line(format string, args ...any) {
	c.print(format+"\n", args...)
}

func (c *CheckRequirement) fail(format string, args ...any) {
	c.failed = true
	c.print(format, args...)
}
