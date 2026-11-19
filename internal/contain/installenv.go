package contain

import (
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
)

// CheckInstallEnv says whether the environment a contained run starts with names
// one install throughout.
//
// A slot overlays $CUBRID and $CUBRID_DATABASES, and the scripts a contained
// run sends take this environment as it is, without ~/.bash_profile. So the
// `cubrid` they find on PATH and the libcubrid the loader finds first have to
// be that install's as well, and $CUBRID/lib has to be on LD_LIBRARY_PATH.
// Otherwise the run reports one build and tests another: a shell that once
// sourced a profile keeps that install's bin on PATH and its lib on
// LD_LIBRARY_PATH, and exporting CUBRID alone does not move them. The loader
// reads LD_LIBRARY_PATH before an executable's RUNPATH, so a right PATH with a
// wrong LD_LIBRARY_PATH still loads the wrong engine.
func CheckInstallEnv() error {
	cubrid := strings.TrimSpace(os.Getenv("CUBRID"))
	if cubrid == "" {
		return fmt.Errorf("CUBRID is not set: export the install under test")
	}
	if strings.TrimSpace(os.Getenv("CUBRID_DATABASES")) == "" {
		return fmt.Errorf("CUBRID_DATABASES is not set: export it as well, under $CUBRID where a slot's overlay covers it")
	}
	found, err := osexec.LookPath("cubrid")
	if errors.Is(err, osexec.ErrDot) {
		// The scripts change directory before they run anything, so a relative
		// entry names a different directory for them than it does here.
		return fmt.Errorf("the cubrid on PATH is %s, found through a relative entry, which the scripts read from "+
			"another directory: put %s ahead of it", found, filepath.Join(cubrid, "bin"))
	}
	if err != nil {
		return fmt.Errorf("there is no cubrid on PATH: put %s on it", filepath.Join(cubrid, "bin"))
	}
	// Directories are compared, each resolved on its own: an install can be a
	// prefix whose entries are links into another, so its root resolves to
	// itself, and a bin whose cubrid is a link is still that bin.
	if bin := realPath(filepath.Join(cubrid, "bin")); realPath(filepath.Dir(found)) != bin {
		return fmt.Errorf("the cubrid on PATH is %s, not the one in %s: put %s ahead of it",
			found, filepath.Join(cubrid, "bin"), filepath.Join(cubrid, "bin"))
	}

	// $CUBRID/lib has to be there, not only unshadowed: ctltool's qactl and
	// qacsql are linked against libcubridcs with no RUNPATH, so without it they
	// fail to start, or load whichever install the loader's cache names.
	lib := realPath(filepath.Join(cubrid, "lib"))
	for _, d := range filepath.SplitList(os.Getenv("LD_LIBRARY_PATH")) {
		if strings.TrimSpace(d) == "" {
			continue
		}
		if realPath(d) == lib {
			return nil
		}
		if libs, _ := filepath.Glob(filepath.Join(d, "libcubrid*.so*")); len(libs) > 0 {
			return fmt.Errorf("LD_LIBRARY_PATH has %s ahead of %s, and it holds %s: the engine would load from there",
				d, filepath.Join(cubrid, "lib"), filepath.Base(libs[0]))
		}
	}
	return fmt.Errorf("LD_LIBRARY_PATH does not have %s: put it there", filepath.Join(cubrid, "lib"))
}

// realPath is p with its symbolic links resolved, or p made absolute when that
// fails, so two names for one directory compare equal.
func realPath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return filepath.Clean(p)
}
