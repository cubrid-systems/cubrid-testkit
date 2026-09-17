# 5. When a case fails

[← back to the isolation category](README.md)

- [Where to look](#where-to-look)
- [Lines that are not the failure](#lines-that-are-not-the-failure)
- [Cases that fail everywhere](#cases-that-fail-everywhere)
- [Cases that CTP does not reproduce](#cases-that-ctp-does-not-reproduce)
- [Running one case again](#running-one-case-again)
- [Cores and fatal errors](#cores-and-fatal-errors)

## Where to look

A failed case is `[NOK]` on the console. Then, in the run directory `$CTP_HOME/result/isolation/current_runtime_logs`:

| file | what it holds for the case |
|---|---|
| `feedback.log` | the verdict line, and after the `D I F F` banner the base answer and the normalized result side by side — `<` a line only the answer has, `>` a line only the result has. The status page shows the same block when the case is clicked |
| `test_local.log` | everything `runone.sh` printed: its `set -x` trace and, for each attempt, `Testing <case> (retry count: n)`, the `diff` against the answer and `flag: NOK` |

And beside the case, unless the run had `scenario_disk`:

| file | after a pass | after a failure |
|---|---|---|
| `result/<name>.log` | the normalized result | the normalized result of the last attempt |
| `result/<name>.result` | the raw result | **replaced** by `<name>:NOK` and the side-by-side diff — the raw output is gone |
| `<name>.result` | `<name>:OK.` | from an earlier pass, if there was one |

The diff in `feedback.log` is always against `answer/<name>.answer`, even for a case that has other answers; the
comparison that decided the verdict tried them all.

## Lines that are not the failure

Every case's trace carries two lines that look alarming and are not:

- `ERROR: Cannot remove user INFORMATION_SCHEMA from the database.` — `clean.sh` tries to drop every user but DBA and
  PUBLIC, and 11.5 has a system user it cannot drop.
- `find: '/home/<you>/CUBRID/log': No such file or directory` — `runone.sh` empties `~/CUBRID/log` before every case,
  whether or not there is one.

## Cases that fail everywhere

At upstream develop's head (engine `f1ae86ff7`, cases `6ab786aa9`), seven cases failed every attempt in two CTP runs and
in both of this runner's, one slot and four ([`isolation-baseline.md`](../../project/evidence/isolation-baseline.md) §4):

- `_01_ReadCommitted/cbrd_21506/unique_index/insert_update_04`
- `_01_ReadCommitted/function/counter_function/delete_delete_rownum_01`
- `_01_ReadCommitted/partition_table/range/dml_ddl/reorganization_select_01`
- `_05_ReadCommitted_RepeatableRead/index_column/common_index/basic_sql/insert_delete_05`
- `_06_features/cbrd_22705_online_index_parallel/normal_index/insert_update_04`
- `_06_features/cbrd_22705_online_index_parallel/unique_index/insert_update_04`
- `_06_features/cbrd_22705_online_index_parallel/dml_online_index/insert_odku_online_index_01`

They fail under CTP on the same engine and the same cases, so they are not the runner's — and each of them now
has a reason ([`isolation-always-failing.md`](../../project/evidence/isolation-always-failing.md)):

- `partition_table/range/dml_ddl/reorganization_select_01` **crashes the server**. One client, a range-partitioned
  table of 100,000 rows and its `group by` are enough, and `runone.sh`'s core check cannot see it (below).
- the three `insert_update_04` and `insert_odku_online_index_01` each print the lines of two clients that one
  lock demotion released, in the order this machine produces and not the order their answers hold. Every
  attempt is the answer with two adjacent lines swapped.
- `insert_delete_05` leaves a select and an insert unordered, and each controller loses a different half of it.
- `delete_delete_rownum_01` is the one case that fails under CTP and **passes here**, in all four whole-corpus
  runs and three times alone: its `MC: wait until C1 ready;` names a client that is idle.

## Cases that CTP does not reproduce

Fifteen more changed verdict between runs, under CTP as well as here. Each was rerun alone, three times under each
runner:

**Flips from run to run**, alone as much as in a whole run:

- `_01_ReadCommitted/index_column/common_index/groupby/delete_select_06`
- `_02_RepeatableRead/no_index_column/aggregate/insert_select_02_1`
- `_02_RepeatableRead/no_index_column/basic_sql/select_insert_01`
- `_02_RepeatableRead/partition_table/range/with_index/unique_with_key/insert_update_03`
- `_02_RepeatableRead/primary_key_column/basic_sql/delete_select_14` — fails far more often than it passes
- `_04_RepeatableRead_ReadCommitted/no_index_column/aggregate/delete_select_03`

**Passes alone every time, and failed inside a run:**

- `_01_ReadCommitted/catalog/db_index_04`
- `_02_RepeatableRead/catalog/db_index_key_03`
- `_02_RepeatableRead/catalog/db_index_key_05`
- `_04_RepeatableRead_ReadCommitted/index_column/common_index/aggregate/max/insert_select_01_2`
- `_06_features/cbrd_22705_online_index_parallel/dml_online_index/insert_odku_online_index_04`

**Passes everywhere but with four slots**, where each slot's `ctldb` has seen a different sequence of cases:

- `_04_RepeatableRead_ReadCommitted/dml_ddl/createindex_02`
- `_05_ReadCommitted_RepeatableRead/dml_ddl/createindex_01`
- `_06_features/cbrd_22705_online_index_parallel/dml_ddl/createindex_02`
- `_06_features/cbrd_22705_online_index_parallel/create_ddl/show_001`

A failure among these is not evidence about the change under test until it fails alone as well.

## Running one case again

Copy the case and its answers into a tree of their own, keeping the path under the isolation root, and point a conf at
it:

```bash
src=/path/to/cubrid-testcases/isolation
case=_02_RepeatableRead/catalog/db_index_key_03
mkdir -p /tmp/one/$(dirname $case)/answer
cp $src/$case.ctl /tmp/one/$(dirname $case)/
cp $src/$(dirname $case)/answer/$(basename $case).answer* /tmp/one/$(dirname $case)/answer/
printf 'scenario=/tmp/one\ntestcase_timeout_in_secs=300\ntestcase_retry_num=0\n' > /tmp/one.conf
TESTKIT_NATIVE=isolation TESTKIT_CONTAIN=1 testkit isolation -c /tmp/one.conf
```

`testcase_retry_num=0` shows the first attempt's verdict rather than the best of five. Run it three times before
drawing a conclusion about a case in the tables above.

## Cores and fatal errors

A core file under `$ctlpath`, `$CUBRID` or the case's directory, `FATAL ERROR` in `$CUBRID/log`, or a crash report the
server wrote about its own death fails the case, and the run says so on standard error as it happens.

**The check is this runner's, and `~/error_backup` is not written by default** ([ADR-021](../../project/adr/ADR-021-crash-reports.md)).
CTP's own check and its backup are one switch — `runone.sh -n` turns off both — and its backup stops the service and
copies the whole install into `~/error_backup/error_<version>_<timestamp>.tar.gz` for every case that finds something.
This runner passes `-n` and looks for itself, after every case, in the slot that owns the install:

| what it finds | what the case says | what is kept |
|---|---|---|
| `$CUBRID/log/coredump/*.coredump`, the engine's own report | `found crash report <file> (cub_server ctldb, <frame>)` | the report, in `current_runtime_logs/crash/` |
| a `core.*` file (CTP's own pattern, less `core.log`) | `found core file <path>` | its gdb stack, in the same directory. The core itself stays where it fell and goes with the slot |
| new `FATAL ERROR` lines in `$CUBRID/log` | `found fatal error in <file> (n line(s))` | the count; the log is the slot's |

Only what is *new since the case before it* counts: nothing sweeps those places between cases, and a log that already
held a fatal error would otherwise fail every case after the one that wrote it — which is what CTP's check did.

`backup_core_file_yn=yes` puts CTP's behaviour back: its check, and the backup with it.

**CTP's check misses a crash; this runner has its own.** CTP looks for files named `core.*` and for `FATAL ERROR`
in `$CUBRID/log`. Where `/proc/sys/kernel/core_pattern` hands cores to a crash handler such as apport, a server that
dies of a signal leaves no `core.*` and no `FATAL ERROR` — only its own call stack in
`$CUBRID/log/coredump/cub_server_<timestamp>.coredump`. That is what
`partition_table/range/dml_ddl/reorganization_select_01` does on this machine
([`isolation-always-failing.md`](../../project/evidence/isolation-always-failing.md) §1), and under CTP it is
reported as an ordinary diff.

After every case this runner reads that directory itself, and **a report it has not seen before fails the case**
([ADR-021](../../project/adr/ADR-021-crash-reports.md)):

```
[NOK] …/reorganization_select_01.ctl
 : NOK found crash report cub_server_20260917125012.888.coredump
   (cub_server ctldb, qdata_save_agg_hentry_to_list at query_aggregate.cpp:2974)
```

The report is copied into `current_runtime_logs/crash/<case>.<report>` before the slot closes — a slot's install
is an overlay that goes with it — and the run says so on standard error as well. A case that passes under CTP and
crashes a server fails here, which is a verdict difference this project chooses to have.
