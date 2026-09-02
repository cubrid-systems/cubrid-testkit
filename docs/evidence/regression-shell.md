# Regression evidence: shell

- **Date:** 2026-09-02
- **What this is:** the first side-by-side run of CTP and testkit on the same cases, on the same
  machine, in the same minute — and an honest account of how far it goes.
- **Method:** ADR-013, normalise then diff. The normaliser is `normalize.sh` in this directory.

> **This is not the Phase 3 exit evidence.** It is four cases, not 3,452, and it does not clear the
> `TESTKIT_NATIVE_SHELL` gate. What it does establish is that the two runners agree where it matters
> and that every place they disagree has a named reason.

---

## 1. What was run

| | |
|---|---|
| Cases | 4 real cases from `_01_utility`: `_23_optimizedb/itrack_10001`, `_38_csql/csql1`, `_38_csql/csql_help`, `_12_spacedb/itrack_10002` |
| Engine | CUBRID 11.3.5.1275-0e31336, a normal (non-ASan) build, its own install and `CUBRID_DATABASES` |
| Mode | local — the configuration names no `env.instanceN`, which is CTP's own local mode |
| CTP | `bash bin/ctp.sh shell -c shell.conf` |
| testkit | `TESTKIT_NATIVE_SHELL=1 testkit shell -c shell.conf` |
| Both inside | `unshare --map-root-user --mount --pid --ipc --fork --mount-proc`, with `/bin/bash` bind-mounted over `/bin/sh` |

### Why a namespace

Two reasons, and both are findings in their own right.

**The reset would have taken the machine down with it.** The process reset that runs before every
case matches on substrings across everything the user owns: `cub` takes any process whose name
contains it, and `sleep`, `expect` and `dos2unix` go too, followed by `ipcrm -m` on every shared
memory segment. On this machine that was 63 processes — three `cub_master`s, a `cub_server`, a dozen
`cub_cas`, a JVM — and 13 segments, none of them belonging to the test. A PID and IPC namespace
makes `ps -u $USER` and `ipcs` show the run its own work and nothing else. **CTP has the same
problem**: this is not a property of the port.

**`/bin/sh` here is dash, and the shell suite has never run on dash.** Without the bind mount CTP
fails every case with `init.sh: Syntax error: "(" unexpected`, and so does testkit. Making `/bin/sh`
bash reproduces the platform CTP supports rather than testing both runners against a shell neither
was written for.

## 2. Result

Both runners: **3 passed, 1 failed** — the same case (`csql1`) failing on both, the same summary
counters, exit code 0.

After normalisation, of the ten files a run leaves behind:

| File | |
|---|---|
| `dispatch_tc_ALL.txt` | **identical** |
| `dispatch_tc_FIN_local.txt` | **identical** |
| `test_status.data` | **identical** |
| `check_local.log` | **identical** (21 lines) |
| `current_task_id` | **identical** |
| `monitor_local.log` | **identical** (empty on both, and present on both) |
| `main_snapshot.properties` | identical once the JVM's own system properties are removed — see 3-1 |
| `feedback.log` | 8 of 560 lines — see 3-2 |
| `test-shell.xml` | 13 of 539 lines — see 3-3 |
| `test_local.log` | 652 of 1,478 lines — see 3-4 |

The two files that carry the verdicts are byte-identical after normalisation. So is the build
identity: both runs recorded `AUTO_TEST_VERSION=11.3.5.1275-0e31336` and `AUTO_TEST_BITS=64bits`,
parsed out of the same `cubrid_rel` line by two different implementations.

## 3. Every remaining difference, and why

### 3-1. `main_snapshot.properties` — the JVM's own properties

CTP's snapshot is the configuration merged with `System.getProperties()`, so it carries
`java.version`, `awt.toolkit`, `sun.arch.data.model` and fifty more. A Go program has none of these,
and inventing them would be theatre. **Remove the JVM properties and the two files are identical**,
including both `AUTO_TEST_*` keys.

**Grade: F2.** The configuration keys are frozen; the JVM's description of itself is not a fact about
the test run.

### 3-2. `feedback.log` — 8 lines

Two blank lines, and six lines inside the failing case's traced output: `LD_LIBRARY_PATH` (see 3-5)
and a `rm -rf $CUBRID/log/*` whose glob expanded on one side and not the other because the directory
was empty. No line that reports a verdict differs.

### 3-3. `test-shell.xml` — 13 lines

CTP writes a passing case as `<testcase ...></testcase>`; this writes `<testcase .../>`. Same
document, different serialisation, which is the F2 grade this file was given when it was written —
CTP produced it through `IndentingXMLStreamWriter` and matching that library's whitespace exactly
would be reproducing an accident. The rest is 3-5.

### 3-4. `test_local.log` — 652 of 1,478 lines

The worker log holds the full traced console output of every case plus four machine-state dumps per
reset: `ps -u $USER -f`, `ps -ef | grep cub`, `netstat -n -e -p -a` and `ipcs`. The socket table
alone accounted for over 1,500 lines of raw difference and is masked, because **two consecutive CTP
runs cannot reproduce it either** — it is a snapshot of everything happening on the box.

What is left is the `csql1` case getting further in the testkit run than in the CTP run: more SQL
executed, more `sed` normalisation, `dos2unix: command not found` sixteen times rather than four.
The verdict is the same on both sides. The cause is 3-5.

### 3-5. `LD_LIBRARY_PATH` differs, and it changes what a case does

CTP's run did not have `$CUBRID/lib` on `LD_LIBRARY_PATH`; this run did. Both wrappers export the
same environment, and `ctp.sh` does not touch the variable, so the difference comes from how each
runner's own process was started rather than from anything either one does.

It is not cosmetic: `csql` resolves a different library and the case takes a different path through
`init.sh`. **This is the one difference that is not yet explained to the bottom**, and it is the
first thing to chase before the corpus is widened. It is recorded here rather than normalised away.

## 4. What this run does not cover

| | |
|---|---|
| Scale | 4 cases. The exit evidence needs 3,452 (ADR-013) |
| Mode | local only. Nothing here exercised deploy over SSH, several environments, or the dispatcher spreading cases across them |
| Retry | `testcase_retry_num=0`. The two-pass retry ordering is covered by unit tests and by nothing here |
| Timeout | no case ran long enough to reach the monitor |
| HA | not attempted; `DeployHA` remains unverified as ADR-013 already records |
| Platform | one machine, with `/bin/sh` replaced. A QA machine where `/bin/sh` is already bash is the real target |

## 5. What it found

Running the two side by side found six things that reading could not. All are recorded in
`spec-corrections.md`; the reason they are listed again here is that every one of them was invisible
until two implementations were put next to each other.

1. **CTP discards standard error.** The remote channel never reads it; the local one concatenates it
   after stdout and then truncates at the completion marker, which sits at the end of stdout. This
   is why a case is run as `sh <case>.sh 2>&1` and the reset script is not.
2. **`check_<envId>.log`** — a whole requirements check, twenty-one lines, written to the file *and*
   to standard output. Not in the specification.
3. **`monitor_<envId>.log`** is created whether or not anything is written to it.
4. **`[INFO] CLEAN PROCESSES: ` and `[INFO] Reset CUBRID: `** are the worker log's literals.
5. **Java prints `null`.** `start MSG Id is null`, `Hostname: null` — absent values, concatenated.
6. **The prologue.** Every script CTP sends is preceded by a `~/.bash_profile` source and a block
   that resolves `CTP_HOME`, puts `$CTP_HOME/bin` and `common/script` on `PATH`, and exports
   `init_path`. Without it every case fails on its own first line, `. $init_path/init.sh`.

## 6. Reproducing it

```sh
# 1. a sandbox: an engine, a databases directory, a CTP tree, a few cases
# 2. an environment: CTP_HOME, init_path, CUBRID, CUBRID_DATABASES, JAVA_HOME, PATH
# 3. both runs, each inside:
unshare --map-root-user --mount --pid --ipc --fork --mount-proc \
  bash -c 'mount --bind /bin/bash /bin/sh; exec "$@"' -- <runner>
# 4. normalise and diff each result file
diff <(normalize.sh < ctp/<file>) <(normalize.sh < testkit/<file>)
```

The normaliser is `normalize.sh` beside this file. Changing it is a change to the freeze (ADR-013
Consequence 1).
