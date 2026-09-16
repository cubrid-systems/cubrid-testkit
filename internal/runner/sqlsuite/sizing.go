package sqlsuite

import (
	"fmt"
	"os"

	"github.com/cubrid-systems/cubrid-testkit/internal/cli"
	"github.com/cubrid-systems/cubrid-testkit/internal/conf"
	"github.com/cubrid-systems/cubrid-testkit/internal/contain"
	"github.com/cubrid-systems/cubrid-testkit/internal/sizing"
)

// suiteOf is which record a task is sized by: medium's cases are a different
// corpus with a different answer to how many slots pay (category/sql §5).
func suiteOf(task cli.Task) sizing.Suite {
	if task == cli.Medium {
		return sizing.Medium
	}
	return sizing.SQL
}

// slotsFor is how many slots a contained run gets, and the sentence that says
// why (ADR-020). It is asked after do_configure, so the install's cubrid.conf
// already holds the [sql/cubrid.conf] buffers every slot inherits.
func slotsFor(ini *conf.Ini, task cli.Task, corpus string, cases, availMB int, recs []sizing.Record) sizing.Plan {
	mode, warn := sizing.ModeOf(ini.GetOr("sql", "parallel", ""))
	configured, _ := ini.Get("sql", "parallel_slots")
	p := sizing.Slots(sizing.Input{
		Suite:      suiteOf(task),
		Configured: configured,
		Mode:       mode,
		Engine:     sizing.EngineFromConf(os.Getenv("CUBRID")),
		AvailMB:    availMB,
		Cases:      cases,
		Corpus:     corpus,
		Lane:       contain.Lane(),
		Records:    recs,
	})
	if warn != "" {
		p.Why = warn + "; " + p.Why
	}
	return p
}

// record writes what this run measured, for the next run on this machine.
func record(m *sizing.Meter, task cli.Task, corpus string, slots int) {
	r := m.Stop()
	r.Slots, r.Corpus, r.Lane = slots, corpus, contain.Lane()
	r.Engine = sizing.EngineFromConf(os.Getenv("CUBRID"))
	suite := suiteOf(task)
	if err := sizing.Save(suite, r); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] this run's sizing was not recorded: %v\n", err)
		return
	}
	fmt.Fprintf(os.Stderr, "[INFO] sizing recorded at %s: %d slots, %d MB at the peak, %d s of cases, the longest directory %d s\n",
		sizing.Path(suite), r.Slots, r.PeakMB, r.CaseS, r.LongestUnitS)
}
