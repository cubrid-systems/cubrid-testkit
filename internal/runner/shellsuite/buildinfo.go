package shellsuite

import (
	"regexp"
	"strconv"
	"strings"
)

// A run has to say which build it tested, and the only place that is written down
// is a string: either the download URL, or what the installed engine reports.
// Both are parsed the same way, which is why a build id can be read out of a URL
// nobody downloaded from.

var versionPattern = regexp.MustCompile(`\d+\.\d+\.\d+\.\d+`)

// newNumberingFrom is where CUBRID's build numbers changed shape. From this
// version on, the four-part number is followed by a suffix that is part of the
// identity, so the id extends past it.
const newNumberingFrom = "10.1.0.6858"

// BuildID extracts the build identity from a package URL or from cubrid_rel's
// output.
//
// The *last* four-part number wins, because a URL commonly has a version in its
// directory path and again in the file name, and the file name is the build.
func BuildID(s string) string {
	all := versionPattern.FindAllString(s, -1)
	if len(all) == 0 {
		return ""
	}
	simple := all[len(all)-1]
	if !newNumbering(simple) {
		return simple
	}

	start := strings.LastIndex(s, simple)
	rest := start + len(simple)

	// From 10.1.0.6858 the four-part number is followed by a commit, as in
	// 11.4.5.1875-74d17e9, and the commit is part of the identity. It is a dash
	// and then letters and digits, and nothing else.
	//
	// CTP looked for the next "-", then ")", then "." after the version and cut
	// there -- which works when a commit is present and runs away when one is not.
	// A cubrid_rel line for a build without a commit gave
	// "11.2.0.0000) (64bit release build for linux_gnu" as the build id, and that
	// went into main_snapshot.properties and into every case's environment.
	if rest < len(s) && s[rest] == '-' {
		i := rest + 1
		for i < len(s) && isBuildSuffixByte(s[i]) {
			i++
		}
		if i > rest+1 {
			return s[start:i]
		}
	}
	return simple
}

// BuildBits reports "64bits" or "32bits". It is not a fact about the machine but
// a reading of the same string, and it selects JAVA_HOME_64BITS or
// JAVA_HOME_32BITS on the remote host.
func BuildBits(s string) string {
	for _, marker := range []string{"_64", "x64", "ppc64", "64bit"} {
		if strings.Contains(s, marker) {
			return "64bits"
		}
	}
	return "32bits"
}

func isBuildSuffixByte(b byte) bool {
	return b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

// newNumbering compares two four-part versions componentwise.
func newNumbering(version string) bool {
	if version == "" {
		return false
	}
	a, b := parts(version), parts(newNumberingFrom)
	for i := range b {
		if i >= len(a) {
			return false
		}
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return true
}

func parts(v string) []int {
	fields := strings.Split(v, ".")
	out := make([]int, 0, len(fields))
	for _, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil {
			return out
		}
		out = append(out, n)
	}
	return out
}

// versionScript is what CTP asked the machine. qa.conf is written by the
// installer and carries the URL the build came from; cubrid_rel is the fallback
// for an engine installed some other way.
const versionScript = "cat ${CUBRID}/qa.conf |grep 'Test_Build_URL'||cubrid_rel"
