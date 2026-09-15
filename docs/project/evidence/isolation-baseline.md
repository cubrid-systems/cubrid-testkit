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

What four slots hold (`tk-sample-p4-mem`, `probe-mem.sh`: the same run again, 58 OK, with the resident memory of
every process run from `tk/` sampled once a second): **2,813 MB at the peak** — four `cub_server` 2,378 MB (595 MB
each, with the shipped `data_buffer_size=512M` and `log_buffer_size=256M`), four `cub_pl` 226 MB, eight `qacsql`
116 MB, four `qactl` 51 MB, four `cub_master` 40 MB. Beside the server a slot is about 110 MB. The sample is light: a
sql slot's server reached 1.38 GB over its corpus with the same buffers (`sql-native.md` §3), and the default slot
count is sized at that — 1.5 GB a slot, after 2 GB for the rest of the machine, no more slots than CPUs, four at most
unless `parallel_slots` says otherwise.

**The default** (`tk-sample-default`, `run-default.sh`: `sample-tk.conf`, which does not set `parallel_slots`, on this
machine's 16 CPUs and 19.5 GB available): standard error says `parallel_slots is not set: 4 slots, the default`, four
`[ENV START]` lines, 58 OK in 32 s. Against `sample-1` every verdict file, `feedback.log` record and 58 of 58 normalized
results are the same; the console markers differ by the three extra `[ENV START]` and three extra `[ENV STOP]` lines.

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
client's `rows affected` among the other's, a unique-constraint error landing on a different statement, and in
`delete_select_03` — which sleeps 30 s by its own `MC: sleep 30;` before the part it tests — where C1's
`2000 rows affected` lands among C2's lines. The other five pass every time alone, under both, and failed only inside a whole run: what they
depend on is what the corpus left in `ctldb` before them, or the load around them, and not the runner. The three cases
that failed in both CTP runs and passed in the native one are in both groups: `delete_select_06` and
`insert_select_02_1` fail alone under both runners, and `db_index_key_03` passes alone under both.

Wall time says the same: a rerun that met its flips took 173–208 s, one that did not 80–88 s, under either runner.

### Four slots

`tk-full-p4`: the native runner over the whole corpus again, `parallel_slots=4`, alone on the machine but for other
users' load.

| run | wall | case seconds | NOK |
|---|---:|---:|---:|
| `tk-full-1`, one slot | 12,301 s | 12,289 | 11 |
| `tk-full-p4`, four slots | **2,978 s** | 11,894 | 14 |

4.1 times faster than `tk-full-1`, which shared the machine with `full-2`, and 3.7 times faster than CTP alone — the
fairer serial figure ([below](#serial-and-parallel-case-by-case)). The run directory's check, dispatch sets and
snapshot are the same as the one-slot run's. Against the verdicts, ADR-018's rules:

- **Four cases that both CTP runs passed fail**, every attempt: `_04_RepeatableRead_ReadCommitted/dml_ddl/createindex_02`,
  `_05_ReadCommitted_RepeatableRead/dml_ddl/createindex_01`, and in `_06_features/cbrd_22705_online_index_parallel`
  `dml_ddl/createindex_02` and `create_ddl/show_001`. Three of the diffs are catalog rows in a different order, the
  fourth a `1 row affected` in a different place. Each slot's `ctldb` has seen a different sequence of cases.
- **Rerun alone** (`mini2/`), three times under each runner: all four pass every time, under both.
- **`_02_RepeatableRead/primary_key_column/basic_sql/delete_select_14`**, which failed in all three earlier runs,
  passes. Alone it fails three of three under CTP and two of three under the native runner — unstable, not always
  failing.

No runner difference with four slots either. Across the four whole runs **seven cases fail in every one**, and fifteen
are unstable: the ten above, `delete_select_14`, and the four a slot's history exposes.

Whether a failure is the engine's or the run's own instability is what a second run decides. `full-2` is the same run
again over a restored corpus, started at 04:55 alongside the native runner's whole-corpus run `tk-full-1` — two serial
runs on separate copies, which is a load this machine carries without effort, but the order is recorded because the
cases wait on locks and on `sleep`.

### Serial and parallel, case by case

What four slots gain, and why. Every number here is read from the runs' `feedback.log` — the summary's elapsed time
and each case's own milliseconds — by `perf.py`; the console logs are not read.

| run | wall | cases a minute | case seconds | case seconds / wall | NOK |
|---|---:|---:|---:|---:|---:|
| `full-1` CTP, alone | 11,095 s (3 h 5 m) | 36.6 | 11,092 | 1.00 | 13 |
| `full-2` CTP, beside `tk-full-1` | 12,423 s | 32.7 | 12,420 | 1.00 | 16 |
| `tk-full-1` native, one slot, beside `full-2` | 12,301 s (3 h 25 m) | 33.0 | 12,289 | 1.00 | 11 |
| `tk-full-p4` native, four slots | **2,978 s (49.6 m)** | **136.5** | 11,894 | **4.00** | 14 |

**The serial figure is `full-1`'s.** The two runs that shared the machine lost their time in the shortest cases and
nowhere else. Against `full-1`, the cases under a second took +1,249 s in `tk-full-1` and +1,278 s in `full-2` — about
0.23 s each over 5,467 cases — while the cases of a second or more took the same or slightly less, by 3.3% at most.
Four slots against `full-1` is **3.7 times**; against `tk-full-1` it would read 4.1, and the difference is the
neighbour, not the slots.

**Four slots barely slow the cases down.** The slots were busy 4.00 times the wall time, and the cases took 11,894 s
together against 12,289 s in one slot. Case by case, over the 6,755 cases that passed in both runs, four slots' time
over one slot's is 1.00 at the median, 0.98 at the 10th percentile, 1.04 at the 90th and 2.27 at the 99th:

| a case's time in one slot | cases | one slot | four slots | change |
|---|---:|---:|---:|---:|
| under 1 s | 5,459 | 3,629 s | 3,832 s | +5.6% |
| 1–3 s | 689 | 1,319 s | 1,358 s | +3.0% |
| 3–10 s | 379 | 2,032 s | 2,069 s | +1.8% |
| 10–30 s | 187 | 2,233 s | 2,227 s | −0.3% |
| 30 s or more | 41 | 2,720 s | 2,119 s | −22% |

Two things are in those rows and are not the cost of running four at once:

- **The setup, once a slot.** A slot's first case also creates `ctldb` and builds ctltool. It took 10.4 s in one slot
  (`changing_owner_01`) and 11.9–13.1 s for each of four slots doing it at the same moment. Those four first cases are
  the largest per-case slowdowns of the four-slot run.
- **One unstable case.** `_05_ReadCommitted_RepeatableRead/partition_table/range/with_index/unique_with_key/insert_insert_01`
  took 0.7 s in `full-1`, 902.8 s in `tk-full-1` and 301.4 s in `tk-full-p4` — steps of the 300 s timeout, an attempt
  at a time. CTP is no steadier with it: it and its `_02_RepeatableRead` twin took 0.7 s each in `full-1` and about
  602 s each in `full-2`. It is the whole −22% of the last row and the whole 0.69 of `_05_ReadCommitted_RepeatableRead`
  below. Without it four slots' cases take 272 s (+2.5%) longer than one slot's.

| directory | cases | one slot | four slots | four / one |
|---|---:|---:|---:|---:|
| `_01_ReadCommitted` | 2,835 | 4,832 s | 5,088 s | 1.05 |
| `_02_RepeatableRead` | 1,372 | 2,231 s | 2,250 s | 1.01 |
| `_04_RepeatableRead_ReadCommitted` | 1,257 | 2,598 s | 2,513 s | 0.97 |
| `_05_ReadCommitted_RepeatableRead` | 850 | 1,927 s | 1,327 s | 0.69 |
| `_06_features` | 454 | 693 s | 708 s | 1.02 |
| `_07_serializable` | 4 | 7 s | 7 s | 1.03 |

**Why it scales:** an isolation case is mostly waiting — on a lock, on a `sleep`, on the controller — and in a serial
run every wait holds the whole queue. In `tk-full-1` the median case is 0.70 s and the mean 1.81 s; the slowest 1% of
cases (67) are 28.3% of the case time, the slowest 5% (338) 49.2%, the slowest 10% 61.0% and the slowest 25% 73.2%. With
four slots the other three keep going while one waits. What four slots cost is the short cases' few percent, the
memory (2,813 MB at the peak of the sample, §2), and the four failures a slot's own `ctldb` history exposes (above).

**The sample understates it.** Sixty cases are too few to spread the setup over:

| run | wall | faster | case seconds | first case of each slot |
|---|---:|---:|---:|---|
| `tk-sample-3`, one slot | 76 s | — | 75 | 12.0 s |
| `tk-sample-p4`, four slots | 38 s | 2.0× | 132 | 16.9, 17.5, 18.1, 18.1 s |
| `tk-sample-default`, four by default | 32 s | 2.4× | 111 | 11.8, 12.2, 12.8, 12.9 s |

Four setups at once are 70.6 of `tk-sample-p4`'s 132 case seconds and 49.8 of `tk-sample-default`'s 111; the 6 s between
the two four-slot runs is almost all setup, which took 17–18 s a slot in the first and 12–13 s in the second. Over the
corpus the four setups are 50.6 of 11,894 case seconds.

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
python3 perf.py runs/perf.txt    # serial against parallel, from the runs' feedback.log
```

Before a run: restore `scenario/` from `cubrid-testcases/isolation` (the previous run's `result/` directories are in
it), and check the three repositories against upstream develop.
