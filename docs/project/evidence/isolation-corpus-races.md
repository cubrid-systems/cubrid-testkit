# Cases that pass because the controller is slow

- **Date:** 2026-09-16
- **What this is:** cases in `cubrid/cubrid-testcases` `isolation/` whose answer records the order that
  ctltool's `qactl` happens to produce, rather than a property of the database. They are found by running the
  corpus with a controller that does not have qactl's two fixed 100 ms sleeps, its 10 ms polls or its serial
  client start (ADR-019): each one then fails, alone, every time.
- **For:** upstream. Every fix below is in the case, not in a runner.
- **Trees:** engine `cubrid/cubrid` `f1ae86ff7` · cases `cubrid/cubrid-testcases` `6ab786aa9` · CTP
  `cubrid/cubrid-testtools` `a1bec87`.

---

## How they were found

The whole corpus, four slots, on one machine: 6,772 cases, 40 NOK. Six of those fail under CTP too and six are
already known to flip run to run (`isolation-baseline.md` §4). The rest had never failed in any of four earlier
whole-corpus runs — two under CTP, two under the native runner with CTP's own controller.

ADR-018 rule 3 is what a disagreement gets: the case **alone**, on a database `prepare.sh` has just made, three
times under each controller. A runner difference is one controller passing all three and the other failing all
three.

**All 28, three times each under each controller, on a fresh database every time:**

| | cases |
|---|---:|
| ctltool's `qactl` passes three times, testkit's controller fails three times — **a runner difference** | **25** |
| both pass alone (`createindex_02`, `createindex_03`) | 2 |
| testkit unstable alone — NOK, OK, OK (`update_insert_03`) | 1 |

ctltool's controller passed all three attempts in **every** one of the 28. The 25 are what this document is
about.

| case | what differs | lines | alone, testkit | rule 3 |
|---|---|---:|---|---|
| `_01_ReadCommitted/catalog/db_index_key_04` | a wait that never came true | 159 | NOK,NOK,NOK | runner difference |
| `_01_ReadCommitted/index_column/common_index/basic_sql/delete_insert_10` | a different number of rows | 11 | NOK,NOK,NOK | runner difference |
| `_01_ReadCommitted/index_column/common_index/basic_sql/insert_insert_20` | a different number of rows | 2 | NOK,NOK,NOK | runner difference |
| `_01_ReadCommitted/index_column/function_index/insert_select_07` | a different number of rows | 3 | NOK,NOK,NOK | runner difference |
| `_01_ReadCommitted/primary_key_column/aggregate/insert_select_05` | a different number of rows | 4 | NOK,NOK,NOK | runner difference |
| `_01_ReadCommitted/primary_key_column/aggregate/insert_select_05_1` | a different number of rows | 4 | NOK,NOK,NOK | runner difference |
| `_01_ReadCommitted/primary_key_column/aggregate/insert_select_05_3` | a different number of rows | 4 | NOK,NOK,NOK | runner difference |
| `_01_ReadCommitted/primary_key_column/aggregate/insert_select_05_5` | a different number of rows | 4 | NOK,NOK,NOK | runner difference |
| `_01_ReadCommitted/primary_key_column/aggregate/insert_select_06_5` | different rows | 2 | NOK,NOK,NOK | runner difference |
| `_01_ReadCommitted/primary_key_column/basic_sql/update_select_04` | different rows | 2 | NOK,NOK,NOK | runner difference |
| `_02_RepeatableRead/index_column/common_index/aggregate/delete_select_01_5` | different rows | 40 | NOK,NOK,NOK | runner difference |
| `_02_RepeatableRead/index_column/common_index/aggregate/delete_select_02` | different rows | 40 | NOK,NOK,NOK | runner difference |
| `_02_RepeatableRead/index_column/common_index/aggregate/select_select_01` | the same lines, in another order | 2 | NOK,NOK,NOK | runner difference |
| `_02_RepeatableRead/trigger/basic_sql/trigger_update_11` | the same lines, in another order | 2 | NOK,NOK,NOK | runner difference |
| `_04_RepeatableRead_ReadCommitted/index_column/common_index/basic_sql/delete_insert_10` | a different number of rows | 8 | NOK,NOK,NOK | runner difference |
| `_04_RepeatableRead_ReadCommitted/index_column/common_index/basic_sql/insert_insert_20` | a different number of rows | 2 | NOK,NOK,NOK | runner difference |
| `_04_RepeatableRead_ReadCommitted/index_column/common_index/groupby/delete_select_06` | the same lines, in another order | 2 | NOK,NOK,NOK | runner difference |
| `_04_RepeatableRead_ReadCommitted/index_column/composite_index/basic_sql/update_delete_09_3` | a different number of rows | 10 | NOK,NOK,NOK | runner difference |
| `_04_RepeatableRead_ReadCommitted/no_index_column/basic_sql/update_select_04` | different rows | 12 | NOK,NOK,NOK | runner difference |
| `_04_RepeatableRead_ReadCommitted/partition_table/range/without_index/update_delete_07` | different rows | 2 | NOK,NOK,NOK | runner difference |
| `_04_RepeatableRead_ReadCommitted/primary_key_column/basic_sql/update_select_13` | different rows | 2 | NOK,NOK,NOK | runner difference |
| `_04_RepeatableRead_ReadCommitted/primary_key_column/multiple_pk/select_delete_01` | a different number of rows | 4 | NOK,NOK,NOK | runner difference |
| `_05_ReadCommitted_RepeatableRead/dml_ddl/createindex_02` | the same lines, in another order | 4 | OK,OK,OK | both pass alone |
| `_05_ReadCommitted_RepeatableRead/index_column/multi_index/basic_sql/update_insert_03` | the same lines, in another order | 4 | NOK,OK,OK | unstable alone |
| `_05_ReadCommitted_RepeatableRead/partition_table/range/with_index/primary_key/delete_delete_01` | different rows | 2 | NOK,NOK,NOK | runner difference |
| `_06_features/cbrd_22705_online_index_parallel/_04_RepeatableRead_ReadCommitted/index_column/common_index/groupby/delete_select_06` | the same lines, in another order | 2 | NOK,NOK,NOK | runner difference |
| `_06_features/cbrd_22705_online_index_parallel/_04_RepeatableRead_ReadCommitted/index_column/composite_index/basic_sql/update_delete_09_3` | a different number of rows | 10 | NOK,NOK,NOK | runner difference |
| `_06_features/cbrd_22705_online_index_parallel/dml_ddl/createindex_03` | the same lines, in another order | 2 | OK,OK,OK | both pass alone |

The 28 paths are 22 cases: six pairs are the same case at another isolation level, or with
`CREATE INDEX … with online parallel 3` in place of a plain one. The place to fix is the same in each pair.

## What the cases have in common

Nothing about the database. Each of them leaves two clients' work unordered and then writes down the order that
one particular controller produced.

### 1. Two statements, no wait between them

`_02_RepeatableRead/index_column/common_index/aggregate/select_select_01.ctl:33-38`

```
C1: DELETE FROM tb1 WHERE id BETWEEN 10 AND 20;      -- 11 rows
C4: DELETE FROM tb1 WHERE id BETWEEN 100 AND 150;    -- 51 rows
C1: commit;
C4: commit;
MC: wait until C1 ready;
MC: wait until C4 ready;
```

Both deletes are in flight at once and whichever finishes first prints first. The answer holds
`11 rows affected` then `51 rows affected`; with a controller that hands C4 its statement a hundred
milliseconds sooner, C4's 51 rows land first and the case fails on two lines that are each correct.

**The fix is a wait**: `MC: wait until C1 ready;` between the two deletes, if the case means them to be ordered;
or a second answer file, if it does not care.

### 2. A `wait until … blocked` with nothing ordering what it waits for

`_01_ReadCommitted/catalog/db_index_key_04.ctl`

```
C2: alter table tb2 drop index i_tb2_id_col;
MC: wait until C3 ready;          -- C3 is idle here: this returns at once
C3: drop index i_tb2_id_col on tb2;
MC: wait until C3 blocked;
```

The wait names C3, which has nothing outstanding, so it returns immediately and nothing guarantees that C2's
`alter` has taken its lock. Under qactl the gap between the two statements is long enough that it always has;
without it C3 wins, drops the index itself, and is never blocked — the run then reports
`ERROR! Client 3 is ready.` and dumps the lock table.

**The fix is one character**: the wait should name **C2**.

### 3. A snapshot nobody waits for

`_01_ReadCommitted/primary_key_column/aggregate/insert_select_05.ctl` and its four siblings
(`_05_1`, `_05_3`, `_05_5`, `_06_5`)

C6 is sent a `select` that sleeps five seconds so that it holds a snapshot, and the statements that must not be
visible to it are sent to C2 and C3 with nothing waiting for C6 to have taken it. The answer has 9 rows; when
the other clients commit sooner, the select sees 11 — `'aa'` and `'cc'` are the two extra.

This one is not new: `full-1`, `full-2` and `tk-full-p4` each lost the same race on a first attempt, with the
same two rows, and won it on the retry (`isolation-baseline.md` §4). A faster controller loses it every time.

**The fix is a wait** for the client that takes the snapshot, before the others commit.

## What this runner does about it meanwhile

`TESTKIT_ISOLATION_CTL` stays off. Without it a run is ADR-007's — ctltool's `qactl`, unchanged — so nothing in
CI moves while these cases say what they say. The controller is not wrong about them: it sends the second
statement when the script says to send it, which is immediately.

## Why this is worth fixing upstream rather than working around

A case that passes only while the controller is slow is not testing what it says it tests. `select_select_01`
is written to check what two concurrent deletes do and instead records which of them a hundred milliseconds of
`sleepms` let finish first; `db_index_key_04` is written to check a blocked DDL and instead checks that C2 got
a head start. They pass today, and they will keep passing under CTP — until the machine is faster, or the
engine is, or the client is.
