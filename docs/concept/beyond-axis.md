# Axis B — the register

- **Started:** 2026-09-03
- **What this is:** every place this system should be better than CTP rather than equal to it, what
  it improves on, and what would show it worked.
- **Rules:** ADR-015. Four admission criteria, all four required. An entry not written here is not
  on the axis.

Axis T and axis O are **rewrites that preserve compatibility** — done when the frozen surface holds
and the diff is zero. Axis B has nothing to be compatible with, so every entry carries its own
evidence.

**Nothing here ships before the T or O item it improves on has passed its own gate.** That is not
ceremony: equivalence cannot be proven against a system that has already been improved.

---

## Status key

| | |
|---|---|
| **shipped** | already done, and why it could not wait |
| **ready** | the parity it improves on has passed its gate; may be built |
| **blocked** | waiting on a named T or O item |
| **idea** | admitted, not yet sized |

---

## B-T — beyond, on the test-execution axis

### B-T1. Reproducible dispatch — **shipped**

| | |
|---|---|
| Improves on | T: `Dispatch.findAllTestCase` |
| Today was | `dispatch_tc_ALL.txt` records the case list in `find` order, which is `readdir` order. Three consecutive runs over the same unchanged tree gave three different lists |
| Now | discovery sorts |
| Evidence | two runs of the same suite produce the same file; `TestDiscoverIsIndependentOfFindOrder` |
| Why it did not wait | **there was no parity to ship first.** CTP cannot reproduce this file against itself, so F1 was not a grade anything could earn. Recorded as a deviation in `evidence/spec-corrections.md` and masked in ADR-013 |

### B-T2. A runner you can hand a container — **blocked**

| | |
|---|---|
| Blocked on | T: the shell task passing the full-corpus gate (ADR-013) |
| Improves on | T: `Constants.createLinKillScripts`, and the posture behind it |
| Today | the reset before every case matches substrings across everything the user owns. On the machine used for `evidence/regression-shell.md` that was 63 processes and 13 shared-memory segments belonging to other work. **A CTP run and anything else the same user is doing cannot share a machine** |
| Beyond | the runner runs inside a PID and IPC namespace it creates for itself, so the reset reaches its own work and nothing else |
| Evidence | the full suite runs on a developer's machine while that developer keeps working; the reset script is unchanged, and `ps -u $USER` inside the namespace shows only the run |
| Note | the wrapper already exists — `regression-shell.md` §1 had to build it before either runner could be measured. It is a script beside the evidence, not part of the runner. **This entry is about making it the runner's own behaviour**, which ADR-014 made possible by scoping the runner to one machine |
| Also in scope | **fixing the JVM sweep** (freeze §11-23). `[ $isExistPid -eq 0]` has no space before the bracket, so the test is a shell syntax error, the branch is never taken, and CTP has never killed a stray JVM through it. Fixing it alone would widen what the runner kills, which is the direction ADR-014 moved away from; inside a namespace "every JVM the user owns" *is* "every JVM this run started", and the sweep does what it was written to do |
| Why the coupling gets tighter | the sweep spares CTP's own JVM by matching `com.navercorp.cubridqa\|service.Server`. **When the migration finishes there is no CTP JVM to spare**, so that filter matches nothing and the fixed sweep becomes an unconditional "kill every JVM". The containment is not a nicety that could be added later -- it is what the fix depends on, and it depends on it more as the port progresses |

**Status correction (2026-09-03).** This entry was first written as *ready*, and that was wrong
under criterion 2 of ADR-015: the T item it improves on is the shell task, and the shell task has
not passed its gate. The rule is not ceremony -- improving the reset before the reset is proven
equivalent would leave any later difference with two possible causes. Corrected the day it was
written, which is the cheapest a correction ever gets.

**A runner at PID 1 has to reap, and this one does not (2026-09-07).** The comparison wrapper found
this before the feature did. `in-ns.sh` ended in `exec "$@"`, which makes whichever runner it starts
PID 1 of the new PID namespace -- and PID 1 must collect orphans. A shell does, so CTP, whose PID 1
was `bash ctp.sh`, collected what its cases left behind. A Go binary does not, so testkit did not:
`cub_master` never learned that `cub_server` had exited, and `cubrid server stop` polled through
`cub_commdb -S` indefinitely. On `_01_sqlx/bug_cubridsus2018` that is 21 s under CTP against over
twelve minutes under testkit, and 15 s under testkit once a shell is left at PID 1.

In the sandbox it is the wrapper's bug and it is fixed there. **For this entry it is the feature's
bug**, because a container is exactly the case where the runner *is* PID 1 with no shell above it
and no system init to fall back on. Handing testkit a container therefore means one of two things,
and the entry has to choose: the runner reaps orphans itself, or the image ships an init and the
runner documents that it requires one. Neither is written yet.

| Also in scope | reaping, on the terms above. The evidence that it matters is already measured: `evidence/smoke-217.md` |

### B-T3. Cases that run at the same time — **blocked**

| | |
|---|---|
| Blocked on | T: the shell task passing the full-corpus gate (ADR-013) |
| Improves on | T: `Test.runAll` — one case at a time, per machine |
| Today | a case restores the whole CUBRID install before it runs (`RestoreScript`), so two cases cannot share a machine. `_01_utility` is 217 cases at roughly 70 seconds each: **about four hours, serially**, and the full corpus is 3,452 |
| Beyond | each case gets its own instance — own port, own shared-memory id, own data directory — so N run at once on one machine. The per-instance parameters this needs are the ones `ConfigureScript` already writes; what is missing is allocating them per case rather than per machine |
| Evidence | wall-clock for `_01_utility` at N=1 against N=4 and N=8, with verdicts identical to the serial run — **identical, not merely similar**: a case that passes only when it has the machine to itself is a finding, not an acceptable cost |
| Risk | this is where the parallel-run bugs live. It must not ship before the serial version is proven, or a difference has two possible causes |

### B-T7. Compare a query plan as data, not as prose — **blocked**

| | |
|---|---|
| Blocked on | T: the sql module (Phase 4) |
| Improves on | T: `StringUtil.replaceQureyPlan` |
| Kind | **capability** — it makes the suite able to see something it currently cannot |

**Today the masking blinds the test to what it was written to catch.** A plan is
compared as rendered text, and normalisation is one blind regex over the whole thing:

```java
queryPlan = queryPlan.replaceAll("[0-9]+", "?");
```

Cost and cardinality *should* be masked — they move with the statistics. But the regex cannot tell
a cost from an identifier, and the committed answer files show what that costs:

```
class: t?              (30 occurrences)   ← t1, t2 and t3 are the same string now
index: i_t?_j?_j?
index: i_t?_i?_i?_i?
```

So a plan that scans `t1` and a plan that scans `t2` compare equal. A plan that switched from
`i_t1_i1_i2` to `i_t1_i1_i3` compares equal. **These are plan tests, and a changed index is exactly
the regression they exist to detect.** The masking does not merely blur the answer; it removes the
signal and leaves the test passing.

**The engine already emits the plan as JSON.** `qo_plan_print_json` in
`src/optimizer/query_planner.c` builds it, and the identifiers are their own fields:

```c
json_object_set_new (scan, "table", json_string (class_name));
json_object_set_new (scan, "index", json_string (...->constraints->name));
```

and the switch is a system parameter that exists — `query_trace_format`, whose keywords are
`text` and `json` (`system_parameter.c:5694`).

| Beyond | ask for `query_trace_format=json`, compare field by field: mask `cost` and `cardinality`, keep `table`, `index` and the plan shape |
| Evidence | a case whose plan changes index from `i_a` to `i_b` fails, where today it passes. That is one case to write, and it is the whole argument |
| Constraint | the text form stays. The `.answer` files are frozen (NG1) and hold rendered plans; JSON is a **second** comparison an operator turns on, not a replacement |
| Note | this is the clearest instance of criterion 1 in ADR-015: it names what it beats and what today is measured, and the measurement is a count from the corpus rather than an opinion |

### B-T8. Stop paying 1.1 GB for every case — **blocked**

| | |
|---|---|
| Blocked on | T: the shell task passing the full-corpus gate (ADR-013) |
| Improves on | T: nothing in the runner. **The cost is inside the cases, and the cases may not be touched (NG1)** |
| Kind | **speed** — the same verdicts, sooner |

**Where the time actually goes, measured.** The runner is not the problem. Between two cases it
does a process reset and `RestoreScript`, which copies `conf/*` and `databases/*` out of
`~/.CUBRID_SHELL_FM` and deletes logs and cores; the gap between one case ending and the next
starting is **about one second**. The rest is inside the cases, and over all 217 of `_01_utility`
the two runners spend it identically:

| | CTP | testkit |
|---|---:|---:|
| total | 7,708 s | 7,670 s |
| **median** | **11 s** | **12 s** |
| mean | 17.8 s | 17.8 s |
| max | 219 s | 219 s |

**The median is the number that matters.** Half the corpus is cases that do very little, and they
still cost eleven seconds each, because eleven seconds is what it costs to arrive at the point where
a case can do anything at all. The mean is higher only because a few cases really are long.

What a case spends it on is visible in its own trace:

```
+ cubrid createdb -r csqldb --db-volume-size=20M en_US
  Creating database with 64.0M size using locale en_US.
  The total amount of disk space needed is 1.1G.
+ cubrid server start csqldb
  ... the actual test ...
+ cubrid server stop csqldb
+ cubrid deletedb csqldb
```

**The case asked for 20M and the engine laid out 1.1 GB**, because the volume size it was given is
not the log volume or the generic volumes. Every case in the corpus does this: create a database,
start a server, do a little work, throw the database away. Across 3,452 cases that is somewhere
near four terabytes written and deleted to run a suite whose actual queries are small.

**The interception point already exists, and it is not the cases.** `init.sh` puts `${init_path}`
at the head of `PATH` and makes `${init_path}/cubrid` executable, so **every `cubrid` a case runs is
already a wrapper script**, which today intercepts `deletedb`, `server` and `checkdb` to save a
snapshot when recovery fails. `cubrid_createdb` — what cases actually call — is a shell function in
`init.sh:1614`. Neither is in the testcases repository, so neither is frozen by NG1.

**And CTP already did this once.** `create_ccidb` in the same file checks for
`$CUBRID/databases/ccidbbak` and, if it is there and big enough, does `cp -r` instead of
`createdb`. The pattern is not a new idea here; it is one function away from being general.

| Beyond | three, in increasing order of what they touch. **(1)** put `$CUBRID_DATABASES` on tmpfs — nothing in CTP or the corpus changes, it is a placement decision. **(2)** generalise `create_ccidb` into `cubrid_createdb`: key a prepared database on `(charset, volume size, the parameters the case set)` and copy it when the key matches. **(3)** give each case a copy-on-write `$CUBRID` through overlayfs, which is worth little serially — the reset is already a second — and is what **B-T3** needs to give parallel cases an install each |
| Evidence | wall clock for `_01_utility` before and after, **with verdicts identical rather than similar**, and `_25_unstable` run both ways. That family is the one whose own readme says it depends on elapsed time and machine load, so it is exactly where a change in I/O timing would show up as a changed verdict — and a case that only passes when the disk is slow is a finding, not a regression |
| Instrument | `PS4='+[${EPOCHREALTIME}] '` exported from the runner's prologue puts a microsecond timestamp on every line `set -x` already prints, with no change to any case and none to `init.sh`, which sets no `PS4` of its own. It changes the bytes of the traced output, so it cannot be the default — it is a measurement mode, and it is what turns "createdb is slow" into a distribution |
| Risk | tmpfs needs the memory, and all three change I/O timing. Under criterion 2 of ADR-015 none of them may be built before the shell task passes its gate, for the reason B-T2 and B-T3 carry: change the ground under a case before the comparison is clean and every later difference has two possible causes |

### B-T4. A verdict that says why — **idea**

| | |
|---|---|
| Improves on | T: `Test.collectGeneralResult` |
| Today | a case fails if any line of its `.result` contains the substring `NOK`. A case that prints the word while explaining itself fails, and always has (`TestVerdict`) |
| Beyond | a structured result — which check, expected against actual — alongside the text, so a failure can be read without opening the case |
| Evidence | a failing case's report identifies the failed check without a human reading `.result`; the substring rule still decides the verdict, so nothing about pass or fail changes |
| Constraint | the `.result` file is a frozen surface written by the cases themselves, and the cases repository cannot be modified (NG1). So this **adds** a channel; it does not replace one |

### B-T5. Flakiness as a first-class outcome — **idea**

| | |
|---|---|
| Improves on | T: `dispatch.Queue`, `feedback.CaseStopRetry` |
| Today | a retried case reports only its final attempt. `feedback.log` carries the intermediate ones, and nothing adds them up |
| Beyond | every attempt is recorded and the case is classified: deterministic failure, passed on retry, or alternating. A build with 3 hard failures and 40 flakes is a different situation from one with 43 failures, and today they print the same |
| Evidence | a run over `_25_unstable` — 195 cases whose own readme says they depend on machine load — separates into classes that match what that readme predicts |

### B-T6. Ask a case why it is slow — **idea**

| | |
|---|---|
| Improves on | T: `TestMonitor` |
| Today | the monitor kills a case that outruns its timeout and records `NOK timeout`. What the case was doing is gone |
| Beyond | before the kill, capture what the monitor can see — the process tree, what the server is waiting on — into the worker log |
| Evidence | a timed-out case's log says what it was blocked on; no change to the verdict or to the timeout |

---

## B-O — beyond, on the QA-operations axis

Every entry here is **blocked on the operations layer existing at all** (ADR-012). They are recorded
now because they were found now, and because they say what that layer is for.

### B-O1. More machines by more runners — **blocked**

| | |
|---|---|
| Blocked on | ADR-012 |
| Improves on | O: the fleet excluded by ADR-014 — instance inventory, deploy across N, dispatch between them |
| Today | CTP taught one runner to manage a fleet, and that is what made it need to own its machines |
| Beyond | the operations layer shards a corpus across N runners, each on its own machine, and merges the results. The runner stays scoped to one machine, which is what makes it something you can hand a container |
| Evidence | 3,452 cases across 4 runners complete in near a quarter of the single-runner time, and the merged verdicts match a single-runner run case for case |

### B-O2. Results that outlive the next run — **blocked**

| | |
|---|---|
| Blocked on | ADR-012 |
| Improves on | O: `FeedbackDB` (excluded), `current_runtime_logs` |
| Today | the run directory has a fixed name and the next run overwrites it. The tarball CTP packs at the end is the only history, and it is a file on the machine that produced it |
| Beyond | a result store with history, so **"when did this case start failing?"** is answerable without unpacking archives |
| Evidence | the question above, answered for a case, across runs, from a query rather than from a shell pipeline |
| Note | this is where `FeedbackDB` should have been. It was excluded as axis O, and it is the right thing built in the right place rather than the wrong thing kept |

### B-O3. Failures grouped by cause — **idea**

| | |
|---|---|
| Blocked on | ADR-012, B-O2 |
| Improves on | O: the JIRA filing excluded in `migration-exclusions.md` §1-3 |
| Today | CTP files an issue per failure from a template. A single broken commit produces as many issues as it broke cases |
| Beyond | failures are grouped by signature — the same assertion, the same stack — so two hundred failures with one cause become one item with two hundred instances |
| Evidence | a run with a known single-cause breakage produces one group |

---

## What is not here

Anything that adds a **new kind of testing** belongs in §6a, not here (ADR-015). SQLancer, fuzzing
and workload generation change what can be found; this axis changes how well the existing work is
done. If an entry would need a new oracle, it is an extension.
