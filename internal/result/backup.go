package result

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"time"
)

// Backup packs the run directory into a tarball beside it.
//
// The name is "fail backup" in CTP and it runs whether or not anything failed:
// TestFactory calls it once, at the end, unconditionally. What it produces is a
// snapshot of the whole run that can be carried off the machine before the next
// run overwrites current_runtime_logs.
func (s *Sink) Backup(buildID, bits string, taskID int, at time.Time) (string, error) {
	name := backupName("shell", buildID, bits, taskID, at)

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

// BackupIsolation is Backup as the isolation module does it
// (isolation/TestFactory.java:130-147). The name has isolation in front, and the
// archive holds the directory's contents relative to it -- `cd <dir>; tar zvcf
// ../<name> .` -- where shell's holds the directory under its full path. Whatever
// unpacks one kind of archive puts the other somewhere else.
func (s *Sink) BackupIsolation(buildID, bits string, taskID int, at time.Time) (string, error) {
	name := backupName("isolation", buildID, bits, taskID, at)
	cmd := exec.Command("tar", "zvcf", filepath.Join("..", name), ".")
	cmd.Dir = s.dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("packing %s: %w: %s", name, err, out)
	}
	return name, nil
}

// backupName is <module>_result_<build>_<bits>_<task>_<stamp>.tar.gz.
//
// The timestamp is CTP's, including the part that looks wrong. Calendar.HOUR is
// the twelve-hour clock, so a run finishing at 14:07 is stamped 2.07 and one
// finishing at 02:07 is stamped the same. Two runs on the same day, twelve hours
// apart, collide -- and the build id and task id in front of it are what actually
// tell them apart. Reproduced rather than corrected: the name is a frozen
// surface, and a reader who greps for a run by its stamp should keep finding it.
func backupName(module, buildID, bits string, taskID int, at time.Time) string {
	return fmt.Sprintf("%s_result_%s_%s_%d_%d.%d.%d_%d.%d.%d.tar.gz",
		module, buildID, bits, taskID,
		at.Year(), int(at.Month()), at.Day(),
		at.Hour()%12, at.Minute(), at.Second())
}
