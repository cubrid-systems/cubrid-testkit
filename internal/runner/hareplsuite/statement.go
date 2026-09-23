package hareplsuite

import "strings"

// Statements splits a case into the statements csql would execute.
//
// Deliberately small, and it refuses rather than guesses. It exists because
// the oracle has to be applied *between* statements and a case is a file.
//
// It handles semicolon-terminated statements with string literals and comments
// in them, and PL/CSQL block bodies, whose semicolons are not terminators. The
// first version refused every case containing the words "create trigger",
// which turned out to reject cases that split perfectly well: a trigger
// without a block body is one statement, `create trigger t ... execute insert
// into t2 values (obj.c1);`. Fifteen of 131 cases were skipped for that.
func Statements(src string) []string {
	var out []string
	var cur strings.Builder
	var quote rune
	inLine, inBlock := false, false
	prev := rune(0)
	depth := 0 // open PL/CSQL blocks

	flush := func() {
		if s := strings.TrimSpace(cur.String()); s != "" {
			out = append(out, s)
		}
		cur.Reset()
	}
	runes := []rune(src)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
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
			if depth > 0 {
				cur.WriteRune(r)
			} else {
				flush()
			}
		default:
			if d, skip := blockDelta(runes, i); d != 0 {
				depth += d
				if depth < 0 {
					depth = 0
				}
				cur.WriteString(string(runes[i : i+skip]))
				i += skip - 1
				prev = runes[i]
				continue
			}
			cur.WriteRune(r)
		}
		prev = r
	}
	flush()
	return out
}

// blockDelta reports whether a PL/CSQL block opens or closes at this position,
// and how many runes the keyword took.
//
// BEGIN opens one. END closes one only when it ends the block rather than a
// construct: `END IF`, `END LOOP`, `END CASE` and `END WHILE` are not block
// ends, and neither is the END of a CASE expression, which is followed by
// something other than a semicolon or a name. Getting this wrong in the
// permissive direction would run half a procedure as a statement, so the rule
// is written to close a block only when it can see the close.
func blockDelta(r []rune, i int) (int, int) {
	if i > 0 && isWordRune(r[i-1]) {
		return 0, 0
	}
	if kw, n := wordAt(r, i); kw != "" {
		switch kw {
		case "BEGIN":
			return 1, n
		case "END":
			next, _ := wordAt(r, skipSpace(r, i+n))
			switch next {
			case "IF", "LOOP", "CASE", "WHILE", "FOR":
				return 0, 0
			}
			// `END;` or `END <name>;` closes the block. Anything else -- the
			// END of a CASE expression -- does not.
			j := skipSpace(r, i+n)
			if j < len(r) && r[j] == ';' {
				return -1, n
			}
			if next != "" {
				k := skipSpace(r, j+len(next))
				if k < len(r) && r[k] == ';' {
					return -1, n
				}
			}
		}
	}
	return 0, 0
}

func wordAt(r []rune, i int) (string, int) {
	j := i
	for j < len(r) && isWordRune(r[j]) {
		j++
	}
	if j == i {
		return "", 0
	}
	return strings.ToUpper(string(r[i:j])), j - i
}

func skipSpace(r []rune, i int) int {
	for i < len(r) && (r[i] == ' ' || r[i] == '\t' || r[i] == '\n' || r[i] == '\r') {
		i++
	}
	return i
}

func isWordRune(r rune) bool {
	return r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// Splittable reports whether Statements can be trusted on this case.
//
// Now that blocks are tracked rather than refused, the only thing left to
// refuse is a case the reader could not finish: a BEGIN whose END it never
// saw. The last statement would then be the rest of the file, and running that
// produces a verdict about nothing. The safe direction is still to under-claim.
func Splittable(src string) bool {
	stmts := Statements(src)
	if len(stmts) == 0 {
		return true
	}
	last := strings.ToUpper(stmts[len(stmts)-1])
	return !strings.Contains(last, "BEGIN") || strings.Contains(last, "END")
}

// IsRead reports whether a statement's output is worth comparing across the
// pair. It is the oracle's trigger, so it under-claims too: a statement this
// calls a write is simply not compared.
//
// # A SELECT that takes a serial's next value is not a read
//
// `SELECT s.next_value FROM db_root` and `SELECT serial_next_value(s, 1)`
// advance the serial. Running one on the master and the same text on the slave
// is therefore not asking the same question twice; it is asking for a write on
// a node that will not take one. Measured over `_01_object`: five cases of
// `_05_serial` came back `differ` with a value on the master and an empty
// answer on the standby, and not one of them was about replication.
//
// So they are writes here, which is what they are: run on the master, waited
// for, and not compared. `current_value` on its own is left comparable -- it
// reads what replication carried -- and a statement that asks for both in one
// line is a write, because the line as a whole moves the serial.
func IsRead(stmt string) bool {
	s := strings.ToLower(strings.TrimSpace(stmt))
	if strings.Contains(s, "next_value") || strings.Contains(s, "nextval") {
		return false
	}
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
