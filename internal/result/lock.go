package result

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// One run at a time per result tree.
//
// Two runs on one machine can share almost everything safely -- the corpus
// overlay, the slot namespaces, the template store, even the status port, each
// of which is either per-process or locked. What they cannot share is where the
// verdicts go: <CTP_HOME>/result/<category> holds one feedback.log, one
// test_status.data and one dispatch_tc_ALL.txt, and two runs writing them
// interleave into a result that describes neither. Nothing about that announces
// itself; the files simply come out wrong.
//
// So it is refused rather than tolerated. A second run says whose tree it is and
// what to change, which is a better answer than either corrupting the first
// run's results or silently waiting for a run that takes two hours.
//
// The lock lives beside `result/` rather than inside it, because the first thing
// a run does with that directory is often `rm -rf` -- a lock the next run
// deletes is not a lock.
type runLock struct {
	f    *os.File
	path string
}

func lockPath(home, category string) string {
	return filepath.Join(home, ".testkit-"+category+".lock")
}

// lockRun takes the tree, or explains who has it.
func lockRun(home, category, resultDir string) (*runLock, error) {
	path := lockPath(home, category)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		// A read-only CTP_HOME is not a reason to refuse to run: the lock is a
		// guard against a mistake, not a requirement of the runner.
		return nil, nil
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		held := describeHolder(f)
		f.Close()
		return nil, fmt.Errorf(
			"another run already has %s\n  %s\n"+
				"Two runs cannot share a result tree: they write one feedback.log and one\n"+
				"test_status.data between them, and the result describes neither. Point this\n"+
				"run somewhere else with CTP_HOME, or wait for that one to finish.",
			resultDir, held)
	}
	// Whoever holds it says so, so the next run's message can name them.
	_ = f.Truncate(0)
	_, _ = f.WriteAt([]byte(fmt.Sprintf("pid %d\nstarted %s\nresult %s\n",
		os.Getpid(), time.Now().Format(time.RFC3339), resultDir)), 0)
	_ = f.Sync()
	return &runLock{f: f, path: path}, nil
}

// describeHolder reads what the holder wrote about itself. It is a courtesy, so
// anything unreadable becomes a shrug rather than an error.
func describeHolder(f *os.File) string {
	b := make([]byte, 512)
	n, _ := f.ReadAt(b, 0)
	var pid, started string
	for _, line := range strings.Split(string(b[:n]), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			continue
		}
		switch k {
		case "pid":
			pid = v
		case "started":
			started = v
		}
	}
	if pid == "" {
		return "held by another process"
	}
	alive := ""
	if n, err := strconv.Atoi(pid); err == nil && syscall.Kill(n, 0) != nil {
		// The lock is the kernel's, so this cannot be stale -- but saying the
		// process is gone points at the right thing when it is a zombie or a
		// namespace makes the pid unreadable.
		alive = " (that pid is not visible from here; it may be in a namespace)"
	}
	if started != "" {
		return "held by pid " + pid + ", started " + started + alive
	}
	return "held by pid " + pid + alive
}

func (l *runLock) release() {
	if l == nil || l.f == nil {
		return
	}
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	_ = l.f.Close()
	l.f = nil
}
