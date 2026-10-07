package perf

import (
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
// its budget. budgetFits compares them with what is left.
const (
	clusterCreateBound = 2 * time.Minute
	fixtureBound       = 5 * time.Minute
	fixtureSwitchBound = 2 * time.Minute
	judgeBound         = 2 * time.Minute
	remeasurePairs     = 5
)

// caseBound is one case's upper bound: every pass to its budget plus the
// warming and the exec slack (client.go gives the node 60 s more), and the
// fixture switch before it.
func caseBound(c *Case) time.Duration {
	passes := (c.Warmup + c.Repeats) * 2
	return time.Duration(passes)*(time.Duration(c.BudgetS+c.WarmS)*time.Second+60*time.Second) + fixtureSwitchBound
}

func remeasureBound(c *Case) time.Duration {
	return time.Duration(remeasurePairs*2) * (time.Duration(c.BudgetS+c.WarmS)*time.Second + 60*time.Second)
}

// pairBound is one pair's upper bound: the clusters it stands up (r, tc, t,
// and the extra ones per conf group), the fixtures each of them builds, the
// canaries, the cases, and the judging.
func pairBound(canaries, cases []*Case, overlap bool) time.Duration {
	sides := 2
	if overlap {
		sides = 3
	}
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
	d := time.Duration(creates)*clusterCreateBound + time.Duration(fixtures)*fixtureBound + judgeBound
	for _, c := range canaries {
		d += caseBound(c)
	}
	for _, c := range cases {
		d += caseBound(c) * time.Duration(sides-1)
	}
	return d
}

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
