package shellsuite

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
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
// The name of a patch is the case's path under the scenario, flattened:
//
//	_06_issues/_17_1h/cbrd_20760_1/cases/cbrd_20760_1.sh
//	  -> patches/shell/_06_issues~_17_1h~cbrd_20760_1.patch
//
// Flat rather than a mirror of the corpus, because the corpus is five levels
// deep and this set is not: `ls patches/shell` should show everything a run
// carries, on one screen. Two segments come out on the way -- "cases", which
// every case has, and a file name that repeats its directory, which almost every
// case has. Nothing collides: a case always lives under cases/, so no case maps
// to the name another case would.
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

	mu sync.Mutex
	// applied is what was actually used, which is not the same as what was
	// found: a patch that refuses to apply fails its case instead.
	applied map[string]string
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

	// Index by the name a case's patch would have, so the lookup is a map hit
	// rather than a walk per case.
	want := map[string]string{}
	for _, c := range cases {
		if n := PatchName(root, c); n != "" {
			want[n] = c
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("case_patch_dir %s does not exist", dir)
		}
		return nil, fmt.Errorf("cannot read %s: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".patch") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".patch")
		if c, ok := want[name]; ok {
			p.byCase[c] = filepath.Join(dir, e.Name())
		} else {
			p.orphans = append(p.orphans, name)
		}
	}
	sort.Strings(p.orphans)
	return p, nil
}

// PatchName is what a case's patch file is called, without the extension.
//
// The two segments that come out are the ones that carry no information: the
// "cases" every case sits in, and a file name that repeats its directory.
func PatchName(scenarioRoot, script string) string {
	rel, err := filepath.Rel(scenarioRoot, script)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	parts := strings.Split(strings.TrimSuffix(rel, ".sh"), string(filepath.Separator))
	kept := parts[:0]
	for _, s := range parts {
		if s != "cases" {
			kept = append(kept, s)
		}
	}
	if n := len(kept); n > 1 && kept[n-1] == kept[n-2] {
		kept = kept[:n-1]
	}
	return strings.Join(kept, "~")
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
	d, f := shQuote(caseDir), shQuote(patchFile)
	return "patch -p0 --batch --forward --dry-run -d " + d + " -i " + f + " >/dev/null 2>&1 && " +
		"patch -p0 --batch --forward -d " + d + " -i " + f
}

// RevertScript puts the case back.
//
// Behind the corpus overlay this is redundant: the writes are in the upper layer
// and go when the directory retires. It is here because that is a property of
// how the run was configured and not of the patch, and "does the corpus come out
// as it went in" must not have "it depends" as its answer. A run without
// scenario_ram_mb writes straight into the checkout, and a patch left there
// would be applied to a case the next run reads from git.
//
// A dry run first for the same reason as forward: if the case's own script was
// changed underneath -- which nothing should do, but this is the check that says
// so -- the revert is refused rather than making it worse, and the caller
// reports it.
func RevertScript(caseDir, patchFile string) string {
	d, f := shQuote(caseDir), shQuote(patchFile)
	// --forward alongside --reverse is what stops a second revert re-applying
	// the patch forwards: to a reversed run, an already-reverted file looks like
	// a reversed patch, and --forward skips those instead of "fixing" them.
	return "patch -p0 --batch --reverse --forward --dry-run -d " + d + " -i " + f + " >/dev/null 2>&1 && " +
		"patch -p0 --batch --reverse --forward -d " + d + " -i " + f
}

// Applied records that a patch was used, and Report writes the record out.
//
// The startup line says what a run intends to patch; this says what it did. The
// two differ when a patch refuses to apply, and a reader of finished results has
// only the files -- feedback.log keeps a case's console output for failures
// only, so an OK case that ran patched leaves no trace there at all.
//
// A file of its own rather than a new line in an existing one: what the runner
// prints on standard output and writes into the result tree is CTP's shape and
// is frozen (ADR-003). A file nothing else reads adds nothing to parse.
func (p *Patches) Applied(script, patchFile string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.applied == nil {
		p.applied = map[string]string{}
	}
	p.applied[script] = patchFile
}

// Report writes the record into the result directory. Nothing is written when
// nothing was patched, so the file's presence is itself the answer to "did this
// run patch anything".
func (p *Patches) Report(dir string) error {
	if p == nil || dir == "" {
		return nil
	}
	p.mu.Lock()
	names := make([]string, 0, len(p.applied))
	for c := range p.applied {
		names = append(names, c)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("# Cases this run did not execute as the corpus has them.\n")
	b.WriteString("# Their verdicts are about the patched case. See patches/README.md.\n")
	for _, c := range names {
		fmt.Fprintf(&b, "%s\t%s\n", c, p.applied[c])
	}
	n := len(names)
	p.mu.Unlock()

	if n == 0 {
		return nil
	}
	return os.WriteFile(filepath.Join(dir, "patched.txt"), []byte(b.String()), 0o644)
}
