package status

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// "this run" has to mean this run. The store outlives a run and so do the
// tallies in it, so counting every .used.<pid> reports another run's hits as
// this one's -- and then loses them again, because the cache folds a finished
// run's tally into the per-template .refs and deletes it. Watched on the page:
// the number went up and down and belonged to nobody.
func TestOnlyThisRunsRestoresAreCounted(t *testing.T) {
	dir := t.TempDir()

	// A previous run's tally, and a template it left behind.
	old := filepath.Join(dir, ".used.111")
	if err := os.WriteFile(old, []byte("k1\nk1\nk2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldKey := filepath.Join(dir, "aaaa")
	if err := os.MkdirAll(oldKey, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().Add(-time.Hour)
	for _, p := range []string{old, oldKey} {
		if err := os.Chtimes(p, stale, stale); err != nil {
			t.Fatal(err)
		}
	}

	// The run starts now, and then restores twice and builds one.
	tp := &templates{dir: dir, since: time.Now()}
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(dir, ".used.222"), []byte("k3\nk3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "bbbb"), 0o755); err != nil {
		t.Fatal(err)
	}

	tp.sample()
	v := tp.snapshot()
	if v.Restored != 2 {
		t.Errorf("this run restored 2 and the page says %d (the older tally leaked in)", v.Restored)
	}
	if v.Built != 1 {
		t.Errorf("this run built 1 and the page says %d", v.Built)
	}
	// The store itself is both, and is not claimed as this run's.
	if v.Count != 2 {
		t.Errorf("the store holds 2 templates and the page says %d", v.Count)
	}
}
