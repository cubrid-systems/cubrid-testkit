package conf

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Ini is a configuration file with sections, which is what the sql family's
// files are: sql.conf keeps what CTP reads under [sql] and what it writes into
// the engine's files under [sql/cubrid.conf], [sql/cubrid_broker.conf/%BROKER1]
// and so on. The same key can mean different things in two sections -- ha_mode
// is in both [sql/cubrid.conf] and [sql/cubrid_ha.conf] -- so Load's flat
// reading cannot hold one.
//
// CTP read these through ini4j (IniData, and IniCommand behind bin/ini.sh). What
// is reproduced is what the shipped files use: [section] headers, key=value,
// whole-line comments, surrounding whitespace trimmed, and ${HOME} and
// ${CTP_HOME} substituted on the way out as IniData.translateValue did. Writing
// the engine's files is not done here: that stays with ini.sh, whose output
// order is a Java HashMap's.
type Ini struct {
	path     string
	sections map[string]map[string]string
}

// LoadIni reads a configuration file with sections.
func (h *Home) LoadIni(path string) (*Ini, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("configuration %s: %w", path, err)
	}
	defer f.Close()

	ini := &Ini{path: path, sections: map[string]map[string]string{}}
	section := ""
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for lineNo := 1; sc.Scan(); lineNo++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		if line[0] == '[' {
			end := strings.IndexByte(line, ']')
			if end < 0 {
				return nil, fmt.Errorf("configuration %s: line %d: unterminated section %q", path, lineNo, line)
			}
			section = strings.TrimSpace(line[1:end])
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			// ini4j takes a line without a separator as a key with no value.
			key, value = line, ""
		}
		key = strings.TrimSpace(key)
		if ini.sections[section] == nil {
			ini.sections[section] = map[string]string{}
		}
		ini.sections[section][key] = substitute(strings.TrimSpace(value), h.Path)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("configuration %s: %w", path, err)
	}
	return ini, nil
}

// Path is where the file was read from.
func (i *Ini) Path() string { return i.path }

// Get returns a value and whether the section has the key.
func (i *Ini) Get(section, key string) (string, bool) {
	v, ok := i.sections[section][key]
	return v, ok
}

// GetOr returns a value, or fallback when the key is absent. An empty value is
// a value, as it is for Config.GetOr.
func (i *Ini) GetOr(section, key, fallback string) string {
	if v, ok := i.Get(section, key); ok {
		return v
	}
	return fallback
}

// Int reads a numeric setting, or fallback when it is absent or not a number.
func (i *Ini) Int(section, key string, fallback int) int {
	v, ok := i.Get(section, key)
	if !ok {
		return fallback
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return fallback
	}
	return n
}
