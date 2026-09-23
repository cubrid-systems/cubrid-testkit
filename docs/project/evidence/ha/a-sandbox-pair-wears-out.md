# A sandbox pair wears out: per-case cost rises with how much the pair has been used

- **Date:** 2026-09-23
- **Status:** measured on this host, mechanism read off the node's own filesystem.
- **What this is:** a reused pair is not a valid instrument, and it is not comparable to a fresh
  one. The per-case reset clears the schema; it does not give back the volumes or the copy log.
- **Found by:** an eight-shard timing measurement that came out at **0.86x** — eight pairs slower
  than one — and turned out to be one sick pair rather than anything about concurrency.

---

## The measurement that exposed it

290 cases over 13 directories, dealt across eight pairs. Per-shard case time, against the same
cases run alone on a fresh pair:

| shard | cases | parallel | control | inflation |
|---|---:|---:|---:|---:|
| **pmha** | 38 | **133.2s** | 13.6s | **9.83x** |
| sh5 | 41 | 26.5s | 17.0s | 1.56x |
| sh7 | 39 | 26.2s | 15.1s | 1.74x |
| sh8 | 38 | 25.0s | 23.2s | 1.07x |
| gbha | 40 | 24.7s | 13.5s | 1.82x |
| sh3 | 33 | 21.1s | 12.2s | 1.72x |
| sh4 | 32 | 19.7s | 12.2s | 1.61x |
| sh6 | 29 | 15.8s | 9.7s | 1.63x |

pmha's 133s alone exceeds the entire single-pair run's case time (117s), so **one pair set the wall
clock** and the run reported 172s against the control's 148s. Rebuilt with eight comparably fresh
pairs, the same slice gives 142s → 31s, **4.58x**, with inflation a uniform 1.38–1.65x and no
outlier.

## It tracks accumulated log, and it is monotone

Sampled the same day, `applied_pageid` on each master:

| pair | applied pages | inflation |
|---|---:|---:|
| sh4–sh8 | 475–834 | 1.07–1.74x |
| sh3 | 1,784 | 1.72x |
| gbha | 7,026 | 1.82x |
| **pmha** | **127,114** | **9.83x** |

160x the log of a fresh pair, and roughly six times the slowdown of the worst healthy one. The
healthy band is noise; pmha is not in it.

## The mechanism, read off the node

`du` inside the nodes, worn pair against fresh:

```
pmha-n1                              sh4-n1
  db/pmha_pmha-n2   2243 MB            db/sh4           513 MB
  db/pmha_x003       513 MB            db/sh4_sh4-n2    264 MB
  db/pmha_x002       513 MB            db/sh4_lgat      257 MB
  db/pmha_x001       513 MB            db/sh4_lgar_t      5 MB
  db/pmha            513 MB
  db/pmha_lgat       257 MB
/work  10,875 MB                     /work  2,076 MB
```

Two things grew. The database **added three generic volumes** of its own accord as it was written
to, and the **copy log that carries the master's log to the slave** is 2.2 GB against 264 MB.

Neither is returned. `reset=case` drops the case's tables, serials, triggers and users — it is
schema, and CUBRID does not shrink a volume when the rows in it go away. So the pair keeps every
page it ever needed, and every catalog scan, checkpoint and reset afterwards walks the larger file.
**Cost per case therefore rises monotonically with use, and never falls.**

## What this costs, beyond the one bad number

**A sharded run needs pairs of equal wear.** Eight shards finish when the slowest finishes, so one
worn pair spends the whole benefit — here it spent more than the whole benefit. Shards should be
built fresh, or their wear recorded and compared before the timings are believed.

**The ledger's duration column is not comparable across a long run.** `verdicts.tsv` carries
milliseconds per case, and a 3,327-case run wears its own pair as it goes. A case judged in the
last hour is not being timed against the same machine as one judged in the first.

**A pair reused across days is not a control.** This is the concrete trap: pmha and gbha were the
pairs that had done the most work, which is exactly why they were reached for as "the ones that are
already up".

## What is not claimed

That this is a defect in the engine. A database not returning volume space is ordinary, and the copy
log growing with replication traffic is what it is for.

That the volumes and the copy log were separated. Both grew together and no experiment here varies
one while holding the other, so which dominates is not established.

That the relationship is quantified. Four points spanning 475 to 127,114 pages establish direction
and order of magnitude, not a curve.

## What to do about it

Build a shard set fresh, and **record `applied_pageid` per pair beside any timing** so a reader can
see whether the instrument was even. Both were done for the 4.58x figure in
[`pairs-across-machines.md`](pairs-across-machines.md); the pairs there ran 172 to 1,810 pages.
