package shellsuite

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
)

// A shell case is a script named after the directory two levels above it:
//
//	<scenario>/<name>/cases/<name>.sh
//
// The name is not decoration. do_check_more_errors in init_path/shell_utils.sh
// derives the result file from the *directory* name:
//
//	result_file_full_name=${test_case_dir%/cases*}/cases/${case_name}.result
//
// while the runner derives it from the *script* name. The two agree only when the
// script is named after its directory, so a script that is not becomes a case
// whose verdict is read from a file nobody wrote.
//
// This is why the sibling scripts in a cases/ directory -- PrintInfo.sh,
// common.sh, build.sh -- are helpers, not cases, and why running every *.sh under
// cases/ would run 270 helper scripts as if they were tests. CTP expresses the
// rule as an awk predicate over find output:
//
//	awk -F "/" '{ if( $(NF-2)".sh"== $NF) print }'
//
// docs/evidence/spec-corrections.md 10.
func IsCase(p string) bool {
	parts := strings.Split(p, "/")
	if len(parts) < 3 {
		return false
	}
	return parts[len(parts)-3]+".sh" == parts[len(parts)-1]
}

// Case is one discovered case, split the way the worker needs it.
type Case struct {
	// Path is the full path as discovered, and as written to dispatch_tc_ALL.txt.
	Path string
	// Dir is where the case runs: the cases/ directory itself.
	Dir string
	// Script is the file to run, relative to Dir.
	Script string
	// Result is the file the case writes its verdict into, relative to Dir.
	Result string
}

// Split locates the cases/ segment and derives the names around it.
//
// CTP used strings.LastIndexOf("cases") on the whole path, which finds the
// substring anywhere -- including inside the file name. A case directory named
// "foo_cases" would split at the wrong offset and send the worker to a directory
// that does not exist. No such case exists in the corpus (proved by
// TestSplitAgreesWithCTPOnTheWholeCorpus), so matching the path *segment* instead
// is identical on every real input and correct on the ones CTP would mishandle.
func Split(p string) (Case, error) {
	parts := strings.Split(p, "/")
	at := -1
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == "cases" {
			at = i
			break
		}
	}
	if at == -1 || at == len(parts)-1 {
		return Case{}, fmt.Errorf("no cases/ segment in %q", p)
	}
	dir := strings.Join(parts[:at+1], "/")
	script := strings.Join(parts[at+1:], "/")
	return Case{
		Path:   p,
		Dir:    dir,
		Script: script,
		Result: strings.TrimSuffix(script, ".sh") + ".result",
	}, nil
}

// findAll is CTP's discovery command with the awk predicate removed: the
// filtering happens in Go so that IsCase is the single statement of the rule.
func findAll(dir string) string {
	return fmt.Sprintf("find %s -name \"*.sh\" -type f -print", dir)
}

// Discover lists the cases under workspace, on whichever machine ch reaches.
//
// The order is find's order, which is readdir order, which is not reproducible:
// three consecutive runs over the same tree give three different orders. CTP
// dispatched in exactly that order, so dispatch_tc_ALL.txt was never byte-stable
// across runs of CTP itself. We sort instead -- the set is the contract, the order
// was never one, and a sorted list makes two runs comparable.
//
// docs/evidence/spec-corrections.md 11.
func Discover(ctx context.Context, ch exec.Channel, workspace string) ([]string, error) {
	res, err := runIn(ctx, ch, findAll(workspace))
	if err != nil {
		return nil, fmt.Errorf("discover cases under %s: %w", workspace, err)
	}
	var cases []string
	for _, line := range strings.Split(res.Output(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !IsCase(line) {
			continue
		}
		cases = append(cases, line)
	}
	slices.Sort(cases)
	return cases, nil
}

// ParseSkipped reads the output of grep <key> over every case file and returns the
// cases whose matching line starts with the key once whitespace is stripped.
//
// A line is only considered when it splits into exactly two colon-separated
// fields. That is CTP's rule and it has a consequence worth knowing: a skip macro
// whose text contains a colon produces three fields and is silently ignored, so
// the case runs.
func ParseSkipped(grepOutput, key string) []string {
	if key == "" {
		return nil
	}
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(grepOutput, "\r", "\n"), "\n") {
		fields := strings.Split(line, ":")
		// Java's split drops trailing empty fields; ours does not, so a line
		// ending in ":" must be rejected the same way.
		for len(fields) > 0 && fields[len(fields)-1] == "" {
			fields = fields[:len(fields)-1]
		}
		if len(fields) != 2 {
			continue
		}
		text := strings.ReplaceAll(strings.ReplaceAll(fields[1], "\t", " "), " ", "")
		if strings.HasPrefix(text, key) {
			out = append(out, strings.TrimSpace(fields[0]))
		}
	}
	return out
}

// ParseExcluded reads an exclusion file. Blank lines and lines starting with #
// or -- are comments. Every entry gets a trailing slash so that a directory name
// cannot match a longer name that merely starts with it.
func ParseExcluded(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "--") {
			continue
		}
		if !strings.HasSuffix(line, "/") {
			line += "/"
		}
		out = append(out, line)
	}
	return out
}

// Exclude removes every case matching any pattern and returns what is left along
// with what was taken out. A case matches when the pattern appears anywhere in its
// path, with a slash appended so that the last segment can be matched whole.
//
// The removed list is ordered the way CTP printed it: per pattern, and within a
// pattern from the end of the case list backwards.
func Exclude(cases, patterns []string) (kept, removed []string) {
	drop := map[int]bool{}
	for _, pat := range patterns {
		for j := len(cases) - 1; j >= 0; j-- {
			if drop[j] {
				continue
			}
			if strings.Contains(cases[j]+"/", pat) {
				drop[j] = true
				removed = append(removed, cases[j])
			}
		}
	}
	for i, c := range cases {
		if !drop[i] {
			kept = append(kept, c)
		}
	}
	return kept, removed
}

// Remove deletes the named cases from the list, preserving order, and reports
// which ones were actually present.
func Remove(cases, unwanted []string) (kept, removed []string) {
	drop := map[string]bool{}
	for _, u := range unwanted {
		drop[u] = true
	}
	for _, c := range cases {
		if drop[c] {
			removed = append(removed, c)
			continue
		}
		kept = append(kept, c)
	}
	return kept, removed
}
