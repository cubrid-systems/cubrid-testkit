package shellsuite

import (
	"fmt"
	"os"
	"strings"

	"github.com/cubrid-systems/cubrid-testkit/internal/conf"
	"github.com/cubrid-systems/cubrid-testkit/internal/contain"
	"github.com/cubrid-systems/cubrid-testkit/internal/sizing"
	"github.com/cubrid-systems/cubrid-testkit/internal/topology"
)

// slotsFor is how many slots a contained run gets, and the sentence that says
// why (ADR-020). It is decided before discovery -- lanes and slots need the
// count first -- so the corpus is known by its path and not by its cases.
//
// Shell is the suite the knee is for: on disk with its syncs, eight slots were
// slower than four (category/shell §4), and a machine finds that by running.
func slotsFor(cfg *conf.Config, machine *topology.Instance, ramMB int, lanes, onDisk bool, availMB int, recs []sizing.Record) sizing.Plan {
	mode, warn := sizing.ModeOf(cfg.GetOr("parallel", ""))
	configured, _ := cfg.Get("parallel_slots")
	p := sizing.Slots(sizing.Input{
		Suite:      sizing.Shell,
		Configured: configured,
		Mode:       mode,
		Engine:     engineOf(machine),
		AvailMB:    availMB,
		// The tmpfs can fill to its ceiling, so all of it is set aside.
		SharedMB: max(ramMB, 0),
		Corpus:   strings.TrimSpace(cfg.GetOr("scenario", "")),
		Lane:     shellLane(ramMB, lanes, onDisk),
		Records:  recs,
	})
	if warn != "" {
		p.Why = warn + "; " + p.Why
	}
	return p
}

// engineOf is the buffers the slots' servers are configured with: the install's
// cubrid.conf, and the configuration's cubrid parameters over it, which is what
// DEPLOY writes. A case that creates its own database can ask for others.
func engineOf(machine *topology.Instance) sizing.Engine {
	role := machine.Role(topology.RoleCUBRID)
	return sizing.EngineFromConf(os.Getenv("CUBRID")).With(role["data_buffer_size"], role["log_buffer_size"])
}

// shellLane is where this run's case writes go. Shell has more places than the
// other suites: memory, memory and disk in two lanes, a slot's overlay of the
// corpus, or the corpus directory itself.
func shellLane(ramMB int, lanes, onDisk bool) string {
	switch {
	case ramMB > 0 && lanes:
		return "memory and disk lanes"
	case ramMB > 0:
		return "memory"
	case onDisk:
		return contain.Lane()
	default:
		return "disk, in the corpus"
	}
}

// recordSizing writes what this run measured, for the next run on this machine.
func recordSizing(m *sizing.Meter, cfg *conf.Config, machine *topology.Instance, ramMB int, lanes, onDisk bool, slots int) {
	r := m.Stop()
	r.Slots, r.Engine = slots, engineOf(machine)
	r.Corpus, r.Lane = strings.TrimSpace(cfg.GetOr("scenario", "")), shellLane(ramMB, lanes, onDisk)
	if err := sizing.Save(sizing.Shell, r); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] this run's sizing was not recorded: %v\n", err)
		return
	}
	fmt.Fprintf(os.Stderr, "[INFO] sizing recorded at %s: %d slots, %d MB at the peak (%d of it the run's writes), %d s of cases, the longest first attempt %d s\n",
		sizing.Path(sizing.Shell), r.Slots, r.PeakMB, r.SharedMB, r.CaseS, r.LongestUnitS)
}
