package sqlsuite

import "testing"

// CQT takes every \r and \n out of both sides before comparing, wherever they
// are -- so two renderings that differ only in where their lines break are the
// same verdict, and one that differs in anything else is not.
func TestMatchesIsCQTsComparison(t *testing.T) {
	for _, c := range []struct {
		name             string
		rendered, answer string
		want             bool
	}{
		{"identical", "a\nb\n", "a\nb\n", true},
		{"CRLF answer", "a\nb\n", "a\r\nb\r\n", true},
		{"a break moved", "ab\n", "a\nb", true},
		{"a trailing newline", "a\nb", "a\nb\n\n", true},
		{"a space is not a break", "a b", "ab", false},
		{"different text", "a\n1\n", "a\n2\n", false},
		{"empty against empty", "", "\n", true},
	} {
		if got := matches([]byte(c.rendered), []byte(c.answer)); got != c.want {
			t.Errorf("%s: matches(%q, %q) = %v, want %v", c.name, c.rendered, c.answer, got, c.want)
		}
	}
}

// An answer that is not UTF-8 is read the way Java reads it: the bad byte is
// U+FFFD, and a rendering that holds U+FFFD there matches it.
func TestAnAnswerThatIsNotUTF8IsReadAsJavaReadsIt(t *testing.T) {
	if !matches([]byte("x�y"), []byte("x\xffy")) {
		t.Error("a malformed byte should compare as U+FFFD")
	}
	if matches([]byte("x?y"), []byte("x\xffy")) {
		t.Error("a malformed byte is not a question mark")
	}
	// One per byte, as Java does for bytes that start nothing.
	if !matches([]byte("a\uFFFD\uFFFDb"), []byte("a\xff\xfeb")) {
		t.Error("two malformed bytes should be two U+FFFD")
	}
}
