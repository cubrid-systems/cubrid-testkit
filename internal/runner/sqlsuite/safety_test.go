package sqlsuite

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tkexec "github.com/cubrid-systems/cubrid-testkit/internal/exec"
)

// do_clean deletes the database, everything under $CUBRID/logs, and every file
// named core* under $CUBRID and $CTP_HOME. The paths it uses come from the
// environment and from a configuration file, so each one is checked before the
// stage runs -- an empty one in a shell script is the root of the filesystem,
// which is how a run on the shell side deleted 69 directories.
func TestThePathsTheCleanStageDeletesUnderAreRefusedWhenTheyAreNotOwn(t *testing.T) {
	for _, c := range []struct {
		name, dir, want string
	}{
		{"unset", "", "empty"},
		{"relative", "cubrid", "absolute"},
		{"the root", "/", "that shallow"},
		{"one segment", "/usr", "that shallow"},
		{"a space, which is two arguments to rm", "/opt/my cubrid", "syntax"},
		{"a variable nobody expanded", "/opt/$HOME/cubrid", "syntax"},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := tkexec.SafeToEmpty("CUBRID", c.dir)
			if err == nil {
				t.Fatalf("%q was accepted as a directory to delete under", c.dir)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("%q was refused with %q, which does not say %q", c.dir, err, c.want)
			}
		})
	}
	if err := tkexec.SafeToEmpty("CUBRID", "/data/cub_sys/projects/regr-sql/CUBRID"); err != nil {
		t.Errorf("an ordinary install path was refused: %v", err)
	}
}

// do_clean's other two steps select by account rather than by run: `pkill cub`
// matches every CUBRID process the user owns, and remove_shared_memory hands
// `ipcrm` every segment `ipcs` attributes to $USER. That is what cleaning up
// means on a machine given over to one run, and it is what run.sh does.
//
// It is also how a run takes down whatever else the account has going --
// another suite's databases, a broker in use, this project's own sandbox
// nodes, whose CUBRID processes are ordinary processes of the same user. So
// they run inside containment, where the account and the run are the same
// thing, and refuse outside it.
func TestTheAccountWideSweepRunsOnlyInsideContainment(t *testing.T) {
	for _, c := range []struct {
		name, flag string
		wantSwept  bool
	}{
		{"contained", "1", true},
		{"not contained", "", false},
		{"something else entirely", "maybe", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			out := runSweep(t, c.flag)
			swept := strings.Contains(out, "PKILL") || strings.Contains(out, "IPCRM")
			if swept != c.wantSwept {
				t.Errorf("testkit_contained=%q swept=%v, want %v; output:\n%s", c.flag, swept, c.wantSwept, out)
			}
			if !c.wantSwept && !strings.Contains(out, "not contained") {
				t.Errorf("refused without saying why:\n%s", out)
			}
			if c.wantSwept && !(strings.Contains(out, "PKILL") && strings.Contains(out, "IPCRM")) {
				t.Errorf("containment should get run.sh's behaviour, both halves of it:\n%s", out)
			}
		})
	}
}

// runSweep calls the one function, with the two commands it guards replaced by
// something that only says it was called. The script is sourced whole so the
// test is against what the runner embeds and not against a copy of it.
func runSweep(t *testing.T, flag string) string {
	t.Helper()
	dir := t.TempDir()
	// stages.sh asks the engine what it supports as it loads. A stub that says
	// nothing is enough: the answer decides other stages, not this one.
	stub := filepath.Join(dir, "cubrid")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	script := "export PATH=" + dir + ":$PATH\ntestkit_contained=" + flag + "\n" +
		stagesScript + "\n" +
		"pkill() { echo PKILL \"$@\"; }\nremove_shared_memory() { echo IPCRM; }\nsweep_this_account\n"
	res, err := tkexec.NewLocal(dir).Run(context.Background(), "exec 2>&1\n"+script)
	if err != nil {
		t.Fatal(err)
	}
	return res.Stdout
}
