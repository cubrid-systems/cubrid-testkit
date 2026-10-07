package perf

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Flags and statuses (Spec §7.6.2).
const (
	FlagNone           = "none"
	FlagRegression     = "regression"
	FlagImprovement    = "improvement"
	FlagWorkloadChange = "workload_change"

	StatusOK      = "ok"
	StatusNull    = "null"
	StatusSkipped = "skipped"
)

// workloadTolerance is FR-21's band for the deterministic counters: a per-op
// count that moved by more than this while the time did not is a workload
// change, and an early warning.
const workloadTolerance = 0.01

// Verdict is what judge says about one case.
type Verdict struct {
	TargetMean    *float64
	ReferenceMean *float64
	// Ratio is the paired estimate: for every repetition k that both sides
	// measured, the ratio of that pair turned so that above 1 is the target
	// being slower; the median of their logarithms, exponentiated. The pair
	// is what ABBA produces -- two passes close in time -- and the median
	// ignores the one pass a checkpoint or a neighbour landed on.
	Ratio *float64
	// RatioOfMeans is FR-19's old statistic, kept for the record.
	RatioOfMeans *float64
	// Pairs are the per-repetition ratios behind Ratio, in order.
	Pairs []float64
	// Confirmed says whether the flag is backed by every pair: with at least
	// three pairs, all on the same side of 1 as the median. An estimate
	// outside the tolerance that the pairs do not agree on is not a flag.
	Confirmed bool
	Flag      string
	Status    string
	// Counters is the per-op value of each requested counter on each side,
	// averaged over the measured passes that had it.
	Counters map[string]counterPair
	// Why names the counter that set workload_change, or says why a ratio
	// outside the tolerance was not flagged.
	Why string
}

type counterPair struct {
	Target    *float64 `json:"target"`
	Reference *float64 `json:"reference"`
	Unit      string   `json:"unit"`
}

// minPairs is how many agreeing pairs a flag needs.
const minPairs = 3

// judge is Design §5.7 as revised on 2026-10-07: the ratio is the median of
// the paired log-ratios (FR-19), the flag needs the estimate outside the
// tolerance and the pairs in agreement (FR-20), a count-valued counter the
// case lists for judging that moved by more than 1 % per op is a workload
// change (FR-21), and the case is null when either side lost more than half
// of its passes (FR-11).
func judge(c *Case, cr *caseResult, repeats int) Verdict {
	v := Verdict{Flag: FlagNone, Status: StatusOK, Counters: map[string]counterPair{}}
	t, ref := cr.measured("target"), cr.measured("reference")
	v.TargetMean = meanValue(t)
	v.ReferenceMean = meanValue(ref)
	for _, name := range c.Counters {
		v.Counters[name] = counterPair{Target: meanPerOp(t, name), Reference: meanPerOp(ref, name), Unit: "per_op"}
	}
	v.Pairs = pairRatios(c, t, ref)
	if nulls(t) > repeats/2 || nulls(ref) > repeats/2 || len(v.Pairs) == 0 {
		v.Status = StatusNull
		return v
	}
	if v.TargetMean != nil && v.ReferenceMean != nil {
		rm := orient(c, *v.TargetMean, *v.ReferenceMean)
		if !math.IsNaN(rm) && !math.IsInf(rm, 0) {
			v.RatioOfMeans = &rm
		}
	}
	logs := make([]float64, len(v.Pairs))
	for i, r := range v.Pairs {
		logs[i] = math.Log(r)
	}
	ratio := math.Exp(median(logs))
	if math.IsNaN(ratio) || math.IsInf(ratio, 0) {
		v.Status = StatusNull
		return v
	}
	v.Ratio = &ratio
	above, below := 0, 0
	for _, r := range v.Pairs {
		if r > 1 {
			above++
		} else if r < 1 {
			below++
		}
	}
	switch {
	case ratio > 1+c.Tolerance:
		if v.Confirmed = len(v.Pairs) >= minPairs && above == len(v.Pairs); v.Confirmed {
			v.Flag = FlagRegression
		} else {
			v.Why = fmt.Sprintf("ratio %.4f is outside the tolerance but only %d of %d pairs agree (%d needed, all on one side)", ratio, above, len(v.Pairs), minPairs)
		}
	case ratio < 1-c.Tolerance:
		if v.Confirmed = len(v.Pairs) >= minPairs && below == len(v.Pairs); v.Confirmed {
			v.Flag = FlagImprovement
		} else {
			v.Why = fmt.Sprintf("ratio %.4f is outside the tolerance but only %d of %d pairs agree (%d needed, all on one side)", ratio, below, len(v.Pairs), minPairs)
		}
	default:
		for _, n := range c.JudgeCounters {
			if !deterministic(n) {
				continue
			}
			cp := v.Counters[n]
			if cp.Target == nil || cp.Reference == nil {
				continue
			}
			moved := false
			switch {
			case *cp.Reference == 0 && *cp.Target == 0:
			case *cp.Reference == 0 || *cp.Target == 0:
				// From nothing to something, or back: a change whatever
				// the ratio would have been.
				moved = true
			default:
				moved = math.Abs(*cp.Target / *cp.Reference - 1) > workloadTolerance
			}
			if moved {
				v.Flag = FlagWorkloadChange
				v.Why = n
				break
			}
		}
	}
	return v
}

// orient turns a target/reference comparison so that above 1 is the target
// being slower, whatever the metric counts (FR-19).
func orient(c *Case, target, reference float64) float64 {
	if c.Metric == "ops_per_s" {
		return reference / target
	}
	return target / reference
}

// pairRatios is the ratio of each repetition that both sides measured.
func pairRatios(c *Case, t, ref []*pass) []float64 {
	byRep := func(ps []*pass) map[int]float64 {
		out := map[int]float64{}
		for _, p := range ps {
			if p.NullReason == "" && p.Value != nil {
				out[p.Rep] = *p.Value
			}
		}
		return out
	}
	tv, rv := byRep(t), byRep(ref)
	reps := make([]int, 0, len(tv))
	for k := range tv {
		if _, ok := rv[k]; ok {
			reps = append(reps, k)
		}
	}
	sort.Ints(reps)
	out := make([]float64, 0, len(reps))
	for _, k := range reps {
		r := orient(c, tv[k], rv[k])
		if r > 0 && !math.IsInf(r, 0) && !math.IsNaN(r) {
			out = append(out, r)
		}
	}
	return out
}

func median(xs []float64) float64 {
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	n := len(s)
	if n == 0 {
		return math.NaN()
	}
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// deterministic says whether a counter is one FR-21 reads: the engine's own
// count-valued statistics and the counts of what left the machine (flushes,
// packets). Not among them: times and ratios, whose difference is not a
// count of work; the page-buffer gauges, whose before/after difference means
// nothing; and syscall counts -- a JVM's socket I/O is recv/send, which the
// kernel's read/write accounting does not see.
func deterministic(name string) bool {
	if name == "dev_flushes" || name == "net_packets" {
		return true
	}
	if !statdumpNames[name] || statdumpGauges[name] {
		return false
	}
	for _, s := range []string{"_usec", "_ratio", "_time"} {
		if strings.HasSuffix(name, s) {
			return false
		}
	}
	return !strings.HasPrefix(name, "Time_")
}

// statdumpGauges are the statistics that describe a state, not work done.
var statdumpGauges = map[string]bool{
	"Num_data_page_fixed": true, "Num_data_page_dirty": true,
	"Num_data_page_lru1": true, "Num_data_page_lru2": true, "Num_data_page_lru3": true,
	"Num_data_page_victim_candidate": true, "Num_prior_lsa_list_size": true,
	"Num_data_page_avoid_dealloc": true, "Num_data_page_avoid_victim": true,
}

func nulls(passes []*pass) int {
	n := 0
	for _, p := range passes {
		if p.NullReason != "" || p.Value == nil {
			n++
		}
	}
	return n
}

func meanValue(passes []*pass) *float64 {
	sum, n := 0.0, 0
	for _, p := range passes {
		if p.NullReason != "" || p.Value == nil {
			continue
		}
		sum += *p.Value
		n++
	}
	if n == 0 {
		return nil
	}
	m := sum / float64(n)
	return &m
}

func meanPerOp(passes []*pass, name string) *float64 {
	sum, n := 0.0, 0
	for _, p := range passes {
		if p.NullReason != "" {
			continue
		}
		v, ok := p.PerOp[name]
		if !ok || v == nil {
			continue
		}
		sum += *v
		n++
	}
	if n == 0 {
		return nil
	}
	m := sum / float64(n)
	return &m
}
