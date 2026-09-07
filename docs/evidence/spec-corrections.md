# Corrections to the freeze specification

- **Started:** 2026-09-02
- **What this is:** every place `concept/external-surface-freeze.md` turned out to be wrong or
  incomplete, what was true instead, and how it was caught.

The individual corrections are recorded inline where the mistake was, dated. This page collects
them so the pattern is visible, because the pattern is the useful part: **each way of checking
finds a different class of error, and none of them finds the others.**

---

## 1. Found by adversarial review

A separate reviewer read the Phase 1 documents against the Phase 0 analysis.

| Spec said | Actually | Source |
|---|---|---|
| isolation cases live under `cases/` with a sibling `answers/` | `.ctl` and `.answer` sit **in the same directory**; the `cases/` rule is shell's `Test.java` only | `case-formats.md:22,152` |
| `.ctl` has 4 DSL tokens | **8** — `sleep`, `pause for deadlock resolution`, `wait until … unblocked`, `… finished` were missing | `ctl-grammar.md` |
| — | `runone.sh` normalises results through a **10+ line sed chain** before comparing. Absent from the spec entirely, though every isolation verdict passes through it | `ctl-grammar.md` |
| `<name>.<DB>_<CHARSET>` | `<name>.answer_<DB>_<C>` — the `answer_` prefix had been dropped | `case-formats.md:93` |
| — | `.diff_1` (273 files) and `.answer_WIN` (155) missing from the variant list | `case-formats.md` |

**What this method catches:** claims that contradict evidence already written down. It cannot catch
anything the analysis never recorded.

## 2. Found by investigating the source tree

Chasing the spec's own open questions.

| Spec said | Actually | How |
|---|---|---|
| the CLI is `ctp.sh` → `CTP.java` | **15 entry points**, of which the Phase 0 map covered one. `jdbc/bin/run.sh` reaches a *shell module* class; `init_path/run_shell.sh` is a second CLI with 13 options, documented in three user guides | `grep` for every script that starts a JVM |
| `conf/shell_agent.conf` is a runtime config, graded F3 | **not shipped.** `Server.java` reads it by relative path, and it is in none of the 13 files | listing `conf/` |
| RMI worker mode: retention undecided | **unreachable with what ships** — the key defaults to `ssh`, appears in no config, the agent config is absent, and nothing launches its server | reading `Context.java:163` |
| `found core file` had no known consumer | **two test cases grep for it**, in a frozen repository | `grep` across the testcases repos |
| `CORE_FILE:` is part of the shell module's stdout | it belongs to **sql/medium**. `common/ext/run_sql.sh` writes and greps it; the whole shell source contains no such string | `grep -rn CORE_FILE shell/` |

**What this method catches:** things the analysis stopped short of. It needs a specific question to
chase; it does not volunteer.

## 3. Found by writing the code

Every literal was taken from CTP's source rather than from the notes. Nineteen disagreed.

| Spec said | Actually | Source |
|---|---|---|
| `[NOK], retry: <N>` | `[NOK], TRY-><N>` | `Constants.RETRY_FLAG` |
| the retry suffix appears when a retry happened | when retries are **configured** — `maxRetryCount != 0`. A first-attempt failure prints `, TRY->0` as soon as `testcase_retry_num` is set | `shell/main/Test.java:189` |
| results go to `result/<task>/<timestamp>/` | `result/<category>/current_runtime_logs` — **no timestamp**. ADR-013 had inherited the error and listed a timestamp among the values to mask | `Context.java:172` |
| `main.info` is part of the shell layout | it belongs to sql/cqt; the shell family never writes one | `TestUtil.TOTAL_SUMMARY_FILE` |
| the unittest plug-in returns values via an `EEOOKK` marker | **three** markers: `GPROPSTART` ends the plug-in's output, values sit between `G_PROPERTY_<K>=` and `EEOOKK` | `GeneralLocalTest.invoke` |
| — | **Remote output is delimited by a frame.** `echo ALL_${NOTEXIST}STARTED` … `echo ALL_${NOTEXIST}COMPLETED`, and only what lies between `ALL_STARTED` and `ALL_COMPLETED` is kept. The unset variable is the mechanism: the script's own text never matches the marker, so a shell echoing its input cannot open the frame early | `ScriptInput`, `SSHConnect` |
| a shell case is any `<…>/cases/*.sh` | a case is `<name>/cases/<name>.sh` — **the script must be named after the directory two levels up**. The rule is an awk predicate, `$(NF-2)".sh" == $NF`, and the `cases/` segment is never checked at all | `Dispatch.getAllTestCaseScripts` |
| the per-case timeout cancels the case | it **kills the processes the case is waiting on**, from a second connection, and lets the case's own command return. Cancelling would close the channel and leave a `cub_server` holding the port for the next case. This is why the monitor has an SSH session of its own | `TestMonitor.resolveTimeout` |
| — | **A role's parameters go to a section that is not named after it.** `broker1` is written to `%query_editor` and `broker2` to `%BROKER1`, because the roles are numbered by position and the sections are named in the shipped `cubrid_broker.conf` | `DeployOneNode.updateCUBRIDConfigurations` |
| — | **The disk-space check takes two mail addresses.** `check_disk_space <fs> <size> "<to>" "<cc>"` runs before every case and notifies when space is short -- axis O reaching into the per-case loop. The check stays, the notification does not | `Test.checkDiskSpace` |
| — | **Cases see `TEST_BUIILD_ID`**, with three i's. Nothing in the corpus reads it | `Test.addSshInfoScript` |
| — | `getExportsOfMEKYParams` exports variables beginning **`MKEY`**. The method name has the two letters the other way round, and the name is what the analysis had copied | `CommonUtils` |
| — | **The engine is not configured at all when only broker-wide parameters are set.** The emptiness test covers five roles and omits `brokercommon`, which is where `MASTER_SHM_ID` lives — so two installs on one machine end up sharing a shared-memory segment and interfering rather than failing | `DeployOneNode.updateCUBRIDConfigurations` |
| — | **Excluding the case update excludes the case tree.** `TestCaseGithub.update` does two things: a git pull, and — when `testcase_workspace_dir` differs from `scenario` — a wipe-and-copy of the case tree into the workspace. The dispatcher searches the *workspace*. Drop the whole method as axis O and the run finds no cases at all | `TestCaseGithub.update`, `Dispatch.findAllTestCase` |
| `JAVA_HOME_<bits>` | **`JAVA_HOME_64BITS`.** The value is `getBuildBits()`, which returns the string "64bits", upper-cased — not a number | `Test.runTestCase_linux` |
| — | **A build id can run past the version.** With no `-` after the four-part number the scan takes everything up to the next `)`, so a `cubrid_rel` line without a commit suffix yields `11.2.0.0000) (64bit release build for linux_gnu` as the build id. Deterministic, so it still identifies a build | `CommonUtils.getBuildId` |
| — | **The retry flag has two spellings.** The console prints `[NOK], TRY->2`; `feedback.log` prints `[NOK]: TRY-> = 2`, using the same literal as a label rather than a prefix | `FeedbackFile.onTestCaseStopEvent` |
| a configuration with no `env.instanceN` keys is an error — `Not found any environment instance to test on it!` | **it is a local run.** The `Context` constructor adds an environment called `local` when the list comes back empty, so `Main.exec`'s check for an empty list can never fire and that error message is unreachable. The specification recorded the dead branch and not the behaviour. Running against the engine on the machine you are sitting at is how the suite is driven during development, and it is the only way to exercise it without a second host | `Context.java:125-130`, `Main.exec` |
| — | **Every script CTP sends has a prologue.** Outside the frame it sources `~/.bash_profile`, preserving an explicitly-set `CTP_HOME` across it; inside the frame it resolves `CTP_HOME`, sets `ulimit -c unlimited`, puts `$CTP_HOME/bin` and `common/script` on `PATH`, `cd`s home and exports `init_path`. An ssh exec session is neither a login nor interactive, so without it a case fails on its own first line — `. $init_path/init.sh` | `ScriptInput.getCommands`, `GeneralScriptInput`, `ShellScriptInput` |
| — | **CTP discards standard error.** `SSHConnect` reads `exec.getInputStream()` and nothing else; the local path concatenates stdout and stderr and then truncates at the completion marker, which sits at the end of stdout. That is why a case is run as `sh <case>.sh 2>&1` and the reset script is not | `SSHConnect.execute`, `LocalInvoker.execCommands` |
| — | **`[INFO] CLEAN PROCESSES: ` and `[INFO] Reset CUBRID: `** are the worker log's literals for the two steps before each case | `Test.resetProcess`, `Test.resetCUBRID_linux` |
| — | **Java prints `null` for an absent value.** `start MSG Id is null` when the scheduler set no `MSG_ID`; `Hostname: null` in the XML when `$HOSTNAME` is unset. Both are what these lines have always said | `FeedbackFile` |
| the worker collects `<name>.result` and diffs it against `<name>.answer` | **there is no diff.** The case writes its own verdict into `<name>.result`; the worker `cat`s it and fails the case if any line contains the substring `NOK`. The entire shell source contains no reference to `answer` | `Test.collectGeneralResult`, `grep -rn answer shell/src` |

**What this method catches:** anything where the spec paraphrased instead of quoting. Writing a
literal into a program forces you to know it exactly; prose lets you almost know it.

## 4. Found by running it

Two kinds: running the new runner, and running it *beside* CTP on the same cases. The second is what
`regression-shell.md` is, and it found the last four rows here on its own.

| | |
|---|---|
| **The dispatcher prints a banner around every task** — a rule, `TEST STARTED`, `TEST END`, `ELAPSE TIME`. The spec had no dispatcher output at all. The opening rule is printed *before* the task name is resolved, so an unknown task gets a banner and then help | `CTP.java:132-143,192-196` |
| **unittest's output resembles nothing else** — step headings with a trailing space, a one-based index, `[SUCC]`/`[FAIL]` rather than `[OK]`/`[NOK]`, verdict on the same line as the name. The spec had described unittest's *plug-in contract* and never its output | `GeneralLocalTest.start` |
| **Two console lines come from a feedback backend.** `Test Category:` and `The Number of Test Cases:` are printed by `FeedbackFile`, to its own file *and* to stdout. Nobody looking for console output would look there | `FeedbackFile.java:134-137` |
| **`dispatch_tc_ALL.txt` is not reproducible.** It records the case list in `find` order, and `find` order is `readdir` order: three consecutive runs over the same unchanged tree gave three different lists, on two separate quiet trees. CTP cannot reproduce its own dispatch order, so this file was never an F1 surface — and neither is which environment ran which case | `find … \| cmp`, run three times |

| **Four more result files.** `feedback.log`, `test_status.data`, `current_task_id` and `test-<category>.xml` are written by the feedback backend, which is the default. The last is JUnit XML with a GitHub URL in the `classname` attribute — the one output here that something other than a person reads | `FeedbackFile` |
| **`test_status.data` is not reproducible.** `Properties.store` stamps the current time into a comment and emits keys in hash order, so the counters file differed on every run for reasons that had nothing to do with the run | `CommonUtils.writeProperties` |
| **A resumed run's XML report is broken.** `writeTestSuiteStart` is reached only from `setTotalTestCase`, which returns early in continue mode — so no `<testsuite>` is opened, cases become children of `<testsuites>`, and the close writes one end-element too many, throws, and skips its own flush | `FeedbackFile.finalizeXmlWriter` |

| **`check_<envId>.log`** — a twenty-one line requirements check (variables, commands, directories) written to the file *and* to standard output before any case runs. The specification had neither the file nor the console lines | `CheckRequirement` |
| **`monitor_<envId>.log` is created whether or not anything is written to it**, so every result directory has one, usually empty | `TestMonitor`, `Log` |

| **A case under CTP inherits the JDK's rewrite of `LD_LIBRARY_PATH`** *(2026-09-06)*. When a JVM library directory is on the variable but not at its head, the JDK 8 launcher rebuilds it as `<its own three directories>:<the original>` and re-execs; `Runtime.exec` then hands that to every case. CTP does not do this and cannot stop it — its entry point is `java`. The spec described the environment a case is entitled to assume and had no row for this | `regression-shell.md` §3-5, reproduced with `jrunscript` |

**What this method catches:** whole surfaces nobody thought to write down. Reading more carefully
would not have found these, because the question "what else does it print?" has no place to be
asked until something prints. The last row is a variant worth naming: not a surface CTP prints, but
one it *passes on* — visible only because the replacement, not being a Java program, does not.

## 5. Found by running it on a machine CTP does not support

The first old-versus-new comparison printed, from CTP:

```
[TESTCASE-1] /tmp/.localexec….sh: 1: source: not found [FAIL]
[TESTCASE-2] /tmp/.localexec….sh: 1: list: not found [FAIL]
```

`/bin/sh` here is dash. Sourcing the plug-in fails, and the two error lines from the failed `list`
become two test case names. CTP's own shipped plug-in declares `function init { … }`, which dash
also rejects, so the unittest path has always required a bash-compatible `/bin/sh` and nothing said
so.

**What this method catches:** assumptions the original system never had to state because its
environment always satisfied them.

---

## One wrong rule, five wrong numbers

The discovery rule above is worth following through, because it shows how far a single wrong
sentence travels once other documents start counting with it.

"A case is any `*.sh` under `cases/`" was used to size every corpus in ADR-013. Every one of those
numbers was too large:

| | Spec said | Actually | Source |
|---|---|---|---|
| shell corpus | 3,722 | **3,452** | `cubrid-testcases-private-ex/shell` |
| `_01_utility` smoke set | 234 | **217** | same |
| `_25_unstable` | 204 | **195** | same |
| HA tree | 162 | **367** | `cubrid-testcases-private/HA/shell` |
| `manually` | 12 | **2** | `cubrid-testcases-private/manually` |

The 270 extra files are not tests. They are helpers the cases source or call: `PrintInfo.sh` (38
copies), `ModifySysConf.sh` (11), `diagdb_parse.sh` (11), `build.sh`, `common.sh`, `sql.sh`. Running
them as cases would not merely have inflated a count — it would have run 270 scripts standalone that
were written to run inside another one.

The naming rule has a reason, and finding it explains the whole thing. `do_check_more_errors` in
`init_path/shell_utils.sh` derives the result file from the **directory** name:

```sh
result_file_full_name=${test_case_dir%/cases*}/cases/${case_name}.result
```

while the worker derives it from the **script** name. The two agree only when the script is named
after its directory. A case that broke the convention would have its verdict read from a file
nobody wrote.

Two things fell out of chasing it:

- **20 directories contain `cases/*.sh` and no case at all** — `_07_index_enhancement/_01_deadlock_default_isolation`
  and its `tc_ds_*.sh` among them. They look like tests, they are in the repository, and CTP has
  never run them. That is a finding for the test owners, not for the port.
- **The rule now lives in one function**, `shellsuite.IsCase`, with a test that walks the real
  corpus and asserts it selects exactly what CTP's awk selects — 3,452, agreeing case for case.

**What this method catches:** nothing on its own. It is what happens when a corrected rule is
carried back through everything that had quietly depended on it. The cost of *not* doing it is that
the numbers keep looking authoritative.

## 6. Found in code that has never run

Two of these were found the way the others were, but they belong together, because
what they have in common is that nothing depended on them and so nothing complained.

| | |
|---|---|
| **The kill script's JVM sweep has never killed a JVM.** `if [ $isExistPid -eq 0]` has no space before the bracket, so the test is a shell syntax error, the branch is never taken, and the list it builds is always empty. Every CTP run prints two `[: missing ']'` lines into the worker log and moves on | `Constants.createLinKillScripts` |
| **`TEST_BUIILD_ID` has no readers.** Every case runs with it exported, and `grep` across both case repositories finds no use of either spelling | `Test.addSshInfoScript` |

Both are kept exactly as they are. Repairing the first would start killing JVMs on
machines where nothing has killed one in years -- a behaviour change disguised as a
typo fix -- and removing the second changes what cases can see for no gain. They are
recorded so that the next person to notice them does not have to work out, again,
whether they matter.

**What this method catches:** nothing anyone was looking for. It is the residue of
porting line by line: code that runs but does not work, and code that works but does
nothing, are both invisible until someone has to decide whether to carry them over.

## What this says about the freeze

A frozen surface is only as good as the reading behind it, and the reading was done six different
ways here with six different yields. The spec was not careless — it was written from a careful
analysis — and it was still wrong in thirty-three places, every one of them F1.

Three practical consequences:

1. **The regression evidence, not the specification, is the contract** (ADR-013). A document can be
   wrong quietly; a diff cannot.
2. **Corrections are recorded where the mistake was, and dated.** A spec that silently improves is
   indistinguishable from one that was always right, and the reader has no way to tell which parts
   have been tested against reality.
3. **A correction is not finished until everything downstream of it is re-derived.** One wrong
   sentence about case discovery produced five wrong corpus sizes in a second document, and those
   numbers looked exactly as trustworthy as the right ones would have.

## Where CTP does not agree with itself

Some of the thirty-three are not errors the spec could have avoided by reading harder. They are
places where CTP cannot reproduce its own behaviour, or where two parts of it contradict each other.
Each one needs a decision, and the decisions are not all the same:

| | Kept or changed |
|---|---|
| `find` order makes `dispatch_tc_ALL.txt` irreproducible across CTP's own runs | **changed** — sorted, so two runs can be compared at all |
| `lastIndexOf("cases")` disagrees with the discovery rule on names no case currently uses | **changed** — matches the path segment; proven identical on all 3,452 real cases |
| the emptiness test for engine configuration omits `brokercommon`, so a broker-only configuration — including `MASTER_SHM_ID` — is silently skipped and two instances share a shared-memory segment | **changed** — collisions do not fail, they interfere, and the results look real |
| `test_status.data` carries a timestamp and hash-ordered keys, so it differs between two identical runs | **changed** — sorted, no timestamp; still readable as Java Properties, which is what a resumed run needs |
| a resumed run's JUnit report is unbalanced XML, and the exception it throws skips the flush | **changed** — the suite opens on first use, so both modes produce a document that parses |
| the JVM sweep is a shell syntax error and kills nothing | **kept** — repairing it widens what the runner kills |
| a build id with no commit suffix runs on to the next `)` | **kept** — ugly, but deterministic, and it is what identifies the build in `main_snapshot.properties` and in every case's environment |
| `TEST_BUIILD_ID` is exported to every case and read by none | **kept** — removing it changes what cases can see, for nothing |
| the JDK 8 launcher reorders `LD_LIBRARY_PATH`, so a case under CTP sees three JVM directories a case under testkit will not *(2026-09-06)* | **kept** — the entries are the same set, the order differs, and there are no basename collisions between `$CUBRID/lib` and the injected directories. Simulating a launcher this runner does not have would be reproducing an accident, and it is one that disappears with the last JVM in the chain |

The rule that decides between the two columns is the config-key policy, applied to behaviour rather
than to keys: **change it when leaving it alone would make someone trust a wrong result; keep it
otherwise.** A nondeterministic dispatch file, a shared shared-memory segment and a report that
silently loses its tail all produce results that look real and are not. A sweep that has never
swept, a typo nobody reads and an ugly-but-stable build id produce nothing at all.

Where the original is nondeterministic, F1 is not a grade anything can earn, and pretending
otherwise would only move the problem into the evidence.
