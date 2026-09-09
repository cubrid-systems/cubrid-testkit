package shellsuite

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The patch for a case is found by the case's own path, so a reader looking for
// what was changed looks in the same place twice.
func TestAPatchIsFoundByTheCasePath(t *testing.T) {
	scenario := t.TempDir()
	pdir := t.TempDir()
	script := filepath.Join(scenario, "_08_shard", "x", "cases", "x.sh")
	writeFile(t, script, "echo hi\n")
	writeFile(t, filepath.Join(pdir, "_08_shard", "x", "cases", "x.sh.patch"), "")
	writeFile(t, filepath.Join(pdir, "_99_gone", "y", "cases", "y.sh.patch"), "")

	p, err := LoadPatches(pdir, scenario, []string{script})
	if err != nil {
		t.Fatal(err)
	}
	if got := p.For(script); got == "" {
		t.Fatal("the patch beside the case was not found")
	}
	if p.Count() != 1 {
		t.Errorf("count %d, want 1", p.Count())
	}
	// A patch with no case is reported, not fatal: a corpus filtered to one
	// family should not have to carry every patch.
	if len(p.orphans) != 1 || !strings.Contains(p.orphans[0], "_99_gone") {
		t.Errorf("the orphan should be named: %v", p.orphans)
	}
	said := strings.Join(p.Describe(), "\n")
	if !strings.Contains(said, "not the corpus") {
		t.Error("the run must say the verdicts are about the patched case")
	}
	if !strings.Contains(said, "_99_gone") {
		t.Error("the orphan should appear in what the run prints")
	}
}

func TestNoPatchDirectoryIsTheOrdinaryCase(t *testing.T) {
	p, err := LoadPatches("", "/x", nil)
	if err != nil || p != nil {
		t.Fatalf("an unset case_patch_dir must be nothing at all: %v %v", p, err)
	}
	if p.Count() != 0 || p.For("/x/y.sh") != "" || p.Describe() != nil {
		t.Error("a nil Patches has to be usable")
	}
}

// A patch that no longer fits means the case moved, and the run must refuse
// rather than quietly answer a question nobody asked.
func TestApplyRefusesAPatchThatDoesNotFit(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "x.sh"), "this is not what the patch expects\n")
	patch := filepath.Join(dir, "p.patch")
	writeFile(t, patch, `--- x.sh
+++ x.sh
@@ -1 +1 @@
-echo hi
+echo hello
`)
	before, _ := os.ReadFile(filepath.Join(dir, "x.sh"))
	if err := exec.Command("bash", "-c", ApplyScript(dir, patch)).Run(); err == nil {
		t.Fatal("a patch that does not apply must fail")
	}
	after, _ := os.ReadFile(filepath.Join(dir, "x.sh"))
	if string(before) != string(after) {
		t.Errorf("a refused patch changed the file anyway:\n%s", after)
	}
	// And the reject files a failed patch would leave behind are not wanted in
	// a case directory that is about to be compared against answers.
	for _, junk := range []string{"x.sh.orig", "x.sh.rej"} {
		if _, err := os.Stat(filepath.Join(dir, junk)); err == nil {
			t.Errorf("%s was left behind", junk)
		}
	}
}

func TestApplyIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "x.sh"), "echo hi\n")
	patch := filepath.Join(dir, "p.patch")
	writeFile(t, patch, `--- x.sh
+++ x.sh
@@ -1 +1 @@
-echo hi
+echo hello
`)
	for i := 0; i < 2; i++ {
		if out, err := exec.Command("bash", "-c", ApplyScript(dir, patch)).CombinedOutput(); err != nil && i == 0 {
			t.Fatalf("first apply failed: %v: %s", err, out)
		}
	}
	got, _ := os.ReadFile(filepath.Join(dir, "x.sh"))
	if strings.TrimSpace(string(got)) != "echo hello" {
		t.Errorf("applying twice should not reverse it: %q", got)
	}
}

// Every patch this repository ships has to apply to the corpus as it is now.
//
// This is the test that keeps them honest. A patch is a claim about a case's
// source, and upstream moves; when a case is fixed properly the patch stops
// applying and this fails, which is the signal to delete it rather than to
// carry a change that no longer describes anything.
func TestTheShippedPatchesApplyToTheCorpus(t *testing.T) {
	root := os.Getenv("TESTKIT_SHELL_CORPUS")
	if root == "" {
		t.Skip("set TESTKIT_SHELL_CORPUS to a cubrid-testcases-private-ex checkout")
	}
	shell := filepath.Join(root, "shell")
	if _, err := os.Stat(shell); err != nil {
		t.Skipf("no shell tree under %s", root)
	}
	pdir := filepath.Join("..", "..", "..", "patches", "shell")
	if _, err := os.Stat(pdir); err != nil {
		t.Skip("no patches directory")
	}

	var n int
	err := filepath.Walk(pdir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".patch") {
			return err
		}
		rel, _ := filepath.Rel(pdir, path)
		caseScript := filepath.Join(shell, strings.TrimSuffix(rel, ".patch"))
		if _, serr := os.Stat(caseScript); serr != nil {
			t.Errorf("%s patches a case that is not in the corpus: %s", rel, caseScript)
			return nil
		}
		n++

		// Applied to a copy, because the corpus is a checkout and a test does
		// not get to write to it.
		work := t.TempDir()
		src := filepath.Dir(caseScript)
		if out, cerr := exec.Command("cp", "-a", src+"/.", work).CombinedOutput(); cerr != nil {
			t.Fatalf("cannot copy %s: %v: %s", src, cerr, out)
		}
		abs, _ := filepath.Abs(path)
		if out, aerr := exec.Command("bash", "-c", ApplyScript(work, abs)).CombinedOutput(); aerr != nil {
			t.Errorf("%s no longer applies -- the case has probably been fixed upstream, "+
				"so delete the patch:\n%s", rel, out)
			return nil
		}
		// And the result has to be a shell script that parses, since the whole
		// point is that it runs.
		patched := filepath.Join(work, filepath.Base(caseScript))
		if out, berr := exec.Command("bash", "-n", patched).CombinedOutput(); berr != nil {
			t.Errorf("%s produces a script bash will not parse: %v: %s", rel, berr, out)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d patches apply", n)
}
