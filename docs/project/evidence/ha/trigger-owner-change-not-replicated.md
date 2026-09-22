# `change_trigger_owner` does not replicate, and its three neighbours do

- **Date:** 2026-09-22
- **Status:** measured and discriminated; **mechanism not established**. Whether it is known is
  not something this document can say.
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

## Where the source was followed to, and where it stopped

`au_change_trigger_owner` and `au_change_serial_owner` both live in
`src/object/authenticate_owner.cpp` and both edit a catalog object — the serial through a
`DB_OTMPL`, the trigger through `obj_get`/`obj_set` on `_db_trigger`. The method entry points are
in `src/compat/db_method_static.cpp`; the DDL paths call the same two functions from
`src/query/execute_statement.c` (3239 and 7515).

`_db_serial` and `_db_trigger` are both system classes with no primary key, so neither replicates
as **data** — which means the serial's change must be reaching the slave by some other route, and
that route is what would explain the difference. **It was not found**, and guessing at it would
be worse than saying so.

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
