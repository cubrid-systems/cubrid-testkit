package sqlsuite

import (
	"strings"
	"testing"

	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
)

// do_clean deletes the database, everything under $CUBRID/logs, and every file
// named core* under $CUBRID and $CTP_HOME. The paths it uses come from the
// environment and from a configuration file, so each one is checked before the
// stage runs -- an empty one in a shell script is the root of the filesystem,
// which is how a run on the shell side deleted 69 directories.
func TestThePathsTheCleanStageDeletesUnderAreRefusedWhenTheyAreNotOwn(t *testing.T) {
	for _, c := range []struct {
		name, dir, want string
	}{
		{"unset", "", "empty"},
		{"relative", "cubrid", "absolute"},
		{"the root", "/", "that shallow"},
		{"one segment", "/usr", "that shallow"},
		{"a space, which is two arguments to rm", "/opt/my cubrid", "syntax"},
		{"a variable nobody expanded", "/opt/$HOME/cubrid", "syntax"},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := exec.SafeToEmpty("CUBRID", c.dir)
			if err == nil {
				t.Fatalf("%q was accepted as a directory to delete under", c.dir)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("%q was refused with %q, which does not say %q", c.dir, err, c.want)
			}
		})
	}
	if err := exec.SafeToEmpty("CUBRID", "/data/cub_sys/projects/regr-sql/CUBRID"); err != nil {
		t.Errorf("an ordinary install path was refused: %v", err)
	}
}
