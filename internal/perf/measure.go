package perf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// pass is one run of the case's program on one side: what it reported, what
// the collect layer read around it, or why there is nothing.
type pass struct {
	Rep        int                 `json:"rep"`
	Side       string              `json:"side"`
	Phase      string              `json:"phase"`
	Ops        *float64            `json:"ops"`
	WarmOps    *float64            `json:"warm_ops"`
	ElapsedNs  *float64            `json:"elapsed_ns"`
	P50Ns      *float64            `json:"p50_ns"`
	P99Ns      *float64            `json:"p99_ns"`
	Value      *float64            `json:"value"` // the metric's value for this pass, in its unit
	Raw        map[string]float64  `json:"raw"`   // counter differences before dividing by ops
	PerOp      map[string]*float64 `json:"per_op"`
	Missing    map[string]string   `json:"missing"`
	NullReason string              `json:"null_reason,omitempty"`
	Notes      []string            `json:"notes,omitempty"`
	WarmS      int                 `json:"warm_included_s"`
	// Host is guard stage 3's reading across a measured pass (session only).
	Host *hostDelta `json:"host,omitempty"`
}

// caseResult is both sides' passes for one case. Reference is the role of
// the side that stood in for the reference (an overlap comparison runs the
// target against the "overlap" side); empty means "reference".
type caseResult struct {
	Case      *Case
	Passes    []*pass
	Reference string
}

func (cr *caseResult) refRole() string {
	if cr.Reference == "" {
		return "reference"
	}
	return cr.Reference
}

func (cr *caseResult) measured(side string) []*pass {
	var out []*pass
	for _, p := range cr.Passes {
		if p.Side == side && p.Phase == "measure" {
			out = append(out, p)
		}
	}
	return out
}

// runCase is the AB warm-up and ABBA measured passes (Design §5.5): every
// repetition runs both sides once, and the side that goes first alternates,
// so neither build is first more often than the other. No other case comes
// between the two builds' passes of this one.
func (r *Runner) runCase(ctx context.Context, c *Case, f *Fixture, t, ref *Side, idle ...*Side) *caseResult {
	cr := &caseResult{Case: c, Reference: ref.Role}
	for k := 1; k <= c.Warmup; k++ {
		cr.Passes = append(cr.Passes,
			r.passOn(ctx, t, append([]*Side{ref}, idle...), c, f, "warmup", k),
			r.passOn(ctx, ref, append([]*Side{t}, idle...), c, f, "warmup", k))
	}
	r.measurePasses(ctx, cr, c, f, t, ref, 1, r.repeats(c), idle...)
	return cr
}

// measurePasses runs repetitions from..to in the ABBA order: the pair of a
// repetition is the two passes next to each other in time. The session's
// re-measurement (FR-20.1) continues the sequence from repeats+1. idle is
// every further side to pause (the third cluster of an overlap).
func (r *Runner) measurePasses(ctx context.Context, cr *caseResult, c *Case, f *Fixture, t, ref *Side, from, to int, idle ...*Side) {
	for k := from; k <= to; k++ {
		order := [][2]*Side{{t, ref}, {ref, t}}
		if k%2 == 0 {
			order = [][2]*Side{{ref, t}, {t, ref}}
		}
		for _, o := range order {
			s, other := o[0], o[1]
			if ctx.Err() != nil {
				cr.Passes = append(cr.Passes, &pass{Rep: k, Side: s.Role, Phase: "measure", NullReason: "signal"})
				continue
			}
			cr.Passes = append(cr.Passes, r.passOn(ctx, s, append([]*Side{other}, idle...), c, f, "measure", k))
		}
	}
}

// passOn is a pass with every idle side paused for its duration (Design
// §5.1, decision §2 of 2026-10-07): the idle clusters' daemons, vacuum and
// flushes stay out of the measured window. A local run does not pause. The
// measured side is unpaused first, in case the last unpause failed; when
// that fails too the pass is null(pause) and the side is marked frozen, so
// the caller stops the pair instead of hanging on every exec (Design §11).
// A pause that fails is a note on the pass.
func (r *Runner) passOn(ctx context.Context, s *Side, idle []*Side, c *Case, f *Fixture, phase string, k int) *pass {
	if !r.PauseIdle {
		return r.pass(ctx, s, c, f, phase, k)
	}
	if err := r.unpauseSide(s); err != nil {
		time.Sleep(2 * time.Second)
		if err = r.unpauseSide(s); err != nil {
			s.frozen = err
			return (&pass{Rep: k, Side: s.Role, Phase: phase, WarmS: c.WarmS, Missing: map[string]string{}}).null("pause: " + err.Error())
		}
	}
	s.frozen = nil // it thawed; whatever failed before is over
	var notes []string
	for _, o := range idle {
		if o == nil || o == s {
			continue
		}
		if err := r.pauseSide(o); err != nil {
			notes = append(notes, o.Role+" not paused: "+err.Error())
		}
	}
	p := r.pass(ctx, s, c, f, phase, k)
	for _, o := range idle {
		if o == nil || o == s {
			continue
		}
		if err := r.unpauseSide(o); err != nil {
			time.Sleep(2 * time.Second)
			if err = r.unpauseSide(o); err != nil {
				o.frozen = err
				p.Notes = append(p.Notes, o.Role+" not unpaused: "+err.Error())
			}
		}
	}
	p.Notes = append(p.Notes, notes...)
	return p
}

// pass is Design §5.5's pass(): reset, the cold sequence when asked, a
// checkpoint pushed out of the window when asked, then the snapshots around
// the client. A step that fails before the client runs makes the pass null
// with that step's name; the collect layer failing does not (FR-16).
func (r *Runner) pass(ctx context.Context, s *Side, c *Case, f *Fixture, phase string, k int) *pass {
	p := &pass{Rep: k, Side: s.Role, Phase: phase, WarmS: c.WarmS, Missing: map[string]string{}}
	db := dbName(f)
	r.logf("%s %s rep %d: %s", c.ID, s.Role, k, phase)
	if err := r.reset(ctx, s, c, f); err != nil {
		return p.null("reset: " + err.Error())
	}
	if c.Cold {
		if err := r.coldStart(ctx, s, db); err != nil {
			return p.null(err.Error())
		}
	}
	if c.Background == "defer" {
		// csql's ;checkpoint session command: only a --sysadm csql may run
		// it, and only from its input, not -c (which is SQL). There is no
		// checkpoint utility in 11.x.
		if err := r.mustExec(ctx, s, "checkpoint", fmt.Sprintf("printf ';checkpoint\\n' | csql --sysadm -u dba %s > %s/log/checkpoint-%s.log 2>&1", db, workPerf, db), 5*time.Minute); err != nil {
			p.Notes = append(p.Notes, "checkpoint before the window failed: "+err.Error())
		}
	}
	out := r.passOut(s, c, phase, k)
	if err := os.MkdirAll(out.Host, 0o755); err != nil {
		return p.null("out dir: " + err.Error())
	}
	var l0pre *l0Snapshot
	var sdpre map[string]float64
	wantStatdump := hasStatdump(c)
	if phase == "measure" {
		// Vacuum left by the reset or the previous pass must not run inside
		// the window; wait for it, within a bound, and say when the bound hit.
		if note, err := r.waitVacuum(ctx, s, db, time.Minute); err != nil {
			p.Notes = append(p.Notes, "vacuum wait: "+err.Error())
		} else if note != "" {
			p.Notes = append(p.Notes, note)
		}
		if note, err := r.watcherEnsure(ctx, s, db); err != nil {
			p.Notes = append(p.Notes, "watcher: "+err.Error())
		} else if note != "" {
			p.Notes = append(p.Notes, note)
		}
		var err error
		if l0pre, err = r.l0Collect(ctx, s, db, out.InNode, out.Host, "l0-pre.json"); err != nil {
			p.Missing["l0"] = "pre: " + err.Error()
		}
		if wantStatdump {
			if sdpre, err = r.statdumpSnapshot(ctx, s, db, out.InNode, out.Host, "statdump-pre.txt"); err != nil {
				p.Missing["statdump"] = "pre: " + err.Error()
			}
		}
	}
	var hostPre hostSnapshot
	if phase == "measure" && r.Guard != nil {
		hostPre = r.Guard.snapshot()
	}
	rec, rc, reason, err := r.runClient(ctx, s, c, f, phase, k)
	if phase == "measure" && r.Guard != nil {
		p.Host = r.Guard.delta(hostPre, r.Guard.snapshot())
	}
	if err != nil {
		return p.null("client: " + err.Error())
	}
	if phase != "measure" {
		return p
	}
	var l0post *l0Snapshot
	var sdpost map[string]float64
	if wantStatdump {
		var e error
		if sdpost, e = r.statdumpSnapshot(ctx, s, db, out.InNode, out.Host, "statdump-post.txt"); e != nil {
			p.Missing["statdump"] = "post: " + e.Error()
		}
	}
	if l0post, err = r.l0Collect(ctx, s, db, out.InNode, out.Host, "l0-post.json"); err != nil {
		p.Missing["l0"] = "post: " + err.Error()
	}
	if reason != "" {
		p.NullReason = reason
		r.logf("%s %s rep %d: null (%s, rc %d)", c.ID, s.Role, k, reason, rc)
	}
	// Guard stage 3 (FR-6.2): a window something else ran in is not a
	// measurement, however the client fared.
	if p.Host != nil && p.NullReason == "" {
		if why := p.Host.contaminated(r.Guard.threshold); why != "" {
			p.NullReason = why
			r.logf("%s %s rep %d: null (%s)", c.ID, s.Role, k, why)
		}
	}
	if rec != nil {
		p.Ops, p.WarmOps, p.ElapsedNs, p.P50Ns, p.P99Ns = &rec.Ops, rec.WarmOps, &rec.ElapsedNs, rec.P50Ns, rec.P99Ns
		if p.NullReason == "" {
			v := metricValue(c, rec)
			if v == nil {
				p.NullReason = "the client did not report the metric " + c.Metric
			}
			p.Value = v
		}
	}
	// Counters: raw differences, then per op. Collected even when the pass
	// is null, with the ops the client reported, so a reader can see what
	// moved; per-op is null without ops.
	p.Raw = map[string]float64{}
	if _, said := p.Missing["l0"]; !said {
		d, miss := l0Delta(l0pre, l0post)
		for k, v := range d {
			p.Raw[k] = v
		}
		for k, v := range miss {
			p.Missing[k] = v
		}
	}
	if rec != nil && rec.Self != nil {
		d, miss := clientDelta(rec.Self)
		for k, v := range d {
			p.Raw[k] = v
		}
		for k, v := range miss {
			p.Missing[k] = v
		}
	} else {
		p.Missing["client"] = "no self-report from the client"
	}
	if wantStatdump {
		if _, said := p.Missing["statdump"]; !said {
			d, miss := statdumpDelta(sdpre, sdpost, c.Counters)
			for k, v := range d {
				p.Raw[k] = v
			}
			for k, v := range miss {
				p.Missing[k] = v
			}
		}
	}
	ops, warm := 0.0, 0.0
	if p.Ops != nil {
		ops = *p.Ops
	}
	// A case that warms has warm-up ops in the server-side window; one that
	// does not (warm_s = 0) has none, whatever the client reported.
	warmKnown := c.WarmS == 0
	if p.WarmOps != nil {
		warm, warmKnown = *p.WarmOps, true
	}
	var miss map[string]string
	p.PerOp, miss = perOp(p.Raw, ops, warm, warmKnown)
	for k, v := range miss {
		p.Missing[k] = v
	}
	return p
}

func (p *pass) null(reason string) *pass {
	p.NullReason = reason
	return p
}

// coldStart is FR-10's order: the server down, the disk synced, the page
// cache dropped, the server up -- so the restore's copy does not warm what
// the pass is meant to read cold. The drop needs root; when it is refused
// the pass is null(cache_drop) rather than a warm number in a cold column.
func (r *Runner) coldStart(ctx context.Context, s *Side, db string) error {
	if err := r.serverStop(ctx, s, db); err != nil {
		return err
	}
	if err := pageCacheDrop(r.SuiteDir); err != nil {
		_ = r.serverStart(ctx, s, db)
		return fmt.Errorf("cache_drop: %v", err)
	}
	if err := r.serverStart(ctx, s, db); err != nil {
		return err
	}
	return r.watcherStart(ctx, s, db)
}

// hasStatdump says whether any of the case's counters is a statdump name.
func hasStatdump(c *Case) bool {
	for _, n := range c.Counters {
		if statdumpNames[n] {
			return true
		}
	}
	return false
}

// metricValue is the pass's value in the metric's unit: i/s for ops_per_s,
// seconds for latency_s (the p50) and elapsed_s.
func metricValue(c *Case, rec *clientRecord) *float64 {
	var v float64
	switch c.Metric {
	case "ops_per_s":
		if rec.ElapsedNs <= 0 {
			return nil
		}
		v = rec.Ops / (rec.ElapsedNs / 1e9)
	case "latency_s":
		if rec.P50Ns == nil {
			return nil
		}
		v = *rec.P50Ns / 1e9
	case "elapsed_s":
		v = rec.ElapsedNs / 1e9
	default:
		return nil
	}
	return &v
}

// keepPassFiles copies a pass's files from the cluster's /work to the results
// directory; /work goes with the cluster when it is destroyed (Design §5.3).
func (r *Runner) keepPassFiles(s *Side, c *Case, phase string, k int, pairDir string) {
	src := r.passOut(s, c, phase, k).Host
	dst := filepath.Join(pairDir, c.ID, s.Role, filepath.Base(src))
	if err := copyTree(src, dst); err != nil {
		r.logf("keeping %s: %v", src, err)
	}
}
