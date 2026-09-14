package isolationsuite

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
)

// The isolation module's CheckRequirement is not shell's: three variables and
// five commands, the scenario and ctltool's directory, and "Check ssh
// connection" where shell says "Check connection(ssh)"
// (isolation/CheckRequirement.java:51-88). check_local.log is one of the files a
// comparison holds to zero differences, so the list and the wording are CTP's.
var (
	checkedVariables = []string{"JAVA_HOME", "CTP_HOME", "CUBRID"}
	checkedCommands  = []string{"java", "diff", "wget", "find", "cat"}
)

// ctltoolDir is checked as written, unexpanded, and expanded by the shell that
// runs the check.
const ctltoolDir = "${CTP_HOME}/isolation/ctltool"

// checker writes check_<envId>.log, and the same lines to standard output: the
// Log CTP used echoes.
type checker struct {
	ch      exec.Channel
	title   string // "local"
	out     io.Writer
	log     io.Writer
	logPath string
	failed  bool
}

func (c *checker) check(ctx context.Context, scenario string) bool {
	c.failed = false
	c.line("=================== Check " + c.title + "============================")

	// CTP checked the connection by opening it. Reaching here means it opened.
	c.print("==> Check ssh connection ")
	c.print("...... PASS")
	c.line("")

	for _, v := range checkedVariables {
		c.print("==> Check variable '" + v + "' ")
		res, err := run(ctx, c.ch, generalScript("echo $"+v))
		switch {
		case err != nil:
			c.fail("...... FAIL: " + err.Error())
		case output(res) == "":
			c.fail("...... FAIL. Please set " + v + ".")
		default:
			c.print("...... PASS")
		}
		c.line("")
	}

	// CTP's rule, which looks for csh's "no <cmd> in ..." and so passes a missing
	// command on a bash machine. shell's runner found that out on dos2unix and
	// changed its own check; this one is kept, because nothing in this list is a
	// command a Linux machine lacks, and the log it writes is compared.
	for _, cmd := range checkedCommands {
		c.print("==> Check command '" + cmd + "' ")
		res, err := run(ctx, c.ch, generalScript("which "+cmd+" 2>&1 "))
		switch {
		case err != nil:
			c.fail("...... FAIL: " + err.Error())
		case strings.Contains(output(res), "no "+cmd):
			c.fail("...... Result: FAIL. Not found executable " + cmd)
		default:
			c.print("...... PASS")
		}
		c.line("")
	}

	for _, dir := range []string{scenario, ctltoolDir} {
		c.print("==> Check directory '" + dir + "' ")
		res, err := run(ctx, c.ch, generalScript(`if [ -d "`+dir+`" ]; then echo PASS; else echo FAIL; fi`))
		switch {
		case err != nil:
			c.fail("...... FAIL: " + err.Error())
		case strings.Contains(output(res), "PASS"):
			c.print("...... PASS")
		default:
			c.fail("...... FAIL. Not found")
		}
		c.line("")
	}

	if c.failed {
		c.line("Log: " + c.logPath)
	}
	c.line("")
	return !c.failed
}

func (c *checker) print(s string) {
	if c.log != nil {
		fmt.Fprint(c.log, s)
	}
	if c.out != nil {
		fmt.Fprint(c.out, s)
	}
}

func (c *checker) line(s string) { c.print(s + "\n") }

func (c *checker) fail(s string) {
	c.failed = true
	c.print(s)
}
