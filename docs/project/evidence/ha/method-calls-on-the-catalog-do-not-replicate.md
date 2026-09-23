# A method call on a catalog class does not replicate, and one of them makes users

- **Date:** 2026-09-23
- **Status:** measured three times over, with the discriminating counter-example. **This is the
  rule the other findings in this directory are instances of**, and the scale run is what showed
  it is a family rather than a curiosity.
- **What this is:** `_01_object`, 3,327 cases, and then `_10_system_table` measured again on a
  clean pair: **11 differences in 244 cases**, and the same sentence explains all of them.
- **Trees:** engine `11.5.0.2513-5f3a30d` from `cubrid/cubrid` develop · pairs `pmha` and `gbha`
  from `cubrid-cluster-sandbox`, rootless podman, one host.

---

## The rule

CUBRID HA has two channels and a catalog method uses neither.

| | |
|---|---|
| **statement replication** | the applier replays a DDL statement's own text (`la_apply_statement_log` → `la_update_query_execute`, `log_applier.c:5657`). `call … on class …` is not DDL and has no text to replay. |
| **data replication** | the log is written where the **primary key** is fetched (`repl_add_update_lsa`, `replication.c`). A catalog class without a primary key carries nothing. |

So a change made by calling a method on a catalog class reaches the slave only if that class has a
primary key. Three of them do not, and the one that does is the proof.

| what changes it | the catalog it edits | its key | reaches the slave |
|---|---|---|---|
| `alter table t owner to u` | — (DDL) | — | **yes**, as a statement |
| `create user u` | — (DDL) | — | **yes**, as a statement |
| `call change_serial_owner (…) on class _db_serial` | `_db_serial` | `pk_db_serial_unique_name`, **primary** | **yes**, as data |
| `call change_trigger_owner (…) on class db_root` | `_db_trigger` | none at all | **no** |
| `call change_owner (…) on class db_root` | `_db_class` | `i__db_class_unique_name`, an **index** | **no** |
| `call add_user (…) on class db_root` | `_db_user` | `u__db_user_name`, **unique, not primary** | **no** |

`_db_user`'s index is added as `DB_CONSTRAINT_UNIQUE` (`authenticate_context.cpp:400`), and unique
is not enough: the replication log is generated where a *primary* key value is fetched.

## The newest instance, and the sharpest

```sql
create user ddl_user;                          -- master: DDL_USER   slave: DDL_USER
call add_user ('meth_user') on class db_root;  -- master: METH_USER  slave: (absent)
```

Two ways to make a user, one line apart, and only one of them arrives. **A user made by the method
exists on the master alone** — so does every grant that rests on it, and a failover to a slave that
never heard of it loses both. Nothing reports this: `fail_counter` stays at zero, the applier stays
`working`, and `cubrid heartbeat status` shows a healthy pair, which is the third time this
directory has had to write that sentence.

## What it looks like at scale

`_10_system_table`, 244 cases, clean pair, **11 differences** — and they are this rule and its
consequences:

| case | the read that differs | why |
|---|---|---|
| `_021_db_authorizations/1001-1005` | `select name from db_user`, `select … from _db_user` | `call add_user (…)` / `call drop_user (…)`: the user is on one node |
| `_021_db_authorizations/1006` | `select class_name, owner_name from db_class` | `call change_owner (…)` |
| `_022_db_trigger/1001` | `select owner, name from _db_trigger` | a trigger made by a user the slave does not have |
| `_004_db_attribute/1003, 1007` | `select … from db_attribute` | **not this rule** — a `CREATE` whose text names a csql variable, [`ddl-with-a-session-variable-does-not-replay.md`](ddl-with-a-session-variable-does-not-replay.md). Re-measured with a control and there is no user, owner change or method call in either case |
| `_012_db_auth/1011, 1015` | `select * from dba.test_class` | **that read was not the engine** — the two nodes ran it as different users; see below. 1011 still differs on a later `db_auth` read, which is |

## The two `_012_db_auth` cases are about this suite, not the engine

Their difference is inverted: the **master** returns nothing and the **slave** returns two rows.

The case does `call add_user ('test_user') …`, grants on a table, then `call login ('test_user')`
and reads. This runner replays session statements in front of every batch, because a csql process
does not outlive one (`batch.go`). On the master the login succeeds and the read runs as
`test_user`; on the slave the user was never created, the login fails, and the read runs as **dba**.
Two different users asked the same question, and the answers differ for that reason.

So the oracle has a precondition nobody had written down: **it compares two nodes only while the
same session can be established on both.** A prelude statement that fails on one node makes the
comparison meaningless, and this suite was reporting the result rather than the precondition.

**Fixed, 2026-09-23.** `SELECT CURRENT_USER` is the last statement of both batches, and a segment
whose two answers disagree about it is skipped, counted, and named — a new outcome
`session_differs` for a case where nothing else was comparable, and a line in the tally otherwise.
Re-measured on the two cases: `_012_db_auth/1011` no longer differs over `select * from
dba.test_class`, which is the read that ran as two different users. It still differs, on a later
read of `db_auth` — the grants really are on one node only, because the grantee is — and that one
is the engine.

## What this cost before it was understood

Three of the runner's own defects were hiding it, and each is fixed and tested:

| | |
|---|---|
| the key conversion added a second `AUTO_INCREMENT` | 96 cases of `_01_object` produced a refused CREATE and then agreed about nothing |
| a keyless table's name **in quotes** counted as a read of it | the catalog read that sees the difference was skipped |
| a `SELECT` of a serial's `next_value` was compared across the pair | five `_05_serial` cases reported `differ` over a write a standby refuses |

And one more, which cost the tail of the 3,327-case run: a **user trigger** — `create trigger t
after rollback …`, no target class — cannot be dropped by DBA (*Not authorized to access trigger
"test_user.test_trigger"*), so the reset left it and its owner behind and 65 cases ran from a state
the run did not choose. The reset now drops such a trigger as its owner.

## Why this was not found before

Not for want of looking. CTP's ha_repl converts the sql corpus before it runs it, and the
conversion **deletes every statement that starts with `CALL`** — 1,707 of them in this corpus, all
of them the route this finding is about
([`ctp-ha-repl-deletes-the-call.md`](ctp-ha-repl-deletes-the-call.md)). The rule dates from 2012,
when `CALL` meant a method on a class; it also takes 10,257 procedure calls with it, which is a
second hole and a larger one.

## What is not claimed

One build, two pairs, one host. `call drop_user` was exercised only as part of the corpus, not
isolated the way `add_user` was. Whether the same holds for the other `db_root` methods this corpus
never calls is unmeasured, and **whether any of this is deliberate is not for this document to
say** — only that the catalog decides it, and that the catalog says three different things about
three classes that are edited the same way.

## The judgement: one issue, to be filed — 2026-09-23

**Decided: file it, as a single CBRD issue covering all three.** They are one sentence with three
instances, and the thing worth reporting is that the catalog says three different things about
three classes that are edited the same way. `_db_trigger` goes in as the *resolved* case —
CBRD-27302 gives it a key — which is what makes the other two answerable rather than arguable.

What separates this from the trigger's "passed by":

| | trigger | class | **user** |
|---|---|---|---|
| a key is coming | **yes**, CBRD-27302 | no | no |
| anything emits the call | no — the one dump that did is dead code behind `ENABLE_UNUSED_FUNCTION` | nothing in the tree emits `call change_owner` | **yes, and it is live** |

The user row is the reason to file. `au_export_users` emits
`call [add_user]('%s', '') on class [db_root]` and `call [add_member](…)`
(`authenticate_migration.cpp:251, 355`), and it is called from **unloaddb**
(`unload_schema.c:1438`, `:5213`) with no guard. So a schema unloaded and loaded onto a master in
an HA pair creates its users and their group memberships **on the master alone**, with every grant
that rests on them, and a failover loses all of it while every gauge reads healthy.

**Not drafted yet, and for one reason only:** the JIRA drafting skill's own rule book
(`issue/methodology/jira-writing.md` and its siblings) is not on this machine, and the skill says
to halt rather than draft without it. Everything the draft needs is here:

| the form asks for | it is |
|---|---|
| type | Correct Error |
| test build | `CUBRID 11.5 (11.5.0.2513-5f3a30d)`, 64-bit, Linux |
| repro | the two lines under *The newest instance*, plus the four-statement one in the class finding |
| expected / actual | the `create user` vs `call add_user` table above |
| components | `Catalog`, and HA if the taxonomy has it |
| affects versions | **TBD — needs a branch sweep.** Only develop at `5f3a30d` is measured |
| body language | mixed: Korean `*Summary*`, English body (chosen 2026-09-23) |
| related | CBRD-27302 (the trigger half, already being fixed) |

## The three instances in full

- [`trigger-owner-change-not-replicated.md`](trigger-owner-change-not-replicated.md) — `_db_trigger`,
  no index at all. Judged: passed by, because CBRD-27302 gives it a key.
- [`class-owner-change-not-replicated.md`](class-owner-change-not-replicated.md) — `_db_class`, an
  index that is deliberately not a key. Awaiting a judgement, and it cannot have the trigger's.
- this document — `_db_user`, unique but not primary. **Also awaiting a judgement**, and the one
  with authorization behind it.
