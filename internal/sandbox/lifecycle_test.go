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

// An orphan is a cluster some run claimed and did not take back. A cluster
// nobody claimed is somebody's by hand and is never one.
func TestOnlyAClaimedClusterCanBeAnOrphan(t *testing.T) {
	all := []Cluster{
		{Name: "hadb"}, // made by hand
		{Name: "tk0923aa-p1", Labels: map[string]string{RunLabel: "tk0923aa"}}, // a live run's
		{Name: "tk0101bb-p1", Labels: map[string]string{RunLabel: "tk0101bb"}}, // nobody's
		{Name: "other", Labels: map[string]string{"team": "qa"}},               // labelled, not by a run
	}
	got := Orphans(all, map[string]bool{"tk0923aa": true})
	if len(got) != 1 || got[0].Name != "tk0101bb-p1" {
		t.Fatalf("want only tk0101bb-p1, got %v", names(got))
	}
}

func names(cs []Cluster) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Name
	}
	return out
}
