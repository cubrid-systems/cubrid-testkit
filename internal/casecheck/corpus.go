package casecheck

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cubrid-systems/cubrid-testkit/internal/runner/shellsuite"
)

// Report is what a corpus check produced.
type Report struct {
	// Cases is how many were read.
	Cases int
	// Findings, sorted by case and then by line.
	Findings []Finding
}

// Count is how many cases each rule named.
func (r *Report) Count(rule string) int {
	seen := map[string]bool{}
	for _, f := range r.Findings {
		if f.Rule == rule {
			seen[f.Case] = true
		}
	}
	return len(seen)
}

// Walk checks every case under scenario against the helper library.
//
// The discovery rule is shellsuite's, which is CTP's: a case is
// <name>/cases/<name>.sh and the other scripts under cases/ are helpers it
// calls. Checking those too would report every helper as unable to fail, which
// is true and useless.
func Walk(scenario string, h *Helpers) (*Report, error) {
	root, err := filepath.Abs(scenario)
	if err != nil {
		return nil, err
	}
	rep := &Report{}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".sh") || !shellsuite.IsCase(path) {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return fmt.Errorf("case %s: %w", path, rerr)
		}
		rep.Cases++
		rel, _ := filepath.Rel(root, path)
		rep.Findings = append(rep.Findings, h.Check(rel, string(b))...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(rep.Findings, func(i, j int) bool {
		if rep.Findings[i].Case != rep.Findings[j].Case {
			return rep.Findings[i].Case < rep.Findings[j].Case
		}
		return rep.Findings[i].Line < rep.Findings[j].Line
	})
	return rep, nil
}
