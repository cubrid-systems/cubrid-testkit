// An example page, so the configuration panel can be looked at while a run that
// predates it is still going. Real values: this machine's cubrid.conf against
// the copy the distribution shipped, and the conf the live run was given.
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cubrid-systems/cubrid-testkit/internal/plan"
	"github.com/cubrid-systems/cubrid-testkit/internal/status"
)

func readConf(path string) map[string]string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l == "" || strings.HasPrefix(l, "#") || strings.HasPrefix(l, "[") {
			continue
		}
		if k, v, ok := strings.Cut(l, "="); ok {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return out
}

func main() {
	home := os.Getenv("CUBRID")
	live := readConf(filepath.Join(home, "conf", "cubrid.conf"))
	ship := readConf(filepath.Join(home, "conf", "cubrid.conf.org"))
	suite := readConf("/data/cub_sys/devrun/full.conf")

	var rows []status.Setting
	keys := []string{}
	for k := range live {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		rows = append(rows, status.Setting{Group: "engine", Key: k, Value: live[k], Default: ship[k]})
	}
	for _, k := range []string{"parallel_slots", "scenario_ram_mb", "testcase_timeout_in_secs",
		"testcase_retry_num", "case_plan", "case_sizes", "testcase_exclude_from_file", "testcase_exclude_by_macro"} {
		v := suite[k]
		if v == "" {
			v = "off"
		}
		rows = append(rows, status.Setting{Group: "suite", Key: k, Value: v})
	}
	for _, e := range [][2]string{
		{"TESTKIT_NATIVE_SHELL", "run cases here rather than through CTP"},
		{"TESTKIT_CONTAIN", "namespaces of the run's own"},
		{"TESTKIT_CONTAIN_SH", "shell bound over /bin/sh"},
		{"TESTKIT_NO_LINK_SHIM", "leave the machine linker as it is"},
		{"CTP_SERVER_START_NOWAIT", "do not wait for the server to come up"},
		{"CTP_DB_TEMPLATE_CACHE", "reuse a built database instead of createdb"},
		{"CTP_ERROR_BACKUP", "pack a failing case's databases"},
		{"SKIP_CHECK_RECOVERY_ERROR", "ignore recovery errors in the server log"},
		{"SKIP_CHECK_FATAL_ERROR", "ignore fatal errors in the server log"},
		{"USER", "who the cases believe they are"},
	} {
		v := os.Getenv(e[0])
		if v == "" {
			v = "unset"
		}
		rows = append(rows, status.Setting{Group: "environment", Key: e[0], Value: v, Note: e[1]})
	}

	known, _ := plan.Read("/data/cub_sys/devrun/plan.full")
	cases := make([]string, 0, len(known))
	for c := range known {
		cases = append(cases, c)
	}
	sort.Slice(cases, func(i, j int) bool { return known[cases[i]] > known[cases[j]] })

	b := status.New(len(cases))
	b.Setup(rows)
	b.Expect(cases, known, 8)
	where, stop, err := b.Serve("0.0.0.0:51533")
	if err != nil {
		fmt.Println("serve:", err)
		return
	}
	defer stop()
	fmt.Println("example page on", where)

	// Enough traffic that the page is not an empty shell.
	for i, c := range cases {
		if i >= 240 {
			break
		}
		slot := fmt.Sprintf("slot%d", i%8)
		b.Begin(slot, c)
		b.End(slot, c, i%13 != 0)
	}
	for i := 240; i < 248 && i < len(cases); i++ {
		b.Begin(fmt.Sprintf("slot%d", i-240), cases[i])
	}
	select {}
}
