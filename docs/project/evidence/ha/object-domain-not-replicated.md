# An object-domain column replicates its row and not its reference

- **Date:** 2026-09-21
- **What this is:** a master/slave pair in which both rows arrive, both tables have primary keys,
  every gauge reads healthy, and one column holds a value on the master and NULL on the slave.
  Found by the native `ha_repl` runner over the `sql` corpus, isolated to four statements.
- **Trees:** engine built here from `cubrid/cubrid` develop (`/data/cub_sys/cubrid-develop`) ·
  corpus `cubrid-testcases` `sql/_06_manipulation/_04_insert` · pair provisioned by
  `cubrid-cluster-sandbox` (`csb cluster create`), rootless podman, one host.
- **Runner:** testkit's own `ha_repl`, `TESTKIT_NATIVE=ha_repl`, `add_primary_key=yes`,
  `reset=case`. No comparison against CTP: this suite's oracle is the pair.

---

## The reproduction

Four statements, on an empty database:

```sql
set system parameters 'create_table_reuseoid=no';
create class op(name varchar(20) primary key, age integer);
create class oe(id integer primary key, attr op);
insert into oe values(1, (insert into op values('xxx', 21)));
```

Then, after waiting for replication — a marker row written on the master and polled on the slave,
not a sleep:

| read | master | slave |
|---|---|---|
| `select id, attr, attr.name from oe` | `1  dba.op  xxx` | `1  NULL  NULL` |
| `select name, age from op` | `xxx  21` | `xxx  21` |
| `select count(*) from oe where attr is null` | **0** | **1** |

So the **referenced** row arrives intact, the **referring** row arrives with its key and its
ordinary columns, and the reference itself arrives as a **stored NULL** — `attr is null` is false
on the master and true on the slave.

`create_table_reuseoid=no` is not incidental: an instance of a reusable-OID class is non-referable,
so the case has to turn it off before a class can be the target of an object domain at all.

## Nothing reports it

| | |
|---|---|
| `applylogdb` error log | nothing about these rows |
| `fail_counter` in `db_ha_apply_info` | unchanged |
| applier state | `working` throughout |
| `cubrid heartbeat status` | one master, one slave, as expected |

Which is the part that matters. `fail_counter` is the separator this project's own inspection
leans on (`design/05-inspect.md` §2) and it does not move, so a monitor watching the engine's
opinion of itself sees a healthy pair over a database that differs.

## Why, from the source

Two facts from the engine tree explain it exactly.

**Replication carries the primary key, not the row's address.**
`src/transaction/replication.c` §`repl_add_update_lsa`: *"In order to reduce the cost of
replication log, we generate a replication log at the point of indexing processing step. (During
index processing, the primary key value is fetched ..)"* — which is also why a table without a
primary key replicates nothing at all.

**The slave rebuilds the row from the master's heap image, attribute by attribute.**
`src/transaction/log_applier.c` §`la_disk_to_obj` → `la_get_current` reads each attribute's disk
value with `att->type->data_readval` and puts it into a template with `dbt_put_internal`. For an
object-domain attribute that disk value is an **OID** — volume, page and slot on the *master*. It
names nothing on the slave, and what lands is NULL.

## Is it known?

Not in this tree. Searched for a guard that refuses to replicate a class with an object-domain
attribute, and for a message naming the restriction: there is none. The only "not replicated"
statement in the source is about `DROP VARIABLE`
(`src/query/execute_statement.c:17033`, and that one says *intentionally*).

Whether CUBRID's manual documents the limitation is outside this tree and is **not** established
here. What is established is that the engine does not detect it, does not report it, and does not
move the counter that exists to say the two nodes disagree.

## How it was found, and what that says about the corpus

Six cases of 39 in `_06_manipulation/_04_insert` differ, and all six are this: four cases, two
shapes — a dereference through the object (`attr.name`) and the object itself (`attr`).

They were noise until the run became reproducible. The same 39 cases gave `differ 0` one run and
`differ 6` the next, because a failed `DROP` looked like a successful one and each run started from
whatever the last one left. With the reset fixed, two runs give identical per-case verdicts and
these six are stable.

## What is not claimed

One pair, one host, one build, one directory of one corpus. Whether the same happens for a
`SET OF` an object domain, for an object domain under `UPDATE` rather than `INSERT`, or across a
failover, is unmeasured.
