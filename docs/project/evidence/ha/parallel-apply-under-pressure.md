# The parallel applier under the paths most likely to break it

- **Date:** 2026-10-29
- **What this is:** [parallel-apply-keeps-conflict-order](parallel-apply-keeps-conflict-order.md)
  left four things unasked: the history map after it is cleared, more than two transactions at
  once, assertions, and restart. This asks them, and one more that reading the commit path raised.
- **Trees:** engine #8052 at `bf560620e`, RelWithDebInfo unless a row says debug; the develop it
  was branched from, `3ca42d9`, for one comparison. cluster-sandbox `4ba7e8c`.
- **Pairs:** csb pairs with a client node, `ha_apply_worker_count=4` unless a row says otherwise.

---

## 1. The result

| path | how it was asked | result |
|---|---|---|
| **assertions** | twelve of the thirteen conflicting pairs on a **debug** build, 10 rounds each — `part_move` was cut off when the disk filled (§3) | every pair in order, every table the same, `fail_counter` 0; the control went first 10 of 10. No round's T2 went unseen, which an applier killed by an assertion and restarted by heartbeat would have caused |
| **history map cleared by capacity** | `ha_writeset_history_size=50000`, 20,000-key transactions, all fourteen patterns, 10 rounds | Capacity clears happened only in the five patterns with no setup DDL between rounds — `ctl`, `fk_insert`, `fk_composite`, `sp_keep`, `sp_redo`, 4 rounds in 10 each; in the rest the DDL barrier before each round had already emptied the map. The control went first in exactly its 6 rounds without a clear: a clear raises the floor and the next transaction waits for everything before it. Every conflicting pair in order, every table the same, `fail_counter` 0. One `part_move` round's T2 was not seen within 30 s; its table matched and `fail_counter` did not move, and the run was late enough that the host's disk may already have been filling |
| **key limit** | the same pair, 60,000-key transactions | the control fell back to commit order in 5 rounds of 5 and never went first. The patterns after it did not run: the disk filled (§3) |
| **several clients** | 8 clients, 200–300 transactions each, 30 keys, primary-key, unique and key-change operations, about 30% of the transactions slow | 3 runs, none undisturbed: in one the applier was down for most of the workload — restarted to change its worker count — and caught up afterwards; the other two are the restart runs below. Every table the same, `fail_counter` 0, 874–1,277 write conflicts found each. An undisturbed run on four workers without foreign keys was not made |
| **restart** | as above, with `applylogdb` killed by `kill -9` three times a run — once while the clients were writing, twice nine seconds into the catch-up after heartbeat restarted it | 2 runs, 6 kills: every table the same, `fail_counter` 0 |
| **the gap before publish** | a dependent write chasing its predecessor's commit as closely as one csql session can, with 6 clients committing alongside to queue the log | 300 rounds as a foreign key, 300 as a unique key: every dependent written, every table the same, `fail_counter` 0. **No control**: how close the chase came is not known |

**Nothing here broke the parallel applier.** Two things were found on the way, and the parallel
applier introduced neither.

## 2. Why the last row was asked

The master makes a commit visible to other transactions (`logtb_complete_mvcc`) before it logs
the commit and before it publishes the commit's keys to the history (`log_commit_local`). A
transaction that saw the commit and decided its own label in between would find nothing to depend
on. A dependent write that has to take a lock the predecessor holds cannot get there — the locks
are released after the publish — and a two-session check, in which a child insert waited for a
session deleting its parent, suggests a foreign key check takes one
([concurrent-cascade-leaves-orphans](concurrent-cascade-leaves-orphans.md) §3). The chase asks
whatever does not wait, and found nothing; with no control, that is a weak no.

## 3. What was found instead

- **Several clients on a parent and a child with `ON DELETE CASCADE` diverge the pair — and did so
  before #8052.** The slave loses a child row the master keeps — after the applier fails deletes
  and updates of rows it does not have, or with nothing reported at all. On the develop #8052 was
  branched from the master itself also ends with children whose parent is gone. It is CBRD-27464
  — a cascade that decides on the children it read before waiting — seen across a pair. Whether
  four workers make it more frequent than one is not settled. That is
  [concurrent-cascade-leaves-orphans](concurrent-cascade-leaves-orphans.md), and it is why the
  several-clients row above leaves foreign keys out.
- **These pairs filled the host's root filesystem.** csb keeps a cluster's volumes under
  `~/.local/share/csb` unless `CSB_HOME` says otherwise, the ballast rows here are about 30 MB a
  round on each node, and four pairs reached 31 GB before the master refused a temporary volume.
  Set `CSB_HOME` to a large disk before running any of this.

## 4. Also measured on the way

- **Every replicated statement clears the whole history.** On the master `History_clear_count`
  equalled `Fallback_statement_count`, 940 and 940: each DDL is a full barrier on the slave and
  empties the map behind it. The marker every wait uses is one
  ([parallel-apply-and-the-marker](parallel-apply-and-the-marker.md)).
- **A history too small for the transactions serialises the applier.** At 50,000 against
  20,000-key transactions the control went first 6 times in 10 rather than 10, and past the key
  limit not at all.

## 5. Reproducing it

The scripts are beside this file; each takes the database, the master and the slave, and runs on
a csb client node with the tools directory mounted.

```bash
export CSB_HOME=/a/large/disk/csb
csb cluster create --cluster wsh4 --build $B --preset ha --clients 1 --tools $T \
    --set-hidden ha_apply_worker_count=4 --set-hidden ha_writeset_history_size=50000
csb node exec wsh4-c1 --cluster wsh4 -- sh /tools/parallel-apply-pairs.sh wsh4 wsh4-n1 wsh4-n2 10 20000 ctl pk_update ...

# several clients, without the foreign keys
csb node exec wsp4-c1 --cluster wsp4 -- sh /tools/parallel-apply-stress.sh wsp4 wsp4-n1 wsp4-n2 8 300 30 52 0,1,2,3,4 1000

# the chase
csb node exec wsp4-c1 --cluster wsp4 -- sh /tools/parallel-apply-chaser.sh wsp4 wsp4-n1 wsp4-n2 300 fk 6
```

The restart was driven from the host: while the stress script ran, `pkill -9 -f "cub_admin
applylogdb"` on the slave whenever the applier had been up eight seconds or more, three times.
