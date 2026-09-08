package shellsuite

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The corpus is 105 MB in the repository and was 20 GB on this machine, because
// cases create databases in their own directory, not all of them delete them,
// and nothing put the tree back. Behind this overlay a run can read the corpus,
// its writes go to memory, the cap stops a case that never cleans up from
// ending the run, and the tree comes out as it went in.
//
// Contained only: the mounts need CAP_SYS_ADMIN in the runner's user namespace.
func TestTheCorpusIsReadOnlyAndItsWritesAreMemory(t *testing.T) {
	if os.Getenv("TESTKIT_CONTAINED") != "1" {
		t.Skip("not contained; run under TESTKIT_CONTAIN=1")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "case.sh"), []byte("clean\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	undo, ramDir, err := ramOverlay(dir, 32)
	if err != nil {
		t.Fatal(err)
	}
	if ramDir == "" {
		t.Error("the overlay did not say where its upper layer is, so the status page cannot report it")
	}

	if b, err := os.ReadFile(filepath.Join(dir, "case.sh")); err != nil ||
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

	undo()
	if _, err := os.Stat(filepath.Join(dir, "db_lgat")); err == nil {
		t.Error("a run's write survived into the corpus")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "case.sh")); strings.TrimSpace(string(b)) != "clean" {
		t.Error("the corpus did not come out as it went in")
	}
}
