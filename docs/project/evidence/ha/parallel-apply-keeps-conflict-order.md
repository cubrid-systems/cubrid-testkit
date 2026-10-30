# Thirteen conflicting pairs, and the parallel applier kept every one in order

- **Date:** 2026-10-29
- **What this is:** CBRD-27508 applies on several workers and holds a transaction back only until
  what the master's writeset says it conflicts with has been applied. If the writeset misses a
  dependency, the slave applies the later transaction first. This asks, pattern by pattern, whether
  it does — with a control that shows the setup would have caught it.
- **Trees, pair:** as in [parallel-apply-and-the-marker](parallel-apply-and-the-marker.md): engine
  #8052 at `bf560620e`, RelWithDebInfo, on `wsp4` (`ha_apply_worker_count=4`) with a client node.

---

## 1. How a round asks

T1 inserts 20,000 ballast rows and **then** does the pattern's first operation, in one
transaction. T2, committed after it from another session, does the dependent operation and inserts
a witness row. On the slave one SELECT reads the witness and T1's ballast count from the same
snapshot: **witness present with count 0 is T2 applied ahead of T1.**

The order inside T1 is the point. The slave applies T1 on one worker for as long as the ballast
takes, and T1 touches the shared key only at the end; a T2 that slipped past the gate would reach
that key first rather than queue behind a lock T1 already holds.

The control, `ctl`, is the same round with nothing shared: T2 is free to go first, and it did in
**20 of 20** rounds. So a missed dependency in any pattern below had about that chance per round of
showing.

## 2. The result

20 rounds a pattern. "Final" compares the patterns' own tables across the pair behind a
`CREATE TABLE` marker; the last column is `db_ha_apply_info.fail_counter`'s movement over the pattern.

| pattern | T1 ends with | T2 | key it shares | T2 ahead | final | fail |
|---|---|---|---|---:|---|---:|
| `ctl` | — | insert an unrelated row | none | **20** | same | 0 |
| `pk_update` | update row k | update row k | PK | 0 | same | 0 |
| `pk_move` | change k's PK to k+10⁸ | insert PK k | PK, old value | 0 | same | 0 |
| `uk_reinsert` | delete the row with unique u | insert another row with u | UNIQUE | 0 | same | 0 |
| `uk_move` | move row a's unique value off u | set row b's to u | UNIQUE, old value | 0 | same | 0 |
| `rk_reinsert` | as `uk_reinsert` | | REVERSE UNIQUE | 0 | same | 0 |
| `fk_insert` | insert parent p | insert child of p | FK as REF | 0 | same | 0 |
| `fk_delete` | delete p's child | delete parent p | FK as REF | 0 | same | 0 |
| `fk_composite` | insert parent (a, b) | insert child of (a, b) | composite FK | 0 | same | 0 |
| `fk_cascade` | insert child of p | delete p, cascading | parent p, as T1's REF — so it does not test whether the cascade's own deletes are in the writeset; `fk_cascade_reuse` below does | 0 | same | 0 |
| `sp_keep` | insert k; savepoint; update k; roll back to it | update k | PK written before the savepoint | 0 | same | 0 |
| `sp_redo` | savepoint; insert k; roll back to it; insert k again | update k | PK written after the rollback | 0 | same | 0 |
| `ddl_dml` | `ALTER TABLE … ADD COLUMN` | insert into that column | none — commit order | 0 | same | 0 |
| `part_move` | move k to the other range partition | insert k | PK across partitions | 0 | same | 0 |

**No pattern was applied out of order, no table differed, and the applier failed nothing**: 5,628
commits applied since the applier started — this run's and the marker measurements' together —
with `fail_counter` at 0 and the applier's error log unchanged since its startup lines. The master
refused none of the writes.

Two patterns were added later, 10 rounds each on the same build: `fk_repoint` (a child moved off
parent X, then X deleted — held by a REF conflict on the old value) and `fk_cascade_reuse` (X
deleted, cascading child r away, then a new child r under Y — held by a write conflict, so the
cascade's deletes are in the writeset). Both in order; the control went first 10 of 10.

**Keys the index holds equal that the writeset saw as different** (2026-10-30). Every pattern above
that shares a key shares an integer, written the same way by both transactions and through the same
class. #8052 then moved to `61ee0944a`, whose new
commits hash a FLOAT or DOUBLE -0.0 as +0.0, hash TIMESTAMPTZ and DATETIMETZ keys without their zone,
and take the class OID out of the key hash. Eight patterns pair T1 and T2 on keys of that kind: T1 frees
or adds a key written one way, T2 takes or refers to it written the other way. 20 rounds each, 4 workers.

| pattern | T1's key / T2's key | `bf560620e`: T2 ahead, fail, final | `61ee0944a`: T2 ahead, conflicts found |
|---|---|---|---|
| `fp_negzero` | UNIQUE (g, d): (r, -0.0) / (r, +0.0) | 17, 17, differ | 0, 20 write |
| `tstz_zone` | TIMESTAMPTZ UNIQUE: 00:00:r +00:00 / 09:00:r +09:00 | 18, 18, differ | 0, 20 write |
| `dttz_zone` | the same in DATETIMETZ | 20, 20, differ | 0, 20 write |
| `inherit_uk` | UNIQUE on a superclass: the subclass's row / the superclass's | 18, 18, differ | 0, 20 write |
| `fk_negzero` | parent (r, -0.0) / child refers to (r, +0.0) | 16, 18, differ | 0, 20 write |
| `fk_tz` | parent at +00:00 / child refers to the instant at +09:00 | 18, 19, differ | 0, 20 write |
| `ci_case` | `utf8_en_ci` UNIQUE: 'Keyr' / 'KEYr' | 0, 0, same | 0, 20 write |
| `fk_ci` | the same as a reference | 0, 0, same | 0, 20 write |

On the old head the writeset missed all six: the five whose bytes differ and the key written through a
subclass. T2 was applied first, failed on the
slave, and the pair diverged. The case-insensitive collation was already handled. On the new head all
eight are found as write conflicts, and the fifteen earlier patterns, run again on it, kept their order
with `fail_counter` 0; the control went first 19 of 20 on both heads. Not covered: an accent-insensitive
collation (`utf8_gen_ai_ci` is not in this build), unique function-based and filtered indexes (the
`CREATE UNIQUE INDEX` statements tried were refused with "invalid create index"), and trailing spaces,
which `utf8_bin` does not treat as equal.

## 3. What this does not establish

The first four items below were asked afterwards, in
[parallel-apply-under-pressure](parallel-apply-under-pressure.md), and none broke the applier.

- **The history map cleared by capacity, and the key-limit fallback.** Both are reached by a small
  `ha_writeset_history_size`; this pair ran at the default of 1,000,000 and never filled. It was
  cleared only by DDL — the barriers between rounds and `ddl_dml`'s ALTER — as every replicated
  statement clears it.
- **More than two transactions at once.** Every round is one pair, committed one after the other.
  Several clients interleaving on a shared key space is what the corpus's isolation cases do on one
  node, and it is not here yet.
- **Assertions.** A RelWithDebInfo build compiles them out; the applier's own `assert`s — there are
  several on `required_lsa` and the frontier — were not running.
- **Restart.** No applier was stopped.
- **Visibility order** beyond what `ctl` shows: independent transactions overtake by design, which
  is the subject of [parallel-apply-and-the-marker](parallel-apply-and-the-marker.md).

## 4. Reproducing it

On a pair made as the marker document says, with
[`parallel-apply-pairs.sh`](parallel-apply-pairs.sh) in the tools directory:

```bash
csb node exec wsp4-c1 --cluster wsp4 --timeout 90m -- \
    sh /tools/parallel-apply-pairs.sh wsp4 wsp4-n1 wsp4-n2 20 20000 \
    ctl pk_update pk_move uk_reinsert uk_move fk_insert fk_delete \
    rk_reinsert fk_composite fk_cascade sp_keep sp_redo ddl_dml part_move
```

It prints one line per pattern, and a `master refused:` line on standard error for any write the
master would not take.
