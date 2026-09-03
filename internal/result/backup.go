package result

import (
	"fmt"
	"os/exec"
	"time"
)

// Backup packs the run directory into a tarball beside it.
//
// The name is "fail backup" in CTP and it runs whether or not anything failed:
// TestFactory calls it once, at the end, unconditionally. What it produces is a
// snapshot of the whole run that can be carried off the machine before the next
// run overwrites current_runtime_logs.
//
// The timestamp is CTP's, including the part that looks wrong. Calendar.HOUR is
// the twelve-hour clock, so a run finishing at 14:07 is stamped 2.07 and one
// finishing at 02:07 is stamped the same. Two runs on the same day, twelve hours
// apart, collide -- and the build id and task id in front of it are what actually
// tell them apart. Reproduced rather than corrected: the name is a frozen
// surface, and a reader who greps for a run by its stamp should keep finding it.
func (s *Sink) Backup(buildID, bits string, taskID int, at time.Time) (string, error) {
	h := at.Hour() % 12
	name := fmt.Sprintf("shell_result_%s_%s_%d_%d.%d.%d_%d.%d.%d.tar.gz",
		buildID, bits, taskID,
		at.Year(), int(at.Month()), at.Day(),
		h, at.Minute(), at.Second())

	// tar is invoked rather than written in Go: the archive has to be readable by
	// whatever CTP's consumers already use, and "tar zvcf <name> <dir>" stores the
	// directory under its full path, which is what those consumers unpack.
	cmd := exec.Command("tar", "zvcf", name, s.dir)
	cmd.Dir = s.root
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("packing %s: %w: %s", name, err, out)
	}
	return name, nil
}
