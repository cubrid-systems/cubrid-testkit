package isolationsuite

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/cubrid-systems/cubrid-testkit/internal/conf"
)

// Slots are the default for isolation, decided on 2026-09-15 once ADR-018's
// rules found no runner difference with four slots over the whole corpus, in
// 2,978 s against 12,301 s for one (docs/project/evidence/isolation-baseline.md
// §4). parallel_slots still decides; a configuration that does not say gets
// defaultSlots, or fewer on a machine that cannot hold them.
const defaultSlots = 4

// slotMB is what one slot is budgeted. The server is most of it and is its
// buffers plus a floor: 1.38 GB at its peak with the shipped 512 MB data buffer
// and 256 MB log buffer, measured on the same engine for the sql family
// (evidence/sql-native.md §3). The rest of an isolation slot -- cub_pl,
// cub_master, qactl and its clients -- came to about 110 MB at the peak of a
// four-slot run of the sample, where each server was 595 MB
// (evidence/isolation-baseline.md §2).
const slotMB = 1500

// reservedMB is left to everything else on the machine, as scripts/sizing.sh
// leaves it.
const reservedMB = 2048

// slotsFor is how many slots a run gets, and the sentence that says why.
//
// cpus and availMB are the machine's; zero means unknown and bounds nothing. A
// value in the configuration is used as it is, whatever the machine: whoever
// wrote it has decided.
func slotsFor(cfg *conf.Config, cpus, availMB int) (int, string) {
	if v, ok := cfg.Get("parallel_slots"); ok && strings.TrimSpace(v) != "" {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || n < 1 {
			return 1, fmt.Sprintf("parallel_slots=%s is not a number of slots; running one", strings.TrimSpace(v))
		}
		return n, fmt.Sprintf("parallel_slots=%d", n)
	}

	n := defaultSlots
	why := fmt.Sprintf("parallel_slots is not set: %d slots, the default", n)
	if cpus > 0 && cpus < n {
		n = cpus
		why = fmt.Sprintf("parallel_slots is not set: %d slot(s), one for each CPU", n)
	}
	if availMB > 0 {
		byMem := (availMB - reservedMB) / slotMB
		if byMem < 1 {
			byMem = 1
		}
		if byMem < n {
			n = byMem
			why = fmt.Sprintf("parallel_slots is not set: %d slot(s), what %d MB of available memory holds at %d MB a slot "+
				"after %d MB for the rest", n, availMB, slotMB, reservedMB)
		}
	}
	return n, why
}

// memAvailableMB is what the kernel says can be handed out without swapping, or
// zero when it cannot be read.
func memAvailableMB() int {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == "MemAvailable:" {
			kb, err := strconv.Atoi(f[1])
			if err != nil {
				return 0
			}
			return kb / 1024
		}
	}
	return 0
}
