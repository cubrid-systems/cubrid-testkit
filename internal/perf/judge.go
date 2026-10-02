package perf

import (
	"math"
	"sort"
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
	Ratio         *float64
	Flag          string
	Status        string
	// Counters is the per-op value of each requested counter on each side,
	// averaged over the measured passes that had it.
	Counters map[string]counterPair
	// Why names the counter that set workload_change, when one did.
	Why string
}

type counterPair struct {
	Target    *float64 `json:"target"`
	Reference *float64 `json:"reference"`
	Unit      string   `json:"unit"`
}

// judge is Design §5.7: the mean of each side's non-null measured passes;
// the ratio turned so that above 1 is the target being slower (FR-19); the
// flag from the tolerance (FR-20) or, inside it, from a deterministic
// counter that moved (FR-21); null when either side lost more than half of
// its passes (FR-11).
func judge(c *Case, cr *caseResult, repeats int) Verdict {
	v := Verdict{Flag: FlagNone, Status: StatusOK, Counters: map[string]counterPair{}}
	t, ref := cr.measured("target"), cr.measured("reference")
	v.TargetMean = meanValue(t)
	v.ReferenceMean = meanValue(ref)
	if nulls(t) > repeats/2 || nulls(ref) > repeats/2 || v.TargetMean == nil || v.ReferenceMean == nil {
		v.Status = StatusNull
	}
	for _, name := range c.Counters {
		v.Counters[name] = counterPair{Target: meanPerOp(t, name), Reference: meanPerOp(ref, name), Unit: "per_op"}
	}
	if v.Status != StatusOK {
		return v
	}
	var ratio float64
	switch c.Metric {
	case "ops_per_s":
		ratio = *v.ReferenceMean / *v.TargetMean
	default:
		ratio = *v.TargetMean / *v.ReferenceMean
	}
	if math.IsNaN(ratio) || math.IsInf(ratio, 0) {
		v.Status = StatusNull
		return v
	}
	v.Ratio = &ratio
	switch {
	case ratio > 1+c.Tolerance:
		v.Flag = FlagRegression
	case ratio < 1-c.Tolerance:
		v.Flag = FlagImprovement
	default:
		names := make([]string, 0, len(v.Counters))
		for n := range v.Counters {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
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

// deterministic says whether a counter is one FR-21 reads: the engine's own
// statistics and the counts of what left the machine (flushes, packets).
// Syscall counts are not among them: a JVM's socket I/O is recv/send, which
// the kernel's read/write accounting does not see, so the client's count is
// a few dozen whatever the work was.
func deterministic(name string) bool {
	return statdumpNames[name] || name == "dev_flushes" || name == "net_packets"
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
