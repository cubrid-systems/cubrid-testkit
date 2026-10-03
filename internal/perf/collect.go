package perf

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The collect layer: what is read around a measured pass and how it becomes
// a per-op value. The names are the closed list (manifest.go, KnownCounter):
// <role>.<l0 metric> from /proc inside the database node, dev_* from the
// physical disk's /proc/diskstats row, net_* and the client's own L0 from
// the client's self-report, and the statdump statistics by the engine's name.

// l0Snapshot is collect_l0.sh's JSON: per role, per pid, the raw counters.
type l0Snapshot struct {
	TsNs   int64                        `json:"ts_ns"`
	Roles  map[string]map[string]l0Proc `json:"roles"`
	Disk   *l0Disk                      `json:"disk"`
	Errors []string                     `json:"errors"`
	raw    map[string]json.RawMessage   // kept for the record
}

// l0Proc is one process's counters; a field the script could not read is
// null, and a null on either side of a difference makes that counter
// missing rather than zero (FR-16).
type l0Proc map[string]*float64

type l0Disk struct {
	Dev     string   `json:"dev"`
	Via     *string  `json:"via"`
	Reads   *float64 `json:"reads"`
	Writes  *float64 `json:"writes"`
	Flushes *float64 `json:"flushes"`
	BusyMs  *float64 `json:"busy_ms"`
}

// l0Fields is what collect_l0.sh reports per process.
var l0Fields = []string{"utime_ms", "stime_ms", "minflt", "majflt", "ctxsw_vol", "ctxsw_invol", "rss_peak_kb",
	"syscr", "syscw", "read_bytes", "write_bytes", "runq_wait_ms"}

// l0Collect runs the snapshot script in the database node and reads the
// file back from the host side of /work. The database's name goes with it
// so the server role is this database's server and not every cub_server
// on the node (a session has one per fixture).
func (r *Runner) l0Collect(ctx context.Context, s *Side, db, outDirInNode, hostDir, name string) (*l0Snapshot, error) {
	path := outDirInNode + "/" + name
	res, err := r.exec(ctx, s.N1, fmt.Sprintf("PERF_DB=%s bash %s/scripts/collect_l0.sh snapshot %s", db, workPerf, path), time.Minute)
	if err != nil {
		return nil, err
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("collect_l0.sh: exit %d: %s", res.ExitCode, tail(res.Stderr, 300))
	}
	b, err := os.ReadFile(filepath.Join(hostDir, name))
	if err != nil {
		return nil, err
	}
	return parseL0(b)
}

func parseL0(b []byte) (*l0Snapshot, error) {
	var snap l0Snapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return nil, fmt.Errorf("l0 snapshot: %w", err)
	}
	return &snap, nil
}

// l0Delta is the difference of two snapshots per counter name, summed over
// the pids a role had in both, with the reasons anything is missing.
// Values are raw differences; dividing by ops is the caller's.
func l0Delta(pre, post *l0Snapshot) (delta map[string]float64, missing map[string]string) {
	delta, missing = map[string]float64{}, map[string]string{}
	if pre == nil || post == nil {
		missing["l0"] = "a snapshot is missing"
		return
	}
	for role := range post.Roles {
		before, after := pre.Roles[role], post.Roles[role]
		if len(before) != len(after) {
			missing[role] = fmt.Sprintf("%s pid churn (%d -> %d)", role, len(before), len(after))
		}
		n := 0
		sums := map[string]float64{}
		unread := map[string]string{}
		for pid, a := range after {
			b, ok := before[pid]
			if !ok {
				missing[role] = fmt.Sprintf("%s pid churn (new pid %s)", role, pid)
				continue
			}
			n++
			for _, f := range l0Fields {
				av, bv := a[f], b[f]
				if av == nil || bv == nil {
					unread[f] = fmt.Sprintf("%s: pid %s had no %s in a snapshot", role, pid, f)
					continue
				}
				if f == "rss_peak_kb" {
					if *av > sums[f] {
						sums[f] = *av
					}
					continue
				}
				sums[f] += *av - *bv
			}
		}
		if n == 0 {
			if _, said := missing[role]; !said {
				missing[role] = role + ": no process in both snapshots"
			}
			continue
		}
		put := func(name string, fields ...string) {
			total := 0.0
			for _, f := range fields {
				if why, bad := unread[f]; bad {
					missing[name] = why
					return
				}
				total += sums[f]
			}
			delta[name] = total
		}
		put(role+".cpu_user", "utime_ms")
		put(role+".cpu_sys", "stime_ms")
		put(role+".ctxsw_vol", "ctxsw_vol")
		put(role+".ctxsw_invol", "ctxsw_invol")
		put(role+".runq_wait", "runq_wait_ms")
		put(role+".rw_syscalls", "syscr", "syscw")
		put(role+".io_read_bytes", "read_bytes")
		put(role+".io_write_bytes", "write_bytes")
		put(role+".page_faults", "minflt", "majflt")
		put(role+".rss_peak", "rss_peak_kb")
		if v, ok := delta[role+".rss_peak"]; ok {
			delta[role+".rss_peak"] = v * 1024
		}
	}
	if pre.Disk == nil || post.Disk == nil {
		missing["disk"] = "no disk row in a snapshot"
	} else {
		pick := func(name string, a, b *float64) {
			if a == nil || b == nil {
				missing[name] = name + ": the kernel did not give the field"
				return
			}
			delta[name] = *a - *b
		}
		pick("dev_reads", post.Disk.Reads, pre.Disk.Reads)
		pick("dev_writes", post.Disk.Writes, pre.Disk.Writes)
		pick("dev_flushes", post.Disk.Flushes, pre.Disk.Flushes)
		pick("dev_busy", post.Disk.BusyMs, pre.Disk.BusyMs)
	}
	for _, e := range post.Errors {
		missing["l0:"+e] = e
	}
	return
}

// clientDelta turns the client's self-report into the same names. A field
// the client reported as null (a utility wrapper cannot see its child's
// context switches; a container without eth0 has no packets) is missing,
// with the reason, not zero.
func clientDelta(self map[string]*float64) (delta map[string]float64, missing map[string]string) {
	delta, missing = map[string]float64{}, map[string]string{}
	if self == nil {
		return
	}
	sum := func(name string, keys ...string) {
		total := 0.0
		for _, k := range keys {
			v, ok := self[k]
			if !ok || v == nil {
				missing[name] = name + ": the client reported no " + k
				return
			}
			total += *v
		}
		delta[name] = total
	}
	sum("client.cpu_user", "utime_ms")
	sum("client.cpu_sys", "stime_ms")
	sum("client.ctxsw_vol", "ctxsw_vol")
	sum("client.ctxsw_invol", "ctxsw_invol")
	sum("client.rw_syscalls", "syscr", "syscw")
	sum("client.io_read_bytes", "read_bytes")
	sum("client.io_write_bytes", "write_bytes")
	sum("client.page_faults", "minflt", "majflt")
	sum("client.rss_peak", "rss_peak_kb")
	if v, ok := delta["client.rss_peak"]; ok {
		delta["client.rss_peak"] = v * 1024
	}
	sum("net_packets", "net_rx_packets", "net_tx_packets")
	sum("net_bytes", "net_rx_bytes", "net_tx_bytes")
	return
}

// statdump

// watcherStart keeps a `cubrid statdump -i` running against a database, because
// the Num_* statistics accumulate only while one is registered (Design §5.6).
// Each start gets a new serial so the series files do not overwrite.
func (r *Runner) watcherStart(ctx context.Context, s *Side, db string) error {
	s.watchers[db]++
	script := fmt.Sprintf("bash %s/scripts/statdump_window.sh start %s %d", workPerf, db, s.watchers[db])
	return r.mustExec(ctx, s, "statdump watcher", script, time.Minute)
}

// watcherEnsure re-arms the watcher when it died (a server restart ends it)
// and says so.
func (r *Runner) watcherEnsure(ctx context.Context, s *Side, db string) (note string, err error) {
	res, err := r.exec(ctx, s.N1, fmt.Sprintf("bash %s/scripts/statdump_window.sh status %s", workPerf, db), time.Minute)
	if err != nil {
		return "", err
	}
	if res.ExitCode == 0 {
		return "", nil
	}
	if err := r.watcherStart(ctx, s, db); err != nil {
		return "", err
	}
	return "statdump watcher was not running and was re-armed", nil
}

func (r *Runner) statdumpSnapshot(ctx context.Context, s *Side, db, outDirInNode, hostDir, name string) (map[string]float64, error) {
	script := fmt.Sprintf("bash %s/scripts/statdump_window.sh snapshot %s %s/%s", workPerf, db, outDirInNode, name)
	res, err := r.exec(ctx, s.N1, script, time.Minute)
	if err != nil {
		return nil, err
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("statdump: exit %d: %s", res.ExitCode, tail(res.Stderr, 300))
	}
	f, err := os.Open(filepath.Join(hostDir, name))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseStatdump(f)
}

// parseStatdump reads `Name = value` lines; anything else (headers, the
// multi-line complex statistics) is skipped.
func parseStatdump(r interface{ Read([]byte) (int, error) }) (map[string]float64, error) {
	out := map[string]float64{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		// A statistic is flush-left; the indented rows under a complex one
		// (Num_data_page_fix_ext: PAGE_FTAB,... = 3) are not statistics by
		// themselves.
		if line == "" || line[0] == ' ' || line[0] == '\t' {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if k == "" || strings.ContainsAny(k, " \t") {
			continue
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			continue
		}
		out[k] = f
	}
	return out, sc.Err()
}

func statdumpDelta(pre, post map[string]float64, wanted []string) (delta map[string]float64, missing map[string]string) {
	delta, missing = map[string]float64{}, map[string]string{}
	for _, name := range wanted {
		if !statdumpNames[name] {
			continue
		}
		a, okA := post[name]
		b, okB := pre[name]
		if !okA || !okB {
			missing[name] = name + ": not in the statdump output"
			continue
		}
		delta[name] = a - b
	}
	return
}

// perOp divides raw differences by the ops that produced them. The client's
// own counters (client.*, net_*) cover the measured window and are divided by
// the measured ops; everything read in the database node -- the roles, the
// disk, statdump -- was snapshotted before the warm-up and after the
// measurement, so it is divided by the warm-up's ops too. Without that, a
// server-side value is inflated by the warm-up's share and moves with the
// warm-up's speed, which FR-21 would read as a workload change. A divisor
// of zero makes the value null; a counter that did not move is 0, not null
// (so a build that stopped doing something is seen as a change).
func perOp(raw map[string]float64, measuredOps, warmOps float64, warmKnown bool) (out map[string]*float64, missing map[string]string) {
	out, missing = map[string]*float64{}, map[string]string{}
	names := make([]string, 0, len(raw))
	for k := range raw {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		v := raw[k]
		divisor := measuredOps + warmOps
		if clientSide(k) {
			divisor = measuredOps
		} else if !warmKnown {
			missing[k] = k + ": the client did not report warm_ops, and the snapshot includes the warm-up"
			continue
		}
		if divisor <= 0 {
			out[k] = nil
			continue
		}
		x := v / divisor
		out[k] = &x
	}
	return out, missing
}

// clientSide says whether a counter was measured by the client over its
// measured window alone.
func clientSide(name string) bool {
	return strings.HasPrefix(name, "client.") || strings.HasPrefix(name, "net_")
}
