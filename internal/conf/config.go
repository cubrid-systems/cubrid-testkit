package conf

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Config is one loaded configuration file.
//
// CTP's files are java.util.Properties: flat key=value, no sections, '#' for
// comments. None of the thirteen shipped files uses a line continuation, a colon
// separator, a unicode escape or a '!' comment -- but the format allows all four,
// so the parser accepts them. Accepting the format is the contract; the fact that
// nobody currently uses a corner of it is not a reason to mis-read it later.
//
// Two substitutions are applied to every value, and only two: ${HOME} and
// ${CTP_HOME}. That is exactly what IniData.translateValue did.
type Config struct {
	path     string
	values   map[string]string
	keyOrder []string
}

// Path is where this configuration was read from.
func (c *Config) Path() string { return c.path }

// Load reads a configuration file. Substitution needs the CTP home, so it is
// passed in rather than looked up again.
func (h *Home) Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("configuration %s: %w", path, err)
	}
	defer f.Close()

	cfg, err := parse(f, h.Path)
	if err != nil {
		return nil, fmt.Errorf("configuration %s: %w", path, err)
	}
	cfg.path = path
	return cfg, nil
}

// ParseText reads a configuration that did not come from a file.
//
// One caller: a topology described by an external provisioner arrives as the
// text a subprocess printed (internal/sandbox), and writing it to a file to read
// it back would put a temporary path in Path() where a reader expects the name
// of the thing that produced it. name is what Path() reports and is used in
// errors; it is never opened.
//
// Substitution is the same as a file's, so ${HOME} and ${CTP_HOME} mean what
// they mean everywhere else.
func ParseText(name, text, ctpHome string) (*Config, error) {
	cfg, err := parse(strings.NewReader(text), ctpHome)
	if err != nil {
		return nil, fmt.Errorf("configuration %s: %w", name, err)
	}
	cfg.path = name
	return cfg, nil
}

func parse(r io.Reader, ctpHome string) (*Config, error) {
	cfg := &Config{values: map[string]string{}}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var pending string
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := sc.Text()

		if pending != "" {
			line = pending + strings.TrimLeft(line, " \t")
			pending = ""
		} else {
			line = strings.TrimLeft(line, " \t\f")
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
				continue
			}
		}

		// A trailing backslash continues the entry on the next line, unless the
		// backslash is itself escaped.
		if trailingBackslashes(line)%2 == 1 {
			pending = strings.TrimSuffix(line, `\`)
			continue
		}

		key, raw, ok := splitEntry(line)
		if !ok {
			return nil, fmt.Errorf("line %d: no separator in %q", lineNo, line)
		}
		value, err := unescape(raw)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		cfg.set(key, substitute(value, ctpHome))
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if pending != "" {
		return nil, fmt.Errorf("file ends with a line continuation")
	}
	return cfg, nil
}

func trailingBackslashes(s string) int {
	n := 0
	for i := len(s) - 1; i >= 0 && s[i] == '\\'; i-- {
		n++
	}
	return n
}

// splitEntry finds the key/value boundary: the first unescaped '=', ':' or run of
// whitespace, whichever comes first.
func splitEntry(line string) (key, value string, ok bool) {
	escaped := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if escaped {
			escaped = false
			continue
		}
		switch c {
		case '\\':
			escaped = true
		case '=', ':':
			return trimKey(line[:i]), strings.TrimLeft(line[i+1:], " \t"), true
		case ' ', '\t', '\f':
			rest := strings.TrimLeft(line[i:], " \t\f")
			if rest != "" && (rest[0] == '=' || rest[0] == ':') {
				return trimKey(line[:i]), strings.TrimLeft(rest[1:], " \t"), true
			}
			return trimKey(line[:i]), rest, true
		}
	}
	// A key on its own is a key with an empty value, which Properties allows.
	return trimKey(line), "", true
}

func trimKey(s string) string {
	k, _ := unescape(strings.TrimRight(s, " \t\f"))
	return k
}

func unescape(s string) (string, error) {
	if !strings.Contains(s, `\`) {
		return s, nil
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			continue
		}
		i++
		if i >= len(s) {
			break // a trailing backslash is dropped, as Properties does
		}
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'f':
			b.WriteByte('\f')
		case 'u':
			if i+4 >= len(s) {
				return "", fmt.Errorf("truncated unicode escape")
			}
			n, err := strconv.ParseUint(s[i+1:i+5], 16, 32)
			if err != nil {
				return "", fmt.Errorf("bad unicode escape %q", s[i+1:i+5])
			}
			b.WriteRune(rune(n))
			i += 4
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String(), nil
}

// substitute applies the only two variables CTP ever expanded.
func substitute(value, ctpHome string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		value = strings.ReplaceAll(value, "${HOME}", home)
	}
	if ctpHome != "" {
		value = strings.ReplaceAll(value, "${CTP_HOME}", ctpHome)
	}
	return value
}

func (c *Config) set(key, value string) {
	if _, seen := c.values[key]; !seen {
		c.keyOrder = append(c.keyOrder, key)
	}
	c.values[key] = value
}

// Override applies values that beat the file.
//
// CTP read its configuration with getPropertiesWithPriority, which is
// getProperties followed by putAll(System.getProperties()) -- so a JVM system
// property silently won over the file. That is how `ctp.sh rqg` becomes a shell
// run with TEST_CATEGORY=rqg. There is no system property table here, so the
// mechanism is explicit instead: whoever wants to win says so.
func (c *Config) Override(kv map[string]string) {
	keys := make([]string, 0, len(kv))
	for k := range kv {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		c.set(k, kv[k])
	}
}

// Get returns a value and whether it was present.
func (c *Config) Get(key string) (string, bool) {
	v, ok := c.values[key]
	return v, ok
}

// GetOr returns a value, falling back only when the key is absent.
//
// An empty value is a value. Properties.getProperty(key, default) returns what
// was stored even when it is the empty string, and CTP's Context.getProperty is a
// direct call to it -- so `default.ssh.pwd=` means an empty password, not "use the
// default password". None of the thirteen shipped files currently has an empty
// value, but treating one as absent would be a silent behaviour change the day
// somebody writes one.
func (c *Config) GetOr(key, fallback string) string {
	if v, ok := c.values[key]; ok {
		return v
	}
	return fallback
}

// Bool reads a yes/no flag. CTP wrote these as yes/no and as true/false, and read
// both through convertBoolean.
func (c *Config) Bool(key string, fallback bool) bool {
	v, ok := c.values[key]
	if !ok {
		return fallback
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "yes", "y", "true", "1", "on":
		return true
	case "no", "n", "false", "0", "off":
		return false
	default:
		return fallback
	}
}

// Int reads a numeric setting.
func (c *Config) Int(key string, fallback int) int {
	v, ok := c.values[key]
	if !ok {
		return fallback
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return fallback
	}
	return n
}

// Keys lists every key in the order it first appeared.
func (c *Config) Keys() []string {
	out := make([]string, len(c.keyOrder))
	copy(out, c.keyOrder)
	return out
}

// Prefixed returns every key under a dot-prefix, with the prefix stripped.
//
// This is the dot-notation matching CTP did in parsePropertiesByPrefix, and it is
// frozen: `default.cubrid.async_commit=on` has to end up as `async_commit=on` in
// the remote cubrid.conf. The property name is not from any list -- whatever
// follows the role is passed through, which is why the freeze spec writes it as
// <property> rather than enumerating it.
func (c *Config) Prefixed(prefix string) map[string]string {
	if !strings.HasSuffix(prefix, ".") {
		prefix += "."
	}
	out := map[string]string{}
	for _, k := range c.keyOrder {
		if strings.HasPrefix(k, prefix) {
			out[strings.TrimPrefix(k, prefix)] = c.values[k]
		}
	}
	return out
}
