package shellsuite

import (
	"context"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
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
	writeFile(t, filepath.Join(pdir, "_08_shard~x.patch"), "")
	writeFile(t, filepath.Join(pdir, "_99_gone~y.patch"), "")

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

// The name drops the two segments that carry no information.
func TestPatchNameDropsWhatCarriesNoInformation(t *testing.T) {
	for _, c := range []struct{ script, want string }{
		{"/c/_06_issues/_17_1h/cbrd_20760_1/cases/cbrd_20760_1.sh", "_06_issues~_17_1h~cbrd_20760_1"},
		{"/c/_08_shard/_02_cubrid_broker01/cases/_02_cubrid_broker01.sh", "_08_shard~_02_cubrid_broker01"},
		// A directory with more than one case keeps both names, because there
		// the file name is the only thing telling them apart.
		{"/c/_06_issues/_25_1h/cbrd_24741/01_basic/cases/01_basic.sh", "_06_issues~_25_1h~cbrd_24741~01_basic"},
		{"/elsewhere/x.sh", ""},
	} {
		if got := PatchName("/c", c.script); got != c.want {
			t.Errorf("PatchName(%s) = %q, want %q", c.script, got, c.want)
		}
	}
}

// The corpus has to come out as it went in whether or not the run had an
// overlay, so the revert is explicit rather than a side effect of the reclaim.
func TestTheCaseIsPutBack(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "x.sh")
	const original = "echo hi\n"
	writeFile(t, script, original)
	patch := filepath.Join(dir, "p.patch")
	writeFile(t, patch, `--- x.sh
+++ x.sh
@@ -1 +1 @@
-echo hi
+echo hello
`)
	if out, err := osexec.Command("bash", "-c", ApplyScript(dir, patch)).CombinedOutput(); err != nil {
		t.Fatalf("apply: %v: %s", err, out)
	}
	if b, _ := os.ReadFile(script); string(b) == original {
		t.Fatal("the patch did not change the case")
	}
	if out, err := osexec.Command("bash", "-c", RevertScript(dir, patch)).CombinedOutput(); err != nil {
		t.Fatalf("revert: %v: %s", err, out)
	}
	if b, _ := os.ReadFile(script); string(b) != original {
		t.Errorf("the case was not put back:\n%q", b)
	}
	// A second revert has nothing to undo and must not re-apply it backwards.
	_ = osexec.Command("bash", "-c", RevertScript(dir, patch)).Run()
	if b, _ := os.ReadFile(script); string(b) != original {
		t.Errorf("reverting twice changed the case:\n%q", b)
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
	if err := osexec.Command("bash", "-c", ApplyScript(dir, patch)).Run(); err == nil {
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
		if out, err := osexec.Command("bash", "-c", ApplyScript(dir, patch)).CombinedOutput(); err != nil && i == 0 {
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
		caseScript := scriptFor(shell, strings.TrimSuffix(rel, ".patch"))
		if _, serr := os.Stat(caseScript); serr != nil {
			t.Errorf("%s patches a case that is not in the corpus: %s", rel, caseScript)
			return nil
		}
		n++

		// Applied to a copy, because the corpus is a checkout and a test does
		// not get to write to it.
		work := t.TempDir()
		src := filepath.Dir(caseScript)
		if out, cerr := osexec.Command("cp", "-a", src+"/.", work).CombinedOutput(); cerr != nil {
			t.Fatalf("cannot copy %s: %v: %s", src, cerr, out)
		}
		abs, _ := filepath.Abs(path)
		if out, aerr := osexec.Command("bash", "-c", ApplyScript(work, abs)).CombinedOutput(); aerr != nil {
			t.Errorf("%s no longer applies -- the case has probably been fixed upstream, "+
				"so delete the patch:\n%s", rel, out)
			return nil
		}
		// And the result has to be a shell script that parses, since the whole
		// point is that it runs.
		patched := filepath.Join(work, filepath.Base(caseScript))
		if out, berr := osexec.Command("bash", "-n", patched).CombinedOutput(); berr != nil {
			t.Errorf("%s produces a script bash will not parse: %v: %s", rel, berr, out)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d patches apply", n)
}

// scriptFor turns a patch name back into the case it patches, for the test that
// checks the shipped patches against a real checkout.
func scriptFor(shell, name string) string {
	parts := strings.Split(name, "~")
	last := parts[len(parts)-1]
	// The generated name drops a file that repeats its directory, so try that
	// shape first and the two-case shape second.
	a := filepath.Join(append(append([]string{shell}, parts...), "cases", last+".sh")...)
	if _, err := os.Stat(a); err == nil {
		return a
	}
	return filepath.Join(append(append([]string{shell}, parts[:len(parts)-1]...), "cases", last+".sh")...)
}

// The lookup key has to be the key the index was built with.
//
// It was not: LoadPatches indexes by the case's whole path and the worker asked
// with Case.Script, which Split returns as only the part after cases/. Every
// patch matched at startup, was announced, and was then never applied -- the
// run said it had patched a case it had not.
func TestTheLookupUsesTheKeyTheIndexWasBuiltWith(t *testing.T) {
	scenario := t.TempDir()
	pdir := t.TempDir()
	script := filepath.Join(scenario, "_06_issues", "_11_1h", "bug_bts_5106", "cases", "bug_bts_5106.sh")
	writeFile(t, script, "echo hi\n")
	writeFile(t, filepath.Join(pdir, "_06_issues~_11_1h~bug_bts_5106.patch"), "")

	p, err := LoadPatches(pdir, scenario, []string{script})
	if err != nil {
		t.Fatal(err)
	}
	c, err := Split(script)
	if err != nil {
		t.Fatal(err)
	}
	if p.For(c.Path) == "" {
		t.Error("the worker looks a patch up by Case.Path, and that must be what the index holds")
	}
	if c.Script == c.Path {
		t.Fatal("this test is meaningless if Script and Path are the same")
	}
	if p.For(c.Script) != "" {
		t.Error("Case.Script is a bare file name; it must not match, so the mistake cannot come back quietly")
	}
}

// What a run intended to patch and what it did are not the same thing, and a
// reader of finished results has only the files: feedback.log keeps a case's
// console output for failures only, so an OK case that ran patched leaves no
// trace there at all.
func TestTheRunRecordsWhatItActuallyPatched(t *testing.T) {
	dir := t.TempDir()
	p := &Patches{dir: "patches/shell"}

	// Nothing applied, nothing written: the file's presence is itself the
	// answer to "did this run patch anything".
	if err := p.Report(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "patched.txt")); err == nil {
		t.Fatal("a run that patched nothing should not leave a record saying it did")
	}

	p.Applied("/c/shell/_08_shard/x/cases/x.sh", "patches/shell/_08_shard~x.patch")
	p.Applied("/c/shell/_06_issues/y/cases/y.sh", "patches/shell/_06_issues~y.patch")
	if err := p.Report(dir); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "patched.txt"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, want := range []string{"_08_shard/x/cases/x.sh", "_06_issues~y.patch", "verdicts are about the patched case"} {
		if !strings.Contains(text, want) {
			t.Errorf("the record should carry %q:\n%s", want, text)
		}
	}
	// Sorted, so two runs of the same corpus produce a file that diffs cleanly.
	if strings.Index(text, "_06_issues/y") > strings.Index(text, "_08_shard/x") {
		t.Error("the record should be sorted")
	}
}

// A command's own exit status and the runner's ability to run it are different
// facts, and Run keeps them apart: a non-zero exit comes back in the Result with
// a nil error. Checking only the error therefore gets both cases wrong -- it
// calls a transport failure "the case changed upstream", and it calls a patch
// that really did not apply a success, running the case unpatched while the run
// claims it was patched.
func TestAFailedPatchAndAFailedCommandAreDifferent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "x.sh"), "this is not what the patch expects\n")
	patch := filepath.Join(dir, "p.patch")
	writeFile(t, patch, `--- x.sh
+++ x.sh
@@ -1 +1 @@
-echo hi
+echo hello
`)
	ch := &exec.Local{}
	res, err := ch.Run(context.Background(), ApplyScript(dir, patch))
	if err != nil {
		t.Fatalf("the command ran, so err must be nil; the failure belongs in the Result: %v", err)
	}
	if res.ExitCode == 0 {
		t.Fatal("a patch that does not apply must leave a non-zero exit code in the Result")
	}
}

// A patch that matched and never ran must be named. A run announced "6 cases
// will run against a compatibility patch", applied none, and said nothing: the
// verdicts were about the unpatched corpus and patched.txt was simply absent,
// which reads as "no patches were configured".
func TestAPatchThatMatchedAndDidNotRunIsNamed(t *testing.T) {
	p := &Patches{dir: "/p", byCase: map[string]string{
		"/c/a/cases/a.sh": "/p/a.patch",
		"/c/b/cases/b.sh": "/p/b.patch",
	}}
	if got := p.Unapplied(); len(got) != 2 {
		t.Fatalf("nothing applied and %d reported unapplied", len(got))
	}
	p.Applied("/c/a/cases/a.sh", "/p/a.patch")
	got := p.Unapplied()
	if len(got) != 1 || got[0] != "/c/b/cases/b.sh" {
		t.Errorf("after applying one, unapplied is %v", got)
	}
	p.Applied("/c/b/cases/b.sh", "/p/b.patch")
	if got := p.Unapplied(); len(got) != 0 {
		t.Errorf("everything applied and %v is still reported", got)
	}
	var nilp *Patches
	if got := nilp.Unapplied(); got != nil {
		t.Errorf("a nil Patches reported %v", got)
	}
}
