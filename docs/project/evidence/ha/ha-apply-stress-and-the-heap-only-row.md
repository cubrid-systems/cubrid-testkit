# `testkit ha-apply stress`: the prototype's SQL, the heap-only row, and CBRD-27464 in `fail_counter`

- **Date:** 2026-10-30
- **What this is:** the completion condition of ADR-EXT-012's M2. A seed names the shell
  prototype's SQL byte for byte; `--compare-only` calls the damaged pair of
  [fk-index-insert-loses-the-victims-error](fk-index-insert-loses-the-victims-error.md) **damaged**;
  a run without foreign-key operations on #8052's new head is **same**.
- **Command:** `testkit ha-apply stress`, built from the tree of the commit that adds this file;
  after the build only documents changed, and the last line on standard error, which now names
  the files directory only when one was made. Eight clients, 200 transactions each, 20 keys, 3,000
  ballast rows, unless said otherwise.
- **Pairs:** `ws98b` and `wsn4`, below.

---

## 1. One seed, the prototype's SQL

`TestASeedNamesTheShellPrototypesSQL` compares the Go generator with three files the awk program of
[`parallel-apply-stress.sh`](parallel-apply-stress.sh) wrote under mawk: seed 45 clients 1 and 3
with all ten operations, and seed 83 client 2 with operations 5–8 and 12 keys. They are equal byte
for byte, ballast inserts and rollbacks included; busybox awk writes the same three files. Two
separate reviews ran both generators over seeds 0 and 999,999, 1 and 21 clients, up to 9,999
transactions, 1 key to 2³¹−1 and single and repeated operations: the Go generator matched mawk
and nawk in every case, and busybox awk wherever the ballast id stays within an INT. With 21
clients it passes 2³¹−1 from 4,749 transactions (sooner with more ballast rows), busybox prints
`-2147483648`, and `--clients`, `--txns` and `--ballast` together refuse such a run.

## 2. The damaged pair: damaged

`ws98b` is the pair whose master kept two children in its heap and not in its primary key after a
stress run on #8198 `6ce1c05d2`. The install tree it mounts is that code without the fix (it has no
`locator_wait_out_child_writer`), built before the damaging run, and the same server read the
tables below. It reports `11.5.0.2839-30066c8` because the source tree's HEAD was then
`30066c8b8`, the fix's commit (`ef96fd9d0` is the same tree with a rewritten message): the ledger's
`build=` is what `cubrid_rel` says, and here that is not the code.

```
# cluster=ws98b build=11.5.0.2839-30066c8 ha_apply_worker_count=unset ha_writeset_history_size=unset mode=compare-only
seed	master_errors	conflict_write	conflict_ref	fallback	clear	fail_counter_delta	orphan_polls	orphan_peak	clients	s_kv	s_parent	s_child	heap_s_kv	heap_s_parent	heap_s_child	s_ballast	orphans_master	orphans_slave	slave	verdict
-	-	-	-	-	-	-	-	-	-	same	same	same	0/0/0/0	14/14/14/14	9/7/7/7	same	2	0	caught	damaged
```

`s_child` reads the same on both nodes through its primary key, 7 rows each, which is what the
prototype compared and why it called this pair the same. Through the heap the master has 9. The
master also has two orphans and the slave none; that they are the two rows the index lacks is a
reading from the counts, not a read of the rows.

## 3. #8052's new head

`wsn4`, #8052 `61ee0944a` with four workers.

| operations | seed | verdict | master errors | write / ref conflicts | `fail_counter` | master orphans seen while running | final tables |
|---|---:|---|---:|---|---:|---|---|
| 0–4, 9 | 1 | **same** | 461 | 767 / 0 | 0 | 0 polls | same |
| all | 1 | diverged | 702 | 798 / 108 | +4 | 0 polls | same |
| all | 2 | diverged | 765 | 762 / 114 | +8 | 1,643 polls, at most 2 | `s_child`: 10 rows on the master, 9 on the slave |
| all | 3 | diverged | 687 | 751 / 99 | +5 | 136 polls, at most 1 | same |

Without the foreign-key operations the run is the same on both nodes. With them all three runs
diverged. In seeds 1 and 3 only `fail_counter` says so: every table, heap count and the ballast
agree at the end, and `fail_counter` counted four and five rows the slave could not apply. In seed
2 the slave also ended without a row the master has, the child `(7, 6)`, and both nodes' reads of
`s_child` are kept beside the ledger; with no orphan on either node at the end, the master holds
that child under a parent the slave also has.

Two earlier builds of the command, before the last two reviews (which changed what is compared on
the slave, what is kept and how a stalled slave is told, not the workload), ran the same five
commands. The FK-free run was the same both times. Of their six runs with all operations, three
diverged on `fail_counter` alone (+5, +1, +2) and three were the same.

So over nine runs with the foreign-key operations, six diverged and three were the same, and no
seed gave one verdict: seed 1 gave same, diverged, diverged; seed 2 diverged, same,
diverged; seed 3 same, diverged, diverged. The seed names the SQL, not how the clients interleave
(ADR-EXT-012 3-2). Five of the six diverged runs saw an orphan child on the master while the
clients ran, and none of the three that stayed the same did; the polls are samples, so an orphan
that came and went between two of them is not seen, and seed 1 above may be that.

**A reading, not a measurement:** this looks like CBRD-27464, which this tree does not fix (#8026
does). There, a concurrent `ON DELETE CASCADE` leaves the master a child whose parent is gone and
the slave without it, which would give a missing row of seed 2's shape; a later statement on such a
child would fail on the slave and count in `fail_counter`, and if the master's orphan is later
cleaned up or re-pointed the final states could agree. That is the path
[concurrent-cascade-leaves-orphans](concurrent-cascade-leaves-orphans.md) describes for the -711
apply failures. The failing rows themselves were not read, and the scripts touch child 7 and
parent 6 from several clients, so which statements met is not known.

## 4. What else showed

- **Refused statements** were 461 to 765 a run on 20 keys: mostly duplicate keys, 147 to 176
  missing parents where the foreign-key operations ran, and 0 to 2 deadlocks. They are the
  workload's, and not findings.
- **One history clear a run.** About 30 % of 1,600 transactions insert 3,000 ballast keys each,
  1.3 to 1.5 million keys against the default capacity of 1,000,000.
- **Time.** The clients took about a minute a run, and a whole run about two.
- **Kept files.** Each run's directory beside its ledger holds the eight scripts and logs, the
  clients' pids and csql's own `csql.err`; seed 2's also holds `s_child.master` and
  `s_child.slave`. The comparison of `ws98b` found no table that differed through the primary key
  (its finding is in the heap counts, which the ledger carries), wrote nothing and made no
  directory.

## 5. Reproducing it

```bash
export CSB_HOME=/data/...  TESTKIT_CSB=.../extensions/cluster-sandbox/bin/csb
testkit ha-apply stress --cluster <a pair with tables a stress run left> --compare-only
testkit ha-apply stress --cluster <a pair of #8052 61ee0944a with a client node c1> --ops 0,1,2,3,4,9 --seed 1
testkit ha-apply stress --cluster <the same pair> --seed 3
```

Each writes its ledger and one line on standard output. A run also keeps its clients' files
beside the ledger; `--compare-only` writes files there only for a table that differs.
