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

Six were carried on 2026-09-19, and **the arithmetic here is not `28 - 6`.** The gate is failed by 27 paths, not 28:
the 25 runner differences in the table above plus the two found at eight and fourteen slots. Five of the six patches
are on one of those 27; the sixth, `db_index_04`, is the same kind but passes when run alone. So 27 - 5 = 22 were
unwritten. **Ten more were written on 2026-09-28** (below), so it is now 27 - 15 = **12**.

Each of the first six is an ordering statement, or an answer the corpus already has and never finished, and
**none of them changes what the case prints** — which is why they could be written from the case's own text:

| case | the change |
|---|---|
| `_01_ReadCommitted/catalog/db_index_key_04` | `:55` the wait names **C2** |
| `_01_ReadCommitted/catalog/db_index_04` | `:37` the wait names **C2** |
| `_02_RepeatableRead/index_column/common_index/aggregate/select_select_01` | `:33` `MC: wait until C1 ready;` between the two deletes |
| `_04_RepeatableRead_ReadCommitted/index_column/common_index/groupby/delete_select_06` | `:32` `MC: sleep 1;` becomes `MC: wait until C2 ready;` |
| `_06_features/cbrd_22705_online_index_parallel/…/groupby/delete_select_06` | the same line, the same fix |
| `_04_RepeatableRead_ReadCommitted/…/unique_with_key/update_insert_01_1_complex` | `.answer1` gets `public.` in front of its two class names |

**Twenty-two were not written.** This section used to say the reason was the same for twenty-one of them —
kind 3's fix is for the client to take its snapshot in a statement of its own, a statement of its own prints, so the
answer changes and has to be re-recorded from a run; and that `trigger_update_11` was the exception, its two moved
lines belonging to clients blocked on each other.

**Measured on 2026-09-28, that is three groups and not two**, and the split is decided by the controller rather than
by the cases: five want only a wait and no new answer, five are the kind 3 described above, and twelve cannot be
expressed at all with the wait states `qactl` has. The next section has the measurement, the case list and why, and
the ten that could be written now are.


### The 22 that were not written, by name — and what each one's patch needed

**Run 2026-09-28**, engine `11.5.0.2513-5f3a30d`, one slot, `testcase_retry_num=0`, three attempts, testkit's
controller throughout. The case list was generated from the tables above minus the patch directory as it stood before
this run. It is text, and does not move when either of them does; the last column says what became of each.

They are not one kind, and the controller is what splits them. `qactl`'s own `WAIT_USAGE_FORMAT` gives four states:

```
command := wait until c<client ID> { blocked | unblocked | ready | finished };
```

Every one of them is about a client being **idle, lock-blocked, or done**. **There is no state meaning "has begun
executing"**, and `rendezvous with` does not fill the gap because the client has to issue it, which a client inside a
long statement cannot. That decides where "take the snapshot in a statement of its own" is expressible:

- **REPEATABLE READ** — it is. The snapshot is the transaction's, so an earlier statement takes it and
  `MC: wait until Cn ready;` pins it. The extra statement prints, so the answer has to be re-recorded from a run.
- **READ COMMITTED** — it is not. The snapshot is the *statement's*, re-taken each time, and what has to be ordered is
  the moment a statement starts. Nothing in the vocabulary says that. Re-recording the answer does not help: it would
  only move which way the race has to fall.
- **No sleeping statement at all** — these looked like the first kind, two statements with nothing between them, and
  this list said a wait would order them with **the answer unchanged**. Writing them showed that holds for three of the
  five, not all of them (see *What writing the ten found*).

So of the 22: **10 could be written as a case change** and now are, and **12 are not a corpus fix at all** — they need
a way to say "this statement has started", which is a change to `qactl`, not to a case.

| case | the snapshot | three attempts, unpatched | patch |
|---|---|---|---|
| **a missing wait** | | | |
| `_01_ReadCommitted/primary_key_column/basic_sql/update_select_04` | — | NOK NOK NOK | written |
| `_02_RepeatableRead/trigger/basic_sql/trigger_update_11` | — | NOK NOK NOK | written |
| `_04_RepeatableRead_ReadCommitted/no_index_column/basic_sql/update_select_04` | — | NOK NOK NOK | written |
| `_04_RepeatableRead_ReadCommitted/partition_table/range/without_index/update_delete_07` | — | NOK NOK NOK | written |
| `_05_ReadCommitted_RepeatableRead/partition_table/range/with_index/primary_key/delete_delete_01` | — | NOK NOK NOK | written |
| **REPEATABLE READ snapshot** | | | |
| `_02_RepeatableRead/index_column/common_index/aggregate/delete_select_01_5` | C4,C5,C6 | NOK NOK NOK | written |
| `_02_RepeatableRead/index_column/common_index/aggregate/delete_select_02` | C4,C5,C6 | NOK NOK NOK | written |
| `_04_RepeatableRead_ReadCommitted/index_column/common_index/basic_sql/delete_insert_10` | C1 | NOK NOK NOK | written |
| `_04_RepeatableRead_ReadCommitted/index_column/common_index/basic_sql/insert_insert_20` | C1 | NOK NOK NOK | written |
| `_06_features/cbrd_22705_online_index_parallel/_04_RepeatableRead_ReadCommitted/index_column/common_index/basic_sql/insert_insert_20` | C1 | NOK NOK NOK | written |
| **READ COMMITTED snapshot** | | | |
| `_01_ReadCommitted/index_column/common_index/basic_sql/delete_insert_10` | C1 | NOK NOK NOK | **not expressible** |
| `_01_ReadCommitted/index_column/common_index/basic_sql/insert_insert_20` | C1 | NOK OK NOK | **not expressible** |
| `_01_ReadCommitted/index_column/function_index/insert_select_07` | C1 | NOK NOK NOK | **not expressible** |
| `_01_ReadCommitted/primary_key_column/aggregate/insert_select_05` | C4,C5,C6 | NOK NOK NOK | **not expressible** |
| `_01_ReadCommitted/primary_key_column/aggregate/insert_select_05_1` | C4,C5,C6 | NOK NOK NOK | **not expressible** |
| `_01_ReadCommitted/primary_key_column/aggregate/insert_select_05_3` | C4,C5,C6 | NOK NOK NOK | **not expressible** |
| `_01_ReadCommitted/primary_key_column/aggregate/insert_select_05_5` | C5,C6 | NOK NOK NOK | **not expressible** |
| `_01_ReadCommitted/primary_key_column/aggregate/insert_select_06_5` | C4,C5,C6 | OK NOK NOK | **not expressible** |
| `_04_RepeatableRead_ReadCommitted/index_column/composite_index/basic_sql/update_delete_09_3` | C2 | NOK NOK NOK | **not expressible** |
| `_04_RepeatableRead_ReadCommitted/primary_key_column/basic_sql/update_select_13` | C2 | NOK NOK NOK | **not expressible** |
| `_04_RepeatableRead_ReadCommitted/primary_key_column/multiple_pk/select_delete_01` | C2 | NOK NOK NOK | **not expressible** |
| `_06_features/cbrd_22705_online_index_parallel/_04_RepeatableRead_ReadCommitted/index_column/composite_index/basic_sql/update_delete_09_3` | C2 | NOK NOK NOK | **not expressible** |

**Two of the 22 did not fail three times out of three**, so under ADR-018 rule 3 they are not runner differences on
this measurement:

| case | attempts |
|---|---|
| `_01_ReadCommitted/index_column/common_index/basic_sql/insert_insert_20` | NOK **OK** NOK |
| `_01_ReadCommitted/primary_key_column/aggregate/insert_select_06_5` | **OK** NOK NOK |

This is **not** presented as a correction to the table above. That table was measured on a different engine, and this
run could not use the same one: `~/.bash_profile` fixes `$CUBRID` for every script CTP sends, so the build under test
here is `11.5.0.2513-5f3a30d` and not the isolation tree's `11.5.0.2574-f1ae86f`
([configuration](../../category/isolation/04-configuration.md#two-ways-the-environment-is-wrong-without-saying-so)).
Two cases moving between builds is exactly what ADR-018 rule 3 exists to catch, and settling which it is means running
both builds, which has not been done.

### What writing the ten found

Each patch was validated five times under testkit's controller and five under `qactl`, against a fresh copy of the
corpus with the patch applied through `case_patch_dir` — the mechanism a run uses, not a hand-edited tree. The copy
came back byte-identical after every run, including for the one patch that creates a file. The patches, and a row for
each, are in `cubrid-testkit-patches/isolation`.

- **The list above pointed at the right kind, and once at the wrong pair.** It finds a missing wait by looking for two
  adjacent printing statements from two clients. In `_01…/update_select_04` those were C1's and C2's selects, but the
  failure was C2's select against C1's *commit* — the wait between them names C1 only. What showed it was the diff:
  `6` became `7`, a value and not an order.
- **Three were the `db_index_key_04` mistake again** — a wait naming a client that has been ready since the line above
  it, where the client with something outstanding is the other: `update_delete_07` `:48`, `delete_delete_01` `:31`,
  and both commit blocks of the `_04` `update_select_04`. In each the case's own comment says what was meant
  (*"expect (1,'abc'),(12,'abc')"*, *"expect 5000"*).
- **"The answer does not change" held for three of the five.** `delete_delete_01`'s answer moves one line: once C2 is
  waited for, its `5000 rows affected` is collected where it runs. And `trigger_update_11` is not a missing wait at
  all. It is a deadlock whose victim is the same under both controllers (`on statement number: 4` in both), with the
  victim's error and the survivor's `1 row affected` released together — the fourth kind, answered the way
  `update_insert_01_1_complex` is, with a second answer.
- **Two places held two races each, and the first patch fixed one.** `_04…/update_select_04` waited for C2 before the
  commit and not for the commit before C2 selected again; it failed one attempt in six under testkit's controller.
  `_01…/update_select_04` waited for C2 before the commit and left the two selects unordered; it passed seven of eight
  and then failed under `qactl` with every row right and the two result blocks swapped. Both now order everything the
  case's text implies, and passed ten of ten and twenty of twenty.
- **For REPEATABLE READ, not one recorded line moved.** Each of the five gained exactly its new statement's result —
  `4` for `t`, `9997` for `tb1` — and every other line of the corpus's answer came out as recorded, three runs of
  three. That is the difference between restoring an answer and inventing one, and it is why these five could be
  re-recorded at all.

`TESTKIT_ISOLATION_CTL` therefore stays off. This used to say *until the remaining answers are re-recorded*, and
for twelve of them that is not a thing that can happen: re-recording only moves which way the race has to fall. It
stays off until those twelve have a way to be ordered — a wait state for "has started", in `qactl` and in this
controller alike — and ADR-019's gate has been run on the patched corpus. The controller was never wrong about these
cases: it sends the second statement when the script says to send it, which is immediately.

## Why this is worth fixing upstream rather than working around

A case that passes only while the controller is slow is not testing what it says it tests. `select_select_01`
is written to check what two concurrent deletes do and instead records which of them a hundred milliseconds of
`sleepms` let finish first; `db_index_key_04` is written to check a blocked DDL and instead checks that C2 got
a head start. They pass today, and they will keep passing under CTP — until the machine is faster, or the
engine is, or the client is.
