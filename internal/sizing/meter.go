package sizing

import (
	"sync"
	"time"
)

// Meter takes the figures a Record is made of while a run goes on.
//
// Memory is measured as the fall in MemAvailable from where it stood when the
// meter started, because that is the quantity Slots divides: a budget in the
// same units as the thing it is set against. Over the whole isolation corpus it
// came to 3-5% above the resident size of the run's own processes, which does not
// see what the kernel holds for them (evidence/isolation-controller.md §8).
type Meter struct {
	avail  func() int
	shared func() int
	start  time.Time
	stop   chan struct{}
	done   chan struct{}
	once   sync.Once

	mu       sync.Mutex
	baseMB   int
	leastMB  int
	sharedMB int
	cases    map[string]bool
	caseMS   int64
	unitMS   map[string]int64
	unitGone map[string]bool // a unit with an attempt that did not pass first time
}

// StartMeter starts sampling now, every interval. shared, when it is not nil, is
// how much of the machine's memory is the run's writes rather than its slots --
// a corpus in memory -- and is subtracted from what a slot is recorded to cost.
func StartMeter(every time.Duration, shared func() int) *Meter {
	return startMeter(every, MemAvailableMB, shared)
}

func startMeter(every time.Duration, avail, shared func() int) *Meter {
	m := &Meter{avail: avail, shared: shared, start: time.Now(), stop: make(chan struct{}), done: make(chan struct{}),
		cases: map[string]bool{}, unitMS: map[string]int64{}, unitGone: map[string]bool{}}
	m.baseMB = avail()
	m.leastMB = m.baseMB
	go func() {
		defer close(m.done)
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-m.stop:
				return
			case <-t.C:
				m.sample()
			}
		}
	}()
	return m
}

func (m *Meter) sample() {
	a := m.avail()
	s := 0
	if m.shared != nil {
		s = m.shared()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if a > 0 && a < m.leastMB {
		m.leastMB, m.sharedMB = a, s
	}
}

// Case counts one attempt at a case. unit is what a slot runs whole -- the case
// itself, or its directory where a directory is claimed whole -- and first is
// whether this attempt passed and was the case's first. Only a unit whose every
// case passed first time is evidence of how long the corpus's units are: one
// that timed out and was retried measures the timeout.
func (m *Meter) Case(name, unit string, elapsed time.Duration, first bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cases[name] = true
	ms := elapsed.Milliseconds()
	m.caseMS += ms
	m.unitMS[unit] += ms
	if !first {
		m.unitGone[unit] = true
	}
}

// Stop ends the sampling and returns the run as a record. The caller adds what
// the meter cannot know: the slots, the engine, the corpus and the lane. It may
// be called more than once, so that a run can defer it and still read it at the
// end.
func (m *Meter) Stop() Record {
	m.once.Do(func() { close(m.stop) })
	<-m.done
	m.sample()
	m.mu.Lock()
	defer m.mu.Unlock()
	var longest int64
	for u, ms := range m.unitMS {
		if !m.unitGone[u] && ms > longest {
			longest = ms
		}
	}
	r := Record{
		Cases:        len(m.cases),
		WallS:        int(time.Since(m.start).Seconds()),
		CaseS:        int(m.caseMS / 1000),
		LongestUnitS: int((longest + 999) / 1000),
	}
	if m.baseMB > 0 && m.leastMB < m.baseMB {
		r.PeakMB = m.baseMB - m.leastMB
		r.SharedMB = min(m.sharedMB, r.PeakMB)
	}
	return r
}
