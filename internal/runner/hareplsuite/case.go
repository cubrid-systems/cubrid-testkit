// Package hareplsuite runs ha_repl against a topology cluster-sandbox provides,
// with the runner driving both nodes from outside (ADR-022).
//
// It is not a port of CTP's ha_repl. CTP reaches its nodes over SSH and a
// sandbox node runs no sshd, so the two cannot be the same program; and nothing
// here is compared against CTP's verdicts, so fidelity to its per-statement
// dump format buys nothing. What is kept is the part that makes the suite worth
// having -- **the oracle is the pair disagreeing with itself**
// (design/module-ha.md §2-2) -- and the part CTP's shell corpus gets wrong:
// synchronisation is a poll on state and never a sleep (§4 P1).
package hareplsuite

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/sandbox"
)

// Outcome is what running one case against the pair established.
//
// Five, not two, because "passed" and "failed" cannot carry what this suite
// actually finds. A case that leaves no data is not a pass -- it is a case with
// nothing to say here, and counting it as a pass would inflate the suite with
// cases that cannot fail (§4 P7 is the same argument applied to the shell
// corpus).
type Outcome string

const (
	// Same is the verdict this suite exists to produce: the case wrote, the
	// wait returned, and every user table reads identically on both nodes.
	Same Outcome = "same"
	// Differ is the finding. A table's contents disagree after replication has
	// been waited for, which is the difference between "in flight" and "not
	// going to arrive".
	Differ Outcome = "differ"
	// NoData is the honest outcome for a read-only case. Most of the sql corpus
	// is one: it converts to a case whose claim is that nothing happened on
	// both nodes.
	NoData Outcome = "no_data"
	// CaseFailed is the master refusing the case's own SQL. Not a replication
	// finding -- the case never got far enough to make one.
	CaseFailed Outcome = "case_failed"
	// WaitTimeout is replication not arriving inside the bound. It is reported
	// separately from Differ because the remedies are not the same one.
	WaitTimeout Outcome = "wait_timeout"
)

// Result is one case's outcome and what it rests on.
type Result struct {
	Case    string
	Outcome Outcome
	// Statements executed, and Compared reads checked across the pair. A case
	// with statements and no comparisons is the NoData case, and the two
	// numbers are what says so.
	Statements int
	Compared   int
	// Unordered counts reads whose answers were the same rows in a different
	// order.
	Unordered int
	// Unreplicated counts reads skipped because they touch a table with no
	// primary key. A case with comparisons AND skips is partly established,
	// and saying so is the point of keeping the two apart.
	Unreplicated int
	// Tables names the keyless tables this case met.
	Tables []string
	// Kept is where both answers were written, when they differed.
	Kept string
	// Differing names the first read that disagreed, and the node it
	// disagreed on.
	Differing string
	Node      string
	// Waited is how long replication took, which is the number P1 is about.
	Waited time.Duration
	// Detail is the sentence a reader needs and nothing more.
	Detail string
}

// systemTablesQuery lists what the case left behind.
//
// `db_class` with `is_system_class='NO'` is the catalog's own answer to "what
// is a user table", so the suite does not keep a list of names to drift from
// the engine's.
const userTablesQuery = "SELECT class_name FROM db_class WHERE is_system_class='NO' AND class_type='CLASS' ORDER BY 1"

// noPrimaryKeyQuery lists the user tables whose rows CUBRID's HA will not
// replicate.
//
// Measured on this pair, two tables created and inserted into in one run,
// differing only in the key:
//
//	create table pk_yes(i int primary key, v varchar(10)) -> master 1, slave 1
//	create table pk_no (i int,             v varchar(10)) -> master 1, slave 0
//
// applylogdb replicates a row by its primary key, so a table without one
// receives its DDL and never its rows. Nothing reports an error: fail_counter
// stays where it was and the applier stays "working", which is why this cost a
// run of 39 cases before it was noticed.
const noPrimaryKeyQuery = "SELECT c.class_name FROM db_class c WHERE c.is_system_class='NO' AND c.class_type='CLASS' " +
	"AND NOT EXISTS (SELECT 1 FROM db_index i WHERE i.class_name=c.class_name AND i.is_primary_key='YES') ORDER BY 1"

// viewsQuery and synonymsQuery are how a read reaches a keyless table without
// naming it.
//
// The first run over 131 cases produced ten differences and every one of them
// was this: `select * from AES` where AES is a view over `create table tx(tx
// int)`, and `select * from s1` where s1 is a synonym for one. The master has
// the rows, the slave has none, and the name in the statement is not the name
// of the table that cannot replicate. Ten findings, one hole, no engine defect
// among them.
const viewsQuery = "SELECT vclass_name, vclass_def FROM db_vclass"
const synonymsQuery = "SELECT synonym_name, target_name FROM db_synonym"

// Skipped is a case this runner will not judge: its semicolons are not all
// statement terminators, so splitting it would run fragments. Named rather
// than attempted, and counted rather than hidden.
const Skipped Outcome = "skipped"

// Unreplicatable is a case whose tables have no primary key. Its rows never
// reach the slave and the engine reports nothing, so a comparison would be
// answering a question about CUBRID's HA design rather than about this build.
// Separated from Differ because calling it a replication failure would be
// wrong, and from Skipped because the case ran.
const Unreplicatable Outcome = "unreplicatable"

// Unordered is a read whose two answers hold the same rows in a different
// order. SQL does not promise an order without ORDER BY, so the two nodes are
// both right and the comparison was never well defined. Reported rather than
// dropped, because the number is what says how much of a corpus can be used
// as a differential one at all.
const Unordered Outcome = "unordered"

// RunCase runs a case statement by statement and compares every read across
// the pair.
//
// The first version of this compared the database *after* the case and found
// almost nothing: 38 of 39 cases in `_06_manipulation/_04_insert` came back
// "no data", because the corpus's shape is create, insert, select, **drop** --
// by the end there is nothing left to disagree about. That is why CTP's
// ha_repl compares per statement, and this now does the same.
//
// The oracle is therefore the case's own SELECTs, run on both nodes and
// compared. The wait sits between a write and the next read and nowhere else:
// waiting before every statement would cost a marker round trip per line, and
// waiting never is the defect P1 is about.
func RunCase(ctx context.Context, p *sandbox.Pair, name, sql string, wait time.Duration, keepDir string) Result {
	res := Result{Case: name}
	if !Splittable(sql) {
		res.Outcome = Skipped
		res.Detail = "the case carries a block body, whose semicolons are not statement terminators"
		return res
	}
	stmts := Statements(sql)
	if len(stmts) == 0 {
		res.Outcome, res.Detail = NoData, "the case has no statement"
		return res
	}

	master := p.MasterChannel()
	dirty, keysStale := false, true
	var noKey []string
	seen := map[string]bool{}
	for _, stmt := range stmts {
		if ctx.Err() != nil {
			res.Outcome, res.Detail = CaseFailed, "interrupted"
			return res
		}
		if isDirective(stmt) {
			continue
		}
		res.Statements++

		out, err := master.Run(ctx, csql(p.DB, stmt))
		if err != nil {
			res.Outcome, res.Detail = CaseFailed, fmt.Sprintf("the master could not be reached: %v", err)
			return res
		}
		if IsWrite(stmt) {
			// A statement the engine refused changed nothing, so it owes the
			// slave nothing. Marking dirty anyway would only cost a wait, but
			// the exit code is the cheapest signal there is.
			if out.ExitCode == 0 {
				dirty, keysStale = true, true
			}
			continue
		}

		if dirty {
			waited, werr := p.WaitForReplication(ctx, wait)
			res.Waited += waited
			if werr != nil {
				res.Outcome, res.Detail = WaitTimeout, werr.Error()
				return res
			}
			dirty = false
		}
		// Which tables cannot replicate is a property of the database as it
		// stands, so it is re-read after a write and not once per case: a case
		// creates its tables as it goes.
		if keysStale {
			bare, kerr := tablesWithoutPrimaryKey(ctx, p)
			if kerr != nil {
				res.Outcome, res.Detail = CaseFailed, kerr.Error()
				return res
			}
			noKey, keysStale = bare, false
			for _, t := range bare {
				if !seen[t] {
					seen[t] = true
					res.Tables = append(res.Tables, t)
				}
			}
		}
		// A read that touches a table whose rows never arrive is not a
		// comparison this suite can make, and forcing one would report CUBRID's
		// HA design as this build's defect. A read that touches none of them --
		// a catalog query, or a table that has a key -- is compared as normal.
		// Name matching rather than parsing: it errs towards skipping, and the
		// safe direction here is to compare less.
		if mentionsAny(stmt, noKey) {
			res.Unreplicated++
			continue
		}
		res.Compared++
		node, want, got, cerr := compareRead(ctx, p, stmt)
		if cerr != nil {
			res.Outcome, res.Detail = CaseFailed, cerr.Error()
			return res
		}
		if node != "" {
			if samePermutation(want, got) {
				res.Unordered++
				continue
			}
			res.Outcome, res.Node = Differ, node
			res.Differing = firstLine(stmt)
			res.Detail = fmt.Sprintf("%s answered this read differently from the master, after replication was waited for", node)
			if keepDir != "" {
				if where, werr := keepDifference(keepDir, name, stmt, want, got, node); werr == nil {
					res.Kept = where
				}
			}
			return res
		}
	}

	if res.Compared == res.Unordered && res.Unordered > 0 {
		res.Outcome = Unordered
		res.Detail = fmt.Sprintf("%d read(s) returned the same rows in a different order, and none of them says ORDER BY",
			res.Unordered)
		return res
	}
	if res.Compared == 0 {
		if res.Unreplicated > 0 {
			res.Outcome = Unreplicatable
			res.Detail = "every read touches a table with no primary key (" +
				strings.Join(res.Tables, ", ") + "), whose rows are never replicated"
			return res
		}
		res.Outcome = NoData
		res.Detail = "the case makes no read, so the pair was never asked to agree about anything"
		return res
	}
	res.Outcome = Same
	return res
}

// csql is one statement, handed to the node whole.
func csql(db, stmt string) string {
	return fmt.Sprintf("csql -u dba %s <<'__TESTKIT_SQL__'\n%s;\n__TESTKIT_SQL__", db, stmt)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i] + " ..."
	}
	if len(s) > 90 {
		s = s[:90] + " ..."
	}
	return s
}

// userTables asks the master what the case left behind.
func userTables(ctx context.Context, p *sandbox.Pair) ([]string, error) {
	q := fmt.Sprintf("csql -u dba -t -N -c %q %s", userTablesQuery, p.DB)
	out, err := p.MasterChannel().Run(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("listing user tables: %w", err)
	}
	return parseTableList(out.Stdout), nil
}

// parseTableList reads csql's -t -N output, which is one bare value per line
// plus whatever the connection notification printed around it.
func parseTableList(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
		case strings.HasPrefix(line, "Time:"):
		case strings.HasPrefix(line, "Program 'csql'"):
		case strings.Contains(line, " sec)"):
		case strings.HasPrefix(line, "Committed."):
		// csql's diagnostics, not a name. Matched with the colon, because a
		// table may legitimately be called ERROR -- one in this corpus is, and
		// dropping it here made its view look like a replication finding.
		case strings.HasPrefix(line, "ERROR:"), strings.Contains(line, "ERROR: "):
		default:
			out = append(out, line)
		}
	}
	return out
}

// DropAll clears the database of user tables.
//
// Called between directories and not between cases, because the sql corpus's
// own contract is that a directory is the unit whose cases may rely on each
// other (category/sql/02-writing-a-case.md). Dropping per case would break
// cases that are correct.
func DropAll(ctx context.Context, p *sandbox.Pair) error {
	tables, err := userTables(ctx, p)
	if err != nil {
		return err
	}
	for _, t := range tables {
		q := fmt.Sprintf("csql -u dba -c %q %s", "DROP TABLE ["+t+"]", p.DB)
		if _, rerr := p.MasterChannel().Run(ctx, q); rerr != nil {
			return fmt.Errorf("dropping %s: %w", t, rerr)
		}
	}
	return nil
}

// tablesWithoutPrimaryKey asks the master what a read must not touch: the
// tables whose rows never replicate, and the views and synonyms that reach one.
func tablesWithoutPrimaryKey(ctx context.Context, p *sandbox.Pair) ([]string, error) {
	bare, err := query(ctx, p, noPrimaryKeyQuery)
	if err != nil {
		return nil, fmt.Errorf("listing tables without a primary key: %w", err)
	}
	if len(bare) == 0 {
		return nil, nil
	}
	// A view is as unreadable as the table under it. Its definition is text, so
	// the same name matching is used on it -- and for the same reason: erring
	// towards skipping costs a comparison, where missing one costs a finding
	// that is not one.
	views, verr := query(ctx, p, viewsQuery)
	if verr == nil {
		for _, row := range views {
			name, def, ok := splitTwo(row)
			if ok && mentionsAny(def, bare) {
				bare = append(bare, name)
			}
		}
	}
	// A synonym is a name for a name, and the target is the whole row.
	syns, serr := query(ctx, p, synonymsQuery)
	if serr == nil {
		for _, row := range syns {
			name, target, ok := splitTwo(row)
			if ok && mentionsAny(target, bare) {
				bare = append(bare, name)
			}
		}
	}
	return bare, nil
}

// query runs a catalog read on the master and returns its rows as lines.
func query(ctx context.Context, p *sandbox.Pair, sql string) ([]string, error) {
	q := fmt.Sprintf("csql -u dba -t -N -c %q %s", sql, p.DB)
	out, err := p.MasterChannel().Run(ctx, q)
	if err != nil {
		return nil, err
	}
	return parseTableList(out.Stdout), nil
}

// splitTwo reads csql's two-column -t -N output, which separates values with
// runs of spaces. The second value is a definition and may itself hold spaces,
// so only the first split counts.
func splitTwo(row string) (string, string, bool) {
	f := strings.Fields(row)
	if len(f) < 2 {
		return "", "", false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(row), f[0]))
	return strings.Trim(f[0], "'"), rest, true
}

// mentionsAny reports whether a statement names one of these tables.
//
// Word-boundary matching on a lowered copy. It is an approximation of "what
// does this read touch", and the approximation is deliberately in the
// direction of skipping: a table named inside a string literal costs one
// comparison, where a missed table costs a finding that is not one.
func mentionsAny(stmt string, tables []string) bool {
	if len(tables) == 0 {
		return false
	}
	low := strings.ToLower(stmt)
	for _, t := range tables {
		name := strings.ToLower(t)
		for i := 0; ; {
			j := strings.Index(low[i:], name)
			if j < 0 {
				break
			}
			at := i + j
			before := byte(' ')
			if at > 0 {
				before = low[at-1]
			}
			after := byte(' ')
			if at+len(name) < len(low) {
				after = low[at+len(name)]
			}
			if !isWordByte(before) && !isWordByte(after) {
				return true
			}
			i = at + len(name)
		}
	}
	return false
}

func isWordByte(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// samePermutation reports whether two answers hold the same lines in a
// different order.
//
// It is how a read without ORDER BY is told from a real difference. SQL
// promises no order without one, so two nodes may both be right; calling that
// a replication finding would be reporting the corpus's own omission as this
// build's defect. Compared line by line rather than row by row, because
// csql's header and separator sort alongside the rows and being present in
// both is exactly what is being checked.
func samePermutation(a, b string) bool {
	la := strings.Split(Normalise(a), "\n")
	lb := strings.Split(Normalise(b), "\n")
	if len(la) != len(lb) {
		return false
	}
	sort.Strings(la)
	sort.Strings(lb)
	for i := range la {
		if la[i] != lb[i] {
			return false
		}
	}
	return true
}
