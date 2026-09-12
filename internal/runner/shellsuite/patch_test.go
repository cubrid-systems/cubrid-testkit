package shellsuite

import (
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cubrid-systems/cubrid-testkit/internal/patch"
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

	p, err := patch.Load(pdir, scenario, ".sh", []string{script})
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
		if out, aerr := osexec.Command("bash", "-c", patch.ApplyScript(work, abs)).CombinedOutput(); aerr != nil {
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
