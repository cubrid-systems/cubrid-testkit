package sandbox

import (
	"regexp"
	"testing"
	"time"
)

// A run's name becomes a cluster name, and csb derives the network, the
// containers and the database from that -- so it has to satisfy csb's rule:
// lowercase letters, digits and dashes, starting with a letter.
func TestARunNameIsUsableAsAClusterName(t *testing.T) {
	ok := regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	seen := map[string]bool{}
	at := time.Date(2026, 9, 23, 17, 4, 5, 0, time.UTC)
	for i := 0; i < 200; i++ {
		run := NewRunName(at)
		if !ok.MatchString(run) {
			t.Fatalf("run name %q is not a legal cluster name", run)
		}
		if !ok.MatchString(PairName(run, 8)) {
			t.Fatalf("pair name %q is not a legal cluster name", PairName(run, 8))
		}
		seen[run] = true
	}
	// Same instant, 200 names: the date alone cannot separate two runs on one
	// day, which is the case the random part exists for.
	if len(seen) < 150 {
		t.Errorf("200 names from one instant produced %d distinct; they must not collide", len(seen))
	}
}

// A set is its numbered members, in order, and a hole is not closed over.
func TestASetIsItsNumberedMembers(t *testing.T) {
	all := []Cluster{
		{Name: "perf-p3"}, {Name: "perf-p1"}, {Name: "perf-p10"},
		{Name: "perf"},       // the bare name is not a member
		{Name: "perfx-p1"},   // a different set that shares a prefix
		{Name: "perf-pzero"}, // not a number
	}
	got := MembersOf(all, "perf")
	want := []string{"perf-p1", "perf-p3", "perf-p10"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if n := len(MembersOf(all, "nothing")); n != 0 {
		t.Errorf("a set with no members should be empty, got %d", n)
	}
}
