# 3. Running it

[← back to the isolation category](README.md)

- [What it needs](#what-it-needs)
- [A run](#a-run)
- [Watching it](#watching-it)
- [Slots](#slots)
- [What it leaves behind](#what-it-leaves-behind)

## What it needs

| | |
|---|---|
| an install | `$CUBRID`, with its headers in `$CUBRID/include`: every slot builds ctltool against it |
| CTP | `$CTP_HOME` at a CTP tree with `isolation/ctltool` — `runone.sh`, the scripts, and the C sources of `qactl` and `qacsql` |
| a corpus | `cubrid-testcases`, at the `isolation` directory |
| a C toolchain | `gcc` and `make` |
| what CTP's check looks for | `$JAVA_HOME` set, and `java`, `diff`, `wget`, `find`, `cat` on the path. The runner does not use Java; the check is CTP's and is kept as it was |
| containment | `TESTKIT_CONTAIN=1`. **Required**: `runone.sh` kills every `cub`, `sleep`, `qactl` and `qacsql` the user owns, and only namespaces keep that to the run |

`TESTKIT_NATIVE=isolation` is the gate that says this program runs the task rather than handing it to CTP.

## A run

```bash
export CUBRID=/path/to/CUBRID
export CUBRID_DATABASES=$CUBRID/databases
export CTP_HOME=/path/to/CTP
export JAVA_HOME=/usr/lib/jvm/java-8-openjdk-amd64
export PATH=$CTP_HOME/bin:$CTP_HOME/common/script:$CUBRID/bin:$JAVA_HOME/bin:$PATH

TESTKIT_NATIVE=isolation TESTKIT_CONTAIN=1 testkit isolation -c isolation.conf
```

A conf that runs the corpus as CTP's daily runs do, with the page:

```
scenario=/path/to/cubrid-testcases/isolation
testcase_timeout_in_secs=300
testcase_retry_num=4
testcase_exclude_from_file=/path/to/cubrid-testcases/isolation/config/daily_regression_test_excluded_list_linux.conf
status_http=on
```

It says nothing about slots, so the run takes the default ([below](#slots)).

The run exits 0 whether or not cases failed, as CTP's does. A machine check that fails, a build that cannot be read,
or a configuration the runner refuses exits 255. A scenario directory that is not there prints `[ERROR]` and exits 0 —
CTP's behaviour, kept because the exit code is part of the frozen surface.

## Watching it

`status_http=on` serves a page on `127.0.0.1:51523` — a second run moves along to the next free port — and says where
on standard error, because standard output is CTP's. A port alone (`status_http=51537`) listens on every interface. It shows what each slot is running, the rate, and the failures as
they land. Clicking a finished case shows its block of `feedback.log`, with the diff for a failure; a running case
shows the raw result `runone.sh` is writing.

## Slots

**Slots are the default, and the machine sizes them.** A run whose conf does not set `parallel_slots` takes the
smallest of: the available memory, less 2 GB for everything else, divided by what one slot cost in this machine's
own runs; one slot per CPU; four on a machine's first run, and afterwards twice the most it has run; and the count
where a run of the same corpus was fastest, when more slots were measured to be no faster. It says on standard
error what it chose and why:

```
[INFO] 12 slot(s): 12 by memory (23212 MB less 2048, at 1739 MB a slot, this machine's own runs)
```

A machine that has not run this corpus yet is sized at 1,750 MB a slot. Every complete run records what it used —
how far available memory fell, its wall time, the case seconds, the longest case that passed on its first attempt,
where its slots wrote — in `~/.local/state/testkit/sizing/isolation.json` (or `$XDG_STATE_HOME/testkit/sizing/`, or
`$TESTKIT_SIZING_DIR`), and the next run on that machine is sized by the largest per-slot peak of its runs of a
corpus at least as large, with 15% on top ([ADR-020](../../project/adr/ADR-020-sizing.md)). The file is not in the checkout and is ignored on any other
machine. `testkit sizing isolation` prints it and what each of the three `parallel` words would decide:

```
  conservative   6 slots -- 6 by memory (23212 MB less 2048, at 3478 MB a slot, this machine's own runs), sized conservative
* measured      12 slots -- 12 by memory (23212 MB less 2048, at 1739 MB a slot, this machine's own runs)
  aggressive    15 slots -- 15 by memory (23212 MB less 2048, at 1391 MB a slot, this machine's own runs), sized aggressive
```

`parallel=conservative` is for a machine something else is using: it doubles the budget and never goes past what was
run. `aggressive` finds the ceiling rather than avoiding it: 0.8 of the budget, four times what was run, and no knee. `parallel_slots=1` runs serially, as CTP does; any other value is used as written. Each slot has its own install
overlay, its own `ctldb` and its own build of ctltool, which it makes at its first case. Measured on this machine
([`isolation-baseline.md`](../../project/evidence/isolation-baseline.md)):

| | one slot | four slots |
|---|---:|---:|
| 60-case sample | 76 s | 38 s, the same verdicts and results |
| the whole corpus, 6,772 cases | 12,301 s | 2,978 s |

CTP alone took 11,095 s over the whole corpus. With four slots four cases failed that pass everywhere else, each a
listing whose rows came back in a different order after a different sequence of cases in the slot's `ctldb`; alone they
pass under both runners ([when a case fails](05-when-a-case-fails.md#cases-that-ctp-does-not-reproduce)).

What decides the gain is what cases wait on. Most of an isolation case is waiting — for a lock, for a `sleep`, for the
controller — so slots add little CPU. Two things set the ceiling instead:

- **Memory.** Each slot starts a `cub_server` for `ctldb` with the install's buffers. A shipped `cubrid.conf` asks for
  `data_buffer_size=512M` and `log_buffer_size=256M`; `default.cubrid.data_buffer_size` in the conf lowers it for
  every slot.
- **Timing.** The cases that flip on timing flip more on a loaded machine. Compare a parallel run with a serial one by
  [ADR-018](../../project/adr/ADR-018-isolation-equivalence.md)'s rules, not verdict for verdict.

`scenario_disk=on` gives each slot its own layer over the cases tree, and `TESTKIT_SLOT_VOLATILE=1` makes the syncs on
slot layers return at once; both are in [configuration](04-configuration.md). Volatile does not make isolation faster:
over the whole corpus at eight slots it saved under 1% of case seconds, where sql's cases halved
([`isolation-controller.md`](../../project/evidence/isolation-controller.md) §8).

## What it leaves behind

- the run directory, `$CTP_HOME/result/isolation/current_runtime_logs`: `check_local.log`, `dispatch_tc_ALL.txt`,
  `dispatch_tc_FIN_local.txt`, `feedback.log`, `main_snapshot.properties`, `test_local.log`, `test_status.data`
- `isolation_result_<build>_<bits>_0_<stamp>.tar.gz` beside it
- in the cases tree, unless `scenario_disk` is on: `result/<name>.result`, `result/<name>.log` and `<name>.result` for
  every case — about twenty thousand files over the whole corpus
- in `~/error_backup`, the backup of any case that dumped core
- nothing in the install or the CTP tree: `ctldb`, the conf line Deploy appends, the ctltool build and `runone.sh`'s logs
  were all in the slots' layers, and went with them
