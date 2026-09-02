package conf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func load(t *testing.T, body string) *Config {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "x.conf")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &Home{Path: "/opt/ctp"}
	cfg, err := h.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return cfg
}

func TestOnlyTwoVariablesAreSubstituted(t *testing.T) {
	// IniData.translateValue expanded ${HOME} and ${CTP_HOME}, and nothing else.
	// Anything that looks like a variable but is not one of those two survives
	// untouched, because the old code left it alone.
	home, _ := os.UserHomeDir()
	cfg := load(t, strings.Join([]string{
		"scenario=${HOME}/cubrid-testcases/shell",
		"testcase_exclude_from_file=${CTP_HOME}/conf/exclusions.txt",
		"untouched=${PATH}/keep",
	}, "\n"))

	if got, want := cfg.GetOr("scenario", ""), home+"/cubrid-testcases/shell"; got != want {
		t.Errorf("HOME: got %q want %q", got, want)
	}
	if got, want := cfg.GetOr("testcase_exclude_from_file", ""), "/opt/ctp/conf/exclusions.txt"; got != want {
		t.Errorf("CTP_HOME: got %q want %q", got, want)
	}
	if got, want := cfg.GetOr("untouched", ""), "${PATH}/keep"; got != want {
		t.Errorf("other variables must survive: got %q want %q", got, want)
	}
}

func TestCommentsAndBlankLines(t *testing.T) {
	cfg := load(t, "# a comment\n! also a comment\n\n   \nkey=value\n")
	if got := cfg.GetOr("key", ""); got != "value" {
		t.Errorf("got %q", got)
	}
	if len(cfg.Keys()) != 1 {
		t.Errorf("keys: %v", cfg.Keys())
	}
}

func TestSeparatorForms(t *testing.T) {
	// Properties accepts '=', ':' and bare whitespace. None of the shipped files
	// uses the latter two, but the format does, so the parser must.
	cfg := load(t, "a=1\nb : 2\nc 3\nd\n")
	for k, want := range map[string]string{"a": "1", "b": "2", "c": "3", "d": ""} {
		if got := cfg.GetOr(k, "<missing>"); got != want {
			t.Errorf("%s: got %q want %q", k, got, want)
		}
	}
}

func TestLineContinuation(t *testing.T) {
	cfg := load(t, "long=one\\\n    two\n")
	if got, want := cfg.GetOr("long", ""), "onetwo"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestBooleanReadsBothSpellings(t *testing.T) {
	// CTP wrote these as yes/no in the config files and as true/false in code.
	cfg := load(t, "a=yes\nb=no\nc=true\nd=false\ne=garbage\n")
	for k, want := range map[string]bool{"a": true, "b": false, "c": true, "d": false} {
		if got := cfg.Bool(k, !want); got != want {
			t.Errorf("%s: got %v want %v", k, got, want)
		}
	}
	if got := cfg.Bool("e", true); !got {
		t.Error("an unreadable value must fall back, not become false")
	}
	if got := cfg.Bool("missing", true); !got {
		t.Error("a missing key must fall back")
	}
}

func TestOverrideBeatsTheFile(t *testing.T) {
	// getPropertiesWithPriority put System.getProperties() over the file, which is
	// how `ctp.sh rqg` turned into a shell run with TEST_CATEGORY=rqg.
	cfg := load(t, "test_category=shell\n")
	cfg.Override(map[string]string{"test_category": "rqg"})
	if got := cfg.GetOr("test_category", ""); got != "rqg" {
		t.Errorf("override: got %q want rqg", got)
	}
}

func TestPrefixedStripsAndKeepsArbitraryProperties(t *testing.T) {
	cfg := load(t, strings.Join([]string{
		"default.cubrid.async_commit=on",
		"default.cubrid.max_clients=100",
		"default.broker1.BROKER_PORT=33000",
		"unrelated=x",
	}, "\n"))
	got := cfg.Prefixed("default.cubrid")
	if len(got) != 2 || got["async_commit"] != "on" || got["max_clients"] != "100" {
		t.Errorf("got %v", got)
	}
}

// TestRealConfigFilesParse reads the thirteen files CTP ships, when the tree is
// present. Accepting them is the whole of the F3 obligation, so this is the test
// that actually proves it.
func TestRealConfigFilesParse(t *testing.T) {
	root := "/data/cub_sys/cubrid-testtools/CTP/conf"
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Skipf("no CTP tree at %s", root)
	}
	h := &Home{Path: "/data/cub_sys/cubrid-testtools/CTP"}
	seen := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".conf" {
			continue
		}
		seen++
		if _, err := h.Load(filepath.Join(root, e.Name())); err != nil {
			t.Errorf("%s: %v", e.Name(), err)
		}
	}
	if seen == 0 {
		t.Skip("no .conf files found")
	}
	t.Logf("parsed %d shipped configuration files", seen)
}
