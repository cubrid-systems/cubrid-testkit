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

System parameters loaded by the server 'perf_narrow_1m'
#
# cubrid.conf
#

# system parameters
[S*] data_buffer_size=4G (32768K)
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
optimization_level=1
removed_in_develop=7

System parameters loaded by the server 'perf_narrow_1m'
#
# cubrid.conf
#

# system parameters
ansi_quotes=y
data_buffer_size=4G
optimization_level=1
removed_in_develop=7
`
)

func TestParseParamdumpKeysByNameAcrossSections(t *testing.T) {
	got := parseParamdump(developDump)
	want := map[string]paramValue{
		"ansi_quotes":        {"client", "y"},
		"optimization_level": {"client", "1"},
		"data_buffer_size":   {"server", "4G"},
		"parallelism":        {"server", "4"},
		"intl_date_lang":     {"server", `""`},
		"some_list":          {"server", "a (b)"},
	}
	if len(got) != len(want) {
		t.Fatalf("develop: %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("develop %s: %+v, want %+v", k, got[k], v)
		}
	}
	// 11.4: both sections list everything; the server's value is kept.
	got = parseParamdump(release114Dump)
	if len(got) != 4 || got["data_buffer_size"] != (paramValue{"server", "4G"}) || got["ansi_quotes"] != (paramValue{"server", "y"}) {
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
	tweaked["optimization_level"] = paramValue{"server", "2"}
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
	for _, n := range []string{"ansi_quotes", "data_buffer_size"} {
		if len(byName[n]) != 0 {
			t.Errorf("%s, in both builds with the same value: %+v", n, byName[n])
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
