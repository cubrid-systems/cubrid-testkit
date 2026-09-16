package ctl

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// qablockedSource is the probe, carried in the binary because it has to be
// compiled where the engine under test is: the symbol it calls belongs to that
// engine's libcubridcs (ADR-019).
//
//go:embed native/qablocked.c
var qablockedSource string

// Install puts this controller where runone.sh looks for one.
//
// runone.sh runs `$ctlpath/qactl` (runone.sh:265), and prepare.sh rebuilds that
// with `make clean qactl qacsql` whenever it runs -- so a file dropped in is
// deleted again, and the rule that would rebuild it compiles ctltool's
// controller. Both halves are therefore replaced: the Makefile gets a `qactl`
// rule that writes a shim to this program and builds the probe, and the probe is
// built once here as well, because prepare.sh runs only when it finds no qactl
// or no server and neither is guaranteed. `qacsql` is not touched; it stays
// ctltool's own client, compiled from ctltool's own source.
//
// The directory is a slot's overlay, so all of this is thrown away with the slot.
func Install(ctltool, testkit string) error {
	if err := os.WriteFile(filepath.Join(ctltool, "qablocked.c"), []byte(qablockedSource), 0o644); err != nil {
		return err
	}
	path := filepath.Join(ctltool, "Makefile")
	mk, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	out, err := rewriteMakefile(string(mk), testkit)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return err
	}
	// Built now, against the engine this run is testing, so that a case does not
	// discover it missing.
	build := exec.Command("make", "qablocked")
	build.Dir = ctltool
	if out, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("building the lock probe in %s: %w\n%s", ctltool, err, out)
	}
	return os.WriteFile(filepath.Join(ctltool, "qactl"), []byte(shim(testkit)), 0o755)
}

func shim(testkit string) string {
	return fmt.Sprintf("#!/bin/sh\nexec %s isolation-ctl \"$@\"\n", testkit)
}

// qactlRule is a make target that does not compile: it writes the shim, and
// hangs the probe off it so that `make qactl` builds that too.
var qactlRule = regexp.MustCompile(`(?m)^qactl[ \t]*:.*\n(?:\t.*\n)*`)

func rewriteMakefile(mk, testkit string) (string, error) {
	if !qactlRule.MatchString(mk) {
		return "", fmt.Errorf("no qactl rule in the Makefile: ctltool is not where it was")
	}
	var b strings.Builder
	b.WriteString("qactl : qablocked\n")
	b.WriteString("\tprintf '#!/bin/sh\\nexec %s isolation-ctl \"$$@\"\\n' '" + testkit + "' > qactl\n")
	b.WriteString("\tchmod +x qactl\n")
	b.WriteString("\n")
	b.WriteString("qablocked : qablocked.c\n")
	b.WriteString("\t$(CC) $(CFLAGS) -I$(CUBRID_INCLUDE) -o $@ $^ $(CUBRID_LDFLAGS)\n")
	return qactlRule.ReplaceAllLiteralString(mk, b.String()), nil
}
