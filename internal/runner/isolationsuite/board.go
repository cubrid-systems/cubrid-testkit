package isolationsuite

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cubrid-systems/cubrid-testkit/internal/conf"
	"github.com/cubrid-systems/cubrid-testkit/internal/contain"
	"github.com/cubrid-systems/cubrid-testkit/internal/status"
)

// openBoard is the status page, when status_http asks for one. It is off by
// default, and it says where it is on standard error: standard output is CTP's,
// and it is compared.
//
// A click on a finished case shows its block of feedback.log, which the
// isolation module writes in a shape the page already reads: the verdict line,
// and for a failure the diff.
func openBoard(cfg *conf.Config, cases []string, slots []*contain.Slot, onDisk bool, feedbackLog, build, sized string) (*status.Board, func()) {
	addr := status.Addr(cfg.GetOr("status_http", ""))
	if addr == "" {
		return nil, func() {}
	}
	board := status.New(len(cases))
	board.Expect(cases, nil, len(slots))
	where, stop, err := board.Open(addr)
	if err != nil {
		// The page is for watching the run; a run is not stopped for want of it.
		fmt.Fprintf(os.Stderr, "[WARN] no status page: %v\n", err)
		return nil, func() {}
	}
	fmt.Fprintf(os.Stderr, "[INFO] status page at http://%s/\n", where)
	board.Watch(os.Getenv("CUBRID"), "", 0)
	board.Detail(feedbackLog)

	lane := "disk"
	if contain.Volatile() {
		lane = "disk, volatile"
	}
	for _, s := range slots {
		board.Lane(s.Label, lane)
	}
	board.Setup([]status.Setting{
		{Group: "suite", Key: "task", Value: "isolation"},
		{Group: "suite", Key: "parallel_slots", Value: fmt.Sprint(len(slots)), Default: "sized",
			Note: "cases at once, each slot with a ctldb of its own. This run: " + sized},
		{Group: "suite", Key: "parallel", Value: orUnset(cfg.GetOr("parallel", "")), Default: "measured",
			Note: "conservative, measured or aggressive: how an unset parallel_slots is sized from this machine's " +
				"own runs (ADR-020)"},
		{Group: "suite", Key: "executor", Value: "runone.sh", Note: "CTP's own script and ctltool, unchanged (ADR-007)"},
		{Group: "suite", Key: "controller", Value: controllerName(), Default: "qactl",
			Note: "what runone.sh runs a case with. testkit's own drops qactl's two fixed sleeps " +
				"and keeps qacsql (ADR-019); TESTKIT_ISOLATION_CTL=1 asks for it"},
		{Group: "suite", Key: "testcase_retry_num", Value: cfg.GetOr("testcase_retry_num", "0"), Default: "0",
			Note: "attempts runone.sh makes after the first"},
		{Group: "suite", Key: "testcase_timeout_in_secs", Value: orUnset(cfg.GetOr("testcase_timeout_in_secs", "")),
			Note: "timeout3.sh's bound on qactl, per attempt"},
		{Group: "suite", Key: "scenario_disk", Value: yesNo(onDisk), Default: "no",
			Note: "the cases tree behind a layer per slot; without it runone.sh writes into it"},
		{Group: "engine", Key: "build", Value: build},
		{Group: "environment", Key: contain.SlotRootEnv, Value: orUnset(os.Getenv(contain.SlotRootEnv)),
			Default: "/var/tmp/testkit-slots", Note: "where a slot's writes land"},
		{Group: "environment", Key: contain.SlotVolatileEnv, Value: yesNo(contain.Volatile()), Default: "no",
			Note: "a sync on a slot's layer returns having done nothing"},
	})
	return board, stop
}

// liveResult is the file runone.sh is writing while qactl runs: the raw result,
// beside the answers, which the page shows as it grows.
func liveResult(tc string) string {
	path := strings.TrimSpace(tc)
	if !filepath.IsAbs(path) {
		path = filepath.Join(os.Getenv("HOME"), path)
	}
	dir, file := filepath.Split(path)
	return filepath.Join(dir, "result", strings.ReplaceAll(file, ".ctl", "")+".result")
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func orUnset(s string) string {
	if strings.TrimSpace(s) == "" {
		return "unset"
	}
	return s
}

// controllerName is what the board shows for the executor's controller.
func controllerName() string {
	if wantOwnController() {
		return "testkit"
	}
	return "qactl"
}
