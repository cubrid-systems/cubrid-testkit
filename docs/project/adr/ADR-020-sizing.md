# ADR-020: A run sizes itself, from what runs on this machine measured

- **Date:** 2026-09-16
- **Status:** Proposed — implemented for isolation, sql, medium and shell
- **Related:** ADR-014 (the runner's scope is one machine), ADR-018 (parallel slots are isolation's default),
  `scripts/sizing.sh`, `evidence/isolation-controller.md` §8, `evidence/isolation-baseline.md` §2,
  `evidence/sql-native.md` §3, `category/sql/05-slots-and-speed.md`, `category/shell/04-configuration.md`

## Context

How many cases a run does at once was decided in three places that did not agree.

| | what it did |
|---|---|
| `scripts/sizing.sh`, 458 lines | said what this machine could run, for `shell` and `sql`, and printed it for a person. It decided nothing |
| `internal/runner/isolationsuite/slots.go` | decided, for isolation, from three constants |
| `shellsuite`, `sqlsuite` | read `parallel_slots` and defaulted to **1**. They did not look at the machine |

The constants were the problem under them. `slotMB = 1500` was taken from the **sql** family, where a slot
peaked at 2.6 GB and its server at 1.38 GB (`sql-native.md` §3). Isolation was then measured on a 60-case
sample at about 700 MB a slot (`isolation-baseline.md` §2), which made the shipped number look twice too
careful — and then the whole corpus at eight slots peaked at **11.11 GB, 1,422 MB a slot**. The sample was
half the answer. A number measured on one corpus, on one machine, by one person, was wrong about the same
suite on the same machine with a bigger corpus.

And memory is only one of the bounds. Where more slots stop paying depends on the disk and the processors:

- **shell**, writing to disk with its syncs, was slower at eight slots than at four; on a machine other work was
  using, sixteen ran no faster than eight (`category/shell/04-configuration.md`).
- **sql** on a 16-core machine delivered eight cores' work at once, and a slot costs 26 s to start
  (`category/sql/05-slots-and-speed.md`).
- **medium** is 12.6 s of cases: four slots were no faster than one.

A constant is the wrong shape for any of this. The cost of a slot and the count past which slots stop paying
depend on the suite, the corpus, the engine's buffers, the disk and the machine, and only the machine that is
about to run knows them — after it has run.

### What the measurements say

Over the whole isolation corpus with testkit's controller, this machine (16 cores, 31 GB, a disk that takes
5–8 ms for a synchronous write; `isolation-controller.md` §8):

| | 4 slots | 8 slots | 14 slots | 8 slots, volatile |
|---|---:|---:|---:|---:|
| wall | 2,509 s | 1,239 s | **797 s** | 1,632 s |
| case seconds, without the cases that hung | 9,093 | 9,101 | 9,899 | 9,025 |
| peak, the fall in available memory | — | — | 18,631 MB, 1,330 a slot | 12,106 MB, 1,513 a slot |
| least available | — | — | 2,033 MB | 12,430 MB |
| NOK | 40 | 32 | 30 | 36 |

- **Wall is case seconds divided by slots**, within a few per cent, until the processors run short: at fourteen
  slots on sixteen processors each case cost 9% more, and wall still fell 36%.
- **The longest case a run sees is often a hang.** The 502 s that looked like the corpus's longest case is five
  attempts of a case that waits a hundred seconds each time. The longest case that passed on its first attempt
  is 64–67 s in every run.
- **Volatile buys isolation nothing** — under 1% of case seconds — where it halves sql's.

## Decision

**A run decides its own parallelism, in one place, from what runs on this machine measured — and learns where
more slots stop paying by having run them.**

### One package

`internal/sizing` holds it. Each suite asks for a slot count and gets a sentence saying why; nothing else
carries a memory constant. The count is the smallest of:

```
(available − 2,048 MB − what the run sets aside) / budget      memory
processors
cases                                                           never more slots than cases
case seconds / the longest unit                                 where the corpus stops more slots helping
growth × the most slots run of this corpus, on this lane         how far a run may go past what was tried
the knee                                                        where more slots were measured not to pay
```

The last two read only the **last four runs of this corpus on this lane**, one per slot count.

`parallel_slots` in the configuration still wins outright. Whoever wrote a number has decided. A run that is
not contained is serial, as CTP's was, and measures nothing.

### The budget adapts

| | budget |
|---|---|
| this machine has run this corpus, or one at least as large | the largest peak per slot of those runs, with 15% on top |
| it has not | the shipped figure for the suite — 1,750 MB for isolation, 2,600 for sql and medium — unless a smaller corpus measured more, which is used instead |
| the suite has no shipped figure (shell) | there is none: it runs **one slot** and measures it. Another corpus's slot cost is not evidence about this one — a shell slot is whatever case it drew |
| the engine's buffers have changed since | each run's figure is adjusted by the change before they are compared: a slot is a floor plus `data_buffer_size` plus `log_buffer_size` |

A slot's peak is the heaviest case it drew, and a sample has fewer heavy ones to draw: 58 cases measured 623 MB
a slot where the whole corpus measured 1,422. So a smaller corpus's record never lowers the budget for a larger
one.

The buffers are the ones the slots run with: the configuration's `default.cubrid.*` for isolation, the
`[sql/cubrid.conf]` section as `do_configure` has written it for sql, the configured cubrid parameters for
shell — each over the install's `cubrid.conf`.

### Where it starts, and how far it goes

A first run of a corpus on a lane starts where the suite was verified: **isolation 4** (ADR-018 found no runner
difference at one and four), **sql 4** (where the slot cost was measured), **medium 1** and **shell 1**
(nothing shows that more pays). Each later run may go to **twice** the most slots this machine has run that
corpus with on that lane — conservative once, aggressive four times. A corpus counts as the same one when its
path matches and its case count is within a tenth: a corpus that grew by a few cases is not a new one, and a
sample of it is.

**The knee** stops it. Among the last four runs of the same corpus on the same lane, the fewest slots that came
within 5% of the fastest wall time are the cap, when a run with more slots is among them. That is how shell on a
slow disk settles at four, medium at one, and sql at whatever its processors deliver — by running, on the
machine in question. `aggressive` ignores the knee.

**Four runs, and not the whole file**, because a knee is a measurement that can go stale: a run a hang
lengthened looks like a run past the knee, and a machine whose disk or load has changed deserves to be asked
again. A count that has not been run for four runs ages out, and the next run tries twice the most it has.

A **lane** is where the slots write: `disk`, `disk, volatile`, a named slot root, and for shell `memory`,
`memory and disk lanes`, or the corpus directory itself. What stopped paying on one says nothing about another.

### What a run records

A contained run samples `MemAvailable` every five seconds from before its slots open, and when it ends writes:

- **the peak** as how far available memory fell — 3–5% above the sum of the run's resident sizes, and the
  quantity the budget is divided into — less, for shell, what was in its corpus tmpfs at that moment;
- **the wall time**, which the knee compares;
- **the case seconds and the case count**, which scale the corpus bound to the next run;
- **the longest unit whose every case passed on its first attempt** — a case for isolation and shell, a
  directory for sql — because a retried or failed-with-an-error case measures a timeout;
- the machine, the engine's two buffers, the slot count, the corpus and the lane.

Only a run of everything it discovered, with every slot up and nothing stopping it, is recorded. A continued
run is a remainder, and a run that stopped on an error measured the error. The slot count recorded is the
number that ran: a sql slot whose turn came after the queue had drained never started a server, and counting
it would divide the peak by more slots than held it.

### Three words, not a number

`parallel` takes `conservative`, `measured` or `aggressive`, and `measured` is the default:

| | budget | growth | knee |
|---|---:|---:|---|
| **conservative** — something else is using the machine | ×2 | ×1 | kept |
| **measured** — the machine is the run's | ×1 | ×2 | kept |
| **aggressive** — find the ceiling rather than avoid it | ×0.8 | ×4 | ignored |

A word it does not know is sized as `measured`, and the slot line says so.

### Where the record lives

`$XDG_STATE_HOME/testkit/sizing/<suite>.json`, which is `~/.local/state/testkit/sizing/` unless
`TESTKIT_SIZING_DIR` says otherwise. **Outside the repository**, for three reasons: the measurement is a fact
about the machine and not about the checkout, so a fresh clone should still have it and a copy carried to
another machine should not; nothing that is not in the repository can be committed by accident; and it is
where the rest of the system keeps this kind of thing.

Each record carries the machine it was taken on — host name, processors, total memory. A record from another
machine is ignored, and the file keeps the last ten runs **of each machine**, so a machine sharing the home
directory cannot push this one's out. It is written beside and renamed over.

### Saying why

`testkit sizing [suite] [mode] [corpus] [lane]` prints the machine, the buffers, this machine's runs of the
suite and the count each of the three words gives, with the sentence for each. It decides nothing and runs
nothing. A run prints the same sentence on standard error and on the status page.

`scripts/sizing.sh` keeps what it measures about the machine and what only it sizes — shell's
`scenario_ram_mb` ceiling — and stops recommending a slot count.

## Consequences

1. **The first run on a new machine is the careful one**, and every run after it is sized by that machine. On
   this one, isolation: 12 slots measured, 6 conservative, 15 aggressive, with 23 GB available.
2. **Defaults change.** Contained, isolation no longer takes four and sql no longer takes one: both start at
   four and grow. Shell and medium start at one, as before, and grow until the knee. Uncontained runs are
   unchanged. A first run over a **new corpus** starts again from the suite's figure, whatever the machine has
   run of another one.
3. **Finding the knee costs runs.** A suite reaches its best count by doubling, so a machine spends a few runs
   below it, and one run past it. That run is what tells the next one where to stop — and, four runs later, the
   question is asked again.
   **Shell with lanes is the exception**: lanes are two pools of slots, so a lane run is floored at two rather
   than stopping on "lanes need at least two slots".
4. **A record can be wrong about the future.** A corpus grows, an engine's buffers change, another job appears
   on the machine. The 15% margin and the largest-of-the-last-ten rule stand between a record and an
   out-of-memory kill; `conservative` stands between them and a machine that is not the run's.
5. **A noisy run can make a false knee.** A run whose wall time a hang lengthened looks like a run past the
   knee. It holds for at most four runs of that corpus, and `aggressive` ignores it meanwhile.
6. **A hang is not sized away.** A case that times out holds its slot for every attempt — 1,500 s at five
   attempts of 300 s — whatever the count. That is `testcase_retry_num` and `testcase_timeout_in_secs`, and the
   cases themselves (`isolation-corpus-races.md`).
7. **The numbers are reviewable.** A slot count comes with the sentence that produced it and the runs it read,
   so an operator can disagree with a figure and see what they are disagreeing with.

## Alternatives considered

**Keep the constants and correct them.** The correction that prompted this would have been from 1,500 to 1,422
— arrived at after a whole-corpus run, and true only of this machine, this corpus and these buffer settings —
and a later measurement of the same machine in the units the formula uses said 1,513. The next machine would
need its own, and nothing in the program would know that.

**Size by memory alone.** It would send shell to sixteen slots on a disk where eight were slower than four, and
medium to as many slots as it has directories. Memory says what a machine can hold, not what pays.

**Model the disk and the processors** — measure a synchronous write and a core's throughput, as `sizing.sh`
does, and derive a count. The figures are real and `sizing.sh` keeps printing them, but a model fitted on one
machine is a constant by another name: shell's disk bound of four is a measurement of this machine's disk. A
run's own wall time is the measurement the count is for.

**Read the record from inside the repository** (`.testkit/sizing/`). Rejected: it is a fact about the machine,
it would travel with a copied checkout, and it would need a `.gitignore` rule that someone can forget.

**Record the sum of resident sizes.** It is what a person watching `ps` sees, and it is 3–5% short of what the
machine gave up.

**Take the longest unit as it is.** On this corpus it would have taken 502 s — a hang — and bounded the run at
nineteen slots for a reason that has nothing to do with the corpus.
