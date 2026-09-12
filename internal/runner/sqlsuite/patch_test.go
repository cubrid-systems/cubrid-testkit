package sqlsuite

import (
	"context"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
	"github.com/cubrid-systems/cubrid-testkit/internal/patch"
)

// A sql patch applies in the directory that holds cases/ and answers/, so one
// patch can change a case and the answer it is judged against together -- which
// is what a case that depends on what ran before it needs.
func TestAPatchChangesACaseAndItsAnswerTogether(t *testing.T) {
	scenario := t.TempDir()
	dir := filepath.Join(scenario, "_01_object", "_10_system_table", "_001_db_class")
	caseFile := filepath.Join(dir, "cases", "1003.sql")
	answer := filepath.Join(dir, "answers", "1003.answer")
	write(t, caseFile, "select class_name from db_class;\n")
	write(t, answer, "b\na\n")

	pdir := t.TempDir()
	write(t, filepath.Join(pdir, "_01_object~_10_system_table~_001_db_class~1003.patch"),
		`--- cases/1003.sql
+++ cases/1003.sql
@@ -1 +1 @@
-select class_name from db_class;
+select class_name from db_class order by class_name;
--- answers/1003.answer
+++ answers/1003.answer
@@ -1,2 +1,2 @@
-b
-a
+a
+b
`)

	cases := []string{caseFile}
	p, err := patch.Load(pdir, scenario, ".sql", cases)
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
	if got := read(t, caseFile); got != "select class_name from db_class order by class_name;\n" {
		t.Errorf("the case was not patched: %q", got)
	}
	if got := read(t, answer); got != "a\nb\n" {
		t.Errorf("the answer was not patched: %q", got)
	}
	if len(p.Unapplied()) != 0 {
		t.Errorf("a patch that was applied is reported as unapplied: %v", p.Unapplied())
	}

	// And the corpus comes out as it went in: a run writes into the checkout,
	// and a patch left there is a patch the next run reads from git.
	putPatchesBack(ch, p, cases)
	if got := read(t, caseFile); got != "select class_name from db_class;\n" {
		t.Errorf("the case was left patched: %q", got)
	}
	if got := read(t, answer); got != "b\na\n" {
		t.Errorf("the answer was left patched: %q", got)
	}
}

// A patch that no longer fits stops the run: the case has moved, and running it
// unpatched answers a question nobody asked.
func TestAPatchThatDoesNotFitStopsTheRun(t *testing.T) {
	scenario := t.TempDir()
	caseFile := filepath.Join(scenario, "_01_object", "x", "cases", "x.sql")
	write(t, caseFile, "select 1;\n")
	pdir := t.TempDir()
	write(t, filepath.Join(pdir, "_01_object~x.patch"),
		`--- cases/x.sql
+++ cases/x.sql
@@ -1 +1 @@
-select 2;
+select 3;
`)
	cases := []string{caseFile}
	p, err := patch.Load(pdir, scenario, ".sql", cases)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyPatches(context.Background(), &exec.Local{}, p, cases); err == nil {
		t.Fatal("a patch that does not apply was accepted")
	}
	if got := read(t, caseFile); got != "select 1;\n" {
		t.Errorf("a refused patch changed the case: %q", got)
	}
}

// Every patch this repository ships for the sql family has to apply to the
// corpus as it is now.
//
// The same guard shellsuite has, for the same reason: a patch is a claim about
// a case's source, and upstream moves. When a case is fixed properly the patch
// stops applying and this fails, which is the signal to delete it.
func TestTheShippedSQLPatchesApplyToTheCorpus(t *testing.T) {
	root := os.Getenv("TESTKIT_SQL_CORPUS")
	if root == "" {
		t.Skip("set TESTKIT_SQL_CORPUS to a cubrid-testcases checkout")
	}
	sql := filepath.Join(root, "sql")
	if _, err := os.Stat(sql); err != nil {
		t.Skipf("no sql tree under %s", root)
	}
	pdir := filepath.Join("..", "..", "..", "patches", "sql")
	entries, err := os.ReadDir(pdir)
	if err != nil {
		t.Skip("no patches directory")
	}

	var n int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".patch") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".patch")
		dir := caseDirFor(sql, name)
		if dir == "" {
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

// caseDirFor turns a patch name back into the directory it applies in: the one
// holding cases/ and answers/. Two shapes, because patch.Name drops a file name
// that repeats its directory.
func caseDirFor(sqlRoot, name string) string {
	parts := strings.Split(name, "~")
	last := parts[len(parts)-1]
	deep := filepath.Join(append([]string{sqlRoot}, parts...)...)
	if _, err := os.Stat(filepath.Join(deep, "cases", last+".sql")); err == nil {
		return deep
	}
	shallow := filepath.Join(append([]string{sqlRoot}, parts[:len(parts)-1]...)...)
	if _, err := os.Stat(filepath.Join(shallow, "cases", last+".sql")); err == nil {
		return shallow
	}
	return ""
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
