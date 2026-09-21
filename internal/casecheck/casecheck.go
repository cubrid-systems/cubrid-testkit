// Package casecheck reads a corpus and reports cases that cannot fail.
//
// A shell case decides its own verdict, and the rule the runner applies is a
// substring: a case fails when any line of its `.result` contains NOK. So a case
// whose failure path cannot be reached does not report a failure — it reports a
// pass, and a green verdict from a case that was incapable of red is worse than
// no case at all, because it occupies the place where coverage is believed to be.
//
// One was found by hand while reading the HA corpus: `_22_ha/bug_xdbms3769`
// calls `wirte_nok` where it means `write_nok`, so the branch that reports its
// third comparison failing is a command that does not exist. Nothing looked for
// more, which is what this package is (`design/module-ha.md` P7).
//
// It reads the corpus and the helper library and runs no case. There is no
// engine, no database and no machine involved, which is the point: this is the
// one check that costs a second and can be run on a laptop against a checkout.
package casecheck

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Rule names a check. They are separate because they fail differently: a case
// that cannot fail is a gap in coverage, a misspelt verdict call is a typo that
// will also print "command not found", and a self-comparison is a test that
// asserts nothing. A reader triaging a report needs to know which.
const (
	// RuleCannotFail is a case with no path to putting NOK in its result.
	RuleCannotFail = "cannot-fail"
	// RuleMisspeltVerdict is a call one edit away from a verdict helper.
	RuleMisspeltVerdict = "misspelt-verdict"
	// RuleSelfComparison is a comparison of something against itself.
	RuleSelfComparison = "self-comparison"
)

// Finding is one thing wrong with one case.
type Finding struct {
	Case   string
	Rule   string
	Line   int
	Detail string
}

func (f Finding) String() string {
	at := f.Case
	if f.Line > 0 {
		at = fmt.Sprintf("%s:%d", f.Case, f.Line)
	}
	return fmt.Sprintf("%-18s %s: %s", f.Rule, at, f.Detail)
}

// Helpers is what the helper library offers: every function it defines, and the
// subset that can put NOK into a result.
type Helpers struct {
	// All is every function name the library defines.
	All map[string]bool
	// Failing is the subset a case can reach a failure through.
	Failing map[string]bool
}

// declaration matches a function at the start of a line, in either of the two
// forms this library uses: `function name` and `name()`.
var declaration = regexp.MustCompile(`^(?:function[ \t]+([A-Za-z_][A-Za-z0-9_]*)|([A-Za-z_][A-Za-z0-9_]*)[ \t]*\(\))`)

// word matches an identifier anywhere. Used for "can this case fail", where
// over-approximating is the safe direction: counting a mention as a call can
// only make this package report fewer cases as unable to fail.
var word = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)

// commandSep splits a line where a new command can begin. Shell is not parsed
// and does not need to be -- this is the list of places a call can start.
var commandSep = regexp.MustCompile("[;&|`(){}]|\\$\\(|\\b(?:then|else|elif|do|fi|done)\\b")

// commandWords is the identifiers in command position, which is where a call
// actually is.
//
// The opposite direction from word, and for the opposite reason: reporting a
// typo is an accusation, so it has to be under-approximate. Two false positives
// came out of not doing this, both from matching an identifier anywhere --
// `db_stats=` is an assignment and `exec_csql.exp` is a filename handed to
// expect, and neither is a call to anything.
func commandWords(code string) []string {
	var out []string
	for _, seg := range commandSep.Split(code, -1) {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		head, _, _ := strings.Cut(seg, " ")
		head, _, _ = strings.Cut(head, "\t")
		// An assignment is not a call, a variable is not a call, and a file name
		// that begins with an identifier is not a call to that identifier.
		if strings.ContainsAny(head, "=$.\"'/") {
			continue
		}
		if m := word.FindString(head); m == head && m != "" {
			out = append(out, m)
		}
	}
	return out
}

// ReadHelpers parses the helper library a case is given.
//
// The set is derived rather than listed. A list in this repository would go
// stale the moment the library gained a helper -- which is exactly how the
// shipped-patch guards went quiet for ten commits when a directory was renamed.
// The library is the authority on what it offers, so it is read.
func ReadHelpers(dir string) (*Helpers, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("helper library %s: %w", dir, err)
	}
	h := &Helpers{All: map[string]bool{}, Failing: map[string]bool{}}
	// name -> the words its body mentions, for the transitive pass below.
	calls := map[string][]string{}
	direct := map[string]bool{}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sh") {
			continue
		}
		b, rerr := os.ReadFile(filepath.Join(dir, e.Name()))
		if rerr != nil {
			return nil, fmt.Errorf("helper %s: %w", e.Name(), rerr)
		}
		fn := ""
		for _, line := range strings.Split(string(b), "\n") {
			if m := declaration.FindStringSubmatch(line); m != nil {
				fn = m[1] + m[2] // exactly one of the two groups matched
				h.All[fn] = true
				continue
			}
			if fn == "" {
				continue
			}
			// A body that writes NOK, either by calling write_nok or by printing
			// a line the substring rule will catch. Both are failure paths and
			// the corpus uses both -- ha_common.sh's waits echo "NOK: ..." and
			// never call the helper.
			if strings.Contains(line, "write_nok") || strings.Contains(line, "NOK") {
				direct[fn] = true
			}
			calls[fn] = append(calls[fn], word.FindAllString(line, -1)...)
		}
	}
	// write_nok is the root even if the library is read without it.
	direct["write_nok"] = true
	h.All["write_nok"] = true

	// Transitive: a helper that calls a failing helper is a failing helper.
	// compare_result_between_files is the one that matters -- 169 HA cases reach
	// their verdict only through it.
	for k := range direct {
		h.Failing[k] = true
	}
	for changed := true; changed; {
		changed = false
		for fn, ws := range calls {
			if h.Failing[fn] {
				continue
			}
			for _, w := range ws {
				if w != fn && h.Failing[w] {
					h.Failing[fn] = true
					changed = true
					break
				}
			}
		}
	}
	return h, nil
}

// verdictNames are the calls a typo in is silent. A misspelt `csql` prints an
// error and the case fails loudly; a misspelt `write_nok` removes the only thing
// that would have said so.
func (h *Helpers) verdictNames() []string {
	out := []string{"write_ok", "write_nok"}
	for fn := range h.Failing {
		out = append(out, fn)
	}
	sort.Strings(out)
	return out
}

// Check reads one case and returns what is wrong with it.
func (h *Helpers) Check(path, body string) []Finding {
	var out []Finding
	lines := strings.Split(body, "\n")

	// Functions the case defines itself count as its own, so a case that wraps
	// its verdict in a local function is not reported.
	own := map[string]bool{}
	for _, line := range lines {
		if m := declaration.FindStringSubmatch(line); m != nil {
			own[m[1]+m[2]] = true
		}
	}

	canFail := false
	for i, line := range lines {
		code := strip(line)
		if code == "" {
			continue
		}
		// A case may print its own NOK line, which the substring rule catches.
		if strings.Contains(code, "NOK") {
			canFail = true
		}
		for _, w := range word.FindAllString(code, -1) {
			if h.Failing[w] || own[w] {
				canFail = true
			}
		}
		out = append(out, h.misspelt(path, i+1, code, own)...)
		out = append(out, selfComparison(path, i+1, code)...)
	}
	if !canFail {
		out = append(out, Finding{Case: path, Rule: RuleCannotFail,
			Detail: "no call reaches write_nok and the case prints no NOK line, so its result cannot contain one"})
	}
	return out
}

// misspelt reports a word one typo away from a verdict call that is not itself
// anything the case can call.
//
// One edit, with transposition counted as one: `wirte_nok` is two substitutions
// away from `write_nok` and one transposition, and transposition is the typo
// people actually make. Two edits was tried and matched ordinary words.
func (h *Helpers) misspelt(path string, line int, code string, own map[string]bool) []Finding {
	var out []Finding
	for _, w := range commandWords(code) {
		if h.All[w] || own[w] || len(w) < 5 {
			continue
		}
		for _, v := range h.verdictNames() {
			if damerau(w, v) == 1 {
				out = append(out, Finding{Case: path, Rule: RuleMisspeltVerdict, Line: line,
					Detail: fmt.Sprintf("%q is one typo from %q and is defined nowhere, so this line reports nothing", w, v)})
				break
			}
		}
	}
	return out
}

// compareCall matches a two-argument comparison helper.
var compareCall = regexp.MustCompile(`\b(compare_result_between_files|compare_result)\b[ \t]+(\S+)[ \t]+(\S+)`)

// selfComparison reports a comparison whose two sides are written the same.
func selfComparison(path string, line int, code string) []Finding {
	m := compareCall.FindStringSubmatch(code)
	if m == nil || m[2] != m[3] {
		return nil
	}
	return []Finding{{Case: path, Rule: RuleSelfComparison, Line: line,
		Detail: fmt.Sprintf("%s compares %s against itself, which cannot differ", m[1], m[2])}}
}

// strip removes a comment, so a verdict call inside one does not count as a
// failure path. Quoting is not tracked: a `#` inside a string is rare in this
// corpus and treating it as a comment can only remove a would-be failure path,
// which reports more cases rather than fewer.
func strip(line string) string {
	if i := strings.Index(line, "#"); i >= 0 {
		line = line[:i]
	}
	return strings.TrimSpace(line)
}

// damerau is the edit distance with transposition, bounded: anything past 2 is
// reported as 3, because only 1 is asked about.
func damerau(a, b string) int {
	if a == b {
		return 0
	}
	if abs(len(a)-len(b)) > 1 {
		return 3
	}
	prev2 := make([]int, len(b)+1)
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min3(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				cur[j] = min(cur[j], prev2[j-2]+1)
			}
		}
		copy(prev2, prev)
		copy(prev, cur)
	}
	return prev[len(b)]
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func min3(a, b, c int) int { return min(a, min(b, c)) }
