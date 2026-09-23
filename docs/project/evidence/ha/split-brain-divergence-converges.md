# A healed split brain diverges for about a minute, and nothing says so

- **Date:** 2026-09-22
- **Status:** measured, three repeats per arm, with a control. **It corrects a published finding**
  of the sibling project rather than confirming it: the divergence a healed split brain leaves is
  not permanent. It is gone by ninety seconds, with nothing written, in three runs of three.
- **What this is:** group B's first measurement — the first time this suite has asked its question
  of a topology that moved. P3, P4 and P5 of [`module-ha.md`](../../design/module-ha.md) §4 are all
  exercised by it: a fault is produced, a split brain is reached on purpose, and the pair
  comparison runs *after* the disturbance.
- **Trees:** engine `11.5.0.2513-5f3a30d` built here from `cubrid/cubrid` develop · pair `gbha`
  from `cubrid-cluster-sandbox`, rootless podman, one host · `ping-survives` flavour, default
  `ha_calc_score_interval_in_msecs`.
- **Runner:** testkit's own, `internal/sandbox` — `SplitBrain`, `ClearFaults`, `SameOnBothNodes`,
  `HAStatus`. The measurement is `TestLiveSplitBrainLeavesWhatTheGaugesDoNotReport`.

---

## The question, and why it was not already answered

`cluster-sandbox` measured this first and published it
(`findings/active-active-window.md`): after a split brain heals, the merge is one-directional —
what the promoted slave wrote comes back to the restored master, what the master wrote during the
split never reaches the standby — and

> **That divergence is permanent, and nothing reports it.**

Seven runs, one direction, no exceptions, on the same engine commit this runs against. The reading
behind it is a direct read **thirty seconds after one heal**.

What this suite adds is not a second opinion but a second *time*. Its oracle is the pair
disagreeing with itself, so "do they differ" is the only question it asks, and asking it more than
once after the same fault costs nothing.

## The method

Six steps, on a cluster of its own, three times per arm:

```
                                     both nodes hold: 1
csb fault splitbrain              →  two masters, flavour ping-survives
INSERT 101 on n1, 201 on n2       →  one row per side, neither side can see the other
  read both nodes                 →  THE CONTROL
csb fault clear                   →  the heal; the clock starts here
  wait for one active node        →  a poll on state, never a sleep
  read both nodes                 →  the oracle, and the gauges beside it
```

**The control is not decoration.** Two nodes holding the same rows afterwards has two explanations
— a merge, and both writes having gone to one node — and the end state cannot tell them apart.
`cluster-sandbox` has published exactly that mistake against itself: a reader reported a
bidirectional merge whose real cause was a selector that resolved to nothing, so the INSERTs
entered no database at all and the pair matched afterwards because replication had worked
normally. So the split is read while it is open, and **every run below shows `n1 = [1 101]` and
`n2 = [1 201]` at that moment**. A run that did not would be void rather than interesting.

The two arms differ in one thing: whether anything is **written** after the heal. This suite's wait
is a marker row written on the master and polled on the slave, so asking "has it replicated yet"
is itself a write; the sandbox harness reads later and writes nothing.

## What the pair holds, with nothing written after the heal

| after the heal | `gbha-n1` (restored master) | `gbha-n2` (standby) | |
|---|---|---|---|
| **30 s**, run 1 | `1 101 201` | `1 201` | differs |
| **30 s**, run 2 | `1 101` | `1 201` | differs, and neither side is complete |
| **30 s**, run 3 | `1 101 201` | `1 201` | differs |
| **90 s**, runs 1-3 | `1 101 201` | `1 101 201` | **agree** |
| **150 s**, runs 1-3 | `1 101 201` | `1 101 201` | agree |

**Three of three differ at thirty seconds and three of three agree at ninety, and nothing was
written in between.** The row the standby is missing at thirty seconds is the one the sandbox
finding names, and it arrives on its own.

Run 2 is worth its own line: at thirty seconds the *restored master* had not got the promoted
slave's row either. Early in the window the two nodes are not "one behind the other" — both are
incomplete, in different ways.

## Why the arm that writes was a coin flip

| arm | runs | agreed | differed |
|---|---:|---:|---:|
| a marker is written after the heal | 8 | 5 | 3 |
| nothing is written after the heal, read at 30 s | 3 | 0 | 3 |
| nothing is written after the heal, read at 90 s or later | 3 | 3 | 0 |

The marker arm reads whenever the marker arrives, and the marker takes **54.8 s to 58.9 s** to
cross a pair that has just healed — four crossings, against about 1.3 s on an undisturbed pair
([`p1-sleep-to-wait.md`](p1-sleep-to-wait.md)). That lands the read in the middle of the
convergence, so the arm is a coin flip for a reason that has nothing to do with the write: in the
**two** runs where the marker never crossed within its minute the pair differed both times, and in
the **four** where it crossed it agreed three times. The two remaining runs of the eight are the
earliest, before the arms existed, and their marker was not timed; the first of those also predates
the control and is the weakest run here.

So the write is not what pulls the row across. **Time is**, and the marker's own slowness is
another measurement of the same window.

## What the gauges said, throughout

Healthy. In every run of every arm, including the ones over two different databases:

| | |
|---|---|
| roles | one `registered_and_active`, one `registered_and_standby` |
| `fail_counter` | **0** on both nodes |
| apply lag | 0-1 page |
| `cubrid heartbeat status` | one master, one slave, as expected |

This is the part of the sandbox finding that stands without qualification, and it is the part that
matters operationally. It is also the third time this suite has met it: an object-domain column and
a trigger's owner both differ across a pair whose `fail_counter` never moves
([`object-domain-not-replicated.md`](object-domain-not-replicated.md),
[`trigger-owner-change-not-replicated.md`](trigger-owner-change-not-replicated.md)). **A gauge that
is accurate about the present is silent about a row that has not arrived.**

## What this changes

**For the sibling project's finding:** "permanent" is measured at one moment, and the moment is
inside a window that closes. The direction it reports is right and reproduces here; the duration is
not. This is reportable back to `cluster-sandbox` — it is that project's own standard, which is why
its finding carries a section on being re-measured after a reader disagreed.

**For P5:** the property is not "divergence the gauges cannot see, forever". It is a window,
about a minute wide on this configuration, in which the two databases differ and every gauge reads
healthy. A suite that reads once, early, reports a divergence that is not there by the time anyone
acts on it; a suite that reads once, late, reports nothing and calls the fault harmless. **So a
verdict after a fault has to carry the time it was taken at**, and a group B case's contract has to
name its read points rather than one deadline.

**For the runner:** nothing yet. This is a measurement, not a case format. What it establishes is
that the parts exist and agree — `SplitBrain` produces the state, `ClearFaults` takes it off, the
group settles, and the oracle answers on the other side.

## What is not claimed

One pair, one host, one build, one shape of writes — a single row per side, on a table with a
primary key. `ha_calc_score_interval_in_msecs` is at its default, and the sandbox finding shows
that parameter setting the width of the active-active window, so a raised interval is unmeasured
here. The convergence point is bracketed between 30 s and 90 s and not measured more finely: the
samples are three, not a sweep.

The host was also running a 3,327-case `ha_repl` run on a second cluster throughout, which is load
this measurement did not control for. It makes the timings an upper bound rather than a clean
figure, and it does not touch the direction or the convergence itself.

`to_be_active` was not sampled. Neither was a partition without a split brain (P3's own case), and
neither was the switchover timing P6 asks for.
