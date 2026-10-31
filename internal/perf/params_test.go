package perf

import "testing"

// paramdump's two formats: develop marks each line with its section and
// whether it differs from the default and prints the default; 11.4.6 prints
// name=value. Both put the client's section first, then the server's.
func TestParseParamdumpReadsBothFormats(t *testing.T) {
	develop := `System parameters to be used by the client program.
#
# cubrid.conf
#

# system parameters were loaded from the files ([@section])
# /opt/cubrid/conf/cubrid.conf

# system parameters
[C ] er_log_debug=n (n)
[C*] parallelism=4 (4)

System parameters loaded by the server 'perf_narrow_1m'
#
# cubrid.conf
#

# system parameters
[S*] data_buffer_size=4G (32768K)
[S ] parallelism=4 (4)
[S ] intl_date_lang="" ("")
`
	got := parseParamdump(develop)
	want := map[string]string{
		"client/er_log_debug":     "n",
		"client/parallelism":      "4",
		"server/data_buffer_size": "4G",
		"server/parallelism":      "4",
		"server/intl_date_lang":   `""`,
	}
	if len(got) != len(want) {
		t.Fatalf("develop: %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("develop %s: %q, want %q", k, got[k], v)
		}
	}
	old := "System parameters to be used by the client program.\n# system parameters\ner_log_debug=n\n\nSystem parameters loaded by the server 'perf_narrow_1m'\n# system parameters\ndata_buffer_size=4G\n"
	got = parseParamdump(old)
	if len(got) != 2 || got["client/er_log_debug"] != "n" || got["server/data_buffer_size"] != "4G" {
		t.Errorf("11.4.6: %v", got)
	}
	if got := parseParamdump("ERROR: database perf_x is unknown\n"); len(got) != 0 {
		t.Errorf("an error message: %v", got)
	}
}

// The diff keeps what differs or is on one side only, and says in which conf
// groups only when it was not seen in all of them.
func TestParamDiffsAcrossConfGroups(t *testing.T) {
	target := map[string]string{"server/data_buffer_size": "2G", "server/parallelism": "4", "server/log_buffer_size": "16M"}
	reference := map[string]string{"server/data_buffer_size": "2G", "server/log_buffer_size": "16M"}
	var d paramDiffs
	d.add("data_buffer_size=2G", target, reference)
	d.add("data_buffer_size=4G", map[string]string{"server/parallelism": "4", "server/checkpoint_interval": "6min"},
		map[string]string{"server/checkpoint_interval": "1440min"})
	d.add("no params", target, nil)
	got := d.list()
	if len(got) != 2 {
		t.Fatalf("entries: %+v", got)
	}
	p := got[0]
	if p.Scope != "server" || p.Name != "parallelism" || p.Target == nil || *p.Target != "4" || p.Reference != nil || p.Conf != nil {
		t.Errorf("parallelism, in both groups compared: %+v", p)
	}
	c := got[1]
	if c.Name != "checkpoint_interval" || *c.Target != "6min" || *c.Reference != "1440min" || len(c.Conf) != 1 || c.Conf[0] != "data_buffer_size=4G" {
		t.Errorf("checkpoint_interval, in one group: %+v", c)
	}
}
