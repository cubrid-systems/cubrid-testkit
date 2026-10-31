package perf

import (
	"strings"
	"testing"
)

// paramdump -b as the two lines print it: develop splits the parameters
// between the client's section and the server's and marks each line;
// 11.4 prints every parameter in both sections as name=value.
const (
	developDump = `System parameters to be used by the client program.
#
# cubrid.conf
#

# system parameters were loaded from the files ([@section])
# /work/pf-x-t-n1/cubrid/conf/cubrid.conf [@perf_narrow_1m]

# system parameters
[C ] ansi_quotes=y (y)
[C*] optimization_level=1 (2)
[C ] intl_collation="utf8_bin" ("")

System parameters loaded by the server 'perf_narrow_1m'
#
# cubrid.conf
#

# system parameters
[S*] data_buffer_size=4G (32768K)
[S*] error_log="server/perf_narrow_1m_20261031_0442.err" ("")
[S ] parallelism=4 (4)
[S ] intl_date_lang="" ("")
[S ] some_list=a (b) (a (b))
`
	release114Dump = `System parameters to be used by the client program.
#
# cubrid.conf
#

# system parameters
ansi_quotes=y
data_buffer_size=4G
error_log=""
intl_collation="utf8_bin"
optimization_level=1
removed_in_develop=7

System parameters loaded by the server 'perf_narrow_1m'
#
# cubrid.conf
#

# system parameters
ansi_quotes=y
data_buffer_size=4G
error_log="server/perf_narrow_1m_20261031_0443.err"
intl_collation=""
optimization_level=1
removed_in_develop=7
`
)

func TestParseParamdumpKeysByNameAcrossSections(t *testing.T) {
	client := func(v string) paramValue { return paramValue{client: v, inClient: true} }
	server := func(v string) paramValue { return paramValue{server: v, inServer: true} }
	got := parseParamdump(developDump)
	want := map[string]paramValue{
		"ansi_quotes":        client("y"),
		"optimization_level": client("1"),
		"data_buffer_size":   server("4G"),
		"parallelism":        server("4"),
		"intl_date_lang":     server(`""`),
		"some_list":          server("a (b)"),
		"intl_collation":     client(`"utf8_bin"`),
		"error_log":          server(`"server/perf_narrow_1m_20261031_0442.err"`),
	}
	if len(got) != len(want) {
		t.Fatalf("develop: %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("develop %s: %+v, want %+v", k, got[k], v)
		}
	}
	// 11.4: both sections list everything.
	got = parseParamdump(release114Dump)
	both := paramValue{server: "4G", client: "4G", inServer: true, inClient: true}
	if len(got) != 6 || got["data_buffer_size"] != both || !got["ansi_quotes"].inClient || !got["ansi_quotes"].inServer {
		t.Errorf("11.4: %v", got)
	}
	if got := parseParamdump("ERROR: database perf_x is unknown\n"); len(got) != 0 {
		t.Errorf("an error message: %v", got)
	}
}

// develop against 11.4: a parameter in different sections of the two dumps
// is in both builds; only what no section of one build has is one-sided.
func TestParamDiffsAcrossBuildsAndConfGroups(t *testing.T) {
	dev, rel := parseParamdump(developDump), parseParamdump(release114Dump)
	var d paramDiffs
	if !d.add("data_buffer_size=4G", dev, rel) {
		t.Fatal("not compared")
	}
	tweaked := map[string]paramValue{}
	for k, v := range rel {
		tweaked[k] = v
	}
	tweaked["optimization_level"] = paramValue{server: "1", client: "2", inServer: true, inClient: true}
	if !d.add("", dev, tweaked) {
		t.Fatal("not compared")
	}
	if d.add("other", dev, nil) {
		t.Error("a side without parameters: compared")
	}
	byName := map[string][]ParamDiff{}
	for _, e := range d.list() {
		byName[e.Name] = append(byName[e.Name], e)
	}
	// intl_collation: the client's value on both, though 11.4's server
	// section prints it empty; error_log: set from the start time.
	for _, n := range []string{"ansi_quotes", "data_buffer_size", "intl_collation", "error_log"} {
		if len(byName[n]) != 0 {
			t.Errorf("%s: %+v", n, byName[n])
		}
	}
	if e := byName["parallelism"]; len(e) != 1 || e[0].Scope != "server" || *e[0].Target != "4" || e[0].Reference != nil || e[0].Conf != nil {
		t.Errorf("parallelism, develop only, every group: %+v", e)
	}
	if e := byName["removed_in_develop"]; len(e) != 1 || e[0].Target != nil || *e[0].Reference != "7" {
		t.Errorf("removed_in_develop, 11.4 only: %+v", e)
	}
	if e := byName["optimization_level"]; len(e) != 1 || *e[0].Target != "1" || *e[0].Reference != "2" || strings.Join(e[0].Conf, ",") != "(no conf)" {
		t.Errorf("optimization_level, apart in one group: %+v", e)
	}
}
