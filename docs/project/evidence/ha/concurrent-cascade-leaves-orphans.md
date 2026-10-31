# CBRD-27464 across a pair: the master's orphan child is a row the slave does not have

- **Date:** 2026-10-29
- **What this is:** found while pressing CBRD-27508's parallel applier with several clients
  ([parallel-apply-under-pressure](parallel-apply-under-pressure.md)), and **already on the
  tracker** as CBRD-27464, "ON DELETE CASCADE decides on the children it read before waiting, not
  the ones there after" (in development; fix in CUBRID/cubrid #8026, open). That issue describes the
  master. What this adds is what it does to an HA pair, which the issue does not mention: in its
  case A the pair diverges, silently. Not introduced by #8052 — the develop it was branched from
  does the same.
- **Trees:** engine CUBRID/cubrid develop at `3ca42d9` (the merge base of #8052; #8026 not in it)
  and #8052 at `bf560620e`, both RelWithDebInfo. cluster-sandbox `4ba7e8c`.

---

## 1. Two sessions, in CBRD-27464's order

Session 1 opens a child write and holds it four seconds; one second in, session 2 deletes the
parent (`ON DELETE CASCADE`), waits on session 1, and commits after it. The pair is then waited
for with a `CREATE TABLE` marker ([`fk-cascade-cbrd-27464-ha.sh`](fk-cascade-cbrd-27464-ha.sh)).

| case | session 1 | master after | slave after |
|---|---|---|---|
| A | insert child 21 under parent 1 | parent 1 gone, **child (21, 1) kept** — an orphan | **no child** |
| B | move child 10 from parent 1 to parent 2 | child 10 **deleted** though it had left | child 10 deleted |

The same on develop and on #8052 with four workers. In both cases the DELETE waited three seconds;
the sessions' error output was not kept for these runs.

**Case A diverges the pair without a word.** The master's cascade decided on the children it read
before waiting and kept child 21; the slave does not have it. Case B is wrong on both nodes alike.
How the slave comes to drop 21 was not traced. A slave that ran the cascade again against what it
had by then applied would do exactly this, and it fits the other observation that a cascade inside
one transaction raises no apply error (§3) — a reading, not a measurement.

## 2. What it looks like under load

[`parallel-apply-stress.sh`](parallel-apply-stress.sh) with operations `5,6,7,8` only — insert a
parent, delete a parent, insert a child, move a child — on `s_parent (p PK)` and
`s_child (c PK, p FK → s_parent ON DELETE CASCADE)`. 8 clients, 200 transactions each, 30 keys,
1–3 operations a transaction, about 30% of them after 1,000 ballast rows.

The seed in these runs labels a run and does not name its SQL: mawk's `srand (n)` does not repeat its
sequence, so the rows below are independent runs under one setting, not one workload on several builds.
The script's generator has since been replaced by a sequence of its own.

"Orphans" are child rows whose parent does not exist, counted after every client has finished
and the pair has been waited for. The count also takes in a child the heap has and no index has, the damage
in [fk-index-insert-loses-the-victims-error](fk-index-insert-loses-the-victims-error.md): it counted those
rows there, one and two of them;
these runs did not run checkdb, so some orphans below may be that rather than CBRD-27464. The `s_child`
comparison reads through the primary key and is not affected: a row it reports is in the index. "During" counts the polls of the master, while the clients ran,
that found at least one — an observation, not a validated measurement.

| build | applier | runs | `fail_counter` moved | `s_child` differs | master orphans at the end | master orphans during |
|---|---|---:|---:|---:|---:|---:|
| develop `3ca42d9` | serial, the old one | 9 | 8 | **4** | **4 runs, 1–3 rows** | 9 runs of 9 |
| #8052 | 1 worker | 12 | 12 | 0 | 0 in the 6 runs counted | 3 runs of 3 counted |
| #8052 | 4 workers | 6 | 6 | 1 | not counted | not counted |
| #8052 | 4 workers, every operation | 5 | 1 | 2 | not counted | not counted |

**Every divergence is one child row the master has and the slave does not** — exactly one, in each
of the seven diverged runs whose tables were kept: case A's shape. Five of the seven had applier
failures; two had none (#8052 on four workers, seed 13; develop, seed 45), so `fail_counter`
alone does not catch it.

Every applier failure that reached the error log is `failed to apply delete replication log` or
`failed to apply update replication log`, class `dba.s_child`, server error -711, "An object with
the supplied attribute value doesn't exist" — the slave asked to change a child row it does not
have, which is what a later statement on a case-A orphan would meet. On four workers that is all
of them, 49 lines against a `fail_counter` of 49; on one worker and on develop the log held fewer
lines than `fail_counter` counted, and what the rest were is not known.

On #8052 the six one-worker runs whose end state was counted left no orphan on the master, and
three runs in eleven on four workers still diverged. Whether four workers make it more frequent
than one is not settled: 3 in 11 against 0 in 12, Fisher's exact test one-sided p ≈ 0.09, and
develop differs from #8052 in more than the applier.

## 3. What was ruled out on the way

- **The writeset.** Two directed pairs that would have shown a missed dependency kept order 10
  rounds of 10 — a child moved off a parent before the parent is deleted (`fk_repoint`, held by a
  REF conflict on the old parent), and a child re-created under another parent after a cascade
  removed it (`fk_cascade_reuse`, held by a write conflict on the child's key)
  ([`parallel-apply-pairs.sh`](parallel-apply-pairs.sh)).
- **A transaction's own order.** Three single-session variants — move a child then delete its old
  parent; delete a parent then re-create its child elsewhere; delete and re-create both — in one
  transaction each, 20 rounds each, on #8052 with one worker: every table the same,
  `fail_counter` 0 ([`fk-cascade-one-transaction.sh`](fk-cascade-one-transaction.sh)).
- **The opposite order of the two sessions.** With the parent DELETE open first and the child
  write after, the child write waited and failed on the foreign key, and nothing was left behind —
  by UPDATE and by INSERT, one trial each, on #8052
  ([`fk-cascade-two-sessions.sh`](fk-cascade-two-sessions.sh)). This is the order the engine
  handles; an earlier version of this note took it for "the plain race" and ruled that out. The
  defect is the other order, §1.

## 4. What follows

- **It is CBRD-27464, not a new issue.** The HA consequence of case A — a silent divergence, and
  -711 apply failures on whatever later touches the orphan — is worth adding to it.
- **#8026 fixes it.** On develop `3ca42d9` with #8026's head `6366bab3a` merged (one call adjusted to
  #8104's `heap_attrvalue_get_key ()`), case A leaves no child on either node and case B keeps child 10
  under parent 2 on both, two runs each; the DELETE still waits for session 1 (about 3 s), and neither
  session reports an error. Seven stress runs as in §2 ended with
  `fail_counter` 0, `s_child` the same on both nodes and no orphan, against develop's 8 of 9 with
  `fail_counter` moved and 4 diverged. One of them did report a single orphan while the clients ran, which
  was not looked into. An eighth run hung, and that was not #8026:
  [fk-index-insert-loses-the-victims-error](fk-index-insert-loses-the-victims-error.md).
- **CBRD-27463** (#8198, open) changes the foreign key check to take no parent row lock; §3's last
  item may behave differently once it lands.

## 5. Reproducing it

```bash
export CSB_HOME=/a/large/disk/csb
csb cluster create --cluster dev --build /path/to/develop-3ca42d9/install.out --preset ha \
    --clients 1 --tools $T
csb node exec dev-c1 --cluster dev -- sh /tools/fk-cascade-cbrd-27464-ha.sh dev dev-n1 dev-n2 insert
csb node exec dev-c1 --cluster dev -- \
    sh /tools/parallel-apply-stress.sh dev dev-n1 dev-n2 8 200 30 32 5,6,7,8 1000
```
