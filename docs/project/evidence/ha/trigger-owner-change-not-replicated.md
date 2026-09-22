# `change_trigger_owner` does not replicate, and its three neighbours do

- **Date:** 2026-09-22
- **Status:** measured, discriminated, explained — and **judged, 2026-09-22: passed by, not
  filed.** The key `_db_trigger` lacks is already on its way (CBRD-27302, PR #7980), and with a key
  the method form has a channel. It is the primary-key rule again, one layer down — in the catalog
  this time. What the judgement rests on, and what it does not, is *The judgement* below.
- **What this is:** four owner changes on one pair. Three reach the slave and one does not, and
  the one that does not is not distinguished by being a trigger or by being a method.
- **Trees:** engine built here from `cubrid/cubrid` develop · pair from `cubrid-cluster-sandbox`,
  rootless podman, one host.
- **Found by:** testkit's `ha_repl` over `sql/_33_elderberry`, the single difference in 131 cases
  (`cbrd_23844/cbrd_24195/cases/change_owner4.sql`), then isolated by hand.

---

## The four

Each is one statement on the master, followed by a wait on state — a marker written on the master
and polled on the slave — then the same catalog read on both nodes.

| what changes | how | master | slave | reaches the slave |
|---|---|---|---|---|
| a table's owner | `alter table own_t owner to u7` | `U7` | `U7` | **yes** |
| a serial's owner | `call change_serial_owner ('dba.sq7', 'u7') on class _db_serial` | `u7.sq7 / U7` | `u7.sq7 / U7` | **yes** |
| a trigger's owner | `alter trigger tg7 owner to u7` | `u7.tg7 / U7` | `u7.tg7 / U7` | **yes** |
| a trigger's owner | `call change_trigger_owner ('tg7', 'u7') on class db_root` | `u7.tg7 / U7` | **`dba.tg7 / DBA`** | **no** |

The last two rows are the same change to the same object by two routes, and only one of them
arrives. The middle two rule out the two obvious explanations:

- **not "a trigger's owner does not replicate"** — the DDL form of exactly that change does.
- **not "a method does not replicate"** — `change_serial_owner`, also a method on a catalog
  class, does.

So it is `change_trigger_owner` specifically.

## What the pair says about itself while this is true

Nothing. `fail_counter` does not move, `applylogdb` logs nothing, the applier stays `working`,
and `cubrid heartbeat status` reports one master and one slave. As with the object-domain
constraint, a monitor asking the engine how it is doing will not learn that the two catalogs
disagree.

## Why, and it is the same rule as everything else

There are **two** replication mechanisms and the four routes use them differently.

**Statement replication.** The applier replays a DDL statement's own text on the slave —
`la_apply_statement_log` ends in `la_update_query_execute (stmt_text, false)`
(`log_applier.c:5657`). So `alter table ... owner to` and `alter trigger ... owner to` arrive
because the statement arrives. A `call ... on class` is not a DDL statement and has no text to
replay.

**Data replication, which carries the primary key.** `repl_add_update_lsa`
(`replication.c`) says the log is generated at index processing *because that is where the
primary key value is fetched*. An instance update therefore replicates only if its class has a
primary key.

That is the whole of it, and the catalog decides it:

```
_db_serial    pk_db_serial_unique_name   is_unique YES   is_primary_key YES
_db_trigger   (no index at all)
```

- `au_change_serial_owner` edits an `_db_serial` instance with `dbt_edit_object` and
  `dbt_put_internal` — a normal instance update, on a class **with** a primary key. It
  replicates as data, so the method form works.
- `au_change_trigger_owner` edits a `_db_trigger` instance with `obj_set`, and `_db_trigger`
  has **no primary key**. There is nothing for data replication to carry it by.

**Both trigger routes call the same function** — `execute_statement.c:7515` for the DDL form and
`db_method_static.cpp:959` for the method — which is why the difference cannot be inside it. The
DDL form arrives on the statement channel; the method form has no channel at all.

So this is the rule this suite has been measuring since the first day of it: **no primary key, no
replication.** The surprise is only that it applies to a system catalog, and that `_db_serial`
was given a key where `_db_trigger` was not.

## The judgement: passed by, and what it rests on

**2026-09-22. Not filed as a defect.** `_db_trigger` is about to be given the key whose absence is
the entire mechanism. **CBRD-27302, [PR #7980](https://github.com/CUBRID/cubrid/pull/7980)** (open,
draft, base `develop`) changes `get_trigger`'s constraint list from `{}` to
`{DB_CONSTRAINT_PRIMARY_KEY, "", {TR_ATT_UNIQUE_NAME, nullptr}, false}`
(`src/object/schema_system_catalog_install.cpp:806`). The PR is about removing the duplicated
trigger storage in `db_root.triggers` and `_db_user.triggers`, and the key is there so that name
lookup and concurrent creation have something to go on. Replication would get it as a side effect —
the method's instance update would then have the channel `_db_serial` already replicates on.

**Two things that judgement does not rest on:**

- **It is an expectation, not a measurement.** Nothing here has been run against #7980. The check
  is the four statements below, on a pair built from a tree that has it.
- **The key may not be the whole of it.** `_db_trigger.owner` is an object-domain column
  (`AU_USER_CLASS_NAME`), and this suite's other finding is that an object-domain attribute arrives
  as a stored NULL ([`object-domain-not-replicated.md`](object-domain-not-replicated.md)). Yet
  `_db_serial.owner` is the same shape and *did* arrive — the slave read `U7`, not NULL. Why a
  catalog class's object reference survives where a user class's does not is unexplained. So the
  re-run has to read `owner.name` and not only `unique_name`.

**Why it is cheap to pass by in the meantime.** The only route to it is an explicit
`call change_trigger_owner (...) on class db_root` by a DBA. `ALTER TRIGGER ... OWNER TO` does not
compile to it, no class-owner change cascades into it — `authenticate_access_class.cpp` cascades to
serials only — and the one dump that emitted the call has been dead code behind
`#if defined(ENABLE_UNUSED_FUNCTION)` (`tr_dump_all_triggers`, `trigger_manager.c:6785`), which
#7980 deletes outright.

**What the suite does with it: nothing changes.** It keeps reporting the difference, and this is
deliberately *not* the fifth thing `ha_repl` skips. The four skips are static rules about shapes
that cannot replicate at all; `_db_trigger` is not one, because `alter trigger ... owner to`
replicates through it today. A skip on `_db_trigger` reads would suppress coverage that works in
order to hide one call that is about to start working.

## The reproduction

```sql
create user u7;
create table ta7(c1 int primary key);
create table tb7(c1 int primary key);
create trigger tg7 after insert on ta7 execute insert into tb7 values (obj.c1);
-- both nodes: dba.tg7 / DBA
call change_trigger_owner ('tg7', 'u7') on class db_root;
-- master: u7.tg7 / U7      slave: dba.tg7 / DBA
```

Read back with
`select unique_name, owner.name from _db_trigger where name='tg7'` on each node.

## What is not claimed

One pair, one host, one build — `develop` at `5f3a30d09`, which is before #7980 and so has
`_db_trigger` with no index at all. Whether the same holds for `change_owner` on a class, for a
trigger owned by a user other than DBA to begin with, or across a failover, is unmeasured.
