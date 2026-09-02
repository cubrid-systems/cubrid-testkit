package exec

import "testing"

// The frame is the only part of the remote protocol that can be tested without a
// remote machine, and it is the part most likely to be broken by a rewrite.

func TestFrameKeepsWhatIsInside(t *testing.T) {
	raw := "banner from /etc/motd\n" +
		startFlagLiteral + "\n" +
		"the output\nmore output\n" +
		compFlagLiteral + "\n" +
		"trailing noise\n"
	if got, want := between(raw), "the output\nmore output\n"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestFrameSurvivesAMissingEnd(t *testing.T) {
	// A script killed halfway never prints the closing marker. Its output is still
	// the most useful thing available, so it is returned rather than discarded.
	raw := startFlagLiteral + "\npartial output\n"
	if got, want := between(raw), "partial output\n"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestFrameReturnsEverythingWhenUnmarked(t *testing.T) {
	raw := "no markers here\n"
	if got := between(raw); got != raw {
		t.Errorf("got %q want %q", got, raw)
	}
}

func TestTheEchoedMarkerCannotMatchItself(t *testing.T) {
	// This is the whole reason for the ${NOTEXIST} indirection. A shell that echoes
	// the script it was given -- set -x, a banner, a replayed sudo line -- must not
	// be mistaken for the frame. So the text the script carries has to differ from
	// the text the frame looks for.
	if startFlagEcho == "echo "+startFlagLiteral {
		t.Fatal("the script would carry the literal marker, and echoing it would open the frame early")
	}
	for _, literal := range []string{startFlagLiteral, compFlagLiteral} {
		for _, script := range []string{startFlagEcho, compFlagEcho} {
			if contains(script, literal) {
				t.Errorf("script text %q contains the literal marker %q", script, literal)
			}
		}
	}
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}
