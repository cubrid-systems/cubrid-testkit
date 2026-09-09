package shellsuite

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cubrid-systems/cubrid-testkit/internal/conf"
	"github.com/cubrid-systems/cubrid-testkit/internal/status"
)

// describeSetup is everything about a run that the verdicts do not say.
//
// The page shows what is happening. What was asked for is just as much a part of
// reading it, and two of the values below change what a result means rather than
// only how fast it arrives: db_volume_size moves a case's footprint by a factor
// of twenty-five, and a server that is not waited for turns a timing assumption
// into a race. Neither is visible in an OK or a NOK.
func describeSetup(cfg *conf.Config, slots, ramMB, slowSecs, slowMB int, planPath, sizePath string) []status.Setting {
	var out []status.Setting

	// The engine, against the configuration it shipped with. cubrid.conf.org is
	// the untouched copy the distribution leaves beside it, so the comparison is
	// with what CUBRID chose rather than with a table kept here and gone stale.
	home := os.Getenv("CUBRID")
	live := readConf(filepath.Join(home, "conf", "cubrid.conf"))
	shipped := readConf(filepath.Join(home, "conf", "cubrid.conf.org"))
	keys := make([]string, 0, len(live))
	for k := range live {
		keys = append(keys, k)
	}
	sortStrings(keys)
	for _, k := range keys {
		out = append(out, status.Setting{
			Group: "engine", Key: k, Value: live[k], Default: shipped[k],
		})
	}
	if len(live) == 0 {
		out = append(out, status.Setting{
			Group: "engine", Key: "cubrid.conf", Value: "not readable",
			Note: "$CUBRID is " + orNone(home),
		})
	}

	// The suite. Only the keys that change what runs or how much of the machine
	// it takes -- the ports and the charset are in the conf file for anyone who
	// wants them.
	suite := []struct{ k, v, note string }{
		{"parallel_slots", itoa(slots), "cases at once"},
		{"scenario_ram_mb", mbOrOff(ramMB), "ceiling for the corpus overlay"},
		{"testcase_timeout_in_secs", cfg.GetOr("testcase_timeout_in_secs", "0"), "per case"},
		{"testcase_retry_num", cfg.GetOr("testcase_retry_num", "0"), ""},
		{"lane_slow_secs", secsOrOff(slowSecs), "duration threshold for the disk lane"},
		{"lane_slow_mb", mbOrOff(slowMB), "footprint threshold for the disk lane"},
		{"case_plan", orOff(planPath), "durations, read and written"},
		{"case_sizes", orOff(sizePath), "footprints, read and written"},
		{"testcase_exclude_from_file", orOff(cfg.GetOr("testcase_exclude_from_file", "")), ""},
		{"testcase_exclude_by_macro", orOff(cfg.GetOr("testcase_exclude_by_macro", "")), ""},
	}
	for _, s := range suite {
		out = append(out, status.Setting{Group: "suite", Key: s.k, Value: s.v, Note: s.note})
	}

	// The environment. These are switches rather than values, and every one of
	// them is a decision someone made about how faithful the run is: whether the
	// shell is CTP's or this one's, whether a server start is waited for, whether
	// a check that would have failed the case is skipped.
	env := []struct{ k, note string }{
		{"TESTKIT_NATIVE_SHELL", "run cases here rather than through CTP"},
		{"TESTKIT_CONTAIN", "namespaces of the run's own"},
		{"TESTKIT_CONTAIN_SH", "shell bound over /bin/sh"},
		{"CTP_SERVER_START_NOWAIT", "do not wait for the server to come up"},
		{"CTP_DB_TEMPLATE_CACHE", "reuse a built database instead of createdb"},
		{"CTP_ERROR_BACKUP", "pack a failing case's databases"},
		{"SKIP_CHECK_RECOVERY_ERROR", "ignore recovery errors in the server log"},
		{"SKIP_CHECK_FATAL_ERROR", "ignore fatal errors in the server log"},
		{"USER", "who the cases believe they are"},
	}
	for _, e := range env {
		out = append(out, status.Setting{
			Group: "environment", Key: e.k, Value: orUnset(os.Getenv(e.k)), Note: e.note,
		})
	}
	return out
}

// readConf reads a CUBRID conf file into key/value, ignoring comments, blank
// lines and section headers. Later wins, which is what the engine does.
func readConf(path string) map[string]string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return out
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }

func orOff(s string) string {
	if strings.TrimSpace(s) == "" {
		return "off"
	}
	return s
}

func orNone(s string) string {
	if s == "" {
		return "not set"
	}
	return s
}

func orUnset(s string) string {
	if s == "" {
		return "unset"
	}
	return s
}

func mbOrOff(n int) string {
	if n <= 0 {
		return "off"
	}
	return fmt.Sprintf("%d MB", n)
}

func secsOrOff(n int) string {
	if n <= 0 {
		return "off"
	}
	return fmt.Sprintf("%ds", n)
}
