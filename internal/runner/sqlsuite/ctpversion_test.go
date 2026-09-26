package sqlsuite

import (
	"strings"
	"testing"
)

// javac reports a missing class. It does not report that the class is missing
// because the CTP checkout predates it, and a reader with an older tree sees two
// lines of Java and no mention of the tree those lines are about -- which cost
// half an hour on a checkout sitting on a feature branch.
func TestAMissingSymbolNamesTheCTPRevision(t *testing.T) {
	real := `TestkitExecutor.java:139: error: cannot find symbol
        Method failure = JunitXmlWriter.class.getDeclaredMethod("buildFailureCdata", CaseResult.class, String.class);
                         ^
  symbol:   class JunitXmlWriter
  location: class TestkitExecutor
2 errors`
	hint := ctpTooOld(real)
	if hint == "" {
		t.Fatal("a missing JunitXmlWriter should point at the CTP revision")
	}
	for _, want := range []string{"CUBRIDQA-1406", "develop", "older than the"} {
		if !strings.Contains(hint, want) {
			t.Errorf("the hint should mention %q; got %q", want, hint)
		}
	}
}

// A failure this file knows nothing about gets no hint. Guessing which revision
// a symbol arrived in would be worse than the silence it replaces.
func TestAnUnknownFailureGetsNoHint(t *testing.T) {
	if got := ctpTooOld("TestkitExecutor.java:12: error: ';' expected"); got != "" {
		t.Errorf("want no hint for an unrelated failure, got %q", got)
	}
	if got := ctpTooOld(""); got != "" {
		t.Errorf("want no hint for empty output, got %q", got)
	}
}
