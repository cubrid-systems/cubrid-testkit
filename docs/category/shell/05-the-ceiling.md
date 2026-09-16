# 5. The memory ceiling

[← back to the shell category](README.md)

`scenario_ram_mb` is the one setting that fails a run rather than slowing it. This document is about
what it is, how it kills a run, and how to size it.

- [What the ceiling is](#what-the-ceiling-is)
- [Three ways it kills a run](#three-ways-it-kills-a-run)
- [Why the gate does not save you](#why-the-gate-does-not-save-you)
- [Sizing it](#sizing-it)
- [`cubrid.conf` is not verdict-neutral](#cubridconf-is-not-verdict-neutral)

## What the ceiling is

A ceiling, not a reservation. A tmpfs occupies what is written to it and nothing more, so a generous
one costs nothing until it is used.

```
   RAM
   ├──────────────────────────────────────────────────────────┤
   │                                                          │
   │  scenario_ram_mb          slots × ~175 MB      the rest  │
   │  ┌────────────────────┐   ┌───────────────┐              │
   │  │ corpus overlay     │   │ one CUBRID    │              │
   │  │ upper layer        │   │ server per    │              │
   │  │                    │   │ slot          │              │
   │  │ ░░░░░░░░           │   └───────────────┘              │
   │  │ ▲       ▲          │                                  │
   │  │ in use  high_water │   ← new cases stop here …        │
   │  └────────────────────┘      … but running ones do not   │
   └──────────────────────────────────────────────────────────┘
```

Its job is to turn *a case that never cleans up kills the machine* into *a case fails for want of
space*.

## Three ways it kills a run

Sized wrong, it fails in three ways, and they look nothing alike:

| sizing | what happens | what you see |
|---|---|---|
| **too large for RAM** | the ceiling plus the servers exceed memory | the **OOM killer** takes the run, with nothing in the run's own log to say why |
| **too small** | cases run out of space | **ENOSPC**, which fails exactly like a case that got the wrong answer |
| **just under** | survives with no margin | correct verdicts, and no room for a corpus that grows |

The second is the dangerous one, because nothing in the output distinguishes *this case is broken*
from *this case had nowhere to write*. So a run that got within 10% of its ceiling **declares its
own verdicts unusable** on the way out.

## Why the gate does not save you

`scenario_ram_high_water` holds back **new** cases. The ones already running fill the rest, and they
do:

```
   t0   gate at 80% ──────────────────────  8 slots start, 8 heavy cases
   t1   ░░░░░░░░░░░░░░░░ 60%                nothing has finished yet
   t2   ░░░░░░░░░░░░░░░░░░░░ 80%  ← gate    no NEW case starts
   t3   ░░░░░░░░░░░░░░░░░░░░░░░░ 100%       the eight already running got there
```

The gate is late by construction — it can only stop the next case, never the ones in flight — which
is exactly why `heavy_in_flight_max` exists beside it. `HeavyCap` is about the *start*, where a
feedback rule cannot help because nothing has been written yet and every slot is empty at once.

## Sizing it

```bash
CUBRID=/path/to/install scripts/sizing.sh
```

It reads the machine and the engine's own configuration and says what bounds the slot count —
memory, cores, disk — and what every number rests on, so an operator who disagrees can see which
measurement it came from.

Two rules it encodes:

1. **The ceiling plus the servers must fit in RAM.** The run warns when
   `scenario_ram_mb + slots × 175 MB` exceeds `MemAvailable`. 175 MB a slot is a reserve, not a
   prediction: a case's own working set is larger and is not knowable in advance.
2. **Sizing can come out negative.** On a 30 GB machine no ceiling fits 24 slots. That is a finding,
   not something to work around — the honest options are fewer slots or more RAM.

## `cubrid.conf` is not verdict-neutral

Lowering `db_volume_size` and `log_volume_size` is what makes many slots fit in a tmpfs: at the
shipped 512 MB, eight slots hold 8 GB of volumes before a single case writes a row.

But **some cases assert the shipped value**, and lowering it makes them measure the run instead of
the engine. There are two classes, and only one is findable by reading:

**Findable.** A case that prints the parameter. The dump reads `name=current (engine default)` and
only the unparenthesised side is settable from `cubrid.conf`, so:

```
db_volume_size=512.0M (512.0M)   ← pins the CURRENT value. Affected
db_volume_size=1.1G   (512.0M)   ← sets its own. Immune
```

Cases that pass their own size to `cubrid_createdb`, or set it with `change_db_parameter`, are
immune. Corpus-wide this class is one case.

**Not findable.** A case that never names a size but depends on the *behaviour* — counting temporary
volumes, say, where the count changes when the volume size does. No search finds this class; it is
found only by running the corpus at both values and diffing the verdicts.

Where a case needs the shipped value, the fix is a patch that restores it for that case alone:

```bash
# in the patch, before the case's own body
sed -i '/^[[:space:]]*db_volume_size[[:space:]]*=/d;/^[[:space:]]*log_volume_size[[:space:]]*=/d' \
    $CUBRID/conf/cubrid.conf
```

`case_patch_dir` applies it into the overlay, `cubrid.conf` is restored before every case, so it
reaches nothing else, and the corpus on disk is unchanged. See
[`../../../overrides/patches/README.md`](../../../overrides/patches/README.md).
