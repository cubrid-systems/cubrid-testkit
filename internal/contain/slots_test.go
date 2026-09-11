package contain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A run's slots start from the install, not from what the last run's slots wrote
// into it. They did not while the slot root was named after the pid: the root
// outlived its run, the pid recurred -- contained, it is the namespace's -- and
// the next run's overlay went over the old upper layer.
func TestASlotDoesNotInheritTheLastRunsWrites(t *testing.T) {
	if !Active() {
		t.Skipf("not contained; run under %s=1", Env)
	}
	base, err := os.MkdirTemp("/var/tmp", "testkit-slotroot-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(base) })
	t.Setenv(SlotRootEnv, base)
	t.Setenv("CUBRID", t.TempDir())
	t.Setenv("CUBRID_DATABASES", "")

	run := func(script string) string {
		t.Helper()
		slots, closeAll, err := OpenSlots(1, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer closeAll()
		res, err := slots[0].Channel().Run(t.Context(), script)
		if err != nil || res.ExitCode != 0 {
			t.Fatalf("%q: exit %d, %v: %s", script, res.ExitCode, err, res.Stderr)
		}
		return strings.TrimSpace(res.Output())
	}

	run(`echo written > "$CUBRID/leftover"`)
	if entries, _ := os.ReadDir(base); len(entries) != 0 {
		t.Errorf("the slot root outlived its run: %v", entries)
	}
	if got := run(`[ -e "$CUBRID/leftover" ] && echo inherited || echo clean`); got != "clean" {
		t.Error("the second run's slot started with the first run's write in its install")
	}
}

// A runner's own mount goes in after the install's overlay and before anything
// runs in the slot, and a slot whose mount fails is not handed out.
func TestAMountHookRunsInsideEachSlot(t *testing.T) {
	if !Active() {
		t.Skipf("not contained; run under %s=1", Env)
	}
	t.Setenv("CUBRID", t.TempDir())
	t.Setenv("CUBRID_DATABASES", "")
	var seen []string
	slots, closeAll, err := OpenSlots(2, func(i int, s *Slot) error {
		seen = append(seen, s.Label)
		// Where a hook puts an overlay's upper: the slot's own directory, which
		// its install's upper is already in.
		if st, err := os.Stat(filepath.Join(s.Dir, filepath.Base(os.Getenv("CUBRID")), "upper")); err != nil || !st.IsDir() {
			t.Errorf("%s: its Dir %q does not hold its install's upper layer: %v", s.Label, s.Dir, err)
		}
		return s.NS.Private("/mnt", t.TempDir())
	})
	if err != nil {
		t.Fatal(err)
	}
	defer closeAll()
	if strings.Join(seen, ",") != "slot0,slot1" {
		t.Errorf("the hook saw %v, want each slot once, in order", seen)
	}
	res, err := slots[1].Channel().Run(t.Context(), `touch /mnt/mine && echo "$CUBRID_TMP"`)
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("exit %d, %v: %s", res.ExitCode, err, res.Stderr)
	}
	if _, err := os.Stat("/mnt/mine"); err == nil {
		t.Error("a file written to the slot's own mount appeared outside it")
	}
	if got := strings.TrimSpace(res.Output()); got == "" {
		t.Error("the slot's commands have no CUBRID_TMP")
	}

	if _, _, err := OpenSlots(1, func(int, *Slot) error { return os.ErrPermission }); err == nil {
		t.Error("a slot whose mount failed was handed out")
	}
}

// The registry gets an overlay of its own only when it is outside the install.
// CUBRID's own default puts it at $CUBRID/databases -- and CTP's reset cleans
// and restores it there -- in which case the install's overlay already covers
// it and a second one would nest overlayfs on overlayfs for nothing.
func TestTheRegistryIsCoveredByTheInstallWhenItIsInsideIt(t *testing.T) {
	for _, c := range []struct {
		name    string
		parents []string
		dir     string
		want    bool
	}{
		{"CUBRID's default layout", []string{"/opt/CUBRID"}, "/opt/CUBRID/databases", true},
		{"the same directory", []string{"/opt/CUBRID"}, "/opt/CUBRID", true},
		{"a registry kept outside", []string{"/opt/CUBRID"}, "/var/db/registry", false},
		{"a sibling, not a child", []string{"/opt/CUBRID"}, "/opt/CUBRID-old", false},
		// The reason this is not a string prefix test: "/a/bc" starts with
		// "/a/b" and is not inside it. A prefix check would skip a real overlay
		// and the slot would share the machine's registry with every other one.
		{"a name that merely starts the same", []string{"/a/b"}, "/a/bc", false},
		{"a path that climbs back out", []string{"/opt/CUBRID"}, "/opt/CUBRID/../other", false},
		{"nothing covered yet", nil, "/opt/CUBRID", false},
		{"deeper inside", []string{"/opt/CUBRID"}, "/opt/CUBRID/databases/x/y", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := under(c.parents, c.dir); got != c.want {
				t.Errorf("under(%q, %q) = %v, want %v", c.parents, c.dir, got, c.want)
			}
		})
	}
}

// The socket lives under $CUBRID whenever that fits, because the cases normalise
// their output against $CUBRID and a path outside it is a path their sed does
// not rewrite.
//
// $CUBRID/tmp and not $CUBRID/var/CUBRID_SOCK, which is where the engine itself
// puts it when CUBRID_TMP says nothing: the per-case reset runs
// `rm -rf ${CUBRID}/var/*`, so a socket directory there does not survive the
// first case. Measured -- a two-case run went from 26 seconds to 426 with both
// failing -- so this assertion is load-bearing rather than arbitrary.
func TestSlotTmpPrefersTheShippedPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CUBRID", home)
	got, err := slotTmp("slot0")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "tmp"); got != want {
		t.Fatalf("CUBRID_TMP is %s, want %s", got, want)
	}
	if _, err := os.Stat(got); err != nil {
		t.Errorf("the directory was not made: %v", err)
	}
}

// And falls back when the install is deep enough that a socket under it would
// not fit in sun_path -- which is a real engine error, not a theory.
func TestSlotTmpFallsBackWhenThePathWouldNotFit(t *testing.T) {
	deep := filepath.Join(t.TempDir(), strings.Repeat("d", 60), strings.Repeat("e", 60))
	t.Setenv("CUBRID", deep)
	got, err := slotTmp("slot0")
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(got, deep) {
		t.Fatalf("a socket under %s cannot fit in %d bytes, and CUBRID_TMP went there anyway", got, sunPathMax)
	}
	if !strings.HasPrefix(got, "/var/tmp/") {
		t.Errorf("the fallback should be the bounded one: %s", got)
	}
	if n := len(got) + len("/CUBRID65535") + 1; n > sunPathMax {
		t.Errorf("the fallback is %d bytes with the socket, over the %d available", n, sunPathMax)
	}
}
