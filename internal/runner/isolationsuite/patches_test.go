package isolationsuite

import (
	"context"
	"errors"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
	"github.com/cubrid-systems/cubrid-testkit/internal/patch"
)

// An isolation patch applies in the directory that holds the case and the
// answer/ beside it -- one level up, where shell and sql are two -- so one patch
// can add the wait a case is missing and re-record the answer it is judged
// against together.
func TestAPatchChangesACaseAndItsAnswerTogether(t *testing.T) {
	scenario := t.TempDir()
	dir := filepath.Join(scenario, "_02_RepeatableRead", "index_column", "aggregate")
	caseFile := filepath.Join(dir, "select_select_01.ctl")
	answer := filepath.Join(dir, "answer", "select_select_01.answer")
	writeFile(t, caseFile, "C1: DELETE FROM tb1 WHERE id BETWEEN 10 AND 20;\nC4: DELETE FROM tb1 WHERE id BETWEEN 100 AND 150;\n")
	writeFile(t, answer, "| 11 rows affected\n| 51 rows affected\n")

	pdir := t.TempDir()
	writeFile(t, filepath.Join(pdir, "_02_RepeatableRead~index_column~aggregate~select_select_01.patch"),
		`--- select_select_01.ctl
+++ select_select_01.ctl
@@ -1,2 +1,3 @@
 C1: DELETE FROM tb1 WHERE id BETWEEN 10 AND 20;
+MC: wait until C1 ready;
 C4: DELETE FROM tb1 WHERE id BETWEEN 100 AND 150;
`)

	cases := []string{caseFile}
	p, err := patch.Load(pdir, scenario, ".ctl", cases)
	if err != nil {
		t.Fatal(err)
	}
	if p.Count() != 1 {
		t.Fatalf("the patch was not matched to the case: %d", p.Count())
	}

	ch := &exec.Local{}
	if err := applyPatches(context.Background(), ch, p, cases); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got := readFile(t, caseFile); !strings.Contains(got, "MC: wait until C1 ready;") {
		t.Errorf("the case was not patched: %q", got)
	}
	if len(p.Unapplied()) != 0 {
		t.Errorf("a patch that was applied is reported as unapplied: %v", p.Unapplied())
	}

	// And the corpus comes out as it went in. Isolation has no overlay over the
	// cases tree unless scenario_disk asks for one -- runone.sh writes into the
	// checkout, as under CTP -- so this revert is the only thing that puts it
	// back, not a second line of defence.
	putPatchesBack(ch, p, cases)
	if got := readFile(t, caseFile); strings.Contains(got, "MC: wait until C1 ready;") {
		t.Errorf("the case was left patched: %q", got)
	}
}

// A patch that no longer fits stops the run: the case has moved, and running it
// unpatched answers a question nobody asked. Here that is more literal than
// elsewhere -- the question is whether the case orders what it prints, and the
// patch is the ordering.
func TestAPatchThatDoesNotFitStopsTheRun(t *testing.T) {
	scenario := t.TempDir()
	caseFile := filepath.Join(scenario, "_01_ReadCommitted", "catalog", "db_index_key_04.ctl")
	writeFile(t, caseFile, "MC: wait until C2 ready;\n")
	pdir := t.TempDir()
	writeFile(t, filepath.Join(pdir, "_01_ReadCommitted~catalog~db_index_key_04.patch"),
		`--- db_index_key_04.ctl
+++ db_index_key_04.ctl
@@ -1 +1 @@
-MC: wait until C3 ready;
+MC: wait until C2 ready;
`)
	cases := []string{caseFile}
	p, err := patch.Load(pdir, scenario, ".ctl", cases)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyPatches(context.Background(), &exec.Local{}, p, cases); err == nil {
		t.Fatal("a patch that does not apply was accepted")
	}
	if got := readFile(t, caseFile); got != "MC: wait until C2 ready;\n" {
		t.Errorf("a refused patch changed the case: %q", got)
	}
}

// Every patch this repository ships for isolation has to apply to the corpus as
// it is now.
//
// The same guard shellsuite and sqlsuite have, for the same reason: a patch is a
// claim about a case's source, and upstream moves. When a case is fixed properly
// the patch stops applying and this fails, which is the signal to delete it --
// and for isolation it is also the signal that the upstream track has landed.
func TestTheShippedIsolationPatchesApplyToTheCorpus(t *testing.T) {
	root := os.Getenv("TESTKIT_ISOLATION_CORPUS")
	if root == "" {
		t.Skip("set TESTKIT_ISOLATION_CORPUS to a cubrid-testcases checkout")
	}
	corpus := filepath.Join(root, "isolation")
	if _, err := os.Stat(corpus); err != nil {
		corpus = root
	}
	pdir, perr := patch.Shipped("isolation")
	if errors.Is(perr, patch.ErrNoPatchSet) {
		t.Skip("set TESTKIT_PATCHES to a cubrid-testkit-patches checkout")
	}
	if perr != nil {
		t.Fatal(perr)
	}
	entries, err := os.ReadDir(pdir)
	if err != nil {
		t.Fatalf("no patch directory at %s: %v", pdir, err)
	}

	var n int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".patch") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".patch")
		dir := filepath.Join(append([]string{corpus}, strings.Split(name, "~")...)...)
		// A case is a file in the directory the patch applies in, so the name's
		// last segment is the case and the rest is the directory.
		dir = filepath.Dir(dir)
		if _, serr := os.Stat(dir); serr != nil {
			t.Errorf("%s patches a case that is not in the corpus", e.Name())
			continue
		}
		n++
		// Applied to a copy: the corpus is a checkout, and a test does not get
		// to write to it.
		work := t.TempDir()
		if out, cerr := osexec.Command("cp", "-a", dir+"/.", work).CombinedOutput(); cerr != nil {
			t.Fatalf("cannot copy %s: %v: %s", dir, cerr, out)
		}
		abs, _ := filepath.Abs(filepath.Join(pdir, e.Name()))
		if out, aerr := osexec.Command("bash", "-c", patch.ApplyScript(work, abs)).CombinedOutput(); aerr != nil {
			t.Errorf("%s no longer applies -- the case has probably been fixed upstream, "+
				"so delete the patch:\n%s", e.Name(), out)
		}
	}
	t.Logf("%d patches apply", n)
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
