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

### B-T2. A runner you can hand a container — **ready**

| | |
|---|---|
| Improves on | T: `Constants.createLinKillScripts`, and the posture behind it |
| Today | the reset before every case matches substrings across everything the user owns. On the machine used for `evidence/regression-shell.md` that was 63 processes and 13 shared-memory segments belonging to other work. **A CTP run and anything else the same user is doing cannot share a machine** |
| Beyond | the runner runs inside a PID and IPC namespace it creates for itself, so the reset reaches its own work and nothing else |
| Evidence | the full suite runs on a developer's machine while that developer keeps working; the reset script is unchanged, and `ps -u $USER` inside the namespace shows only the run |
| Note | the wrapper already exists — `regression-shell.md` §1 had to build it before either runner could be measured. It is a script beside the evidence, not part of the runner. **This entry is about making it the runner's own behaviour**, which ADR-014 made possible by scoping the runner to one machine |

### B-T3. Cases that run at the same time — **blocked**

| | |
|---|---|
| Blocked on | T: the shell task passing the full-corpus gate (ADR-013) |
| Improves on | T: `Test.runAll` — one case at a time, per machine |
| Today | a case restores the whole CUBRID install before it runs (`RestoreScript`), so two cases cannot share a machine. `_01_utility` is 217 cases at roughly 70 seconds each: **about four hours, serially**, and the full corpus is 3,452 |
| Beyond | each case gets its own instance — own port, own shared-memory id, own data directory — so N run at once on one machine. The per-instance parameters this needs are the ones `ConfigureScript` already writes; what is missing is allocating them per case rather than per machine |
| Evidence | wall-clock for `_01_utility` at N=1 against N=4 and N=8, with verdicts identical to the serial run — **identical, not merely similar**: a case that passes only when it has the machine to itself is a finding, not an acceptable cost |
| Risk | this is where the parallel-run bugs live. It must not ship before the serial version is proven, or a difference has two possible causes |

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
