![cubrid-testkit — finds the cases, runs them, judges them, records what happened. Every task it takes over keeps the same commands and the same output. Shown: a run marking cases OK and NOK, and the summary it writes out.](docs/assets/banner.svg)

**cubrid-testkit** runs CUBRID's functional tests. Find the cases, run them against an engine,
decide whether each one passed, write down what happened — and nothing else.

It is a drop-in for CTP's `bin/ctp.sh`: the same task names, the same config keys, the same markers
on stdout, the same result files, the same exit codes. Tasks that have been rewritten in Go run
here; the rest are handed to the original CTP as a subprocess, and from outside there is no way to
tell which is which. That is what lets the old system keep running while the new one takes over one
task at a time.

Deciding *when* to run, telling people the result, and filing the issue that comes out of it are a
different job. They are out of scope, listed with reasons in
[`concept/migration-exclusions.md`](docs/concept/migration-exclusions.md), and get rebuilt later as
a layer that consumes this system's output.

For engine developers and QA. Part of
[CUBRID Systems Research](https://github.com/cubrid-systems).

> **Where it is:** Phase 3 is in progress. `unittest` runs natively, and `shell` runs end-to-end
> and agrees with CTP on four real cases — behind an opt-in gate until the full corpus clears.
> Every other task dispatches to CTP unchanged. A shell run can now use several slots on one
> machine; see [Running the shell suite in parallel](#running-the-shell-suite-in-parallel) and
> [Status](#status).

## Prerequisites

| | Why |
|---|---|
| **Go** | to build the binary. Version from `go.mod` |
| **A CTP checkout** | any task that has not been rewritten runs the original as a subprocess: `CTP_HOME`, and `JAVA_HOME` for that path |
| **A CUBRID build** | the engine under test, with its own install and `CUBRID_DATABASES` |
| **A testcases checkout** | the cases themselves, for the tasks that read a corpus |

Linux. Windows is stale and out of scope — the native runner refuses it and says so.

The shell suite has always needed a bash-compatible `/bin/sh` — CTP's own `init.sh` opens with
`function get_os(){` — and **the runner now arranges that itself**, binding bash over `/bin/sh`
inside its own mount namespace. The machine is not changed and nothing outside the run sees a
different shell; on a machine that already has it right the bind is skipped. Set
`TESTKIT_CONTAIN_SH` to override. Without it, on a distribution where `/bin/sh` is dash, every case
dies on `init.sh`'s first line — measured: seventeen cases, seventeen blank results, seventeen
`Syntax error: "(" unexpected`.

## Build

```bash
go build -o bin/testkit ./cmd/testkit
go test ./... -count=1
```

The tests under `internal/cli` are the frozen command line written down, so a failure there is a
contract change rather than a broken refactor. CI runs gofmt, `go vet`, the tests and the build.

## From `ctp.sh` to `testkit`

The compatibility is not a promise made in prose; it is where the binary sits. The endpoint is that
a QA machine keeps calling `bin/ctp.sh` and gets this runner, and nothing that reads the output can
tell.

**The shim is not in place yet.** `bin/ctp.sh` in `cubrid-testtools` is still the original, and it
should stay that way until the corpus comparison clears — the gate is what earns the swap. Today you
reach this runner by invoking it directly, which is what the comparison harness does when it runs
both sides on the same shard. Everything below the shim is built and running; the diagram is the
design ([`design/architecture.md`](docs/design/architecture.md)) with the top line marked as the
part that is still ahead.

```
        what a QA machine runs                what actually happens
                                              (── shim: not yet ──)
        ──────────────────────                ─────────────────────
        $ ctp.sh shell -c shell.conf   ──┐
        $ ctp.sh unittest -c u.conf    ──┤    a shim: exec testkit "$@"
        $ ctp.sh sql medium -c s.conf  ──┘              │
                                                        ▼
                                              ┌───────────────────┐
                                              │  one registry     │   task name → runner
                                              └─────────┬─────────┘
                                        rewritten ──────┴────── not yet
                                            │                     │
                                    ┌───────▼──────┐      ┌───────▼────────┐
                                    │ native (Go)  │      │ CTP, as a      │
                                    │ shell·unittest│      │ subprocess     │
                                    └───────┬──────┘      └───────┬────────┘
                                            └──────────┬──────────┘
                                                       ▼
                                        the same files, the same stdout markers,
                                        the same exit codes — the frozen surface
```

Three things follow, and they are the whole design.

**The entry scripts stay.** `bin/ctp.sh` becomes a shim that hands its arguments over unchanged, so
every Jenkins job, every `docker-entrypoint.sh`, every habit keeps working — no caller is edited and
nothing has to be migrated on a schedule. The command line is already frozen and tested as such:
`internal/cli` is that contract written down, which is what makes the eventual swap a one-line
change rather than a negotiation.

**A task is routed, not converted.** One registry turns a name into either a native runner or the
legacy one, and the legacy path reproduces CTP's argv and environment byte for byte. `shell` and
`unittest` run natively; the other twelve are handed to the original CTP.

**The output belongs to neither.** Both paths write through the same result layer, because the
surface is a property of the output rather than of whichever implementation ran
([ADR-003](docs/adr/ADR-003-external-surface-freeze.md)). That is what lets a task move from one
side to the other without anything downstream noticing — and what makes the equivalence gate
meaningful, since it compares two runners' files rather than two runners' intentions.

So the usage below is the usage `ctp.sh` has always had: same task names, same `-c`, same exit
codes. `testkit` is what you type today; `ctp.sh` is what a machine will type, and the point of
freezing the command line first is that the two are the same words.

## Using it

```
testkit <task>... [-c <conf>] [--interactive] [-h] [-v]
```

```bash
testkit shell -c shell.conf
testkit sql medium -c ~/CTP/conf/sql.conf     # several tasks, in the order given
TESTKIT_NATIVE_SHELL=1 testkit shell -c shell.conf
```

Task names are matched case-insensitively. A name that is not a task prints help and skips that
task only — the tasks after it still run. `CTP_HOME` comes from the environment if it is set,
otherwise from the parent of the binary.

**`run-shell` is the other CLI tree** — one case, looped until it fails, which is what you reach for
after the suite has told you a case is unreliable:

```bash
testkit run-shell --loop --maxloop 200 _01_utility/_38_csql/csql1
testkit run-shell -h
```

`--loop`, `--maxloop`, `--maxtime`, `--extend-script` and `--prompt-continue` are the axis-T options
of CTP's `run_shell.sh`; the testcase argument may name the case directory, its `cases/`
subdirectory, or a file in either, and defaults to the working directory. Touching a file named
`STOP` in the case directory ends the loop after the attempt in flight. The seven QA-operations
options — `--update-build`, `--enable-report`, `--mailto` and the rest — say so by name instead of
being ignored.

**Fourteen tasks**, the set CTP accepts:

| | |
|---|---|
| runs natively | `unittest` · `shell` behind `TESTKIT_NATIVE_SHELL=1` |
| dispatched to CTP | `sql` `medium` `kcc` `neis05` `neis08` `sql_by_cci` `rqg` `isolation` `ha_repl` `cdc_repl` `jdbc` `webconsole` |

Seven more names — `cci` `dots` `nbd` `sysbench` `tpcc` `tpcw` `ycsb` — are ones CTP accepted and
silently did nothing about. They now say they are retired and move on to the next task: the same
outcome, without the silence.

The runner runs on **one machine** ([ADR-014](docs/adr/ADR-014-one-machine.md)): local by default,
and a remote machine over SSH is still one machine. RMI worker mode is retired and asking for it
fails loudly rather than falling back.

### What a run leaves behind

The result files are part of the frozen surface, so they are the same whichever runner produced
them: `dispatch_tc_ALL.txt` and `dispatch_tc_FIN_local.txt` (the cases found and finished),
`test_status.data` and `check_local.log` (the verdicts), `current_task_id`, `monitor_local.log`,
`main_snapshot.properties`, `feedback.log`, `test-shell.xml`, `test_local.log`, plus `main.info`
and `summary_info`.

One file is this runner's own, and it is absent unless it has something to say: `patched.txt`, the
cases that did not run as the corpus has them. See [`patches/README.md`](patches/README.md).

![Animated: a shell run checks the machine, finds the cases, runs each one after a process reset, judges the result file, and writes the verdicts down. Four real cases: three OK and one NOK. Every stage leaves a file that is part of the frozen surface.](docs/assets/anim-shell-run.svg)

## The shell task in detail

`shell` is the task this project has rewritten, and it is the one with options. What a run does, in
order:

```
  check the machine      commands, variables, directories — once, so that a machine
        │                that cannot run cases says so here rather than 3,444 times
        ▼
  prepare the workspace  the corpus as the conf points at it
        │
        ▼
  find the cases         <scenario>/**/<name>/cases/<name>.sh, then the two exclusions
        │
        ▼
  order them             longest first, if a plan from an earlier run is available
        │
        ▼
  deploy                 the shell helpers CTP's cases source
        │
        ▼
  run                    N slots pull from one queue ── for each case:
        │                  reset processes → run it → check for more errors →
        │                  read its .result → record the verdict
        ▼
  retries                only when nothing else is running
        │
        ▼
  write everything down  the frozen files, and the durations for next time
```

### Configuration

Everything is a key in the conf file `-c` points at. The first group is CTP's and behaves as it
always did; the second is this runner's and is off unless set.

| CTP's keys | default | |
|---|---|---|
| `scenario` | — | the corpus root. Required |
| `test_category` | — | `shell` |
| `testcase_retry_num` | `0` | attempts after the first failure |
| `testcase_timeout_in_secs` | `0` | 0 is no timeout. CI uses 720 |
| `testcase_exclude_by_macro` | — | skip any case whose text contains this. CI uses `LINUX_NOT_SUPPORTED`, which is 21 cases |
| `testcase_exclude_from_file` | — | files of path fragments to skip, comma-separated. Upstream's list and this machine's stay separate — see `exclusions/README.md`. CI uses the corpus's own `daily_regression_test_excluded_list_linux.conf`, which is 9 cases |
| `testcase_workspace_dir` | — | where the corpus is prepared |
| `test_continue_yn` | `false` | resume, skipping what already has a verdict |
| `cubrid_db_charset` | `en_US` | passed to every `createdb` |
| `enable_check_disk_space_yn` | `false` | check free space before each case |
| `reserve_disk_space_size` | `2G` | how much that check demands |
| `large_space_dir` | — | `$TEST_BIG_SPACE` for cases that need room |
| `ignore_core_by_keywords` | — | core dumps to disregard |
| `feedback_type` | — | `file` |

| this runner's keys | default | |
|---|---|---|
| `parallel_slots` | `1` | how many cases run at once |
**`$CUBRID_DATABASES` belongs inside `$CUBRID`.** CUBRID's default is
`$CUBRID/databases`, the install ships `databases.txt.sample` there, and CTP's
own per-case reset cleans and restores exactly that path. Pointing it elsewhere
leaves the reset scrubbing a directory nothing uses while the real registry
carries entries from case to case and from run to run — and it fails the twelve
cases that write to the documented location. Measured: 12 of 12 NOK with the
registry outside the install, 12 of 12 OK with it back where CUBRID puts it.
Slots stay isolated either way, because `$CUBRID` already has a per-slot overlay
and one overlay covers both.

| `scenario_ram_mb` | off | put the corpus behind an overlay whose upper layer is a tmpfs of this size |
| `status_http` | off | serve the progress page |
| `case_plan` | off | a file of per-case durations, read to order the run and written from what it measured |
| `case_sizes` | off | a file of per-directory footprints in MB, written from what the run measured |
| `case_patch_dir` | off | apply a per-case compatibility patch where one exists. See `patches/README.md` |
| `lane_slow_secs` | off | split the slots into a tmpfs lane and a disk lane at this duration |
| `lane_slow_mb` | off | the same split, at this footprint. Needs `case_sizes` |
| `heavy_in_flight_max` | `slots/4` | how many of the heaviest cases may run at once |
| `scenario_ram_high_water` | `80` | percent of `scenario_ram_mb` above which no new case starts |

And the environment:

| | |
|---|---|
| `TESTKIT_NATIVE_SHELL=1` | run `shell` here rather than handing it to CTP. The opt-in gate, until the corpus comparison clears |
| `TESTKIT_CONTAIN=1` | put the run in namespaces of its own. Required by slots and by `scenario_ram_mb` |
| `TESTKIT_CONTAIN_SH` | which shell to bind over `/bin/sh`; `bash` if it can be found |
| `TESTKIT_SLOT_ROOT` | where per-slot overlays go. `/var/tmp/testkit-slots` |

CTP's own environment still applies — `CUBRID`, `CUBRID_DATABASES`, `CTP_HOME`, `JAVA_HOME` — and
so do the shell suite's switches, `SKIP_CHECK_RECOVERY_ERROR`, `SKIP_CHECK_FATAL_ERROR` and
`CTP_ERROR_BACKUP`.

### Watching a run

Clicking a case in **slots** shows what it has written *so far*. A running case
has nothing in `feedback.log` — the block is written when it ends — but it
appends a line to its own result file at every check, so this is where a case
that has held a slot for 325 seconds against a plan of 6 says which check it is
stuck on. The panel polls while it is open and stops when the case leaves the
slots table. Clicking a case in **finished** shows the recorded block instead.


```bash
# in the conf
status_http=on              # 127.0.0.1:51523
status_http=8123            # every interface, port 8123
status_http=0.0.0.0:51523   # spelled out
```

```bash
TESTKIT_CONTAIN=1 TESTKIT_NATIVE_SHELL=1 testkit shell -c shell.conf
# [INFO] status page at http://127.0.0.1:51523/
```

Open it and the page says, once a second: how far the run is and how fast, which slot is on which
case and for how long, how long cases take by duration bucket and where the seconds go, totals by
family and by slot, what the machine is doing, and every failure so far. Nothing is written to
standard output, because what the runner prints there is frozen and the comparison reads it — a
screen drawn over it would be drawn over the evidence.

**A finished run can be played back through the same page**, which is the only way to see the shape
of one after it ends — which slots were busy together, where they went idle, which family owned the
time:

```bash
testkit replay --speed 60 <result-dir-or-feedback.log>
testkit replay --speed 3600 --http 8123 ~/CTP/result/shell/current_runtime_logs/feedback.log
```

Nothing is recorded for this. A run already writes the slot, the case, the verdict, the elapsed time
and the wall clock each case finished at, into `feedback.log`, which every runner produces — so a
replay works on runs that finished before the feature existed. The wall clock is compressed; the
durations reported are the real ones.

## Running the shell suite in parallel

Upstream CI spreads this corpus over 60 Kubernetes pods. The same corpus can be spread over the
slots of one machine instead, and everything here exists to make that safe rather than merely fast.

Parallelism is off by default. Everything below is one machine, one binary, and no change to the
output — a slotted run writes the files a serial run writes, because slots share an environment id
on purpose.

| key | | |
|---|---|---|
| `parallel_slots` | `1` | how many cases run at once. Each slot gets its own PID, IPC, mount and **network** namespace, so every slot keeps the shipped port 1523 and no configuration is rewritten |
| `scenario_ram_mb` | off | put the corpus behind an overlay whose upper layer is a tmpfs of this size. The corpus stays read-only, the run's writes go to memory, and a directory's writes are dropped when its last case finishes |
| `status_http` | off | serve a progress page. `on` takes `127.0.0.1:51523`; a bare port takes every interface |
| `case_plan` | off | a file of per-case durations. The run reads it to hand the longest cases out first and writes it back from what it measured |
| `case_sizes` | off | a file of per-directory footprints in MB — how much each case directory was holding when its last case finished. Written from what the run measured, and read by `lane_slow_mb` |
| `case_patch_dir` | off | a tree of per-case patches, mirroring the corpus. Where one exists it is applied into the run's overlay before the case runs, and the run says so on standard output, in the case log, and on the page. The corpus on disk is unchanged. See [`patches/README.md`](patches/README.md) |
| `lane_slow_secs` | off | split the slots into a fast lane whose writes go to memory and a slow lane whose writes go to disk, assigning directories by duration |
| `lane_slow_mb` | off | the same split, assigning directories by footprint instead. Needs `case_sizes`. Set with `lane_slow_secs` to get the union of the two |

Namespaces are what make a slot cost nothing to configure, and they need the run to be contained:

```bash
TESTKIT_CONTAIN=1 TESTKIT_NATIVE_SHELL=1 testkit shell -c shell.conf
```

`TESTKIT_SLOT_ROOT` says where the per-slot overlays go, `/var/tmp/testkit-slots/<pid>` by default.

**Two runs at once are refused**, and only for one reason: they would share
`<CTP_HOME>/result/<category>`, which holds one `feedback.log` and one
`test_status.data` between them, so the result would describe neither and nothing
about it would say so. Everything else a run makes is already its own — the
corpus overlay, the slot namespaces, `CUBRID_TMP`, and the status port, which
moves along when the default is taken. So a second run on the same tree stops
before it has done anything, naming the run that has it; give it a different
`CTP_HOME` and the two coexist.

**How many slots, and how big a ceiling.** `tools/sizing.sh` answers both from the machine and the
engine's own configuration, and says what every number rests on:

```bash
CUBRID=/path/to/install tools/sizing.sh
```

The ceiling is not a reservation. A tmpfs occupies what is written to it and nothing more, so a
generous one costs nothing until it is used; its job is to turn a case that never cleans up from a
run that dies into a case that fails for want of space. The run reports its high-water mark on the
way out and says the verdicts are unusable if it got within 10% of the ceiling — because a case
that runs out of space fails like a case that got the wrong answer, and nothing else in the output
would say so.

### What it is worth, measured

`_01_utility` on develop — 217 cases, 3,189 case-seconds of work, longest case 195 s:

| | wall clock | verdicts |
|---|---:|---|
| 8 slots | 407 s | the baseline |
| 8 slots, longest first (`case_plan`) | **391 s** | **identical, case for case** |
| 8 slots, lanes at 30 s | 696 s | identical |

Two things that table is for. **The scheduler is already at its floor**: every arm lands within 1-2%
of `total work ÷ slots`, so the wall clock is decided by how much work there is and how many slots,
not by how the work is ordered. Ordering pays from about sixteen slots up, where the longest case
becomes the wall.

And **lanes lose here**, which is the opposite of what the design predicted. Splitting a pool costs
flexibility, and the criterion was wrong: duration selects the I/O-heavy cases, which are exactly
the ones memory helps most. Measured per case, moving the slow lane to disk cost 1.55× at three
slots and 1.93× at five — `_16_restoredb/cbrd_24892` went 67.5 s to 236.1, while
`_03_start_server/itrack_10003`, which spends its 115 seconds waiting rather than writing, cost
nothing at all. Lanes are for a machine where memory is the binding constraint; on this one it is
not. The reasoning is in [`concept/beyond-axis.md`](docs/concept/beyond-axis.md) B-T12 and B-T13,
including the numbers that contradict it.

### What a slot is

A slot is not a process or a thread. It is a set of namespaces held open for the whole run, with a
worker pulling cases through it:

```
  runner (its own user, mount, PID and IPC namespaces)
    │
    ├── the corpus, read-only, behind an overlay whose upper layer is a tmpfs
    │
    ├── slot0 ── namespaces: mount · PID · IPC · NET
    │             ├── $CUBRID          an overlay of its own — writable, nothing copied
    │             ├── $CUBRID_DATABASES  likewise: the registry is per slot
    │             ├── /dev/shm         its own, so POSIX segments do not collide
    │             ├── CUBRID_TMP       $CUBRID/tmp, so the master sockets do not collide
    │             ├── 192.0.2.1        an address, so `hostname -I` answers
    │             └── port 1523        its own, because the network namespace says so
    │
    ├── slot1 ── the same, and it cannot see any of slot0's
    ⋮
    └── slotN
```

**The network namespace is what makes a slot free.** Two slots would otherwise collide on the master
port, the two broker ports, and whatever else a case starts. A namespace gives each of them the whole
port space, so every slot runs on the shipped 1523 and **no configuration is rewritten** — which is
the point, because a slotted run's conf files and log lines then stay byte-identical to a serial
run's, and that is the evidence this project has to produce.

The PID namespace matters for a different reason: CTP's reset selects processes with `ps`, and
inside a slot `ps -e` *is* the slot. Without it a worker's reset kills every other slot's server —
measured, and it is why the first slot cannot be left outside.

Two things a slot does not get, on purpose. It shares `/tmp`, because cases are entitled to, and it
shares the machine's hostname, because 126 cases read it.

**And four things it has to be given, because the isolation takes them away.** Each was found the
same way — a failure that looked like the case's and was the runner's — and each is measured:

| what | why | measured |
|---|---|---|
| an address on a dummy interface | `hostname -I` reports every interface *except* loopback, so in a fresh network namespace it answers with an empty string. 108 case scripts start `hostip=$(hostname -I \| awk '{print $1}')` and then connect to it, register a dblink server, or hand the empty string to a utility that says `Incorrect hostname format` | 6 failures → 0 |
| `TAR_OPTIONS=--no-same-owner` | a user namespace maps one uid, so `tar -x` cannot restore an archive's recorded ownership: it extracts the files and exits 2. 51 case scripts unpack something, and the exit status is what fails them | exit 2 → exit 0 |
| a linker that keeps what the command line names | not the isolation but the distribution: `--as-needed` is the default here and not on the CI image, and CTP's `xgcc` puts `-lcascci` *before* the object that needs it. 173 case scripts compile C | 112 failures → 2 |
| `$CUBRID_TMP` at the shipped path | it was on `/var/tmp/tk<pid>/<slot>`, which is per-slot and short but is not where the engine puts it — and cases compare output holding a socket path against an answer written as `${CUBRID}/...` | 1 failure → 0 |

`192.0.2.0/24` is TEST-NET-1, reserved by RFC 5737 for documentation, so it cannot be mistaken for a
real host anywhere a case records it. Every slot gets the same address for the same reason every slot
keeps port 1523: a slot should look like a machine, and they are machines that cannot see each other.

**The registry has a shared floor.** `$CUBRID_DATABASES` is an overlay like `$CUBRID`, so a slot's
*writes* are its own — which matters more than it sounds: the corpus makes 4,056 `createdb` calls
using 115 distinct names, and `testdb` alone is used by 74 different cases. What the overlay cannot
isolate is the lower layer, the directory as the machine left it, so an entry an earlier run left
behind is in every slot at once. `make_tz -g extend` walks every name in `databases.txt`, so one
stale entry failed all 38 timezone cases. The run says so before the first case, and the reclaim now
takes a directory's entries with it when it drops the directory.

**A queue, not a partition.** All slots pull from one queue, so a slot that draws short cases keeps
drawing; nothing is assigned up front. With durations from an earlier run the queue hands out the
longest first, which is the same schedule as partitioning by rank beforehand without having to know
how many slots there are.

**And the corpus is shared but not written.** One overlay for the whole run, with the writes going to
memory and a directory's writes dropped when its last case finishes. That is why fifteen directories
holding more than one case matter: with lanes the overlay becomes per slot, and then the queue has
to keep a directory's cases together. Without lanes there is one overlay and it does not arise.

### How a case is chosen

The queue's order is the schedule -- longest first, from `case_plan` -- and it is
right about makespan. What it does not know is that a slot is not free just
because it is idle: the run shares a memory ceiling and a machine.

Measured at 24 slots on the full corpus: the tmpfs went from empty to
**25,584 MB of 25,600 in under three minutes, with zero cases finished**. Nothing
was wrong with the order. The order puts the heaviest cases first, so every slot
started one at once, and nobody asked whether the machine could take another yet.

So the order proposes and an **admission policy** disposes
([`internal/dispatch/policy.go`](internal/dispatch/policy.go)). The queue offers
its candidates in order and hands out the first the policy admits; a run without
a policy gets the order alone. A policy is never asked to admit the only case --
one that can refuse everything is one that can stop the run.

Two are implemented, and they answer different halves of that failure:

| | what it does | why it cannot be the other one |
|---|---|---|
| `heavy_in_flight_max` | at most N of the heaviest tenth run at once. Default `slots/4` | this is the rule for the **start**. A ceiling is empty when the first case begins, so a rule that watches the ceiling admits every slot at once |
| `scenario_ram_high_water` | nothing new starts while the corpus tmpfs is over N% full. Default 80 | feedback, not prediction. It is late by construction -- it can only stop the next case, never the ones already running |

**They are not the same kind of rule, and treating them alike cost half an hour.**
The ceiling is a **constraint**: crossing it fails cases, so it never yields. The
heavy cap is a **preference about order**: it exists so that N slots do not start
N heavy cases at once, and at the tail — where every case left is heavy — holding
to it means idling slots through exactly the long tail that ordering
longest-first exists to prevent. Measured: a 24-slot run finished its last 150
cases six at a time with eighteen slots idle.

So the queue scans for a case both rules allow; failing that, for one the
constraint allows. The second scan only ever finds something at the tail, because
while ordinary work remains the first scan finds it.

Feedback rather than prediction is deliberate. The run does measure per-directory
footprints, and the twenty-four cases at the head of that queue measured 377 MB
between them while actually filling 25,584. `case_sizes` records what a directory
*held when it retired*; a ceiling is about what it *peaked at while running*, and
those are not the same number.

### What a lane is

A lane is a block of slots whose corpus writes go to the same place: the **fast** lane's to the
tmpfs, the **slow** lane's to disk. Off by default — one overlay, every slot in memory.

The idea in one sentence: a case that cannot get speed out of memory has no business occupying it.
The ceiling is finite, and a directory that holds gigabytes while gaining nothing from being in RAM
is spending the ceiling on behalf of the cases that would.

**Which cases those are is the part that took two attempts.** `lane_slow_secs` picks them by
duration, and it lost its own measurement — 696 s against a predicted 440. Duration is answering two
questions with one number: for *which case should a free slot take next* it is right, and makespan
proves it; for *which cases should not be given memory* it is a proxy, and an anti-correlated one,
because the long cases are the I/O-heavy ones and those are what memory helps most.

`lane_slow_mb` picks them by footprint instead, from what `case_sizes` measured. What the ceiling
counts is megabytes, so megabytes are what the lane is chosen by. Set both and a directory goes to
the slow lane for either reason.

Slots are divided in proportion to the case-seconds each lane holds, so the two lanes finish
together. Both lanes get at least one slot, and a split that would starve one is refused rather than
made.

**Lanes are for a machine where memory is the binding constraint.** On the machine this was measured
on it is not, and the fast lane alone wins — see the table above.

### Running it on your own machine

```bash
# once, to see what this machine can do
CUBRID=/path/to/install tools/sizing.sh

# then
TESTKIT_CONTAIN=1 TESTKIT_NATIVE_SHELL=1 testkit shell -c shell.conf
```

with, in `shell.conf`:

```
parallel_slots=8
scenario_ram_mb=14336
case_plan=/path/to/plan          # written on the first run, read on the next
case_sizes=/path/to/sizes        # likewise, and what lane_slow_mb thresholds
status_http=on
```

No root, no container runtime, no configuration of the machine: the namespaces are unprivileged and
the overlay is unprivileged, so this is a normal user running a normal binary. What it needs is
`TESTKIT_CONTAIN=1` — without it there are no namespaces and slots refuse rather than colliding
silently.

**Where the numbers come from.** `tools/sizing.sh` reads the machine and the engine's own conf and
says what bounds the slot count — memory, cores, disk — and what every figure rests on. It is
deliberately arguable: an operator who disagrees with a number can see which measurement it came
from.

## What it does

![Architecture: frozen entry scripts call one Go binary, which routes each task either to a native runner or to the old CTP as a subprocess; both produce the same frozen output, and QA operations are excluded](docs/assets/architecture.svg)

![Animated: one binary and one registry lookup. unittest goes to the native runner; shell goes to CTP as a subprocess while TESTKIT_NATIVE_SHELL is unset and to the native runner when it is set to 1. Both paths write the same frozen output, so from outside there is no way to tell which ran.](docs/assets/anim-dispatch.svg)

One binary. Every entry script becomes a shim that hands its arguments over unchanged, and a single
registry turns a task name into either a native runner or the legacy runner, which reproduces CTP's
argv and environment byte for byte. Both paths write the same output, because the surface belongs to
the output layer rather than to whichever implementation ran.

There is no jar-compatibility layer: old modules keep their own jars, which keep being built, until
their turn comes. [`design/architecture.md`](docs/design/architecture.md) and
[`design/contracts.md`](docs/design/contracts.md) are the specifications.

## The frozen surface

[`concept/external-surface-freeze.md`](docs/concept/external-surface-freeze.md) is normative, and
everything in it carries a grade you can rely on:

| | |
|---|---|
| **F1** | byte-identical. Something out there greps this |
| **F2** | same meaning; order, spacing and extra detail may differ |
| **F3** | must be accepted as input; the internal representation is free |
| **NF** | not frozen |
| **unsettled** | no grade yet, with the date that gets resolved |

Some of it is deliberately ugly and stays that way. `main.info` separates with `:` and
`summary_info` with `=`; both are F1 and will not be harmonised. `-1` is returned as an exit code
and reaches the shell as 255; it will not be tidied into `1`.

**The freeze preserves what CTP did, not what CTP got wrong.** Where a behaviour is plainly
unintended and reproducing it would make someone trust something false, it is fixed and the decision
recorded. Three so far: the requirements check now actually fails on a missing command — it used to
match csh's wording and so reported `PASS` for everything absent; `dos2unix` is off the required
list, because nothing in the corpus needs it; and a build with no commit suffix is now just its
version instead of `11.2.0.0000) (64bit release build for linux_gnu`.

## Status

| Phase | | |
|---|---|---|
| 0 — analysis | **done** | 39 documents on what CTP actually does |
| 1 — concept and freeze | **done** | north star, the freeze spec with a 24-row old↔new mapping, non-goals NG1–NG11, migration exclusions |
| 2 — architecture | **done** | architecture, five contracts, four module documents |
| **3 — rewrite `shell`** | **in progress** | `unittest` native; `shell` end-to-end and matching CTP's verdicts on four real cases; `run-shell` complete, with six axis-T options. The harness that compares a corpus is built; the gate is running it. Parallel slots, a corpus that cleans itself, per-case durations and a progress page are in and measured |
| 4 — the rest | — | `sql` family, `isolation`, `ha_repl`, `cdc_repl`, `jdbc` |
| 5 — retire | — | isolate what is no longer called; decide what to keep |

![Animated: CTP and testkit run on the same four cases on the same machine in the same minute. Both report three passed and one failed, the same case failing on both, and exit 0. Of the ten files a run leaves behind, six are byte-identical after normalisation, one is identical once the JVM's own properties are removed, and three differ by a named number of lines. Ten of those lines are still unexplained, and the largest difference turned out to be a defect in the new runner rather than a difference between the two.](docs/assets/anim-equivalence.svg)

**What is proven.** CTP and testkit, run on the same four cases on the same machine in the same
minute: both **3 passed, 1 failed**, the same case failing on both, the same summary counters, exit
code 0. Of the ten files a run leaves behind, **six are byte-identical** after normalisation —
including the two that carry the verdicts — one more is identical once the JVM's own system
properties are removed, and the remaining three differ by a named number of lines
([`evidence/regression-shell.md`](docs/evidence/regression-shell.md)).

Those counts were taken again on 2026-09-07 and most of them moved, because the largest difference
turned out to be **this runner's own defect** — a failing case wrote its console output to the
worker log twice — and `test_local.log` fell from 650 lines to 154 when it was fixed. Ten lines are
still unexplained, and saying so is the point: the page previously had one unexplained difference
and it was the wrong one.

**The gate.** Equivalence is proven by comparing normalised output over the whole shell corpus —
3,475 cases, with the 195 in `_25_unstable` counted separately because their own readme says they
depend on machine load and elapsed time ([ADR-013](docs/adr/ADR-013-regression-equivalence.md)).
`TESTKIT_NATIVE_SHELL` comes off when that clears. That is 64 shards and tens of hours on each
runner, so it is run by [`evidence/compare/`](docs/evidence/compare/README.md) rather than by hand:
a shard is the unit of resume, the six files that carry verdicts are held to zero differences with
no baseline allowed, and everything else is classified — so that a difference nobody has seen
before is the only thing on the page.

![Animated: how the whole shell corpus is compared. 3,452 cases are cut into 64 shards, with _06_issues holding half of everything and split into sub-shards; every case lands in exactly one shard. One shard runs on CTP and on testkit, both result trees are kept, and a shard whose report already ends in COMPLETE is skipped, so the loop survives being killed. In the report the six files that carry verdicts must be identical with no baseline allowed, and the other four are classified: on the four-case calibration 154 differences became 143 known and 11 new, and ten of the eleven are one finding.](docs/assets/anim-compare.svg)

**Corrections are recorded where the mistake was made.** The CLI survey that justified the project
covered one entry point out of fifteen; the freeze specification was wrong in eight further places;
the corpus counts were 3,722 and 204 until the discovery rule was fixed; and the one difference the
first comparison could not explain was blamed on the environment when it belonged to this runner
([`evidence/spec-corrections.md`](docs/evidence/spec-corrections.md)).

## Layout

```
CONTEXT.md               the glossary — task ≠ suite ≠ module ≠ runner
cmd/testkit/             the entry point
internal/                cli · conf · registry · dispatch · runner (legacy, shellsuite) ·
                         runshell · exec · result · feedback · topology ·
                         contain (namespaces per slot) · plan (case durations) ·
                         status (the progress page)
tools/sizing.sh          how many slots and how big a ceiling, for this machine
ext/cubrid-sqlancer/     submodule — a SQLancer provider for CUBRID
docs/
  ROADMAP.md             phases, exit conditions, risks, the re-evaluation gate
  adr/                   decisions, numbered; README.md is the numbering authority
  concept/               north star, the freeze, non-goals, exclusions
  design/                architecture, contracts, one doc per module
  analysis/              what CTP actually does, measured
  evidence/              regression evidence, the normaliser, and the corrections
    compare/             the harness that runs the corpus and classifies what differs
  extensions/            testing axes CTP never had
  survey/                the DBMS testing ecosystem, classified into eight axes
```

## Where to start reading

| If you want | Read |
|---|---|
| the vocabulary | [`CONTEXT.md`](CONTEXT.md) |
| what this system is for | [`concept/north-star.md`](docs/concept/north-star.md) |
| what may never change | [`concept/external-surface-freeze.md`](docs/concept/external-surface-freeze.md) |
| what was left out, and why | [`concept/migration-exclusions.md`](docs/concept/migration-exclusions.md) |
| how it is built | [`design/architecture.md`](docs/design/architecture.md) · [`design/contracts.md`](docs/design/contracts.md) |
| how equivalence is decided, and run | [`adr/ADR-013`](docs/adr/ADR-013-regression-equivalence.md) · [`evidence/compare/`](docs/evidence/compare/README.md) |
| how a run is made parallel | [`concept/beyond-axis.md`](docs/concept/beyond-axis.md) B-T3, B-T12, B-T13 |
| where the run's hours go, and what to do next | [`concept/beyond-axis.md`](docs/concept/beyond-axis.md) B-T14 |
| what the old system could fix cheaply | [`evidence/ctp-improvements.md`](docs/evidence/ctp-improvements.md) |
| what happens next | [`ROADMAP.md`](docs/ROADMAP.md) · [`design/module-shell.md`](docs/design/module-shell.md) |
| every decision so far | [`adr/README.md`](docs/adr/README.md) |

New documents are in English. The Phase 0–2 documents under `docs/` are in Korean and stay that
way — a freeze specification is worth exactly what its sentences are worth, and re-writing six
thousand lines of analysis buys nothing but a chance to introduce errors.
