package perf

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// builds.json is what the build step leaves for the session (Design §5.2):
// every pair's commits and install trees, resolved before the session starts
// so that the build and the session use the same commit (FR-1.1). The session
// reads it and resolves no symbolic name itself; a pair the file does not
// have, or whose tree is not there, is skipped as "build missing".

const buildsSchema = "perf-builds/1"

// buildsMaxAge is L9's N: the build step runs right before the session, so a
// file older than this is last week's, and a session on last week's commits
// would be reported as this week's.
const buildsMaxAge = 24 * time.Hour

type BuildsManifest struct {
	Schema  string                   `json:"schema"`
	Written string                   `json:"written"`
	Pairs   map[string]*ManifestPair `json:"pairs"`
}

type ManifestPair struct {
	Target    ManifestBuild  `json:"target"`
	Reference ManifestBuild  `json:"reference"`
	Overlap   *ManifestBuild `json:"overlap"`
	Skipped   *string        `json:"skipped"`
	Repo      string         `json:"repo,omitempty"`
	Owner     string         `json:"owner,omitempty"`
	Cases     []string       `json:"cases,omitempty"`
}

type ManifestBuild struct {
	Ref    string `json:"ref"`
	Commit string `json:"commit"`
	Build  string `json:"build"`
}

func readBuilds(path string) (*BuildsManifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m BuildsManifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if m.Schema != buildsSchema {
		return nil, fmt.Errorf("%s: schema %q, want %s", path, m.Schema, buildsSchema)
	}
	return &m, nil
}

// stale says whether the manifest was written too long before now to be this
// session's (L9). A manifest with no readable time is stale.
func (m *BuildsManifest) stale(now time.Time) bool {
	t, err := time.Parse(time.RFC3339, m.Written)
	if err != nil {
		return true
	}
	return now.Sub(t) > buildsMaxAge
}

// SessionPair is one comparison of the session, as session.json carries it
// (Design §4.2) and as the loop fills it in.
type SessionPair struct {
	Name               string         `json:"name"`
	Owner              string         `json:"owner,omitempty"`
	Repo               string         `json:"repo,omitempty"`
	Target             BuildRef       `json:"target"`
	Reference          BuildRef       `json:"reference"`
	Overlap            *BuildRef      `json:"overlap,omitempty"`
	FingerprintChanged bool           `json:"fingerprint_changed"`
	FingerprintNote    string         `json:"fingerprint_note,omitempty"`
	Canaries           []CanaryResult `json:"canaries"`
	Valid              bool           `json:"valid"`
	CasesRun           int            `json:"cases_run"`
	CasesNull          int            `json:"cases_null"`
	CasesSkipped       int            `json:"cases_skipped"`
	Flags              int            `json:"flags"`
	Seconds            int            `json:"seconds"`
	Skipped            string         `json:"skipped,omitempty"`
	Clusters           []string       `json:"clusters,omitempty"`

	branch         *Branch     // nil for a pair perf.conf names
	selected       []*Case     // the cases this pair runs A/B
	entries        []CaseEntry // what the sidecar got, for the summary
	overlapEntries []CaseEntry // the same against the overlap reference (FR-29)
	written        bool        // the pair's sidecar is on disk: the pair ran
}

type CanaryResult struct {
	ID        string   `json:"id"`
	Ratio     *float64 `json:"ratio"`
	Tolerance float64  `json:"tolerance"`
	OK        bool     `json:"ok"`
	Status    string   `json:"status"`
	Reason    string   `json:"reason,omitempty"`
}

func isInstallTree(dir string) bool {
	return dir != "" && exists(filepath.Join(dir, "bin", "cub_server"))
}

// pairsFrom is Design §5.2's reader: the conf's pairs in file order, then the
// active registrations in registration order, each resolved through the
// manifest. A conf pair whose two sides are install trees needs no build
// step and is taken as written; everything symbolic needs the manifest.
func pairsFrom(c *Conf, branches []Branch, m *BuildsManifest, stale bool) []*SessionPair {
	var out []*SessionPair
	lookup := func(name string) *ManifestPair {
		if m == nil {
			return nil
		}
		return m.Pairs[name]
	}
	resolve := func(p *SessionPair, mp *ManifestPair) {
		if mp.Skipped != nil && *mp.Skipped != "" {
			p.Skipped = *mp.Skipped
			return
		}
		p.Target = BuildRef{Ref: mp.Target.Ref, Commit: mp.Target.Commit, Build: mp.Target.Build}
		p.Reference = BuildRef{Ref: mp.Reference.Ref, Commit: mp.Reference.Commit, Build: mp.Reference.Build}
		if mp.Overlap != nil && mp.Overlap.Build != "" {
			p.Overlap = &BuildRef{Ref: mp.Overlap.Ref, Commit: mp.Overlap.Commit, Build: mp.Overlap.Build}
		}
	}
	for _, pr := range c.Pairs {
		p := &SessionPair{Name: pr.Name, Canaries: []CanaryResult{}}
		switch mp := lookup(pr.Name); {
		case stale:
			p.Skipped = "builds stale"
		case mp != nil:
			resolve(p, mp)
		case isInstallTree(pr.Target) && isInstallTree(pr.Reference) && (pr.Overlap == "" || isInstallTree(pr.Overlap)):
			p.Target = BuildRef{Ref: pr.Target, Build: pr.Target}
			p.Reference = BuildRef{Ref: pr.Reference, Build: pr.Reference}
			if pr.Overlap != "" {
				p.Overlap = &BuildRef{Ref: pr.Overlap, Build: pr.Overlap}
			}
		default:
			p.Skipped = "build missing"
		}
		checkTrees(p)
		out = append(out, p)
	}
	for i := range branches {
		b := branches[i]
		p := &SessionPair{Name: b.Name, Owner: b.Owner, Repo: b.Repo, Canaries: []CanaryResult{}, branch: &b}
		switch mp := lookup(b.Name); {
		case stale:
			p.Skipped = "builds stale"
		case mp == nil:
			p.Skipped = "build missing"
		default:
			resolve(p, mp)
			if p.Owner == "" {
				p.Owner = mp.Owner
			}
		}
		checkTrees(p)
		out = append(out, p)
	}
	return out
}

// checkTrees turns a resolved pair whose install tree is not there into a
// skipped one, naming the tree.
func checkTrees(p *SessionPair) {
	if p.Skipped != "" {
		return
	}
	refs := []*BuildRef{&p.Target, &p.Reference}
	if p.Overlap != nil {
		refs = append(refs, p.Overlap)
	}
	for _, b := range refs {
		if !isInstallTree(b.Build) {
			p.Skipped = fmt.Sprintf("build missing: %s is not an install tree", b.Build)
			return
		}
	}
}

// fingerprints reads each build's fingerprint and settles the commit: the
// manifest's when it has one, else what the build says about itself.
func (p *SessionPair) fingerprints() {
	refs := []*BuildRef{&p.Target, &p.Reference}
	if p.Overlap != nil {
		refs = append(refs, p.Overlap)
	}
	for _, b := range refs {
		b.Fingerprint = readFingerprint(b.Build)
		if b.Commit == "" {
			b.Commit = b.Fingerprint.Commit
		}
	}
}

// debugBuild names the side that is a Debug build, or "" (Spec §12).
func (p *SessionPair) debugBuild() string {
	for _, b := range []struct {
		name string
		ref  *BuildRef
	}{{"target", &p.Target}, {"reference", &p.Reference}, {"overlap", p.Overlap}} {
		if b.ref != nil && b.ref.Fingerprint.BuildType == "Debug" {
			return b.name + " " + b.ref.Build
		}
	}
	return ""
}
