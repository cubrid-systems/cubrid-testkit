package shellsuite

import "github.com/cubrid-systems/cubrid-testkit/internal/exec"

// safeToEmpty and shQuote live in internal/exec now: both are about handing a
// path to a shell, both suites need them, and the lesson behind the first one
// (a run that deleted 69 directories because a variable was unset) is not
// shell's alone.
func safeToEmpty(what, dir string) error { return exec.SafeToEmpty(what, dir) }
