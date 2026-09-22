package hareplsuite

import (
	"context"
	"fmt"
	"strings"
	"time"

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
func Reset(ctx context.Context, p *sandbox.Pair) (ResetOutcome, error) {
	for pass := 0; pass < 5; pass++ {
		left, err := remaining(ctx, p)
		if err != nil {
			return ResetOutcome{}, err
		}
		if len(left) == 0 {
			stranded, serr := strandedOnSlaves(ctx, p)
			return ResetOutcome{Stranded: stranded}, serr
		}
		drops := make([]string, 0, len(left))
		for _, l := range left {
			drops = append(drops, l.drop())
		}
		if _, _, err := newBatch(drops).runRaw(ctx, p.MasterChannel(), p.DB); err != nil {
			return ResetOutcome{}, err
		}
		after, aerr := remaining(ctx, p)
		if aerr != nil {
			return ResetOutcome{}, aerr
		}
		// No progress means the rest cannot be removed this way, and saying so
		// is better than four more passes that will not either.
		if len(after) >= len(left) && len(after) > 0 {
			return ResetOutcome{Left: leftoverNames(after)}, nil
		}
	}
	left, err := remaining(ctx, p)
	if err != nil {
		return ResetOutcome{}, err
	}
	if len(left) == 0 {
		stranded, serr := strandedOnSlaves(ctx, p)
		return ResetOutcome{Stranded: stranded}, serr
	}
	return ResetOutcome{Left: leftoverNames(left)}, nil
}

// ResetOutcome is what a reset has to say afterwards.
//
// Two different failures and they are not interchangeable. Left is the
// master's, and is a reset that did not finish. Stranded is a slave's, and is
// something else entirely: the master is clean, the pair is not, and the
// difference between them is a fact about the engine rather than about this
// runner.
type ResetOutcome struct {
	Left     []string
	Stranded []Stranded
}

// Stranded is one object a slave holds that the master does not.
//
// # Why it is kept and not just fixed
//
// Because it is evidence. A slave holds an object the master does not only if
// something the master did failed to arrive, or arrived and could not be
// undone by name -- which is exactly what this suite exists to find. Repairing
// it silently would erase, once per case, the very thing a run is looking for,
// and nobody reading the tally afterwards could tell an intended divergence
// from an accident.
//
// So each one is printed when it happens, written to a file beside the
// differences, attributed to the case that left it, and counted at the end.
// The repair is what keeps the *next* case honest; the record is what keeps
// this one.
type Stranded struct {
	Node string
	Word string
	Name string
	// Repaired says whether the object is gone from the slave now.
	Repaired bool
	// Why is what stopped the repair, when it did.
	Why string
}

func (s Stranded) String() string {
	out := strings.ToLower(s.Word) + " " + s.Name + " on " + s.Node
	switch {
	case s.Repaired:
		out += " (repaired)"
	case s.Why != "":
		out += " (not repaired: " + s.Why + ")"
	}
	return out
}

// strandedOnSlaves reports what a slave still holds once the master is clean.
//
// # Why a clean master is not a clean pair
//
// This reset runs on the master and reaches a slave the only way anything
// does: as replication. So a DROP the master accepts is a DROP the slave
// replays -- and a slave replays it by name. Once the two catalogs disagree
// about a name, the drop names something the slave does not have, fails there,
// and the object stays.
//
// Measured, four statements
// (evidence/ha/class-owner-change-not-replicated.md): `call change_owner
// ('t1', 'u1') on class db_root` leaves `u1.t1` on the master and `dba.t1` on
// the slave. The reset then drops `[U1].[t1]`, which is right for the master
// and names nothing on the slave. The master is clean, the slave holds
// `dba.t1`, and the next case's `create table t1` succeeds on the master and
// is refused on the slave for a name clash -- so from there on the two nodes
// differ for a reason belonging to a case that has already finished, and a
// comparison of class *names* cannot even see it.
//
// This cannot be repaired from here: a standby takes no writes, and the only
// hand that reaches it is the master's log. So it is reported, which is this
// file's whole contract -- a reset that quietly failed is the defect it exists
// to remove, and one that half-failed is the same defect on one node.
//
// # Why it costs nothing when nothing is wrong
//
// The check is one query per slave, and it is only reached when the master
// came back clean. A slave that looks dirty may simply be behind, so that case
// -- and only that case -- pays for one marker round trip and a second look.
func strandedOnSlaves(ctx context.Context, p *sandbox.Pair) ([]Stranded, error) {
	stranded, err := slaveLeftovers(ctx, p)
	if err != nil || len(stranded) == 0 {
		return nil, err
	}
	// Behind is not stranded. One marker crossing tells them apart, and it is
	// paid for only by a pair that already looks wrong.
	if _, werr := p.WaitForReplication(ctx, slaveCatchUp); werr != nil {
		for i := range stranded {
			stranded[i].Why = "no marker crossed, so this may be lag rather than divergence"
		}
		return stranded, nil
	}
	stranded, err = slaveLeftovers(ctx, p)
	if err != nil || len(stranded) == 0 {
		return nil, err
	}
	return repairSlaves(ctx, p, stranded), nil
}

// repairSlaves takes a stranded object off a slave, by the only hand that
// reaches one: the master's log.
//
// A standby accepts no writes, so nothing here connects to it. What it does
// instead is make the master issue the DROP the slave will accept. The master
// does not have the object -- that is what stranded means -- so the DROP has
// to be given something to drop:
//
//	master:  CREATE TABLE [DBA].[xxx](...)   slave: refused, the name is taken
//	master:  DROP TABLE [DBA].[xxx]          slave: the stranded object goes
//
// The CREATE failing on the slave is the point rather than a problem. Both
// statements succeed on the master, which ends as clean as it started.
//
// # What it will not do
//
// Invent a definition. A view needs a query, a synonym needs a target and a
// trigger needs a table and an action, and a placeholder for any of those is
// this runner deciding what the case meant. Those are reported unrepaired,
// which is worse for the run and better than a guess.
func repairSlaves(ctx context.Context, p *sandbox.Pair, stranded []Stranded) []Stranded {
	var drops []string
	for i, s := range stranded {
		make, ok := placeholderFor(s.Word, s.Name)
		if !ok {
			stranded[i].Why = "a " + strings.ToLower(s.Word) +
				" needs a definition this runner will not invent"
			continue
		}
		drops = append(drops, make, "DROP "+s.Word+" "+s.Name)
	}
	if len(drops) == 0 {
		return stranded
	}
	if _, _, err := newBatch(drops).runRaw(ctx, p.MasterChannel(), p.DB); err != nil {
		for i := range stranded {
			if stranded[i].Why == "" {
				stranded[i].Why = err.Error()
			}
		}
		return stranded
	}
	if _, werr := p.WaitForReplication(ctx, slaveCatchUp); werr != nil {
		for i := range stranded {
			if stranded[i].Why == "" {
				stranded[i].Why = "the repair did not reach the slave within " + slaveCatchUp.String()
			}
		}
		return stranded
	}
	// Said rather than assumed: the slave is asked again, and what is still
	// there is still reported.
	after, err := slaveLeftovers(ctx, p)
	if err != nil {
		return stranded
	}
	still := map[string]bool{}
	for _, s := range after {
		still[s.Node+" "+s.Word+" "+s.Name] = true
	}
	for i, s := range stranded {
		if s.Why != "" {
			continue
		}
		if still[s.Node+" "+s.Word+" "+s.Name] {
			stranded[i].Why = "the master's DROP did not remove it"
			continue
		}
		stranded[i].Repaired = true
	}
	return stranded
}

// placeholderFor is the shortest thing the master can create under a given
// name so that dropping it names the slave's copy too.
func placeholderFor(word, name string) (string, bool) {
	switch word {
	case "TABLE":
		return "CREATE TABLE " + name + "(" + KeyColumn + " INT)", true
	case "SERIAL":
		return "CREATE SERIAL " + name, true
	case "USER":
		return "CREATE USER " + name, true
	default:
		return "", false
	}
}

// slaveCatchUp bounds the one wait strandedOnSlaves pays for.
const slaveCatchUp = 30 * time.Second

// markerPrefix names the tables the replication wait makes and unmakes, as
// internal/sandbox spells them.
const markerPrefix = "tkrepl_"

func slaveLeftovers(ctx context.Context, p *sandbox.Pair) ([]Stranded, error) {
	var out []Stranded
	for i, name := range p.Slaves {
		ch, cerr := p.SlaveChannel(i)
		if cerr != nil {
			return nil, cerr
		}
		res, rerr := ch.Run(ctx, fmt.Sprintf("csql -u dba -t -N -c %q %s", resetQuery, p.DB))
		if rerr != nil {
			return nil, fmt.Errorf("asking %s what it still holds: %w", name, rerr)
		}
		for _, row := range parseTableList(res.Stdout) {
			word, n, ok := strings.Cut(bare(row), " ")
			if !ok || n == "" {
				continue
			}
			// The wait above leaves its own marker behind for a moment: the
			// slave reports the row, the master drops the table, and that DROP
			// is still crossing while this read happens. Reporting it would
			// accuse the pair of the check's own footprint.
			if strings.Contains(strings.ToLower(n), "["+markerPrefix) {
				continue
			}
			out = append(out, Stranded{Node: name, Word: strings.ToUpper(word), Name: n})
		}
	}
	return out, nil
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
