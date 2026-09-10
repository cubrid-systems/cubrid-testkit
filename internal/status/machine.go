package status

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// What the run is competing for.
//
// Everything a parallel run has been wrong about has turned out to be a resource
// question -- eight slots slower than four because concurrent writes are seeks,
// a ceiling filled by one case writing nine gigabytes, a load of 50 on sixteen
// cores that was iowait rather than work. None of it was visible while a run was
// going, and a slot holding a case for four minutes reads the same whether it is
// waiting on a lock or on a disk with nothing left to give.
//
// So the panel is not a summary, it is an instrument: CPU split into what it is
// doing, memory split into what is holding it, and the disk in both bytes and
// operations, because those two say different things -- 300 MB/s in 300
// operations is a stream and in 30,000 is thrashing.
type machineView struct {
	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`
	Cores  int     `json:"cores"`
	Procs  int     `json:"procs"`

	// CPU, as a percentage of all cores over the last sample.
	CPUUser   float64 `json:"cpuUser"`
	CPUSys    float64 `json:"cpuSys"`
	CPUIOWait float64 `json:"cpuIowait"`
	CPUIdle   float64 `json:"cpuIdle"`
	CPUSteal  float64 `json:"cpuSteal"`

	// Memory in MB. Shmem is where a tmpfs lives, so the corpus overlay's upper
	// layer shows up there as well as in the corpus row -- the same megabytes
	// seen from the machine's side.
	MemAll   int `json:"memAll"`
	MemUsed  int `json:"memUsed"`
	MemFree  int `json:"memFree"`
	MemCache int `json:"memCache"`
	MemShmem int `json:"memShmem"`
	SwapAll  int `json:"swapAll"`
	SwapUsed int `json:"swapUsed"`

	// Disk over the last sample: megabytes a second and operations a second,
	// summed over the physical devices.
	ReadMBs  float64 `json:"readMBs"`
	WriteMBs float64 `json:"writeMBs"`
	ReadIOPS float64 `json:"readIops"`
	WritIOPS float64 `json:"writeIops"`

	// Where this run puts things: free space on the corpus's filesystem, and the
	// tmpfs the corpus writes into against the ceiling it was given.
	Corpus int `json:"corpus"`
	Ram    int `json:"ram"`
	RamCap int `json:"ramCap"`
}

// sampler holds the previous counters, because CPU and disk are counters and a
// rate needs two readings. One second: short enough that a stall is visible
// while it happens, long enough that the numbers are not noise.
type sampler struct {
	mu   sync.Mutex
	view machineView

	corpusDir, ramDir string
	ramCap            int

	prevCPU  [8]uint64
	prevDisk diskCounters
	prevAt   time.Time
	stop     chan struct{}
}

type diskCounters struct {
	readOps, writeOps, readSectors, writeSectors uint64
}

func newSampler(corpusDir, ramDir string, ramCap int) *sampler {
	s := &sampler{corpusDir: corpusDir, ramDir: ramDir, ramCap: ramCap, stop: make(chan struct{})}
	s.sample() // seed the counters; rates appear on the second reading
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-s.stop:
				return
			case <-t.C:
				s.sample()
			}
		}
	}()
	return s
}

func (s *sampler) snapshot() machineView {
	if s == nil {
		return machineView{Cores: runtime.NumCPU()}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.view
}

func (s *sampler) sample() {
	v := machineView{Cores: runtime.NumCPU(), RamCap: s.ramCap}
	now := time.Now()

	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		f := strings.Fields(string(b))
		if len(f) >= 3 {
			v.Load1, _ = strconv.ParseFloat(f[0], 64)
			v.Load5, _ = strconv.ParseFloat(f[1], 64)
			v.Load15, _ = strconv.ParseFloat(f[2], 64)
		}
		// "running/total" -- the second half is every process the kernel knows,
		// which inside a PID namespace is this run and nothing else.
		if len(f) >= 4 {
			if at := strings.IndexByte(f[3], '/'); at > 0 {
				v.Procs, _ = strconv.Atoi(f[3][at+1:])
			}
		}
	}

	if b, err := os.ReadFile("/proc/meminfo"); err == nil {
		m := map[string]int{}
		for _, line := range strings.Split(string(b), "\n") {
			f := strings.Fields(line)
			if len(f) < 2 {
				continue
			}
			n, _ := strconv.Atoi(f[1])
			m[strings.TrimSuffix(f[0], ":")] = n / 1024
		}
		v.MemAll, v.MemFree = m["MemTotal"], m["MemFree"]
		v.MemUsed = m["MemTotal"] - m["MemAvailable"]
		v.MemCache = m["Cached"] + m["Buffers"]
		v.MemShmem = m["Shmem"]
		v.SwapAll = m["SwapTotal"]
		v.SwapUsed = m["SwapTotal"] - m["SwapFree"]
	}

	cpu, okCPU := readCPU()
	disk := readDisk()
	elapsed := now.Sub(s.prevAt).Seconds()

	s.mu.Lock()
	defer s.mu.Unlock()

	if okCPU && !s.prevAt.IsZero() {
		var total float64
		var d [8]float64
		for i := range cpu {
			if cpu[i] >= s.prevCPU[i] {
				d[i] = float64(cpu[i] - s.prevCPU[i])
			}
			total += d[i]
		}
		if total > 0 {
			pct := func(x float64) float64 { return 100 * x / total }
			// user includes nice; system includes irq and softirq, which is where
			// the network and block layers charge their work.
			v.CPUUser = pct(d[0] + d[1])
			v.CPUSys = pct(d[2] + d[5] + d[6])
			v.CPUIdle = pct(d[3])
			v.CPUIOWait = pct(d[4])
			v.CPUSteal = pct(d[7])
		}
	}
	if !s.prevAt.IsZero() && elapsed > 0 {
		const sector = 512.0
		rate := func(now, was uint64) float64 {
			if now < was {
				return 0
			}
			return float64(now-was) / elapsed
		}
		v.ReadMBs = rate(disk.readSectors, s.prevDisk.readSectors) * sector / (1 << 20)
		v.WriteMBs = rate(disk.writeSectors, s.prevDisk.writeSectors) * sector / (1 << 20)
		v.ReadIOPS = rate(disk.readOps, s.prevDisk.readOps)
		v.WritIOPS = rate(disk.writeOps, s.prevDisk.writeOps)
	}
	if okCPU {
		s.prevCPU = cpu
	}
	s.prevDisk = disk
	s.prevAt = now

	v.Corpus = freeMB(s.corpusDir)
	v.Ram = usedMB(s.ramDir)
	s.view = v
}

// readCPU is the aggregate line of /proc/stat: user nice system idle iowait irq
// softirq steal.
func readCPU() ([8]uint64, bool) {
	var out [8]uint64
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return out, false
	}
	line, _, _ := strings.Cut(string(b), "\n")
	f := strings.Fields(line)
	if len(f) < 9 || f[0] != "cpu" {
		return out, false
	}
	for i := 0; i < 8; i++ {
		out[i], _ = strconv.ParseUint(f[i+1], 10, 64)
	}
	return out, true
}

// readDisk sums /proc/diskstats over the physical devices.
//
// Partitions are skipped, because the kernel counts a write to sdb1 on sdb as
// well and adding both doubles it. So are loop, ram and device-mapper devices --
// dm sits on top of something already counted.
func readDisk() diskCounters {
	var c diskCounters
	b, err := os.ReadFile("/proc/diskstats")
	if err != nil {
		return c
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 10 {
			continue
		}
		name := f[2]
		if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") ||
			strings.HasPrefix(name, "dm-") || strings.HasPrefix(name, "zram") {
			continue
		}
		// A partition ends in a digit on sd/vd/hd, and in "pN" on nvme and mmc.
		if last := name[len(name)-1]; last >= '0' && last <= '9' {
			if !strings.HasPrefix(name, "nvme") && !strings.HasPrefix(name, "mmcblk") {
				continue
			}
			if at := strings.LastIndexByte(name, 'p'); at > 0 {
				continue
			}
		}
		ro, _ := strconv.ParseUint(f[3], 10, 64)
		rs, _ := strconv.ParseUint(f[5], 10, 64)
		wo, _ := strconv.ParseUint(f[7], 10, 64)
		ws, _ := strconv.ParseUint(f[9], 10, 64)
		c.readOps += ro
		c.readSectors += rs
		c.writeOps += wo
		c.writeSectors += ws
	}
	return c
}
