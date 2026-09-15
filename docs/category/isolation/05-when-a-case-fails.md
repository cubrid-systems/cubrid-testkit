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

At upstream develop's head (engine `f1ae86ff7`, cases `6ab786aa9`), eight cases failed every attempt in two CTP runs and
in this runner's ([`isolation-baseline.md`](../../project/evidence/isolation-baseline.md) §4):

- `_01_ReadCommitted/cbrd_21506/unique_index/insert_update_04`
- `_01_ReadCommitted/function/counter_function/delete_delete_rownum_01`
- `_01_ReadCommitted/partition_table/range/dml_ddl/reorganization_select_01`
- `_02_RepeatableRead/primary_key_column/basic_sql/delete_select_14`
- `_05_ReadCommitted_RepeatableRead/index_column/common_index/basic_sql/insert_delete_05`
- `_06_features/cbrd_22705_online_index_parallel/normal_index/insert_update_04`
- `_06_features/cbrd_22705_online_index_parallel/unique_index/insert_update_04`
- `_06_features/cbrd_22705_online_index_parallel/dml_online_index/insert_odku_online_index_01`

They fail under CTP on the same engine and the same cases, so they are the engine's or the corpus's to explain, not
the runner's.

## Cases that CTP does not reproduce

Ten more changed verdict between runs, under CTP as well as here. Rerun alone three times under each runner, five
flipped under each, and five passed every time alone while having failed inside a whole run:

| flips from run to run | passes alone, fails in a whole run |
|---|---|
| `groupby/delete_select_06` | `_01_ReadCommitted/catalog/db_index_04` |
| `aggregate/insert_select_02_1` | `_02_RepeatableRead/catalog/db_index_key_03` |
| `basic_sql/select_insert_01` | `_02_RepeatableRead/catalog/db_index_key_05` |
| `unique_with_key/insert_update_03` | `aggregate/max/insert_select_01_2` |
| `aggregate/delete_select_03` | `dml_online_index/insert_odku_online_index_04` |

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

A core file under `$ctlpath`, `$CUBRID` or the case's directory, or `FATAL ERROR` in `$CUBRID/log`, fails the case with
`found core file on host …` or `found fatal error file on host …`, and `runone.sh` backs up the cores and a copy of the
whole install into `~/error_backup/error_<version>_<timestamp>.tar.gz` before recreating `ctldb`. In a slot the backup
is written to the slot's own directory and copied into the real `~/error_backup` when the slot closes; the run says so
on standard error. `backup_core_file_yn=no` turns the check and the backup off.
