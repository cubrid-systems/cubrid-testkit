package sqlsuite

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cubrid-systems/cubrid-testkit/internal/conf"
)

func iniFile(t *testing.T, body string) (*conf.Home, *conf.Ini, string) {
	t.Helper()
	home := &conf.Home{Path: "/opt/ctp"}
	path := filepath.Join(t.TempDir(), "sql.conf")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	ini, err := home.LoadIni(path)
	if err != nil {
		t.Fatal(err)
	}
	return home, ini, path
}

var engine115 = engine{ver: "11.5.0.2562-1642235", major: 11, minor: 5, patch: "0", build: "2562-1642235", prefix: "11.5.0", bits: "64"}

// What run.sh decides before it runs anything, decided the same way.
func TestSettingsAreRunShsDecisions(t *testing.T) {
	corpus := t.TempDir()
	home, ini, path := iniFile(t, "[sql]\nscenario="+corpus+"\ntestcase_exclude_from_file=${CTP_HOME}/conf/exclusions.txt\ndb_charset=en_US\n"+
		"[sql/cubrid_ha.conf]\nha_mode=yes\n")

	s, err := newSettings(home, path, "sql", ini, engine115)
	if err != nil {
		t.Fatal(err)
	}
	if s.dbName != "basic" || s.alias != "sql" || s.needLocale != "yes" || s.jdbcConfig != "test_default.xml" {
		t.Errorf("defaults: %+v", s)
	}
	// CUBRIDQA-1287: from 11.5 the sql database is utf8 whatever the file says.
	if s.dbCharset != "en_US.utf8" {
		t.Errorf("sql on 11.5 has charset %q, want en_US.utf8", s.dbCharset)
	}
	// do_test adds the slash, and CQT measures every recorded path from it.
	if s.scenario != corpus+"/" {
		t.Errorf("scenario is %q, want the trailing slash", s.scenario)
	}
	if want := corpus + "/?db=basic_qa&filter=/opt/ctp/conf/exclusions.txt"; s.cqtArg() != want {
		t.Errorf("CQT's argument is %q, want %q", s.cqtArg(), want)
	}
	if s.confHAMode != "yes" {
		t.Errorf("ha_mode from [sql/cubrid_ha.conf] is %q", s.confHAMode)
	}

	m, err := newSettings(home, path, "medium", ini, engine115)
	if err != nil {
		t.Fatal(err)
	}
	// medium keeps the file's charset, and its own database.
	if m.dbName != "mdb" || m.dbCharset != "en_US" || m.alias != "medium" {
		t.Errorf("medium: %+v", m)
	}

	old := engine115
	old.minor = 4
	o, _ := newSettings(home, path, "sql", ini, old)
	if o.dbCharset != "en_US" {
		t.Errorf("sql before 11.5 keeps the file's charset, got %q", o.dbCharset)
	}
}

func TestSettingsWithoutACorpusStopTheRun(t *testing.T) {
	home, ini, path := iniFile(t, "[sql]\nscenario=/nowhere/at/all\n")
	if _, err := newSettings(home, path, "sql", ini, engine115); err == nil ||
		!strings.Contains(err.Error(), "please make sure your scenario directory") {
		t.Errorf("got %v, want run.sh's message", err)
	}
}

// The header is shell: every value survives quoting, including one that
// would otherwise split or expand.
func TestTheHeaderQuotesWhatItSets(t *testing.T) {
	s := &settings{ctpHome: "/opt/ctp", createdbOpts: "--db-volume-size=512M -r", dataFile: "/a b/it's.tar.gz"}
	h := s.header(engine115, "/log")
	for _, want := range []string{
		"export CTP_HOME='/opt/ctp'\n",
		"cubrid_createdb_opts='--db-volume-size=512M -r'\n",
		`test_data_file='/a b/it'\''s.tar.gz'` + "\n",
		"cubrid_ver_p1='11'\n",
	} {
		if !strings.Contains(h, want) {
			t.Errorf("the header lacks %q:\n%s", want, h)
		}
	}
}
