package plan

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A run that was not asked for a plan must not have to check anywhere.
func TestANilRecordIsUsable(t *testing.T) {
	var r *Record
	r.Add("a", time.Second)
	if err := r.Write(filepath.Join(t.TempDir(), "p")); err != nil {
		t.Errorf("a nil record failed to write nothing: %v", err)
	}
}

// The first run on a machine has no plan, and that is the normal case.
func TestAMissingPlanIsNotAnError(t *testing.T) {
	got, err := Read(filepath.Join(t.TempDir(), "absent"))
	if err != nil || got != nil {
		t.Errorf("a missing plan gave %v %v", got, err)
	}
	if _, err := Read(""); err != nil {
		t.Errorf("an unset path gave %v", err)
	}
}

func TestARoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "plan")
	r := NewRecord()
	r.Add("/x/short.sh", 6*time.Second)
	r.Add("/x/long.sh", 183*time.Second)
	r.Add("/x/zero.sh", 0)
	if err := r.Write(p); err != nil {
		t.Fatal(err)
	}

	// Longest first in the file itself, because an operator reads it.
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	want := "183.0 /x/long.sh\n6.0 /x/short.sh\n0.0 /x/zero.sh\n"
	if string(b) != want {
		t.Errorf("the file reads\n%q\nwant\n%q", b, want)
	}

	back, err := Read(p)
	if err != nil {
		t.Fatal(err)
	}
	if back["/x/long.sh"] != 183*time.Second || back["/x/short.sh"] != 6*time.Second {
		t.Errorf("the durations did not survive the round trip: %v", back)
	}
}

// A plan half-written by a run that was killed would put the cases it never
// reached first, which is the opposite of the point.
func TestWritingIsAtomic(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "plan")
	r := NewRecord()
	r.Add("/x/a.sh", time.Second)
	if err := r.Write(p); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "plan" {
			t.Errorf("the write left %q behind", e.Name())
		}
	}
}

// The point of the whole thing: the 183-second case has to go out first, or a
// slot that draws it at the end holds every other slot up.
func TestOrderPutsTheLongestFirst(t *testing.T) {
	cases := []string{"/x/short.sh", "/x/long.sh", "/x/mid.sh"}
	known := map[string]time.Duration{
		"/x/short.sh": 6 * time.Second,
		"/x/long.sh":  183 * time.Second,
		"/x/mid.sh":   19 * time.Second,
	}
	got := Order(cases, known)
	want := []string{"/x/long.sh", "/x/mid.sh", "/x/short.sh"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order is %v, want %v", got, want)
		}
	}
}

// A case the plan does not mention is new or renamed. Last is the one place it
// must not go: if it turns out to be the four-minute case, every slot waits.
func TestAnUnknownCaseGoesFirst(t *testing.T) {
	cases := []string{"/x/known.sh", "/x/new.sh"}
	got := Order(cases, map[string]time.Duration{"/x/known.sh": 183 * time.Second})
	if got[0] != "/x/new.sh" {
		t.Errorf("an unmeasured case was scheduled behind a known one: %v", got)
	}
}

// Two runs of the same corpus have to hand cases out in the same order, or a
// comparison between them is comparing two schedules as well as two builds.
func TestOrderIsStable(t *testing.T) {
	cases := []string{"/x/c.sh", "/x/a.sh", "/x/b.sh"}
	known := map[string]time.Duration{
		"/x/a.sh": time.Second, "/x/b.sh": time.Second, "/x/c.sh": time.Second,
	}
	first := Order(cases, known)
	for i := 0; i < 20; i++ {
		got := Order([]string{"/x/b.sh", "/x/c.sh", "/x/a.sh"}, known)
		for j := range got {
			if got[j] != first[j] {
				t.Fatalf("tied cases came out %v then %v", first, got)
			}
		}
	}
	if first[0] != "/x/a.sh" {
		t.Errorf("tied cases are not in name order: %v", first)
	}
}

// No plan means the corpus order, untouched. That is the old behaviour and it is
// what a run gets unless it asked for this.
func TestNoPlanKeepsTheCorpusOrder(t *testing.T) {
	cases := []string{"/x/c.sh", "/x/a.sh", "/x/b.sh"}
	got := Order(cases, nil)
	for i := range cases {
		if got[i] != cases[i] {
			t.Errorf("an empty plan reordered the cases: %v", got)
		}
	}
}

// A plan is a file on disk that another run wrote. It must not be able to make
// this one fail.
func TestAJunkPlanIsIgnoredLineByLine(t *testing.T) {
	p := filepath.Join(t.TempDir(), "plan")
	os.WriteFile(p, []byte(
		"# a comment\n\nnotanumber /x/a.sh\n-5 /x/neg.sh\n12\n7.5 /x/ok.sh\n3.0 /x/with space.sh\n"), 0o644)
	got, err := Read(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("kept %d entries of the two that were valid: %v", len(got), got)
	}
	if got["/x/ok.sh"] != 7500*time.Millisecond {
		t.Errorf("the valid line did not survive: %v", got)
	}
	if got["/x/with space.sh"] != 3*time.Second {
		t.Errorf("a path with a space was mangled: %v", got)
	}
}

// A run that is killed part way through must not take the plan with it.
func TestAnInterruptedRunKeepsWhatItDidNotReach(t *testing.T) {
	prior := map[string]time.Duration{
		"a": 800 * time.Second,
		"b": 700 * time.Second,
		"c": 600 * time.Second,
	}
	r := Continue(prior)
	// Only one case ran, and it was faster this time.
	r.Add("a", 500*time.Second)

	path := filepath.Join(t.TempDir(), "plan")
	if err := r.Write(path); err != nil {
		t.Fatal(err)
	}
	got, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]time.Duration{
		"a": 500 * time.Second,
		"b": 700 * time.Second,
		"c": 600 * time.Second,
	}
	if len(got) != len(want) {
		t.Fatalf("plan has %d entries, want %d: %v", len(got), len(want), got)
	}
	for n, d := range want {
		if got[n] != d {
			t.Errorf("%s: %v, want %v", n, got[n], d)
		}
	}

	// Seeding copies rather than aliases: the caller's map is not the record.
	if prior["a"] != 800*time.Second {
		t.Errorf("Continue wrote through to the prior plan: a is %v", prior["a"])
	}
}
