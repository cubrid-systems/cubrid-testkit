package shellsuite

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Patches are the corpus changes a run needs and does not own.
//
// Some cases fail for reasons that are neither the engine's nor this runner's.
// They assume the layout of the machine that has always run them: that $CUBRID
// sits under $HOME, that [common] is empty, that the linker resolves libraries
// in an order it has not for years. Fixing the corpus is right and belongs
// upstream, where it is one pull request per case and someone else's review.
// Until then a run can either fail on them or carry the change.
//
// Carrying it as a patch file, applied at the moment the case runs, keeps three
// properties that matter:
//
//   - The corpus on disk is not modified. Writes go to the run's overlay and
//     are dropped when the directory retires, so the checkout comes out as it
//     went in and two runs of different vintages do not fight over it.
//   - The change is a diff. It can be read, reviewed, and sent upstream as it
//     stands, and it stops applying the moment the case it patches changes --
//     which is the signal that upstream has moved.
//   - It is visible. A verdict produced from patched source is not the same
//     claim as a verdict produced from the corpus, and the run says which is
//     which -- on standard output, in the case log, and on the page.
//
// The layout mirrors the corpus under a category directory, so finding the
// patch for a case is finding the case:
//
//	patches/shell/_08_shard/_02_cubrid_broker01/cases/_02_cubrid_broker01.sh.patch
//
// The diff's own paths are relative to the case directory, so one patch may
// touch the case script and its answer files together.
type Patches struct {
	// dir is the category root, e.g. patches/shell.
	dir string
	// byCase maps a case script to the patch file for it.
	byCase map[string]string
	// orphans are patches with no case in this corpus. Reported, not fatal: a
	// corpus filtered to one family should not have to carry every patch.
	orphans []string
}

// LoadPatches indexes dir against the cases this run will execute.
func LoadPatches(dir, scenario string, cases []string) (*Patches, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, nil
	}
	root, err := filepath.Abs(scenario)
	if err != nil {
		return nil, err
	}
	p := &Patches{dir: dir, byCase: map[string]string{}}

	// Index by the path a case would have relative to the scenario, so the
	// lookup is a map hit rather than a walk per case.
	want := map[string]string{}
	for _, c := range cases {
		rel, err := filepath.Rel(root, c)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		want[rel] = c
	}
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".patch") {
			return err
		}
		rel, rerr := filepath.Rel(dir, path)
		if rerr != nil {
			return nil
		}
		rel = strings.TrimSuffix(rel, ".patch")
		if c, ok := want[rel]; ok {
			p.byCase[c] = path
		} else {
			p.orphans = append(p.orphans, rel)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", dir, err)
	}
	sort.Strings(p.orphans)
	return p, nil
}

// For returns the patch file for a case, or "".
func (p *Patches) For(script string) string {
	if p == nil {
		return ""
	}
	return p.byCase[script]
}

// Count is how many of this run's cases carry a patch.
func (p *Patches) Count() int {
	if p == nil {
		return 0
	}
	return len(p.byCase)
}

// Describe is the line the run prints, because a corpus that was changed before
// it ran is the first thing a reader of the verdicts needs to know.
func (p *Patches) Describe() []string {
	if p == nil || (len(p.byCase) == 0 && len(p.orphans) == 0) {
		return nil
	}
	out := []string{fmt.Sprintf(
		"[INFO] %d case(s) will run against a compatibility patch from %s. "+
			"Their verdicts are about the patched case, not the corpus.", len(p.byCase), p.dir)}
	names := make([]string, 0, len(p.byCase))
	for c := range p.byCase {
		names = append(names, c)
	}
	sort.Strings(names)
	for _, c := range names {
		out = append(out, "[INFO]   "+c)
	}
	for _, o := range p.orphans {
		out = append(out, "[WARN]   "+o+": a patch with no case in this corpus")
	}
	return out
}

// ApplyScript applies one patch inside the case's directory.
//
// --forward so a patch already applied is not reversed, and a dry run first so
// a patch that does not fit changes nothing before it is refused. Refusing is
// the point: a patch that no longer applies means the case has moved, and
// running the case unpatched would answer a question nobody asked.
func ApplyScript(caseDir, patchFile string) string {
	d, f := shellQuote(caseDir), shellQuote(patchFile)
	return "patch -p0 --batch --forward --dry-run -d " + d + " -i " + f + " >/dev/null 2>&1 && " +
		"patch -p0 --batch --forward -d " + d + " -i " + f
}
