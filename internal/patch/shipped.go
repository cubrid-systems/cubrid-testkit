package patch

import (
	"errors"
	"os"
	"path/filepath"
)

// ErrNoPatchSet says TESTKIT_PATCHES names nothing, so there is no set to guard.
//
// It is not a failure. The patches are optional the way a corpus checkout is
// optional, and a guard with nothing to guard skips rather than fails.
var ErrNoPatchSet = errors.New("TESTKIT_PATCHES is unset, so there is no patch set to check")

// Shipped is where a family's patches are.
//
// Not in this repository. A patch is a diff, and a diff carries the lines
// around the change: the shell set patches cubrid-testcases-private-ex, so
// those files reproduce private corpus source and cannot sit in a public
// repository. The whole set lives in cubrid-testkit-patches, which is private,
// and TESTKIT_PATCHES names a checkout of it.
//
// The path is built from that root rather than from the module, because the
// module no longer holds it. Counting is what broke the last version of this:
// the tree was renamed from patches/ to overrides/patches/, the guards went on
// pointing at the old path, os.ReadDir failed, the guards skipped, and for ten
// commits nothing checked that any patch still applied. A guard that cannot
// find what it guards has to say so — which is why a root that is set but wrong
// still returns a directory, and the caller fails on the stat.
func Shipped(family string) (string, error) {
	root := os.Getenv("TESTKIT_PATCHES")
	if root == "" {
		return "", ErrNoPatchSet
	}
	return filepath.Join(root, family), nil
}
