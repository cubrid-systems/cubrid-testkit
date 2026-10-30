# E12 — Apply-Order Checks for a Parallel HA Applier (Requirements)

*English · [한국어](requirements.ko.md)*

**Source:** CBRD-27459 (writeset and applylogdb parallelisation, design), CBRD-27508 /
CUBRID/cubrid #8052 (implementation); the shell prototypes and their results in
`docs/project/evidence/ha/parallel-apply-*`
**Status:** incubating — **in progress** (ADR-EXT-012 proposed 2026-10-30; M1 is `pairs` and `list`)
**Axis mapping:** a new oracle — the order in which a slave applies dependent transactions

---

## 1. The problem this entry solves

With `ha_apply_worker_count` above one, the slave applies transactions on several workers and keeps
only the order between those that conflict. The master decides what conflicts: it hashes the keys a
transaction wrote and referred to into a writeset, and the slave's gate holds a transaction until
what it depends on is applied. A key the writeset hashes differently from the index loses the
dependency, and the slave applies the later transaction first: it fails there, or the pair diverges
without a word.

No existing check sees this. `ha_repl` writes in one session, in order, and compares at the end; its
cases give the applier almost no chance to reorder. The shell prototypes showed the question can be
asked: on #8052's head of 2026-10-29 they caught six kinds of key the index holds equal and the
writeset did not, which #8052 then fixed.

## 2. Why in testkit

- It is a runner: it stands HA pairs up through cluster-sandbox, drives transactions at them from a
  client node, waits, compares, and writes a ledger.
- It consumes `internal/sandbox` the way `ha_repl` and `perf` do (ADR-022), with one more create
  option (`--set-hidden`, for `ha_apply_worker_count`).
- It is a new entry point, `testkit ha-apply`, beside the frozen task names; nothing about `ha_repl`
  changes. It goes before containment, as `perf` does.

## 3. What it asks

| verb | question |
|---|---|
| `pairs` | For each pattern, T1 writes ballast and then a key; T2, committed after it, depends on that key and writes a witness. Is T2 ever applied before T1? |
| `stress` (M2) | N clients on one small key space, a deterministic workload: do the nodes end the same — through the primary key, by heap count, with no orphan and no apply failure? |
| `list` | The patterns, and what each pair shares. |

The patterns are a closed list in Go: primary, unique, reverse-unique and foreign keys (single,
composite, cascading), savepoints, DDL, partitions, and keys the index holds equal while their writes
differ (-0.0 and +0.0, one instant in two zones, a case-insensitive collation, a subclass and its
superclass). An unknown name is refused.

## 4. The verdict

- **missed** — T2 seen first in a round, an apply failure (`fail_counter` moved), tables that differ
  at the end, or a slave that stopped: its count of a round's ballast read short of the round and
  unchanged for two minutes for every 20,000 rows of ballast (two at least). T1 is one
  transaction, so that is the time the slave gets to apply it; at the default a round on the new
  head averaged 2.3 to 5.5 seconds a run. A slave the client cannot read for a minute is the run's
  failure, not the engine's.
- **held** — none of those, and the control (`ctl`, sharing nothing) went first at least once in the
  same run.
- **inconclusive** — held, but the control never went first: the pair did not reorder, so holding
  proves nothing. Also a pattern with a round whose witness did not arrive in time (T2 waiting for
  T1 is what a held dependency looks like, and a slow slave looks the same), or with a write the
  master refused.

The exit code says whether the run finished. The verdicts are in the ledger. A slave that stopped
ends the run after that pattern's row, since every later pattern starts by waiting on it.

A `stress` run has one verdict: **damaged** (a node's heap holds rows its primary key does not;
only the master's counts, when the slave is stalled and its two reads may see two states),
**stalled** (the slave did not show the final marker within five minutes; compared, but a slave
behind differs for that alone; a master that would not take the marker fails the run), **diverged** (a table read through its primary key, a heap count,
the ballast or the orphan count differs across the pair, or `fail_counter` moved), **hung** (the
clients did not finish within `--timeout` and were stopped), or **same**, the first that applies in
that order. A statement the master refused is not a finding: whatever the master ends up holding is
the answer, and the question is whether the slave holds it too. A client that ended no transaction
fails the run, since a pair nobody wrote to reads the same. `--compare-only` makes the same
comparison on the tables a pair already has, with no workload and without `fail_counter`, which has
no before; it creates and drops one marker table on the master to wait for the slave. A run keeps
its clients' scripts and logs, and both reads of any table that differs, beside its ledger, and
refuses to start where an earlier run's files already are.

## 5. Acceptance

- **M1** (`pairs`, `list`): on #8052 `bf560620e` the six equality patterns that the prototype caught
  are **missed**; on `61ee0944a` all 24 patterns are **held**, with the control going first.
- **M2** (`stress`): one seed names the shell prototype's SQL byte for byte; `--compare-only` calls
  the damaged pair of `fk-index-insert-loses-the-victims-error` (#8198 `6ce1c05d2`, the master's
  `s_child` 9 rows by its heap and 7 through its primary key) **damaged**; a run without foreign-key
  operations on #8052's new head is **same**.

## 6. Not in scope

- Changing `ha_repl`, its conf or its output.
- A case file format: the patterns are code, as `module-ha.md` concludes HA needs no new case
  language.
- Judging the engine's speed: the ballast exists to make reordering visible, not to measure.
