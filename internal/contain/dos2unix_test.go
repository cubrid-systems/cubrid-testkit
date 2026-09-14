package contain

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// withoutDos2Unix gives the test a PATH that has what the stand-in needs and
// no dos2unix, whether or not this machine has one.
func withoutDos2Unix(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	for _, tool := range []string{"sed"} {
		real, err := exec.LookPath(tool)
		if err != nil {
			t.Skipf("%s is not on this machine", tool)
		}
		if err := os.Symlink(real, filepath.Join(bin, tool)); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
}

// A machine that has dos2unix keeps it: the real tool is what CI runs, and a
// stand-in in front of it would be a difference rather than the removal of one.
func TestNoShimWhereTheMachineHasTheTool(t *testing.T) {
	dir := t.TempDir()
	// A dos2unix of our own, first on PATH, so this does not depend on whether
	// the machine running the test has one.
	real := t.TempDir()
	if err := os.WriteFile(filepath.Join(real, "dos2unix"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", real+":"+os.Getenv("PATH"))

	got, err := Dos2UnixShim(dir)
	if err != nil || got != "" {
		t.Fatalf("Dos2UnixShim = %q, %v; want no shim", got, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "dos2unix")); !os.IsNotExist(err) {
		t.Error("a shim was written for a machine that has the tool")
	}
}

func TestTheShimIsOffWhenAskedToBe(t *testing.T) {
	withoutDos2Unix(t)
	t.Setenv(Dos2UnixEnv, "1")
	if got, err := Dos2UnixShim(t.TempDir()); err != nil || got != "" {
		t.Fatalf("Dos2UnixShim = %q, %v; want no shim", got, err)
	}
}

// What init.sh asks for: the carriage returns go, in place, and the file is
// otherwise what it was.
func TestTheShimStripsCarriageReturnsInPlace(t *testing.T) {
	withoutDos2Unix(t)
	dir := t.TempDir()
	got, err := Dos2UnixShim(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("Dos2UnixShim = %q, want %q", got, dir)
	}
	shim := filepath.Join(dir, "dos2unix")

	work := filepath.Join(t.TempDir(), "output.log")
	if err := os.WriteFile(work, []byte("one\r\ntwo\r\n\r\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(shim, work).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	b, err := os.ReadFile(work)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "one\ntwo\n\nthree\n" {
		t.Errorf("the file is %q", b)
	}

	// init.sh hands it two files at a time.
	a := filepath.Join(t.TempDir(), "a")
	c := filepath.Join(t.TempDir(), "b")
	for _, f := range []string{a, c} {
		if err := os.WriteFile(f, []byte("x\r\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := exec.Command(shim, a, c).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	for _, f := range []string{a, c} {
		if b, _ := os.ReadFile(f); string(b) != "x\n" {
			t.Errorf("%s is %q", f, b)
		}
	}
}

// An option is refused rather than ignored: the only caller passes a bare
// filename, and a silently misread flag would corrupt a comparison instead of
// failing it.
func TestTheShimRefusesWhatItCannotDo(t *testing.T) {
	withoutDos2Unix(t)
	dir := t.TempDir()
	if _, err := Dos2UnixShim(dir); err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(dir, "dos2unix")

	for _, c := range []struct {
		what string
		args []string
	}{
		{"an option", []string{"-k", "somefile"}},
		{"a file that is not there", []string{filepath.Join(t.TempDir(), "absent")}},
	} {
		if err := exec.Command(shim, c.args...).Run(); err == nil {
			t.Errorf("%s should be an error", c.what)
		}
	}
}
