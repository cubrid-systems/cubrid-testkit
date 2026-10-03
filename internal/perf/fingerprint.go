package perf

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Fingerprint is the part of a build that decides whether two numbers are
// comparable: the compiler, the build type, the flags, and how each
// third-party library was compiled (Spec §7.9). It comes from the build
// step's build-info.json; without that file, what cubrid_rel and the binary
// itself say, marked partial.
type Fingerprint struct {
	Version    string            `json:"version,omitempty"`
	Commit     string            `json:"commit,omitempty"`
	Label      string            `json:"label,omitempty"`
	BuiltAt    string            `json:"built_at,omitempty"`
	Compiler   string            `json:"compiler,omitempty"`
	BuildType  string            `json:"build_type,omitempty"`
	CXXFlags   string            `json:"cxx_flags,omitempty"`
	Thirdparty map[string]string `json:"thirdparty,omitempty"` // lib -> -O level, "" when unknown
	Partial    bool              `json:"partial"`
}

type buildInfo struct {
	Commit     string `json:"commit"`
	Label      string `json:"label"`
	BuiltAt    string `json:"built_at"`
	Compiler   string `json:"compiler"`
	BuildType  string `json:"build_type"`
	CXXFlags   string `json:"cxx_flags"`
	Thirdparty map[string]struct {
		Opt *string `json:"opt"`
	} `json:"thirdparty"`
}

var (
	relRe = regexp.MustCompile(`CUBRID\s+(\S+)\s+\(([^)]*)\)\s+\(([^)]*)\)`)
	gccRe = regexp.MustCompile(`GCC: \(([^)]*)\) ([0-9.]+)`)
)

// readFingerprint reads a build's fingerprint (Design §5.8).
func readFingerprint(build string) Fingerprint {
	if b, err := os.ReadFile(filepath.Join(build, "build-info.json")); err == nil {
		var bi buildInfo
		if json.Unmarshal(b, &bi) == nil {
			fp := Fingerprint{Commit: bi.Commit, Label: bi.Label, BuiltAt: bi.BuiltAt, Compiler: bi.Compiler,
				BuildType: bi.BuildType, CXXFlags: bi.CXXFlags, Thirdparty: map[string]string{}}
			for lib, t := range bi.Thirdparty {
				if t.Opt != nil {
					fp.Thirdparty[lib] = *t.Opt
				} else {
					fp.Thirdparty[lib] = ""
				}
			}
			return fp
		}
	}
	fp := Fingerprint{Partial: true}
	rel := exec.Command(filepath.Join(build, "bin", "cubrid_rel"))
	rel.Env = append(os.Environ(), "CUBRID="+build, "LD_LIBRARY_PATH="+filepath.Join(build, "lib"))
	if out, err := rel.Output(); err == nil {
		if m := relRe.FindStringSubmatch(string(out)); m != nil {
			fp.Version = m[1]
			// "11.5.0.2513-5f3a30d": the commit is after the last dash.
			if i := strings.LastIndex(m[2], "-"); i >= 0 {
				fp.Commit = m[2][i+1:]
			}
			if strings.Contains(m[3], "debug") {
				fp.BuildType = "Debug"
			} else if strings.Contains(m[3], "release") {
				fp.BuildType = "Release"
			}
		}
	}
	if out, err := exec.Command("readelf", "-p", ".comment", filepath.Join(build, "bin", "cub_server")).Output(); err == nil {
		if m := gccRe.FindStringSubmatch(string(out)); m != nil {
			fp.Compiler = "gcc " + m[2]
		}
	}
	return fp
}

// SameFingerprint compares the four groups Design §5.8 names.
func SameFingerprint(a, b Fingerprint) bool {
	if a.Compiler != b.Compiler || a.BuildType != b.BuildType || a.CXXFlags != b.CXXFlags {
		return false
	}
	if len(a.Thirdparty) != len(b.Thirdparty) {
		return false
	}
	for k, v := range a.Thirdparty {
		if b.Thirdparty[k] != v {
			return false
		}
	}
	return true
}
