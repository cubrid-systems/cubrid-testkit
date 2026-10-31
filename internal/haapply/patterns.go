package haapply

import (
	"fmt"
	"sort"
	"strings"
)

// Pattern is one conflicting pair (ADR-EXT-012 3-4).
//
// Each round commits T1 -- ballast rows, then Round.T1, in one transaction --
// and then T2 -- Round.T2 and a witness row -- in another. T2 depends on what
// T1 wrote last, so a slave that applies T2 while T1's ballast is still
// missing has missed the dependency. Tables are named after the pattern, so
// two patterns never share one.
type Pattern struct {
	Name string
	// Shares is what T1 and T2 have in common, as `list` prints it.
	Shares string
	// Setup is the pattern's own DDL. The ballast and witness tables are the
	// runner's.
	Setup func(p string) string
	// Round is the statements of round r: Pre is committed and replicated
	// before the round, T1 follows the ballast, T2 the dependent operation.
	Round func(p string, r int) Round
	// TrailingSpace is a pattern whose keys are one key only on a pair that
	// compares VARCHAR without trailing spaces (ignore_trailing_space); on
	// another pair T2 may rightly go first.
	TrailingSpace bool
}

// Round is one round's statements, each a list ending in a semicolon.
type Round struct{ Pre, T1, T2 string }

func pk(p string) string {
	return fmt.Sprintf("CREATE TABLE t_%s (id INT PRIMARY KEY, v INT);", p)
}

func parentChild(p, action string) string {
	return fmt.Sprintf("CREATE TABLE parent_%[1]s (p INT PRIMARY KEY); "+
		"CREATE TABLE child_%[1]s (c INT PRIMARY KEY, p INT, "+
		"CONSTRAINT fk_%[1]s FOREIGN KEY (p) REFERENCES parent_%[1]s (p)%[2]s);", p, action)
}

// tz0 and tz9 are one instant per round, written in UTC and in +09:00.
func tz0(r int) string { return fmt.Sprintf("'2020-01-01 00:%02d:%02d +00:00'", r/60, r%60) }
func tz9(r int) string { return fmt.Sprintf("'2020-01-01 09:%02d:%02d +09:00'", r/60, r%60) }

// Catalog is every pattern, in the order a run takes them: the control first,
// then the keys the index holds equal while the writeset might not, then the
// prototype's integer-key patterns.
var Catalog = []Pattern{
	{Name: "ctl", Shares: "none (the control)", Setup: pk,
		Round: func(p string, r int) Round {
			return Round{T2: fmt.Sprintf("INSERT INTO t_%s VALUES (%d, %d);", p, r, r)}
		}},

	// T1 frees a key and T2 takes one the index holds equal to it but written
	// differently. A writeset that hashes the bytes, or the class, misses it.
	{Name: "fp_negzero", Shares: "UNIQUE (g, d): -0.0 and +0.0",
		Setup: func(p string) string {
			return fmt.Sprintf("CREATE TABLE t_%s (id INT PRIMARY KEY, g INT, d DOUBLE, UNIQUE (g, d));", p)
		},
		Round: func(p string, r int) Round {
			return Round{
				Pre: fmt.Sprintf("INSERT INTO t_%s VALUES (%d * 2, %d, -0e0);", p, r, r),
				T1:  fmt.Sprintf("DELETE FROM t_%s WHERE id = %d * 2;", p, r),
				T2:  fmt.Sprintf("INSERT INTO t_%s VALUES (%d * 2 + 1, %d, 0e0);", p, r, r),
			}
		}},
	tzPattern("tstz_zone", "TIMESTAMPTZ"),
	tzPattern("dttz_zone", "DATETIMETZ"),
	{Name: "ci_case", Shares: "utf8_en_ci UNIQUE: two cases",
		Setup: func(p string) string {
			return fmt.Sprintf("CREATE TABLE t_%s (id INT PRIMARY KEY, s VARCHAR(20) COLLATE utf8_en_ci UNIQUE);", p)
		},
		Round: func(p string, r int) Round {
			return Round{
				Pre: fmt.Sprintf("INSERT INTO t_%s VALUES (%d * 2, 'Key%d');", p, r, r),
				T1:  fmt.Sprintf("DELETE FROM t_%s WHERE id = %d * 2;", p, r),
				T2:  fmt.Sprintf("INSERT INTO t_%s VALUES (%d * 2 + 1, 'KEY%d');", p, r, r),
			}
		}},
	{Name: "inherit_uk", Shares: "UNIQUE of a superclass: subclass row, then superclass row",
		Setup: func(p string) string {
			return fmt.Sprintf("CREATE TABLE t_%[1]s (id INT PRIMARY KEY, uk INT UNIQUE); CREATE TABLE sub_%[1]s UNDER t_%[1]s;", p)
		},
		Round: func(p string, r int) Round {
			return Round{
				Pre: fmt.Sprintf("INSERT INTO sub_%s VALUES (%d * 2, %d);", p, r, r),
				T1:  fmt.Sprintf("DELETE FROM sub_%s WHERE uk = %d;", p, r),
				T2:  fmt.Sprintf("INSERT INTO t_%s VALUES (%d * 2 + 1, %d);", p, r, r),
			}
		}},
	// The same equalities as a reference: T1 adds the parent one way and T2's
	// child names it the other way.
	{Name: "fk_negzero", Shares: "FK (g, d): parent -0.0, child +0.0",
		Setup: func(p string) string {
			return fmt.Sprintf("CREATE TABLE parent_%[1]s (g INT, d DOUBLE, PRIMARY KEY (g, d)); "+
				"CREATE TABLE child_%[1]s (c INT PRIMARY KEY, g INT, d DOUBLE, "+
				"CONSTRAINT fk_%[1]s FOREIGN KEY (g, d) REFERENCES parent_%[1]s (g, d));", p)
		},
		Round: func(p string, r int) Round {
			return Round{
				T1: fmt.Sprintf("INSERT INTO parent_%s VALUES (%d, -0e0);", p, r),
				T2: fmt.Sprintf("INSERT INTO child_%s VALUES (%d, %d, 0e0);", p, r, r),
			}
		}},
	{Name: "fk_tz", Shares: "FK TIMESTAMPTZ: parent +00:00, child +09:00",
		Setup: func(p string) string {
			return fmt.Sprintf("CREATE TABLE parent_%[1]s (ts TIMESTAMPTZ PRIMARY KEY); "+
				"CREATE TABLE child_%[1]s (c INT PRIMARY KEY, ts TIMESTAMPTZ, "+
				"CONSTRAINT fk_%[1]s FOREIGN KEY (ts) REFERENCES parent_%[1]s (ts));", p)
		},
		Round: func(p string, r int) Round {
			return Round{
				T1: fmt.Sprintf("INSERT INTO parent_%s VALUES (TIMESTAMPTZ%s);", p, tz0(r)),
				T2: fmt.Sprintf("INSERT INTO child_%s VALUES (%d, TIMESTAMPTZ%s);", p, r, tz9(r)),
			}
		}},
	{Name: "fk_ci", Shares: "FK utf8_en_ci: parent 'Key', child 'KEY'",
		Setup: func(p string) string {
			return fmt.Sprintf("CREATE TABLE parent_%[1]s (s VARCHAR(20) COLLATE utf8_en_ci PRIMARY KEY); "+
				"CREATE TABLE child_%[1]s (c INT PRIMARY KEY, s VARCHAR(20) COLLATE utf8_en_ci, "+
				"CONSTRAINT fk_%[1]s FOREIGN KEY (s) REFERENCES parent_%[1]s (s));", p)
		},
		Round: func(p string, r int) Round {
			return Round{
				T1: fmt.Sprintf("INSERT INTO parent_%s VALUES ('Key%d');", p, r),
				T2: fmt.Sprintf("INSERT INTO child_%s VALUES (%d, 'KEY%d');", p, r, r),
			}
		}},
	// Under ignore_trailing_space the index compares VARCHAR without its
	// trailing spaces, so a value and the same with one are one key there.
	// CHAR has no such pair: it is padded to its width before it is written,
	// and 'Key' and 'Key ' are the same bytes everywhere.
	{Name: "vc_trail", Shares: "VARCHAR UNIQUE under ignore_trailing_space: 'Key' and 'Key '", TrailingSpace: true,
		Setup: func(p string) string {
			return fmt.Sprintf("CREATE TABLE t_%s (id INT PRIMARY KEY, s VARCHAR(20) UNIQUE);", p)
		},
		Round: func(p string, r int) Round {
			return Round{
				Pre: fmt.Sprintf("INSERT INTO t_%s VALUES (%d * 2, 'Key%d');", p, r, r),
				T1:  fmt.Sprintf("DELETE FROM t_%s WHERE id = %d * 2;", p, r),
				T2:  fmt.Sprintf("INSERT INTO t_%s VALUES (%d * 2 + 1, 'Key%d ');", p, r, r),
			}
		}},
	{Name: "fk_vc_trail", Shares: "FK VARCHAR under ignore_trailing_space: parent 'Key', child 'Key '", TrailingSpace: true,
		Setup: func(p string) string {
			return fmt.Sprintf("CREATE TABLE parent_%[1]s (s VARCHAR(20) PRIMARY KEY); "+
				"CREATE TABLE child_%[1]s (c INT PRIMARY KEY, s VARCHAR(20), "+
				"CONSTRAINT fk_%[1]s FOREIGN KEY (s) REFERENCES parent_%[1]s (s));", p)
		},
		Round: func(p string, r int) Round {
			return Round{
				T1: fmt.Sprintf("INSERT INTO parent_%s VALUES ('Key%d');", p, r),
				T2: fmt.Sprintf("INSERT INTO child_%s VALUES (%d, 'Key%d ');", p, r, r),
			}
		}},

	{Name: "pk_update", Shares: "PK", Setup: pk,
		Round: func(p string, r int) Round {
			return Round{
				Pre: fmt.Sprintf("INSERT INTO t_%s VALUES (%d, 0);", p, r),
				T1:  fmt.Sprintf("UPDATE t_%s SET v = 1 WHERE id = %d;", p, r),
				T2:  fmt.Sprintf("UPDATE t_%s SET v = 2 WHERE id = %d;", p, r),
			}
		}},
	{Name: "pk_move", Shares: "PK, the old value", Setup: pk,
		Round: func(p string, r int) Round {
			return Round{
				Pre: fmt.Sprintf("INSERT INTO t_%s VALUES (%d, 0);", p, r),
				T1:  fmt.Sprintf("UPDATE t_%s SET id = id + 100000000 WHERE id = %d;", p, r),
				T2:  fmt.Sprintf("INSERT INTO t_%s VALUES (%d, 1);", p, r),
			}
		}},
	{Name: "uk_reinsert", Shares: "UNIQUE",
		Setup: func(p string) string { return fmt.Sprintf("CREATE TABLE t_%s (id INT PRIMARY KEY, uk INT UNIQUE);", p) },
		Round: reinsert},
	{Name: "uk_move", Shares: "UNIQUE, the old value",
		Setup: func(p string) string { return fmt.Sprintf("CREATE TABLE t_%s (id INT PRIMARY KEY, uk INT UNIQUE);", p) },
		Round: func(p string, r int) Round {
			return Round{
				Pre: fmt.Sprintf("INSERT INTO t_%[1]s VALUES (%[2]d * 2, %[2]d); INSERT INTO t_%[1]s VALUES (%[2]d * 2 + 1, -%[2]d);", p, r),
				T1:  fmt.Sprintf("UPDATE t_%s SET uk = uk + 100000000 WHERE id = %d * 2;", p, r),
				T2:  fmt.Sprintf("UPDATE t_%s SET uk = %d WHERE id = %d * 2 + 1;", p, r, r),
			}
		}},
	{Name: "fk_insert", Shares: "FK as REF", Setup: func(p string) string { return parentChild(p, "") },
		Round: func(p string, r int) Round {
			return Round{
				T1: fmt.Sprintf("INSERT INTO parent_%s VALUES (%d);", p, r),
				T2: fmt.Sprintf("INSERT INTO child_%s VALUES (%d, %d);", p, r, r),
			}
		}},
	{Name: "fk_delete", Shares: "FK as REF", Setup: func(p string) string { return parentChild(p, "") },
		Round: func(p string, r int) Round {
			return Round{
				Pre: fmt.Sprintf("INSERT INTO parent_%[1]s VALUES (%[2]d); INSERT INTO child_%[1]s VALUES (%[2]d, %[2]d);", p, r),
				T1:  fmt.Sprintf("DELETE FROM child_%s WHERE c = %d;", p, r),
				T2:  fmt.Sprintf("DELETE FROM parent_%s WHERE p = %d;", p, r),
			}
		}},
	{Name: "rk_reinsert", Shares: "REVERSE UNIQUE",
		Setup: func(p string) string {
			return fmt.Sprintf("CREATE TABLE t_%[1]s (id INT PRIMARY KEY, uk INT); CREATE REVERSE UNIQUE INDEX rk_%[1]s ON t_%[1]s (uk);", p)
		},
		Round: reinsert},
	{Name: "fk_composite", Shares: "composite FK",
		Setup: func(p string) string {
			return fmt.Sprintf("CREATE TABLE parent_%[1]s (a INT, b INT, PRIMARY KEY (a, b)); "+
				"CREATE TABLE child_%[1]s (c INT PRIMARY KEY, a INT, b INT, "+
				"CONSTRAINT fk_%[1]s FOREIGN KEY (a, b) REFERENCES parent_%[1]s (a, b));", p)
		},
		Round: func(p string, r int) Round {
			return Round{
				T1: fmt.Sprintf("INSERT INTO parent_%s VALUES (%d, -%d);", p, r, r),
				T2: fmt.Sprintf("INSERT INTO child_%s VALUES (%d, %d, -%d);", p, r, r, r),
			}
		}},
	// T2's own delete reaches the child only through the cascade, so the key T1
	// wrote is one T2 never names.
	{Name: "fk_cascade", Shares: "parent p, as T1's REF", Setup: func(p string) string { return parentChild(p, " ON DELETE CASCADE") },
		Round: func(p string, r int) Round {
			return Round{
				Pre: fmt.Sprintf("INSERT INTO parent_%s VALUES (%d);", p, r),
				T1:  fmt.Sprintf("INSERT INTO child_%s VALUES (%d, %d);", p, r, r),
				T2:  fmt.Sprintf("DELETE FROM parent_%s WHERE p = %d;", p, r),
			}
		}},
	// The row is written before the savepoint and kept; only a later change to
	// it is rolled back.
	{Name: "sp_keep", Shares: "PK written before the savepoint", Setup: pk,
		Round: func(p string, r int) Round {
			return Round{
				T1: fmt.Sprintf("INSERT INTO t_%[1]s VALUES (%[2]d, 0); SAVEPOINT s1; UPDATE t_%[1]s SET v = 9 WHERE id = %[2]d; ROLLBACK TO SAVEPOINT s1;", p, r),
				T2: fmt.Sprintf("UPDATE t_%s SET v = 2 WHERE id = %d;", p, r),
			}
		}},
	// The row is written, rolled back, and written again: the key that survives
	// is the second write's.
	{Name: "sp_redo", Shares: "PK written after the rollback", Setup: pk,
		Round: func(p string, r int) Round {
			return Round{
				T1: fmt.Sprintf("SAVEPOINT s1; INSERT INTO t_%[1]s VALUES (%[2]d, 0); ROLLBACK TO SAVEPOINT s1; INSERT INTO t_%[1]s VALUES (%[2]d, 5);", p, r),
				T2: fmt.Sprintf("UPDATE t_%s SET v = 2 WHERE id = %d;", p, r),
			}
		}},
	{Name: "ddl_dml", Shares: "none (commit order)", Setup: pk,
		Round: func(p string, r int) Round {
			return Round{
				T1: fmt.Sprintf("ALTER TABLE t_%s ADD COLUMN c%d INT;", p, r),
				T2: fmt.Sprintf("INSERT INTO t_%[1]s (id, c%[2]d) VALUES (%[2]d, %[2]d);", p, r),
			}
		}},
	{Name: "part_move", Shares: "PK across partitions",
		Setup: func(p string) string {
			return fmt.Sprintf("CREATE TABLE t_%s (id INT PRIMARY KEY, v INT) PARTITION BY RANGE (id) "+
				"(PARTITION p0 VALUES LESS THAN (100000000), PARTITION p1 VALUES LESS THAN MAXVALUE);", p)
		},
		Round: func(p string, r int) Round {
			return Round{
				Pre: fmt.Sprintf("INSERT INTO t_%s VALUES (%d, 0);", p, r),
				T1:  fmt.Sprintf("UPDATE t_%s SET id = id + 100000000 WHERE id = %d;", p, r),
				T2:  fmt.Sprintf("INSERT INTO t_%s VALUES (%d, 1);", p, r),
			}
		}},
	// The child leaves X for Y, then X goes. The child's UPDATE names X only as
	// the value it had.
	{Name: "fk_repoint", Shares: "FK, the old parent", Setup: func(p string) string { return parentChild(p, " ON DELETE CASCADE") },
		Round: func(p string, r int) Round {
			return Round{
				Pre: fmt.Sprintf("INSERT INTO parent_%[1]s VALUES (%[2]d); INSERT INTO parent_%[1]s VALUES (%[2]d + 1000); INSERT INTO child_%[1]s VALUES (%[2]d, %[2]d);", p, r),
				T1:  fmt.Sprintf("UPDATE child_%[1]s SET p = %[2]d + 1000 WHERE c = %[2]d;", p, r),
				T2:  fmt.Sprintf("DELETE FROM parent_%s WHERE p = %d;", p, r),
			}
		}},
	// X goes and takes child r with it by cascade; then a new child r is made
	// under Y. Only the cascade wrote child r in T1.
	{Name: "fk_cascade_reuse", Shares: "child key, written by the cascade", Setup: func(p string) string { return parentChild(p, " ON DELETE CASCADE") },
		Round: func(p string, r int) Round {
			return Round{
				Pre: fmt.Sprintf("INSERT INTO parent_%[1]s VALUES (%[2]d); INSERT INTO parent_%[1]s VALUES (%[2]d + 1000); INSERT INTO child_%[1]s VALUES (%[2]d, %[2]d);", p, r),
				T1:  fmt.Sprintf("DELETE FROM parent_%s WHERE p = %d;", p, r),
				T2:  fmt.Sprintf("INSERT INTO child_%[1]s VALUES (%[2]d, %[2]d + 1000);", p, r),
			}
		}},
}

func tzPattern(name, ty string) Pattern {
	return Pattern{Name: name, Shares: ty + " UNIQUE: one instant in two zones",
		Setup: func(p string) string {
			return fmt.Sprintf("CREATE TABLE t_%s (id INT PRIMARY KEY, ts %s UNIQUE);", p, ty)
		},
		Round: func(p string, r int) Round {
			return Round{
				Pre: fmt.Sprintf("INSERT INTO t_%s VALUES (%d * 2, %s%s);", p, r, ty, tz0(r)),
				T1:  fmt.Sprintf("DELETE FROM t_%s WHERE id = %d * 2;", p, r),
				T2:  fmt.Sprintf("INSERT INTO t_%s VALUES (%d * 2 + 1, %s%s);", p, r, ty, tz9(r)),
			}
		}}
}

func reinsert(p string, r int) Round {
	return Round{
		Pre: fmt.Sprintf("INSERT INTO t_%s VALUES (%d * 2, %d);", p, r, r),
		T1:  fmt.Sprintf("DELETE FROM t_%s WHERE uk = %d;", p, r),
		T2:  fmt.Sprintf("INSERT INTO t_%s VALUES (%d * 2 + 1, %d);", p, r, r),
	}
}

// tablesOf is the tables a pattern's rounds leave rows in, compared across the
// pair at the end.
func tablesOf(p Pattern) []string {
	switch {
	case strings.HasPrefix(p.Name, "fk_"):
		return []string{"parent_" + p.Name, "child_" + p.Name}
	case p.Name == "inherit_uk":
		return []string{"t_" + p.Name, "sub_" + p.Name}
	}
	return []string{"t_" + p.Name}
}

// Select is the patterns a run asks for, in catalog order with the control
// first whether or not it was named. No names is the whole catalog. Unknown
// names are refused together, by name.
func Select(names []string) ([]Pattern, error) {
	if len(names) == 0 {
		return append([]Pattern(nil), Catalog...), nil
	}
	want := map[string]bool{"ctl": true}
	var unknown []string
	for _, n := range names {
		if find(n) == nil {
			unknown = append(unknown, n)
			continue
		}
		want[n] = true
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, fmt.Errorf("unknown pattern(s): %s (testkit ha-apply list names them)", strings.Join(unknown, ", "))
	}
	var out []Pattern
	for _, p := range Catalog {
		if want[p.Name] {
			out = append(out, p)
		}
	}
	return out, nil
}

// ForPair is the patterns that ask something on a pair that does or does not
// compare VARCHAR without trailing spaces, and the names of the ones left out:
// on a pair that keeps them, a trailing-space pattern's T2 may rightly go first.
func ForPair(pats []Pattern, trailingSpace bool) (kept []Pattern, left []string) {
	for _, p := range pats {
		if p.TrailingSpace && !trailingSpace {
			left = append(left, p.Name)
			continue
		}
		kept = append(kept, p)
	}
	return kept, left
}

func find(name string) *Pattern {
	for i := range Catalog {
		if Catalog[i].Name == name {
			return &Catalog[i]
		}
	}
	return nil
}
