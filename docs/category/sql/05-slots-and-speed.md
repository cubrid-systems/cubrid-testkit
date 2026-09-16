# 5. Slots and speed

[← back to the sql category](README.md)

- [What it costs today](#what-it-costs-today)
- [The syncs are the wall](#the-syncs-are-the-wall)
- [Which disk](#which-disk)
- [How many slots](#how-many-slots)
- [Why medium is serial](#why-medium-is-serial)

Every figure here was measured on one 16-core, 30 GB machine, against the same corpus and engine.
The workings are in [`project/evidence/sql-native.md`](../../project/evidence/sql-native.md); this is what to do with
them.

## What it costs today

sql, 17,459 cases:

| | wall | against CTP |
|---|---:|---:|
| CTP, serial | 1,726 s | 1.0× |
| this runner, serial, same conditions | 1,695–1,753 s | 1.0× |
| serial, `TESTKIT_SLOT_VOLATILE=1` | 763 s | 2.3× |
| **six slots, volatile, slot root on the fast disk** | **270–340 s** | **5–6×** |

Two settings account for all of it, and neither is `parallel_slots` alone.

## The syncs are the wall

A server waits for its log to reach the disk at every commit, and CQT runs in autocommit, so nearly
every statement that writes waits for one. On this machine that was most of a case: eight slots
summed 6,846 s of case time against CTP's 1,693 s — four times per case — and the disk took 199,133
flushes.

`TESTKIT_SLOT_VOLATILE=1` makes those syncs return at once. It is sound for a slot because a slot's
layer is thrown away at the end, and a sync only matters to a machine that goes down: a server killed
mid-case loses nothing, because what it wrote is in the page cache and recovery reads it back from
there. With it on, the disk took 750 flushes, and the cases on eight slots took **half** the time
CTP's single serial run spends on them.

It is off by default, because the syncs are part of the conditions CTP runs under, and a run that
skips them is a different run. Turn it on for the runs where speed is the point; leave it off when
the run is evidence about CTP.

## Which disk

`TESTKIT_SLOT_ROOT` decides where a slot's writes land, and the disk under it is worth more than a
slot or two:

| eight slots, sql | wall |
|---|---:|
| slot root on the machine's root SSD | 1,131 s |
| slot root on its data disk | 722 s |

A 4 KB synchronous write took 5.8 ms on the first and 1.3 ms on the second. And `volatile` does not
rescue a slow disk — four slots on the slow one took 572 s against 362 s on the fast one, because
what is left after the syncs is still bandwidth.

`scripts/sizing.sh sql <conf>` measures the candidates on your machine and names the fastest.

## How many slots

Three things bound it, and the smallest wins:

- **Memory.** A sql slot peaks at about **2.6 GB**: its server 1.38 GB, the PL server 0.58, the
  executor's JVM 0.52, its CAS processes 0.12. Eight slots is 21 GB, which on a 30 GB machine shared
  with a desktop is close enough to the edge that two runs here were stopped for it.
- **What the machine delivers.** Not the core count: this one counts 16 and delivers 8 cores' work at
  once. `sizing.sh` measures that rather than assuming.
- **The corpus.** A directory runs on one slot, and the run cannot finish before its longest
  directory does — 72 s of the 891 s of cases here, so about twelve slots is where the wall stops
  improving whatever the machine has.

Six is what this machine takes. More slots also stopped paying earlier than expected when the slots
started one at a time; with `volatile` they start together — all four up in 27 s where one at a time
took 4 × 26 s — because what a start waits for is the heartbeat, not the disk.

## Why medium is serial

medium's 975 cases are **12.6 s** of work in total, and `_02_xtests` is 5.9 s of that on its own.
Starting a slot costs about 26 s, and the database is prepared once for 30.

| medium | wall |
|---|---:|
| CTP, serial | 85–91 s |
| this runner, serial, volatile | **77 s** |
| four slots, volatile | 77–91 s |

So parallel cannot win: the cases are not the wall, the setup is. And splitting a directory across
slots is not available either — reversing the order inside each directory fails 71 of the 975 cases,
62 of them in `_08_mc_ind`, which is a chain of 68 cases that each build on the last.

Run medium on one slot. Use `volatile` if you like — it is worth the 8–14 s between 91 and 77.
