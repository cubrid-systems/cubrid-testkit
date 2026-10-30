# Parallel apply reorders the marker, and the CREATE TABLE is why the wait still holds

- **Date:** 2026-10-29
- **What this is:** CBRD-27508 lets the slave apply transactions on several workers, keeping
  conflict order and nothing else. Every replication wait this project knows reads "the marker
  arrived" as "everything committed before it arrived". This measures whether that still holds,
  and for which shape of marker.
- **Trees:** engine CUBRID/cubrid #8052 at `bf560620e` (`feature/CBRD-27508-parallel-applylogdb-writeset`,
  base `develop`), RelWithDebInfo, on every node · cluster-sandbox `4ba7e8c` · testkit `bf7c19c`.
- **Pairs:** two csb pairs on one host's bridge network, each with a client node: `wsp4` with
  `ha_apply_worker_count=4`, `wsp1` with `ha_apply_worker_count=1`. `cubrid applyinfo` on each
  slave reports the worker count it was given.

---

## 1. Three waits, two shapes

| wait | marker | shape |
|---|---|---|
| `sandbox.WaitForReplication` — and so testkit's `ha_repl`, which waits with it (`case.go`, `reset.go`) | `CREATE TABLE tkrepl_<ns>` + `INSERT`, dropped after (`replication.go`) | **DDL** |
| CTP's `wait_for_slave` | `create table wait_for_slave` + `insert`, dropped after (`CTP/shell/init_path/make_ha_upper.sh:74-86`) | **DDL** |
| CTP's `ha_repl` flag | `UPDATE QA_SYSTEM_TB_FLAG set v=<n>` on a table created once for the run (`Test.java:755-758`, `:930`) | **DML only** |

## 2. The result

Each round commits T_big — 20,000 rows with a secondary index and a 1,500-byte column, in one
transaction — then a marker from a second session. On the slave **one SELECT reads the marker and
T_big's row count from the same snapshot**, so "marker present, count 0" is the marker overtaking
T_big and cannot be two reads racing each other. Run twice, the second time with the script as
published beside this file:

| workers | marker | rounds | marker visible before T_big |
|---:|---|---:|---:|
| 4 | `INSERT` into a table that already exists | 20 · 20 | **18 · 17** |
| 4 | `CREATE TABLE` + `INSERT` | 20 · 20 | **0 · 0** |
| 1 | `INSERT` into a table that already exists | 20 · 20 | **0 · 0** |

**Independent transactions are reordered, as designed.** The gate says so in its own header:
"Only conflict order is enforced: independent transactions are dispatched at once and commute, so
the slave converges on the master without keeping commit order" (`log_applier_gate.cpp`). A marker
that shares no key with T_big is free to finish first, and on four workers it nearly always did.

**A marker that creates its table is not reordered.** A replicated statement has no row image to
build a writeset from, so it falls back to commit order (`repl_log_insert_statement`,
`replication.c`). The master then labels it with the previous commit and raises the history floor
to itself (`wset_find_dependency_from_history`, `wset_note_commit_with_log_lock`): the CREATE waits
for everything before it, and everything after it waits for the CREATE. The INSERT that follows is
behind that floor, so seeing it means T_big was applied.

## 3. What follows

- **`sandbox.WaitForReplication` is correct on this build**, and for a reason it did not know it
  had. Its comment now says the CREATE is load-bearing. Measured, not read.
- **CTP's `wait_for_slave` has the same shape, so the same argument covers it.** Read, not run.
  CBRD-26482's thread asks about the files that call it — 142 in `cubrid-testcases-private`, by
  the count given there — and this is the answer the engine's design gives: the marker is safe
  while it creates its table and while the engine applies a replicated statement in commit order.
- **CTP's `ha_repl` flag is the shape that overtakes.** It is appended after the case's statements
  and shares no key with them, so on a parallel applier it can be visible on the slave before the
  case's own writes are, and the dump comparison that follows can then read the case as a difference.
  Not run: CTP's `ha_repl` reaches its nodes over SSH and a csb node runs no sshd (ADR-022).
- **The second condition is the engine's to keep, not this project's.** It is how #8052 handles
  DDL today, not a property CUBRID documents. If a later revision gives DDL a finer dependency, a
  marker that creates a table stops being a barrier.

## 4. The test that holds it

`TestLiveTheWaitCoversWhatWasCommittedAheadOfTheMarker` (`internal/sandbox/integration_test.go`)
commits a bulk transaction, calls the wait, and reads the slave's count of that transaction at once.

- **It first proves the pair can tell.** An INSERT-only marker has to overtake a bulk transaction
  at least once in five rounds, read in one SELECT as above. On `wsp1` it never did and the test
  skips, saying why; on `wsp4` it overtook in round 1 every time.
- **It was checked against the change it guards**, by turning the marker into an INSERT on a kept
  table. With a 20,000-row transaction the test failed in **one run of three**: it can only read
  after the wait has returned, and the wait drops its marker first, which gave the slave time to
  finish. With 60,000 rows of 500 bytes it failed in **three runs of three**, in rounds 3, 1 and 1,
  and that is the size it uses.

## 5. What this does not establish

- That a serial applier never reorders. One worker kept order in 40 rounds of this one shape; the
  developer's own note in CBRD-26482 says a scenario that breaks order on one thread may exist.
- Anything about conflicting transactions. These rounds share no key on purpose. Whether the
  writeset finds every dependency it should is a different question, and a marker cannot ask it;
  [parallel-apply-keeps-conflict-order](parallel-apply-keeps-conflict-order.md) asks it of
  thirteen patterns.
- Restart. A slave restarted mid-stream re-applies from `committed_lsa` and skips by the markers it
  stored; nothing here stops an applier.

## 6. Reproducing it

```bash
B=/path/to/cubrid-pr8052/install.out
T=/path/to/tools        # holds parallel-apply-marker-probe.sh, from beside this file
csb cluster create --cluster wsp4 --build $B --preset ha --clients 1 --tools $T \
    --set-hidden ha_apply_worker_count=4
csb node exec wsp4-c1 --cluster wsp4 --timeout 20m -- \
    sh /tools/parallel-apply-marker-probe.sh wsp4 wsp4-n1 wsp4-n2 20 20000 dml   # then ddl
```

The script writes one line per round to `/results/probe-<kind>.tsv`; column 4 is T_big's row count
at the moment the marker was first seen, so any value other than the round's row count is a marker
that overtook it. It stops at the first write the master refuses.

The live test runs against any pair, and skips on one that does not reorder:

```bash
TESTKIT_CSB=extensions/cluster-sandbox/bin/csb TESTKIT_CSB_CLUSTER=wsp4 \
    go test ./internal/sandbox/ -run TestLiveTheWaitCoversWhatWasCommittedAheadOfTheMarker -v
```
