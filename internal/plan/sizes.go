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
)

// Sizes records how many megabytes each case directory holds while it runs.
//
// The duration plan answers "how long", and that turned out to be the wrong
// question for one of the two decisions it was asked to make. Ordering wants
// duration and gets it right. Lanes wanted "which cases should not be given
// memory", and duration was a proxy that picked the wrong cases: it selects the
// I/O-heavy ones, which are the ones memory helps most, and the measured split
// lost -- 696 s against a predicted 440.
//
// Space is the thing the ceiling is actually made of, so space is what the lane
// should be chosen by. Measured on this corpus at db_volume_size=20M, the peak
// is not accumulation but coincidence: three directories that each hold
// gigabytes start within a hundred seconds of each other, because they are also
// long and the queue is ordered longest-first. Eighteen cases in, 18,417 MB of
// an 18,432 MB ceiling.
//
// By directory rather than by case because a directory is what a slot owns and
// what a reclaim drops -- see Corpus.Retire. The format is one line per
// directory, biggest first:
//
//	4820 /path/to/scenario/_06_issues/_11_1h/bug_bts_4823/cases
//
// A run measures a directory once, when it retires, and the figure replaces
// what an earlier run recorded. It is a measurement of the corpus under this
// configuration, so a run that changes db_volume_size should delete this file
// rather than let the old numbers decide its lanes.
type Sizes struct {
	mu sync.Mutex
	mb map[string]int
}

// NewSizes starts an empty record.
func NewSizes() *Sizes { return ContinueSizes(nil) }

// ContinueSizes starts from a record already on disk, so a directory this run
// does not reach keeps the figure an earlier run measured -- and so a run with
// lanes on, which cannot measure a directory it sent to disk, does not forget
// why it sent it there.
func ContinueSizes(prior map[string]int) *Sizes {
	mb := make(map[string]int, len(prior))
	for d, n := range prior {
		mb[d] = n
	}
	return &Sizes{mb: mb}
}

// Add records what a directory held. A nil record accepts everything and writes
// nothing.
func (s *Sizes) Add(dir string, mb int) {
	if s == nil || dir == "" || mb <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mb[dir] = mb
}

// Merge takes a whole run's measurements at once, which is how the corpus hands
// over what it saw as each directory was reclaimed.
func (s *Sizes) Merge(m map[string]int) {
	for dir, mb := range m {
		s.Add(dir, mb)
	}
}

// Write saves the record, biggest directory first.
func (s *Sizes) Write(path string) error {
	if s == nil || path == "" {
		return nil
	}
	s.mu.Lock()
	dirs := make([]string, 0, len(s.mb))
	for d := range s.mb {
		dirs = append(dirs, d)
	}
	sort.Slice(dirs, func(i, j int) bool {
		if s.mb[dirs[i]] != s.mb[dirs[j]] {
			return s.mb[dirs[i]] > s.mb[dirs[j]]
		}
		return dirs[i] < dirs[j]
	})
	lines := make([]string, 0, len(dirs))
	for _, d := range dirs {
		lines = append(lines, fmt.Sprintf("%d %s", s.mb[d], d))
	}
	s.mu.Unlock()

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

// ReadSizes loads a record. A missing file is not an error: the first run on a
// machine has none, and lanes then say so rather than guessing.
func ReadSizes(path string) (map[string]int, error) {
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

	out := map[string]int{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// One split rather than fields, for the same reason the duration plan
		// does it: a directory could contain a space and a megabyte count cannot.
		at := strings.IndexByte(line, ' ')
		if at < 0 {
			continue
		}
		mb, err := strconv.Atoi(line[:at])
		if err != nil || mb < 0 {
			continue
		}
		if dir := strings.TrimSpace(line[at+1:]); dir != "" {
			out[dir] = mb
		}
	}
	return out, sc.Err()
}
