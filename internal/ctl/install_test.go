package ctl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The Makefile prepare.sh runs must come out building this controller and the
// probe, and still building ctltool's own client from ctltool's own source.
func TestInstallRewritesTheMakefile(t *testing.T) {
	dir := t.TempDir()
	const original = `CC	= gcc
CUBRID_DIR	= $${CUBRID}
CUBRID_INCLUDE=$(CUBRID_DIR)/include
CFLAGS	= -O0 -g -W -Wall
CUBRID_LDFLAGS	= -L$(CUBRID_DIR)/lib -lcubridcs

all : qactl qacsql

qactl : common.o parse.o qamccom.o qactl.o cubrid_drv.o 
	$(CC) -o $@ $^ $(CUBRID_LDFLAGS)

qacsql : common.o parse.o cubrid_drv.o qacsql.o
	$(CC) -o $@ $^ $(CUBRID_LDFLAGS) 

clean :
	rm -rf *.o qactl qactlm qactlo qacsql qamysql qaoracle
`
	if err := os.WriteFile(filepath.Join(dir, "Makefile"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	fakeMake(t, dir)
	if err := Install(dir, "/opt/testkit"); err != nil {
		t.Fatal(err)
	}

	mk, err := os.ReadFile(filepath.Join(dir, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(mk)
	if strings.Contains(got, "qactl.o cubrid_drv.o") {
		t.Error("the old qactl rule is still there; make would compile ctltool's controller")
	}
	for _, want := range []string{"qactl : qablocked", "isolation-ctl", "qablocked : qablocked.c"} {
		if !strings.Contains(got, want) {
			t.Errorf("the Makefile does not have %q", want)
		}
	}
	if !strings.Contains(got, "qacsql : common.o parse.o cubrid_drv.o qacsql.o") {
		t.Error("the client's rule was touched; it must stay ctltool's")
	}
	if !strings.Contains(got, "rm -rf *.o qactl") {
		t.Error("clean was touched")
	}

	// The probe's source and a working shim are in place before make runs at all,
	// for the case where prepare.sh decides it has nothing to do.
	if b, err := os.ReadFile(filepath.Join(dir, "qablocked.c")); err != nil || !strings.Contains(string(b), "tran_is_blocked") {
		t.Errorf("qablocked.c: %v", err)
	}
	fi, err := os.Stat(filepath.Join(dir, "qactl"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o111 == 0 {
		t.Error("the shim is not executable")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "qactl")); !strings.Contains(string(b), "/opt/testkit isolation-ctl") {
		t.Errorf("shim: %s", b)
	}
}

// fakeMake puts a make on PATH that records what it was asked for, so that
// Install can be tested without an engine to compile the probe against.
func fakeMake(t *testing.T, dir string) {
	t.Helper()
	bin := t.TempDir()
	script := "#!/bin/sh\necho \"$@\" >> " + filepath.Join(dir, "make.log") + "\ntouch " + filepath.Join(dir, "qablocked") + "\n"
	if err := os.WriteFile(filepath.Join(bin, "make"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestInstallBuildsTheProbe(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Makefile"), []byte("qactl : a.o\n\tgcc -o $@ $^\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fakeMake(t, dir)
	if err := Install(dir, "/opt/testkit"); err != nil {
		t.Fatal(err)
	}
	// prepare.sh runs only when it finds no qactl or no server, so the probe
	// cannot wait for it.
	if b, err := os.ReadFile(filepath.Join(dir, "make.log")); err != nil || strings.TrimSpace(string(b)) != "qablocked" {
		t.Errorf("make was asked for %q, %v; want qablocked", b, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "qablocked")); err != nil {
		t.Errorf("the probe was not built: %v", err)
	}
}

func TestInstallRefusesAnUnfamiliarMakefile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Makefile"), []byte("all:\n\techo nothing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Install(dir, "/opt/testkit"); err == nil {
		t.Error("a Makefile with no qactl rule should be refused, not silently left alone")
	}
}
