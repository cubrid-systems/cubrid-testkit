package status

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// templateView is the database-template cache as the page shows it.
//
// The cache lives on the shell side -- CTP's cubrid_createdb copies a database
// it built earlier instead of running createdb -- and it leaves everything
// needed to describe itself in its own store: a directory per key, a .refs count
// in each, a .origin naming the case it was built from, and a .used.<pid> file
// per process listing the keys that process restored. So this reads the store
// rather than asking the shell to report, and the two cannot disagree.
//
// It is worth watching because the cache is not obviously a win. Measured on
// _01_utility: it makes the median case 0.1 s faster and costs 78 s over the
// arm, because twelve volume-manipulating cases got 20 to 75 seconds slower --
// the origin path is embedded in the binary volumes and only the two text files
// are rewritten. A panel that shows the store filling up while the run gets
// slower is the shortest way to see that.
type templateView struct {
	Dir string `json:"dir"`
	// Count and MB are the store as it stands; MB is allocated blocks, which is
	// what the cache's own eviction measures, and a template is sparse enough
	// that the apparent size is twice it.
	Count int `json:"count"`
	MB    int `json:"mb"`
	CapMB int `json:"capMB"`
	// Restored is how many times this run took a template instead of running
	// createdb, and Built how many it had to make. Together they are the hit
	// rate, which is the number the cache exists for.
	Restored int           `json:"restored"`
	Built    int           `json:"built"`
	Top      []templateRow `json:"top"`
}

type templateRow struct {
	Key    string `json:"key"`
	Refs   int    `json:"refs"`
	MB     int    `json:"mb"`
	Origin string `json:"origin"`
}

// templates samples the store on its own ticker.
//
// Not on every page load: the store is capped at 10 GB by default and walking it
// is hundreds of stats. Five seconds is far below the rate at which a cache of
// whole databases changes.
type templates struct {
	dir   string
	capMB int
	since time.Time

	mu   sync.Mutex
	view templateView
	stop chan struct{}
}

// WatchTemplates starts reporting the template cache. An empty dir turns it off,
// which is what a run with the cache off passes.
func (b *Board) WatchTemplates(dir string, capMB int) {
	if b == nil || strings.TrimSpace(dir) == "" {
		return
	}
	t := &templates{dir: dir, capMB: capMB, since: time.Now(), stop: make(chan struct{})}
	b.mu.Lock()
	b.templates = t
	b.mu.Unlock()
	go t.watch()
}

func (t *templates) watch() {
	t.sample()
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-t.stop:
			return
		case <-tick.C:
			t.sample()
		}
	}
}

func (t *templates) snapshot() *templateView {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	v := t.view
	return &v
}

func (t *templates) sample() {
	v := templateView{Dir: t.dir, CapMB: t.capMB}
	entries, err := os.ReadDir(t.dir)
	if err != nil {
		t.mu.Lock()
		t.view = v
		t.mu.Unlock()
		return
	}
	for _, e := range entries {
		name := e.Name()
		// A dot-prefixed entry is the store's own bookkeeping: .used.<pid> is a
		// process's tally, .lk.<key> a lock, .tmp.<pid> a save in progress, and
		// .plan the eviction order. Only the rest are templates.
		if strings.HasPrefix(name, ".") {
			// Only this run's tallies. The store outlives a run and so do the
			// tallies in it, so counting them all reports another run's hits as
			// this one's -- and then loses them again, because the cache folds a
			// finished run's tally into the per-template .refs and deletes it.
			// The number went up and down and belonged to nobody. Built is
			// already filtered this way; Restored was not.
			if strings.HasPrefix(name, ".used.") {
				if info, err := e.Info(); err == nil && info.ModTime().After(t.since) {
					v.Restored += countLines(filepath.Join(t.dir, name))
				}
			}
			continue
		}
		if !e.IsDir() {
			continue
		}
		v.Count++
		row := templateRow{Key: name}
		row.MB = allocatedMB(filepath.Join(t.dir, name))
		v.MB += row.MB
		row.Refs = readInt(filepath.Join(t.dir, name, ".refs"))
		row.Origin = shortOrigin(readLine(filepath.Join(t.dir, name, ".origin")))
		if info, err := e.Info(); err == nil && info.ModTime().After(t.since) {
			v.Built++
		}
		v.Top = append(v.Top, row)
	}
	// Most used first, and by key where that ties -- sort.Slice is not stable and
	// the page refreshes once a second.
	sort.Slice(v.Top, func(i, j int) bool {
		if v.Top[i].Refs != v.Top[j].Refs {
			return v.Top[i].Refs > v.Top[j].Refs
		}
		return v.Top[i].Key < v.Top[j].Key
	})
	if len(v.Top) > 8 {
		v.Top = v.Top[:8]
	}
	t.mu.Lock()
	t.view = v
	t.mu.Unlock()
}

// allocatedMB is what the store actually occupies, which is what the cache's own
// eviction measures. A template is copied with --sparse=always and its apparent
// size is about twice its blocks, so counting bytes would evict on a number the
// filesystem does not charge for.
func allocatedMB(dir string) int {
	var blocks int64
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			if st, ok := info.Sys().(*syscall.Stat_t); ok {
				blocks += st.Blocks
			}
		}
		return nil
	})
	return int(blocks * 512 / (1 << 20))
}

func countLines(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	n := 0
	s := bufio.NewScanner(f)
	for s.Scan() {
		if strings.TrimSpace(s.Text()) != "" {
			n++
		}
	}
	return n
}

func readInt(path string) int {
	n, _ := strconv.Atoi(readLine(path))
	return n
}

func readLine(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.SplitN(string(b), "\n", 2)[0])
}

// shortOrigin names the case a template was built from the way the rest of the
// page names cases: the family and the case, not the path they share.
func shortOrigin(p string) string {
	p = strings.TrimSuffix(p, "/cases")
	parts := strings.Split(p, "/")
	if len(parts) <= 2 {
		return p
	}
	return strings.Join(parts[len(parts)-2:], "/")
}
