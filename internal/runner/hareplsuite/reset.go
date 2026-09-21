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
// So: views and synonyms first, because they depend on tables; then tables,
// repeatedly, until a pass removes nothing. Dependency order is discovered by
// retrying rather than computed, which is shorter than reading the foreign
// keys and is right for the same reason. What is still there after that is
// returned, because a reset that quietly failed is the defect this exists to
// remove.
func Reset(ctx context.Context, p *sandbox.Pair) ([]string, error) {
	for pass := 0; pass < 5; pass++ {
		views, _ := query(ctx, p, userViewsQuery)
		syns, _ := query(ctx, p, userSynonymsQuery)
		tables, err := query(ctx, p, userTablesQuery)
		if err != nil {
			return nil, err
		}
		if len(views)+len(syns)+len(tables) == 0 {
			return nil, nil
		}
		var drops []string
		for _, v := range views {
			drops = append(drops, "DROP VIEW ["+bare(v)+"]")
		}
		for _, s := range syns {
			drops = append(drops, "DROP SYNONYM ["+bare(s)+"]")
		}
		for _, t := range tables {
			drops = append(drops, "DROP TABLE ["+bare(t)+"]")
		}
		before := len(tables) + len(views) + len(syns)
		if _, _, err := newBatch(drops).runRaw(ctx, p.MasterChannel(), p.DB); err != nil {
			return nil, err
		}
		after, aerr := query(ctx, p, userTablesQuery)
		if aerr != nil {
			return nil, aerr
		}
		// No progress means the rest cannot be removed this way, and saying so
		// is better than four more passes that will not either.
		if len(after) >= before && len(after) > 0 {
			return names(after), nil
		}
	}
	left, err := query(ctx, p, userTablesQuery)
	if err != nil {
		return nil, err
	}
	return names(left), nil
}

// bare strips the quoting csql's -t -N output puts around a name.
func bare(s string) string { return strings.Trim(strings.TrimSpace(s), "'") }

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

// userViewsQuery and userSynonymsQuery name only what a case made.
//
// db_vclass on its own returns the catalog's own views -- `tables`, `views`,
// `triggers` and forty more -- so a reset built on it would try to drop the
// catalog, and the keyless-table check built on it was reading every system
// view's definition on every refresh.
const userViewsQuery = "SELECT c.class_name FROM db_class c, db_vclass v " +
	"WHERE c.class_name = v.vclass_name AND c.is_system_class='NO' ORDER BY 1"

const userSynonymsQuery = "SELECT synonym_name FROM db_synonym ORDER BY 1"

// ResetNote is the sentence a run prints when a reset left something behind.
func ResetNote(left []string) string {
	return fmt.Sprintf("the database still holds %s after a reset; "+
		"every case after this one starts from a state this run did not choose",
		strings.Join(left, ", "))
}
