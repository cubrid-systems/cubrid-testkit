package shellsuite

import (
	"context"
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

// runIn sends a script with the prologue in front of it. Everything this package
// puts on a channel goes through here, because everything it sends assumes the
// environment the prologue sets up.
func runIn(ctx context.Context, ch exec.Channel, script string) (exec.Result, error) {
	return ch.Run(ctx, withPrologue(script))
}
