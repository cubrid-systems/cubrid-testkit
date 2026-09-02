package topology

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cubrid-systems/cubrid-testkit/internal/conf"
)

func load(t *testing.T, body string) *conf.Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), "x.conf")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &conf.Home{Path: "/opt/ctp"}
	cfg, err := h.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestInstanceOverridesDefault(t *testing.T) {
	// The whole of the dot-notation contract in one case: a default applies to
	// every instance, and an instance key beats it.
	cfg := load(t, strings.Join([]string{
		"default.ssh.port=22",
		"default.ssh.pwd=secret",
		"default.cubrid.async_commit=on",
		"env.instance1.ssh.host=host-a",
		"env.instance2.ssh.host=host-b",
		"env.instance2.ssh.port=2222",
		"env.instance2.cubrid.async_commit=off",
	}, "\n"))

	insts, err := From(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(insts) != 2 {
		t.Fatalf("got %d instances, want 2", len(insts))
	}

	one, two := insts[0].SSH(), insts[1].SSH()
	if one.Host != "host-a" || one.Port != "22" || one.Password != "secret" {
		t.Errorf("instance1: %+v", one)
	}
	if two.Host != "host-b" || two.Port != "2222" || two.Password != "secret" {
		t.Errorf("instance2 must override the port and inherit the password: %+v", two)
	}
	if got := insts[0].Role("cubrid")["async_commit"]; got != "on" {
		t.Errorf("instance1 async_commit: %q", got)
	}
	if got := insts[1].Role("cubrid")["async_commit"]; got != "off" {
		t.Errorf("instance2 async_commit: %q", got)
	}
}

func TestArbitraryPropertiesSurvive(t *testing.T) {
	// <property> is not from a list. Anything after the role has to reach the
	// remote configuration file untouched.
	cfg := load(t, "env.instance1.cubrid.some_parameter_nobody_listed=7\n")
	insts, err := From(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := insts[0].Role("cubrid")["some_parameter_nobody_listed"]; got != "7" {
		t.Errorf("got %q, want 7", got)
	}
}

func TestInstancesAreOrderedAndNamedAsInTheMarkers(t *testing.T) {
	cfg := load(t, "env.instance3.ssh.host=c\nenv.instance1.ssh.host=a\n")
	insts, err := From(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(insts) != 2 || insts[0].ID != 1 || insts[1].ID != 3 {
		t.Fatalf("got %v", insts)
	}
	// "[ENV START] env1" is a frozen marker, so the identifier has to read that way.
	if got := insts[0].EnvID(); got != "env1" {
		t.Errorf("EnvID: got %q want env1", got)
	}
}

func TestNoInstanceKeysMeansNoInstances(t *testing.T) {
	// unittest runs locally and its configuration names no machine.
	cfg := load(t, "scenario=/x\ndefault.ssh.port=22\n")
	insts, err := From(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(insts) != 0 {
		t.Errorf("got %d instances, want none", len(insts))
	}
}
