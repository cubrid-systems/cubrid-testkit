# Where a parallel run's time goes: not the cores, not the bandwidth, and never yet the whole machine

- **Date:** 2026-09-24
- **Status:** measured, seven arms, one of them a control for a hypothesis the first six raised.
- **What this is:** the measurement [`slots-and-the-layer-above.md`](../../design/slots-and-the-layer-above.md)
  §6-1 said had to come before deciding whether to open axis O. It answers it, and not the way
  either candidate answer expected.
- **Trees:** engine `11.5.0.2513-5f3a30d` · pairs from `cubrid-cluster-sandbox`, docker, one host,
  16 cores, 31 GB.
- **Corpus:** `_01_object/_01_type` — 290 cases over 13 directories, chosen because its directories
  are evenly sized, so the dealing is not the thing being measured.

---

## The question

ADR-014 closed the fleet on a measurement: CTP was not used as one. Reopening it needs the same
kind of evidence — **that one machine is really not enough** — and the design note named the three
answers that measurement could give:

| what dominates as parallelism rises | prescription |
|---|---|
| iowait | this machine's disk. One NVMe is cheaper than a layer |
| CPU, with slots at the knee | more machines. Axis O gets its case |
| neither, and idle remains | serialisation. Neither purchase helps |

## What was measured

Pairs from 1 to 12, `/proc/stat` and `/proc/diskstats` sampled every second, each arm's wear
recorded first so the instrument can be seen to be even.

| n | wall | speedup | eff | user | sys | iowait | idle | write | set |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|
| 1 | 133 s | 1.00x | 100% | 5.4% | 2.0% | 5.7% | **86.8%** | 6.9 M/s | A |
| 2 | 73 s | 1.82x | 91% | 7.2% | 3.0% | 10.7% | 79.0% | 9.9 M/s | A |
| 4 | 44 s | 3.02x | 76% | 10.5% | 4.8% | 19.4% | 65.1% | 15.6 M/s | A |
| 8 | 28 s | 4.75x | 59% | 15.3% | 7.7% | 30.0% | 46.9% | 25.0 M/s | A |
| 8 | 42 s | 3.17x | 40% | 14.6% | 6.5% | **44.9%** | 33.8% | 29.1 M/s | B, disk 90% full |
| 8 | 32 s | 4.16x | 52% | 13.2% | 6.7% | 31.5% | 48.4% | 21.5 M/s | B, disk 87% full |
| 12 | 28 s | 4.75x | 40% | 17.0% | 8.9% | 39.6% | 34.3% | 25.1 M/s | B, disk 90% full |

As core-equivalents of sixteen:

```
n= 1:  working 1.2 · blocked on io 0.9 · idle 13.9
n= 8:  working 3.4 · blocked on io 4.8 · idle  7.5
n=12:  working 4.2 · blocked on io 6.3 · idle  5.5
```

## The answer: none of the three, and the reason matters

**Not the cores.** At twelve pairs — twenty-four CUBRID servers — 4.2 of 16 cores are working. 26%.

**Not the bandwidth.** The most this run ever wrote is **25 MB/s**. That is nothing to any storage
made this decade, and it does not grow with `n` the way wall clock falls.

**iowait is the only term that grows** — 5.7 → 10.7 → 19.4 → 30.0 → 39.6% — and it grows **while the
bandwidth stays idle**. 25 MB/s with 40% iowait is not a throughput problem; it is **latency**. A
database committing per statement issues small synchronous writes, and what is being waited on is
each `fsync` returning, not bytes moving.

**And the machine has never once been full.** 5.5 cores idle at n=12, with the disk barely touched.

So the third answer is the closest but is not right either: it is not serialisation in the runner,
because adding pairs keeps helping. **The run is latency-bound on commits, and the machine is not
saturated in any dimension.**

## What follows for axis O

**Nothing yet, and that is the finding.** Wall clock still falls from eight pairs to twelve — 42 s to
28 s on the same set — while efficiency falls from 59% to 40%. Efficiency falling is not a reason to
stop: the thing being bought is wall clock.

> To say "we need more machines", this machine has to be used up. It is not.
> Adding a machine to a host that idles 5.5 of 16 cores is buying a second idle machine.

The design note's §6-1 leaned on one earlier reading — 52% iowait on a sixteen-core host — as the
number that might reopen the question. **That reading was wrong twice.** It was taken with eleven
pairs standing, one of them worn to 127,114 log pages, so it described the load rather than the
machine; and measured properly, iowait rises *while half the machine idles*, which is not what a
saturated host looks like.

## The finding nobody was looking for: free disk is a performance variable

Two arms at n=8 disagreed by 50% — 28 s and 42 s — on the same corpus with the same pair count. The
only difference was how full the filesystem was. So it was tested directly: **the same eight pairs,
the same arm, after freeing 8 GB.**

| | free | wall | iowait | idle |
|---|---:|---:|---:|---:|
| filesystem 90% full | 23 GB | **42 s** | 44.9% | 33.8% |
| filesystem 87% full | 31 GB | **32 s** | 31.5% | 48.4% |

**Eight gigabytes bought 24% of the wall clock.** iowait fell by a third and the idle came back.

This lands directly on this suite, because [a pair wears out](a-sandbox-pair-wears-out.md): pairs
only grow, so a machine running them walks its own filesystem toward the region where everything
costs more. Eleven pairs took this host to 98% on 2026-09-23, and every number measured that day
carries the effect — including the 4.58x reported for eight shards.

**Cleaning up is not only hygiene. It is a performance setting**, and the `DISK` column in
`csb cluster ls` is how you see it.

## The prescription

| | |
|---|---|
| storage with lower commit latency | an NVMe. Not for the 25 MB/s — for the `fsync` |
| keep the filesystem under ~85% | measured above; and pair wear eats this on its own |
| raise `n` further | twelve still helps. The knee has not been found |
| axis O | **no case.** It is what comes after those three are spent |


## Two categories on one machine: the part scaling one category does not show

The arms above scale **one** category. A second measurement asked what happens when two run at once,
because that is the shape a real schedule has. `shell` on a 10-case slice at 4 slots, `ha_repl` on
the 290-case slice at 4 pairs, then both together — small samples, one machine, nothing else on it.

| arm | | wall | user | sys | iowait | idle | write |
|---|---|---:|---:|---:|---:|---:|---:|
| A | `shell` alone | 141 s | 0.6% | 0.4% | **55.5%** | 43.5% | 54.9 M/s |
| B | `ha_repl` alone | 44 s | 8.3% | 3.9% | 18.3% | 69.3% | 14.6 M/s |
| C | **both together** | **182 s** | 2.9% | 1.4% | 49.1% | 46.5% | 46.7 M/s |

```
max(A,B) = 141 s        A + B = 185 s        C = 182 s
```

**C landed on the sum, not the maximum.** They did not overlap; they serialised. And the cost fell
almost entirely on one side:

| | alone | together | |
|---|---:|---:|---|
| `shell` | 141 s | 153 s | **1.09x** — barely touched |
| `ha_repl` | 44 s | 182 s | **4.14x** — starved |

### What did the starving, and why it is not schedulable

Look at arm A. `shell` alone runs at **user 0.6%, sys 0.4%, iowait 55.5%** and 55 MB/s. It hardly
uses a processor. Its cases create and drop a database each, so what it consumes is the disk — and
not its bandwidth but its queue.

`ha_repl` spends its time waiting for a write to reach a slave, which is waiting for a commit to
reach the platter. When `shell` owns the queue, `ha_repl`'s small `fsync`s wait behind it. That is
why the queue's owner barely notices and the waiter pays 4x.

**And in arm C the CPU is 4.3% with 46.5% idle.** There is nothing to schedule around: the machine
is not short of any resource that a scheduler can hand out. The contention is in paths one host has
exactly one of — one kernel, one page cache, one I/O queue, one `fsync` path.

## What this measurement does and does not license

**It does not license "buy an NVMe".** Faster storage shortens the queue; it does not stop two runs
sharing it. The shape would be the same with less of it.

**It does not license scheduling categories apart** — and that is wrong twice over. A category is
**how a test is written**, not what it costs: `shell` holds cases that build nine-gigabyte databases
and cases that echo a string, and the corpus changes whenever someone adds a directory. So a
category is the wrong key. And even with the right key, the contention above is not of a kind a
scheduler can avoid, because it is not a resource being allocated.

**What it does establish is that one machine shares things that cannot be partitioned**, and that a
second machine does not share them. That is not an optimisation with a ratio attached. It is a
different guarantee: a separate kernel and a separate queue.

**Whose choice that is, is the point.** Whether to buy isolation is the person running the tests
deciding what their run is worth — not this project deciding for them. So the job is to make the
choice expressible, not to be clever on their behalf. The measurement's contribution is the number
that makes the choice informed: **4.14x on the starved side, at 4% CPU.**

*(Not measured: the two-machine arm. That a separate kernel and queue remove this contention follows
from what is shared, not from an experiment run here. The one-machine half is measured; the other
half is the mechanism.)*

## What is not claimed

**That 12 is the knee.** Wall clock was still falling there and no arm beyond it was run, because
sixteen pairs do not fit on this disk with room to grow.

**That an NVMe was measured.** The latency reading is inferred from 25 MB/s beside 40% iowait, which
is a strong shape but not the same as having swapped the disk.

**That the single-category conclusion covers a real schedule.** It does not, and the section above
is why: scaling one category leaves the machine half idle, and running two fills a path neither can
see. "Axis O has no case" was measured about one category widening and must not be read further than
that.

**That another machine was measured.** The two-category arms are one host. What a second host
removes follows from what a host has one of; it was not run.

**That the two n=8 arms differ *only* by disk fullness.** Set B's pairs were built fresh and set A's
had been through four arms, so their wear differs — 206-244 pages against 208-1,654. The control arm
holds set and wear fixed and varies only free space, which is why it is the one to trust.
