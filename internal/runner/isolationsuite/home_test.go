package isolationsuite

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestOnlySomebodyElsesCUBRIDLogIsHidden(t *testing.T) {
	home := t.TempDir()
	if g := guardsFor(home, "/opt/cubrid"); g.cubridLog != "" || g.errorBackup != filepath.Join(home, "error_backup") {
		t.Errorf("no ~/CUBRID at all: %+v", g)
	}

	if err := os.MkdirAll(filepath.Join(home, "CUBRID", "log"), 0o755); err != nil {
		t.Fatal(err)
	}
	// The install under test is ~/CUBRID: its log is behind the slot's overlay
	// already, and hiding it would hide the log runone.sh greps for FATAL ERROR.
	if g := guardsFor(home, filepath.Join(home, "CUBRID")); g.cubridLog != "" {
		t.Errorf("the install's own log was hidden: %+v", g)
	}
	// Through a symlink it is still the same directory.
	link := filepath.Join(t.TempDir(), "cubrid")
	if err := os.Symlink(filepath.Join(home, "CUBRID"), link); err != nil {
		t.Fatal(err)
	}
	if g := guardsFor(home, link); g.cubridLog != "" {
		t.Errorf("the install's own log, reached through a link, was hidden: %+v", g)
	}
	// Anything else under test, and ~/CUBRID/log belongs to somebody else.
	if g := guardsFor(home, "/data/sandbox/CUBRID"); g.cubridLog != filepath.Join(home, "CUBRID", "log") {
		t.Errorf("another install's log was left exposed: %+v", g)
	}
	if g := guardsFor("", "/data/sandbox/CUBRID"); g != (homeGuard{}) {
		t.Errorf("no HOME, yet guards: %+v", g)
	}
}

func TestBackupsAreKeptWithoutReplacingEachOther(t *testing.T) {
	to := t.TempDir()
	slot0, slot1 := t.TempDir(), t.TempDir()
	name := "error_11.5.0.2574-f1ae86f_20260915043000.tar.gz"
	for _, d := range []string{slot0, slot1} {
		if err := os.WriteFile(filepath.Join(d, name), []byte(d), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a, err := keepBackups(slot0, to, "slot0")
	if err != nil {
		t.Fatal(err)
	}
	b, err := keepBackups(slot1, to, "slot1")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(to, name), filepath.Join(to, "slot1-"+name)}
	if got := append(a, b...); !slices.Equal(got, want) {
		t.Errorf("kept %q, want %q", got, want)
	}
	if body, _ := os.ReadFile(filepath.Join(to, name)); string(body) != slot0 {
		t.Errorf("the first backup was replaced: %q", body)
	}
	if kept, err := keepBackups(filepath.Join(to, "nowhere"), to, "slot2"); err != nil || kept != nil {
		t.Errorf("a slot that backed nothing up: %q, %v", kept, err)
	}
}
