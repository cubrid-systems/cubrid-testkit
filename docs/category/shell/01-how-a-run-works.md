# 1. How a run works

[← back to the shell category](README.md)

- [The stages](#the-stages)
- [What a slot is](#what-a-slot-is)
- [Which case a free slot takes](#which-case-a-free-slot-takes)
- [Module structure](#module-structure)

## The stages

```
  check the machine      once, so a machine that cannot run cases says so here
        |                rather than 3,444 times
        v
  prepare the corpus     read-only lower layer; every write goes to an overlay
        v
  find the cases         <scenario>/**/<name>/cases/<name>.sh, then the exclusions;
        v                a directory that cannot be read is a [WARN], not a stop
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

## What a slot is

A slot is one case at a time in namespaces of its own. This is what makes several slots cost nothing
to configure: every slot believes it is the only CUBRID on the machine.

```
   the machine                       slot 0                    slot 1
   ───────────                       ──────                    ──────
   $CUBRID  (real install)  ──┐   overlay: writes go up     overlay: writes go up
                              ├──►  lower = the install       lower = the install
   scenario (repository)    ──┤     upper = tmpfs or disk     upper = tmpfs or disk
                              │
   $CUBRID_DATABASES        ──┘   port 1523  (own netns)    port 1523  (own netns)
                                  PID 1 = its own init      PID 1 = its own init
                                  its own IPC, /dev/shm     its own IPC, /dev/shm
                                  /tmp is the machine's     /tmp is the machine's
```

Four namespaces per slot:

| namespace | what it buys |
|---|---|
| **network** | every slot keeps the shipped port 1523. No conf is rewritten, no port is allocated |
| **PID** | `pkill cub` in a case kills that slot's servers and nothing else |
| **IPC** | broker shared memory does not collide between slots |
| **mount** | the overlay, and a bash-compatible `/bin/sh` bound over the machine's |

**`/tmp` is not separated.** Slots did collide there, on `cub_master`'s socket `/tmp/CUBRID1523`:
four masters over one socket, and none of them came up. That socket now goes to `CUBRID_TMP`, which
each slot sets to a directory of its own: `$CUBRID/tmp`, behind the slot's overlay. A case that
writes a fixed name under `/tmp` still shares it with every other slot and with the machine.

The overlay is what keeps the corpus clean: **the scenario on disk is never written to**. A case's
writes — its databases, its logs, its `.result` — land in the upper layer, and when a directory's
last case finishes they are dropped.

Namespaces need `TESTKIT_CONTAIN=1`. Without it there are none, and slots refuse to start rather
than colliding silently.

## Which case a free slot takes

The queue orders cases longest-first, which is right about makespan and wrong about everything else:
at 24 slots it starts 24 of the heaviest cases at once. So the order proposes and a policy disposes.

```
   queue (longest first)          policy                    slot
   ────────────────────           ──────                    ────
   case A  (523 s, 9 GB)  ──►  HeavyCap: already n          A starts
   case B  (490 s, 8 GB)  ──►    heavy in flight?    ──►    B waits
   case C  (17 s, 40 MB)  ──►  Headroom: tmpfs over         C starts
   …                             high_water?
```

| policy | refuses |
|---|---|
| `HeavyCap` | more than `heavy_in_flight_max` of the heaviest cases at once. It is about the *start*, where a feedback rule cannot help because nothing has been written yet |
| `Headroom` | anything new while the tmpfs is above `scenario_ram_high_water` percent. It is about everything after that, where the only trustworthy number is the one the machine reports |

Neither is ever asked to refuse the *only* case: when nothing is running the queue hands one out
regardless, because a policy that can refuse everything is a policy that can stop the run.

## Module structure

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

Two properties of this package decide most of [Running it](03-running-it.md) and
[The ceiling](05-the-ceiling.md):

- **Everything runs inside the namespace**, because `Enter` is the first thing `main` does.
- **The namespace maps exactly one uid**, which is why file ownership matters so much in Docker.

### `internal/dispatch` — the queue and the policies

`dispatch.go` is the queue; `policy.go` is `HeavyCap` and `Headroom`, above.

### `internal/status` — the progress page

The board and its JSON, the page, a running case's live output, a finished case's block, and replay
of a finished run's `feedback.log`.
