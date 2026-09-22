# `change_trigger_owner` does not replicate, and its three neighbours do

- **Date:** 2026-09-22
- **Status:** measured, discriminated and **explained**. It is the primary-key rule again, one
  layer down — in the catalog this time. Whether the catalog's shape is deliberate is not
  something this document can say.
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

One pair, one host, one build. Whether the same holds for `change_owner` on a class, for a
trigger owned by a user other than DBA to begin with, or across a failover, is unmeasured.
