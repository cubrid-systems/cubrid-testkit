package hareplsuite

import "strings"

// AddPrimaryKey makes a CREATE TABLE replicate, by giving it the key CUBRID's
// HA needs and the case did not think to write.
//
// # The rule, and where it comes from
//
// **The first column becomes the primary key.** That is CTP's rule -- its
// ha_repl conversion has done this since 2012 (`migrate/SQLFileReader.java`)
// -- and it is reused because the alternatives are worse, not because it is
// obviously right:
//
//   - A synthetic extra column would break every positional
//     `insert into t values (...)` in the corpus, which is most of them.
//   - Choosing a column by type or by name guesses at intent and would differ
//     between the two things that convert the same corpus.
//
// What it costs is stated rather than hidden: a first column with duplicates
// or NULLs makes the INSERT fail, and the case then establishes nothing.
// That is a case the conversion cannot save, and the runner counts it.
//
// # Why this is not CTP's implementation
//
// CTP's is eighty lines of comparing the positions of '(' ')' and ',' inside
// one line of text, with a branch per arrangement. It cannot see a column list
// that spans lines, and it has no notion of a parenthesis inside a type. This
// one walks the statement once, tracking quotes and depth, which is the same
// machinery Statements already needs.
func AddPrimaryKey(stmt string) (string, bool) {
	if !isCreateTable(stmt) || strings.Contains(strings.ToUpper(stmt), "PRIMARY KEY") {
		return stmt, false
	}
	// A CREATE TABLE AS SELECT has no column list to amend.
	open := indexAtDepth(stmt, '(', 0)
	if open < 0 {
		return stmt, false
	}
	// The first column definition ends at the first comma directly inside the
	// list, or at its closing parenthesis when the table has one column. The
	// search runs on the text after the open parenthesis, so "directly inside"
	// is depth 0 there. Depth matters: `col1 numeric(10,2), col2 int` has a
	// comma inside the type and it is not the end of a column.
	inner := stmt[open+1:]
	end := indexAtDepth(inner, ',', 0)
	if end < 0 {
		end = indexAtDepth(inner, ')', 0)
	}
	if end < 0 {
		return stmt, false
	}
	at := open + 1 + end
	col := strings.TrimRight(stmt[open+1:at], " \t\n\r")
	if strings.TrimSpace(col) == "" {
		return stmt, false
	}
	return stmt[:open+1] + col + " PRIMARY KEY" + stmt[at:], true
}

func isCreateTable(stmt string) bool {
	s := strings.ToUpper(strings.TrimSpace(stmt))
	if !strings.HasPrefix(s, "CREATE") {
		return false
	}
	// CREATE TABLE, CREATE CLASS, and their IF NOT EXISTS / OR REPLACE forms.
	return strings.Contains(s, " TABLE ") || strings.Contains(s, " CLASS ")
}

// indexAtDepth finds the first occurrence of c at exactly the given paren
// depth, ignoring anything inside a string literal. Depth is relative to the
// start of the text it is given, so a caller that has already stepped past an
// open parenthesis asks for depth 0.
func indexAtDepth(s string, c byte, want int) int {
	depth := 0
	var quote byte
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case quote != 0:
			if ch == quote {
				quote = 0
			}
			continue
		case ch == '\'' || ch == '"':
			quote = ch
			continue
		}
		if ch == c && depth == want {
			return i
		}
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
		}
	}
	return -1
}
