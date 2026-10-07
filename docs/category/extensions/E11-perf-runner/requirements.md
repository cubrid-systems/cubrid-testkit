# E11 — Weekly Performance-Regression Runner (Requirements)

*English · [한국어](requirements.ko.md)*

**Source:** cubrid_cv `plan/perf_regression/` — PROPOSAL, Spec v0.1.3, Design v0.1.3 (the interface
and the algorithms live there; this entry says what of it enters testkit, and why here)
**Status:** incubating — **in progress** (ADR-EXT-011 proposed 2026-10-02; `perf validate` and
`perf list` are in the tree (M1), `perf run` was merged in #16 (M2, 2026-10-03), `session` in M3 (2026-10-07))
**Axis mapping:** none of the eight — a *measurement* capability, not an oracle. The nearest
neighbour is E7 (workload), and this is what C-004 left on testkit's side of the line
**Companion docs:** the Spec and the Design in cubrid_cv; `io-contract` is Spec §7

---

## 1. The problem this entry solves

Nobody can say today whether `develop` got slower at a single-row commit, a cold heap scan, a
`backupdb`, or a CDC extraction since the last release — because nothing measures those on the
same machine every week and keeps the number. The standard benchmarks the hub runs (DOTS, TPC-C
against PostgreSQL and MySQL) answer a different question: throughput under a mixed load. A few
percent on one basic operation disappears inside them.

The runner measures a fixed catalogue of basic operations, target build against reference build,
interleaved on one quiet machine, and records the ratio with the build's fingerprint. A team branch
registers itself in a file and is compared against its merge-base the same way.

## 2. Why in testkit

- It is a runner: it stands clusters up, runs a program against them on a schedule, collects what the
  program and `/proc` say, and writes files. That is what testkit is.
- It consumes `cubrid-cluster-sandbox` the way testkit already does (ADR-022, `internal/sandbox`),
  with the five flags the sandbox gained for it (`single` with `ha_mode=off`, `--set` to
  `cubrid.conf`, `--client-image`, `--broker-set`, and `--cpuset`).
- It is a **new entry point**, `testkit perf`, beside the frozen task names (`external-surface-freeze`
  §6-1: the task list is F1; a new subcommand is NF). Nothing in it changes what `testkit shell`
  or `testkit sql` does, and it is routed before containment because none of it is a corpus run.

## 3. The boundary (C-004, closed for this axis)

| Lives in | What |
|---|---|
| **cubrid-engine-suite** `benchmarks/regression/` | the cases (`case.json` + client source), the fixtures, the scripts that run inside a node, the client image, `branches.conf`, `perf.conf`, the ledger |
| **cubrid-testkit** `internal/perf/` | the one parser for those files, the session, the measurement, the judgment, the sidecars conbench ingests |
| **cubrid-cluster-sandbox** | the clusters |
| **cubrid-conbench** | where the ratios go, and the history |
| **cubrid-desktop** | the hub: `bench-client` lease, the weekly timer, the build step |

The runner is Go only. The hub has Python, but that `uv` environment is cbingest's, and the
runner does not lean on it.

## 4. What is in the tree now

`testkit perf validate <case-dir|fixture-dir|suite-dir|branches.conf|perf.conf>` reads a file with
the session's own parser and names every problem, exit 2 when there is one. A conf is validated
with what it points at: the suite, the registrations, and that each canary is a case the suite has.
`testkit perf list -c <perf.conf> | --suite <dir>` prints the cases with the pass budget and the
bound the session checks the weekend against.

The refusals are the Spec's (§7.2): an unknown key, a missing key, a wrong type, an id that is not
the directory, a counter off the collect layer's list (a statdump name is checked against the engine's own
table, `statdump_names.go`, 234 names at develop `5f3a30d`), `repeats < 3`, `warmup < 1`,
`tolerance ≤ 0`, a client shaped for another driver, a `clients` count that does not fit the driver (0 for a
utility, 1 or more otherwise), a restored snapshot with `warm_s = 0`, a fixture `size` that is not a volume
size such as `2G`, a fixture name, case directory or `client.bin` outside `[a-z][a-z0-9_]*` (a fixture's name
becomes the database `perf_<name>`, and createdb refuses a dash), a `client.main` that is not a Java class
name, and a fixture the suite does not have or has at another version. `branches.conf` refuses an unknown key,
a line without `owner=`, a `repo=` that is not `owner/repo`, a date that is not a date, a glob
that cannot match, and a branch registered twice. `perf.conf` is a closed key set too.

`testkit perf run <case-id> --suite <dir> --build <target> --build <reference> [--repeats N] [--out <dir>]
[--cpuset <list>] [--client-cpuset <list>] [--client-image <image>] [--keep]` is a session's path for one
case: two `single` clusters through `internal/sandbox` (`CreateWith`: client image, pinning, the case's
`cubrid.conf` keys, the broker's CAS count pinned to the largest `clients`), the fixture built on each and
snapshotted with a plain copy (`--reflink=never`), the AB warm-up and ABBA measured passes, the L0 and
statdump snapshots around each pass, the client's own report, the judgment, and the files of Spec §7.6 under `--out` (`regression-case.json`, `cases.csv`,
`counters.json`, the describe artifacts, `session.json`). The last line of standard output is the ratio.
Nothing is published. The two cpusets are two flags because a CPU list has commas of its own.

`testkit perf session -c <perf.conf> [--dry-run] [--only <case-glob>] [--pair <name>] [--out <dir>]
[--deadline <RFC3339>] [--id <run-id>] [--keep]` is the weekly session (Design §5.1, M3): the conf, the
suite and the registrations read and refused as one (exit 2), the builds taken from `builds.manifest`
(`builds.go` -- a pair the file does not have or whose tree is missing is `skipped: build missing`, a file
older than 24 h skips every pair as `builds stale`), the fingerprints read and compared with the previous
`*_perf-weekly` run under `$BENCH_RUNS` (FR-2), a Debug build refused, the host checked (`hub.go`: csb,
the client image, disk, `MemAvailable`, the page-cache sudo line, bench-mode and boost recorded, and the
`cub_server`/`postgres`/`mysqld` outside any container ended or the session refused with exit 3 -- guard
stage 1), stale `pf-` clusters purged, then every pair: the canaries A/A on two clusters both running the
reference build, the pair invalid and its cases `skipped` when one is outside `canary_tolerance`, then the
cases in a per-session shuffle, grouped by `cubrid.conf` overrides (one cluster per group, `schedule.go`),
with the idle side's containers paused during every pass and only the current fixture's server up on each
side (`cluster.go`), the host's foreign CPU and `pswpin` read around every measured pass (guard stage 3,
`null(contaminated)` beyond a core or any swap-in), five more ABBA pairs when the estimate is outside the
tolerance without agreement (FR-20.1), the lease (`$BENCH_RUNS/.lease.json`) checked at every case boundary
(L7), the budget or the `$PERF_DEADLINE` honoured at pair and case boundaries (`skipped: budget`, exit 0),
and the files: a sidecar per pair (and `canary/`, `overlap/`), `counters.json` per case, `summary.md`,
`ledger_rows.md`, `session.json` (`summary.go`). `--dry-run` writes the plan and `session.json` and creates
nothing (Spec §13 A1). The results directory is `$REPORTS_DIR` when bench-client set it, and the run id is
its name.

## 5. What comes next

- On the hub: the `perf-weekly` wrapper and units, `builds.json` from the build step, `hub.json` and the
  dashboard page (`bench-hub`), the T3 drill (Design §6.5.1 L11).
- In conbench: the `regression-case.json` ingest branch; in engine-suite: `build_fingerprint.sh`.
- Later: the canary redesign (Design §13 13), build isolation (§13 12), the T1 thresholds for the
  contamination guard and the minimum absolute counter change.
