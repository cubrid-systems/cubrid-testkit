package perf

import (
	"fmt"
	"math/rand"
	"path"
	"sort"
	"strings"
	"time"
)

// The order a session runs things in and the time it allows them (Design
// §5.1, §5.9; Spec FR-5, FR-7.1, FR-20.1).

// confKey is a case's cubrid.conf overrides as one string, so cases that can
// share a cluster share a key. The empty key is a case with no override.
func confKey(c *Case) string {
	parts := make([]string, 0, len(c.Conf))
	for k, v := range c.Conf {
		parts = append(parts, k+"="+v)
	}
	sort.Strings(parts)
	return strings.Join(parts, ";")
}

type confGroup struct {
	Key   string
	Cases []*Case
}

// confGroups splits cases by conf, keeping the order they came in: the first
// case of a group decides where the group goes, so a shuffled list stays
// shuffled between groups.
func confGroups(cases []*Case) []confGroup {
	var out []confGroup
	at := map[string]int{}
	for _, c := range cases {
		k := confKey(c)
		i, ok := at[k]
		if !ok {
			i = len(out)
			at[k] = i
			out = append(out, confGroup{Key: k})
		}
		out[i].Cases = append(out[i].Cases, c)
	}
	return out
}

// shuffled is a new order of the cases, from the seed (FR-7.1): the session
// id, so a re-run of the same session walks the same order.
func shuffled(cases []*Case, seed int64) []*Case {
	out := append([]*Case(nil), cases...)
	r := rand.New(rand.NewSource(seed))
	r.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

func seedFrom(s string) int64 {
	var h int64 = 1469598103934665603
	for i := 0; i < len(s); i++ {
		h ^= int64(s[i])
		h *= 1099511628211
	}
	return h
}

// selectCases is the suite's cases a pair runs A/B: the registration's
// cases= when it has one, narrowed by --only.
func selectCases(s *Suite, b *Branch, only string) []*Case {
	var out []*Case
	for _, c := range s.Cases {
		if b != nil && !b.Selects(c.ID) {
			continue
		}
		if only != "" {
			if ok, _ := path.Match(only, c.ID); !ok {
				continue
			}
		}
		out = append(out, c)
	}
	return out
}

// Bounds (§5.9), all upper: the time a thing takes when every pass runs to
// its budget. clock.fits compares them, or pairEstimate, with what is left.
const (
	clusterCreateBound = 2 * time.Minute
	fixtureBound       = 5 * time.Minute
	fixtureSwitchBound = 2 * time.Minute
	judgeBound         = 2 * time.Minute
	remeasurePairs     = 5
)

// caseBound is one case's upper bound: every pass to its budget plus the
// warming and the exec slack (client.go gives the node 60 s more), and the
// fixture switch before it. A defer case's checkpoint before the window gets
// deferCheckpointAllowance a pass, its usual cost with room, not its 10 min
// bound: that bound on every pass would put most pairs past the session's
// budget. pass() logs what each one took.
func caseBound(c *Case) time.Duration {
	passes := (c.Warmup + c.Repeats) * 2
	per := time.Duration(c.BudgetS+c.WarmS)*time.Second + 60*time.Second
	if c.Background == "defer" {
		per += deferCheckpointAllowance
	}
	return time.Duration(passes)*per + fixtureSwitchBound
}

// deferCheckpointAllowance is a defer case's checkpoint wait with room: a
// checkpoint writes about 1,000 pages a second (checkpoint_sleep_msecs), so
// a minute covers 60,000 dirty pages.
const deferCheckpointAllowance = time.Minute

func remeasureBound(c *Case) time.Duration {
	per := time.Duration(c.BudgetS+c.WarmS)*time.Second + 60*time.Second
	if c.Background == "defer" {
		per += deferCheckpointAllowance
	}
	return time.Duration(remeasurePairs*2) * per
}

// pairBound is one pair's upper bound: the clusters it stands up (r, tc, t,
// and the extra ones per conf group), the fixtures each of them builds, the
// canaries, the cases, and the judging.
func pairBound(canaries, cases []*Case, overlap bool) time.Duration {
	sides := pairSides(overlap)
	d := pairSetupBound(canaries, cases, overlap)
	for _, c := range canaries {
		d += caseBound(c)
	}
	for _, c := range cases {
		d += caseBound(c) * time.Duration(sides-1)
	}
	return d
}

func pairSides(overlap bool) int {
	if overlap {
		return 3
	}
	return 2
}

// pairSetupBound is the part of pairBound that is not the passes: the
// clusters, the fixtures and the judging.
func pairSetupBound(canaries, cases []*Case, overlap bool) time.Duration {
	sides := pairSides(overlap)
	groups := len(confGroups(cases))
	if groups == 0 {
		groups = 1
	}
	creates := 2 + sides*groups
	fixtures := 0
	for _, g := range confGroups(canaries) {
		fixtures += 2 * len(fixturesOf(g.Cases))
	}
	for _, g := range confGroups(cases) {
		fixtures += sides * len(fixturesOf(g.Cases))
	}
	return time.Duration(creates)*clusterCreateBound + time.Duration(fixtures)*fixtureBound + judgeBound
}

// groupSetupBound is what a conf group costs before its first case: its
// clusters and their fixtures. The session checks it with the group's first
// case before standing the group up; the case gate starts only after.
func groupSetupBound(g confGroup, sides int) time.Duration {
	return time.Duration(sides) * (clusterCreateBound + time.Duration(len(fixturesOf(g.Cases)))*fixtureBound)
}

// pairTime is a time a pair took, for the estimate: the seconds, the cases
// it ran, its sides (an overlap pair runs each case twice), and where it
// comes from.
type pairTime struct {
	seconds, cases, sides int
	from                  string
}

// timeOf is a pair record's time, or nil when it ran no case or has no
// time: an invalid pair, a session that died before it, a dry run. FR-2's
// previous pair can be any of those; the estimate passes them over.
func timeOf(p *SessionPair, from string) *pairTime {
	if p == nil || p.Seconds <= 0 || p.CasesRun <= 0 {
		return nil
	}
	return &pairTime{seconds: p.Seconds, cases: p.CasesRun, sides: pairSides(p.Overlap != nil), from: from}
}

// pairEstimate is what a pair is expected to take, for the gate before it
// starts. From a time it took (the newest earlier session in which it ran
// cases; else a pair that ran earlier in this session): that time per case
// run there, scaled to the cases now and their sides, times
// pairEstimateMargin, at least the setup part of the bound and at most the
// bound. With no time, the bound. The bound has every pass run to its
// budget and is several times the real time: the pairs of 16 cases took
// 2.1 h on 2026-10-31, and the bound of the 37-case pairs after it is 47 h,
// which put the second pair past the Monday deadline. Within the pair the
// group gate (groupSetupBound) and the case gate (caseBound) still stop a
// session that runs late.
func pairEstimate(canaries, cases []*Case, overlap bool, t *pairTime) (time.Duration, string) {
	bound := pairBound(canaries, cases, overlap)
	switch {
	case len(cases) == 0:
		return bound, "the bound: no case selected"
	case t == nil:
		return bound, "the bound: no earlier time"
	}
	sides := pairSides(overlap)
	secs := float64(t.seconds) / float64(t.cases) * float64(len(cases)) * float64(sides-1) / float64(t.sides-1) * pairEstimateMargin
	floor := pairSetupBound(canaries, cases, overlap).Seconds()
	if secs < floor {
		secs = floor
	}
	if secs >= bound.Seconds() {
		return bound, fmt.Sprintf("the bound: %d s for %d case(s) (%s) scales to %.1f h", t.seconds, t.cases, t.from, secs/3600)
	}
	return time.Duration(secs) * time.Second, fmt.Sprintf("%d s for %d case(s) (%s), scaled to %d, times %.1f; bound %s", t.seconds, t.cases, t.from, len(cases), pairEstimateMargin, bound.Round(time.Minute))
}

// estimate is pairEstimate for this pair's cases from its own earlier time,
// else from fallback.
func (p *SessionPair) estimate(canaries, cases []*Case, fallback *pairTime) (time.Duration, string) {
	t := p.earlier
	if t == nil {
		t = fallback
	}
	return pairEstimate(canaries, cases, p.Overlap != nil, t)
}

// pairEstimateMargin covers cases slower than the earlier ones on average,
// such as a batch of new ones that reload a fixture every pass.
const pairEstimateMargin = 1.5

func fixturesOf(cases []*Case) []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range cases {
		if !seen[c.Fixture.Name] {
			seen[c.Fixture.Name] = true
			out = append(out, c.Fixture.Name)
		}
	}
	sort.Strings(out)
	return out
}

// clock is the session's end: the budget from its start, or the deadline the
// wrapper gave, whichever comes first (L5).
type clock struct {
	End time.Time
}

func (k clock) left(now time.Time) time.Duration {
	return k.End.Sub(now)
}

func (k clock) fits(now time.Time, d time.Duration) bool {
	return k.left(now) >= d
}
