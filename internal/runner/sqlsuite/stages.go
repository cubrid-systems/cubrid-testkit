package sqlsuite

import (
	"context"
	_ "embed"

	"github.com/cubrid-systems/cubrid-testkit/internal/contain"
	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
)

//go:embed stages.sh
var stagesScript string

// stage runs stages.sh's functions in p -- calls is the line that names them --
// and hands back what they printed, standard error folded into standard output
// in the order it came, as run.sh's own output reached CTP's console. A stage
// that exits non-zero is an error: each of them prepares something every case
// depends on.
func stage(ctx context.Context, ch exec.Channel, s *settings, e engine, logFile, calls string) (exec.Result, error) {
	return exec.Check(ch.Run(ctx, "exec 2>&1\n"+contained(ch)+s.header(e, logFile)+stagesScript+"\n"+calls+"\n"))
}

// contained tells the script whether the run has the account to itself.
//
// do_clean's two account-wide steps -- `pkill cub` and the ipcrm loop -- select
// everything the user owns, which is the run only inside namespaces of its own.
// The script refuses them otherwise (stages.sh, sweep_this_account), so this is
// the one fact it needs and cannot work out for itself: the environment does
// not cross a channel that reaches another machine, and for a machine this
// process cannot see the honest answer is no.
func contained(ch exec.Channel) string {
	if contain.Active() && local(ch) {
		return "testkit_contained=1\n"
	}
	return "testkit_contained=\n"
}

// local reports whether the script will run on this machine, where this
// process's namespaces are the ones it would run in.
func local(ch exec.Channel) bool {
	_, ok := ch.(*exec.Local)
	return ok
}
