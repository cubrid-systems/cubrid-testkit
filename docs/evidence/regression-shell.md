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

**Corrected 2026-09-07.** The paragraph above is wrong about what the namespace
achieves. `ps -u $USER` selects *nothing* inside `unshare --map-root-user` — the
processes are uid 0 while `$USER` is still the outer name — so the sweep is not
scoped to the run's own work, it is empty. The containment is real and both
runners were affected identically, so this comparison stands; what it did not do
is exercise the reset at all. Measured and written up in
[`smoke-217.md`](smoke-217.md) §3.

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
| `feedback.log` | 6 of 560 lines — see 3-2 |
| `test-shell.xml` | 11 of 539 lines — see 3-3 |
| `test_local.log` | 154 of 1,478 lines — see 3-4 |

**Recounted 2026-09-06.** A count here means `diff | grep -c '^[<>]'` over the two normalised
files. Against the same stored artifacts — `regr/baseline-ctp/` and `regr/new-testkit/`, untouched
since the run:

| | `feedback.log` | `test-shell.xml` | `test_local.log` |
|---|---:|---:|---:|
| the normaliser as it stood on 09-02 | 20 | 25 | 700 |
| with dates masked (§6) | 6 | 11 | 650 |
| with the §3-4 defect removed | 6 | 11 | **154** |

**The figures this page first carried — 8, 13 and 652 — reproduce under none of those.** The
convention was not written down, so what was counted cannot now be recovered; the row that matters
is the first, which is this page's own numbers measured again the way the page now says. The
artifacts did not change. Only the counting, the masking and one defect did.

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

### 3-2. `feedback.log` — 6 lines

Two blank lines, and six lines inside the failing case's traced output: `LD_LIBRARY_PATH` (see 3-5)
and a `rm -rf $CUBRID/log/*` whose glob expanded on one side and not the other because the directory
was empty. No line that reports a verdict differs.

### 3-3. `test-shell.xml` — 11 lines

CTP writes a passing case as `<testcase ...></testcase>`; this writes `<testcase .../>`. Same
document, different serialisation, which is the F2 grade this file was given when it was written —
CTP produced it through `IndentingXMLStreamWriter` and matching that library's whitespace exactly
would be reproducing an accident. The rest is 3-5.

### 3-4. `test_local.log` — 154 of 1,478 lines, and a defect of ours behind the first count

The worker log holds the full traced console output of every case plus four machine-state dumps per
reset: `ps -u $USER -f`, `ps -ef | grep cub`, `netstat -n -e -p -a` and `ipcs`. The socket table
alone accounted for over 1,500 lines of raw difference and is masked, because **two consecutive CTP
runs cannot reproduce it either** — it is a snapshot of everything happening on the box.

**What was written here first was wrong.** It said the `csql1` case got further in the testkit run
than in the CTP run — more SQL executed, more `sed` normalisation, `dos2unix: command not found`
sixteen times rather than four — and blamed 3-5. The case did not get further. **Its console output
was written to the worker log twice**, and the second copy is what the extra `dos2unix` lines are.

CTP keeps two buffers: `resultItemList`, which `workerLog` receives line by line, and `resultCont`,
which is that list plus — for a case that failed — the `CONSOLE OUTPUT` banner and the console
itself (`Test.java:166` against `:176`). Only `resultCont` goes to feedback. This runner appended
the banner and the console to the verdict's items instead, and `finish` logs every item — so the
console reached `test_local.log` a second time, after `runOne` had already written it there as the
case produced it.

Fixed on 2026-09-06: the console is passed to `finish` separately and joined into the feedback text
only. `feedback.log` and `test-shell.xml` were already right — the banner appears exactly once in
each on both sides — so nothing frozen changed. `TestAWorkerRunsCasesAndRecordsTheirVerdicts` now
counts the console in the worker log and requires one, and fails if the defect is put back.

**Removing the duplicated block from the recorded log takes the count from 650 to 154**, and the
csql1 section from 544 normalised lines against CTP's 542 — that is, the case behaved the same on
both runners, line for line. That number is a simulation on the stored artifact, not a fresh run;
the next run measures it for real.

What the 154 are:

| | lines | |
|---|---:|---|
| CTP's `deploy_ctp` and `installBuild` output | 106 | `BEGIN TO UPGRADE CTP`, `SKIP CTP UPDATE FOR LOCAL TEST!`, `upgrade.sh`'s dump of the environment. Both steps are axis-O exclusions and this runner does not have them (`deploy.go`) |
| `ps` and reset noise | 20 | each run's own pids |
| `LD_LIBRARY_PATH` | 9 | 3-5; the harness change removes it on the next run |
| blank lines | 8 | trailing the above |
| `rm -rf $CUBRID/log/*` | 2 | 3-2 |
| `csql_help`'s console shape | 10 | **open** — see below |

**Corrected 2026-09-07.** The last row said "`diff` alignment — lines whose
bytes are identical, paired inside a changed hunk", and that was wrong. The
bytes are not identical:

```
CTP      \tCUBRID SQL Interpreter
testkit  \t\tCUBRID SQL Interpreter\r
```

An extra leading tab and a carriage return, on every line of the `_38_csql/
csql_help` case's `csql` session. It was written off because `diff` reports
such a pair the same way it reports an alignment artifact, and the difference
was not looked at. Replacing `diff` with `comm` — the two files are sorted, so
`comm` is the exact multiset difference and has no alignment step to blame —
made the ten lines the only unexplained thing left, which is what they had been
all along. The two runners do give a case a different standard input -- a pipe
against `/dev/null` -- but that is measurably not the cause, and
`evidence/compare/README.md` records what eliminating it left.

Everything else is accounted for.

### 3-5. `LD_LIBRARY_PATH` differs, and the JDK 8 launcher is why

**What was written here first was wrong twice.** It said CTP's run did not have `$CUBRID/lib` on
`LD_LIBRARY_PATH`. Both runs had it. And it said `csql` therefore resolved a different library:
there are **zero basename collisions** between `$CUBRID/lib` and the directories in question, and
`ldd $CUBRID/bin/csql` names `libc` and the loader and nothing else.

What the two runs actually gave a case:

```
CTP     …/commonforc/lib : JVM/jre/lib/amd64/server : JVM/jre/lib/amd64 :
        JVM/jre/../lib/amd64 : <ROOT>/CUBRID/lib : JVM/jre/lib/amd64/server
testkit …/commonforc/lib : <ROOT>/CUBRID/lib : JVM/jre/lib/amd64/server
```

Same entries, three JVM directories in front on the CTP side. **The JDK 8 launcher put them
there.** When a JVM library directory is on `LD_LIBRARY_PATH` but not at the head, the launcher
rebuilds the variable as `<its own three directories>:<the original>` and re-execs itself;
everything CTP spawns inherits the rewritten value, and `Runtime.exec` passes the environment
straight through. testkit is a Go binary and has no launcher.

The trigger is conditional, and this is the measurement:

| `LD_LIBRARY_PATH` | |
|---|---|
| `$CUBRID/lib` alone · a JVM directory alone · two non-JVM entries | unchanged |
| **`$CUBRID/lib:$JVM/server`** — a JVM directory present, not first | **rewritten** |
| `$JVM/server:$CUBRID/lib` — a JVM directory first | unchanged |

```sh
LD_LIBRARY_PATH=$CUBRID/lib:$JAVA_HOME/jre/lib/amd64/server \
  $JAVA_HOME/bin/jrunscript -e 'print(java.lang.System.getenv("LD_LIBRARY_PATH"))'
```

which prints, byte for byte, the value in `baseline-ctp/test_local.log:56`.

**So it belonged to the harness, not to either runner.** `regr/env.sh` wrote
`LD_LIBRARY_PATH=$CUBRID/lib:$JAVA_HOME/jre/lib/amd64/server` — a JVM directory, second. Putting it
first makes the launcher leave the value alone and the two runners hand a case the same string;
verified by sourcing the fixed `env.sh` and comparing what the shell sees against what a JVM sees.

**What does not go away with it.** After the migration there is no JVM in the chain at all. On a
machine whose profile puts a JVM library directory somewhere in the middle of `LD_LIBRARY_PATH`, a
case under CTP saw the launcher's rewrite and a case under testkit will not — permanently, and for
a reason that was always an accident of CTP being a Java program. That is a difference the freeze
has to grade rather than a bug either side can fix; it is entered in `spec-corrections.md` with the
other three of its kind.

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

Running the two side by side found seven things that reading could not. All are recorded in
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
7. **A case under CTP inherits the JDK's rewrite of `LD_LIBRARY_PATH`.** Not because CTP does
   anything: because its entry point is `java`, and the launcher reorders the variable when a JVM
   library directory is on it but not first (3-5). Nothing in the source says so, on either side.

The comparison also found a defect in **this** runner, which is the point of running it: a failing
case's console output went into the worker log twice (3-4). It survived the unit tests because one
of them asserted the wrong thing — that the banner reaches the worker log, which is where CTP never
puts it. A test can encode a bug as easily as it can catch one, and only the side-by-side run could
tell which this was.

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
