package sqlsuite

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// engine is what run.sh's init_cubrid_version reads out of cubrid_rel, in the
// shape run.sh keeps it: the version string, its four parts, and the word size.
//
// Not shellsuite's BuildID. The shell task's rule is CTP's Java, which looks for
// a four-part number anywhere; the sql task's is awk over the line that says
// CUBRID, and the two differ on a build without a commit suffix. Each suite
// reproduces its own.
type engine struct {
	// rel is cubrid_rel's CUBRID line, which main.info records whole.
	rel string
	// ver is the text between the first "(" and the ")" after it:
	// 11.5.0.2560-a7a1db8.
	ver string
	// major and minor are compared as numbers by run.sh; the other two parts
	// are only ever written out.
	major, minor int
	patch, build string
	// prefix is ver without its last dotted part: 11.5.0.
	prefix string
	// bits is "64" or "32": the word before "bit" in the second parenthesis.
	bits  string
	debug bool
}

var leadingDigits = regexp.MustCompile(`[0-9]+`)

// parseEngine reads cubrid_rel's output.
//
//	CUBRID 11.5.0 (11.5.0.2560-a7a1db8) (64bit release build for Linux) (Sep 11 2026 13:55:59)
func parseEngine(out string) (engine, error) {
	var e engine
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "CUBRID") {
			e.rel = strings.TrimSpace(line)
			break
		}
	}
	if e.rel == "" {
		return e, fmt.Errorf("cubrid_rel printed no CUBRID line: %q", strings.TrimSpace(out))
	}
	e.debug = strings.Contains(out, "debug")

	// awk -F'(' '{print $2}' | awk -F')' '{print $1}'
	paren := strings.Split(e.rel, "(")
	if len(paren) < 3 {
		return e, fmt.Errorf("cannot read a version out of %q", e.rel)
	}
	e.ver, _, _ = strings.Cut(paren[1], ")")
	// awk -F'(' '{print $3}' | awk '{print $1}', then ${cubrid_bits%bit}
	if f := strings.Fields(paren[2]); len(f) > 0 {
		e.bits = strings.TrimSuffix(f[0], "bit")
	}

	parts := strings.Split(e.ver, ".")
	if len(parts) < 4 {
		return e, fmt.Errorf("version %q from %q does not have four parts", e.ver, e.rel)
	}
	var err error
	if e.major, err = strconv.Atoi(leadingDigits.FindString(parts[0])); err != nil {
		return e, fmt.Errorf("version %q: %w", e.ver, err)
	}
	if e.minor, err = strconv.Atoi(parts[1]); err != nil {
		return e, fmt.Errorf("version %q: %w", e.ver, err)
	}
	e.patch, e.build = parts[2], parts[3]
	e.prefix = e.ver[:strings.LastIndex(e.ver, ".")]
	return e, nil
}

// atLeast is run.sh's `[ p1 -gt M ] || { [ p1 -eq M ] && [ p2 -ge m ]; }`.
func (e engine) atLeast(major, minor int) bool {
	return e.major > major || e.major == major && e.minor >= minor
}
