package hareplsuite

import "strings"

// KeyColumn is the column the conversion adds. Named so it is obvious in a
// `select *` that it was not the case's idea.
const KeyColumn = "tk_repl_key"

// Conversion gives a case's tables the primary key CUBRID's HA needs and the
// case did not think to write, and keeps the case's own statements working
// around it.
//
// # Why not the first column
//
// CTP's ha_repl migration makes the first column the primary key
// (`migrate/SQLFileReader.java`, since 2012). That was tried here first and
// measured, over `_06_manipulation/_04_insert`: it raised comparable reads
// from 4 to 32, and it also made the engine refuse **20 writes it had
// accepted before** -- 14 on a duplicate first column, 6 on a NULL one. A
// refused INSERT leaves the table empty on both nodes, and two empty tables
// agree, so seven reads "passed" on nothing where one had before.
//
// A corpus of tests is exactly where duplicate and NULL columns live. So the
// key is a column of its own, `tk_repl_key INT AUTO_INCREMENT PRIMARY KEY`,
// which no data can violate. Measured on the pair: it accepts the rows the
// first-column key refused, and the generated values are identical on both
// nodes, because the value is generated on the master and replicated with the
// row.
//
// # What it costs, and why the cost is paid here
//
// An extra column breaks `insert into t values (...)`, which is most of the
// corpus -- which is why CTP did not do it. That objection is answered rather
// than accepted: the conversion knows the column names, because it just read
// them off the CREATE, so it writes them into the INSERT. `insert into t
// values (1,2)` becomes `insert into t(c1,c2) values (1,2)`, and the same for
// `insert into t select ...`.
//
// A `select *` then returns the extra column -- on both nodes, with the same
// value, so the comparison still means what it meant.
type Conversion struct {
	// cols is the column list of each table this conversion changed.
	cols map[string][]string
}

// NewConversion starts one. It is per case: a table's shape is the case's.
func NewConversion() *Conversion { return &Conversion{cols: map[string][]string{}} }

// Apply converts one statement, and reports whether it changed it.
func (c *Conversion) Apply(stmt string) (string, bool) {
	if out, ok := c.convertCreate(stmt); ok {
		return out, true
	}
	return c.nameColumns(stmt)
}

// convertCreate appends the key column to a CREATE TABLE that has none.
func (c *Conversion) convertCreate(stmt string) (string, bool) {
	if !isCreateTable(stmt) || strings.Contains(strings.ToUpper(stmt), "PRIMARY KEY") {
		return stmt, false
	}
	// A subclass inherits its parent's columns, including the key this
	// conversion gave the parent, so it must not be given one of its own --
	// and its positional inserts carry the parent's columns first, which the
	// parent's entry alone cannot say.
	//
	// Found by the counter rather than by thinking: `select * from sub_t1`
	// was the one read agreeing about nothing that `--[er]` did not explain,
	// and the reason was that this conversion had broken the case.
	if parent := subclassOf(stmt); parent != "" {
		inherited, ok := c.cols[strings.ToLower(parent)]
		if !ok {
			return stmt, false // the parent was not converted, so nothing changes
		}
		table := tableNameOf(stmt)
		if table == "" {
			return stmt, false
		}
		own := []string{}
		if open := indexAtDepth(stmt, '(', 0); open >= 0 {
			inner := stmt[open+1:]
			if close := indexAtDepth(inner, ')', 0); close >= 0 {
				own = columnNames(inner[:close])
			}
		}
		c.cols[strings.ToLower(table)] = append(append([]string{}, inherited...), own...)
		return stmt, false
	}
	open := indexAtDepth(stmt, '(', 0)
	if open < 0 {
		return stmt, false // CREATE TABLE ... AS SELECT has no column list
	}
	inner := stmt[open+1:]
	close := indexAtDepth(inner, ')', 0)
	if close < 0 {
		return stmt, false
	}
	table := tableNameOf(stmt)
	if table == "" {
		return stmt, false
	}
	cols := columnNames(inner[:close])
	if len(cols) == 0 {
		return stmt, false
	}
	c.cols[strings.ToLower(table)] = cols
	at := open + 1 + close
	return stmt[:at] + ", " + KeyColumn + " INT AUTO_INCREMENT PRIMARY KEY" + stmt[at:], true
}

// nameColumns writes the column list into an INSERT that relied on position.
//
// Every occurrence, because the corpus nests them: `insert into employees
// values(1001, 10, (insert into person2 values (...)))` is one statement with
// two inserts in it, and both tables may have been converted.
func (c *Conversion) nameColumns(stmt string) (string, bool) {
	if len(c.cols) == 0 {
		return stmt, false
	}
	r := []rune(stmt)
	var out strings.Builder
	changed := false
	for i := 0; i < len(r); {
		table, valuesAt, n := insertHeadAt(r, i)
		if n == 0 {
			out.WriteRune(r[i])
			i++
			continue
		}
		cols, ok := c.cols[strings.ToLower(table)]
		if !ok {
			out.WriteString(string(r[i : i+n]))
			i += n
			continue
		}
		out.WriteString(string(r[i:valuesAt]))
		out.WriteString("(" + strings.Join(cols, ", ") + ") ")
		i = valuesAt
		changed = true
	}
	return out.String(), changed
}

// insertHeadAt recognises `INSERT INTO <name>` followed directly by VALUES or
// SELECT -- the forms that rely on column position. It returns the table, the
// offset of the VALUES or SELECT, and the length consumed.
//
// A form that already names its columns, or sets them by name, is left alone:
// it is already correct around an extra column.
func insertHeadAt(r []rune, i int) (string, int, int) {
	if i > 0 && isWordRune(r[i-1]) {
		return "", 0, 0
	}
	kw, n := wordAt(r, i)
	if kw != "INSERT" && kw != "REPLACE" {
		return "", 0, 0
	}
	j := skipSpace(r, i+n)
	if kw2, n2 := wordAt(r, j); kw2 == "INTO" {
		j = skipSpace(r, j+n2)
	} else if kw == "REPLACE" {
		return "", 0, 0
	}
	name, nn := wordAt(r, j)
	if nn == 0 {
		return "", 0, 0
	}
	table := string(r[j : j+nn])
	k := skipSpace(r, j+nn)
	next, _ := wordAt(r, k)
	if next != "VALUES" && next != "SELECT" {
		return "", 0, 0 // already names its columns, or is a SET form
	}
	_ = name
	return table, k, k - i
}

// tableNameOf is the identifier after CREATE ... TABLE|CLASS.
func tableNameOf(stmt string) string {
	r := []rune(stmt)
	for i := 0; i < len(r); {
		kw, n := wordAt(r, i)
		if n == 0 {
			i++
			continue
		}
		if kw == "TABLE" || kw == "CLASS" {
			j := skipSpace(r, i+n)
			// IF NOT EXISTS sits between the keyword and the name.
			for {
				w, wn := wordAt(r, j)
				if w == "IF" || w == "NOT" || w == "EXISTS" {
					j = skipSpace(r, j+wn)
					continue
				}
				break
			}
			name, nn := wordAt(r, j)
			_ = name
			if nn == 0 {
				return ""
			}
			return strings.Trim(string(r[j:j+nn]), "[]\"`")
		}
		i += n
		i = skipSpace(r, i)
	}
	return ""
}

// columnNames reads the names out of a column list, skipping the table-level
// constraints that share it.
func columnNames(list string) []string {
	var out []string
	for _, item := range splitTopLevel(list) {
		f := strings.Fields(strings.TrimSpace(item))
		if len(f) == 0 {
			continue
		}
		switch strings.ToUpper(f[0]) {
		case "PRIMARY", "FOREIGN", "UNIQUE", "INDEX", "KEY", "CONSTRAINT", "CHECK", "SHARED":
			continue
		}
		out = append(out, strings.Trim(f[0], "[]\"`,"))
	}
	return out
}

// splitTopLevel cuts a column list at the commas that separate its items,
// ignoring the ones inside a type or a literal.
func splitTopLevel(s string) []string {
	var out []string
	start := 0
	for {
		i := indexAtDepth(s[start:], ',', 0)
		if i < 0 {
			out = append(out, s[start:])
			return out
		}
		out = append(out, s[start:start+i])
		start += i + 1
	}
}

// subclassOf is the parent named by `... AS SUBCLASS OF <name>`, or "".
func subclassOf(stmt string) string {
	r := []rune(stmt)
	for i := 0; i < len(r); {
		kw, n := wordAt(r, i)
		if n == 0 {
			i++
			continue
		}
		if kw == "SUBCLASS" {
			j := skipSpace(r, i+n)
			if w, wn := wordAt(r, j); w == "OF" {
				j = skipSpace(r, j+wn)
				if _, nn := wordAt(r, j); nn > 0 {
					return strings.Trim(string(r[j:j+nn]), "[]\"`")
				}
			}
			return ""
		}
		i = skipSpace(r, i+n)
	}
	return ""
}

func isCreateTable(stmt string) bool {
	s := strings.ToUpper(strings.TrimSpace(stmt))
	if !strings.HasPrefix(s, "CREATE") {
		return false
	}
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
