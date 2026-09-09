package status

import "sync"

// Setting is one line of how the run was set up.
//
// A page that shows what a run is doing without showing what it was told to do
// leaves the reader to guess, and the guesses are wrong in a specific way: the
// engine's defaults are not what the suite runs on. db_volume_size alone moves a
// case's footprint by a factor of twenty-five, and whether the server is waited
// for changes what a timing failure means. Both are invisible in the verdicts.
//
// Default is what the value would be if nothing had set it, and empty when there
// is nothing to compare against -- a suite key that has no engine default, or a
// switch that is simply on or off. A Value that differs from a non-empty Default
// is the interesting case and the page says so.
type Setting struct {
	Group   string `json:"group"`
	Key     string `json:"key"`
	Value   string `json:"value"`
	Default string `json:"default,omitempty"`
	Note    string `json:"note,omitempty"`
}

// Changed reports whether this setting departs from the default it names.
func (s Setting) Changed() bool { return s.Default != "" && s.Value != s.Default }

type setupBox struct {
	mu   sync.Mutex
	rows []Setting
}

// Setup records how the run was configured. Called once, before the first case,
// and served with every snapshot: it is small, it never changes, and a reader
// who opens the page an hour in needs it as much as one who was there at the
// start.
func (b *Board) Setup(rows []Setting) {
	if b == nil {
		return
	}
	b.setup.mu.Lock()
	defer b.setup.mu.Unlock()
	b.setup.rows = append([]Setting(nil), rows...)
}

func (b *Board) setupRows() []Setting {
	b.setup.mu.Lock()
	defer b.setup.mu.Unlock()
	return append([]Setting(nil), b.setup.rows...)
}
