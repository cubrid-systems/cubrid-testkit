# 4. Configuration

[← back to the shell category](README.md)

- [CTP's keys](#ctps-keys)
- [This runner's keys](#this-runners-keys)
- [Environment](#environment)
- [What each setting is worth](#what-each-setting-is-worth)
- [Recommended settings](#recommended-settings)

## CTP's keys

These behave as they always did.

| key | default | |
|---|---|---|
| `scenario` | — | the corpus root. Required |
| `test_category` | — | `shell` |
| `testcase_retry_num` | `0` | attempts after the first failure |
| `testcase_timeout_in_secs` | `0` | 0 is no timeout. CI uses 720 |
| `testcase_exclude_by_macro` | — | skip a case whose text contains this, e.g. `LINUX_NOT_SUPPORTED` |
| `testcase_exclude_from_file` | — | files of path fragments to skip, comma-separated |
| `test_continue_yn` | `false` | resume, skipping what already has a verdict |
| `cubrid_db_charset` | `en_US` | what `cubrid_createdb` passes as the locale |
| `feedback_type` | — | `file` |

## This runner's keys

| key | default | impact | |
|---|---|---|---|
| `parallel_slots` | `1` | **large** | how many cases run at once |
| `scenario_ram_mb` | off | **large; can fail a run** | the corpus overlay's upper layer becomes a tmpfs of this size. See [the ceiling](05-the-ceiling.md) |
| `scenario_ram_high_water` | `80` | protective | percent of the ceiling above which no new case starts |
| `heavy_in_flight_max` | `slots/4` | protective | how many of the heaviest cases may run at once |
| `case_plan` | off | small; grows with slots | per-case durations. Read to order the run, written from what it measured |
| `case_sizes` | off | none directly | per-directory peak footprints, read by the lane keys |
| `case_patch_dir` | off | none | per-case patches applied into the overlay; the corpus on disk is unchanged |
| `lane_slow_secs` | off | **negative** | split the slots into a memory lane and a disk lane, by duration |
| `lane_slow_mb` | off | situational | the same split, by footprint |
| `lane_slow_mbps` | off | situational | the same split, by write rate |
| `case_logs` | off | small | keep what a case wrote: `fail`, or `all` to keep the cheap tier for passing cases too. See [keeping what failed](06-keeping-what-failed.md) |
| `case_logs_max_mb` | off | none | a budget for the whole run's captures |
| `status_http` | off | none | the progress page. `on` is `127.0.0.1:51523`; a bare port takes every interface |

## Environment

| | |
|---|---|
| `TESTKIT_NATIVE_SHELL=1` | run `shell` here rather than handing it to CTP. The opt-in gate |
| `TESTKIT_CONTAIN=1` | put the run in namespaces of its own. **Required** by slots and by `scenario_ram_mb` |
| `TESTKIT_CONTAIN_SH` | which shell to bind over `/bin/sh`. `bash` if it can be found |
| `TESTKIT_SLOT_ROOT` | where per-slot overlays go |

And CTP's failure snapshot, which is off the frozen surface and configurable:

| | default | |
|---|---|---|
| `CTP_ERROR_BACKUP` | on | `off` takes no snapshot at all |
| `CTP_ERROR_BACKUP_DIR` | `~/ERROR_BACKUP` | where snapshots go. It used to be hard-coded |

A snapshot is the whole install plus the case directory — 748 MB per failing case — so a run with
many failures writes tens of gigabytes into the home directory, inside the measurement. See
[keeping what failed](06-keeping-what-failed.md).

CTP's own environment still applies — `CUBRID`, `CUBRID_DATABASES`, `CTP_HOME`, `JAVA_HOME` — and so
do `SKIP_CHECK_RECOVERY_ERROR`, `SKIP_CHECK_FATAL_ERROR` and `CTP_ERROR_BACKUP`.

**`$CUBRID_DATABASES` must be inside `$CUBRID`.** CTP's per-case reset cleans exactly
`$CUBRID/databases`; pointing the variable elsewhere leaves the reset scrubbing a directory nothing
uses while the real registry carries entries from case to case.

## What each setting is worth

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
250 MB/s and the disk delivered 88 under four concurrent writers; that gap is the whole win, and it
is also why four slots was the bound before memory was used.

**Lanes lose on a machine where memory is not the binding constraint**, which was the machine they
were measured on. `lane_slow_secs` selects by duration, and duration is anti-correlated with what
memory helps — the long cases are the I/O-heavy ones — so it cost 1.55-1.93x per case. Footprint and
duration turned out to be unrelated, so `lane_slow_mb` picks the write rate at random;
`lane_slow_mbps` is the only criterion that selects what the disk is actually bounded by. **Leave
all three off** unless you have measured that memory binds on your machine.

**What is not a lever.** `max_clients` and `data_buffer_size` do not buy a slot — a server has a
157 MB floor no parameter reaches. And 44% of the corpus's wall clock is literal `sleep` in the case
scripts, which no setting here touches. The harness's own overhead is 2%.

## Recommended settings

### A developer's machine, part of the corpus

```
scenario=/path/to/testcases/shell
test_category=shell
feedback_type=file
testcase_retry_num=0
parallel_slots=4
case_plan=/path/to/plan
status_http=on
```

No tmpfs: at four slots the disk is not yet the bound, and a ceiling you have not sized is a way to
fail a run rather than a way to speed one up.

### One machine, the whole corpus

Sized for a 30 GB machine — run `tools/sizing.sh` before trusting these on another.

```
scenario=/path/to/testcases/shell
test_category=shell
feedback_type=file
testcase_retry_num=0
testcase_timeout_in_secs=720
parallel_slots=16
scenario_ram_mb=18432
scenario_ram_high_water=80
case_plan=/path/to/plan           # written on the first run, read on the next
case_sizes=/path/to/sizes         # likewise
case_patch_dir=/path/to/patches/shell
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
volume sizes are what make the slots fit in the ceiling. Read
[the ceiling](05-the-ceiling.md#cubridconf-is-not-verdict-neutral) before changing them — those four
lines are not verdict-neutral.

### CI

The same, plus `testcase_retry_num=0` so a flaky case stays visible rather than being retried into a
pass, a `case_plan` carried from the last green run, and `status_http` on a published port.
