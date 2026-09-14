package contain

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// LinkerEnv turns off the shim that makes the linker behave the way the CI
// image's does. Any non-empty value leaves the toolchain alone.
const LinkerEnv = "TESTKIT_NO_LINK_SHIM"

// GccShim writes a gcc that adds -Wl,--no-as-needed, and returns the directory
// to put in front of PATH. It returns "" when nothing needs doing.
//
// This is the same argument as the bash bind above, one layer down. CTP's xgcc
// builds its command line as
//
//	gcc_option="-g -I$CUBRID/include -L$CUBRID/lib -lcascci $gcc_option"
//
// where $gcc_option is the case's own "-o <out> <src>.c" -- so the library is
// named before the object that needs it. GNU ld resolves left to right, and
// with --as-needed a shared library reached before any symbol requires it is
// dropped. Every reference then comes out undefined.
//
// --as-needed is the default on Debian and Ubuntu and not on RHEL or Rocky, so
// this is a property of the distribution rather than of the test, of the
// engine, or of the runner. Measured on one case, same source, same library,
// same compiler:
//
//	gcc ... -lcascci -o lb1 x.c                    undefined reference
//	gcc ... -o lb2 x.c ... -lcascci                links
//	gcc -Wl,--no-as-needed ... -lcascci -o lb3 x.c  links
//
// The third line is why the shim is the right shape: the fix is to stop the
// linker dropping a library the command line asked for, which is what CI's
// toolchain already does. 173 case scripts call xgcc, and on a full-corpus run
// this was 112 of 279 failures; re-running those with the shim left 2.
//
// The 2 were g++. Cases build C++ clients with their own makefiles rather than
// through xgcc, in the same order and against the same library, so every driver
// a case might reach has to be covered and not only the one CTP's helper names.
//
// It does not paper over a defect that would fail in CI. CTP's link order is
// wrong and stays wrong -- it is item C2 in docs/project/evidence/ctp-improvements.md
// and belongs upstream. What this removes is a difference between this machine
// and the machine the verdicts are compared against, which is the whole job.
func GccShim(dir string) (string, error) {
	if os.Getenv(LinkerEnv) != "" {
		return "", nil
	}
	gcc, err := exec.LookPath("gcc")
	if err != nil {
		// No compiler: the cases that need one fail for a reason of their own,
		// and a shim for a compiler that is not there helps nobody.
		return "", nil
	}
	if !linkerDropsUnusedLibraries(gcc) {
		// RHEL, Rocky, and anything else whose driver does not pass --as-needed.
		// Nothing to correct, and no shim in the mount table.
		return "", nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	// One probe, every driver: --as-needed is passed by the specs the drivers
	// share, so what is true of gcc is true of g++ on the same machine. Only
	// drivers that exist get a shim, so a machine without a C++ compiler does
	// not grow one that fails differently.
	var wrote int
	for _, name := range Drivers {
		real, lerr := exec.LookPath(name)
		if lerr != nil {
			continue
		}
		body := "#!/bin/sh\n" +
			"# Written by testkit. See internal/contain/linker.go.\n" +
			"exec " + real + " -Wl,--no-as-needed \"$@\"\n"
		if werr := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); werr != nil {
			return "", fmt.Errorf("write %s shim: %w", name, werr)
		}
		wrote++
	}
	if wrote == 0 {
		return "", nil
	}
	return dir, nil
}

// Drivers are the compiler front ends a case might reach. CTP's xgcc calls gcc,
// but cases build C++ clients with makefiles of their own, and two of them were
// the only link failures left after the first version of this shim covered gcc
// alone.
var Drivers = []string{"gcc", "g++", "cc", "c++"}

// linkerDropsUnusedLibraries asks the toolchain rather than the distribution.
//
// A file that references nothing, linked against a library named before it: if
// the driver passes --as-needed the library is dropped and does not appear as a
// NEEDED entry. Two seconds, once per run, and it cannot go stale.
func linkerDropsUnusedLibraries(gcc string) bool {
	tmp, err := os.MkdirTemp("", "testkit-ld")
	if err != nil {
		return false
	}
	defer os.RemoveAll(tmp)

	src := filepath.Join(tmp, "probe.c")
	if err := os.WriteFile(src, []byte("int main(void){return 0;}\n"), 0o644); err != nil {
		return false
	}
	out := filepath.Join(tmp, "probe")
	// -lm is in every toolchain and nothing here uses it, which is the point.
	if err := exec.Command(gcc, "-lm", "-o", out, src).Run(); err != nil {
		return false
	}
	readelf, err := exec.LookPath("readelf")
	if err != nil {
		return false
	}
	b, err := exec.Command(readelf, "-d", out).Output()
	if err != nil {
		return false
	}
	return !strings.Contains(string(b), "libm.so")
}
