package isolationsuite

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/cubrid-systems/cubrid-testkit/internal/contain"
)

// runone.sh reaches into $HOME in two places, and a slot's overlays do not cover
// $HOME.
//
// Before every case it empties every file under ~/CUBRID/log (runone.sh:182).
// On a QA machine the install under test is ~/CUBRID, the directory is the
// install's own log, and in a slot it is behind the install's overlay. On a
// machine where the install under test is anywhere else, ~/CUBRID -- if there is
// one -- is somebody's other CUBRID, and every case of every slot would empty
// its logs.
//
// A case that dumps core, or leaves a FATAL ERROR in the log, is backed up into
// ~/error_backup: the cores, a copy of the whole install, and a tarball named for
// the build and the second it was made (runone.sh:185-236). Slots are separate
// machines in every other respect, and two of them crashing in the same second
// would write the same name.
type homeGuard struct {
	// errorBackup is $HOME/error_backup, which each slot gets a directory of its
	// own behind.
	errorBackup string
	// cubridLog is $HOME/CUBRID/log when it exists and is not the log of the
	// install under test; each slot sees an empty directory there instead. Empty
	// when there is nothing to protect.
	cubridLog string
}

func guardsFor(home, cubrid string) homeGuard {
	if home == "" {
		return homeGuard{}
	}
	g := homeGuard{errorBackup: filepath.Join(home, "error_backup")}
	log := filepath.Join(home, "CUBRID", "log")
	if st, err := os.Stat(log); err == nil && st.IsDir() && !sameDir(log, filepath.Join(cubrid, "log")) {
		g.cubridLog = log
	}
	return g
}

func sameDir(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return ra == rb
}

// mount puts the guards in place inside one slot. The slot's view changes and
// the machine's does not: ~/error_backup is created if it is missing, which is
// what runone.sh would have done at the first core.
func (g homeGuard) mount(s *contain.Slot) error {
	if g.errorBackup == "" {
		return nil
	}
	if err := os.MkdirAll(g.errorBackup, 0o755); err != nil {
		return fmt.Errorf("%s: %w", s.Label, err)
	}
	if err := s.NS.Private(g.errorBackup, filepath.Join(s.Dir, "error_backup")); err != nil {
		return err
	}
	if g.cubridLog != "" {
		if err := s.NS.Private(g.cubridLog, filepath.Join(s.Dir, "home-CUBRID-log")); err != nil {
			return err
		}
	}
	return nil
}

// keepBackups copies what one slot backed up into the machine's ~/error_backup,
// where CTP would have left it, before the slot's directory is removed. A name
// that is already there gets the slot's label in front rather than replacing
// what is there. It returns what it kept.
//
// Copied, not moved: the slot root and $HOME are rarely one filesystem.
func keepBackups(from, to, label string) ([]string, error) {
	entries, err := os.ReadDir(from)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var kept []string
	for _, e := range entries {
		dst := filepath.Join(to, e.Name())
		if _, err := os.Lstat(dst); err == nil {
			dst = filepath.Join(to, label+"-"+e.Name())
		}
		if out, err := exec.Command("cp", "-a", filepath.Join(from, e.Name()), dst).CombinedOutput(); err != nil {
			return kept, fmt.Errorf("keep %s: %w: %s", e.Name(), err, out)
		}
		kept = append(kept, dst)
	}
	return kept, nil
}
