# The seven cases that fail under CTP as well

- **Date:** 2026-09-17
- **What this is:** why each of the cases that fail in every whole-corpus run fails — the ones
  `category/isolation/05-when-a-case-fails.md` lists as "cases that fail everywhere" and this project had only
  said were "the engine's or the corpus's, not the runner's". One is an engine crash. Five are races in the
  case, four of them the same race. One is a race the runner decides, and this runner wins it.
- **For:** upstream, except where it says otherwise.
- **Trees:** engine `cubrid/cubrid` `f1ae86ff7` · cases `cubrid/cubrid-testcases` `6ab786aa9` · CTP
  `cubrid/cubrid-testtools` `a1bec87`, in the `regr-iso` sandbox (`isolation-baseline.md` §1).
- **How:** `always-fail.sh` — each case alone, three times under ctltool's `qactl` and three under testkit's
  controller, alternating, each attempt on a database `prepare.sh` had just made — keeping every attempt's
  `runone.sh` output, its normalized result, and any crash report the server wrote to `$CUBRID/log/coredump`
  while it ran. `always-fail-2.sh` and `always-fail-3.sh` are the same with each attempt's files named after the
  case's directories as well, because three of the seven are called `insert_update_04`; `repro.sh` runs the
  reductions of §1.

| case | alone, ctltool | alone, testkit | what it is |
|---|---|---|---|
| `_01…/partition_table/range/dml_ddl/reorganization_select_01` | NOK,NOK,NOK | NOK,NOK,NOK | **an engine bug**: the server dies (§1) |
| `_01…/cbrd_21506/unique_index/insert_update_04` | NOK,NOK,NOK | NOK,NOK,NOK | **a race in the case**, kind 4 (§2) |
| `_06…/normal_index/insert_update_04` | NOK,NOK,NOK | NOK,NOK,NOK | the same, with `online parallel 15` |
| `_06…/unique_index/insert_update_04` | NOK,NOK,NOK | NOK,NOK,NOK | the same |
| `_06…/dml_online_index/insert_odku_online_index_01` | NOK,NOK,NOK | NOK,NOK,NOK | the same |
| `_05…/index_column/common_index/basic_sql/insert_delete_05` | NOK,NOK,NOK | NOK,NOK,NOK | **a race in the case**, kinds 1 and 3 (§3) |
| `_01…/function/counter_function/delete_delete_rownum_01` | NOK,NOK,NOK | **OK,OK,OK** | **a race the controller decides** (§4) |

The kinds are `isolation-corpus-races.md`'s. **None of the seven is the environment**: the
`Cannot remove user INFORMATION_SCHEMA` line and the missing `~/CUBRID/log` appear in every case's trace,
passing or failing (`05-when-a-case-fails.md`).

---

## 1. `reorganization_select_01` — the server dies in the aggregate hash table

Every one of the six attempts alone ended with the client saying

```
| ERROR RETURNED: Your transaction has been aborted by the system due to server failure or mode change.
Client C2 … is not connected any longer, the server seems to have died.
```

and the server leaving a crash report in `$CUBRID/log/coredump/cub_server_<timestamp>.coredump`. The stack is
the same in all of them, and in the six reports the two CTP runs of 2026-09-15 left behind (`full-1` at
03:03–03:04, `full-2` at 06:19–06:20):

```
qdata_save_agg_hentry_to_list   query_aggregate.cpp:2974
qdata_save_agg_htable_to_list   query_aggregate.cpp:3264
qexec_groupby                   query_executor.c:5640
```

`:3264` is the "dump accumulators to partial list" call, which is how an aggregate hash table that will not fit
is spilled to a list file; `:2974` is `list_id->tpl_descr.f_valp[col++] = key->values[i]` in the tuple
descriptor it builds.

**The second client is not needed, and neither is the partition reorganization.** Three single-client
reductions of the case (`repro/`), six attempts each:

| reduction | 100,000 rows into | then | crashed |
|---|---|---|---|
| `reorg_serial` | a range-partitioned table | `alter table t reorganize partition p1 …`, commit, the group by | 6 of 6 |
| `noreorg_serial` | a range-partitioned table | the group by | 6 of 6 |
| `nopart_serial` | a table with no partitions | the group by | **0 of 6** — the 20 rows come back |
| `small_serial` | the same partitioned table, 10,000 rows | the group by | **0 of 6** — the 20 rows come back |

So `select col,count(id) from t group by col order by 1,2` over a range-partitioned table of 100,000 rows is
enough, and 10,000 rows in the same table is not. The case's `MC: wait until C2 blocked;` and its concurrent
`reorganize partition` are not part of it. The row count and the spill call at `:3264` point the same way — the
crash is on the path taken when the aggregate hash table has to go to a list file — but what is wrong there is
**not determined** here.

**`runone.sh` cannot see this.** `checkCoreAndFatalError` (`runone.sh:185-193`) looks for files named `core.*`
under `$ctlpath`, `$CUBRID` and the case directory, and for `FATAL ERROR` in `$CUBRID/log/*`. On this machine
`/proc/sys/kernel/core_pattern` pipes cores to apport, so there is no `core.*`; the engine's own report is
`log/coredump/cub_server_*.coredump`, which matches neither pattern; and a SIGSEGV writes no `FATAL ERROR`
line. The case is reported as an ordinary diff, with no core and no fatal error — which is what
`isolation-baseline.md` §4 recorded of it.

**Not determined:** which engine change, if any, introduced it. `query_aggregate.cpp`'s last change is
`7749f4ff8` (2026-09-09, *CBRD-27178 Improve SUM/AVG performance with a resident sum accumulator*), which
edited the loop below the crashing line but not the line itself (`git blame`: `:2974` is from 2019). No older
engine was built to compare.

## 2. Four cases where two clients are released by one lock demotion

`_01_ReadCommitted/cbrd_21506/unique_index/insert_update_04.ctl:39-51` and, with
`create index … with online parallel 15` in place of the plain one, the two `_06_features` `insert_update_04`;
`dml_online_index/insert_odku_online_index_01.ctl:32-44` is the same shape with an `on duplicate key update`:

```
C3: insert into t1 values (10);
MC: wait until C3 blocked;
C4: update t1 set a = -10 where a = 1;
MC: wait until C4 blocked;
C1: commit;
MC: wait until C2 unblocked;
/* C2 starts scan and will demote to IX. C3 and C4 will resume */
MC: wait until C3 ready;
MC: wait until C4 ready;
```

C3 and C4 are both blocked behind C2's online index build, and C2's demotion to IX releases both. The two waits
order nothing that is printed: the two clients run at once and their lines come out in the order the controller
reads them. **Every one of the 24 attempts — six per case, under both controllers — is the answer with two
adjacent lines swapped**, C3's before C4's:

| case | the answer has | every attempt has |
|---|---|---|
| `cbrd_21506/unique_index/insert_update_04` | `2 rows affected` (C4), `1 row affected` (C3) | the two the other way round |
| `_06…/normal_index/insert_update_04` | the same | the same |
| `_06…/unique_index/insert_update_04` | the same | the same |
| `_06…/dml_online_index/insert_odku_online_index_01` | `1 row affected` (C4), `2 rows affected` (C3) | the two the other way round |

Nothing else differs — the results are byte-identical to the answer once the two lines are swapped. This is
kind 4 of `isolation-corpus-races.md`, and the same shape as `update_insert_01_1_complex` there, which is a
runner difference; here it is not, because on this machine C3 answers first under **both** controllers.

**Three of the four already have the other order on file, and it has gone stale**:

| case | second answer | how far behind |
|---|---|---|
| `cbrd_21506/unique_index/insert_update_04` | `.answer1`, C3 first | holds the pre-`CBRD-24396` tail (`Cannot find the index 't1.i(-)'`) that `.answer` was revised away from in 2022 (`179410981`) |
| `_06…/unique_index/insert_update_04` | `.answer1`, C3 first | the same |
| `_06…/normal_index/insert_update_04` | `.answer1`, C3 first | missing the `Statistics updated successfully: 1 table, 1 column.` line added to `.answer` in 2026-08 (`83dd0c4c5`, CBRD-26959), and its class name is `'t1'` where `.answer` has `'public.t1'` (user schema, `8c328969f`, 2022) |
| `_06…/dml_online_index/insert_odku_online_index_01` | none | — |

**The fix is the second answer**: `.answer` with the two lines swapped, and one written for
`insert_odku_online_index_01`. Whether the order C3-then-C4 is what this engine now always produces, or what
this machine produces, is **not determined** — it held for 24 of 24 attempts here, and upstream's revisions of
2022 and 2026 kept `.answer` in the other order.

## 3. `insert_delete_05` — a snapshot and a print order, neither ordered

`_05_ReadCommitted_RepeatableRead/index_column/common_index/basic_sql/insert_delete_05.ctl:36-39`:

```
C2: select * from t order by id;      -- C2 is REPEATABLE READ
C1: insert into t values(2,'abc');
MC: wait until C1 ready;
C1: commit work;
```

Nothing waits for C2 between its select and C1's insert, so two things are unordered at once, and the two
controllers lose different ones:

- under **ctltool's** controller, all three attempts differ from the answer by one line's position: C1's
  `1 row affected` prints after C2's two rows instead of before them (kind 1);
- under **testkit's**, all three attempts have C2 seeing three rows and deleting two, because C1's insert and
  its commit both landed before C2's statement took its snapshot (kind 3).

**The fix is a `MC: wait until C2 ready;` after line 36**, which orders both, with `.answer`'s
`1 row affected` moved after the select's rows to match. The case is not new to this: `isolation-baseline.md`
§4 records it passing in the prototype run that removed one of `qactl`'s two sleeps.

## 4. `delete_delete_rownum_01` — the one this runner wins

`_01_ReadCommitted/function/counter_function/delete_delete_rownum_01.ctl:50-55`:

```
C2: DELETE FROM t1 WHERE ROWNUM < 3;
/* expect: no transactions need to wait */
MC: wait until C1 ready;              -- C1 has nothing outstanding
/* expect: C1 2 rows are deleted */
C1: SELECT count(*) FROM t1;
```

The wait names C1, which is idle, so it returns at once and nothing orders C2's delete against C1's select —
kind 2, with the roles of `isolation-corpus-races.md`'s example reversed. The answer holds C1's `5` before
C2's `2 rows affected`, which is the order the select finishing first produces.

Alone, three times each: **ctltool's controller fails all three** — under it C2's delete finishes first every
time — and **testkit's passes all three**. The whole-corpus runs say the same: NOK in all four runs that used `qactl`
(`full-1`, `full-2`, `tk-full-1`, `tk-full-p4`) and OK in all four that used this controller.

Under ADR-018 rule 3 this is a runner difference in the direction the gate does not look for: the case has been
failing under CTP for as long as the sandbox has records, and it passes here. **The fix is the wait naming
C2**, which is also what the case's own comment asks for.
