package result

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/conf"
)

// The isolation module names its archive after itself and packs the directory's
// contents relative to it, so what comes out of it is ./dispatch_tc_ALL.txt and
// not a path to the run directory.
func TestIsolationPacksTheRunDirectoryFromInside(t *testing.T) {
	home := &conf.Home{Path: t.TempDir()}
	s, err := Open(home, "isolation", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.All([]string{"/s/a/a.ctl"}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	name, err := s.BackupIsolation("11.5.0.2574-f1ae86f", "64bits", 0,
		time.Date(2026, 9, 15, 3, 7, 22, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if want := "isolation_result_11.5.0.2574-f1ae86f_64bits_0_2026.9.15_3.7.22.tar.gz"; name != want {
		t.Errorf("name = %q, want %q", name, want)
	}
	archive := filepath.Join(home.Path, "result", "isolation", name)
	out, err := exec.Command("tar", "tzf", archive).CombinedOutput()
	if err != nil {
		t.Fatalf("the archive was not written beside the run directory: %v: %s", err, out)
	}
	if !strings.Contains(string(out), "./dispatch_tc_ALL.txt\n") {
		t.Errorf("the archive holds:\n%s\nwant ./dispatch_tc_ALL.txt", out)
	}
}

func TestBackupPacksTheRunDirectory(t *testing.T) {
	home := &conf.Home{Path: t.TempDir()}
	s, err := Open(home, "shell", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.All([]string{"a/cases/a.sh"}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	name, err := s.Backup("11.3.5.1275-0e31336", "64bits", 0,
		time.Date(2026, 9, 3, 14, 7, 22, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if want := "shell_result_11.3.5.1275-0e31336_64bits_0_2026.9.3_2.7.22.tar.gz"; name != want {
		t.Errorf("name = %q, want %q", name, want)
	}
	if _, err := os.Stat(filepath.Join(home.Path, "result", "shell", name)); err != nil {
		t.Errorf("the archive was not written beside the run directory: %v", err)
	}
}

// Calendar.HOUR is the twelve-hour clock, so 14:07 and 02:07 stamp the same. It
// is reproduced rather than corrected, because the name is a frozen surface --
// but it is worth a test, so that nobody reads the collision as a bug in this
// code and quietly fixes it.
func TestTheStampUsesATwelveHourClockJustAsCTPDid(t *testing.T) {
	home := &conf.Home{Path: t.TempDir()}
	s, err := Open(home, "shell", false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	afternoon, err := s.Backup("b", "64bits", 0, time.Date(2026, 9, 3, 14, 7, 22, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	morning, err := s.Backup("b", "64bits", 0, time.Date(2026, 9, 3, 2, 7, 22, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if afternoon != morning {
		t.Errorf("the two stamps differ (%q vs %q); the clock is no longer CTP's", afternoon, morning)
	}
	if !strings.Contains(afternoon, "_2.7.22.") {
		t.Errorf("got %q, want an hour of 2 for 14:07", afternoon)
	}
}
