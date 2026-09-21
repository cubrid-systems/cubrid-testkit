package hareplsuite

import "strings"

// Statements splits a case into the statements csql would execute.
//
// Deliberately small, and it refuses rather than guesses. CTP has a real reader
// for this (`TestReader.readOneStatement`); this one exists because the oracle
// has to be applied *between* statements and a case is a file. What it handles
// is a corpus of semicolon-terminated statements with string literals and
// comments in them. What it does not handle is a body whose semicolons are not
// terminators -- a PL/CSQL procedure, a trigger -- and Splittable says so
// instead of producing fragments that would run as nonsense.
func Statements(src string) []string {
	var out []string
	var cur strings.Builder
	var quote rune
	inLine, inBlock := false, false
	prev := rune(0)

	flush := func() {
		if s := strings.TrimSpace(cur.String()); s != "" {
			out = append(out, s)
		}
		cur.Reset()
	}
	for _, r := range src {
		switch {
		case inLine:
			if r == '\n' {
				inLine = false
				cur.WriteRune(r)
			}
		case inBlock:
			if prev == '*' && r == '/' {
				inBlock = false
			}
		case quote != 0:
			cur.WriteRune(r)
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"':
			quote = r
			cur.WriteRune(r)
		case r == '-' && prev == '-':
			// The '-' already written belongs to the comment, not the statement.
			s := cur.String()
			cur.Reset()
			cur.WriteString(strings.TrimSuffix(s, "-"))
			inLine = true
		case r == '*' && prev == '/':
			s := cur.String()
			cur.Reset()
			cur.WriteString(strings.TrimSuffix(s, "/"))
			inBlock = true
		case r == ';':
			flush()
		default:
			cur.WriteRune(r)
		}
		prev = r
	}
	flush()
	return out
}

// blockBodies are the constructs whose semicolons are not statement
// terminators. A case containing one is not split.
var blockBodies = []string{
	"create procedure", "create function", "create or replace procedure",
	"create or replace function", "create trigger", "create or replace trigger",
}

// Splittable reports whether Statements can be trusted on this case.
//
// The safe direction is to under-claim: a case reported unsplittable is
// skipped and named, which costs coverage. A case wrongly split runs
// fragments, and the verdict that comes back is about nothing.
func Splittable(src string) bool {
	low := strings.ToLower(src)
	for _, b := range blockBodies {
		if strings.Contains(low, b) {
			return false
		}
	}
	return true
}

// IsRead reports whether a statement's output is worth comparing across the
// pair. It is the oracle's trigger, so it under-claims too: a statement this
// calls a write is simply not compared.
func IsRead(stmt string) bool {
	s := strings.ToLower(strings.TrimSpace(stmt))
	for _, p := range []string{"select ", "select\n", "select\t", "show ", "values "} {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return s == "select" || strings.HasPrefix(s, "select(")
}

// IsWrite reports whether a statement may have changed something the slave has
// to receive. Over-claims on purpose: an unnecessary wait costs a second, and a
// missed one compares a slave that was never given the chance to catch up.
func IsWrite(stmt string) bool {
	return !IsRead(stmt) && !isDirective(stmt)
}

// isDirective is csql's own vocabulary, which is not SQL and reaches no node's
// data: the holdcas pragma the sql corpus wraps its bodies in, and autocommit.
func isDirective(stmt string) bool {
	s := strings.ToLower(strings.TrimSpace(stmt))
	return strings.HasPrefix(s, "--+") ||
		strings.HasPrefix(s, "autocommit ") ||
		strings.HasPrefix(s, "commit") ||
		strings.HasPrefix(s, "rollback")
}
