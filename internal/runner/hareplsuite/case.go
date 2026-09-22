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
	// Converted counts CREATE TABLE statements given a primary key they did
	// not have, and WriteFailed the writes the engine refused.
	Converted   int
	WriteFailed int
	// KeyDuplicate and KeyNull count the two ways an added primary key can
	// refuse the corpus's own data. The first is unambiguous; the second
	// includes the corpus's own NOT NULL columns.
	KeyDuplicate int
	KeyNull      int
	// ObjectDomain counts reads skipped because they touch a table with a
	// column whose type is another class.
	ObjectDomain int
	// EmptyAgreement counts reads the two nodes agreed on where the master
	// itself returned no rows. Agreement about nothing is still agreement,
	// and it is what a refused write leaves behind.
	EmptyAgreement int
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
	// Empty is the reads that agreed on nothing, in the case's own words.
	Empty []string
	// ErrorCase says this case was never going to put a row in front of the
	// pair -- the corpus marked it `--[er]`, or it writes no data at all --
	// and EmptyExpected counts the empty agreements that follow from that.
	ErrorCase     bool
	EmptyExpected int
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
// Joined against db_class so the catalog's own forty-odd views are not read
// on every refresh, and not mistaken for something a case made.
const viewsQuery = "SELECT v.vclass_name, v.vclass_def FROM db_class c, db_vclass v " +
	"WHERE c.class_name = v.vclass_name AND c.is_system_class='NO'"
const synonymsQuery = "SELECT synonym_name, target_name FROM db_synonym"

// objectDomainQuery names the tables holding a column whose type is another
// class.
//
// Such a column replicates its row and not its reference: the referenced row
// arrives, the referring row arrives with its key and its ordinary columns,
// and the reference itself arrives as a stored NULL
// (evidence/ha/object-domain-not-replicated.md). Replication carries the
// primary key and the slave rebuilds the row from the master's heap image, in
// which an object reference is an OID -- a volume, page and slot on the
// master, naming nothing on the slave.
//
// **A known constraint of CUBRID HA, undocumented.** It is therefore the
// fourth thing this suite cannot ask the pair about, beside a missing primary
// key, a view onto a keyless table and a synonym for one -- and reporting it
// as a difference would be reporting a design decision as this build's
// defect, which is the noise every one of those three exists to remove.
const objectDomainQuery = "SELECT DISTINCT a.class_name FROM db_attribute a, db_class c " +
	"WHERE a.class_name = c.class_name AND c.is_system_class='NO' " +
	"AND a.domain_class_name IS NOT NULL ORDER BY 1"

// Skipped is a case this runner will not judge: its semicolons are not all
// statement terminators, so splitting it would run fragments. Named rather
// than attempted, and counted rather than hidden.
const Skipped Outcome = "skipped"

// Replicating is a case that wrote, made no comparable read, and whose write
// was followed across to the slave. It establishes less than Same and more
// than nothing: not that this case's data matches, but that the pair was
// replicating while it ran.
const Replicating Outcome = "replicating"

// Unreplicatable is a case whose tables have no primary key. Its rows never
// reach the slave and the engine reports nothing, so a comparison would be
// answering a question about CUBRID's HA design rather than about this build.
// Separated from Differ because calling it a replication failure would be
// wrong, and from Skipped because the case ran.
const Unreplicatable Outcome = "unreplicatable"

// RunCase runs a case and compares every read it makes across the pair.
//
// The first version compared the database *after* the case and found almost
// nothing: 38 of 39 cases in `_06_manipulation/_04_insert` came back "no
// data", because the corpus's shape is create, insert, select, **drop** -- by
// the end there is nothing left to disagree about. So the oracle is the case's
// own SELECTs, run on both nodes.
//
// The second version sent one statement per call and cost about a second each.
// This one sends a run of statements at a time (batch.go), which is 6.7x on
// the measurement there. The runs are not arbitrary: a case is writes, then
// reads, then writes, and **the wait belongs between a write and the read that
// follows it** and nowhere else. So the grouping the oracle needs and the
// grouping that is fast are the same grouping.
func RunCase(ctx context.Context, p *sandbox.Pair, name, sql string, wait time.Duration, keepDir string, addKey bool) Result {
	res := Result{Case: name}
	if !Splittable(sql) {
		res.Outcome = Skipped
		res.Detail = "the case opens a block whose END never comes, so the rest of the file would run as one statement"
		return res
	}
	res.ErrorCase = IsErrorCase(sql) || IsDataless(sql)
	stmts := Statements(sql)
	if len(stmts) == 0 {
		res.Outcome, res.Detail = NoData, "the case has no statement"
		return res
	}

	master := p.MasterChannel()
	conv := NewConversion()
	var prelude []string
	dirty, keysStale := false, true
	var noKey, withObject []string
	seen := map[string]bool{}

	for _, seg := range segments(stmts) {
		if ctx.Err() != nil {
			res.Outcome, res.Detail = CaseFailed, "interrupted"
			return res
		}
		res.Statements += len(seg.stmts)

		if !seg.read {
			list := make([]string, len(seg.stmts))
			copy(list, seg.stmts)
			if addKey {
				for i := range list {
					if converted, changed := conv.Apply(list[i]); changed {
						list[i] = converted
						res.Converted++
					}
				}
			}
			for _, st := range list {
				if IsSessionStatement(st) {
					prelude = append(prelude, st)
				}
			}
			out, _, err := newBatchWith(prelude, list).runRaw(ctx, master, p.DB)
			if err != nil {
				res.Outcome, res.Detail = CaseFailed, fmt.Sprintf("the master could not be reached: %v", err)
				return res
			}
			// A batch reports one exit code for many statements, so refusals
			// are counted from the output. They matter because the conversion
			// can cause them and the failure is silent where it lands: an
			// INSERT the added key rejects leaves the table empty on both
			// nodes, and two empty tables agree.
			res.WriteFailed += strings.Count(out, "ERROR: ")
			// Split out the refusals the conversion can cause. The unique
			// violation names the index and CUBRID generates a primary key's
			// as pk_<table>_<column>, so that one is unambiguous. A NOT NULL
			// violation is not -- the corpus has its own NOT NULL columns --
			// and is counted apart rather than folded into either.
			res.KeyDuplicate += strings.Count(out, "unique constraint violations. INDEX pk_")
			res.KeyNull += strings.Count(out, "violated NOT NULL constraint")
			// Over-claimed on purpose: an unnecessary wait costs a second,
			// and a missed one compares a slave that was never given the
			// chance to catch up.
			dirty, keysStale = true, true
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
		if keysStale {
			bare, kerr := tablesWithoutPrimaryKey(ctx, p)
			if kerr != nil {
				res.Outcome, res.Detail = CaseFailed, kerr.Error()
				return res
			}
			objs, oerr := query(ctx, p, objectDomainQuery)
			if oerr != nil {
				res.Outcome, res.Detail = CaseFailed, oerr.Error()
				return res
			}
			withObject = names(objs)
			noKey, keysStale = bare, false
			for _, t := range bare {
				if !seen[t] {
					seen[t] = true
					res.Tables = append(res.Tables, t)
				}
			}
		}

		// The reads carry the prelude too, and so does the slave's copy: a
		// read has to run as the user the case logged in as, on both nodes.
		b := newBatchWith(prelude, seg.stmts)
		want, _, err := b.run(ctx, master, p.DB)
		if err != nil {
			res.Outcome, res.Detail = CaseFailed, fmt.Sprintf("the master could not be reached: %v", err)
			return res
		}
		slave, serr := p.SlaveChannel(0)
		if serr != nil {
			res.Outcome, res.Detail = CaseFailed, serr.Error()
			return res
		}
		got, _, gerr := b.run(ctx, slave, p.DB)
		if gerr != nil {
			res.Outcome, res.Detail = CaseFailed, fmt.Sprintf("the slave could not be reached: %v", gerr)
			return res
		}

		for i, stmt := range seg.stmts {
			// A read that touches a table whose rows never arrive is not a
			// comparison this suite can make; forcing one would report
			// CUBRID's HA design as this build's defect.
			if mentionsAny(stmt, noKey) {
				res.Unreplicated++
				continue
			}
			// Counted apart from the keyless tables on purpose: they are two
			// different facts about a corpus, and one number would hide the
			// second.
			if mentionsAny(stmt, withObject) {
				res.ObjectDomain++
				continue
			}
			res.Compared++
			if Normalise(want[i]) == Normalise(got[i]) {
				// Two empty answers agree, and that is how a refused write
				// looks from here: the rows never landed on either node. It
				// is the conversion's quiet failure mode and the corpus's own
				// negative cases both, so it is counted rather than judged --
				// the number says how much of "same" rests on nothing.
				if noRows(want[i]) {
					res.EmptyAgreement++
					if res.ErrorCase {
						res.EmptyExpected++
					}
					// Kept so the number can be read rather than guessed at.
					// "110 reads agreed about nothing" is a fact about the
					// corpus, and which reads they were is the part that says
					// whether anything can be done about it.
					res.Empty = append(res.Empty, firstLine(stmt))
				}
				continue
			}
			// The same rows in a different order is agreement. SQL promises
			// no order without ORDER BY, so both nodes are right and the
			// question this suite asks is answered yes.
			if samePermutation(want[i], got[i]) {
				res.Unordered++
				continue
			}
			res.Outcome, res.Node = Differ, p.Slaves[0]
			res.Differing = firstLine(stmt)
			res.Detail = fmt.Sprintf("%s answered this read differently from the master, after replication was waited for", p.Slaves[0])
			if keepDir != "" {
				if where, werr := keepDifference(keepDir, name, stmt, want[i], got[i], p.Slaves[0]); werr == nil {
					res.Kept = where
				}
			}
			return res
		}
	}

	if res.Compared == 0 {
		if res.ObjectDomain > 0 && res.Unreplicated == 0 {
			res.Outcome = Unreplicatable
			res.Detail = "every read touches an object-domain column, whose reference is not replicated"
			return res
		}
		if res.Unreplicated > 0 {
			res.Outcome = Unreplicatable
			res.Detail = "every read touches a table with no primary key (" +
				strings.Join(res.Tables, ", ") + "), whose rows are never replicated"
			return res
		}
		// The case asked the pair nothing about its own data. If it wrote
		// anything, one question is still worth asking, and it is the one
		// CTP's ha_repl asks of every case whatever the case contains:
		// **was replication alive while this ran?**
		//
		// Taken from CTP deliberately (Test.java:937 `waitDataReplicated`,
		// which polls a flag table of its own on the slave, once per case).
		// This suite had the parts already -- WaitForReplication writes and
		// polls exactly such a marker -- and threw the answer away, reporting
		// `no_data` for 14 cases that write. What it kept instead is the part
		// CTP gets wrong: comparing statements the engine refused, and
		// retrying as though the slave were behind.
		if dirty {
			waited, werr := p.WaitForReplication(ctx, wait)
			res.Waited += waited
			if werr != nil {
				res.Outcome, res.Detail = WaitTimeout, werr.Error()
				return res
			}
			res.Outcome = Replicating
			res.Detail = "the case makes no read this suite can compare, but it wrote and the pair " +
				"carried a marker across afterwards, so replication was alive while it ran"
			return res
		}
		res.Outcome = NoData
		res.Detail = "the case neither writes nor reads, so the pair was never asked anything"
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
				bare = append(bare, bothSpellings(name)...)
			}
		}
	}
	// A synonym is a name for a name, and the target is the whole row.
	syns, serr := query(ctx, p, synonymsQuery)
	if serr == nil {
		for _, row := range syns {
			name, target, ok := splitTwo(row)
			if ok && mentionsAny(target, bare) {
				bare = append(bare, bothSpellings(name)...)
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

// bothSpellings is a catalog name and the name a statement is likely to use.
//
// db_vclass and db_synonym carry an owner-qualified name -- `u1.v1` -- and a
// case says `select * from v1`, so matching only what the catalog returned
// misses it. That was one of the two differences left after the view check
// went in, and it was not the engine.
//
// Both are kept rather than only the tail: a case may qualify too, and an
// extra name in this list costs a comparison that is skipped, which is the
// cheap direction.
func bothSpellings(name string) []string {
	out := []string{name}
	if i := strings.LastIndexByte(name, '.'); i >= 0 && i+1 < len(name) {
		out = append(out, name[i+1:])
	}
	return out
}

// noRows reports whether csql's answer held no data.
//
// Matched on csql's own sentence rather than on the absence of lines, because
// a result block always carries a header and a separator.
func noRows(block string) bool {
	b := Normalise(block)
	return strings.TrimSpace(b) == "" || strings.Contains(b, "There are no results.") ||
		strings.Contains(b, "0 rows selected") || strings.Contains(b, "0 row selected")
}

// IsErrorCase reports whether the corpus marked this case as one whose point
// is that the engine refuses something.
//
// The `sql` corpus writes `--[er]` in a case's first-line comment when the
// case is about an error, and 2,911 of them do
// (category/sql/02-writing-a-case.md). Such a case converts to an ha_repl
// case whose reads return nothing on both nodes -- which is agreement, and
// which establishes nothing, and which is **not** a defect in the conversion
// or in the pair.
//
// It matters because "110 reads agreed about nothing" is two different
// populations. The ones in an error case are the corpus working as written.
// The rest are the ones worth looking at.
func IsErrorCase(sql string) bool {
	return strings.Contains(sql, "--[er]")
}

// IsDataless reports whether a case writes no rows at all.
//
// A large part of this corpus is about plans, syntax and the catalog rather
// than about data: `cbrd_24082/outer_join.sql` creates two empty tables and
// runs selects over them to check an optimiser decision. Its reads return
// nothing on both nodes, which is agreement and establishes nothing -- and it
// is the case working as written, not a defect in the conversion or the pair.
//
// Detected by the absence of any DML rather than by a marker, because the
// `--[er]` convention is not used everywhere: in `_06_manipulation` it
// explains six of seven empty agreements and in `_33_elderberry` it explains
// none of a hundred.
func IsDataless(sql string) bool {
	low := strings.ToLower(sql)
	for _, kw := range []string{"insert ", "insert\n", "update ", "delete ", "replace ", "load "} {
		if strings.Contains(low, kw) {
			return false
		}
	}
	return true
}
