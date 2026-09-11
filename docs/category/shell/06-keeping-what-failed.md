# 6. Keeping what failed

[← back to the shell category](README.md)

A run that fails a case and keeps nothing has to be run again to be diagnosed. This document is what
is kept today, what it costs, and what should be kept instead.

- [What exists today](#what-exists-today)
- [What it costs](#what-it-costs)
- [The design: keep the delta, not the install](#the-design-keep-the-delta-not-the-install)
- [Three tiers](#three-tiers)
- [Where it goes](#where-it-goes)
- [Configuration](#configuration)

## What exists today

CTP's `do_check_more_errors` takes a snapshot when a case leaves a core file, or when the count of
`FATAL ERROR` lines in `$CUBRID/log` has gone up:

```
$CTP_ERROR_BACKUP_DIR/AUTO_<build_id>_<datetime>.tar.gz
   ├── CUBRID/       ← the whole install
   ├── <case_dir>/
   └── init_path/
```

Two things about that trigger are worth knowing before relying on it:

- **An ordinary NOK gets nothing.** A wrong answer, or a server that would not start, leaves no
  snapshot at all — only the verdict and the console output in `feedback.log`.
- `SKIP_CHECK_FATAL_ERROR=TRUE` does not turn it off. It is one arm of an `||` whose other arm fires
  whenever a core file exists, so a core is snapshotted whatever it says.

**The destination is configurable** (`CTP_ERROR_BACKUP_DIR`), and the whole mechanism can be turned
off (`CTP_ERROR_BACKUP=off`). It still defaults to `~/ERROR_BACKUP`, so a run that sets neither
behaves exactly as it always did.

## What it costs

Measured on this corpus and this build:

| | |
|---|---:|
| the whole install, which a snapshot copies per failing case | **748.8 MB** |
| a slot's overlay upper layer — everything one case actually changed | **~630 KB** |
| a case's server logs alone (`log/server/*.err`, `.event`, `.access`) | **~5 KB** |
| the broker's SQL log, which is 99.6% of the log tree | **MB, and highly variable** |

A run with seventy failures writes tens of gigabytes into the home directory, and it writes them
*inside* the measurement — the wall clock carries it and nothing in the output says so.

## The design: keep the delta, not the install

The install is identical for every case in a run. Copying it per failing case is 748 MB of the same
bytes, over and over.

**The delta is already separated, by construction.** Each slot runs behind an overlay whose lower
layer is the pristine install and whose upper layer is everything the case wrote. Nothing has to be
diffed and nothing has to be excluded — the upper layer *is* the answer:

```
   lower layer                     upper layer
   ───────────                     ───────────
   $CUBRID  (748.8 MB)             everything this case changed  (~630 KB)
   bin/ lib/ share/ java/          log/*.err  log/server/*  log/pl/*
   jdbc/ msg/ demo/ timezones/     databases/  var/  conf/ as the case left it
   conf/ as shipped
        ▲                                ▲
        │                                │
   identical every case.            1,200× smaller, and it is the part
   Never copy it.                   that says what happened.
```

Everything the earlier snapshot was for is in there — the server's `.err` and `.event` logs, the
utility logs, `cubrid.conf` as the case left it, and the databases if the case still had them.

Core files are the exception: they are large, they are the one thing a stack trace needs, and they
are worth their own tier rather than being folded into this one.

## Three tiers

The tier boundary is the **file class**, not the verdict. This matters because the cheap tier is
worth keeping for cases that *passed*:

- CTP greps the server log for `Internal Error` only when it is writing a NOK, so a case that passed
  with an internal error in its log says nothing about it.
- A case that fails intermittently cannot be diagnosed from the failures alone — the run that passed
  is half the comparison.
- Warnings that never change a verdict are how an engine regression shows up first.

| tier | what | when | cost over 3,444 cases |
|---|---|---|---|
| **1. always** | `log/server/*`, `log/*.err`, `cubrid_utility.log`, and what the case wrote in its own directory | every case, pass or fail | **~15 MB plus a few KB a case** |
| **2. on failure** | the broker's SQL log and `cubrid.conf` as the case left it | NOK | MB per case, capped |
| **3. on request** | core files, and the databases if asked | core, or explicitly | GB |

Tier 1 needs no policy. Fifteen megabytes against a tmpfs measured in gigabytes is a rounding error,
so the question of whether to keep it is not worth asking.

**The case's own files are in tier 1, not tier 2**, and that boundary was moved after it got in the
way. They are what actually says what happened -- `exp.log`, `load.log`, a diff -- and they are
kilobytes. Putting them behind the verdict made the comparison this document argues for impossible:
two expect cases that pass on one machine and fail on another could not be diagnosed, because the
passing run kept nothing to compare against. The 1,979 bytes that were missing cost a run to
recover. The broker log stays behind the verdict, because it is the megabytes.

## Where it goes

```
$CTP_HOME/result/<category>/
  current_runtime_logs/       ← frozen. Untouched
  case-logs/                  ← this runner's own, like patched.txt
    _06_issues/_14_1h/bug_a/cases/
      server/                 ← tier 1: log/server/*
      pl/                     ← tier 1
      *.err                   ← tier 1: log/*.err, cubrid_utility.log
      broker/                 ← tier 2, on failure
      cubrid.conf             ← tier 2: as the case left it
```

The path is the case **as the corpus names it** — the scenario off the front and
`<name>.sh` off the end, since `<name>/cases/<name>.sh` repeats itself. Keeping the absolute path
instead buries every capture under a copy of wherever the corpus happened to be checked out, which
is what the first implementation did and what a test now prevents.

**Named by the case path, not by a timestamp.** CTP's `AUTO_<build>_<datetime>` is unambiguous when
one case runs at a time and stops being so at eight — several tarballs land in the same second and
the name does not say which case is in which. A case path is unique by construction, so nothing has
to be allocated or disambiguated. A retry becomes `<case path>/try2/`.

`~/ERROR_BACKUP` keeps its own trigger and its own contents, unchanged. It answers a different
question — *give me everything, including cores* — and putting successful cases' logs under a
directory named ERROR_BACKUP would make the name lie and break anything reading it.

## Capture point, and what it must not do

The capture has to happen **after the verdict and before the slot reset**. The reset (`RestoreScript`)
runs at the *start* of a case, not at the end, so what a case wrote survives until the next case
claims that slot — and `dropInSlot` discards the corpus overlay when a directory's last case
finishes. Before the verdict there is nothing to keep; after either of those there is nothing left
to copy.

**And when the run ends, the slots go.** Their upper layers are removed with the run's slot root,
so the last case in each slot loses what it left in `$CUBRID` too. After the run, what a case wrote
survives only where it was copied: `case-logs/`, and `~/ERROR_BACKUP` for a case that dumped core or
logged a fatal error while that backup is on. The upper layers used to outlive the run, under
`/var/tmp/testkit-slots/<pid>`, until a later run with the same pid mounted over them and started
from what they held.

It copies **by path, through the case's own channel**, rather than reading the slot's overlay upper
layer from outside. It is the same files either way, and this works identically whether the run has
overlays, one slot, or neither. The destination is under `CTP_HOME`, which no slot overlays, so a
copy made inside a slot lands on the real filesystem and survives the slot.

Two rules:

- **Copy, never move.** The case is still the source of truth for its own verdict.
- **A capture that fails must not fail the case.** A diagnostic that changes what it is diagnosing is
  worse than no diagnostic. Record that the capture did not happen and carry on.

And a caveat: the destination is disk while the source is often tmpfs, so tier 2 adds I/O to the
reclaim path. Tier 1's fifteen megabytes are not worth measuring; tier 2's are.

## Configuration

| | default | |
|---|---|---|
| `case_logs` | `off` | `off`, `fail`, or `all` — `all` keeps tier 1 for passing cases too. A mode that is none of the three is refused rather than read as `off` |
| `case_logs_max_mb` | — | a budget for the whole run; capture stops when it is reached and says so, once |
| `CTP_ERROR_BACKUP` | on | `off` turns the heavyweight snapshot off entirely |
| `CTP_ERROR_BACKUP_DIR` | `~/ERROR_BACKUP` | where the heavyweight snapshot goes |

`off` is the default because the feature is new and a run that has never asked for it should not
start writing megabytes it did not ask for. Tier 1 has been measured at 15 MB for the whole corpus,
so `all` is defensible as a default later; that should be decided by measuring the reclaim-path
cost, not by argument.

```
case_logs=all
case_logs_max_mb=512
```

```
[INFO] case logs under /path/CTP/result/shell/case-logs
[INFO] case logs: 1 kept, 0 MB, under /path/CTP/result/shell/case-logs
```

## Two rules the implementation keeps

**A capture that fails does not fail the case.** Every copy is `2>/dev/null` and the script exits 0
whatever happened; a capture that could not run at all is reported as a `[WARN]` line and the case
keeps its own verdict. A diagnostic that changes what it is diagnosing is worse than no diagnostic.

**A capture over budget is kept, and is the last one.** Refusing it after it has been written would
leave the run having paid for it and thrown it away, so the budget stops the *next* capture rather
than discarding this one — and the run says so once, not once per case.

---

The motivating case, recorded honestly: a run in which nineteen of twenty-two cases reported
`cubrid server start: fail` could not be diagnosed, because the server's own `.err` file explaining
why was in an overlay that had already been dropped. That is what this exists to stop.
