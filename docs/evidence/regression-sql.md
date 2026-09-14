# Regression evidence: sql and medium

- **Date:** 2026-09-12
- **What this is:** ADR-017's gate. CTP and sqlsuite over the whole sql and medium corpora, one case
  at a time, on the same machine, with all three repositories at upstream develop's head — and what
  the two runners produced, compared file by file.
- **Method:** ADR-013, measured before judged. `sql/baseline.sh` runs CTP's side, `sql/gate.sh`
  sqlsuite's and the comparison; `sql/gate.py` is the comparison itself.

---

## 1. What was run

| | |
|---|---|
| Pins | engine `c3967ec2` (11.5.0.2568), cases `b10727db`, CTP `a1bec876` — all three at upstream develop's head, checked before and after (`sandbox.sh check`) |
| Corpora | sql 17,459 cases, medium 975 (`medium_dev.conf`) |
| CTP | `bash bin/ctp.sh <task> -c <conf>`, inside `in-ns.sh`: its own namespaces, so a run cannot meet another process's server |
| sqlsuite | `TESTKIT_NATIVE_SQL=1 TESTKIT_CONTAIN=1 testkit <task> -c <conf>`, one slot, no `TESTKIT_SLOT_VOLATILE` — the gate is about what a run produces under CTP's own conditions |
| Machine | 16 cores, 30 GB; the sandbox and the slot's layer on the same disk |
| Compared | every case's verdict, every `.result` on its bytes, `main.info`, and all 2,762 (sql) / 22 (medium) record files |

Two runs of each side, because a difference between two runners means nothing until the difference
between two runs of one runner is known (§3).

## 2. Result

**medium: identical, in every file.**

| | CTP | sqlsuite |
|---|---:|---:|
| OK / NOK | 975 / 0 | 975 / 0 |
| `.result` identical to CTP's | — | **975 of 975** |
| `main.info` | — | identical |
| record files | 22 | **22 identical** |
| wall | 91 s | 101 s |

**sql: a clean sqlsuite run is a clean CTP run, in all 17,459 files.**

| | CTP run 1 | CTP run 2 | sqlsuite run 1 | sqlsuite run 2 |
|---|---:|---:|---:|---:|
| OK / NOK | 17,459 / 0 | 17,457 / 2 | 17,458 / 1 | **17,459 / 0** |
| wall | 1,726 s | 1,743 s | 1,753 s | 1,695 s |
| `.result` differing from CTP run 1 | — | 2 | 2 | **0 of 17,459** |

sqlsuite's second run against CTP's first, the two that passed everything:

| | |
|---|---|
| verdicts | identical, 17,459 of 17,459 |
| `.result` | **byte for byte identical, 17,459 of 17,459** |
| `main.info` | identical |
| record files | **2,762 of 2,762 identical** |

Neither runner is stable on two cases — `_01_object/_01_type/_003_numeric/1004` and
`_36_guava/partition_table/cbrd_25542` — and those two are the whole of what moved anywhere: CTP's
own two runs differ on exactly them, in verdict and in the bytes of their `.result`, and so do
sqlsuite's. Every run that failed neither is identical to every other run that failed neither.

## 3. The two cases

Both are cases whose result is missing a block the answer has, and neither is stable in CTP alone.

| case | CTP run 1 | CTP run 2 | sqlsuite run 1 | sqlsuite run 2 |
|---|---|---|---|---|
| `_003_numeric/1004` | OK | NOK — the table from `select * from v1` is absent | NOK — `Error:0` in its place | OK |
| `partition_table/cbrd_25542` | OK | NOK — the `Query Plan:` and `Trace Statistics:` blocks are absent | OK, byte for byte CTP run 1's | OK |

Two runs of each runner, and each runner produced one clean run and one with these cases missing a
block. **The instability is the engine's, not either runner's**, and the gate is read against it:
what the two runners produce when neither hits it is identical.

- `1004` creates a view whose columns are declared `numeric(0)`, `bit(0)`, `char(0)`, and selects
  through it. The answer has the row; a failing run has it missing, and sqlsuite's failing run has
  `Error:0` — a driver exception carrying no CUBRID error code.
- **`Error:` is CTP's own line.** It is written by `ConsoleDAO.java`, which both runners execute:
  sqlsuite runs CQT's `executeSqlFile` and records nothing of its own here (ADR-016). So the
  difference in those bytes is a difference in what the driver returned, not in what the runner
  wrote.
- `cbrd_25542` is the `show trace` shape §4 of `sql-native.md` describes: what a trace prints
  depends on state earlier cases leave in the server, and here it moved between two runs of CTP
  itself, in the same order.

## 4. What this does not cover

- **Slots.** The gate is one case at a time. Many slots move which cases share a database, and what
  that costs is `sql-native.md` §4: CQT's server-message flag, which is reproduced, and the corpus's
  own order dependencies, which are not.
- **The settings that make a run fast.** `TESTKIT_SLOT_VOLATILE` and a slot root on a fast disk are
  measured in `sql-native.md` §3 and were not used here.
- **The two cases above**, as a defect. They are unstable in both runners at these pins; nothing
  here says why, and a report upstream needs a reproduction rather than four runs.

## 5. Reproducing it

```bash
docs/evidence/sql/sandbox.sh refresh <a CUBRID install built at upstream develop>
SANDBOX=/data/cub_sys/projects/regr-sql docs/evidence/sql/baseline.sh     # CTP's side
SANDBOX=/data/cub_sys/projects/regr-sql docs/evidence/sql/gate.sh <testkit>   # sqlsuite's, and the comparison
```

`gate.py` normalises what two runs cannot share — the clock, the run's own directory name, and the
order of the children in a `summary_info` and of the cases in the JUnit report, which is JDK 8
`Hashtable` iteration over paths carrying the run's timestamp. That order is checked byte for byte
elsewhere, by `records.sh`, against a single run's own verdicts. Before it judged anything it was
run on CTP against itself: two medium runs and two sql runs agree on all 22 and all 2,762 record
files, and `medium.conf` against `medium_dev.conf` is reported as 579 verdicts apart.
