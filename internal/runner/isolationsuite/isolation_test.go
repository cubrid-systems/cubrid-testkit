package isolationsuite

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cubrid-systems/cubrid-testkit/internal/cli"
	"github.com/cubrid-systems/cubrid-testkit/internal/conf"
	"github.com/cubrid-systems/cubrid-testkit/internal/runner"
)

func isolationRequest(t *testing.T, body string) runner.Request {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "conf"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "conf", "isolation.conf")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return runner.Request{Task: cli.Isolation, Home: &conf.Home{Path: home}, ConfigPath: path}
}

// The install under test comes from the environment the run starts with, so a
// run whose environment does not name one install stops before it starts, with
// the exit code of a run that could not start, and says what is missing.
func TestValidateAsksForTheInstallUnderTest(t *testing.T) {
	req := isolationRequest(t, "scenario=/s/isolation\n")
	t.Setenv("TESTKIT_CONTAINED", "1")

	install := t.TempDir()
	for _, d := range []string{"bin", "lib", "databases"} {
		if err := os.MkdirAll(filepath.Join(install, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(install, "bin", "cubrid"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Join(install, "bin"))
	t.Setenv("LD_LIBRARY_PATH", filepath.Join(install, "lib"))
	t.Setenv("CUBRID_DATABASES", filepath.Join(install, "databases"))

	t.Setenv("CUBRID", "")
	err := (&Isolation{}).Validate(req)
	var exit *runner.ExitError
	if !errors.As(err, &exit) || exit.Code != exitEnvironment || !strings.Contains(err.Error(), "CUBRID is not set") {
		t.Errorf("Validate without CUBRID = %v, want exit %d saying CUBRID is not set", err, exitEnvironment)
	}

	t.Setenv("CUBRID", install)
	if err := (&Isolation{}).Validate(req); err != nil {
		t.Errorf("Validate with one install throughout = %v, want nil", err)
	}
}
