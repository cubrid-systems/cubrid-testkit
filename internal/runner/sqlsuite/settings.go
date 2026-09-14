package sqlsuite

import (
	"fmt"
	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
	"os"
	"path/filepath"
	"strings"

	"github.com/cubrid-systems/cubrid-testkit/internal/conf"
)

// settings is every decision run.sh takes from the configuration before it
// runs anything: do_init, and the charset half of do_configure. The names in
// the comments are run.sh's variables, which stages.sh still uses.
type settings struct {
	ctpHome  string
	confPath string // config_file_main
	// category is the task, sql or medium: run.sh's -s, scenario_category.
	category string
	// alias is test_category, or the category when that is unset:
	// scenario_alias, which names the result and CQT's typeAlias.
	alias  string
	dbName string // basic, or mdb for medium
	// scenario is the corpus root with the trailing slash do_test adds for
	// CQT. CQT measures every path it records from this string's length, so
	// the slash is part of the output, not a cosmetic.
	scenario     string
	excludeFile  string // testcase_exclude_file
	createdbOpts string // cubrid_createdb_opts
	needLocale   string // need_make_locale, "yes" when unset
	dataFile     string // test_data_file
	jdbcConfig   string // jdbc_config_file_ext
	dbCharset    string // db_charset, after do_configure's rules
	confHAMode   string // [sql/cubrid_ha.conf] ha_mode, for ha_mode_of
}

// newSettings reads the configuration the way run.sh does. An empty value is
// no value: every one of these is tested with [ -z ] there.
func newSettings(home *conf.Home, confPath, task string, ini *conf.Ini, e engine) (*settings, error) {
	get := func(key string) string { return strings.TrimSpace(ini.GetOr("sql", key, "")) }
	s := &settings{
		ctpHome:      home.Path,
		confPath:     confPath,
		category:     task,
		alias:        get("test_category"),
		dbName:       "basic",
		excludeFile:  get("testcase_exclude_from_file"),
		createdbOpts: get("cubrid_createdb_opts"),
		needLocale:   get("need_make_locale"),
		dataFile:     get("data_file"),
		jdbcConfig:   get("jdbc_config_file"),
		dbCharset:    get("db_charset"),
		confHAMode:   strings.TrimSpace(ini.GetOr("sql/cubrid_ha.conf", "ha_mode", "")),
	}
	if s.alias == "" {
		s.alias = task
	}
	if task == "medium" {
		s.dbName = "mdb"
	}
	if s.needLocale == "" {
		s.needLocale = "yes"
	}
	if s.jdbcConfig == "" {
		s.jdbcConfig = "test_default.xml"
	}

	scenario := get("scenario")
	st, err := os.Stat(scenario)
	if scenario == "" || err != nil {
		return nil, fmt.Errorf("please make sure your scenario directory")
	}
	if st.IsDir() && !strings.HasSuffix(scenario, "/") {
		scenario += "/"
	}
	s.scenario = scenario

	// do_configure: iso88591 unless the file says otherwise -- and from 11.5 the
	// sql database is utf8 whatever the file says (CUBRIDQA-1287).
	if s.dbCharset == "" {
		s.dbCharset = "en_US.iso88591"
	}
	if task == "sql" && e.atLeast(11, 5) {
		s.dbCharset = "en_US.utf8"
	}
	return s, nil
}

// cqtArg is the one file argument run.sh hands CQT: the corpus, the database
// alias, and the exclusion file when there is one.
func (s *settings) cqtArg() string {
	arg := s.scenario + "?db=" + s.dbName + "_qa"
	if s.excludeFile != "" {
		arg += "&filter=" + s.excludeFile
	}
	return arg
}

// header is stages.sh's globals, in front of every stage.
func (s *settings) header(e engine, logFile string) string {
	vars := [][2]string{
		{"CTP_HOME", s.ctpHome},
		{"config_file_main", s.confPath},
		{"scenario_category", s.category},
		{"scenario_full_name", s.category},
		{"db_name", s.dbName},
		{"cubrid_bits", e.bits},
		{"cubrid_ver", e.ver},
		{"cubrid_ver_p1", fmt.Sprint(e.major)},
		{"cubrid_ver_p2", fmt.Sprint(e.minor)},
		{"cubrid_ver_p3", e.patch},
		{"cubrid_ver_p4", e.build},
		{"cubrid_ver_prefix", e.prefix},
		{"os_type", "Linux"},
		{"db_charset", s.dbCharset},
		{"cubrid_createdb_opts", s.createdbOpts},
		{"need_make_locale", s.needLocale},
		{"test_data_file", s.dataFile},
		{"log_filename", logFile},
		{"cubrid_root_dir", os.Getenv("CUBRID")},
		{"conf_ha_mode", s.confHAMode},
		{"jdbc_config_file_ext", s.jdbcConfig},
	}
	var b strings.Builder
	b.WriteString("export CTP_HOME=" + shQuote(s.ctpHome) + "\n")
	for _, kv := range vars[1:] {
		b.WriteString(kv[0] + "=" + shQuote(kv[1]) + "\n")
	}
	return b.String()
}

// logName is do_init's log file: <log_dir>/<category>_<version>_<epoch>.log.
func (s *settings) logName(e engine, epoch int64) string {
	return filepath.Join(s.ctpHome, "sql", "log", fmt.Sprintf("%s_%s_%d.log", s.category, e.ver, epoch))
}

func shQuote(s string) string { return exec.Quote(s) }
