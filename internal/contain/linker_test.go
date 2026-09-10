package contain

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The shim exists to remove a difference between this machine and the machine
// the verdicts are compared against, and it must only appear where there is a
// difference to remove.
func TestTheShimIsWrittenOnlyWhereTheLinkerDropsLibraries(t *testing.T) {
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("no gcc")
	}
	dir := t.TempDir()
	got, err := GccShim(dir)
	if err != nil {
		t.Fatal(err)
	}
	drops := linkerDropsUnusedLibraries(mustPath(t, "gcc"))
	switch {
	case drops && got == "":
		t.Fatal("this toolchain drops a library named before the object, and no shim was written")
	case !drops && got != "":
		t.Fatal("this toolchain needs no shim and one was written anyway")
	}
	if !drops {
		return
	}
	// Every driver that exists, not only the one CTP's helper names: two cases
	// build C++ clients with makefiles of their own, and they were the only link
	// failures left when this covered gcc alone.
	for _, name := range Drivers {
		if _, lerr := exec.LookPath(name); lerr != nil {
			continue
		}
		if _, serr := os.Stat(filepath.Join(dir, name)); serr != nil {
			t.Errorf("%s is on this machine and has no shim: %v", name, serr)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "gcc")); err != nil {
		t.Fatalf("the shim directory does not hold a gcc: %v", err)
	}

	// And it has to actually fix the link it exists for: CTP's order, library
	// before source, against a real library.
	src := filepath.Join(dir, "x.c")
	if err := os.WriteFile(src, []byte("#include <math.h>\nint main(void){return (int)sqrt(4.0);}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bad := exec.Command("gcc", "-lm", "-o", filepath.Join(dir, "plain"), src)
	shimmed := exec.Command(filepath.Join(dir, "gcc"), "-lm", "-o", filepath.Join(dir, "fixed"), src)
	// glibc folds sqrt into the binary, so both may link; what must differ is
	// whether the library the command line asked for is recorded.
	if err := bad.Run(); err != nil {
		t.Logf("unshimmed link failed, which is the failure this exists for: %v", err)
	}
	if out, err := shimmed.CombinedOutput(); err != nil {
		t.Fatalf("the shim must link what CTP's order asks for: %v: %s", err, out)
	}
	b, err := exec.Command("readelf", "-d", filepath.Join(dir, "fixed")).Output()
	if err != nil {
		t.Skip("no readelf")
	}
	if !strings.Contains(string(b), "libm.so") {
		t.Error("the shim did not keep the library the command line named")
	}
}

func TestTheShimCanBeTurnedOff(t *testing.T) {
	t.Setenv(LinkerEnv, "1")
	dir := t.TempDir()
	got, err := GccShim(dir)
	if err != nil || got != "" {
		t.Fatalf("%s must leave the toolchain alone: %q %v", LinkerEnv, got, err)
	}
	for _, name := range Drivers {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			t.Errorf("a %s shim was written with the switch off", name)
		}
	}
}

func mustPath(t *testing.T, name string) string {
	t.Helper()
	p, err := exec.LookPath(name)
	if err != nil {
		t.Skip("no " + name)
	}
	return p
}
