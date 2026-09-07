package exec

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Cancelling a case has to reach what the case started, not just the shell it
// started in. Without a process group the shell dies and everything below it --
// a csql, a cub_commdb asleep in a retry loop -- keeps running and keeps the
// pipe open, so Wait never returns and the runner waits for a case that will
// never end. That is not hypothetical: docs/evidence/regression-shell.md records
// the run it stopped, at case 2 of 217.
func TestCancellingLocalReachesWhatTheScriptStarted(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "started.pid")

	ctx, cancel := context.WithCancel(t.Context())
	l := &Local{}

	done := make(chan struct{})
	go func() {
		defer close(done)
		// The sleep would outlive this test by minutes. The point is that it does not.
		_, _ = l.Run(ctx, "sleep 600 & echo $! > "+pidFile+"; wait")
	}()

	pid := 0
	for range 300 {
		if b, err := os.ReadFile(pidFile); err == nil {
			if n, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && n > 0 {
				pid = n
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid == 0 {
		cancel()
		<-done
		t.Fatal("the script never reported the process it started")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("Run did not return after the context was cancelled")
	}

	// Signal 0 asks whether the process is still there without touching it.
	for range 300 {
		if err := syscall.Kill(pid, 0); err != nil {
			return // gone, which is the whole point
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	t.Fatalf("the process the script started (%d) survived cancellation", pid)
}
