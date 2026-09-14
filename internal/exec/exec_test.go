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

// Cancellation has to reach what the script started, not just the script. A
// survivor keeps the pipe open, and then Wait never returns.
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

// A case that starts a daemon leaves the daemon holding the pipe: the shell
// exits, its output is complete, and Wait blocks until WaitDelay gives up. The
// case has not failed, and reporting a runtime error there threw the verdict
// away -- measured on _36_cub_master/bug_xdbms40 and _40_broker/itrack03, which
// failed with no verdict at all.
func TestADaemonHoldingThePipeDoesNotLoseTheVerdict(t *testing.T) {
	l := NewLocal(t.TempDir())
	// A background process that outlives the shell and keeps stdout open, which
	// is what cub_master does.
	res, err := l.Run(context.Background(),
		"sleep 30 & echo started; exit 3")
	if err != nil {
		t.Fatalf("a held pipe was reported as a failure: %v", err)
	}
	if res.ExitCode != 3 {
		t.Errorf("the shell's exit code is %d, want 3", res.ExitCode)
	}
	if !strings.Contains(res.Stdout, "started") {
		t.Errorf("the output collected before the delay is missing: %q", res.Stdout)
	}
}

// The other half of WaitDelay's job: a cancelled case must never come back
// looking like it succeeded. It comes back as a killed process rather than an
// error -- the worker is what turns that into a timeout verdict -- but a zero
// exit code there would let a case that would not stop be recorded as a pass.
func TestACancelledCaseNeverLooksLikeSuccess(t *testing.T) {
	l := NewLocal(t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	res, err := l.Run(ctx, "sleep 30; echo finished")
	if err == nil && res.ExitCode == 0 {
		t.Errorf("a cancelled case came back as a success: %+v", res)
	}
	if strings.Contains(res.Stdout, "finished") {
		t.Error("a cancelled case ran to completion")
	}
}

// A script that exits non-zero has failed, and Check says so. The channel keeps
// err for not being able to run a script at all, and every caller that checked
// err alone took a failed script for one that worked.
func TestCheckTakesANonZeroExitAsAnError(t *testing.T) {
	l := NewLocal("")
	res, err := Check(l.Run(t.Context(),
		`echo partial; echo "the message" >&2; echo "a line between" >&2; echo "the last word" >&2; exit 3`))
	if err == nil {
		t.Fatal("a script that exited 3 was reported as a success")
	}
	for _, want := range []string{"exit 3", "the message", "the last word"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not say %q: %v", want, err)
		}
	}
	// Bounded: a cp refused a thousand files says so a thousand times.
	if strings.Contains(err.Error(), "a line between") {
		t.Errorf("the error carries all of stderr rather than its first and last lines: %v", err)
	}
	// What it printed before it failed is still the caller's to read.
	if !strings.Contains(res.Output(), "partial") {
		t.Errorf("the output was lost with the failure: %q", res.Output())
	}
	if _, err := Check(l.Run(t.Context(), "true")); err != nil {
		t.Errorf("a script that exited 0 was reported as a failure: %v", err)
	}
}

// A script that reports on stdout, as check_disk_space does, still gets its
// reason into the error -- the last thing it printed.
func TestFailureFallsBackToTheLastLineAScriptPrinted(t *testing.T) {
	err := Result{Stdout: "first\nUsage: nothing like this\n", ExitCode: 1}.Failure()
	if err == nil || err.Error() != "exit 1: Usage: nothing like this" {
		t.Errorf("got %v, want the status and the last line", err)
	}
	if err := (Result{ExitCode: 2}).Failure(); err == nil || err.Error() != "exit 2" {
		t.Errorf("got %v, want the bare status when the script said nothing", err)
	}
}
