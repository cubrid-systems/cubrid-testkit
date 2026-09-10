# The `shell` category

`shell` is CTP's largest test category and the first one testkit rewrote. This is the as-built
guide: what the runner is made of, what each setting does and costs, how to run it on a host and in
Docker, and what to set.

The pre-implementation design is [`../../design/module-shell.md`](../../design/module-shell.md).

---

## 1. What a run does

```
  check the machine      once, so a machine that cannot run cases says so here
        |                rather than 3,444 times
        v
  prepare the corpus     read-only lower layer; every write goes to an overlay
        v
  find the cases         <scenario>/**/<name>/cases/<name>.sh, then the exclusions
        v
  order them             longest first, from case_plan, if an earlier run left one
        v
  deploy                 the helpers CTP's cases source, and the snapshot every
        v                case is restored from
  run                    N slots pull from one queue -- for each case:
        |                  reset -> patch if one exists -> run -> check for new
        v                  errors -> read its .result -> record the verdict
  retries                only when nothing else is left to start
        v
  write everything down  the frozen files, plus the durations and footprints this
                         run measured, for the next one
```

## 2. Module structure

Four packages. Only `shellsuite` is shell-specific; the other three are general and the later
categories will use them.

### `internal/runner/shellsuite` — the category

| file | what it owns |
|---|---|
| `shell.go` | the run: reads the conf, wires everything below, owns the stage order above |
| `discover.go` | what counts as a case, and the two exclusion mechanisms |
| `corpus.go` | the scenario tree as a run sees it, and the watcher that samples each directory's peak footprint |
| `deploy.go` | what must be true before the first case, and the snapshot each case is restored from |
| `worker.go` | one slot: pull, reset, run, judge, repeat |
| `case.go` | what a case needs from the run that is not the case itself |
| `prologue.go` | the preamble every case script is given |
| `check.go` | the machine check — variables, commands, directories |
| `monitor.go` | how often a worker is looked at, and what counts as stuck |
| `patch.go` | per-case corpus changes the run needs and does not own |
| `registry.go` | `$CUBRID_DATABASES` isolation inside the overlay |
| `lanes.go` | the split between a memory lane and a disk lane |
| `safepath.go` | refuses a delete whose root is empty or `/` |
| `setup.go` | what the run was asked for, so the page can show it |

### `internal/contain` — one slot's isolation

| file | what it owns |
|---|---|
| `contain.go` | `Enter` re-executes the binary in new user, mount, PID and IPC namespaces; `Init` makes PID 1 a reaper that only reaps; `Setup` binds a bash-compatible `/bin/sh` |
| `slot.go` | the per-slot overlay over `$CUBRID`, the scenario and the registry |
| `linker.go` | the shim that keeps a slot's view of the install consistent |

Two properties of this package decide most of §5 and §6: **everything runs inside the namespace**,
because `Enter` is the first thing `main` does, and **the namespace maps exactly one uid**.

### `internal/dispatch` — which case a free slot may start

`dispatch.go` is the queue (longest-first, per-directory ownership, lane assignment). `policy.go` is
admission: `Headroom` refuses new cases above a fraction of the tmpfs, `HeavyCap` refuses more than
*n* of the heaviest at once.

The order proposes and the policy disposes. The order alone is right about makespan and wrong about
everything else: at 24 slots it starts 24 of the heaviest cases at once.

### `internal/status` — the progress page

The board and its JSON, the page, a running case's live output, a finished case's block, and replay
of a finished run's `feedback.log`.

---

## 3. Configuration

Keys in the conf file `-c` points at. The first group is CTP's and behaves as it always did.

### CTP's keys

| key | default | |
|---|---|---|
| `scenario` | — | the corpus root. Required |
| `test_category` | — | `shell` |
| `testcase_retry_num` | `0` | attempts after the first failure |
| `testcase_timeout_in_secs` | `0` | 0 is no timeout. CI uses 720 |
| `testcase_exclude_by_macro` | — | skip a case whose text contains this |
| `testcase_exclude_from_file` | — | files of path fragments to skip, comma-separated |
| `test_continue_yn` | `false` | resume, skipping what already has a verdict |
| `cubrid_db_charset` | `en_US` | passed to every `createdb` |
| `feedback_type` | — | `file` |

### This runner's keys

| key | default | impact | |
|---|---|---|---|
| `parallel_slots` | `1` | **large** | how many cases run at once. Each slot gets its own PID, IPC, mount and network namespace, so every slot keeps the shipped port 1523 and nothing is reconfigured |
| `scenario_ram_mb` | off | **large; can fail a run** | the corpus overlay's upper layer becomes a tmpfs of this size |
| `scenario_ram_high_water` | `80` | protective | percent of the ceiling above which no new case starts |
| `heavy_in_flight_max` | `slots/4` | protective | how many of the heaviest cases may run at once |
| `case_plan` | off | small; grows with slots | per-case durations. Read to order the run, written from what it measured |
| `case_sizes` | off | none directly | per-directory peak footprints, read by the lane keys |
| `case_patch_dir` | off | none | per-case patches applied into the overlay; the corpus on disk is unchanged |
| `lane_slow_secs` | off | **negative** | split the slots into a memory lane and a disk lane by duration |
| `lane_slow_mb` | off | situational | the same split, by footprint |
| `lane_slow_mbps` | off | situational | the same split, by write rate |
| `status_http` | off | none | the progress page. `on` is `127.0.0.1:51523`; a bare port takes every interface |

### Environment

| | |
|---|---|
| `TESTKIT_NATIVE_SHELL=1` | run `shell` here rather than handing it to CTP. The opt-in gate |
| `TESTKIT_CONTAIN=1` | put the run in namespaces of its own. **Required** by slots and by `scenario_ram_mb` |
| `TESTKIT_CONTAIN_SH` | which shell to bind over `/bin/sh` |
| `TESTKIT_SLOT_ROOT` | where per-slot overlays go |

CTP's own environment still applies, and **`$CUBRID_DATABASES` must be inside `$CUBRID`** — CTP's
per-case reset cleans exactly that path, and pointing it elsewhere leaves the reset scrubbing a
directory nothing uses.

---

## 4. What each setting is worth

**`parallel_slots` is the only large lever.** Every configuration lands within 1-2% of
`total work / slots`: the scheduler is already at its floor, so the wall clock is decided by how much
work there is and how many slots, not by how the work is ordered. The corpus is 3,444 cases and 22.9
hours of work, with a longest case of 27 minutes that no slot count gets under:

| slots | floor |
|---:|---:|
| 8 | 2.9 h |
| 16 | 1.4 h |
| 24 | 1.0 h |

**`case_plan` pays from about sixteen slots up**, where the longest case becomes the wall; below
that the floor is `work / slots` and order cannot move it. Leave it on regardless — it costs
nothing, and the progress page's "remaining" is only trustworthy with a plan.

**`scenario_ram_mb` is worth sustained bandwidth, not startup.** On disk the corpus demands about
250 MB/s and the disk delivers 88 under four concurrent writers; that gap is the whole win, and it
is also why four slots was the bound before memory was used.

**Lanes lose on a machine where memory is not the binding constraint**, which was the machine they
were measured on. `lane_slow_secs` selects by duration and duration is anti-correlated with what
memory helps, so it cost 1.55-1.93x per case. Footprint and duration turned out to be unrelated, so
`lane_slow_mb` picks the write rate at random; `lane_slow_mbps` is the only criterion that selects
what the disk is actually bounded by. **Leave all three off** unless you have measured that memory
binds on your machine.

**What is not a lever.** `max_clients` and `data_buffer_size` do not buy a slot — a server has a
157 MB floor no parameter reaches. And 44% of the corpus's wall clock is literal `sleep` in the case
scripts, which no setting here touches. The harness's own overhead is 2%.

---

## 5. The ceiling, and how a run dies at it

`scenario_ram_mb` is a ceiling, not a reservation: a tmpfs occupies what is written to it, so a
generous one costs nothing until it is used. Its job is to turn *a case that never cleans up kills
the machine* into *a case fails for want of space*.

It fails in three ways depending on how it is sized, and they look nothing alike:

| ceiling | what happens |
|---|---|
| too large for RAM | the run plus its servers exceed memory and the **OOM killer** takes it, with nothing in the run's log to say why |
| too small | cases fail **ENOSPC**, and a case that ran out of space fails exactly like a case that got the wrong answer |
| just under | survives with no margin |

**The gate does not save you.** `scenario_ram_high_water` holds back *new* cases; the ones already
running fill the rest, and they do. The gate is late by construction, which is why
`heavy_in_flight_max` exists beside it.

Two things follow:

1. **The ceiling plus the servers must fit in RAM.** The run warns when
   `scenario_ram_mb + slots * 175 MB` exceeds `MemAvailable`. 175 MB a slot is a reserve, not a
   prediction — a case's own working set is larger and is not knowable in advance.
2. **A run that got within 10% of its ceiling declares its verdicts unusable**, because nothing else
   in the output separates ENOSPC from a wrong answer.

`tools/sizing.sh` sizes both from the machine and the engine's own configuration, and says what each
number rests on:

```bash
CUBRID=/path/to/install tools/sizing.sh
```

Sizing can also come out negative: on a 30 GB machine no ceiling fits 24 slots. That is a finding,
not something to work around — the honest options are fewer slots or more RAM.

### `cubrid.conf` is not verdict-neutral

Lowering `db_volume_size` and `log_volume_size` is what makes many slots fit in a tmpfs — at the
shipped 512 MB, eight slots hold 8 GB of volumes before a case writes a row. But some cases assert
the shipped value, and lowering it makes them measure the run instead of the engine.

Two classes, and only one is findable by reading:

- A case that **prints the parameter** is affected only if it pins the current value to the shipped
  default. The dump reads `name=current (engine default)` and only the unparenthesised side is
  settable, so cases that pass their own size to `createdb` or set it with `change_db_parameter` are
  immune. Corpus-wide this class is one case.
- A case that **never names a size** but depends on the behaviour — counting temporary volumes, for
  instance — is invisible to any search and is found only by running the corpus at both values.

Where a case needs the shipped value, the fix is a patch restoring it for that case alone;
`case_patch_dir` applies it into the overlay and the corpus on disk is unchanged.

---

## 6. Running it

Both ways run the same binary with the same conf. Only what the environment brings differs.

### On a host machine

```bash
CUBRID=/path/to/install tools/sizing.sh          # once, to size the run

TESTKIT_CONTAIN=1 TESTKIT_NATIVE_SHELL=1 testkit shell -c shell.conf
```

**No root, no container runtime, no change to the machine** — the namespaces and the overlay are
both unprivileged, so this is a normal user running a normal binary. Without `TESTKIT_CONTAIN=1`
there are no namespaces and slots refuse rather than colliding silently.

The host needs `$CUBRID`, `$CUBRID_DATABASES` inside it, `$CTP_HOME`, `$JAVA_HOME`, a scenario tree
that is not itself a mount point, and a slot root this user owns.

### In Docker

```bash
docker run --rm \
  --security-opt seccomp=unconfined \      # to make namespaces
  --security-opt systempaths=unconfined \  # to remount /proc
  -p 51523:51523 \                         # to watch the page
  -v /host/work:/home \
  -e TESTKIT_CONTAIN=1 -e TESTKIT_NATIVE_SHELL=1 \
  -e TESTKIT_SLOT_ROOT=/home/slots/run \
  <image> testkit shell -c /home/shell.conf
```

`--privileged` is **not** needed. The two `--security-opt` flags are, and each has a symptom that
names nothing on its own:

| symptom | flag |
|---|---|
| the namespace cannot be created | `seccomp=unconfined` |
| `mount /proc: operation not permitted` | `systempaths=unconfined` |

Three things bite in a container and not on a host:

**A bind-mounted scenario is a mount point, and an overlay's lower layer cannot be one.** `-v` makes
a mount point of any path it is given, and overlayfs refuses it with the one message it has for
every refusal. Mount the **parent** and let the scenario be an ordinary directory inside it. The
same applies to `$CUBRID`, the registry, and the slot root's parent — overlay upper and work
directories cannot sit on the container's own overlayfs either.

**Ownership decides everything, because the namespace maps one uid.** A bind mount carries the
host's uids in, so a tree owned by uid 1000 is `nobody` inside a namespace that maps only uid 0, and
the failure surfaces as `permission denied` from whatever writes first. **Give the whole work tree
one owner and run the container as that uid.**

**Results persist between runs.** `feedback.log` and the rest live under
`$CTP_HOME/result/<category>/current_runtime_logs`; if `$CTP_HOME` is a bind mount, a second run
appends to the first run's files and reading them afterwards mixes two runs. Clear the result tree
between runs, or give each run its own `CTP_HOME`.

---

## 7. Recommended settings

### A developer's machine, part of the corpus

```
parallel_slots=4
case_plan=/path/to/plan
status_http=on
```

No tmpfs: at four slots the disk is not yet the bound, and a ceiling you have not sized is a way to
fail a run rather than a way to speed one up.

### One machine, the whole corpus

Sized for a 30 GB machine — run `tools/sizing.sh` before trusting these on another.

```
parallel_slots=16
scenario_ram_mb=18432
scenario_ram_high_water=80
case_plan=/path/to/plan          # written on the first run, read on the next
case_sizes=/path/to/sizes        # likewise
case_patch_dir=/path/to/patches/shell
testcase_retry_num=0
status_http=on
```

and in the engine's `cubrid.conf`:

```
db_volume_size=20M
log_volume_size=20M
data_buffer_size=64M
log_buffer_size=4M
```

16 slots is where `case_plan` starts paying and is still under the point where the disk binds; the
volume sizes are what make the slots fit; the patch directory is what keeps the cases that assert
the shipped sizes honest. Leave lanes off.

### CI

The same, plus `testcase_retry_num=0` so a flaky case stays visible rather than being retried into a
pass, a `case_plan` carried from the last green run, and `status_http` on a published port.

---

## 8. Further reading

| | |
|---|---|
| the pre-implementation design | [`../../design/module-shell.md`](../../design/module-shell.md) |
| what may never change in the output | [`../../concept/external-surface-freeze.md`](../../concept/external-surface-freeze.md) |
| how parallelism was reasoned about | [`../../concept/beyond-axis.md`](../../concept/beyond-axis.md) B-T3, B-T12, B-T13 |
| per-case patches | [`../../../patches/README.md`](../../../patches/README.md) |
| exclusion lists | [`../../../exclusions/README.md`](../../../exclusions/README.md) |
| how equivalence with CTP is decided | [`../../adr/ADR-013-regression-equivalence.md`](../../adr/ADR-013-regression-equivalence.md) |
