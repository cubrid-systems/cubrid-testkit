package isolationsuite

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/conf"
	"github.com/cubrid-systems/cubrid-testkit/internal/contain"
	"github.com/cubrid-systems/cubrid-testkit/internal/sizing"
)

// sizingInput is what this run is sized from (ADR-020): parallel_slots when it
// is written, and otherwise the machine's own runs of this corpus on this lane,
// as `parallel` asks -- conservative, measured or aggressive.
func sizingInput(cfg *conf.Config, corpus string, cases, availMB int, recs []sizing.Record) (sizing.Input, string) {
	mode, warn := sizing.ModeOf(cfg.GetOr("parallel", ""))
	configured, _ := cfg.Get("parallel_slots")
	return sizing.Input{
		Suite:      sizing.Isolation,
		Configured: configured,
		Mode:       mode,
		Engine:     engineOf(cfg),
		AvailMB:    availMB,
		Cases:      cases,
		Corpus:     corpus,
		Lane:       contain.Lane(),
		Records:    recs,
	}, warn
}

// slotsFor is how many slots a run gets, and the sentence that says why.
func slotsFor(cfg *conf.Config, corpus string, cases, availMB int, recs []sizing.Record) sizing.Plan {
	in, warn := sizingInput(cfg, corpus, cases, availMB, recs)
	p := sizing.Slots(in)
	if warn != "" {
		p.Why = warn + "; " + p.Why
	}
	return p
}

// engineOf is the buffers the slots' servers run with: the install's
// cubrid.conf, as DEPLOY leaves it after writing the configuration's
// default.cubrid.* keys into each slot.
func engineOf(cfg *conf.Config) sizing.Engine {
	return sizing.EngineFromConf(os.Getenv("CUBRID")).With(
		cfg.GetOr("default.cubrid.data_buffer_size", ""), cfg.GetOr("default.cubrid.log_buffer_size", ""))
}

// sampleEvery is how often a run looks at the machine's available memory. A
// slot's server takes seconds to reach its size, and the peak holds for most of
// the run, so five seconds misses nothing a budget needs.
const sampleEvery = 5 * time.Second

// firstAttempt reports whether runone.sh's output is a single attempt: its
// `set -x` trace has one `+ elapse=` line for each time it ran the controller.
func firstAttempt(out string) bool {
	return strings.Count(out, "\n+ elapse=") == 1
}

// record writes what this run measured, for the next run on this machine. Only a
// run of the whole of what it discovered is written: a continued run is a
// remainder, and its case seconds would bound the next run by the remainder.
func record(m *sizing.Meter, cfg *conf.Config, corpus string, slots int) {
	r := m.Stop()
	r.Slots, r.Engine, r.Corpus, r.Lane = slots, engineOf(cfg), corpus, contain.Lane()
	if err := sizing.Save(sizing.Isolation, r); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] this run's sizing was not recorded: %v\n", err)
		return
	}
	fmt.Fprintf(os.Stderr, "[INFO] sizing recorded at %s: %d slots, %d MB at the peak, %d s of cases, the longest first attempt %d s\n",
		sizing.Path(sizing.Isolation), r.Slots, r.PeakMB, r.CaseS, r.LongestUnitS)
}
