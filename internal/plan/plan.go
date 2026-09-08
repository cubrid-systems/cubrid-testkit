// Package plan records how long each case took, and orders the next run by it.
//
// This is the number two separate things have been waiting on. B-T3 wants the
// queue longest-first, because handing cases out in corpus order lets a
// four-minute case start when the queue is nearly empty and every other slot
// then waits for it -- and longest-first on a shared queue is the same schedule
// as partitioning the cases by rank beforehand, without needing to know how many
// slots there are. B-T13 wants the durations to decide which cases are worth
// giving memory to, because a case holds its database for as long as it runs and
// the cases differ by two orders of magnitude.
//
// Measured over _01_utility: 3,189 case-seconds across 217 cases, of which the
// 17 cases over 30 seconds are 41%. The longest is 195 s and the median is
// under 10.
//
// The format is one line per case, longest first:
//
//	195.0 /path/to/scenario/_15_backupdb/itrack_10002/cases/itrack_10002.sh
//
// Text, sorted, one field and a path, because it is a file an operator will want
// to read, grep and diff between runs -- "what got slower" is the question it
// should be able to answer without a tool.
package plan

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Record collects this run's durations. A nil Record accepts everything and
// writes nothing, so a run that was not asked for a plan needs no call site to
// check.
type Record struct {
	mu   sync.Mutex
	took map[string]time.Duration
}

func NewRecord() *Record { return &Record{took: map[string]time.Duration{}} }

// Add records what a case took. A retried case is added once, when it retires,
// so the duration is the attempt that produced the verdict.
func (r *Record) Add(name string, d time.Duration) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.took[name] = d
}

// Write saves the record, longest case first.
//
// Written to a temporary file and renamed, because the run that reads this is
// usually the next one on the same machine and a plan half-written by a run that
// was killed is worse than no plan: Order would put the cases it did not reach
// first, which is exactly backwards.
func (r *Record) Write(path string) error {
	if r == nil || path == "" {
		return nil
	}
	r.mu.Lock()
	lines := make([]string, 0, len(r.took))
	names := make([]string, 0, len(r.took))
	for n := range r.took {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		a, b := r.took[names[i]], r.took[names[j]]
		if a != b {
			return a > b
		}
		return names[i] < names[j]
	})
	for _, n := range names {
		lines = append(lines, fmt.Sprintf("%.1f %s", r.took[n].Seconds(), n))
	}
	r.mu.Unlock()

	if len(lines) == 0 {
		return nil
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Read loads a plan. A missing file is not an error -- the first run on a machine
// has none, and that is the normal case rather than a fault.
func Read(path string) (map[string]time.Duration, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	out := map[string]time.Duration{}
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// One split, not fields: a case path could contain a space, and the
		// duration cannot.
		at := strings.IndexByte(line, ' ')
		if at < 0 {
			continue
		}
		secs, err := strconv.ParseFloat(line[:at], 64)
		if err != nil || secs < 0 {
			continue
		}
		if name := strings.TrimSpace(line[at+1:]); name != "" {
			out[name] = time.Duration(secs * float64(time.Second))
		}
	}
	return out, s.Err()
}

// Order returns cases longest-first, by what a previous run measured.
//
// A case the plan does not mention goes first, not last. It is either new or it
// was added since, so its duration is unknown -- and an unknown case scheduled
// last is the one arrangement that can leave every slot waiting on it. First
// costs nothing if it turns out to be short.
//
// Ties break by name so that two runs of the same corpus hand cases out in the
// same order, which is what makes a comparison between them mean anything.
func Order(cases []string, known map[string]time.Duration) []string {
	out := make([]string, len(cases))
	copy(out, cases)
	if len(known) == 0 {
		return out
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, aok := known[out[i]]
		b, bok := known[out[j]]
		switch {
		case !aok && !bok:
			return out[i] < out[j]
		case !aok:
			return true
		case !bok:
			return false
		case a != b:
			return a > b
		}
		return out[i] < out[j]
	})
	return out
}
