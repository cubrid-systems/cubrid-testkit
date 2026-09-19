package patch

import (
	"fmt"
	"os"
	"path/filepath"
)

// Shipped is where this repository keeps a family's patches.
//
// Found by walking up to the module root rather than by counting ".." from a
// package, because counting is what broke: the tree was renamed from patches/ to
// overrides/patches/ and the two guards that check the shipped set against the
// corpus went on pointing at the old path. os.ReadDir failed, the guards skipped,
// and for the ten commits since nothing checked that any patch still applied. A guard
// that cannot find what it guards has to say so, which is why this returns an
// error and its callers fail on it.
func Shipped(family string) (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "overrides", "patches", family), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod above %s, so overrides/patches/%s cannot be found", dir, family)
		}
		dir = parent
	}
}
