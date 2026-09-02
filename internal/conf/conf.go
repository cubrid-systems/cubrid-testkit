// Package conf resolves where things live: the CTP home directory and the
// configuration file a task should read.
//
// It does not parse configuration yet. Nothing needs the contents until a task is
// rewritten natively -- until then the old code reads the file itself, and the
// only thing testkit must get right is which file.
package conf

import (
	"fmt"
	"os"
	"path/filepath"
)

// Home is the CTP installation root. Everything in CTP is addressed relative to
// it, which is the ambient global state M4 is meant to remove: it is read once,
// here, and passed explicitly from then on.
type Home struct {
	Path string
}

// FindHome resolves CTP_HOME. The environment wins; otherwise it is the parent of
// the directory holding the executable, which is what bin/ctp.sh computed.
func FindHome() (*Home, error) {
	if p := os.Getenv("CTP_HOME"); p != "" {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, fmt.Errorf("CTP_HOME=%q: %w", p, err)
		}
		return &Home{Path: abs}, nil
	}

	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("cannot locate the executable, and CTP_HOME is unset: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve %q: %w", exe, err)
	}
	return &Home{Path: filepath.Dir(filepath.Dir(exe))}, nil
}

// ConfigFor returns the configuration path for a suite. An explicit -c wins;
// otherwise it is $CTP_HOME/conf/<suite>.conf, which is part of the frozen
// surface (docs/concept/external-surface-freeze.md §1-1).
func (h *Home) ConfigFor(suite, explicit string) string {
	if explicit != "" {
		return explicit
	}
	return filepath.Join(h.Path, "conf", suite+".conf")
}

// Jar is a path under the CTP tree.
func (h *Home) Jar(parts ...string) string {
	return filepath.Join(append([]string{h.Path}, parts...)...)
}
