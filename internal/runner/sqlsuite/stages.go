package sqlsuite

import (
	"context"
	_ "embed"

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
	return exec.Check(ch.Run(ctx, "exec 2>&1\n"+s.header(e, logFile)+stagesScript+"\n"+calls+"\n"))
}
