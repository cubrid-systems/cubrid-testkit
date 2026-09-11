# The sql and medium baseline

- **Date:** 2026-09-11
- **What this is:** CTP running the `sql` and `medium` suites on this machine, measured before any
  of it is rewritten. It is the P0 of `design/module-sql.md`: the numbers the design is sized by, the
  facts the analysis documents were missing, and the reference a native runner will be compared
  against. Everything here is measured or read from source, and the source lines are given.
- **All three at upstream develop** (§1): **engine** `cubrid/cubrid` `a7a1db84b` (11.5.0.2560-a7a1db8,
  JDBC driver 11.4.0.0078). **Cases:** `cubrid/cubrid-testcases` `b94995abf`. **CTP:**
  `cubrid/cubrid-testtools` `a1bec87`.

---

## 1. The sandbox

`/data/cub_sys/projects/regr-sql`, next to the shell sandboxes. **Every tree in it is a copy**,
because a CTP sql run changes all three of the things it is given:

| it | does | so |
|---|---|---|
| `do_configure` | writes `cubrid.conf`, `cubrid_broker.conf` and `cubrid_ha.conf` into `$CUBRID/conf` | the install is a copy |
| `do_clean` | `pkill cub`, `ipcrm` every segment | the run is inside a PID, IPC and network namespace (`in-ns.sh`) |
| the cases | leave a `.result` beside every `.sql`, **in the testcases tree** — 975 after one medium run, 17,459 after one sql run | the cases are a copy |

`in-ns.sh` is the shell sandbox's wrapper with `--net` added: the suite takes ports 1822 and 33120,
and a namespace of its own is the difference between measuring the suite and measuring whatever else
on the machine holds those ports. PID 1 stays a shell so that orphans are reaped
(`regr/in-ns.sh` explains why that is load-bearing).

### Three repositories, all at upstream develop, checked before every run

The engine, the cases and CTP each move every day, so **a baseline is a statement about three
commits**, and it means nothing without them. The sandbox is laid down from upstream develop and
checked against it before a run starts (`sql/sandbox.sh`):

| | from | how |
|---|---|---|
| engine | `cubrid/cubrid` develop | built with `build_cubrid.sh release` in a worktree of its own, then copied in. `WITH_CMSERVER=OFF`: the Manager server's bundled `libevent.a` calls `sysctl`, which this machine's glibc no longer has, and nothing here needs the Manager |
| cases | `cubrid/cubrid-testcases` develop | `git archive … sql medium` |
| CTP | `cubrid/cubrid-testtools` develop | `git archive … CTP`. CI takes the same branch (`BRANCH_TESTTOOLS=develop`) |

`sandbox.sh check` fetches all three and fails unless the sandbox matches every head.
`sandbox.sh refresh <install>` lays the sandbox down again and moves the previous results to
`archive/`. Nothing is checked out: the checkouts on this machine stay on whatever branch they were
on. `PROVENANCE` records the three commits a run was pinned to.

**Two rules, because the two kinds of run want different things.** A run that measures something
new — `baseline.sh`, and later the gate — does not start unless `check` passes. A run that compares
against a baseline — the spike in §9 — is pinned to that baseline's commits instead, and says so when
upstream has moved. Upstream moved within fifteen minutes of this baseline finishing
(`8cb558b3b`, one engine commit), which is how the second rule came to be written.

**The first measurements were not at develop, and that is why this section exists.** They used a
PR branch of the cases (`tc/pr-7720`, 35 commits behind develop), an engine 7 commits behind (one
of the seven moves the JDBC driver from 11.4.0.0077 to 0078), and a feature branch of CTP (19
behind, with 55 files changed). Those runs are kept in `archive/20260911-140336-c6ff75e/`. What
they found about CTP's behaviour does not depend on the versions and is kept below. Every number
below is from the develop sandbox.

## 2. The smoke run

Three cases, `sql/_01_object/_04_trigger/_004_event_target`: **3 OK, 54 s of wall clock, 30 ms of
it in the cases.** The rest is setup — `make_locale`, `createdb`, compiling and loading the Java
stored procedures — and it is the same fifty-odd seconds whatever the run's size.

The result tree has the four files the analysis describes: `main.info`, `summary_info`,
`summary.info` and `summary.xml`, at the root and again per directory.

## 3. medium

| conf | OK | NOK | wall | in the cases |
|---|---:|---:|---:|---:|
| `medium.conf` | 396 | **579** | 70 s | — |
| `medium_dev.conf` | **975** | 0 | 90 s | 24.7 s |

**`medium.conf` does not work on an 11.x engine, and the reason is one parameter.** The data load
fails at the schema:

```
ERROR: The class 'dba.picture' is marked as REUSE_OID and is non-referable. Non-referable classes
can't be the domain of an attribute and their instances' OIDs cannot be returned.
Error occurred during schema loading.
```

11.x creates tables as REUSE_OID by default, and the medium schema uses an OID reference.
`medium_dev.conf` is `medium.conf` plus `create_table_reuseoid=no`, and says so in a comment: *"It
needs to CUBRID 11.x over"*. The 579 failures that follow are `Error:-493` and `-494`, which are
queries against tables that were never loaded. CTP does not notice: `make_db_data` sends `loaddb`'s
output to the log file and carries on (`run.sh`, `make_db_data`). This closes the open question in
`analysis/medium/implementation-notes.md` about when `create_table_reuseoid` applies.

Inside the namespace `tar -zxvf mdb.tar.gz` also complains that it cannot restore uid 1106's
ownership and exits non-zero. That turned out to be a distraction: the files are extracted and the
script does not check tar's status. It is the same isolation cost shell slots pay for with
`TAR_OPTIONS=--no-same-owner`.

Per case: median **6 ms**, mean 25 ms, maximum 2.9 s.

## 4. sql

| | |
|---|---:|
| cases | **17,459** — CQT's own count, `.sql` files under the scenario |
| OK / NOK | **17,459 / 0** |
| wall | **1,771 s** (29.5 min) |
| in the cases | 1,693 s |
| setup | ~78 s |

**Every case passes at develop.** The mismatched first runs failed 39 (§6), and none of those was
the runner's.

**A sql case is short.** Median 41 ms, mean 97 ms, p90 146 ms, p99 917 ms:

| duration | cases | share of cases | share of time |
|---|---:|---:|---:|
| < 50 ms | 10,527 | 60.3% | 15.0% |
| 50–200 ms | 5,726 | 32.8% | 30.2% |
| 200 ms–1 s | 1,046 | 6.0% | 23.0% |
| 1–10 s | 146 | 0.8% | 17.4% |
| > 10 s | 14 | 0.1% | 14.4% |

The longest is `_01_object/_09_partition/_001_create/cases/bug_xdbms294.sql` at 51.9 s. Of the
twelve longest, five create or reorganise partitions, four are i18n checks (a large Turkish table,
two charset conversions, a Japanese collation) and three test `ON UPDATE`. By family,
`_23_apricot_qa` is 16.5% of the time, `_01_object` 15.4% and `_13_issues` 13.5%.

Two things follow for the design.

- **The executor has to be a process that lives for the run.** A JVM started per case would add
  0.3–0.5 s to a case whose median is 40 ms — two hours or more over the corpus. CQT itself is one
  JVM and one connection for the whole run, and ADR-016's `Open`/`Run`/`Close` keeps that shape.
- **Parallelism is worth minutes here, not hours.** With eight slots the floor is the larger of the
  longest case (52 s) and the work divided by the slots (212 s), plus the one setup. That is 29
  minutes down to about five. It is worth having for a development loop, but it is a different
  proposition from shell's seventeen hours (`concept/beyond-axis.md` B-T14).

What a second run said about the first is in §6.

## 5. What the answers are made of

`sql/census.py` beside this file counts base answer files — not lines — that contain each class of
rendering.
The patterns are regular expressions and approximate: a count is a lower bound on the class, not an
exact figure.

| class | sql (17,475) | medium (987) |
|---|---:|---:|
| `java.sql.Timestamp` fraction (`…:ss.0`) | 3,018 · 17.3% | 24 |
| an error line (`Error:-N`) | 9,128 · 52.2% | 289 |
| of which raised by the driver (`-21xxx`, `-10000`) | 104 · 0.6% | 0 |
| query plan | 966 · 5.5% | 0 |
| float, plain with six or more decimals | 882 · 5.0% | 16 |
| float in `E` notation | 339 · 1.9% | 20 |
| timezone-typed value | 713 · 4.1% | 0 |
| `BigDecimal` in `E` notation | 48 · 0.3% | 4 |

**2,796 sql answers (16.0%) fall into at least one class that is hard to reproduce outside Java** —
the floats, the timezone types, the plans and the driver's own errors. For medium it is 34 (3.4%).
Timestamps are not in that set: their rule is simple. `census.py` prints the figure as
`(hard, any)`, and it is the number ADR-016 cites.

Variants beside the base answers: `.answer_cci` 2,121 (read only by `ccqt`),
`_D_<db charset>_C_<collation>` 237 (chosen by `run_mode`, §7), `_win` 57, `_ci` 6, `_csql` 1.

## 6. The runner against itself

Each suite ran twice at develop, one after the other, over the same copies. `sql/baseline.sh`
hashes every `.result` after each sql run, and `sql/selfcheck.py` compares two runs:

| | run 1 | run 2 | verdicts that moved | `.result` files that differ |
|---|---|---|---:|---:|
| medium (`medium_dev.conf`) | 975 OK, 90 s | 975 OK, 88 s | **0** | — |
| sql | 17,459 OK, 1,771 s | 17,459 OK, 1,772 s | **0** | **0** of 17,459 |

The case order was identical, and the time in the cases moved by 0.5% (1,693 s against 1,685 s).
**At develop, CTP agrees with itself completely** — every verdict and every byte of every case's
output.

### What the first, mismatched runs showed

Three sql runs came before the sandbox was at develop (§1). They are worth keeping for what they show
that the develop runs happened not to.

**The floor is not zero in principle.** One case moved once in three runs:
`_27_banana_qa/issue_5765_timezone_support/_01_table_operations/_04_select/cases/_09_view_dt.sql`. Line 37
is `select unique newt.col1, newt.col2 from (…) newt order by 1`, and `col1` holds the same instant
written in different zones: 15:30 JST (+9) and 08:30 EET (+2) are both 06:30 UTC. Rows that tie on
the sort key come back in whatever order the engine chooses, and the rendered zone names follow
them. It was also the one `.result` that differed between the second and third runs. Any difference
between two runners on that case is not evidence of anything.

**The server writes into the run's standard output.** In the third run, `*** XASL generation failed
***` landed between `Row_Value_Constructor.sql`'s progress line and its `[OK]`, so the verdict
moved to the next line. It appeared in none of the other runs. The server a run starts inherits the
run's standard output, so anything that reads verdicts from it has to read across line breaks
(`selfcheck.py` does, and ADR-017 requires it).

**Thirty-eight cases failed every time** on the mismatched set, the same way, and pass at develop.
They show what a mismatch between the engine and the answers looks like. By what their diff
contains:

| | cases | what changed |
|---|---:|---|
| plan or rewritten query | 24 | derived-table columns renamed (`d?.col` → `d?.a_?`), index skip scan planned as `sscan` (four cases in `_19_apricot/_03_index_skip_scan`), `nl-join` → `idx-join`, parallel gather `row by row` → `mergeable list`, join terms in another order |
| values | 8 | seven `median` and `percentile_cont` cases — in the two with the shortest diff the answer has `1` and the engine returns `1.0` — and one system-catalog count (`INFORMATION_SCHEMA` 288 → 300) |
| line order or whitespace only | 4 | the same lines in a different order — a plan's class order, grant rows |
| error code | 1 | `bug_bts_6460`: `-458` → `-731` |
| timezone value | 1 | `_09_view_ts.sql` — the sibling of the case above, and the same shape |

None of them was the sandbox or the runner. Every class is a difference between what the engine
returned and what that branch's answers expected, and each failed the same way every time. That is
what the three-repository check in §1 exists to rule out.

## 7. Read from the source while doing this

The analysis left the part of CQT that executes a case and renders its output unread
(`analysis/sql/cqt-deep-dive.md`, "line 1056+"). These are the facts the design depends on, each
checked against the source at `CTP/sql/src/com/navercorp/cubridqa/cqt/` (`cubrid-testtools` develop `a1bec87`):

| | |
|---|---|
| one connection for the run | `CubridConnManager.getDbmsConnection` (`:132`) caches it; nothing closes it. Before each case the reset scripts and `autocommit` are applied (`ConsoleBO.java:812-848`), and `SELECT 1;` checks the server (`:935`) |
| cases in order | one loop, `executeSqlFile` per case (`ConsoleBO.java:482`, `:983`) |
| rendering | column names each followed by four spaces, then each row's values each followed by five, one line per row (`ConsoleDAO.java:873-934`). A value is `getColumnValue(…, rs.getObject(i), …)` — `toString()` for all but BIT, OID and collections (`:957-1019`) |
| comparison | every `\r` and `\n` removed from both result and answer, then `String.equals` (`ConsoleBO.java:633-654`) |
| answer choice | `answers/<name>.answer`; if the config names a `run_mode`, then `…answer_<run_mode>`, then `…answer_<run_mode_secondary>`, then the base (`ConsoleBO.java:368-396`). `run_mode` is the `<run_mode>` element of the `jdbc_config_file` XML; `test_default.xml` has it commented out |
| no answer | the case is not run (`shouldRun=false`, `ConsoleBO.java:373-381`) |
| discovery | files ending in `.sql`, and nothing else (`TestUtil.java:112`, `:342`). **medium's ten `.api` files are not cases** |
| exclusions | each line of the filter file is matched against the case's path relative to the scenario with `containPath` (`TestUtil.java:371-391`). **A filter file that does not exist is a stack trace and no exclusions** (`:350-363`) — and `${CTP_HOME}/conf/exclusions.txt`, which every conf names, does not exist |
| sql charset | 11.5 and later force `en_US.utf8` for `sql` and `sql_by_cci` (`run.sh:688-694`, CUBRIDQA-1287) |
| CCI mode | renders in C with `isdouble`, `formatdatetime`, `trimnumeric`, `trimdouble` and `trimfloat` (`sql_by_cci/execute.c:266-510`), appends `.0` to anything that looks like a time (`:1535`, `:1648`), and compares **byte for byte** (`cmp_result_files`, `:2548`) — a different comparison rule from the JDBC path's |

## 8. Where the specification was wrong

Each of these goes into `spec-corrections.md`.

- **There are two files called `summary_info`, and the freeze has one of them.** Its `=` form
  (`external-surface-freeze.md` §5-3) is `run.sh`'s, written by `generate_summary_info` in CCI mode
  only (`run.sh:804-850`). A JDBC-mode run instead has CQT's, in `key:value` form, at the root and in
  every directory: `total:975`, `success:975`, `fail:0`, `totalTime:23565ms`, `SiteRunTimes:1times`.
- **The freeze's `main.info` is not `main.info`'s key set.** §5-2 lists `total`, `success`, `fail`,
  `totalTime`, `SiteRunTimes`, `cubrid_rel`, `user`, `machine`. The real file is CQT's twelve keys
  (`build`, `version`, `os`, `category`, `elapse_time`, `success`, `fail`, `total`, `execute_case`,
  `totalTime`, `end_time`, `result_path`) followed by the three `do_summary_and_clean` appends
  (`cubrid_rel`, `user`, `machine` — `run.sh:853-855`). `SiteRunTimes` belongs to CQT's
  `summary_info`.
- **`.api` is not a case extension.** `module-medium.md` §3 carried its meaning as open. Discovery
  takes only `.sql`.
- **A missing exclusion file is not an error to CQT.** That matters because shell, as of 2026-09-11,
  treats it as one, and the sql runner must not.
- **`run_mode` comes from the `jdbc_config_file` XML.** This closes freeze §11-7.
- **medium needs `create_table_reuseoid=no` on 11.x**, and `medium.conf` does not have it.

## 9. The `jdbc` executor, tried

ADR-016 says the `jdbc` executor renders identically by construction, because it runs CQT's own
code. That was a claim about a program that did not exist yet, so a spike wrote it and tested it
against the baseline (`sql/spike/`).

**What it is.** `TestkitExecutor.java`, about 100 lines. It builds the `Test` the way
`ConsoleAgent.runTest` does, then calls `ConsoleBO`'s own setup through reflection — `init`, the
DAO, `buildTest`, `checkDb` — and stops before the loop. From then on it reads case paths on
standard input and, for each, calls `executeSqlFile` and hands back `caseResult.getResult()` as
UTF-8. That is exactly what `saveTempResults` writes to the case's `.result`
(`ConsoleBO.java:588-596`; `FileUtil.writeToFile` defaults to UTF-8). CQT's own messages go to
standard error, and the protocol has standard output to itself.

**How it was run.** CTP's `run.sh` does all the setup — configure, `createdb`, locales, stored
procedures or the `mdb` load. In a copy of it, the one line that starts CQT is replaced by a driver
that starts the executor and feeds it cases in CQT's order. Every `.result` was deleted first, so a
case the executor failed to write could not pass on CTP's file.

| | cases | written | identical to CTP, byte for byte |
|---|---:|---:|---:|
| medium, whole | 975 | 975 | **975** |
| sql, one case directory in ten — 117 directories, 28 of 38 families | 1,658 | 1,658 | **1,658** |

Over the sql sample the executor took 157 s and 183 s in two spikes. CQT's own time for the same
1,658 cases was 180 s and 177 s in the two baseline runs, so the executor is no slower than CQT. It
also discovered the whole corpus exactly as CQT does (`READY 17459`) before running only the sample.
The committed `spike.sh` reproduced both rows.

**What the spike found that the design did not know:**

- **The JVM does not exit when `main` returns.** CQT leaves non-daemon threads behind, so the
  executor has to call `System.exit`. The first medium spike wrote all 975 files and then hung.
- **`run.sh` finds `CTP_HOME` from its own path** (`CTP_HOME=$(cd $(dirname $(readlink -f $0))/../..`,
  line 26), so a modified copy has to sit in CTP's `sql/bin`. A Go runner that reproduces the stages
  sets `CTP_HOME` itself and is not affected.
- **CQT runs with `sql/lib` as its working directory** (`run.sh` `cd $CPLIB` before starting it).
  The executor keeps that, because CQT reads `local.properties` and writes `ConsoleBO.log` relative
  to it.
- **`run.sh`'s last stage depends on CQT's result tree.** `do_summary_and_clean` finds the result
  directory by grepping the log for `Result Root Dir`. With the executor in CQT's place there is no
  such line, so the stage prints `No Results!!` and exits 1. The spike ignores that status. A Go
  runner writes the result tree itself.

**What it does not show.** That the whole sql corpus renders identically is P1's gate (ADR-017).
The spike sampled directories, not cases, so that a case leaning on its directory's earlier cases
was not mistaken for a rendering difference. The executor also had CQT's order and one connection,
as CQT does. Slots change both, and that is measured separately.

## 10. Reproducing it

```sh
# an engine built from upstream develop (§1), e.g. in a worktree of cubrid/cubrid:
#   cmake --preset release -DWITH_CMSERVER=OFF && CUBRID=$PWD/install.out build_cubrid.sh release
docs/evidence/sql/sandbox.sh refresh <that install.out>   # lays the sandbox down, then checks it
docs/evidence/sql/baseline.sh                              # checks, runs everything, checks again
python3 docs/evidence/sql/census.py $SANDBOX/cubrid-testcases                        # §5
python3 docs/evidence/sql/selfcheck.py $SANDBOX/sql-1.out $SANDBOX/sql-2.out <root-1> <root-2>   # §6
JAVA_HOME=/usr/lib/jvm/java-8-openjdk-amd64 docs/evidence/sql/spike/spike.sh              # §9
```

`SANDBOX` defaults to `/data/cub_sys/projects/regr-sql`, and the repository paths to this
machine's. `sandbox.sh` points CTP's confs at the copies, because their defaults name
`${HOME}/cubrid-testcases`, which is empty on this machine.
