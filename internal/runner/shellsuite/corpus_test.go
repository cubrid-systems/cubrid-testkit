package shellsuite

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cubrid-systems/cubrid-testkit/internal/contain"
)

func contained(t *testing.T) {
	t.Helper()
	if os.Getenv("TESTKIT_CONTAINED") != "1" {
		t.Skip("not contained; run under TESTKIT_CONTAIN=1")
	}
}

// caseTree lays out one directory of the shape the suite has, and returns the
// scenario root, the cases directory, and the case paths in it.
func caseTree(t *testing.T, names ...string) (string, string, []string) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "family", "cases")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var cases []string
	for _, n := range names {
		p := filepath.Join(dir, n+".sh")
		if err := os.WriteFile(p, []byte("clean\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, p)
	}
	return root, dir, cases
}

// The corpus is 105 MB in the repository and was 20 GB on this machine, because
// cases create databases in their own directory, not all of them delete them,
// and nothing put the tree back. Behind this overlay a run can read the corpus,
// its writes go to memory, the cap stops a case that never cleans up from
// ending the run, and the tree comes out as it went in.
//
// Contained only: the mounts need CAP_SYS_ADMIN in the runner's user namespace.
func TestTheCorpusIsReadOnlyAndItsWritesAreMemory(t *testing.T) {
	contained(t)
	root, dir, _ := caseTree(t, "a")

	c, err := OpenCorpus(root, 32, false)
	if err != nil {
		t.Fatal(err)
	}
	if c.Ram() == "" {
		t.Error("the overlay did not say where its upper layer is, so the status page cannot report it")
	}

	if b, err := os.ReadFile(filepath.Join(dir, "a.sh")); err != nil ||
		strings.TrimSpace(string(b)) != "clean" {
		t.Errorf("the corpus does not read through the overlay: %q %v", b, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "db_lgat"), make([]byte, 4<<20), 0o644); err != nil {
		t.Fatalf("a run cannot write to its case directory: %v", err)
	}

	// The cap is the reason not to put everything in memory, so it has to bite.
	out, _ := exec.Command("bash", "-c",
		"dd if=/dev/zero of="+dir+"/big bs=1M count=64 2>&1").CombinedOutput()
	if !strings.Contains(string(out), "No space") {
		t.Errorf("the 32 MB ceiling did not stop a 64 MB write: %s", out)
	}

	c.Close()
	if _, err := os.Stat(filepath.Join(dir, "db_lgat")); err == nil {
		t.Error("a run's write survived into the corpus")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "a.sh")); strings.TrimSpace(string(b)) != "clean" {
		t.Error("the corpus did not come out as it went in")
	}
}

// Two of this family's 217 cases never clean up, and they held 813 MB to the end
// of the run. That ratio is what put 20 GB into a corpus that is 105 MB in git,
// and at the same rate the full 3,475 cases leave around 13 GB behind. Retiring
// a directory has to give that back.
func TestRetiringADirectoryGivesTheMemoryBack(t *testing.T) {
	contained(t)
	root, dir, cases := caseTree(t, "a")

	c, err := OpenCorpus(root, 64, false)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.Plan(cases)

	if err := os.WriteFile(filepath.Join(dir, "db_lgat"), make([]byte, 16<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	if used := c.used(); used < 16 {
		t.Fatalf("a 16 MB write left the tmpfs holding %d MB", used)
	}

	c.Retire("slot0", dir)

	if used := c.used(); used > 4 {
		t.Errorf("retiring the directory left %d MB behind", used)
	}
	if _, err := os.Stat(filepath.Join(dir, "db_lgat")); err == nil {
		t.Error("the database the case left behind is still there")
	}
	// And what the corpus itself has must survive, because the cases that run
	// afterwards read it.
	if b, err := os.ReadFile(filepath.Join(dir, "a.sh")); err != nil ||
		strings.TrimSpace(string(b)) != "clean" {
		t.Errorf("the reclaim took a corpus file with it: %q %v", b, err)
	}
}

// 15 of this family's 217 directories hold more than one case. Reclaiming after
// the first would delete the state its sibling is about to look for, so the unit
// is the directory and not the case.
func TestADirectoryIsReclaimedOnlyWhenItsLastCaseRetires(t *testing.T) {
	contained(t)
	root, dir, cases := caseTree(t, "a", "b")

	c, err := OpenCorpus(root, 64, false)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.Plan(cases)

	shared := filepath.Join(dir, "db_lgat")
	if err := os.WriteFile(shared, make([]byte, 8<<20), 0o644); err != nil {
		t.Fatal(err)
	}

	c.Retire("slot0", dir)
	if _, err := os.Stat(shared); err != nil {
		t.Fatalf("the first of two cases took the directory with it: %v", err)
	}

	c.Retire("slot0", dir)
	if _, err := os.Stat(shared); err == nil {
		t.Error("the directory was not reclaimed after its last case")
	}
}

// A case that edits a corpus file has to keep it readable for the cases that
// come after. Removing a name the lower layer has creates a whiteout, which
// would hide the file from the rest of the run -- so a name present in both
// layers is left alone, and the reclaim gives up the kilobytes it holds.
func TestTheReclaimDoesNotWhiteOutTheCorpus(t *testing.T) {
	contained(t)
	root, dir, cases := caseTree(t, "a")

	c, err := OpenCorpus(root, 64, false)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.Plan(cases)

	edited := filepath.Join(dir, "a.sh")
	if err := os.WriteFile(edited, []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.Retire("slot0", dir)

	if _, err := os.Stat(edited); err != nil {
		t.Errorf("a corpus file a case had edited is gone from the run: %v", err)
	}
}

// Nothing is reclaimed until Plan has said what the directories hold, and a run
// with no case list behaves as it did before this existed.
func TestNothingIsReclaimedWithoutAPlan(t *testing.T) {
	contained(t)
	root, dir, _ := caseTree(t, "a")

	c, err := OpenCorpus(root, 64, false)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	left := filepath.Join(dir, "db_lgat")
	if err := os.WriteFile(left, make([]byte, 4<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	c.Retire("slot0", dir)
	if _, err := os.Stat(left); err != nil {
		t.Error("an unplanned directory was reclaimed anyway")
	}
}

// A run that asked for no memory has no corpus, and every method has to survive
// that without the call sites checking.
func TestANilCorpusIsUsable(t *testing.T) {
	var c *Corpus
	c.Plan([]string{"/x/cases/x.sh"})
	c.Retire("slot0", "/x/cases")
	c.Close()
	if c.Ram() != "" {
		t.Error("a nil corpus named an upper layer")
	}
}

// With lanes each slot has an overlay of its own over the corpus, and where its
// upper layer sits is what the lane means. Three things have to hold: the
// corpus reads through in both lanes, a slot's writes are invisible to the
// other, and only the fast lane's writes come out of the ceiling.
func TestEachLanesWritesGoWhereItsLaneSays(t *testing.T) {
	contained(t)
	root, dir, cases := caseTree(t, "a")

	c, err := OpenCorpus(root, 64, true)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.Plan(cases)

	// Two slots, one in each lane, each with a namespace and an overlay.
	type slot struct {
		ns   *contain.Namespace
		name string
	}
	var slots []slot
	for i, onRAM := range []bool{true, false} {
		name := fmt.Sprintf("slot%d", i)
		ns, err := contain.Open(name)
		if err != nil {
			t.Fatal(err)
		}
		defer ns.Close()
		upper, err := c.Slot(name, onRAM, func(script string) error {
			out, err := ns.Channel("").Run(context.Background(), script)
			if err != nil {
				return fmt.Errorf("%v: %s", err, out.Output())
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if onRAM && !strings.HasPrefix(upper, c.Ram()) {
			t.Errorf("the fast lane's upper is at %q, not on the tmpfs at %q", upper, c.Ram())
		}
		if !onRAM && strings.HasPrefix(upper, c.Ram()) {
			t.Errorf("the slow lane's upper is on the tmpfs: %q", upper)
		}
		if err := ns.Overlay(root, upper); err != nil {
			t.Fatal(err)
		}
		slots = append(slots, slot{ns, name})
	}

	sh := func(s slot, script string) string {
		out, err := s.ns.Channel("").Run(context.Background(), script)
		if err != nil {
			t.Fatalf("%s: %s: %v: %s", s.name, script, err, out.Output())
		}
		return strings.TrimSpace(out.Output())
	}

	// The corpus reads through in both lanes.
	for _, s := range slots {
		if got := sh(s, "cat "+filepath.Join(dir, "a.sh")); got != "clean" {
			t.Errorf("%s cannot read the corpus through its overlay: %q", s.name, got)
		}
	}

	// A slot's writes are its own. Two slots writing the same path is exactly
	// what two cases in two lanes do, and neither may see the other.
	sh(slots[0], "dd if=/dev/zero of="+filepath.Join(dir, "db_lgat")+" bs=1M count=8 2>/dev/null")
	sh(slots[1], "echo slow > "+filepath.Join(dir, "db_lgat"))
	if got := sh(slots[1], "cat "+filepath.Join(dir, "db_lgat")); got != "slow" {
		t.Errorf("the slow lane sees the fast lane's write: %q", got)
	}
	if got := sh(slots[0], "stat -c %s "+filepath.Join(dir, "db_lgat")); got != "8388608" {
		t.Errorf("the fast lane's own 8 MB write reads back as %q bytes", got)
	}
	// And neither reached the corpus itself.
	if _, err := os.Stat(filepath.Join(dir, "db_lgat")); err == nil {
		t.Error("a slot's write landed in the corpus")
	}

	// Only the fast lane's writes are in the ceiling.
	if used := c.used(); used < 8 {
		t.Errorf("the fast lane's 8 MB is not in the ceiling: %d MB used", used)
	}

	// And the reclaim goes through the right slot's overlay.
	c.Retire("slot0", dir)
	if got := sh(slots[0], "ls "+dir); strings.Contains(got, "db_lgat") {
		t.Errorf("retiring slot0's directory left its write behind: %q", got)
	}
	if got := sh(slots[1], "cat "+filepath.Join(dir, "db_lgat")); got != "slow" {
		t.Errorf("retiring slot0 took slot1's write with it: %q", got)
	}
	if got := sh(slots[0], "cat "+filepath.Join(dir, "a.sh")); got != "clean" {
		t.Errorf("the reclaim whited out a corpus file: %q", got)
	}
}

// A corpus mounted once for every slot has no per-slot upper, and asking for
// one is a mistake worth an error rather than a surprise later.
func TestASharedCorpusHasNoPerSlotUpper(t *testing.T) {
	contained(t)
	root, _, _ := caseTree(t, "a")
	c, err := OpenCorpus(root, 32, false)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Slot("slot0", true, func(string) error { return nil }); err == nil {
		t.Error("a shared corpus handed out a per-slot upper")
	}
}

// The lane that gives memory away is chosen by footprint, and the footprint has
// to come from somewhere. It comes from here: the megabytes a directory gives
// back when it retires are the megabytes it was holding.
func TestRetiringADirectoryRecordsWhatItHeld(t *testing.T) {
	contained(t)
	root, dir, cases := caseTree(t, "a")

	c, err := OpenCorpus(root, 128, false)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.Plan(cases)

	if len(c.Held()) != 0 {
		t.Error("a directory that has not retired has no measurement yet")
	}
	if err := os.WriteFile(filepath.Join(dir, "db_lgat"), make([]byte, 48<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	c.Retire("slot0", dir)

	held := c.Held()
	got, ok := held[dir]
	if !ok {
		t.Fatalf("no footprint recorded for the directory that just retired: %v", held)
	}
	// Whole-tmpfs statfs in megabytes, so allow a megabyte either way for the
	// overlay's own bookkeeping. The figure has to be the 48 that was written,
	// not the 128 the ceiling allows or the 0 a missed measurement would give.
	if got < 47 || got > 49 {
		t.Errorf("the directory held 48 MB and was recorded at %d", got)
	}

	// Held is a copy: the run writes it out while the corpus may still be
	// sampling, and a map handed out by reference would race.
	held[dir] = 999
	if c.Held()[dir] == 999 {
		t.Error("Held handed out its own map")
	}
}
