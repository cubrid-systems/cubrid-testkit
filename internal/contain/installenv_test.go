package contain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeInstall makes a directory that looks like an install to CheckInstallEnv:
// an executable bin/cubrid and a lib/libcubrid.so.
func fakeInstall(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, d := range []string{"bin", "lib", "databases"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "cubrid"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "lib", "libcubrid.so"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func setInstallEnv(t *testing.T, cubrid, path, ldPath string) {
	t.Helper()
	t.Setenv("CUBRID", cubrid)
	t.Setenv("CUBRID_DATABASES", filepath.Join(cubrid, "databases"))
	t.Setenv("PATH", path)
	t.Setenv("LD_LIBRARY_PATH", ldPath)
}

func TestCheckInstallEnv(t *testing.T) {
	given, other := fakeInstall(t), fakeInstall(t)
	join := func(dirs ...string) string { return strings.Join(dirs, string(os.PathListSeparator)) }

	t.Run("one install throughout is accepted", func(t *testing.T) {
		setInstallEnv(t, given, join(filepath.Join(given, "bin"), filepath.Join(other, "bin")),
			join(filepath.Join(given, "lib"), filepath.Join(other, "lib")))
		if err := CheckInstallEnv(); err != nil {
			t.Errorf("CheckInstallEnv = %v, want nil", err)
		}
	})

	t.Run("a symbolic link to the install is the install", func(t *testing.T) {
		link := filepath.Join(t.TempDir(), "cub")
		if err := os.Symlink(given, link); err != nil {
			t.Fatal(err)
		}
		setInstallEnv(t, link, filepath.Join(given, "bin"), filepath.Join(link, "lib"))
		if err := CheckInstallEnv(); err != nil {
			t.Errorf("CheckInstallEnv = %v, want nil", err)
		}
	})

	t.Run("a prefix whose entries link into the install is the install", func(t *testing.T) {
		prefix := t.TempDir()
		for _, d := range []string{"bin", "lib"} {
			if err := os.Symlink(filepath.Join(given, d), filepath.Join(prefix, d)); err != nil {
				t.Fatal(err)
			}
		}
		setInstallEnv(t, prefix, filepath.Join(prefix, "bin"), filepath.Join(prefix, "lib"))
		if err := CheckInstallEnv(); err != nil {
			t.Errorf("CheckInstallEnv = %v, want nil", err)
		}
	})

	t.Run("a bin whose cubrid is a link is that bin", func(t *testing.T) {
		linked := fakeInstall(t)
		bin := filepath.Join(linked, "bin", "cubrid")
		if err := os.Remove(bin); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(given, "bin", "cubrid"), bin); err != nil {
			t.Fatal(err)
		}
		setInstallEnv(t, linked, filepath.Join(linked, "bin"), filepath.Join(linked, "lib"))
		if err := CheckInstallEnv(); err != nil {
			t.Errorf("CheckInstallEnv = %v, want nil", err)
		}
	})

	t.Run("a cubrid found through a relative entry is refused for that reason", func(t *testing.T) {
		setInstallEnv(t, given, ".", filepath.Join(given, "lib"))
		t.Chdir(filepath.Join(given, "bin"))
		if err := CheckInstallEnv(); err == nil || !strings.Contains(err.Error(), "relative entry") {
			t.Errorf("CheckInstallEnv = %v, want an error about the relative entry", err)
		}
	})

	for _, c := range []struct {
		name, want string
		set        func(t *testing.T)
	}{
		{"no CUBRID", "CUBRID is not set", func(t *testing.T) {
			setInstallEnv(t, given, filepath.Join(given, "bin"), "")
			t.Setenv("CUBRID", "")
		}},
		{"no CUBRID_DATABASES", "CUBRID_DATABASES is not set", func(t *testing.T) {
			setInstallEnv(t, given, filepath.Join(given, "bin"), "")
			t.Setenv("CUBRID_DATABASES", "")
		}},
		{"no cubrid on PATH", "there is no cubrid on PATH", func(t *testing.T) {
			setInstallEnv(t, given, t.TempDir(), "")
		}},
		{"another install's cubrid first on PATH", "the cubrid on PATH is", func(t *testing.T) {
			setInstallEnv(t, given, join(filepath.Join(other, "bin"), filepath.Join(given, "bin")), "")
		}},
		{"another install's lib ahead on LD_LIBRARY_PATH", "LD_LIBRARY_PATH has", func(t *testing.T) {
			setInstallEnv(t, given, filepath.Join(given, "bin"),
				join(filepath.Join(other, "lib"), filepath.Join(given, "lib")))
		}},
		{"another install's lib with this one's not on LD_LIBRARY_PATH", "LD_LIBRARY_PATH has", func(t *testing.T) {
			setInstallEnv(t, given, filepath.Join(given, "bin"), filepath.Join(other, "lib"))
		}},
		{"this install's lib not on LD_LIBRARY_PATH", "LD_LIBRARY_PATH does not have", func(t *testing.T) {
			setInstallEnv(t, given, filepath.Join(given, "bin"), t.TempDir())
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			c.set(t)
			err := CheckInstallEnv()
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("CheckInstallEnv = %v, want an error saying %q", err, c.want)
			}
		})
	}
}
