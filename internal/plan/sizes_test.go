package plan

import (
	"path/filepath"
	"testing"
)

func TestSizesRoundTripBiggestFirst(t *testing.T) {
	s := NewSizes()
	s.Add("/c/_a/cases", 20)
	s.Add("/c/_b/cases", 5000)
	s.Add("/c/_c/cases", 300)
	// A directory that held nothing measurable is not a directory that held
	// nothing: the tmpfs is measured whole, so a small one under seven busy
	// slots can read zero. Recording it as zero would send it to the fast lane
	// on evidence that is not there, which is where it would go anyway.
	s.Add("/c/_d/cases", 0)

	path := filepath.Join(t.TempDir(), "sizes")
	if err := s.Write(path); err != nil {
		t.Fatal(err)
	}
	got, err := ReadSizes(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("read %d entries, want 3: %v", len(got), got)
	}
	if got["/c/_b/cases"] != 5000 || got["/c/_a/cases"] != 20 {
		t.Errorf("wrong figures: %v", got)
	}
}

// The same rule the duration plan needed: a run that does not reach a directory
// must not forget it. With lanes on this is not an edge case -- a slow-lane
// directory writes to disk and is never measured, so every run would drop the
// directories the previous run sent to disk, and the lane would empty itself.
func TestSizesCarryForward(t *testing.T) {
	prior := map[string]int{"/c/_a/cases": 5000, "/c/_b/cases": 300}
	s := ContinueSizes(prior)
	s.Add("/c/_b/cases", 350)

	path := filepath.Join(t.TempDir(), "sizes")
	if err := s.Write(path); err != nil {
		t.Fatal(err)
	}
	got, err := ReadSizes(path)
	if err != nil {
		t.Fatal(err)
	}
	if got["/c/_a/cases"] != 5000 {
		t.Errorf("the unmeasured directory lost its figure: %v", got)
	}
	if got["/c/_b/cases"] != 350 {
		t.Errorf("the measured directory kept a stale figure: %v", got)
	}
	if prior["/c/_b/cases"] != 300 {
		t.Errorf("ContinueSizes wrote through to the caller's map: %v", prior)
	}
}

func TestMissingSizesIsNotAnError(t *testing.T) {
	got, err := ReadSizes(filepath.Join(t.TempDir(), "absent"))
	if err != nil {
		t.Fatalf("a machine with no measurements yet is the normal case: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want nothing, got %v", got)
	}
}
