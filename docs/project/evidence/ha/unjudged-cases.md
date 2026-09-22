# The cases `ha_repl` cannot judge, and why CTP could not judge them either

- **Date:** 2026-09-22
- **Question:** the suite reports 24 cases as establishing nothing — `no_data` and
  `unreplicatable`. Would CTP's `ha_repl` establish something on them?
- **Answered by reading, not by running.** See [§ Why not by running](#why-not-by-running):
  CTP's deploy kills every process the invoking user owns, on the master, and the master is this
  machine.

---

## The answer

**23 of the 24 execute no read at all.** CTP's oracle is the same shape as this suite's — the
case's own reads, run on both nodes and compared, master dump against slave dump
([`design/module-ha.md`](../../design/module-ha.md) §1). A case that reads nothing gives CTP
nothing to compare, so CTP would not judge them either.

The 24th does read, and is the one place the two would differ. It is
`cbrd_24042/cbrd_24181/remove_list.sql`, which builds its tables with
`CREATE TABLE ta AS SELECT ...`. A CTAS has no column list, so neither conversion can give it a
primary key — not this one, and not CTP's, whose rule inserts `PRIMARY KEY` into a column list
and returns the line unchanged when there is no `(`. Without a key the rows do not replicate, so
**CTP would compare and report a failure** whose cause is the primary-key requirement. This suite
says `unreplicatable` instead, which is the same fact without the accusation.

## How "no read" was established, three ways

Counting reads with the suite's own splitter is one reading of the corpus, so it was checked
against the text and then against the engine.

| | |
|---|---:|
| cases whose text does not contain the word `select` | 18 |
| cases that contain it inside `create view ... as select`, `insert ... select`, `merge ... using (select)`, a prepared-statement string, or HTML documentation text | 5 |
| cases that contain a standalone `select` | **1** |

The one is `_06_manipulation/_04_insert/cases/1009.sql`, and it is the interesting one. Its first
line is `--[er]test insert with invalid use of single quote`, and line 6 is:

```sql
insert into tb values(''a');
```

The quote is deliberately unbalanced, so everything after it is inside a string literal that never
closes — including the `select`. **csql agrees**, and says so in its own error:

```
ERROR: In line 7, column 25 before '');
select * from tb;
drop class tb; '
Syntax error: unexpected 'a', expecting ',' or ')'
```

The `select` is quoted *inside* the message as part of the broken statement. It does not run, for
csql and therefore for anything that drives csql.

## What the 23 are

Fourteen of them write and never read: create, insert, drop. Nine write nothing at all. Both are
cases whose subject is the statement being accepted or refused, not the data that results — which
is what [`module-ha.md`](../../design/module-ha.md) predicted of the conversion: *"a case whose
point is the rendering has nothing to say here … converting one is not wrong, it is empty."*

## Why not by running

CTP's `ha_repl` deploy calls `clean_processes` on **every node, master included**, and
`CTP/common/script/util_common.sh:38` is:

```bash
function kill_process {
   all_pids=$(calc_pids "$$")
   kill -9 `ps -u $USER -o pid | grep -v "PID" | grep -E -v "$all_pids" | grep -vw $$`
}
```

`kill -9` on every process the invoking user owns, excepting only itself and its own ancestors.
On a two-machine pair whose master is a workstation, that is the operator's ssh session, their
editors, and anything else they were running. It was run twice here before the cause was found,
and both times it took the session down with it. It also stopped the podman sandbox on the same
host, because conmon belongs to the same user.

The preflight and the scenario are prepared and kept, so the run is one line away on a pair whose
master is not a machine anyone is using:
`docs/project/evidence/ha/preflight.sh` reports READY on this pair, and the 24 cases are a
scenario tree of six groups with their answer files.

## What is not claimed

That CTP would agree case for case — nothing was run. The claim is narrower and rests on the
oracle being the same shape: a case that executes no read offers neither runner anything to
compare, and 23 of the 24 execute no read.
