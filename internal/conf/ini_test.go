package conf

import (
	"os"
	"path/filepath"
	"testing"
)

// The same key in two sections is two settings. sql.conf has ha_mode under
// [sql/cubrid.conf] and under [sql/cubrid_ha.conf], and Load's flat reading
// would keep only the second.
func TestIniKeepsSectionsApart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sql.conf")
	body := `# a comment
[sql]
scenario = ${CTP_HOME}/../cubrid-testcases/sql
test_category=sql
  db_charset=en_US
cubrid_createdb_opts=
; ini4j's other comment

[sql/cubrid.conf]
ha_mode=yes
cubrid_port_id=1822

[sql/cubrid_ha.conf]
ha_mode=no
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	ini, err := (&Home{Path: "/opt/ctp"}).LoadIni(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ section, key, want string }{
		{"sql", "scenario", "/opt/ctp/../cubrid-testcases/sql"},
		{"sql", "test_category", "sql"},
		{"sql", "db_charset", "en_US"},
		{"sql/cubrid.conf", "ha_mode", "yes"},
		{"sql/cubrid_ha.conf", "ha_mode", "no"},
	} {
		if got, _ := ini.Get(c.section, c.key); got != c.want {
			t.Errorf("[%s] %s = %q, want %q", c.section, c.key, got, c.want)
		}
	}
	if v, ok := ini.Get("sql", "cubrid_createdb_opts"); !ok || v != "" {
		t.Errorf("an empty value should be present and empty, got %q, %v", v, ok)
	}
	if _, ok := ini.Get("sql", "ha_mode"); ok {
		t.Error("a key from another section leaked into [sql]")
	}
	if got := ini.Int("sql/cubrid.conf", "cubrid_port_id", 0); got != 1822 {
		t.Errorf("cubrid_port_id read as %d", got)
	}
	if got := ini.Int("sql", "test_category", 7); got != 7 {
		t.Errorf("a value that is not a number should take the fallback, got %d", got)
	}
	if got := ini.GetOr("sql", "need_make_locale", "yes"); got != "yes" {
		t.Errorf("an absent key should take the fallback, got %q", got)
	}
}
