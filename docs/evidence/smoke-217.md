# The smoke corpus

- **Date:** 2026-09-07
- **What this is:** `_01_utility` in full — 217 cases rather than four — twice:
  a first attempt that did not finish, and a second that did.
- **Method:** `evidence/compare/`, one shard, ADR-013 normalisation.

> **It produced no comparison.** CTP finished; testkit stopped at case 2 of 217
> and would not have restarted. What it produced instead is four findings, and
> they are worth more than the comparison would have been.

---

## 1. What ran

| | |
|---|---|
| Shard | `_01_utility`, 217 cases, the ADR-013 smoke set |
| CTP | **217/217** — 170 passed, 47 failed, exit 0, **4,101 s (68 min)** |
| testkit | **stopped at 2/217**, no progress for 23 minutes, killed by hand |

**Per-case cost, measured rather than assumed: 18.9 seconds.** `design/module-shell.md`
and the entries that quote it use about 70, which came from a QA machine and is
nearly four times what this one does. At the measured rate the whole corpus is
around 18 hours a runner rather than 67 — still long enough for the shard to be
the unit of resume, and short enough that the plan can stop treating the number
as forbidding.

## 2. Why testkit stopped

`_01_sqlx/bug_cubridsus2018`. **CTP ran the same case in 14 seconds.**

```
sh bug_cubridsus2018.sh
 └─ /bin/bash $init_path/cubrid server stop testdb
     └─ cub_commdb -S testdb          hrtimer_nanosleep -- a retry loop
```

The monitor did its job: it fired at 600 seconds, and `feedback.log` carries the
`[RESOLVE]` entry to prove it. The sweep took `cub_master`, `cub_server` and
`cub_javasp` — all three were still there as zombies when the tree was
captured — and left `cub_commdb`, which is what the case was actually waiting
on. Nothing else happened for 23 minutes, and nothing else was going to.

**Three defects, one behind the other.**

**(a) The monitor resolved once.** `markTimedOut` cleared the worker's start
time, so every later check found nothing running and returned. CTP keeps
resolving every three seconds until the case ends (`TestMonitor.resolveTimeout`
leaves `test.startTime` alone). A plain parity defect, and fixed as one.

**(b) A sweep that does not free the case blocks the run for ever.** CTP has no
escalation either — it resets the processes and trusts the case to come back —
so this is not a divergence. It is a shared assumption that held until it did
not. The runner now ends the case's process group after a grace period, which is
a deviation admitted because the gate cannot be reached without it: one case in
3,452 behaving this way stops the run, and ADR-013 needs a run that finishes.

**(c) Cancelling reached the shell and nothing below it.** Ending a case means
ending what the case started. Local commands now run in a process group of their
own and cancellation kills the group; before, the shell died and its children
kept the pipe open, so `Wait` never returned and the runner waited on a case that
no longer existed.

## 3. The process reset has never run inside the namespace

This one is about the evidence rather than the runner, and it is the
uncomfortable one.

Both runners sweep with `ps -u $USER`. Both are run inside
`unshare --map-root-user`, where the processes belong to uid 0 while `$USER` is
still the outer name:

```
$ ./in-ns.sh bash -c 'echo "$(whoami) uid=$(id -u) USER=$USER"; sleep 30 & ps -u $USER -o pid,comm; ps -e -o pid,comm'
root uid=0 USER=hgryoo
    PID COMMAND              <- ps -u $USER: nothing
      1 bash                 <- ps -e: the processes are there
      5 sleep
```

**`ps -u $USER` selects nothing.** Every `killPatterns` entry, the JVM sweep, the
`sleep`/`expect`/`dos2unix` kills and `ipcs | grep $USER` have been no-ops in
every namespaced run. What still worked is `cubrid service stop`, the one line of
the script that does not go through `ps` — which is exactly why the master and
the server died and `cub_commdb` did not.

**Two consequences, and they point opposite ways.**

The four-case comparison in `regression-shell.md` **still holds**. Both runners
use the same `ps -u $USER` and both ran inside the same wrapper, so both were
equally neutered; the comparison was fair. What cannot be claimed is that it
exercised the reset, and §4 of that page already listed the timeout as untested
for the same kind of reason.

**`regression-shell.md` §1 explains the namespace wrongly.** It says a PID and
IPC namespace "makes `ps -u $USER` and `ipcs` show the run its own work and
nothing else". It does not show the run its own work; it shows nothing. The
containment is real — nothing outside the namespace died, and eleven `cub_server`
and `cub_master` processes belonging to other work on the machine were untouched
— but it comes from emptying the sweep, not from scoping it. **B-T2 is about
making the sweep reach exactly the run's own work, and the wrapper does not do
that today.**

The fix is one line and it is not obviously safe: `USER=root` inside the
namespace makes `ps -u $USER` select the namespace's own processes, which is the
intent. But `$USER` reaches the cases — `TEST_SSH_USER` is built from it — and
`ipcs | grep $USER` reads it too, so it is a change to what a case observes.
That is a freeze question, and it is left open here rather than answered.

## 4. Why it was slow, which turned out not to be the runner

Diagnosed after the run was stopped. `_01_sqlx/bug_cubridsus2018` runs in 21 seconds under CTP and
timed out under testkit, on the same machine in the same minute, so it was not machine state. It was
the wrapper.

`in-ns.sh` ended in `exec "$@"`, which makes whichever runner it starts **PID 1 of the new PID
namespace**. PID 1 has to collect orphans. A shell does; a Go binary does not. CTP's PID 1 was
`bash ctp.sh` and it collected what its cases left; testkit's was the runner and it did not, so
`cub_master` never learned that `cub_server` had exited and `cubrid server stop` polled through
`cub_commdb -S` indefinitely. The zombies in §2 were the symptom, read at the time as debris.

| `_01_sqlx/bug_cubridsus2018` | |
|---|---|
| CTP, `exec` wrapper | 21 s, `[NOK]` |
| testkit, `exec` wrapper | **over 12 minutes**, hung |
| testkit, shell left at PID 1 | **15 s**, `[NOK]` |
| CTP, shell left at PID 1 | 23 s, `[NOK]` |

Same verdict on all four. The wrapper keeps a shell at PID 1 now.

**It is not a defect in the runner** — a QA machine has no namespace, so PID 1 is the system init and
reaps. It is a defect in the runner *as a container image*, which is what B-T2 is about, and it is
recorded there: handing testkit a container puts it at PID 1 for real, with no shell above it.

## 5. What this run does not tell us

| | |
|---|---|
| Equivalence | nothing. No comparison was produced |
| The rest of the corpus | 215 of 217 cases on the testkit side never ran |
| Whether the fixes work at scale | the next run is the test of that, and it has not happened |

The shard wrote no `report.txt`, so a resumed run does not skip it — which is
the one part of the harness this confirmed by using it rather than by testing it.

---

## 6. The second run, which finished

testkit again, with the monitor fix and a shell left at PID 1. Nothing stalled;
**217/217, exit 0**, at 14.5 s a case against the morning's 18.9.

Compared against the CTP tree from that morning:

| | |
|---|---|
| `dispatch_tc_ALL.txt` | **217 lines, identical** — the same cases, in the same order |
| `dispatch_tc_FIN_local.txt` | **217 lines, identical** — both finished all of them |
| `current_task_id` · `monitor_local.log` | identical |
| `main_snapshot.properties` | 55 differences, all classified, **0 new** |
| `check_local.log` | identical, one recorded deviation (`dos2unix-not-required`) |
| exit codes | 0 and 0 |
| `test_status.data` | **170/47 against 173/44** |

**Five cases disagree, and four of them are not a runner difference.**

| CTP | testkit | case |
|---|---|---|
| NOK | OK | `_37_cubrid/_04_broker` |
| NOK | OK | `_40_broker/_enhance_b5` |
| NOK | OK | `_40_broker/itrack01` |
| NOK | OK | `_40_broker/itrack02` |
| OK | **NOK** | `_08_copydb/bug_xdbms_sus1210` |

The four broker cases fail on CTP at `cubrid broker start: fail` and pass on
testkit where the same line reads `success`. **Running `itrack01` under CTP again
a few hours later gives OK** — so CTP does not reproduce its own verdict here,
and the cases belong with `_25_unstable` rather than in a smoke set: whether a
broker starts depends on what else on the machine is holding a port, and this
machine runs other CUBRID work continuously.

`_08_copydb/bug_xdbms_sus1210` points the other way and is **not explained**.

**The method has a hole and it is mine.** The CTP side is the 13:08 run and the
testkit side is the 15:39 one, reused to save 68 minutes of re-running CTP for an
answer already known. Three hours is long enough for this machine to change, and
it did. The five disagreements therefore cannot be read as runner differences
from this report alone — each needs the two runners minutes apart, which is what
`shard.sh` does when it is allowed to run both sides.

**What it does establish**, and did not before: both runners discover the same
217 cases in the same order, finish all of them, agree on 212, produce identical
verdict files apart from one recorded deviation, and cost the same per case.
