# `ha_repl` over 131 cases: what the suite establishes, and what stops it

- **Date:** 2026-09-22
- **What this is:** the first sample wide enough to say anything about the corpus rather than
  about a directory. Two runs, per-case verdicts identical.
- **Trees:** engine built here from `cubrid/cubrid` develop · corpus `cubrid-testcases`
  `sql/_33_elderberry` (131 cases) · pair from `cubrid-cluster-sandbox`, rootless podman, one host.
- **Runner:** testkit's `ha_repl`, `add_primary_key=yes`, `reset=case`. No comparison against CTP.

---

## The run

| | |
|---|---:|
| cases | **131** |
| same | **119** |
| differ | **1** |
| unreplicatable | 1 |
| no_data | 10 |
| skipped | **0** |
| statements | 3,758 |
| reads compared across the pair | **765** |
| reads skipped for want of a primary key | 4 |
| statements rewritten to carry a generated key | 701 |
| writes the engine refused | 291 |
| **agreeing reads where the master returned no rows either** | **110** |
| time spent waiting for replication | 1 m 53 s — and never slept |

Two runs, 131 verdicts each, **identical**.

## Against the same 131 cases before this week's changes

| | first-column key | a key of its own, blocks kept whole |
|---|---:|---:|
| same | 106 | **119** |
| skipped | 15 | **0** |
| reads compared | 682 | **765** |
| differ | 0 | **1** |

The fifteen skips were cases whose text contained `create trigger`, `create procedure` or
`create function`, refused wholesale by a splitter that could not read a block body. Tracking
blocks instead of refusing them recovered all fifteen — **and one of them is where the single
difference comes from**, which is the argument for having bothered.

## The 110 is the number to read twice

Of 765 reads the two nodes agreed on, **110 agreed about nothing**: the master returned no rows
either. Two empty answers are equal, so they count as agreement and they establish nothing.

They are the corpus's own, not the conversion's — measured on
`_06_manipulation/_04_insert`, the count does not move between a run with the conversion and one
without. A case that creates a table, inserts into it, reads it back and drops it leaves a great
many reads whose answer is empty by the time this suite asks.

So the honest reading of "119 same" is: 765 reads compared, 655 of them about something.

## The one difference: a trigger's owner

`cbrd_23844/cbrd_24195/cases/change_owner4.sql`, reading `_db_trigger`:

| | unique_name | owner.name |
|---|---|---|
| master | `'u1.trig2'` | `'U1'` |
| slave | `'dba.trig2'` | `'DBA'` |

The owner change did not reach the slave. What makes it worth a line rather than a shrug is the
contrast inside the same directory: `change_owner1` and `change_owner2` use
`alter table t1 owner to u1` — a DDL statement — and both come back `same`. This one uses
`call change_trigger_owner ('trig2', 'u1') on class db_root`, a **method call**.

**Suspected and not established:** that an owner change made through a method does not replicate
where the DDL form does. A minimal reproduction was attempted and did not settle it — the method
returned `Trigger "dba.tg9" was not found` and the trigger then disappeared from both nodes, which
is its own question. Recorded here as observed, with the case named and both answers on disk.

## What is not claimed

One directory, one pair, one host, one build. 131 of 17,447 cases.
