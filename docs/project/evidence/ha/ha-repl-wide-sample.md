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
| replicating | 10 |
| unreplicatable | 1 |
| **no_data** | **0** |
| skipped | **0** |
| statements | 3,758 |
| reads compared across the pair | **765** |
| reads skipped for want of a primary key | 4 |
| statements rewritten to carry a generated key | 701 |
| writes the engine refused | 291 |
| **agreeing reads where the master returned no rows either** | **110** |
| time spent waiting for replication | 1 m 53 s — and never slept |

Two runs, 131 verdicts each, **identical**.

**Nothing in this directory establishes nothing any more.** The ten that used to come back
`no_data` all write, and the pair carried a marker across after each of them, so they are
`replicating`: not that their data matches — they make no comparable read — but that replication
was alive while they ran. That check is taken from CTP, which asks it of every case
([`unjudged-cases.md`](unjudged-cases.md)).

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

## The 100 is the number to read twice

Of 765 reads the two nodes agreed on, **100 agreed about nothing**: the master returned no rows
either. Two empty answers are equal, so they count as agreement and they establish nothing.

It was 110 before session state was replayed into each batch. Ten of them were a case saying
`call login ('u1') on class db_user` and then reading its own table — every batch is a new csql
process, so the read was running as dba and finding nothing. That one was the runner's and is
fixed.

What the other hundred are, as far as static reading goes:

| | |
|---|---:|
| a case that writes no rows at all — a plan or catalog test | 17 |
| a case that writes, but whose particular read is empty | 83 |

The second group is a read after the case's own DELETE, a filter that matches nothing, a table
the case emptied. Chasing it further statically has stopped paying: what matters is that the
number is reported, so **"119 same" reads honestly as 765 reads compared, 665 of them about
something.**

In `_06_manipulation/_04_insert` the same number is 6 of 33, and all six are cases the corpus
marks `--[er]` — a convention `_33_elderberry` does not use at all, which is why the split is
made two ways.

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

**Settled later the same day**, and not as "a method": it is `change_trigger_owner` specifically,
because `_db_trigger` has no primary key while `_db_serial` has one —
[`trigger-owner-change-not-replicated.md`](trigger-owner-change-not-replicated.md), which also
carries the judgement (passed by; CBRD-27302 gives the catalog class its key).

## What is not claimed

One directory, one pair, one host, one build. 131 of 17,447 cases.
