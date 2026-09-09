package shellsuite

import (
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/contain"
)

// Corpus is the scenario tree as a run sees it: the repository's copy read-only
// underneath, this run's writes in memory on top, and each case's writes dropped
// again as soon as the last case in its directory is done.
//
// The first two are B-T12. A case creates its database in its own directory and
// not all of them delete it, and nothing puts the tree back -- 105 MB in the
// repository was 20 GB on this machine, and a case that finds a database it did
// not create behaves differently from one that does not. An overlay makes clean
// structural rather than a step, and an upper layer in memory takes the run's
// writes off a disk that was measured as the bottleneck.
//
// The third is what running it taught, and it took two measurements to get
// right. The ceiling has two parts and only one of them can be given back.
//
// Sampled on a running arm at case 112 of 217, the upper layer held 4,730 MB.
// At case 216 it held 817 -- so it is not accumulation, and the first reading of
// it as accumulation was wrong. Of that 817, two directories held 813:
// _17_loaddb/_enhance_1002 at 531 MB and _19_loaddb_parameter/bigdata_alltype_test
// at 282. The other 215 directories held under a megabyte each. Subtracting
// gives 3,917 MB in flight across eight slots at case 112, which is the sum of
// the eight largest case databases to within 2%: six of the eight slots were
// holding a 512 MB database at the same time.
//
// So the working set is roughly the slots times the biggest databases, it is
// live data, and nothing can reclaim it -- keeping the big cases off each other
// is B-T13's ranked lanes, not this. What this reclaims is the residual: the
// cases that never clean up. Two in 217 here, and that is the ratio that put
// 20 GB in a corpus that is 105 MB in git. At the same rate the full 3,475 cases
// leave around 13 GB behind, which no ceiling this machine can spare would
// survive -- and unlike the working set, it is pure waste held to the end of the
// run.
type Corpus struct {
	// root is the scenario directory, and where the overlay is mounted.
	root string
	// ram is the tmpfs holding up/ and work/, and the thing whose fullness is
	// the ceiling.
	ram string
	// pristine is where the tree as the repository has it can be read.
	//
	// Without it there is no way to tell a file this run created from a file it
	// modified, and the difference decides whether removing it is a reclaim or a
	// whiteout. In shared mode it is a read-only bind taken before the overlay
	// went on top of root; with lanes nothing is mounted over root in this
	// process's namespace, so root itself is the pristine tree and no bind is
	// needed.
	pristine string
	// shared is the one-overlay-for-every-slot arrangement: mounted here, before
	// the slots exist, so they inherit it. It is what a run without lanes gets,
	// unchanged.
	shared bool
	// disk is where slow-lane slots put their upper layers. Empty without lanes.
	disk string
	// slots is each slot's upper root and the way to run a command in its
	// namespace -- needed because a reclaim has to delete through the slot's own
	// overlay: overlayfs does not allow its layers to change underneath it.
	slots map[string]*slotStore

	mu sync.Mutex
	// pending counts, per case directory, the cases that have yet to retire.
	// Reclaiming per case rather than per directory would be wrong: 15 of this
	// family's 217 directories hold more than one case, and a case that set up a
	// database for its sibling would find it gone.
	pending map[string]int
	freed   int
	// held is how many megabytes each directory was holding when it retired,
	// which is the measurement lanes need: the ceiling is made of space, so the
	// lane that gives space away has to be chosen by space.
	//
	// The tmpfs is measured whole, so a directory reclaimed while other slots are
	// writing reads low. It is the right kind of wrong -- the figure selects the
	// directories worth keeping off memory, and those are the ones whose own
	// gigabytes dwarf what seven other slots move in the same instant.
	held map[string]int

	peak int
	stop chan struct{}
	mb   int
}

// slotStore is one slot's place to write and the way to reach it.
type slotStore struct {
	upperRoot string
	onRAM     bool
	run       func(script string) error
}

// OpenCorpus puts root behind an overlay whose upper layer is a tmpfs of mb
// megabytes.
//
// Without lanes it mounts one overlay here, before the slots are opened, so that
// they inherit it -- which is what B-T12 described and what a run gets unless it
// asks for lanes. With lanes it mounts only the tmpfs, and each slot mounts an
// overlay of its own over the corpus with an upper from Slot.
//
// Either way the case list does not exist yet at this point, so what to reclaim
// arrives later, through Plan.
func OpenCorpus(root string, mb int, lanes bool) (*Corpus, error) {
	if root == "" {
		return nil, fmt.Errorf("scenario_ram_mb needs scenario to be set")
	}
	if !contain.Active() {
		return nil, fmt.Errorf("scenario_ram_mb needs the runner contained; set %s=1", contain.Env)
	}
	ram, err := os.MkdirTemp("", "testkit-corpus-*")
	if err != nil {
		return nil, err
	}
	c := &Corpus{
		root:    root,
		ram:     ram,
		pending: map[string]int{},
		held:    map[string]int{},
		slots:   map[string]*slotStore{},
		stop:    make(chan struct{}),
		mb:      mb,
		shared:  !lanes,
		pristine: func() string {
			if lanes {
				// Nothing is mounted over root here, so this process reads the
				// tree as the repository has it.
				return root
			}
			return filepath.Join(ram, "lower")
		}(),
	}
	if lanes {
		c.disk = filepath.Join(filepath.Dir(strings.TrimRight(root, "/")), ".testkit-slow-lane")
		if err := os.MkdirAll(c.disk, 0o755); err != nil {
			_ = os.Remove(ram)
			return nil, err
		}
	}
	if err := c.run(fmt.Sprintf("mount -t tmpfs -o size=%dm corpus %s", mb, ram)); err != nil {
		_ = os.Remove(ram)
		return nil, err
	}
	if c.shared {
		// The bind before the overlay, because afterwards the path leads to the
		// overlay and the tree underneath is no longer reachable by name.
		if err := c.run(fmt.Sprintf(
			"mkdir -p %s/up %s/work %s && mount --bind %s %s && mount -o remount,bind,ro %s",
			ram, ram, c.pristine, root, c.pristine, c.pristine)); err != nil {
			_ = c.run("umount " + ram)
			_ = os.Remove(ram)
			return nil, err
		}
		if err := c.run(fmt.Sprintf(
			"mount -t overlay overlay -o lowerdir=%s,upperdir=%s/up,workdir=%s/work %s",
			c.pristine, ram, ram, root)); err != nil {
			_ = c.run("umount " + c.pristine + "; umount " + ram)
			_ = os.Remove(ram)
			return nil, err
		}
	}
	go c.watch()
	return c, nil
}

// Slot says where one slot's corpus writes go, and returns the upper root the
// caller hands to Namespace.Overlay.
//
// The fast lane's upper is a directory on the run's tmpfs -- one tmpfs, one
// ceiling, an upper each. Two overlays cannot share an upperdir, but they can
// take subdirectories of the same filesystem, which keeps the ceiling a single
// flexible pool rather than N fixed reservations. Sized per slot it would need
// the largest directory a slot might draw, 1,078 MB, so eight private ceilings
// want 8.6 GB where one pool needed 6.
//
// The slow lane's upper is on disk beside the corpus, uncapped: a lane on disk
// is a lane whose whole point is not to spend the ceiling.
//
// run is how a reclaim reaches this slot. It has to go through the slot's own
// overlay, because overlayfs does not allow its layers to change underneath it.
func (c *Corpus) Slot(slot string, onRAM bool, run func(script string) error) (string, error) {
	if c == nil {
		return "", nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.shared {
		return "", fmt.Errorf("this corpus is mounted once for every slot; it has no per-slot upper")
	}
	base := c.ram
	if !onRAM {
		base = c.disk
	}
	root := filepath.Join(base, slot)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	c.slots[slot] = &slotStore{upperRoot: root, onRAM: onRAM, run: run}
	return root, nil
}

// Plan tells the corpus how many cases each directory holds, which is what makes
// Retire able to tell the last one. Until it is called nothing is reclaimed, so a
// run that never gets a case list behaves as it did before.
func (c *Corpus) Plan(cases []string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, p := range cases {
		if split, err := Split(p); err == nil {
			c.pending[split.Dir]++
		}
	}
}

// Ram is the tmpfs, for the panel that reports how full it is.
// Held is what each case directory was holding when it retired, in megabytes.
// A copy, because the run writes it to a file after the slots have stopped and
// the corpus may still be sampling.
func (c *Corpus) Held() map[string]int {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]int, len(c.held))
	for d, mb := range c.held {
		out[d] = mb
	}
	return out
}

func (c *Corpus) Ram() string {
	if c == nil {
		return ""
	}
	return c.ram
}

// Retire records that a case in dir is finished for good, and drops the
// directory's writes when it was the last one.
//
// For good: a case going back for a retry has not finished, and reclaiming under
// it would delete the state its next attempt is about to look for.
func (c *Corpus) Retire(slot, dir string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	n, ok := c.pending[dir]
	if !ok {
		return
	}
	if n > 1 {
		c.pending[dir] = n - 1
		return
	}
	delete(c.pending, dir)
	if !c.shared {
		st := c.slots[slot]
		if st == nil {
			return
		}
		before := c.used()
		if before > c.peak {
			c.peak = before
		}
		c.dropInSlot(st, dir)
		if st.onRAM {
			if freed := before - c.used(); freed > 0 {
				c.freed += freed
				c.held[dir] = freed
			}
		}
		// A slow-lane directory writes to disk, so there is nothing on the tmpfs
		// to measure and none is recorded. It keeps the figure that sent it to
		// disk, which is what ContinueSizes is for.
		return
	}
	// The high-water mark is read here as well as on the ticker: reclaiming is
	// exactly when the tmpfs is at its fullest, and a two-second sample can miss
	// a peak that a case reached and gave back between ticks.
	before := c.used()
	if before > c.peak {
		c.peak = before
	}
	c.dropAdditions(dir, c.pristineOf(dir))
	if freed := before - c.used(); freed > 0 {
		c.freed += freed
		c.held[dir] = freed
	}
}

func (c *Corpus) pristineOf(dir string) string {
	rel, ok := c.relOf(dir)
	if !ok {
		return ""
	}
	return filepath.Join(c.pristine, rel)
}

func (c *Corpus) relOf(dir string) (string, bool) {
	rel, err := filepath.Rel(c.root, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return rel, true
}

// dropInSlot is the reclaim when each slot has an overlay of its own.
//
// Two views are needed and neither alone is enough. This process can read the
// slot's *upper layer* directly -- it is a directory on the tmpfs or on disk --
// which says what the slot wrote; and it can read the corpus at root, because
// with lanes nothing is mounted over it here, which says what the repository
// has. What it cannot do is delete: the file has to go through the slot's own
// overlay, because overlayfs does not allow its layers to change underneath it.
//
// So: enumerate here, decide here, delete there. A name the pristine tree also
// has is left alone -- removing it would write a whiteout and hide a corpus file
// from every case that ran afterwards.
func (c *Corpus) dropInSlot(st *slotStore, dir string) {
	rel, ok := c.relOf(dir)
	if !ok {
		return
	}
	var gone []string
	c.collectAdditions(filepath.Join(st.upperRoot, "upper", rel), filepath.Join(c.root, rel), dir, &gone)
	if len(gone) == 0 {
		return
	}
	args := make([]string, 0, len(gone))
	for _, g := range gone {
		args = append(args, "'"+strings.ReplaceAll(g, "'", `'''`)+"'")
	}
	_ = st.run("rm -rf -- " + strings.Join(args, " "))
}

// collectAdditions walks the upper layer and names, in the overlay's own
// coordinates, everything the pristine tree does not have.
func (c *Corpus) collectAdditions(upper, pristine, live string, out *[]string) {
	entries, err := os.ReadDir(upper)
	if err != nil {
		return
	}
	for _, e := range entries {
		p := filepath.Join(pristine, e.Name())
		ps, err := os.Lstat(p)
		if err != nil {
			*out = append(*out, filepath.Join(live, e.Name()))
			continue
		}
		if e.IsDir() && ps.IsDir() {
			c.collectAdditions(filepath.Join(upper, e.Name()), p, filepath.Join(live, e.Name()), out)
		}
	}
}

// dropAdditions deletes everything under live that the pristine tree does not
// have, and leaves everything it does.
//
// Through the overlay and not out of its upper layer, because overlayfs does not
// allow its layers to be changed underneath it. What that costs is one rule:
// removing a name the lower layer has creates a whiteout, which would hide a
// corpus file from every case that ran afterwards, so a name present in both is
// left alone. A case that edited a corpus file keeps the edit in memory for the
// rest of the run, and those are kilobytes; what the run has to get rid of is
// the database volumes, and those exist only above.
//
// A case that *deleted* a corpus file is the same rule seen from the other side:
// its whiteout is invisible from here, so the deletion stands. It costs no
// memory, which is what this is for, and it is the one hole in "clean".
func (c *Corpus) dropAdditions(live, pristine string) {
	if pristine == "" {
		return
	}
	entries, err := os.ReadDir(live)
	if err != nil {
		return
	}
	for _, e := range entries {
		l := filepath.Join(live, e.Name())
		p := filepath.Join(pristine, e.Name())
		ps, err := os.Lstat(p)
		if err != nil {
			_ = os.RemoveAll(l)
			continue
		}
		if e.IsDir() && ps.IsDir() {
			c.dropAdditions(l, p)
		}
	}
}

// used is how many megabytes the tmpfs is holding.
//
// statfs on the tmpfs itself, not on the corpus directory: statfs through an
// overlay answers for its upper layer, so asking the corpus reports the same
// megabytes a second time and a panel adding the two arrives at the ceiling.
func (c *Corpus) used() int {
	var st syscall.Statfs_t
	if err := syscall.Statfs(c.ram, &st); err != nil {
		return 0
	}
	return int(uint64(st.Bsize) * (st.Blocks - st.Bfree) / (1 << 20))
}

// watch samples the ceiling, because a run that fills it does not say so.
//
// A ceiling below what a run needs does not stop the run and does not put ENOSPC
// anywhere a reader will find it: the cases simply fail. One arm of this suite's
// own measurements gave 47 OK against 170 NOK for exactly that reason and
// nothing in the output said why, and a later one filled 6,144 MB of 6,144.
func (c *Corpus) watch() {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-t.C:
			c.mu.Lock()
			if v := c.used(); v > c.peak {
				c.peak = v
			}
			c.mu.Unlock()
		}
	}
}

// Close takes the overlay down and says what the run did with its ceiling.
func (c *Corpus) Close() {
	if c == nil {
		return
	}
	close(c.stop)
	c.mu.Lock()
	if v := c.used(); v > c.peak {
		c.peak = v
	}
	peak, freed := c.peak, c.freed
	c.mu.Unlock()
	if peak*100 >= c.mb*90 {
		fmt.Printf("[ERROR] the corpus used %d MB of its %d MB ceiling. "+
			"Cases that ran out of space fail without saying so; raise scenario_ram_mb "+
			"or lower log_volume_size, and treat this run's verdicts as unusable.\n", peak, c.mb)
	} else {
		fmt.Printf("[INFO] the corpus held at most %d MB of the %d MB it was allowed, "+
			"and %d MB were reclaimed as directories finished\n", peak, c.mb, freed)
	}
	if c.shared {
		_ = c.run("umount " + c.root)
		_ = c.run("umount " + c.pristine)
	}
	_ = c.run("umount " + c.ram)
	_ = os.Remove(c.ram)
	// The slow lane's uppers are on disk and outlive the tmpfs, so they are the
	// one thing here that has to be removed rather than unmounted. Leaving them
	// would put back exactly the 20 GB of leftovers this exists to prevent.
	if c.disk != "" {
		_ = os.RemoveAll(c.disk)
	}
}

func (c *Corpus) run(script string) error {
	out, err := osexec.Command(contain.Shell, "-c", script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", script, err, strings.TrimSpace(string(out)))
	}
	return nil
}
