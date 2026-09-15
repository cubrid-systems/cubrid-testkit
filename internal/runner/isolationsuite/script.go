package isolationsuite

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/cubrid-systems/cubrid-testkit/internal/conf"
	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
)

// The scripts below are the ones CTP's isolation module sends, assembled the way
// it assembled them. shell/common/ScriptInput.addCommand ends every command with a
// newline, and three classes put something in front: GeneralScriptInput finds
// CTP_HOME and goes home, ShellScriptInput adds init_path, and
// IsolationScriptInput adds ctlpath. What each prologue sets is the environment
// runone.sh and the checks are entitled to assume.

// general is GeneralScriptInput's prologue.
const general = `if [ "${CTP_HOME}" == "" ]; then
  if which ctp.sh >/dev/null 2>&1 ; then
    CTP_HOME=$(dirname $(readlink -f ` + "`which ctp.sh`" + `))/..
  elif [ ! "${init_path}" == "" ]; then
    CTP_HOME=${init_path}/../..
  fi
fi
ulimit -c unlimited
if [ "${CTP_HOME}" != "" ]; then
  export CTP_HOME=$(cd ${CTP_HOME}; pwd)
  export PATH=${CTP_HOME}/bin:${CTP_HOME}/common/script:$PATH
fi
cd
`

// isolationInit is IsolationScriptInput's addition (IsolationScriptInput.java:35-36).
// $ctlpath is where runone.sh is run from, and nothing on the machine sets it:
// the runner does, in every script.
const isolationInit = "export ctlpath=${CTP_HOME}/isolation/ctltool\nexport PATH=${ctlpath}:$PATH\n"

// shellInit is ShellScriptInput's addition. The build query and the engine
// configuration go through it.
const shellInit = "export init_path=${CTP_HOME}/shell/init_path\n"

func assemble(prologue string, commands []string) string {
	var b strings.Builder
	b.WriteString(prologue)
	for _, c := range commands {
		b.WriteString(c)
		b.WriteByte('\n')
	}
	return b.String()
}

func isolationScript(commands ...string) string {
	return assemble(general+isolationInit, commands)
}
func generalScript(commands ...string) string { return assemble(general, commands) }
func shellScript(commands ...string) string   { return assemble(general+shellInit, commands) }

// run sends a script the way SSHConnect did, even to the local machine: behind
// the profile and between two markers (ScriptInput.getCommands). Read what it
// printed with output, not Result.Output. The exit status is the closing echo's,
// so a script whose failure matters has to say so in what it prints.
func run(ctx context.Context, ch exec.Channel, script string) (exec.Result, error) {
	return ch.Run(ctx, exec.Profile+"\necho "+startMock+"\n"+script+"echo "+compMock+"\n")
}

// The markers are written so that the script's own text does not contain the
// word it prints: a trace or an echo of the script cannot end the output early.
const (
	startMock = "ALL_${NOTEXIST}STARTED"
	compMock  = "ALL_${NOTEXIST}COMPLETED"
	startFlag = "ALL_STARTED"
	compFlag  = "ALL_COMPLETED"
)

// output is what CTP read back from a script (SSHConnect.extractOutput): standard
// output from the start marker to the end marker, trimmed as Java's String.trim
// trims.
//
// The trim shows. CTP writes a script's output to the worker log with println, so
// a trailing newline is not a blank line in its log, and a diff handed to feedback
// ends at its last line -- measured, one blank line per case before this was done.
// Standard error never reaches it: the local invoker appended it after the end
// marker, where the cut drops it, and runone.sh's trace is there only because the
// command ends with 2>&1.
func output(res exec.Result) string {
	s := res.Stdout
	if p := strings.Index(s, startFlag); p != -1 {
		s = s[p+len(startFlag):]
	}
	if p := strings.Index(s, compFlag); p != -1 {
		s = s[:p]
	}
	return javaTrim(s)
}

// javaTrim is String.trim: every character up to and including the space, and no
// other kind of white space.
func javaTrim(s string) string { return strings.TrimFunc(s, func(r rune) bool { return r <= ' ' }) }

// versionCommand is what CommonUtils.getBuildVersionInfo asks the machine.
const versionCommand = "cat ${CUBRID}/qa.conf |grep 'Test_Build_URL'||cubrid_rel"

// options is what a case is run with, read as Context read it.
type options struct {
	retries    int    // testcase_retry_num; runone.sh gets one more attempt than this
	timeout    string // testcase_timeout_in_secs, passed through as written
	client     string // the client program cubrid_testdb_name maps to
	backupCore bool   // backup_core_file_yn
}

func optionsOf(cfg *conf.Config) options {
	o := options{
		timeout:    cfg.GetOr("testcase_timeout_in_secs", strconv.Itoa(1<<31-1)),
		client:     clientFor(cfg.GetOr("cubrid_testdb_name", "")),
		backupCore: cfg.Bool("backup_core_file_yn", true),
	}
	if v := cfg.GetOr("testcase_retry_num", "0"); v != "" {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			fmt.Println("Fail to read 'testcase_retry_num' value")
		} else if n > 0 {
			o.retries = n
		}
	}
	return o
}

// clientFor is Context.getTestingDatabase. The key's name says database and the
// value is not one: it picks the client program runone.sh is handed as its last
// argument, and the database is always ctldb (runone.sh:122). A value CTP had no
// entry for became the string "null", and runone.sh then refuses to run; that is
// kept, because it fails the case where the configuration is wrong.
func clientFor(testdb string) string {
	switch v := strings.ToLower(strings.TrimSpace(testdb)); v {
	case "", "cubrid":
		return "qacsql"
	case "mysql":
		return "qamysql"
	default:
		return "null"
	}
}

// runoneScript is the command Test.runTestCase sends for one case
// (Test.java:174-192), spaces included: the core option is " -n " or nothing,
// between two spaces of its own.
func runoneScript(tc string, o options) string {
	path := strings.TrimSpace(tc)
	if !strings.HasPrefix(path, "/") {
		path = "$HOME/" + path
	}
	core := ""
	if !o.backupCore {
		core = " -n "
	}
	return isolationScript("",
		"ulimit -c unlimited",
		"export TEST_ID=0",
		"cd $ctlpath",
		"sh runone.sh "+core+" -r "+strconv.Itoa(o.retries+1)+" "+path+" "+o.timeout+" "+o.client+" 2>&1")
}

// diffScript is what Test.showDifferenceBetweenAnswerAndResult runs for a failed
// case. It compares the base answer only, even for a case that has others, and
// with the normalized result rather than the raw one.
func diffScript(tc string) string {
	tc = strings.ReplaceAll(tc, `\`, "/")
	dir, name := ".", tc
	if p := strings.LastIndex(tc, "/"); p >= 0 {
		dir, name = tc[:p], tc[p+1:]
	}
	name = strings.ReplaceAll(name, ".ctl", "")
	answer := dir + "/answer/" + name + ".answer"
	result := dir + "/result/" + name + ".log"
	return isolationScript("cd ",
		"touch "+result,
		"mkdir -p "+dir+"/result/",
		"diff -a -y -W 185 "+answer+" "+result)
}

// killScript is Constants.LIN_KILL_PROCESS, which Deploy runs before anything
// else. It selects on $USER, so inside a slot -- where every process is the
// slot's and none answers to the user's name -- it finds nothing, and the slot is
// new anyway. Kept for what it prints into the worker log.
func killScript() string {
	var b strings.Builder
	b.WriteString("cubrid service stop\n")
	for _, name := range []string{"cub_admin", "cub_master", "cub_server"} {
		q := "ps -u $USER -f| grep -v grep | grep " + name + " | awk '{print $2}'"
		b.WriteString(q + " | xargs -i kill -9 {} \n")
		b.WriteString("kill -9 `" + q + "`\n")
		b.WriteString("\n")
	}
	return b.String()
}

// inquireOnExit is the line Deploy appends to cubrid.conf for a build of major
// version 10 or later -- on every run, so an install that is not thrown away
// collects one per run (DeployOneNode.java:75-79). In a slot the install is
// behind an overlay, and the line goes with it.
const inquireOnExit = "echo inquire_on_exit=3 >> $CUBRID/conf/cubrid.conf"

func installScript(buildID string) string {
	major, err := strconv.Atoi(strings.SplitN(buildID, ".", 2)[0])
	if err != nil || major < 10 {
		return isolationScript()
	}
	return isolationScript(inquireOnExit)
}
