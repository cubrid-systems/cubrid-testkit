package isolationsuite

import (
	"slices"
	"strings"
)

// verdict is what Test.runTestCase decided from what runone.sh printed.
type verdict struct {
	ok      bool
	hasCore bool
	// items are the result lines, " : NOK <why>", written to the worker log and
	// to feedback.
	items []string
}

const (
	okMarker    = "flag: OK"
	nokMarker   = "flag: NOK"
	coreMarker  = "found core file"
	fatalMarker = "found fatal error"
)

// judge is Test.runTestCase's rule (Test.java:195-218), applied to runone.sh's
// whole output.
//
// It reads positions, not lines, and the output is mostly runone.sh's own
// `set -x` trace -- which is worth knowing when the rule looks wrong. The trace
// of the retry loop's `grep 'flag: OK' .test.log` contains "flag: OK" after every
// attempt, so a failed attempt counts as a failure only because `cat .test.log`
// comes after it and prints "flag: NOK" later still. A retry that passes prints
// "flag: OK" last. The rule works on that order, and it is runone.sh's order that
// makes it work.
func judge(output string) verdict {
	v := verdict{ok: true}
	lastNOK := strings.LastIndex(output, nokMarker)
	lastOK := strings.LastIndex(output, okMarker)
	if lastNOK > lastOK {
		v.ok = false
	}

	errs := append(extractItems(output, coreMarker), extractItems(output, fatalMarker)...)
	if len(errs) > 0 {
		v.ok = false
		v.hasCore = true
		for _, e := range errs {
			v.items = append(v.items, item("NOK", e))
		}
	}

	if v.ok && lastOK == -1 {
		v.ok = false
		v.items = append(v.items, item("NOK", "Not found OK word."))
	}
	return v
}

// item is Test.addResultItem.
func item(flag, msg string) string { return " : " + flag + " " + msg }

// extractItems is Test.extractItems: from each occurrence of find to the end of
// its line, trimmed, with every single quote removed, and each distinct line
// once. The quotes go because the trace repeats the same message quoted.
func extractItems(source, find string) []string {
	var out []string
	start := 0
	for {
		i := strings.Index(source[start:], find)
		if i == -1 {
			return out
		}
		start += i
		end := strings.IndexByte(source[start:], '\n')
		var line string
		if end == -1 {
			line = source[start:]
		} else {
			end += start
			line = source[start:end]
		}
		line = strings.TrimSpace(strings.ReplaceAll(strings.TrimSpace(line), "'", ""))
		if !slices.Contains(out, line) {
			out = append(out, line)
		}
		if end == -1 {
			return out
		}
		start = end
	}
}

// diffBanner separates a failed case's result lines from its diff in feedback.
// Its width is part of the frozen output (external-surface-freeze.md §5-4).
const diffBanner = "=================================================================== D I F F ==================================================================="

// resultText is the result content Test.runAll hands to feedback: the items, and
// for a failed case the banner and the diff. Every piece ends with a newline,
// which is where the blank lines between cases in feedback.log come from.
func resultText(v verdict, diff string) string {
	var b strings.Builder
	for _, it := range v.items {
		b.WriteString(it)
		b.WriteByte('\n')
	}
	if !v.ok {
		b.WriteString(diffBanner)
		b.WriteByte('\n')
		b.WriteString(diff)
		b.WriteByte('\n')
	}
	return b.String()
}

// crlf is what the worker does to the diff before handing it on: a diff that
// has no Windows line ending gets one on every line (Test.java:168-170).
func crlf(diff string) string {
	if strings.Contains(diff, "\r\n") {
		return diff
	}
	return strings.ReplaceAll(diff, "\n", "\r\n")
}
