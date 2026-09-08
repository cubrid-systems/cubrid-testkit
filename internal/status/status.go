// Package status shows what a run is doing while it does it.
//
// Not on standard output. What the runner prints there is a frozen surface --
// ADR-003 -- and the comparison that proves this system equivalent reads it, so
// a screen drawn over it would be a screen drawn over the evidence. A page
// served on a port touches none of it.
//
// It exists because of slots. One worker's progress is legible in the log it
// writes; ten workers' is not, and the question that matters during a run --
// which slot is stuck, and on what -- has no answer in a stream of finished
// cases.
package status

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// DefaultAddr is where the page goes when the configuration asks for one
// without saying where.
//
// In the private range, 49152-65535, so it cannot collide with a registered
// service, and not 8080, which everything else on a developer's machine is
// already using. The digits are CUBRID's own 1523 with a 5 in front, which is
// the only reason this number rather than another.
const DefaultAddr = "127.0.0.1:51523"

// Addr reads what the configuration said. A bare "on" takes DefaultAddr, a bare
// port takes every interface, and anything else is passed through as written.
//
// Loopback by default rather than every interface: a QA machine's run should not
// become a page the rest of the network can read because someone turned it on.
func Addr(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "":
		return ""
	case "on", "yes", "true", "1":
		return DefaultAddr
	}
	v = strings.TrimSpace(v)
	if !strings.Contains(v, ":") {
		return ":" + v
	}
	return v
}

// Board is what the workers report to and the page reads from.
//
// A run must not fail because nobody was watching it, so every method here is
// safe on a nil Board: the runner passes one when a port was asked for and nil
// when it was not, and no call site has to know which.
type Board struct {
	mu      sync.Mutex
	started time.Time
	total   int
	running map[string]inflight
	done    int
	ok      int
	recent  []finished
	// failed is every case that failed, not the failures among the last few.
	// A run of 217 with 56 failures is a run where the list of failures is the
	// thing being watched, and filtering a recency window would answer a
	// different question.
	failed []finished
	// buckets counts completions per bucketSpan, so the page can show whether
	// the run is keeping pace. With slots that is the question the numbers alone
	// do not answer: ten workers finishing nothing looks like one worker on a
	// long case until you see the rate go flat.
	buckets []int
	bucket0 time.Time
	// hist counts completions by how long they took, and byFamily and bySlot
	// aggregate them. All three are counters rather than lists: a corpus of
	// 3,452 cases should not be held twice so that a page can add it up.
	// corpusDir and ramDir are what the machine panel reports free space for,
	// set by the runner when it knows them.
	corpusDir string
	ramDir    string
	ramCap    int

	hist []int
	// histSecs is the same buckets weighted by time rather than by count, and
	// the two together are the lane decision. Counting cases says the corpus is
	// mostly short cases; counting seconds says a handful of long ones own the
	// run -- six cases of _01_sqlx measured 0, 6, 19, 182, 183 and 183 seconds,
	// so three of six are 96% of the time. A lane split is a threshold on this
	// panel, and without the second series the panel cannot show where to put
	// it.
	histSecs []int
	byFamily map[string]*tally
	bySlot   map[string]*tally
	// laneOf is where a slot's writes go, and byLane adds the cases up by it.
	// One lane today -- every slot's corpus writes go to the same place -- which
	// is why the panel reads "ram 8 slots" rather than a split. It is here
	// because the split is the next thing (B-T13) and because even undivided it
	// answers a question the other panels do not: what fraction of the run's
	// case-seconds is holding memory.
	laneOf map[string]string
	byLane map[string]*tally
}

// tally is what is known about a group of cases without keeping the cases.
type tally struct {
	Done int
	OK   int
	Secs int
	Max  int
}

// histEdges are the upper bounds of the duration buckets, in seconds. Chosen
// from the corpus rather than round numbers: the median is 11 s, the fixed cost
// of createdb, start, stop and delete is about 2.6, and the longest case
// measured is 227 -- so the interesting detail is between 2 and 30, and above
// 120 all that matters is that something is there.
var histEdges = []int{2, 5, 10, 20, 30, 60, 120, 300}

// bucketSpan is the resolution of the rate. Ten seconds is short enough that a
// stall shows within one screen refresh and long enough that a single case
// finishing does not read as a spike.
const bucketSpan = 10 * time.Second

// bucketMax bounds the series at half an hour of history.
const bucketMax = 180

type inflight struct {
	Case  string
	Since time.Time
}

type finished struct {
	Slot string
	Case string
	OK   bool
	Took time.Duration
	At   time.Time
}

// recentMax is how much of the run's tail the page keeps. Enough to see what
// just happened; the log is where the run is actually recorded.
const recentMax = 40

// failedMax bounds the failure list. Above this the page is not the tool for
// the job -- a run failing a thousand cases is read in the result tree -- but
// it is far enough above a bad day that the list stays complete on one.
const failedMax = 500

func New(total int) *Board {
	return &Board{started: time.Now(), total: total, running: map[string]inflight{},
		hist:     make([]int, len(histEdges)+1),
		histSecs: make([]int, len(histEdges)+1),
		byFamily: map[string]*tally{}, bySlot: map[string]*tally{},
		laneOf: map[string]string{}, byLane: map[string]*tally{}}
}

// Lane says where a slot's corpus writes go, so the page can group by it.
//
// Called once per slot when the run opens it. A slot with no lane is still a
// slot: the panel leaves it out rather than inventing a name for it.
func (b *Board) Lane(slot, lane string) {
	if b == nil || lane == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.laneOf[slot] = lane
	if b.byLane[lane] == nil {
		b.byLane[lane] = &tally{}
	}
	// And the slot itself, so it is in the by-slot table from the start.
	//
	// The table used to be built only from cases that had finished, which hid
	// exactly the slots worth looking at: the slow lane's three slots were on
	// 195, 166 and 125-second cases, so for the first three minutes of a run the
	// panel showed five slots of eight and said nothing about the other three.
	// The same shape of bug as a disk-free row that disappears when it reaches
	// zero.
	if b.bySlot[slot] == nil {
		b.bySlot[slot] = &tally{}
	}
}

func (b *Board) Begin(slot, name string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.running[slot] = inflight{Case: name, Since: time.Now()}
}

func (b *Board) End(slot, name string, ok bool) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	took := time.Duration(0)
	if in, live := b.running[slot]; live {
		took = time.Since(in.Since)
	}
	delete(b.running, slot)
	b.done++
	if ok {
		b.ok++
	}
	b.recent = append(b.recent, finished{Slot: slot, Case: name, OK: ok, Took: took, At: time.Now()})
	if len(b.recent) > recentMax {
		b.recent = b.recent[len(b.recent)-recentMax:]
	}
	if !ok {
		b.failed = append(b.failed, finished{Slot: slot, Case: name, Took: took, At: time.Now()})
		if len(b.failed) > failedMax {
			b.failed = b.failed[len(b.failed)-failedMax:]
		}
	}
	secs := int(took.Seconds())
	bucket := bucketOf(secs)
	b.hist[bucket]++
	b.histSecs[bucket] += secs
	groups := map[string]map[string]*tally{familyOf(name): b.byFamily, slot: b.bySlot}
	if lane := b.laneOf[slot]; lane != "" {
		groups[lane] = b.byLane
	}
	for key, m := range groups {
		t := m[key]
		if t == nil {
			t = &tally{}
			m[key] = t
		}
		t.Done++
		t.Secs += secs
		if ok {
			t.OK++
		}
		if secs > t.Max {
			t.Max = secs
		}
	}
	b.count(time.Now())
}

func bucketOf(secs int) int {
	for i, e := range histEdges {
		if secs < e {
			return i
		}
	}
	return len(histEdges)
}

// familyOf is the corpus's own grouping: the first path segment named the way
// the families are, _NN_something. A case two levels down -- _06_issues/_11_1h
// -- counts under the family, because that is the unit anyone asks about.
func familyOf(path string) string {
	for _, seg := range strings.Split(path, "/") {
		if len(seg) > 3 && seg[0] == '_' && seg[1] >= '0' && seg[1] <= '9' {
			return seg
		}
	}
	return "(other)"
}

// count puts one completion in its bucket, filling the empty buckets in
// between: a gap has to appear in the series as zeroes, or a stall would draw
// as a straight line from before it to after.
func (b *Board) count(at time.Time) {
	if b.bucket0.IsZero() {
		b.bucket0 = b.started
	}
	want := int(at.Sub(b.bucket0)/bucketSpan) + 1
	for len(b.buckets) < want {
		b.buckets = append(b.buckets, 0)
	}
	b.buckets[want-1]++
	if len(b.buckets) > bucketMax {
		drop := len(b.buckets) - bucketMax
		b.buckets = b.buckets[drop:]
		b.bucket0 = b.bucket0.Add(time.Duration(drop) * bucketSpan)
	}
}

type view struct {
	Total    int         `json:"total"`
	Done     int         `json:"done"`
	OK       int         `json:"ok"`
	NOK      int         `json:"nok"`
	Elapsed  int         `json:"elapsed"`
	Remain   int         `json:"remain"`
	Slots    []slotView  `json:"slots"`
	Recent   []doneView  `json:"recent"`
	Failed   []doneView  `json:"failed"`
	Rate     []int       `json:"rate"`
	RateSpan int         `json:"rateSpan"`
	Hist     []int       `json:"hist"`
	HistEdge []int       `json:"histEdge"`
	HistSecs []int       `json:"histSecs"`
	Family   []groupView `json:"family"`
	Slot     []groupView `json:"slot"`
	Lanes    []laneView  `json:"lanes"`
	Machine  machineView `json:"machine"`
	Finished bool        `json:"finished"`
}

type slotView struct {
	Slot string `json:"slot"`
	Lane string `json:"lane"`
	Case string `json:"case"`
	Held int    `json:"held"`
}

// laneView is one lane added up. Share is the fraction of the run's case-seconds
// that ran in it, which is the number a lane split exists to change: a lane on
// memory that holds 5% of the seconds is memory spent where it does not pay.
type laneView struct {
	Name string `json:"name"`
	// Slots is which slots are in this lane, compacted into ranges. A count
	// alone does not answer the question the panel is read for -- when a slot is
	// stuck, which lane is it in.
	Slots  string `json:"slots"`
	NSlots int    `json:"nslots"`
	Done   int    `json:"done"`
	NOK    int    `json:"nok"`
	Secs   int    `json:"secs"`
	Share  int    `json:"share"`
}

// groupView is a family or a slot, added up.
type groupView struct {
	Name string `json:"name"`
	Lane string `json:"lane,omitempty"`
	Done int    `json:"done"`
	OK   int    `json:"ok"`
	NOK  int    `json:"nok"`
	Secs int    `json:"secs"`
	Max  int    `json:"max"`
}

// machineView is what the run is competing for. Everything this project has
// measured about parallelism is a resource question, and none of it was visible
// while a run was going: a slot holding a case for four minutes reads the same
// whether it is waiting on a lock or on a disk with nothing left to give.
type machineView struct {
	Load    float64 `json:"load"`
	Cores   int     `json:"cores"`
	MemUsed int     `json:"memUsed"`
	MemAll  int     `json:"memAll"`
	Corpus  int     `json:"corpus"`
	Ram     int     `json:"ram"`
	RamCap  int     `json:"ramCap"`
}

type doneView struct {
	Slot string `json:"slot"`
	Case string `json:"case"`
	OK   bool   `json:"ok"`
	Took int    `json:"took"`
}

func (b *Board) snapshot() view {
	b.mu.Lock()
	defer b.mu.Unlock()
	elapsed := time.Since(b.started)
	v := view{
		Total: b.total, Done: b.done, OK: b.ok, NOK: b.done - b.ok,
		Elapsed:  int(elapsed.Seconds()),
		Finished: b.done >= b.total && len(b.running) == 0,
	}
	// Remaining time from the rate so far. Wrong early and wrong for a corpus
	// whose long cases are all at the end, which is the reason to order the
	// queue by duration rather than to make this cleverer.
	if b.done > 0 && b.total > b.done {
		per := elapsed / time.Duration(b.done)
		v.Remain = int((per * time.Duration(b.total-b.done)).Seconds())
	}
	for slot, in := range b.running {
		v.Slots = append(v.Slots, slotView{
			Slot: slot, Lane: b.laneOf[slot], Case: in.Case,
			Held: int(time.Since(in.Since).Seconds()),
		})
	}
	sort.Slice(v.Slots, func(i, j int) bool { return v.Slots[i].Slot < v.Slots[j].Slot })
	// The series runs to now, not to the last completion, so a stall is visible
	// as it happens rather than only once something finishes.
	b.count(time.Now())
	b.buckets[len(b.buckets)-1]--
	for i := len(b.failed) - 1; i >= 0; i-- {
		f := b.failed[i]
		v.Failed = append(v.Failed, doneView{
			Slot: f.Slot, Case: f.Case, OK: false, Took: int(f.Took.Seconds()),
		})
	}
	v.Rate = append([]int(nil), b.buckets...)
	v.RateSpan = int(bucketSpan.Seconds())
	v.Hist = append([]int(nil), b.hist...)
	v.HistSecs = append([]int(nil), b.histSecs...)
	v.HistEdge = append([]int(nil), histEdges...)
	v.Family = groups(b.byFamily)
	v.Slot = groups(b.bySlot)
	// By name, not by seconds: the lanes are contiguous blocks of slots, so
	// name order groups them and a slot does not move as it works. The family
	// table wants the slowest first; this one wants to sit still.
	sort.Slice(v.Slot, func(i, j int) bool { return slotLess(v.Slot[i].Name, v.Slot[j].Name) })
	for i := range v.Slot {
		v.Slot[i].Lane = b.laneOf[v.Slot[i].Name]
	}
	v.Lanes = b.lanes()
	v.Machine = machine(b.corpusDir, b.ramDir, b.ramCap)
	for i := len(b.recent) - 1; i >= 0; i-- {
		f := b.recent[i]
		v.Recent = append(v.Recent, doneView{
			Slot: f.Slot, Case: f.Case, OK: f.OK, Took: int(f.Took.Seconds()),
		})
	}
	return v
}

// Serve starts the page and returns the address it is on and a way to stop it.
//
// The listener is opened before returning, so a port already in use is an error
// the caller sees rather than a page that silently never appears.
func (b *Board) Serve(addr string) (string, func(), error) {
	if b == nil {
		return "", func() {}, nil
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", nil, fmt.Errorf("status page: %w", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(b.snapshot())
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	return ln.Addr().String(), func() { _ = srv.Close() }, nil
}

// Watch tells the board where the run's disk and memory actually are.
//
// diskDir must not be the corpus directory once that has an overlay on it: a
// statfs there answers for the upper layer, which is the tmpfs, so the panel
// reported the same filesystem twice and called one of them disk. It is the
// install instead, which is on the disk the run actually writes to.
func (b *Board) Watch(diskDir, ramDir string, ramCapMB int) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.corpusDir, b.ramDir, b.ramCap = diskDir, ramDir, ramCapMB
}

// groups sorts the tallies slowest-first: the question is which family or slot
// is costing the run, and an alphabetical list does not answer it.
func groups(m map[string]*tally) []groupView {
	out := make([]groupView, 0, len(m))
	for name, t := range m {
		out = append(out, groupView{
			Name: name, Done: t.Done, OK: t.OK, NOK: t.Done - t.OK,
			Secs: t.Secs, Max: t.Max,
		})
	}
	// Slowest first, and by name where that ties. sort.Slice is not stable, so
	// without the second key equal rows swap places on every poll and a table
	// that is refreshed once a second never sits still.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Secs != out[j].Secs {
			return out[i].Secs > out[j].Secs
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// machine reads what the run is competing for. Straight from /proc every time
// the page asks, once a second, because caching a number this cheap would be
// one more thing that can be stale.
func machine(corpusDir, ramDir string, ramCap int) machineView {
	v := machineView{Cores: runtime.NumCPU(), RamCap: ramCap}
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		if f := strings.Fields(string(b)); len(f) > 0 {
			v.Load, _ = strconv.ParseFloat(f[0], 64)
		}
	}
	if b, err := os.ReadFile("/proc/meminfo"); err == nil {
		var total, avail int
		for _, line := range strings.Split(string(b), "\n") {
			f := strings.Fields(line)
			if len(f) < 2 {
				continue
			}
			n, _ := strconv.Atoi(f[1])
			switch f[0] {
			case "MemTotal:":
				total = n / 1024
			case "MemAvailable:":
				avail = n / 1024
			}
		}
		v.MemAll, v.MemUsed = total, total-avail
	}
	v.Corpus = freeMB(corpusDir)
	v.Ram = usedMB(ramDir)
	return v
}

func freeMB(dir string) int {
	if dir == "" {
		return 0
	}
	var st syscall.Statfs_t
	if syscall.Statfs(dir, &st) != nil {
		return 0
	}
	return int(uint64(st.Bavail) * uint64(st.Bsize) / (1 << 20))
}

func usedMB(dir string) int {
	if dir == "" {
		return 0
	}
	var st syscall.Statfs_t
	if syscall.Statfs(dir, &st) != nil {
		return 0
	}
	return int((uint64(st.Blocks) - uint64(st.Bavail)) * uint64(st.Bsize) / (1 << 20))
}

// lanes adds the run up by lane, with each lane's share of the case-seconds.
//
// Ordered by share, largest first, and by name where that ties -- the same
// reason the family table needs a second key: the page refreshes once a second
// and sort.Slice is not stable, so tied rows would swap places on every poll.
func (b *Board) lanes() []laneView {
	if len(b.byLane) == 0 {
		return nil
	}
	members := map[string][]string{}
	for slot, lane := range b.laneOf {
		members[lane] = append(members[lane], slot)
	}
	total := 0
	for _, t := range b.byLane {
		total += t.Secs
	}
	out := make([]laneView, 0, len(b.byLane))
	for name, t := range b.byLane {
		share := 0
		if total > 0 {
			share = t.Secs * 100 / total
		}
		out = append(out, laneView{
			Name: name, Slots: compactSlots(members[name]), NSlots: len(members[name]),
			Done: t.Done, NOK: t.Done - t.OK, Secs: t.Secs, Share: share,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Secs != out[j].Secs {
			return out[i].Secs > out[j].Secs
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// slotLess orders slot0, slot1, ... slot10 the way a reader expects rather than
// the way strings sort, where slot10 comes before slot2.
func slotLess(a, b string) bool {
	na, oka := slotNum(a)
	nb, okb := slotNum(b)
	if oka && okb && na != nb {
		return na < nb
	}
	return a < b
}

func slotNum(s string) (int, bool) {
	i := len(s)
	for i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		i--
	}
	if i == len(s) {
		return 0, false
	}
	n, err := strconv.Atoi(s[i:])
	return n, err == nil
}

// compactSlots turns slot0,slot1,slot2,slot5 into "slot0-slot2, slot5".
//
// Sixteen slot names on one line is a line nobody reads, and the lanes are
// contiguous by construction -- the fast slots are the first ones -- so almost
// always this is one range a lane.
func compactSlots(names []string) string {
	if len(names) == 0 {
		return ""
	}
	sorted := append([]string(nil), names...)
	sort.Slice(sorted, func(i, j int) bool { return slotLess(sorted[i], sorted[j]) })

	var parts []string
	run := 0 // index in sorted where the current run started
	flush := func(end int) {
		if end == run {
			parts = append(parts, sorted[run])
			return
		}
		parts = append(parts, sorted[run]+"-"+sorted[end])
	}
	for i := 1; i <= len(sorted); i++ {
		contiguous := false
		if i < len(sorted) {
			a, oka := slotNum(sorted[i-1])
			b, okb := slotNum(sorted[i])
			contiguous = oka && okb && b == a+1
		}
		if !contiguous {
			flush(i - 1)
			run = i
		}
	}
	return strings.Join(parts, ", ")
}
