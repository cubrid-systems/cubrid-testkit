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

**What this method catches:** things the analysis stopped short of. It needs a specific question to
chase; it does not volunteer.

## 3. Found by writing the code

Every literal was taken from CTP's source rather than from the notes. Four disagreed.

| Spec said | Actually | Source |
|---|---|---|
| `[NOK], retry: <N>` | `[NOK], TRY-><N>` | `Constants.RETRY_FLAG` |
| the retry suffix appears when a retry happened | when retries are **configured** — `maxRetryCount != 0`. A first-attempt failure prints `, TRY->0` as soon as `testcase_retry_num` is set | `shell/main/Test.java:189` |
| results go to `result/<task>/<timestamp>/` | `result/<category>/current_runtime_logs` — **no timestamp**. ADR-013 had inherited the error and listed a timestamp among the values to mask | `Context.java:172` |
| `main.info` is part of the shell layout | it belongs to sql/cqt; the shell family never writes one | `TestUtil.TOTAL_SUMMARY_FILE` |
| the unittest plug-in returns values via an `EEOOKK` marker | **three** markers: `GPROPSTART` ends the plug-in's output, values sit between `G_PROPERTY_<K>=` and `EEOOKK` | `GeneralLocalTest.invoke` |

| — | **Remote output is delimited by a frame.** `echo ALL_${NOTEXIST}STARTED` … `echo ALL_${NOTEXIST}COMPLETED`, and only what lies between `ALL_STARTED` and `ALL_COMPLETED` is kept. The unset variable is the mechanism: the script's own text never matches the marker, so a shell echoing its input cannot open the frame early | `ScriptInput`, `SSHConnect` |

**What this method catches:** anything where the spec paraphrased instead of quoting. Writing a
literal into a program forces you to know it exactly; prose lets you almost know it.

## 4. Found by running it

| | |
|---|---|
| **The dispatcher prints a banner around every task** — a rule, `TEST STARTED`, `TEST END`, `ELAPSE TIME`. The spec had no dispatcher output at all. The opening rule is printed *before* the task name is resolved, so an unknown task gets a banner and then help | `CTP.java:132-143,192-196` |
| **unittest's output resembles nothing else** — step headings with a trailing space, a one-based index, `[SUCC]`/`[FAIL]` rather than `[OK]`/`[NOK]`, verdict on the same line as the name. The spec had described unittest's *plug-in contract* and never its output | `GeneralLocalTest.start` |
| **Two console lines come from a feedback backend.** `Test Category:` and `The Number of Test Cases:` are printed by `FeedbackFile`, to its own file *and* to stdout. Nobody looking for console output would look there | `FeedbackFile.java:134-137` |

**What this method catches:** whole surfaces nobody thought to write down. Reading more carefully
would not have found these, because the question "what else does it print?" has no place to be
asked until something prints.

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

## What this says about the freeze

A frozen surface is only as good as the reading behind it, and the reading was done four different
ways here with four different yields. The spec was not careless — it was written from a careful
analysis — and it was still wrong in nine places, every one of them F1.

Two practical consequences:

1. **The regression evidence, not the specification, is the contract** (ADR-013). A document can be
   wrong quietly; a diff cannot.
2. **Corrections are recorded where the mistake was, and dated.** A spec that silently improves is
   indistinguishable from one that was always right, and the reader has no way to tell which parts
   have been tested against reality.
