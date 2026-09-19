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

### 4. Two clients released by one commit

`_04_RepeatableRead_ReadCommitted/partition_table/range/with_index/unique_with_key/update_insert_01_1_complex.ctl:45-53`

```
C2: insert into t values(11,'abc');   -- blocked on C1's update to 11
MC: wait until C2 blocked;
C3: insert into t values(1,'abc');    -- blocked on C1's insert of 1
MC: wait until C3 blocked;
C1: commit;
MC: wait until C1 ready;
MC: wait until C2 ready;
```

Both clients are released by the same commit and both fail on a unique key. The waits are there, and they
order nothing that is printed: the two errors come out in the order the controller reads them from two clients
that are running at the same time. The answer holds C3's (`key: {1, 'abc'}`, partition `p1`) before C2's
(`{11, 'abc'}`, `p2`); testkit's controller printed C2's first, alone, three times out of three.

The case already allows both orders — `update_insert_01_1_complex.answer1` holds C2's error first — but that
file was not updated when the class names gained their owner (`t__p__p2` against `public.t__p__p2`), so it
matches nothing. With `public.` put in front of the two class names it is byte-identical to all three of
testkit's results.

**The fix is the second answer**, with `public.` in the two class names.

## Found at eight and fourteen slots

The four-slot run above is where the 28 came from. Three more whole-corpus runs with testkit's controller —
eight slots, fourteen, and eight with volatile overlays (`isolation-controller.md` §8) — failed five cases that
are not among its 40. Each went through rule 3 the same way, with every attempt's result kept
(`rule3-later.sh`):

| case | failed at | alone, ctltool | alone, testkit | rule 3 | kind |
|---|---|---|---|---|---|
| `_06_features/cbrd_22705_online_index_parallel/_04_RepeatableRead_ReadCommitted/index_column/common_index/basic_sql/insert_insert_20` | 8, 14, 8 volatile | OK,OK,OK | NOK,NOK,NOK | **runner difference** | 3 |
| `_04_RepeatableRead_ReadCommitted/partition_table/range/with_index/unique_with_key/update_insert_01_1_complex` | 8 volatile | OK,OK,OK | NOK,NOK,NOK | **runner difference** | 4 |
| `_02_RepeatableRead/index_column/common_index/aggregate/delete_select_02_5` | 14, 8 volatile | OK,OK,OK | OK,NOK,NOK | unstable alone | 3 |
| `_01_ReadCommitted/index_column/filter_index/basic_sql/insert_select_16` | 8 volatile | OK,OK,OK | OK,OK,OK | both pass alone | 3 |
| `_01_ReadCommitted/catalog/db_index_04` | 8 volatile | OK,OK,OK | OK,OK,OK | both pass alone | 2 |

Two more runner differences, and one of them is not a new case: the `_06_features` `insert_insert_20` is
`_04_RepeatableRead_ReadCommitted/index_column/common_index/basic_sql/insert_insert_20` with
`with online parallel 7` on its `create unique index` (line 31), and it fails the same way. The other three
are the same kinds, lost less often.

**`insert_insert_20`** (`:40-43`). C1's `insert … select … where … (select sleep(1)) = 0` is the statement
whose snapshot matters, and C2's `insert into t values(20,'b')` and its `commit` are sent with nothing waiting
for C1 to have taken it. Under ctltool's controller C1 copies 4 rows; under testkit's, C2 has committed first
and C1 copies 5. C1 is REPEATABLE READ, so **the fix is for C1 to take its snapshot in a statement of its own**
— a `select` and a `MC: wait until C1 ready;` before line 40.

**`delete_select_02_5`** (`:70-76`). C6's `SELECT col,AVG(id),sleep(3) …` and then, with no wait, C3's
`DELETE … BETWEEN 10100 AND 10150` and its `commit`. In the two failing attempts both of C6's selects average
higher — `1.453440e+04` where the answer has `1.450500e+04` for `'0'`, which is the average of the answer's 900
ids for `'0'` without the six that C3 deletes (`10100`, `10110` … `10150`). C3 had committed before C6 took its
snapshot. Every client is REPEATABLE READ; **the fix is the same**: C6 takes its snapshot before line 72.

**`insert_select_16`** (`:63-68`). C6, READ COMMITTED, selects with `(select sleep(1)=0)<>0`; C3 inserts
`(8,'cc')` at line 64 and commits at line 68, while C6 may still be sleeping. In the eight-slot volatile run
C6's result had `8 'cc'` — five rows for the answer's four. **The fix is a wait for C6 before line
64**, and the answer's lines reordered to match, since C3's `1 row affected` then prints after C6's rows.

**`db_index_04`** (`:36-39`) is `db_index_key_04`'s kind exactly: `MC: wait until C1 ready;` between C2's
`alter table tb2 drop constraint fk_tb2_id_col` and C3's `alter table tb1 drop constraint pk_tb1_id_col` names
a client with nothing outstanding. In all five attempts of the volatile run C3 went first, failed on the
foreign key C2 had not yet dropped, and `MC: wait until C2 ready;` waited out the attempt. **The fix is the
wait at line 37 naming C2.** (`full-1` failed it too, differently: a catalog listing in another order.)

## What this runner does about it

**Revised 2026-09-19.** This section said the switch stays off *until upstream fixes the cases*, which made a gate
here wait on someone else's review queue. It does not any more: the fixes are carried in this repository as
patches, the way shell's and sql's corpus problems already were, and ADR-018 consequences 6 and 7 say so. What
`cubrid-testkit-patches/isolation` holds is written to be sent upstream as it stands, and the pull request that lands
one deletes its patch — the run then refuses the case, which is how this repository finds out (ADR-013's rule,
applied here).

Six are carried today. Each is an ordering statement, or an answer the corpus already has and never finished, and
**none of them changes what the case prints** — which is why they could be written from the case's own text:

| case | the change |
|---|---|
| `_01_ReadCommitted/catalog/db_index_key_04` | `:55` the wait names **C2** |
| `_01_ReadCommitted/catalog/db_index_04` | `:37` the wait names **C2** |
| `_02_RepeatableRead/index_column/common_index/aggregate/select_select_01` | `:33` `MC: wait until C1 ready;` between the two deletes |
| `_04_RepeatableRead_ReadCommitted/index_column/common_index/groupby/delete_select_06` | `:32` `MC: sleep 1;` becomes `MC: wait until C2 ready;` |
| `_06_features/cbrd_22705_online_index_parallel/…/groupby/delete_select_06` | the same line, the same fix |
| `_04_RepeatableRead_ReadCommitted/…/unique_with_key/update_insert_01_1_complex` | `.answer1` gets `public.` in front of its two class names |

**The other twenty-two are not written, and the reason is the same for twenty-one of them.** Kind 3's fix is for
the client to take its snapshot in a statement of its own, and a statement of its own prints — so the answer
changes and has to be re-recorded from a run. A `.ctl` patched without its answer fails the case for a new reason,
which is worse than leaving it. The twenty-second is `trigger_update_11`, whose two moved lines belong to clients
that are blocked on each other: which the engine releases first is not settled by the case's text.

`TESTKIT_ISOLATION_CTL` therefore stays off, but for a different reason than before — not until upstream moves,
until the remaining answers are re-recorded and ADR-019's gate has been run on the patched corpus. The controller
was never wrong about these cases: it sends the second statement when the script says to send it, which is
immediately.

## Why this is worth fixing upstream rather than working around

A case that passes only while the controller is slow is not testing what it says it tests. `select_select_01`
is written to check what two concurrent deletes do and instead records which of them a hundred milliseconds of
`sleepms` let finish first; `db_index_key_04` is written to check a blocked DDL and instead checks that C2 got
a head start. They pass today, and they will keep passing under CTP — until the machine is faster, or the
engine is, or the client is.
