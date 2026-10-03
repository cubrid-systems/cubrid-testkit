package perf

import (
	"strings"
	"testing"
	"time"
)

func f(v float64) *float64 { return &v }

func measuredPass(side string, k int, value float64, perOp map[string]float64) *pass {
	p := &pass{Rep: k, Side: side, Phase: "measure", Value: f(value), Ops: f(1000), PerOp: map[string]*float64{}}
	for name, v := range perOp {
		p.PerOp[name] = f(v)
	}
	return p
}

func nullPass(side string, k int) *pass {
	return &pass{Rep: k, Side: side, Phase: "measure", NullReason: "budget"}
}

// FR-19: the ratio is turned so that above 1 is the target being slower, for
// every metric; FR-20: the flag follows the tolerance.
func TestJudgeTurnsTheRatioTheRightWay(t *testing.T) {
	cases := []struct {
		metric      string
		target, ref float64
		wantRatio   float64
		wantFlag    string
	}{
		{"latency_s", 0.0022, 0.0020, 1.1, FlagRegression},
		{"elapsed_s", 9.0, 10.0, 0.9, FlagImprovement},
		{"ops_per_s", 9000, 10000, 10000.0 / 9000.0, FlagRegression},
		{"ops_per_s", 10200, 10000, 10000.0 / 10200.0, FlagNone},
	}
	for _, c := range cases {
		cs := &Case{ID: "x.y", Metric: c.metric, Tolerance: 0.05, Repeats: 3}
		cr := &caseResult{Case: cs}
		for k := 1; k <= 3; k++ {
			cr.Passes = append(cr.Passes, measuredPass("target", k, c.target, nil), measuredPass("reference", k, c.ref, nil))
		}
		v := judge(cs, cr, 3)
		if v.Status != StatusOK || v.Ratio == nil || abs(*v.Ratio-c.wantRatio) > 1e-9 || v.Flag != c.wantFlag {
			t.Errorf("%s %v/%v: status=%s ratio=%v flag=%s, want ratio %v flag %s", c.metric, c.target, c.ref, v.Status, v.Ratio, v.Flag, c.wantRatio, c.wantFlag)
		}
	}
}

// FR-21: inside the tolerance, a deterministic counter that moved by more
// than 1% per op is a workload change; a non-deterministic one is not.
func TestJudgeFlagsAWorkloadChangeFromTheCounters(t *testing.T) {
	cs := &Case{ID: "x.y", Metric: "latency_s", Tolerance: 0.05, Repeats: 3, Counters: []string{"Num_file_iosynches", "server.cpu_user"}}
	cr := &caseResult{Case: cs}
	for k := 1; k <= 3; k++ {
		cr.Passes = append(cr.Passes,
			measuredPass("target", k, 0.0020, map[string]float64{"Num_file_iosynches": 1.03, "server.cpu_user": 2.0}),
			measuredPass("reference", k, 0.0020, map[string]float64{"Num_file_iosynches": 1.00, "server.cpu_user": 1.0}))
	}
	v := judge(cs, cr, 3)
	if v.Flag != FlagWorkloadChange || v.Why != "Num_file_iosynches" {
		t.Errorf("flag=%s why=%s", v.Flag, v.Why)
	}
	// cpu_user doubling alone is diagnostic, not a flag.
	cs.Counters = []string{"server.cpu_user"}
	if v := judge(cs, cr, 3); v.Flag != FlagNone {
		t.Errorf("a non-deterministic counter flagged: %s", v.Flag)
	}
}

// FR-11: more than half of one side's passes null makes the case null; the
// mean ignores the nulls there are.
func TestJudgeGoesNullWhenHalfThePassesDid(t *testing.T) {
	cs := &Case{ID: "x.y", Metric: "elapsed_s", Tolerance: 0.05, Repeats: 5}
	cr := &caseResult{Case: cs}
	for k := 1; k <= 5; k++ {
		cr.Passes = append(cr.Passes, measuredPass("reference", k, 10, nil))
		if k <= 2 {
			cr.Passes = append(cr.Passes, nullPass("target", k))
		} else {
			cr.Passes = append(cr.Passes, measuredPass("target", k, 10.5, nil))
		}
	}
	v := judge(cs, cr, 5)
	if v.Status != StatusOK || v.TargetMean == nil || *v.TargetMean != 10.5 {
		t.Errorf("two nulls of five: status=%s mean=%v", v.Status, v.TargetMean)
	}
	cr.Passes = append(cr.Passes[:0:0], cr.Passes...)
	cr.Passes[4] = nullPass("target", 3) // a third null
	if v := judge(cs, cr, 5); v.Status != StatusNull || v.Ratio != nil {
		t.Errorf("three nulls of five: status=%s ratio=%v", v.Status, v.Ratio)
	}
}

func TestClientRecordIsTheLastJSONLine(t *testing.T) {
	out := "warming 10 s\nconnected\n{\"ops\": 20000, \"elapsed_ns\": 41873002113, \"p50_ns\": 1980112, \"p99_ns\": 4911800, \"errors\": 0, \"self\": {\"utime_ms\": 3120, \"syscr\": 120412, \"syscw\": 120380, \"net_rx_packets\": 60120, \"net_tx_packets\": 60118}}\n\n"
	rec, err := parseClientRecord([]byte(out))
	if err != nil || rec.Ops != 20000 || rec.P50Ns == nil || *rec.P50Ns != 1980112 || *rec.Self["syscw"] != 120380 {
		t.Fatalf("rec=%+v err=%v", rec, err)
	}
	d, missing := clientDelta(rec.Self)
	if d["client.rw_syscalls"] != 240792 || d["net_packets"] != 120238 || d["client.cpu_user"] != 3120 {
		t.Errorf("client delta = %v", d)
	}
	// What the client did not report is missing by name, not zero.
	if _, ok := d["client.cpu_sys"]; ok || !strings.Contains(missing["client.cpu_sys"], "no stime_ms") {
		t.Errorf("an unreported field: delta=%v missing=%v", d["client.cpu_sys"], missing)
	}
	// A null from a utility wrapper is the same.
	rec2, _ := parseClientRecord([]byte(`{"ops":1,"elapsed_ns":5,"errors":0,"self":{"utime_ms":10,"stime_ms":2,"ctxsw_vol":null}}`))
	if _, m := clientDelta(rec2.Self); !strings.Contains(m["client.ctxsw_vol"], "no ctxsw_vol") {
		t.Errorf("null: %v", m)
	}
	c := &Case{Metric: "latency_s"}
	if v := metricValue(c, rec); v == nil || abs(*v-0.001980112) > 1e-12 {
		t.Errorf("latency value = %v", v)
	}
	c.Metric = "ops_per_s"
	if v := metricValue(c, rec); v == nil || abs(*v-20000/41.873002113) > 1e-6 {
		t.Errorf("ops/s value = %v", v)
	}
	if _, err := parseClientRecord([]byte("something went wrong\n")); err == nil {
		t.Error("a last line that is not JSON was accepted")
	}
}

func TestL0DeltaSumsPidsPresentInBothAndNamesChurn(t *testing.T) {
	pre, _ := parseL0([]byte(`{"ts_ns":1,"roles":{"server":{"10":{"utime_ms":100,"stime_ms":50,"syscr":1000,"syscw":500,"minflt":10,"majflt":0,"ctxsw_vol":40,"ctxsw_invol":2,"rss_peak_kb":1000,"read_bytes":0,"write_bytes":4096,"runq_wait_ms":1}},
	"cas":{"20":{"utime_ms":10,"syscr":100,"syscw":100},"21":{"utime_ms":10,"syscr":100,"syscw":100}}},
	"disk":{"dev":"nvme0n1","via":"dm-0","reads":100,"writes":200,"flushes":50,"busy_ms":1000},"errors":[]}`))
	post, _ := parseL0([]byte(`{"ts_ns":2,"roles":{"server":{"10":{"utime_ms":400,"stime_ms":150,"syscr":3000,"syscw":1500,"minflt":30,"majflt":1,"ctxsw_vol":90,"ctxsw_invol":5,"rss_peak_kb":1200,"read_bytes":0,"write_bytes":8192,"runq_wait_ms":3}},
	"cas":{"20":{"utime_ms":30,"syscr":300,"syscw":300},"22":{"utime_ms":5,"syscr":50,"syscw":50}}},
	"disk":{"dev":"nvme0n1","via":"dm-0","reads":130,"writes":260,"flushes":70,"busy_ms":1400},"errors":[]}`))
	d, missing := l0Delta(pre, post)
	if d["server.cpu_user"] != 300 || d["server.rw_syscalls"] != 3000 || d["server.page_faults"] != 21 || d["server.rss_peak"] != 1200*1024 || d["server.runq_wait"] != 2 {
		t.Errorf("server delta = %v", d)
	}
	if d["dev_reads"] != 30 || d["dev_flushes"] != 20 || d["dev_busy"] != 400 {
		t.Errorf("disk delta = %v", d)
	}
	// CAS 21 went and 22 came: the surviving pid is summed and the churn is said.
	if d["cas.rw_syscalls"] != 400 || !strings.Contains(missing["cas"], "cas pid churn") {
		t.Errorf("cas delta=%v missing=%v", d["cas.rw_syscalls"], missing)
	}
	// Server-side counters are divided by measured + warm-up ops; a client
	// counter by measured ops alone.
	d["client.cpu_user"] = 500
	per, miss := perOp(d, 1000, 500, true)
	if per["server.cpu_user"] == nil || *per["server.cpu_user"] != 0.2 || per["client.cpu_user"] == nil || *per["client.cpu_user"] != 0.5 || len(miss) != 0 {
		t.Errorf("per op = %v %v missing=%v", per["server.cpu_user"], per["client.cpu_user"], miss)
	}
	if per, _ := perOp(d, 0, 0, true); per["server.cpu_user"] != nil {
		t.Error("per op with zero ops must be null")
	}
	// A warming case whose client did not say how much it warmed: the
	// server-side values are missing, the client's are not.
	per, miss = perOp(d, 1000, 0, false)
	if _, ok := per["server.cpu_user"]; ok || !strings.Contains(miss["server.cpu_user"], "warm_ops") || per["client.cpu_user"] == nil {
		t.Errorf("unknown warm-up: per=%v missing=%v", per, miss)
	}
}

func TestStatdumpLinesAreNameEqualsValue(t *testing.T) {
	text := `
 *** SERVER EXECUTION STATISTICS ***
Num_file_creates              =          0
Num_file_iosynches            =      20004
Num_log_page_iowrites         =      20019
Data_page_buffer_hit_ratio    =      99.80

 *** OTHER STATISTICS ***
Num_data_page_fix_ext:
  PAGE_FTAB,PAGE_... = 3
`
	got, err := parseStatdump(strings.NewReader(text))
	if err != nil || got["Num_file_iosynches"] != 20004 || got["Data_page_buffer_hit_ratio"] != 99.8 || len(got) != 4 {
		t.Fatalf("got=%v err=%v", got, err)
	}
	pre := map[string]float64{"Num_file_iosynches": 20004, "Num_log_page_iowrites": 20019}
	post := map[string]float64{"Num_file_iosynches": 40010, "Num_log_page_iowrites": 40040}
	d, missing := statdumpDelta(pre, post, []string{"Num_file_iosynches", "Num_log_page_iowrites", "Num_tran_commits", "dev_flushes"})
	if d["Num_file_iosynches"] != 20006 || d["Num_log_page_iowrites"] != 20021 || len(d) != 2 {
		t.Errorf("delta = %v", d)
	}
	if _, said := missing["Num_tran_commits"]; !said {
		t.Errorf("a statdump name absent from the output must be missing: %v", missing)
	}
	if _, said := missing["dev_flushes"]; said {
		t.Error("a non-statdump counter is not statdump's to miss")
	}
}

func TestClusterNamesFollowCSBsRule(t *testing.T) {
	day := time.Date(2026, 10, 4, 2, 0, 0, 0, time.Local)
	n := ClusterName("perf-run-20261004-020000", day, "t")
	if !strings.HasPrefix(n, "pf-20261004-") || !strings.HasSuffix(n, "-t") || len(n) != len("pf-20261004-xxxx-t") {
		t.Errorf("name = %q", n)
	}
	if ClusterName("a", day, "t") == ClusterName("b", day, "t") {
		t.Error("two sessions on one day collide")
	}
	if u := confUnion([]*Case{{Conf: map[string]string{"log_buffer_size": "16M", "data_buffer_size": "4G"}}}); strings.Join(u, " ") != "data_buffer_size=4G log_buffer_size=16M" {
		t.Errorf("conf union = %v", u)
	}
	// The broker is pinned to the largest client count that goes through it.
	if n := brokerCAS([]*Case{{Driver: "jdbc", Clients: 1}, {Driver: "jdbc", Clients: 16}, {Driver: "utility"}}); n != 16 {
		t.Errorf("brokerCAS = %d", n)
	}
	if n := brokerCAS([]*Case{{Driver: "utility"}}); n != 1 {
		t.Errorf("brokerCAS with no jdbc case = %d", n)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
