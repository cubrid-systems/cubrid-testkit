package sizing

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// keep is how many runs a suite's file remembers for each machine: enough for a
// few lanes and corpora each to have been run at more than one slot count.
const keep = 10

// Record is one run, as the next run needs to know it.
type Record struct {
	When    string  `json:"when"`
	Machine Machine `json:"machine"`
	Engine  Engine  `json:"engine"`
	// Corpus is the scenario the run was over, and Lane where its slots wrote --
	// "disk", "disk, volatile", memory. A slot count that stopped paying on one
	// says nothing about another.
	Corpus       string `json:"corpus,omitempty"`
	Lane         string `json:"lane,omitempty"`
	Slots        int    `json:"slots"`
	Cases        int    `json:"cases"`
	WallS        int    `json:"wallS"`
	CaseS        int    `json:"caseS"`        // the sum of what the cases took
	LongestUnitS int    `json:"longestUnitS"` // the longest unit that passed on its first attempt
	PeakMB       int    `json:"peakMB"`       // how far available memory fell below where it started
	// SharedMB is what, at that moment, was the run's writes in memory rather
	// than its slots -- shell's corpus tmpfs.
	SharedMB  int `json:"sharedMB,omitempty"`
	PerSlotMB int `json:"perSlotMB"` // (PeakMB - SharedMB) / Slots, which is what a budget is made of
}

type file struct {
	Suite string   `json:"suite"`
	Runs  []Record `json:"runs"`
}

// Load returns this machine's runs of a suite, newest first. Nothing is not an
// error: the first run on a machine is meant to find nothing.
func Load(suite Suite) []Record {
	here := ThisMachine()
	var mine []Record
	for _, r := range read(suite) {
		if r.Machine.same(here) {
			mine = append(mine, r)
		}
	}
	sort.SliceStable(mine, func(i, j int) bool { return mine[i].When > mine[j].When })
	return mine
}

// Save adds a run to a suite's file, keeping the last few of each machine's. A
// failure to save is a failure to help the next run and nothing more, so the
// caller is expected to mention it and carry on.
func Save(suite Suite, r Record) error {
	d := dir()
	if d == "" {
		return fmt.Errorf("no state directory: set TESTKIT_SIZING_DIR")
	}
	if err := os.MkdirAll(d, 0o755); err != nil {
		return err
	}
	if r.When == "" {
		r.When = time.Now().UTC().Format(time.RFC3339)
	}
	if r.Machine.Host == "" {
		r.Machine = ThisMachine()
	}
	if r.PerSlotMB == 0 && r.Slots > 0 && r.PeakMB > r.SharedMB {
		r.PerSlotMB = (r.PeakMB - r.SharedMB) / r.Slots
	}
	runs := append(read(suite), r)
	// Trimmed per machine: another machine sharing the home directory must not
	// push this one's runs out.
	count := map[Machine]int{}
	for _, x := range runs {
		count[x.Machine]++
	}
	kept := runs[:0]
	for _, x := range runs {
		if count[x.Machine] > keep {
			count[x.Machine]--
			continue
		}
		kept = append(kept, x)
	}
	b, err := json.MarshalIndent(file{Suite: string(suite), Runs: kept}, "", "  ")
	if err != nil {
		return err
	}
	// Written beside and renamed over, so that a run killed while saving leaves
	// the last file whole rather than half of a new one.
	tmp, err := os.CreateTemp(d, "."+string(suite)+".*.json")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path(suite))
}

// Path is where a suite's record is, for a message that tells someone where to
// look.
func Path(suite Suite) string { return path(suite) }

func path(suite Suite) string {
	d := dir()
	if d == "" {
		return ""
	}
	return filepath.Join(d, string(suite)+".json")
}

func read(suite Suite) []Record {
	p := path(suite)
	if p == "" {
		return nil
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var f file
	if err := json.Unmarshal(b, &f); err != nil {
		return nil
	}
	return f.Runs
}
