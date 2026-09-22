package hareplsuite

import (
	"context"
	"fmt"
	"strings"

	"github.com/cubrid-systems/cubrid-testkit/internal/sandbox"
)

// Reset empties the database of everything a case can leave behind, and
// reports what it could not remove.
//
// # Why this is its own thing, and why it retries
//
// The first version dropped tables one at a time in catalog order and
// reported nothing. Six tables survived a run of 39 cases -- `emp`,
// `employees`, `otgt`, `person2`, `t1`, `t2` -- because a table another
// table's foreign key references cannot be dropped, and because a failed drop
// looked exactly like a successful one.
//
// That is what made the suite irreproducible: the same 39 cases with the same
// settings gave `differ 0` one run and `differ 6` the next, and the state the
// run started from is the variable. A suite whose answer depends on what the
// last run forgot cannot be used to judge anything.
//
// So: everything the catalog names, dropped in an order whose dependencies
// work, repeatedly, until a pass removes nothing. Dependency order is
// discovered by retrying rather than computed, which is shorter than reading
// the foreign keys and is right for the same reason. What is still there after
// that is returned, because a reset that quietly failed is the defect this
// exists to remove.
//
// # Why every name is owner-qualified
//
// The same defect returned one layer down. One table survived every reset of a
// run -- `own_t`, left behind by a case that had changed its owner -- and the
// drop meant to remove it reported nothing. A bare name resolves in the
// caller's schema, so `DROP TABLE [own_t]` as dba names `dba.own_t`, which does
// not exist, while the table that does is `u7.own_t`.
//
// So the catalog is asked for `[owner].[name]` and the DROP is given that.
// Every corpus that changes an owner needs it, and `_01_object` is one.
//
// # Why serials, triggers and users as well
//
// A table is not all a case leaves. A serial and a trigger that stands on no
// table both outlive a table drop, and a user outlives everything. The next
// case's `create serial s1` or `create user u1` then fails for a reason
// belonging to a case that has already run, which is the history this exists to
// remove. A user cannot be dropped while it still owns an object, and nothing
// arranges for that: the retry does it, because the user's drop fails on the
// pass that removes what it owned and succeeds on the next.
//
// # Why one query and not six
//
// Because it runs before every case. Each catalog read is a csql process on the
// far side of a container exec, about four tenths of a second, and six of them
// per case is over two hours across `_01_object`'s 3,327. They are one
// UNION ALL, ranked so that the order the drops need survives the trip.
func Reset(ctx context.Context, p *sandbox.Pair) ([]string, error) {
	for pass := 0; pass < 5; pass++ {
		left, err := remaining(ctx, p)
		if err != nil {
			return nil, err
		}
		if len(left) == 0 {
			return nil, nil
		}
		drops := make([]string, 0, len(left))
		for _, l := range left {
			drops = append(drops, l.drop())
		}
		if _, _, err := newBatch(drops).runRaw(ctx, p.MasterChannel(), p.DB); err != nil {
			return nil, err
		}
		after, aerr := remaining(ctx, p)
		if aerr != nil {
			return nil, aerr
		}
		// No progress means the rest cannot be removed this way, and saying so
		// is better than four more passes that will not either.
		if len(after) >= len(left) && len(after) > 0 {
			return leftoverNames(after), nil
		}
	}
	left, err := remaining(ctx, p)
	if err != nil {
		return nil, err
	}
	return leftoverNames(left), nil
}

// leftover is one object a case left behind, carrying the word its DROP needs.
type leftover struct {
	word string
	// name is already `[owner].[name]`, as the catalog was asked to give it.
	name string
}

func (l leftover) drop() string { return "DROP " + l.word + " " + l.name }

func (l leftover) String() string { return strings.ToLower(l.word) + " " + l.name }

// remaining asks the master what is still there, in the order the drops need.
//
// A catalog that could not be read is an error and not an empty catalog. The
// first version of the serial term said `owner.name`, which is right for
// `_db_serial` and a syntax error against the `db_serial` view -- where owner
// is a varchar -- and with the error swallowed it read as "no serials". The
// serial then survived every reset, and the user who owned it could not be
// dropped either, so the reset reported a stubborn user and nothing about the
// reason. A silent failure is what this file was written to remove, and it is
// not allowed back in to read the catalog.
func remaining(ctx context.Context, p *sandbox.Pair) ([]leftover, error) {
	rows, err := query(ctx, p, resetQuery)
	if err != nil {
		return nil, err
	}
	var out []leftover
	for _, r := range rows {
		word, name, ok := strings.Cut(bare(r), " ")
		if !ok || name == "" {
			continue
		}
		out = append(out, leftover{word: word, name: name})
	}
	return out, nil
}

// resetQuery names everything a case can leave, as `WORD [owner].[name]`.
//
// The rank is the drop order and the only reason the terms are not in the order
// a reader would list them: a view, a synonym and a trigger come off before the
// table they stand on, and the user who owns them comes off last. It is ordered
// on the server because the order is the point, and a client that re-sorted
// would have to know the same thing twice.
//
// `db_class` with `is_system_class='NO'` is the catalog's own answer to "what
// did a case make", so this keeps no list of names to drift from the engine's.
// That matters twice: a reset built on `db_vclass` alone would try to drop the
// catalog's own forty-odd views, and the keyless-table check built on it was
// reading every system view's definition on every refresh.
//
// Two terms carry a condition worth reading:
//
//   - a serial a table owns is that table's auto_increment. It goes when the
//     table goes, and naming it here would be a DROP SERIAL the engine refuses;
//     `class_name` is what the catalog calls that attachment.
//   - DBA, PUBLIC and INFORMATION_SCHEMA are the database's own users and no
//     case's to leave behind.
const resetQuery = "SELECT w || ' ' || n FROM (" +
	"SELECT 1 AS r, 'VIEW' AS w, '[' || owner_name || '].[' || class_name || ']' AS n " +
	"FROM db_class WHERE is_system_class='NO' AND class_type='VCLASS' " +
	"UNION ALL SELECT 2, 'SYNONYM', '[' || synonym_owner_name || '].[' || synonym_name || ']' " +
	"FROM db_synonym " +
	"UNION ALL SELECT 3, 'TRIGGER', '[' || owner_name || '].[' || trigger_name || ']' " +
	"FROM db_trigger " +
	"UNION ALL SELECT 4, 'SERIAL', '[' || owner || '].[' || name || ']' " +
	"FROM db_serial WHERE class_name IS NULL " +
	"UNION ALL SELECT 5, 'TABLE', '[' || owner_name || '].[' || class_name || ']' " +
	"FROM db_class WHERE is_system_class='NO' AND class_type='CLASS' " +
	"UNION ALL SELECT 6, 'USER', '[' || name || ']' " +
	"FROM db_user WHERE name NOT IN ('DBA', 'PUBLIC', 'INFORMATION_SCHEMA')" +
	") t ORDER BY r, n"

// bare strips the quoting csql's -t -N output puts around a name.
func bare(s string) string { return strings.Trim(strings.TrimSpace(s), "'") }

// names strips the quoting from a column of names and drops the blanks.
func names(rows []string) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if n := bare(r); n != "" {
			out = append(out, n)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func leftoverNames(left []leftover) []string {
	out := make([]string, 0, len(left))
	for _, l := range left {
		out = append(out, l.String())
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ResetNote is the sentence a run prints when a reset left something behind.
func ResetNote(left []string) string {
	return fmt.Sprintf("the database still holds %s after a reset; "+
		"every case after this one starts from a state this run did not choose",
		strings.Join(left, ", "))
}
