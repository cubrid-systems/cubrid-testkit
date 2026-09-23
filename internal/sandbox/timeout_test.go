package sandbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A call the runner's own deadline killed must say so. `exec` reports it as
// "signal: killed", which reads exactly like something outside the run killing
// the process -- and a whole-corpus run produced eight of them, every one from
// this bound rather than from anything wrong with the pair.
func TestATimeoutSaysItWasTheBoundAndNotTheNode(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "csb")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexec sleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// `exec sleep`, not `sleep`: CommandContext kills the process it started, and
	// a shell that merely waits on a child leaves that child holding the pipes,
	// so cmd.Run would block until the child ended on its own. Worth a line
	// because it is also true of csb -- the bound reaches csb, not whatever csb
	// has started underneath it.
	c := &CLI{Bin: bin, Cluster: "hadb", Timeout: 150 * time.Millisecond}

	_, err := c.call(context.Background(), "ha", "status")
	if err == nil {
		t.Fatal("want an error from a call that outlived its bound")
	}
	if !errors.Is(err, ErrTimedOut) {
		t.Errorf("want ErrTimedOut, got %v", err)
	}
	if got := err.Error(); !strings.Contains(got, "150ms") {
		t.Errorf("the message should name the bound it broke; got %q", got)
	}
	if strings.Contains(err.Error(), "signal: killed") {
		t.Errorf("the message should not repeat exec's wording; got %q", err.Error())
	}
}
