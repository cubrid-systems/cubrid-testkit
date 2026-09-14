# The isolation baseline

- **Date:** 2026-09-15
- **What this is:** CTP running the `isolation` suite on this machine, measured before any of it is rewritten. It is the
  P0 of `design/module-isolation.md`: the numbers the design is sized by, the facts the analysis documents were missing,
  and the reference a native runner will be compared against. Everything here is measured or read from source.
- **All three at upstream develop:** **engine** `cubrid/cubrid` `f1ae86ff7` (11.5.0.2574-f1ae86f). **Cases:**
  `cubrid/cubrid-testcases` `6ab786aa9`. **CTP:** `cubrid/cubrid-testtools` `a1bec87`.
- **Status:** the sample is done; the whole-corpus run is in progress and §4 is filled in when it ends.

---

## 1. The sandbox

`/data/cub_sys/projects/regr-iso`, next to the shell and sql sandboxes. **Every tree in it is a copy**, because a CTP
isolation run changes all of them:

| it | does | so |
|---|---|---|
| Deploy | `cubrid service stop`, `kill -9` of the user's `cub_admin`, `cub_master`, `cub_server`; appends `inquire_on_exit=3` to `$CUBRID/conf/cubrid.conf` — **every run**, measured: two lines after two runs | the install is a copy |
| the update step | `chmod u+x *.sh` in `isolation/ctltool` — the scripts are committed `100644` — and `upgrade.sh`, which prints the whole environment to the console (so `runs/*/stdout.log` is not something to publish) | the CTP tree is a copy |
| `prepare.sh` | `pkill -9 -u $(whoami) cub`, `deletedb` / `createdb ctldb`, and `make clean qactl qacsql` **in the CTP tree** | the CTP tree is a copy |
| `runone.sh` | `pkill -u $(whoami) -9 sleep` after every case; `runone.log`, `.test.log`, `timeout.log`, `csql.err` in `$ctlpath` | the run is inside a PID, IPC, network and mount namespace (`in-ns.sh`) |
| the cases | `result/<name>.result`, `result/<name>.log` and `<name>.result` beside every case | the cases are a copy (`scenario/`); the git checkout is only read |

| | |
|---|---|
| `cubrid-testcases/` | sparse clone of `isolation/` at `6ab786aa9`, never written |
| `scenario/` | a copy of it — the full run's corpus |
| `sample/` | six topics of it plus one deadlock case (§2) |
| `CTP/` | `regr-sql/CTP` (`a1bec87`) without its results |
| `CUBRID/` | built from `regr/engine-src` checked out at `f1ae86ff7`, preset `release`; `conf/cubrid.conf.shipped` is the conf as installed |
| `isolation.conf`, `sample.conf` | CTP's shipped `conf/isolation.conf` — `testcase_timeout_in_secs=300`, `testcase_retry_num=4` — local mode, with the corpus's own exclusion list |
| `run-ctp.sh <conf> <label>` | runs `ctp.sh isolation` inside `in-ns.sh` and copies what it wrote to `runs/<label>/` |

The install's `cubrid.conf` is left as shipped. `prepare.sh` passes the volume sizes itself (50M each), so the conf's
`db_volume_size` does not reach `ctldb`.

## 2. The sample

`_01_ReadCommitted/cbrd_22202_invisible_indexes`, `_02_RepeatableRead/function`, `_02_RepeatableRead/serial`,
`_04_RepeatableRead_ReadCommitted/serial`, `_05_ReadCommitted_RepeatableRead/dml_ddl`, `_07_serializable/issues`, and
one case that waits for deadlock resolution: 60 cases, 44 with two clients, 12 with three, 3 with five.

| run | cases | OK | NOK | skipped | wall |
|---|---:|---:|---:|---:|---:|
| `sample-1` | 60 | 58 | 0 | 2 (the exclusion list) | 78 s |

The run directory holds **seven files**: `check_local.log`, `dispatch_tc_ALL.txt`, `dispatch_tc_FIN_local.txt`,
`feedback.log`, `main_snapshot.properties`, `test_local.log`, `test_status.data`. There is no `current_task_id`,
`monitor_local.log` or JUnit report, which a shell run has. `test_local.log` is 7,497 lines for 58 cases, almost all of
it `runone.sh`'s `set -x` trace.

CTP's own count per case: median 0.61 s, 90th percentile 1.72 s. The first case took 11.9 s because it paid for the
setup — `createdb` and building ctltool. The next longest were `dml_ddl/createindex_02` (10.3 s),
`serial/serial_06` (3.7 s), the deadlock case (3.6 s) and `dml_ddl/droptable_01` (2.7 s).

### The native runner on the same sample

`tk-sample-3`: `testkit isolation` under `TESTKIT_CONTAIN=1 TESTKIT_NATIVE=isolation`, one slot, over separate copies
of the install, CTP and the sample (`tk/`) — an overlay must not sit on a lower layer another run is writing. 58 OK,
76 s. Against `sample-1`, by `compare-sample.sh`, with paths, dates and elapsed times masked and CTP's `find` order
sorted:

| | |
|---|---|
| file list, `check_local.log`, dispatch sets, `test_status.data`, `feedback.log` (frame and every case record), snapshot keys, console markers | same |
| `result/<name>.log`, the normalized result of every case | **58 of 58 byte-identical** |
| `test_local.log` | same length and the same blank lines. 23 of 58 case blocks differ once numbers are masked, and none by content: 2 carry the setup — `createdb` and the ctltool build — on a different first case, because CTP's first case is `find`'s and the runner's is the sorted first; the rest differ in the order bash's trace of a pipeline's commands arrives in, and in which case of a directory is the one that runs `mkdir result` |

**Four slots** (`tk-sample-p4`, `parallel_slots=4`): 58 OK in 38 s against 76 s for one. Every verdict file,
`feedback.log` record and normalized result is the same as CTP's. What differs is what four slots are: each prints its
own `[ENV START]` and `[ENV STOP]`, and writes its own setup and `Stop service` into the worker log — as shell's slots
do, and for the same reason, one EnvID so that a parallel run writes the files a serial one writes.

Two runs came before it, and each failure is a fact about CTP rather than about the runner. `tk-sample-1` failed 58 of
58 on `timeout3.sh: Permission denied`: the scripts are committed without an execute bit, and CTP sets it in its
update step (`spec-corrections.md` §8). `tk-sample-2` wrote one blank line per case too many into the worker log:
CTP trims what a script prints before logging it, and reads only standard output between two markers
(`SSHConnect.extractOutput`).

## 3. The corpus

At `6ab786aa9`:

| | |
|---|---:|
| `.ctl` cases | 6,790 |
| `answer/<name>.answer` | 6,865 |
| `.answer1` / `.answer2` / `.answer_1` | 58 / 6 / 3 |
| cases with more than one answer | 59 |
| misspelled answers (`.asnwer` 2, `.amswer` 1) and `compare.log` — not matched by `answer*`, never read | 4 |
| cases with no answer | 0 |
| `answer/` directories | 189 |
| `<name>.sql` or `<name>.sh` beside a case (hooks `runone.sh` supports) | 0 |
| entries in `config/daily_regression_test_excluded_list_linux.conf` | 18 |
| `MC: sleep` in total | 3,414 s |

Clients per case (`MC: setup NUM_CLIENTS`): 38 × 1, 3,687 × 2, 2,734 × 3, 243 × 4, 7 × 5, 78 × 6, 1 × 8, 2 × 22.

Top-level directories: `_01_ReadCommitted`, `_02_RepeatableRead`, `_04_RepeatableRead_ReadCommitted`,
`_05_ReadCommitted_RepeatableRead`, `_06_features`, `_07_serializable`, and `config/`. The largest topics are
`_01_ReadCommitted/index_column` (925), `_04_RepeatableRead_ReadCommitted/index_column` (612) and
`_01_ReadCommitted/partition_table` (539).

## 4. The whole corpus

`full-1`, 01:48 to 04:54, over `scenario/`, on a machine whose load average was 16 from other work:

| cases | excluded | run | OK | NOK | wall |
|---:|---:|---:|---:|---:|---:|
| 6,790 | 18 | 6,772 | 6,759 | **13** | 11,095 s (3 h 5 m) |

CTP's own count of case time adds up to 11,092 s of those 11,095: the Java around the cases costs nothing that shows,
and the time is `runone.sh`'s. Median 0.70 s a case, 90th percentile 2.71 s, 99th 12.84 s, longest 112.7 s.

| seconds a case | under 1 | 1–9 | 10–19 | 20 or more |
|---|---:|---:|---:|---:|
| cases | 5,467 | 1,071 | 180 | 54 |

| directory | cases | case seconds | NOK |
|---|---:|---:|---:|
| `_01_ReadCommitted` | 2,835 | 4,927 | 5 |
| `_02_RepeatableRead` | 1,372 | 1,945 | 3 |
| `_04_RepeatableRead_ReadCommitted` | 1,257 | 2,490 | 1 |
| `_05_ReadCommitted_RepeatableRead` | 850 | 1,019 | 1 |
| `_06_features` | 454 | 695 | 3 |
| `_07_serializable` | 4 | 16 | 0 |

The thirteen failures. Every one failed all five attempts (`testcase_retry_num=4`), and none reported a core or a fatal
error:

| case seconds | case |
|---:|---|
| 2.7 | `_05_ReadCommitted_RepeatableRead/index_column/common_index/basic_sql/insert_delete_05` |
| 3.4 | `_01_ReadCommitted/function/counter_function/delete_delete_rownum_01` |
| 4.3 | `_01_ReadCommitted/catalog/db_index_04` |
| 4.5 | `_01_ReadCommitted/cbrd_21506/unique_index/insert_update_04` |
| 17.9 | `_01_ReadCommitted/index_column/common_index/groupby/delete_select_06` |
| 49.4 | `_01_ReadCommitted/partition_table/range/dml_ddl/reorganization_select_01` |
| 112.7 | `_04_RepeatableRead_ReadCommitted/index_column/common_index/aggregate/max/insert_select_01_2` |
| 3.8 | `_02_RepeatableRead/no_index_column/aggregate/insert_select_02_1` |
| 4.6 | `_02_RepeatableRead/catalog/db_index_key_03` |
| 3.6 | `_02_RepeatableRead/primary_key_column/basic_sql/delete_select_14` |
| 4.6 | `_06_features/cbrd_22705_online_index_parallel/normal_index/insert_update_04` |
| 4.5 | `_06_features/cbrd_22705_online_index_parallel/unique_index/insert_update_04` |
| 4.6 | `_06_features/cbrd_22705_online_index_parallel/dml_online_index/insert_odku_online_index_01` |

### Three runs of the whole corpus

`full-2` (CTP again, corpus restored) and `tk-full-1` (the native runner, one slot, on the `tk/` copies) ran side by side
from 04:55; `full-1` had run alone.

| run | NOK | case seconds |
|---|---:|---:|
| `full-1` CTP | 13 | 11,092 |
| `full-2` CTP | 16 | 12,420 |
| `tk-full-1` native | 11 | 12,289 |

- **Eight cases fail in all three**, every attempt: `cbrd_21506/unique_index/insert_update_04`,
  `function/counter_function/delete_delete_rownum_01`, `partition_table/range/dml_ddl/reorganization_select_01`
  (all `_01_ReadCommitted`); `_02_RepeatableRead/primary_key_column/basic_sql/delete_select_14`;
  `_05_ReadCommitted_RepeatableRead/index_column/common_index/basic_sql/insert_delete_05`; and three in
  `_06_features/cbrd_22705_online_index_parallel` — `normal_index/insert_update_04`, `unique_index/insert_update_04`,
  `dml_online_index/insert_odku_online_index_01`.
- **CTP against itself moves seven verdicts**, and not because of order: both CTP runs dispatched in the same order
  (`find`'s, over two copies of the same tree). Several of the seven are catalog listings whose rows come back in a
  different order, and several are the position of one client's `rows affected` among the other's lines.
- **The native runner against `full-2`**: the run directory's check, dispatch sets and snapshot are the same; the
  verdicts differ on seven cases, and 9 of 6,772 normalized results differ — those seven, and two cases that passed
  in both by matching different answers: `primary_key/update_delete_06` and `basic_sql/update_update_05_complex`,
  whose results match `.answer1` under CTP and `.answer` under the native runner. Both answers are the case's own.
- **Three cases failed in both CTP runs and passed in the native one**: `groupby/delete_select_06`,
  `_02_RepeatableRead/catalog/db_index_key_03` and `no_index_column/aggregate/insert_select_02_1`. The runner sorts,
  so a case can follow a different case than it does under CTP — but `delete_select_06` follows
  `delete_select_05` in both orders, so for that one at least the predecessor is not the explanation.

### The ten cases, alone

The ten cases whose verdict moved, in a corpus of their own (`mini/`), three times under CTP and then three times under
the native runner, one run at a time:

| case | CTP alone | native alone | whole runs (`full-1`, `full-2`, `tk-full-1`) |
|---|---|---|---|
| `_01_ReadCommitted/index_column/common_index/groupby/delete_select_06` | OK OK NOK | NOK NOK NOK | NOK NOK OK |
| `_02_RepeatableRead/no_index_column/aggregate/insert_select_02_1` | NOK NOK OK | NOK OK NOK | NOK NOK OK |
| `_02_RepeatableRead/no_index_column/basic_sql/select_insert_01` | NOK OK OK | NOK OK NOK | OK NOK OK |
| `_02_RepeatableRead/partition_table/range/with_index/unique_with_key/insert_update_03` | OK NOK NOK | OK OK OK | OK NOK NOK |
| `_04_RepeatableRead_ReadCommitted/no_index_column/aggregate/delete_select_03` | OK OK NOK | NOK OK NOK | OK NOK NOK |
| `_01_ReadCommitted/catalog/db_index_04` | OK OK OK | OK OK OK | NOK OK OK |
| `_02_RepeatableRead/catalog/db_index_key_03` | OK OK OK | OK OK OK | NOK NOK OK |
| `_02_RepeatableRead/catalog/db_index_key_05` | OK OK OK | OK OK OK | OK NOK OK |
| `_04_RepeatableRead_ReadCommitted/index_column/common_index/aggregate/max/insert_select_01_2` | OK OK OK | OK OK OK | NOK OK NOK |
| `_06_features/cbrd_22705_online_index_parallel/dml_online_index/insert_odku_online_index_04` | OK OK OK | OK OK OK | OK NOK OK |

**No case separates the runners.** Five flip from run to run under each runner on its own — the position of one
client's `rows affected` among the other's, a unique-constraint error landing on a different statement, a wait that
runs out at 30 s. The other five pass every time alone, under both, and failed only inside a whole run: what they
depend on is what the corpus left in `ctldb` before them, or the load around them, and not the runner. The three cases
that failed in both CTP runs and passed in the native one are in both groups: `delete_select_06` and
`insert_select_02_1` fail alone under both runners, and `db_index_key_03` passes alone under both.

Wall time says the same: a rerun that met its flips took 173–208 s, one that did not 80–88 s, under either runner.

Whether a failure is the engine's or the run's own instability is what a second run decides. `full-2` is the same run
again over a restored corpus, started at 04:55 alongside the native runner's whole-corpus run `tk-full-1` — two serial
runs on separate copies, which is a load this machine carries without effort, but the order is recorded because the
cases wait on locks and on `sleep`.

## 5. Read from the source while doing this

The analysis documents were wrong or silent in nineteen places; they are in `spec-corrections.md` §8. The three that
change the design:

- `runone.sh`'s last argument is the **client program**, not a database; the database is always `ctldb`.
- the answers are in **`answer/`**, and a run writes **three files per case** into the cases tree.
- the kills are **the user's**, not the run's — which is why even a one-slot native run is contained.

## 6. Reproducing it

```bash
cd /data/cub_sys/projects/regr-iso
./run-ctp.sh /data/cub_sys/projects/regr-iso/sample.conf sample-1
./run-ctp.sh /data/cub_sys/projects/regr-iso/isolation.conf full-1
```

Before a run: restore `scenario/` from `cubrid-testcases/isolation` (the previous run's `result/` directories are in
it), and check the three repositories against upstream develop.
