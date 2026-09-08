package shellsuite

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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

	c, err := OpenCorpus(root, 32)
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

	c, err := OpenCorpus(root, 64)
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

	c.Retire(dir)

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

	c, err := OpenCorpus(root, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.Plan(cases)

	shared := filepath.Join(dir, "db_lgat")
	if err := os.WriteFile(shared, make([]byte, 8<<20), 0o644); err != nil {
		t.Fatal(err)
	}

	c.Retire(dir)
	if _, err := os.Stat(shared); err != nil {
		t.Fatalf("the first of two cases took the directory with it: %v", err)
	}

	c.Retire(dir)
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

	c, err := OpenCorpus(root, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.Plan(cases)

	edited := filepath.Join(dir, "a.sh")
	if err := os.WriteFile(edited, []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.Retire(dir)

	if _, err := os.Stat(edited); err != nil {
		t.Errorf("a corpus file a case had edited is gone from the run: %v", err)
	}
}

// Nothing is reclaimed until Plan has said what the directories hold, and a run
// with no case list behaves as it did before this existed.
func TestNothingIsReclaimedWithoutAPlan(t *testing.T) {
	contained(t)
	root, dir, _ := caseTree(t, "a")

	c, err := OpenCorpus(root, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	left := filepath.Join(dir, "db_lgat")
	if err := os.WriteFile(left, make([]byte, 4<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	c.Retire(dir)
	if _, err := os.Stat(left); err != nil {
		t.Error("an unplanned directory was reclaimed anyway")
	}
}

// A run that asked for no memory has no corpus, and every method has to survive
// that without the call sites checking.
func TestANilCorpusIsUsable(t *testing.T) {
	var c *Corpus
	c.Plan([]string{"/x/cases/x.sh"})
	c.Retire("/x/cases")
	c.Close()
	if c.Ram() != "" {
		t.Error("a nil corpus named an upper layer")
	}
}
