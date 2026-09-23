# `change_owner` on a class does not replicate either, and it poisons everything after it

- **Date:** 2026-09-22
- **Status:** measured, explained, and **awaiting a judgement** — the same one the trigger got, but
  it cannot be given the same answer, because no key is coming for `_db_class`.
- **What this is:** the second difference in `_33_elderberry`, and the cause of the third. Both new
  differences of that run come from this one fact, and finding it took two runner defects out of
  the suite on the way.
- **Trees:** engine `11.5.0.2513-5f3a30d` from `cubrid/cubrid` develop · pairs `pmha` and `gbha`
  from `cubrid-cluster-sandbox`, rootless podman, one host.
- **Found by:** `ha_repl` over `sql/_33_elderberry` — `cbrd_23844/cbrd_24195/cases/create5.sql` —
  then isolated to three statements on a second pair.

---

## The reproduction

```sql
create user u1;
create table t1;
call change_owner ('t1', 'u1') on class db_root;
```

Then, after a marker has crossed:

| read | master | slave |
|---|---|---|
| `select unique_name from _db_class where class_name='t1'` | `u1.t1` | **`dba.t1`** |

The same holds for `call change_owner('xxx','public')`, which is what
`_01_object/_02_class/_003_auto_increment/cubridsus-965.sql` does: master `public.xxx`, slave
`dba.xxx`.

**It is the rule this suite keeps meeting.** `_db_class` has one index and it is not a primary key
— `{DB_CONSTRAINT_INDEX, "i__db_class_unique_name", …}`
(`src/object/schema_system_catalog_install.cpp:457`) — so an instance update on it has no data
channel, and `call … on class db_root` is not a DDL statement, so it has no statement channel
either. `alter table t1 owner to u1`, the DDL form of the same change, replicates
([`trigger-owner-change-not-replicated.md`](trigger-owner-change-not-replicated.md) measured it).

**This is not the trigger's situation.** `_db_trigger` is being given a `unique_name` primary key by
CBRD-27302 (PR #7980), which is why that difference was passed by. **Nothing equivalent is on its
way for `_db_class`**, and the index it has is deliberately not unique — a class name is unique per
owner, not per database. So the judgement here has to be made on its own.

## What it does to everything after it

This is the part that matters more than the divergence itself, and it is what made the run's third
difference.

A reset runs on the master and reaches a slave the only way anything does: as replication. So a
DROP the master accepts is a DROP the slave **replays by name**.

```
case N   : call change_owner ('t1','u1')   master: u1.t1        slave: dba.t1
reset    : DROP TABLE [U1].[t1]            master: clean        slave: names nothing, fails
case N+1 : create table t1 (c1, c2, c3)    master: accepted     slave: refused, the name is taken
case N+1 : select …                        master: c1 c2 c3     slave: a table with no columns
```

Measured on `gbha`, exactly as written. So one owner change by method call leaves the two nodes
holding different databases **for the rest of the run**, and a comparison of class *names* cannot
see it: both nodes have a `t1`.

That is the whole of `cbrd_24195/rename1.sql`'s difference. It runs after `create5.sql`, its
`create table t3` was refused on a slave that still held the previous case's `t1`, and **there is
nothing wrong with `rename1` or with how a foreign key replicates** — tried five ways on a clean
pair, every one of them arrives.

## What the suite does about it now

**It repairs the slave, and it records that it had to.** A standby takes no writes, so the only hand
that reaches it is the master's log:

```
master:  CREATE TABLE [DBA].[t1](…)    slave: refused, the name is taken
master:  DROP TABLE  [DBA].[t1]        slave: the stranded object goes
```

Both statements succeed on the master, which ends as clean as it started. Tables, serials and users
are repaired this way; a view, a synonym or a trigger is reported unrepaired, because a placeholder
for one would be this runner deciding what the case meant.

**The repair is visible in `fail_counter`, and that has to be said.** The CREATE it issues is
*meant* to fail on the slave, and a failed statement replay is what that counter counts: `pmha-n2`
went from 22 to 24 across the runs that repaired. So a reader who finds a non-zero `fail_counter`
after a run of this suite has to check `slave-stranded.tsv` before blaming the engine — which is the
same reason the file exists. Everything this directory says about `fail_counter` not moving is about
what the *engine* does with a divergence; this is the suite moving it on purpose.

**The record is the point, not the repair.** An object a slave holds and the master does not is
evidence that something did not arrive — which is what a run is for. So each one is printed as it
happens, attributed to the case that left it, written to `ha_repl_differences/slave-stranded.tsv`,
and counted at the end, beside but not inside the `differ` tally. Repairing quietly would delete,
once per case, the very thing the suite is looking for, and no reader afterwards could tell a
divergence the corpus intended from one it caused by accident.

## Three runner defects this found on the way

**The conversion was hiding the case that causes it.** `add_primary_key` appends
`tk_repl_key INT AUTO_INCREMENT PRIMARY KEY` to a `CREATE TABLE` with no key — and a class can have
only one AUTO_INCREMENT attribute, so a table that already had one produced a CREATE the engine
refused, a case with nothing to work on, and two empty answers that agree. `cubridsus-965.sql`
makes this exact divergence and the suite called it `same`. **96 of `_01_object`'s 3,327 cases and
2 of `_33_elderberry`'s 131** hold such a CREATE. Those tables now keep no key, and their reads come
back `unreplicatable`, which is the honest answer.

**A SELECT that takes a serial's next value is not a read.** `select se1.next_value from db_root`
advances the serial, so running the same text on the standby asks a read-only node for a write. Five
cases of `_05_serial` came back `differ` with a value on the master and an empty answer on the
slave, and not one was about replication. They are writes now: run on the master, waited for, not
compared.

**And the read that would have caught this was being skipped for a quoted name.** The
keyless-table check matches names anywhere in a statement, so
`select class_name, owner_name from db_class where class_name='xxx'` counted as a read of the
keyless table `xxx` and was not compared — while it is a read of the catalog, which replicates as
DDL. That is the read that sees master PUBLIC and slave DBA. A name inside a string literal is a
value now; a read that selects *from* a keyless table is still skipped, which is what that check was
written for.

**All three were hiding the same fact, in three different ways**, which is the argument for the run
that found them: the conversion stopped the case from running, the quoted name stopped its read
from being compared, and the serial reads filled the difference list with something else.

## What is not claimed

One build, two pairs, one host. Whether `change_owner` behaves this way for a class owned by
someone other than DBA to begin with, across a failover, or for the other `db_root` methods this
corpus calls, is unmeasured. Whether the absence of a primary key on `_db_class` is deliberate is
not something this document can say — only that it is not an oversight of the same shape as
`_db_trigger`'s, because the index that is there was chosen and a class's name is not unique
without its owner.
