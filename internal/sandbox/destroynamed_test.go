package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Replacing a set removes pairs an earlier run made, and those carry that run's
// claim rather than this one's. Selecting by the current run's label matched
// nothing, said nothing went down, and the caller built on top of what was
// still standing -- so the names the caller holds are what it has to use.
func TestDestroyNamedReportsEveryClusterThatWouldNotGo(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "csb")
	// Refuses everything, the way a backend that cannot be reached would.
	script := "#!/bin/sh\n" +
		`printf '{"schema":"csb/v1","command":"cluster destroy","ok":false,` +
		`"notes":[{"code":"destroy_failed","severity":"error","message":"nope"}]}'` + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(BinEnv, bin)

	err := DestroyNamed(context.Background(), []string{"a-p1", "a-p2"})
	if err == nil {
		t.Fatal("want an error when nothing could be destroyed")
	}
	for _, want := range []string{"2 of 2", "a-p1", "a-p2"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error should name %q; got %q", want, err)
		}
	}
}

// Nothing to destroy is not a failure: a set that does not exist yet is the
// ordinary first run.
func TestDestroyNamedOfNothingSucceeds(t *testing.T) {
	if err := DestroyNamed(context.Background(), nil); err != nil {
		t.Errorf("want nil for an empty list, got %v", err)
	}
}
