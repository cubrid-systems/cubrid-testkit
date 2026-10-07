package perf

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The files a run leaves (Spec §7.6): regression-case.json is what cbingest
// reads, cases.csv is for a person, counters.json is the per-case evidence,
// session.json says what the run was.

// Sidecar is regression-case.json.
type Sidecar struct {
	Schema       string      `json:"schema"`
	Session      string      `json:"session"`
	Pair         string      `json:"pair"`
	Target       BuildRef    `json:"target"`
	Reference    BuildRef    `json:"reference"`
	Machine      string      `json:"machine"`
	CPUSet       CPUSetPair  `json:"cpuset"`
	Pinning      string      `json:"pinning"`
	Interleave   string      `json:"interleave"`
	ClientImage  ClientImage `json:"client_image"`
	SessionValid bool        `json:"session_valid"`
	// CanaryOf names the pair a canary sidecar (pair "canary") belongs to.
	CanaryOf string      `json:"canary_of,omitempty"`
	Cases    []CaseEntry `json:"cases"`
}

// ClientImage is the client node's image, by name and by the id the runtime
// gave it: the JVM and the driver are half of what a jdbc case measures, and
// a rebuilt image is a change to record (the driver itself is the engine's,
// per build, by decision of 2026-10-07).
type ClientImage struct {
	Name   string `json:"name"`
	Digest string `json:"digest,omitempty"`
}

type BuildRef struct {
	Ref         string      `json:"ref,omitempty"` // what perf.conf or the registration called it (session)
	Build       string      `json:"build"`
	Commit      string      `json:"commit"`
	Fingerprint Fingerprint `json:"fingerprint"`
}

type CPUSetPair struct {
	Server string `json:"server"`
	Client string `json:"client"`
}

type SideValues struct {
	Values []*float64 `json:"values"`
	Unit   string     `json:"unit"`
	Ops    *float64   `json:"ops"`
}

type CaseEntry struct {
	ID           string                 `json:"id"`
	Version      int                    `json:"version"`
	Metric       string                 `json:"metric"`
	Op           string                 `json:"op"`
	Target       SideValues             `json:"target"`
	Reference    SideValues             `json:"reference"`
	Ratio        *float64               `json:"ratio"`          // median of the paired log-ratios (FR-19)
	RatioOfMeans *float64               `json:"ratio_of_means"` // for the record
	Pairs        []float64              `json:"pairs"`
	Confirmed    bool                   `json:"confirmed"`
	Tolerance    float64                `json:"tolerance"`
	Flag         string                 `json:"flag"`
	Counters     map[string]counterPair `json:"counters"`
	Status       string                 `json:"status"`
	Reason       string                 `json:"reason,omitempty"`
}

// metricUnit is conbench's unit for the metric (Spec §7.6.2).
func metricUnit(metric string) string {
	if metric == "ops_per_s" {
		return "i/s"
	}
	return "s"
}

func caseEntry(c *Case, cr *caseResult, v Verdict) CaseEntry {
	e := CaseEntry{
		ID: c.ID, Version: c.Version, Metric: c.Metric, Op: c.Op,
		Target: sideValues(c, cr.measured("target")), Reference: sideValues(c, cr.measured("reference")),
		Ratio: v.Ratio, RatioOfMeans: v.RatioOfMeans, Pairs: v.Pairs, Confirmed: v.Confirmed,
		Tolerance: c.Tolerance, Flag: v.Flag, Counters: v.Counters, Status: v.Status,
	}
	if e.Pairs == nil {
		e.Pairs = []float64{}
	}
	switch {
	case v.Status == StatusNull:
		e.Reason = firstNullReason(cr)
	case v.Flag == FlagWorkloadChange:
		e.Reason = "counter " + v.Why + " moved beyond 1%"
	case v.Why != "":
		e.Reason = v.Why
	}
	return e
}

func sideValues(c *Case, passes []*pass) SideValues {
	sv := SideValues{Unit: metricUnit(c.Metric), Values: []*float64{}}
	for _, p := range passes {
		if p.NullReason != "" {
			sv.Values = append(sv.Values, nil)
			continue
		}
		sv.Values = append(sv.Values, p.Value)
		if p.Ops != nil && sv.Ops == nil {
			sv.Ops = p.Ops
		}
	}
	return sv
}

func firstNullReason(cr *caseResult) string {
	for _, p := range cr.Passes {
		if p.Phase == "measure" && p.NullReason != "" {
			return p.Side + " rep " + strconv.Itoa(p.Rep) + ": " + p.NullReason
		}
	}
	return "too few measured passes"
}

// counters.json: every pass with its raw and per-op values, and what was
// missing and why (Design §4.2).
type countersFile struct {
	Case    string                 `json:"case"`
	Version int                    `json:"version"`
	Roles   []string               `json:"roles"`
	Passes  []*pass                `json:"passes"`
	PerOp   map[string]counterPair `json:"per_op"`
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// csvHeader is FR-18's fixed column order; a new column goes at the end.
var csvHeader = []string{"session", "pair", "case", "version", "metric", "target_mean", "reference_mean", "ratio", "tolerance", "flag", "status",
	"ratio_of_means", "pairs", "confirmed"}

func writeCasesCSV(path, session, pair string, entries []CaseEntry, verdicts map[string]Verdict) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if err := w.Write(csvHeader); err != nil {
		return err
	}
	for _, e := range entries {
		v := verdicts[e.ID]
		row := []string{session, pair, e.ID, strconv.Itoa(e.Version), e.Metric,
			fmtPtr(v.TargetMean), fmtPtr(v.ReferenceMean), fmtPtr(e.Ratio),
			strconv.FormatFloat(e.Tolerance, 'g', -1, 64), e.Flag, e.Status,
			fmtPtr(e.RatioOfMeans), fmtFloats(e.Pairs), strconv.FormatBool(e.Confirmed)}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func fmtFloats(xs []float64) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = strconv.FormatFloat(x, 'g', 6, 64)
	}
	return strings.Join(parts, " ")
}

func fmtPtr(p *float64) string {
	if p == nil {
		return ""
	}
	return strconv.FormatFloat(*p, 'g', 6, 64)
}

// RunSession is session.json for a local run: enough for a reader to know
// what the files beside it are about.
type RunSession struct {
	Schema      string      `json:"schema"`
	ID          string      `json:"id"`
	Kind        string      `json:"kind"`
	Started     time.Time   `json:"started"`
	Ended       time.Time   `json:"ended"`
	State       string      `json:"state"`
	Host        string      `json:"host"`
	Interleave  string      `json:"interleave"`
	CPUSet      CPUSetPair  `json:"cpuset"`
	Pinning     string      `json:"pinning"`
	ClientImage ClientImage `json:"client_image"`
	Pair        string      `json:"pair"`
	Target      BuildRef    `json:"target"`
	Reference   BuildRef    `json:"reference"`
	Cases       []string    `json:"cases"`
	Clusters    []string    `json:"clusters"`
	ExitReason  string      `json:"exit_reason"`
	Notes       []string    `json:"notes,omitempty"`
}

// lastLine is the one line a reader of standard output gets (FR-28): the
// ratio, the means, and every measured value of each side in order, null
// where a pass was.
func lastLine(c *Case, v Verdict, e CaseEntry) string {
	ratio := "null"
	if v.Ratio != nil {
		ratio = strconv.FormatFloat(*v.Ratio, 'f', 4, 64)
	}
	vals := func(sv SideValues) string {
		parts := make([]string, len(sv.Values))
		for i, p := range sv.Values {
			if p == nil {
				parts[i] = "null"
			} else {
				parts[i] = strconv.FormatFloat(*p, 'g', 6, 64)
			}
		}
		return "[" + strings.Join(parts, ",") + "]"
	}
	return fmt.Sprintf("%s ratio=%s pairs=[%s] confirmed=%t target_mean=%s reference_mean=%s unit=%s flag=%s status=%s target=%s reference=%s",
		c.ID, ratio, strings.Join(strings.Fields(fmtFloats(e.Pairs)), ","), e.Confirmed, fmtPtr(v.TargetMean), fmtPtr(v.ReferenceMean),
		metricUnit(c.Metric), v.Flag, v.Status, vals(e.Target), vals(e.Reference))
}
