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
	"sort"
	"strings"
	"sync"
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
}

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

// recentMax is how much history the page keeps. Enough to see what just
// happened; the log is where the run is actually recorded.
const recentMax = 40

func New(total int) *Board {
	return &Board{started: time.Now(), total: total, running: map[string]inflight{}}
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
}

type view struct {
	Total    int        `json:"total"`
	Done     int        `json:"done"`
	OK       int        `json:"ok"`
	NOK      int        `json:"nok"`
	Elapsed  int        `json:"elapsed"`
	Remain   int        `json:"remain"`
	Slots    []slotView `json:"slots"`
	Recent   []doneView `json:"recent"`
	Finished bool       `json:"finished"`
}

type slotView struct {
	Slot string `json:"slot"`
	Case string `json:"case"`
	Held int    `json:"held"`
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
			Slot: slot, Case: in.Case, Held: int(time.Since(in.Since).Seconds()),
		})
	}
	sort.Slice(v.Slots, func(i, j int) bool { return v.Slots[i].Slot < v.Slots[j].Slot })
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
