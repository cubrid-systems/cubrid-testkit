package perf

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// What the session reads from the hub around it (Design §6.1 hub.go): the
// bench-mode and boost state it records, the lease it must still hold at
// every case boundary (L7), the servers outside its containers it must not
// share the cores with (guard stage 1, FR-6.2), the previous session's
// fingerprints (FR-2), and the memory and disk it checks before starting.

// BenchMode is what session.json says about the host's state (FR-6.1).
type BenchMode struct {
	On    bool   `json:"on"`
	Note  string `json:"note,omitempty"`
	Boost *bool  `json:"boost"` // nil when the sysfs file could not be read
}

const (
	benchModeFile = "/run/bench-mode"
	boostFile     = "/sys/devices/system/cpu/cpufreq/boost"
)

func benchModeState() BenchMode {
	bm := BenchMode{}
	if b, err := os.ReadFile(benchModeFile); err == nil {
		bm.On = true
		bm.Note = strings.TrimSpace(string(b))
	} else {
		bm.Note = "no " + benchModeFile
	}
	if b, err := os.ReadFile(boostFile); err == nil {
		on := strings.TrimSpace(string(b)) == "1"
		bm.Boost = &on
	}
	return bm
}

// The lease (bench-hub): $BENCH_RUNS/.lease.json, {id, client, user, since,
// heartbeat}. The session holds it when the id is its own run id, which is
// the stage directory's name bench-client exported as REPORTS_DIR.

type lease struct {
	ID        string `json:"id"`
	Client    string `json:"client"`
	User      string `json:"user"`
	Since     string `json:"since"`
	Heartbeat string `json:"heartbeat"`
}

func runsRoot() string {
	if r := os.Getenv("BENCH_RUNS"); r != "" {
		return r
	}
	return "/data/runs"
}

func readLease(path string) (*lease, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var l lease
	if err := json.Unmarshal(b, &l); err != nil {
		return nil, err
	}
	return &l, nil
}

// leaseHeldBy says whether the lease file names this run. bench-hub rewrites
// the file on every heartbeat, so a read that fails or does not parse is
// tried again before it counts: the lease is lost when a lease that parses
// names another run, or when the file stays unreadable.
func leaseHeldBy(path, runID string) bool {
	for i := 0; i < 3; i++ {
		l, err := readLease(path)
		if err == nil {
			return l.ID == runID
		}
		time.Sleep(time.Second)
	}
	return false
}

// Guard stage 1: a cub_server, postgres or mysqld on the host that is not in
// a container is a neighbour on the server cores, and the session does not
// start beside one (Design §7). Processes inside containers are left alone:
// the clusters' own servers are there, and so is Conbench's postgres.

var foreignNames = map[string]bool{"cub_server": true, "postgres": true, "mysqld": true}

type procInfo struct {
	PID       int
	Comm      string
	Args      string
	UID       int
	State     string // from /proc/<pid>/status: R, S, D, Z, ...
	Container bool   // in a container cgroup (left alone; reported)
}

func (p procInfo) String() string {
	return fmt.Sprintf("%d %s (uid %d): %s", p.PID, p.Comm, p.UID, p.Args)
}

// serverProcesses scans procRoot (/proc) for the three server programs,
// saying for each whether it is inside a container cgroup. A zombie is not
// a process and is left out.
func serverProcesses(procRoot string) []procInfo {
	ents, err := os.ReadDir(procRoot)
	if err != nil {
		return nil
	}
	var out []procInfo
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || !e.IsDir() {
			continue
		}
		dir := filepath.Join(procRoot, e.Name())
		comm, err := os.ReadFile(filepath.Join(dir, "comm"))
		if err != nil {
			continue
		}
		name := strings.TrimSpace(string(comm))
		if !foreignNames[name] {
			continue
		}
		p := procInfo{PID: pid, Comm: name, UID: -1, Container: inContainer(dir)}
		if b, err := os.ReadFile(filepath.Join(dir, "cmdline")); err == nil {
			p.Args = strings.TrimSpace(strings.ReplaceAll(string(b), "\x00", " "))
		}
		if b, err := os.ReadFile(filepath.Join(dir, "status")); err == nil {
			for _, line := range strings.Split(string(b), "\n") {
				if f := strings.Fields(line); len(f) > 1 {
					switch f[0] {
					case "Uid:":
						p.UID, _ = strconv.Atoi(f[1])
					case "State:":
						p.State = f[1]
					}
				}
			}
		}
		if p.State == "Z" {
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PID < out[j].PID })
	return out
}

// foreignServers is the servers outside any container: the ones the guard
// ends. The containers' servers (the clusters' own, Conbench's postgres, a
// colleague's csb cluster) are reported by guardBefore and left alone.
func foreignServers(procRoot string) []procInfo {
	var out []procInfo
	for _, p := range serverProcesses(procRoot) {
		if !p.Container {
			out = append(out, p)
		}
	}
	return out
}

func inContainer(procDir string) bool {
	b, err := os.ReadFile(filepath.Join(procDir, "cgroup"))
	if err != nil {
		return false
	}
	s := string(b)
	for _, marker := range []string{"libpod-", "docker-", "docker/", "containerd", "/machine.slice/"} {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}

// GuardReport is what session.json carries about stage 1.
type GuardReport struct {
	Before       []string `json:"foreign_before"`
	Cleaned      []string `json:"cleaned"`
	Left         []string `json:"left"`
	OtherUsers   []string `json:"other_users"`   // servers this user cannot signal; the session goes on and says so
	InContainers []string `json:"in_containers"` // servers inside containers that are not pf- clusters; left alone
	PSSnapshot   string   `json:"ps_snapshot,omitempty"`
}

// guardBefore records the host's servers, ends the foreign ones that are
// this user's (TERM -- INT for postgres, whose postmaster treats TERM as a
// smart shutdown that waits for its clients -- then KILL), and reports what
// would not go. Servers of other users cannot be ended and are reported,
// not refused: a refusal every week would say nothing new. With clean=false
// it only looks.
func guardBefore(outDir string, clean bool) (GuardReport, error) {
	g := GuardReport{Before: []string{}, Cleaned: []string{}, Left: []string{}, OtherUsers: []string{}, InContainers: []string{}}
	if err := os.MkdirAll(outDir, 0o755); err == nil {
		if out, err := exec.Command("ps", "-eo", "pid,uid,ppid,etime,comm,args").Output(); err == nil {
			path := filepath.Join(outDir, "ps-before.txt")
			if os.WriteFile(path, out, 0o644) == nil {
				g.PSSnapshot = path
			}
		}
	}
	me := os.Getuid()
	perf := perfContainers()
	var mine []procInfo
	for _, p := range serverProcesses("/proc") {
		switch {
		case p.Container:
			if !inPerfCluster(p.PID, perf) {
				g.InContainers = append(g.InContainers, p.String())
			}
		case p.UID != me:
			g.OtherUsers = append(g.OtherUsers, p.String())
		default:
			g.Before = append(g.Before, p.String())
			mine = append(mine, p)
		}
	}
	if !clean || len(mine) == 0 {
		return g, nil
	}
	ownForeign := func() []procInfo {
		var out []procInfo
		for _, p := range foreignServers("/proc") {
			if p.UID == me {
				out = append(out, p)
			}
		}
		return out
	}
	for _, p := range mine {
		sig := syscall.SIGTERM
		if p.Comm == "postgres" {
			sig = syscall.SIGINT
		}
		_ = syscall.Kill(p.PID, sig)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) && len(ownForeign()) > 0 {
		time.Sleep(time.Second)
	}
	for _, p := range ownForeign() {
		_ = syscall.Kill(p.PID, syscall.SIGKILL)
	}
	time.Sleep(2 * time.Second)
	left := ownForeign()
	leftPIDs := map[int]bool{}
	for _, p := range left {
		leftPIDs[p.PID] = true
		g.Left = append(g.Left, p.String())
	}
	for _, p := range mine {
		if !leftPIDs[p.PID] {
			g.Cleaned = append(g.Cleaned, p.String())
		}
	}
	if len(left) > 0 {
		return g, fmt.Errorf("%d server process(es) outside the clusters would not stop: %s", len(left), strings.Join(g.Left, "; "))
	}
	return g, nil
}

// perfContainers is the runtime's id of every pf- container, by name.
func perfContainers() map[string]string {
	ids := map[string]string{}
	out, err := exec.Command(runtimeBin(), "ps", "-a", "--format", "{{.ID}} {{.Names}}").Output()
	if err != nil {
		return ids
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && strings.HasPrefix(f[1], "pf-") {
			ids[f[1]] = f[0]
		}
	}
	return ids
}

// inPerfCluster says whether a container process belongs to a pf- cluster:
// its cgroup path carries the container's id.
func inPerfCluster(pid int, perf map[string]string) bool {
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cgroup"))
	if err != nil {
		return false
	}
	for _, id := range perf {
		if id != "" && strings.Contains(string(b), id) {
			return true
		}
	}
	return false
}

// Previous sessions (FR-2): the newest run of the same name under the runs
// root, its session.json beside the results or one level down.

func previousSessions(root, selfID string) []string {
	ents, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var ids []string
	for _, e := range ents {
		if !e.IsDir() || e.Name() == selfID || !strings.Contains(e.Name(), "_perf-weekly") {
			continue
		}
		ids = append(ids, e.Name())
	}
	sort.Sort(sort.Reverse(sort.StringSlice(ids)))
	return ids
}

func readSessionPairs(root, id string) []*SessionPair {
	for _, rel := range []string{"results/session.json", "session.json"} {
		b, err := os.ReadFile(filepath.Join(root, id, rel))
		if err != nil {
			continue
		}
		var doc struct {
			Pairs []*SessionPair `json:"pairs"`
		}
		if json.Unmarshal(b, &doc) != nil {
			return nil
		}
		return doc.Pairs
	}
	return nil
}

// previousPair finds the newest earlier session that measured a pair of
// this name: one that was skipped, or whose fingerprint is empty, is not a
// session to compare against and is passed over (FR-2).
func previousPair(root, selfID, name string) (*SessionPair, string) {
	for _, id := range previousSessions(root, selfID) {
		for _, p := range readSessionPairs(root, id) {
			if p.Name == name && p.Skipped == "" && (p.Target.Fingerprint.Compiler != "" || p.Target.Fingerprint.BuildType != "") {
				return p, id
			}
		}
	}
	return nil, ""
}

// fingerprintDiff names the groups that differ between two fingerprints, in
// the words the summary's first line uses.
func fingerprintDiff(prev, now Fingerprint) string {
	var parts []string
	if prev.Compiler != now.Compiler {
		parts = append(parts, fmt.Sprintf("compiler %s → %s", orUnknown(prev.Compiler), orUnknown(now.Compiler)))
	}
	if prev.BuildType != now.BuildType {
		parts = append(parts, fmt.Sprintf("build_type %s → %s", orUnknown(prev.BuildType), orUnknown(now.BuildType)))
	}
	if prev.CXXFlags != now.CXXFlags {
		parts = append(parts, fmt.Sprintf("cxx_flags %q → %q", prev.CXXFlags, now.CXXFlags))
	}
	libs := map[string]bool{}
	for k := range prev.Thirdparty {
		libs[k] = true
	}
	for k := range now.Thirdparty {
		libs[k] = true
	}
	for _, k := range sortedKeys(libs) {
		if prev.Thirdparty[k] != now.Thirdparty[k] {
			parts = append(parts, fmt.Sprintf("%s %s → %s", k, orUnknown(prev.Thirdparty[k]), orUnknown(now.Thirdparty[k])))
		}
	}
	return strings.Join(parts, ", ")
}

func orUnknown(s string) string {
	if s == "" {
		return "?"
	}
	return s
}

// Memory and disk (Design §7, §11).

// parseSize reads 24G, 512M, 16k, or a bare byte count.
func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty size")
	}
	mult := int64(1)
	switch s[len(s)-1] {
	case 'k', 'K':
		mult, s = 1<<10, s[:len(s)-1]
	case 'm', 'M':
		mult, s = 1<<20, s[:len(s)-1]
	case 'g', 'G':
		mult, s = 1<<30, s[:len(s)-1]
	case 't', 'T':
		mult, s = 1<<40, s[:len(s)-1]
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, err
	}
	return n * mult, nil
}

func memAvailable() (int64, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "MemAvailable:") {
			fs := strings.Fields(sc.Text())
			if len(fs) >= 2 {
				kb, err := strconv.ParseInt(fs[1], 10, 64)
				return kb << 10, err
			}
		}
	}
	return 0, errors.New("no MemAvailable in /proc/meminfo")
}

func diskFree(path string) (int64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	return int64(st.Bavail) * int64(st.Bsize), nil
}

func osExecOutput(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return string(out), err
}

// The container runtime, for what csb has no verb for: pausing a cluster's
// containers, and finding a container's cgroup. $CSB_BACKEND names it; else
// whichever of podman and docker is here.
func runtimeBin() string {
	if b := os.Getenv("CSB_BACKEND"); b != "" {
		return b
	}
	for _, b := range []string{"podman", "docker"} {
		if _, err := exec.LookPath(b); err == nil {
			return b
		}
	}
	return "podman"
}

func clusterContainers(cluster string) ([]string, error) {
	out, err := exec.Command(runtimeBin(), "ps", "-a", "--filter", "label=csb.cluster="+cluster, "--format", "{{.Names}}").Output()
	if err != nil {
		return nil, fmt.Errorf("%s ps: %v", runtimeBin(), err)
	}
	names := strings.Fields(string(out))
	sort.Strings(names)
	return names, nil
}

// containerCgroup is the cgroup directory of a container's scope, read from
// /proc of its init so it is the same whichever runtime and cgroup driver
// made it. The init's own leaf may be a child of the scope (podman puts it
// in <scope>/container); cpu.stat at the scope covers every process the
// runtime started in the container, exec'd ones included.
func containerCgroup(container string) (string, error) {
	out, err := exec.Command(runtimeBin(), "inspect", "--format", "{{.State.Pid}}", container).Output()
	if err != nil {
		return "", fmt.Errorf("%s inspect %s: %v", runtimeBin(), container, err)
	}
	pid := strings.TrimSpace(string(out))
	b, err := os.ReadFile(filepath.Join("/proc", pid, "cgroup"))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(b), "\n") {
		// cgroup v2: 0::/user.slice/.../libpod-<id>.scope[/container]
		if strings.HasPrefix(line, "0::") {
			path := strings.TrimPrefix(line, "0::")
			if i := strings.Index(path, ".scope/"); i >= 0 {
				path = path[:i+len(".scope")]
			}
			return filepath.Join("/sys/fs/cgroup", path), nil
		}
	}
	return "", fmt.Errorf("no cgroup v2 line for pid %s", pid)
}

// Guard stage 3: the CPU time the server cores spent outside the clusters'
// containers, and the pages swapped in, across a measured pass (Design §7).

// hostSnapshot is the host's counters at one instant.
type hostSnapshot struct {
	at         time.Time
	busyS      float64 // server cores, user+nice+system+steal
	irqS       float64 // server cores, irq+softirq -- the measured side's own network, mostly
	clusterS   float64 // the clusters' server containers, cpu.stat usage
	pswpin     float64
	unreadable []string
}

// hostDelta is what counters.json carries for a measured pass.
type hostDelta struct {
	ElapsedS   float64  `json:"elapsed_s"`
	ServerBusy float64  `json:"server_cores_busy_s"`
	ServerIRQ  float64  `json:"server_cores_irq_s"`
	ClusterCPU float64  `json:"cluster_cpu_s"`
	ForeignCPU float64  `json:"foreign_cpu_s"`
	Pswpin     float64  `json:"pswpin"`
	Unreadable []string `json:"unreadable,omitempty"`
}

// contaminationCores is the foreign CPU, averaged over the window as a number
// of cores, above which a pass is null(contaminated). Zero: the reading is
// recorded on every measured pass and nulls nothing, because the threshold
// is T1's to set (Spec FR-6.2): kernel writeback for the measured side is
// outside the container's cgroup and so counts as foreign here, and a canary
// nulled by a wrong threshold invalidates the whole pair. A swap-in during a
// pass nulls it whatever the threshold.
const contaminationCores = 0

// clockTicks is CLK_TCK; 100 on every Linux this runs on.
const clockTicks = 100.0

// hostGuard reads the counters for a given set of server cores and the
// cgroups of the clusters currently up, by cluster name: a cluster that is
// destroyed leaves the list, or its vanished cpu.stat would make every later
// reading unreadable.
type hostGuard struct {
	cores     []int
	cgroups   map[string]string
	threshold float64 // contaminationCores; 0 records without nulling
}

func (g *hostGuard) add(cluster, cgroup string) {
	if g.cgroups == nil {
		g.cgroups = map[string]string{}
	}
	g.cgroups[cluster] = cgroup
}

func (g *hostGuard) remove(cluster string) {
	delete(g.cgroups, cluster)
}

func parseCPUList(s string) ([]int, error) {
	var out []int
	for _, part := range strings.Split(strings.TrimSpace(s), ",") {
		if part == "" {
			continue
		}
		lo, hi, isRange := strings.Cut(part, "-")
		a, err := strconv.Atoi(lo)
		if err != nil {
			return nil, fmt.Errorf("cpu list %q", s)
		}
		b := a
		if isRange {
			if b, err = strconv.Atoi(hi); err != nil || b < a {
				return nil, fmt.Errorf("cpu list %q", s)
			}
		}
		for i := a; i <= b; i++ {
			out = append(out, i)
		}
	}
	return out, nil
}

// serverBusyFromStat sums the busy jiffies of the named cores in /proc/stat:
// user+nice+system+steal as busy, irq+softirq apart.
func serverBusyFromStat(stat string, cores []int) (busy, irq float64, err error) {
	want := map[string]bool{}
	for _, c := range cores {
		want["cpu"+strconv.Itoa(c)] = true
	}
	seen := 0
	for _, line := range strings.Split(stat, "\n") {
		fs := strings.Fields(line)
		if len(fs) < 8 || !want[fs[0]] {
			continue
		}
		seen++
		// user nice system idle iowait irq softirq steal
		for _, i := range []int{1, 2, 3, 8} {
			if i < len(fs) {
				v, _ := strconv.ParseFloat(fs[i], 64)
				busy += v
			}
		}
		for _, i := range []int{6, 7} {
			v, _ := strconv.ParseFloat(fs[i], 64)
			irq += v
		}
	}
	if seen != len(cores) {
		return 0, 0, fmt.Errorf("/proc/stat has %d of the %d server cores", seen, len(cores))
	}
	return busy / clockTicks, irq / clockTicks, nil
}

func cgroupUsageS(dir string) (float64, error) {
	b, err := os.ReadFile(filepath.Join(dir, "cpu.stat"))
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "usage_usec ") {
			v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimPrefix(line, "usage_usec ")), 64)
			return v / 1e6, err
		}
	}
	return 0, fmt.Errorf("%s: no usage_usec", dir)
}

func vmstatValue(name string) (float64, error) {
	b, err := os.ReadFile("/proc/vmstat")
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, name+" ") {
			return strconv.ParseFloat(strings.TrimSpace(strings.TrimPrefix(line, name+" ")), 64)
		}
	}
	return 0, fmt.Errorf("no %s in /proc/vmstat", name)
}

func (g *hostGuard) snapshot() hostSnapshot {
	s := hostSnapshot{at: time.Now()}
	if b, err := os.ReadFile("/proc/stat"); err != nil {
		s.unreadable = append(s.unreadable, "/proc/stat: "+err.Error())
	} else if s.busyS, s.irqS, err = serverBusyFromStat(string(b), g.cores); err != nil {
		s.unreadable = append(s.unreadable, err.Error())
	}
	for _, name := range sortedKeys(g.cgroups) {
		u, err := cgroupUsageS(g.cgroups[name])
		if err != nil {
			s.unreadable = append(s.unreadable, err.Error())
			continue
		}
		s.clusterS += u
	}
	var err error
	if s.pswpin, err = vmstatValue("pswpin"); err != nil {
		s.unreadable = append(s.unreadable, err.Error())
	}
	return s
}

func (g *hostGuard) delta(pre, post hostSnapshot) *hostDelta {
	d := &hostDelta{ElapsedS: post.at.Sub(pre.at).Seconds()}
	d.ServerBusy = post.busyS - pre.busyS
	d.ServerIRQ = post.irqS - pre.irqS
	d.ClusterCPU = post.clusterS - pre.clusterS
	d.ForeignCPU = d.ServerBusy - d.ClusterCPU
	if d.ForeignCPU < 0 {
		d.ForeignCPU = 0
	}
	d.Pswpin = post.pswpin - pre.pswpin
	d.Unreadable = append(append([]string{}, pre.unreadable...), post.unreadable...)
	return d
}

// contaminated says whether the delta is beyond what a measured pass may
// carry: any swap-in, or more than threshold cores of foreign CPU on average
// (a threshold of 0 means record only). An unreadable counter is not
// contamination.
func (d *hostDelta) contaminated(threshold float64) string {
	if len(d.Unreadable) > 0 || d.ElapsedS <= 0 {
		return ""
	}
	if d.Pswpin > 0 {
		return fmt.Sprintf("contaminated: %.0f pages swapped in during the pass", d.Pswpin)
	}
	if threshold > 0 && d.ForeignCPU/d.ElapsedS > threshold {
		return fmt.Sprintf("contaminated: %.1f s of CPU on the server cores outside the clusters over %.0f s", d.ForeignCPU, d.ElapsedS)
	}
	return ""
}
