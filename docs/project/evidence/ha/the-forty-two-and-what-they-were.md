# The 42 the scale run called failures, and what each of them was

- **Date:** 2026-09-23
- **Status:** measured, and every one of the 42 is accounted for. Four runs of the same cases under
  four conditions, plus a direct timing of the case that survived them all.
- **What this is:** a whole-corpus run over `_01_object` reported 42 cases as `differ`,
  `wait_timeout` or `case_failed`. **Thirty-one were the runner, not the engine** — and finding
  which took three reruns, because two of the three causes are things this suite does to itself.
- **Trees:** engine `11.5.0.2513-5f3a30d` · pairs from `cubrid-cluster-sandbox`, docker, one host.
- **The headline:** **no new engine finding in 3,327 cases.** The eleven real differences are the
  three defects this directory already documents, met again in new places.

---

## The four runs

| | pairs | machine | 42's outcome |
|---|---|---|---|
| **A** — the scale run | 8, fresh | eight-way, iowait 52% | 11 `differ` · 23 `wait_timeout` · 8 `case_failed` |
| **B** — first reproduction | 1, **worn** (35,417 pages) | quiet | 1 `differ` · 4 `case_failed` · 1 `wait_timeout` of 34 judged |
| **C** — clean reproduction | 1, fresh | quiet | 11 `differ` · 2 `wait_timeout` · 1 `case_failed` |
| **D** — after the fixes | 1, fresh | quiet | **11 `differ`, nothing else** |

**B is in this table because it was wrong**, and the way it was wrong is the point. Its pair had
been reused all day. The same case that takes 639 ms on a fresh pair took **121,369 ms** on it — a
190x slowdown with no contention — and came back `case_failed` instead of `differ`. That is
[the wear this directory already recorded](a-sandbox-pair-wears-out.md), applied to the very
experiment meant to control for it: *"a pair reused across days is not a control"*, written down
two runs earlier and not followed.

## What the 42 actually were

| cause | cases | whose |
|---|---:|---|
| **eight-way contention** | 26 | the measurement's |
| **the runner's marker bound was a tenth of CTP's** | 2 | the runner's |
| **the runner had no bound for running a case** | 3 | the runner's |
| **replication genuinely differs** | 11 | **the engine's** |

### 26 were contention

Every one is in `_09_partition`. Alone on a fresh pair they pass. Per-case cost rises 1.52x under
eight-way concurrency ([`pairs-across-machines.md`](pairs-across-machines.md)), and that is enough
to push a case whose slave was already catching up past a sixty-second bound.

The clearest single case is `_09_partition/_002_alteration/1007`: **88.7 s and `wait_timeout` in run
A, 1.4 s and `same` when it is the only thing running.** Nothing about that case is slow. It was
queued behind a neighbour's partition DDL, and the bound was measuring the queue.

### 2 were a conf key this runner never read

CTP's key is **`ha_sync_detect_timeout_in_secs`**, in seconds
(`ConfigParameterConstants.java:74`), and its default is **600 s** (`Constants.java:46`). This
runner read `ha_sync_detect_timeout_in_ms` — the name of the *Java field* CTP parses it into
(`Context.java:90`) — and defaulted to 60 s.

So two things were wrong at once. A CTP `ha_repl.conf` setting the real key was read here as nothing
at all, though conf keys are F3 on the frozen surface and have to be accepted as input. And the
default was a tenth of the baseline's.

**CTP would have waited ten minutes. This waited one.** The measurement agrees with CTP: the slowest
of the two needs about **146 s** alone, which is comfortably inside 600 s and outside 60 s.

### 3 were a bound that did not exist

`internal/sandbox.DefaultTimeout` is two minutes, and it bounded everything — a `ha status` read and
a call running a case's own SQL alike. The comment said why two minutes was enough: *"every call
this package makes is a read or an exec."* A call that runs a case is neither.

`_09_partition/_001_create/bug_xdbms294` is the corpus's largest case, 67 KB: two `CREATE TABLE`s of
1,024 list partitions each, with a `DROP` after each. Timed directly on a fresh quiet pair:

| | |
|---|---:|
| first CREATE, cold database | **158.0 s** |
| DROP | 4.8 s |
| second CREATE, warm database | **82.2 s** |
| DROP | 5.1 s |
| all four in one `csql` call, warm | **130.0 s** |
| the slave applying one such CREATE | 27.2 s |

`reset=case` hands every case a cold database, so this case costs about **250 s** every time it
runs. It was `case_failed` at exactly 120,000 ms in every run at every level of load — and until
this morning it said **"the master could not be reached"**, which is not what happened: the runner's
own clock killed the call, and `exec`'s `signal: killed` is indistinguishable from something outside
the run doing it.

It now passes at **275.3 s**.

### 11 are the engine, and all three causes are already written down

| what | cases | finding |
|---|---:|---|
| `call add_user(...)` does not replicate, so the user and everything it owns is absent on the slave | 8 | [method-calls-on-the-catalog-do-not-replicate](method-calls-on-the-catalog-do-not-replicate.md) |
| a DDL whose text names a csql session variable cannot be replayed | 2 | [ddl-with-a-session-variable-does-not-replay](ddl-with-a-session-variable-does-not-replay.md) |
| `call change_owner(...)` does not replicate | 1 | [class-owner-change-not-replicated](class-owner-change-not-replicated.md) |

The eight are one fact with seven consequences. `TEST_USER` is not on the slave, so the class it
owns is not, so that class's attributes are not, so the grants to it are not — which is why the
reads come back `There are no results.` rather than with a wrong row.

## What was changed, and the evidence that it changed nothing else

Three bounds, in three commits: a bound for running a case (`CaseTimeout`, ten minutes, from the
250 s measurement), CTP's key and CTP's default for the wait, and a thirty-second bound on a probe
of the slave — which had been carrying the case bound, so a slave that stopped answering would have
held a ten-minute probe against a deadline the loop only checks between probes.

**Run D is the check that matters.** With all three in place, the eleven differences are still
eleven, still the same eleven, and still take between 0.7 s and 4.3 s. Raising a bound did not turn
a failure into a pass anywhere: it stopped the bounds from hiding what was underneath them.

## What is not claimed

That `_09_partition` passes under eight-way concurrency now. The 26 contention cases were not re-run
sharded after the bounds changed. The bounds are larger than what they need, so they should — but
that is an expectation, not a measurement.

That 42 is the corpus's true difference count. It is what this runner reports; CTP's conversion
deletes the statements several of these are about, so there is no second opinion to compare against
([ctp-ha-repl-deletes-the-call](ctp-ha-repl-deletes-the-call.md)).

That ten minutes is the right case bound anywhere but here. It is 250 s measured on this machine
with room for a slower one.
