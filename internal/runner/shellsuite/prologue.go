package shellsuite

import (
	"context"
	"fmt"
	"strings"

	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
)

// Every script the shell task sends starts with this. Without it a case cannot
// find $init_path, and $init_path is the first line of every case in the corpus:
//
//	. $init_path/init.sh
//
// CTP assembled it in three layers -- GeneralScriptInput resolves CTP_HOME and
// puts the CTP scripts on PATH, ShellScriptInput adds init_path, and every
// command follows. The layering is not interesting; what it produces is, because
// it is the environment a case is entitled to assume.
//
// It also explains two things that look arbitrary elsewhere. The bare `cd` is
// why a case script begins by cd-ing to its own directory. And the disk check's
// `source ${init_path}/../../common/script/util_common.sh` only resolves because
// init_path was exported here.
//
// docs/evidence/spec-corrections.md.
const prologue = `if [ "${CTP_HOME}" == "" ]; then
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
export init_path=${CTP_HOME}/shell/init_path
`

// withPrologue puts the environment in front of a script.
//
// The prologue uses [ "$x" == "" ], which is a bashism: under dash it is an
// error, and the CTP_HOME resolution silently does nothing. That is CTP's, it is
// why the shell task has always needed a bash-compatible shell, and it is kept
// rather than corrected -- a POSIX rewrite would change which of the three
// CTP_HOME sources wins on machines where it currently fails through.
func withPrologue(script string) string {
	return prologue + strings.TrimPrefix(script, "\n")
}

// runIn sends a script with the prologue in front of it, and a script that exits
// non-zero is an error. Every script that assumes the environment the prologue
// sets up goes through here or through probeIn.
//
// The error is the default because the other way round failed silently, and more
// than once. A channel reports a command's own status in the Result and keeps
// err for not being able to run it at all, so a caller that checks err alone
// takes a script that failed for one that worked. That is how the reset's guard
// against an unset $CUBRID did nothing for its first hours, and how a patch that
// did not apply could have been reported as applied. Most of what this package
// sends is housekeeping whose failure should stop something -- the snapshot,
// the configuration, a list of exclusions -- so a script that wants its status
// read as an answer has to say so, with probeIn.
//
// Over SSH less of this applies. That channel closes its frame with an echo, so
// a remote script that runs to its end reports the echo's status, and only one
// that calls exit reports its own.
func runIn(ctx context.Context, ch exec.Channel, script string) (exec.Result, error) {
	res, err := probeIn(ctx, ch, script)
	if err == nil && res.ExitCode != 0 {
		err = exitError(res)
	}
	return res, err
}

// probeIn is runIn for a script whose exit status is an answer rather than a
// failure: a case, whose verdict is what it writes and not how it exits; grep,
// whose 1 means nothing matched; a result file that is not there yet; a walk
// over a corpus that could not read all of it. The caller reads res.ExitCode
// itself, or says why it does not.
func probeIn(ctx context.Context, ch exec.Channel, script string) (exec.Result, error) {
	return ch.Run(ctx, withPrologue(script))
}

// exitError says how a script failed: its status, and its own explanation.
//
// From stderr, the first line and the last, and nothing between: a cp that was
// refused a thousand files says so a thousand times, a Java tool puts its
// message first and a stack under it, and a complaint from the prologue comes
// before the script's own. Without stderr, the last thing the script printed,
// which is where a script that reports on stdout puts its reason.
func exitError(res exec.Result) error {
	why := ""
	if lines := strings.Split(strings.TrimSpace(res.Stderr), "\n"); lines[0] != "" {
		why = strings.TrimSpace(lines[0])
		if len(lines) > 1 {
			why += " ... " + strings.TrimSpace(lines[len(lines)-1])
		}
	}
	if why == "" {
		why = strings.TrimSpace(lastLine(strings.TrimSpace(res.Output())))
	}
	if why == "" {
		return fmt.Errorf("exit %d", res.ExitCode)
	}
	return fmt.Errorf("exit %d: %s", res.ExitCode, why)
}
