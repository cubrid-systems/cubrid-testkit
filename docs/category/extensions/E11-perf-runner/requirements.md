# E11 — Weekly Performance-Regression Runner (Requirements)

*English · [한국어](requirements.ko.md)*

**Source:** cubrid_cv `plan/perf_regression/` — PROPOSAL, Spec v0.1.2, Design v0.1.2 (the interface
and the algorithms live there; this entry says what of it enters testkit, and why here)
**Status:** incubating — **in progress** (ADR-EXT-011 proposed 2026-10-02; `perf validate` and
`perf list` are in the tree, `session` and `run` follow in M2)
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
  with the four flags the sandbox gained for it (`single` with `ha_mode=off`, `--set` to
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
the directory, a counter off the collect layer's list, `repeats < 3`, `warmup < 1`,
`tolerance ≤ 0`, a client shaped for another driver, a restored snapshot with `warm_s = 0`, and a
fixture the suite does not have or has at another version. `branches.conf` refuses an unknown key,
a line without `owner=`, a `repo=` that is not `owner/repo`, a date that is not a date, a glob
that cannot match, and a branch registered twice. `perf.conf` is a closed key set too.

## 5. What comes next

- **M2** — `session` and `run`: `internal/sandbox` gains the create options, the ABBA passes,
  L0 and statdump collection, judgment, sidecars (Design §6.1).
- **M0** — the hub checks the Design depends on: csb under rootless podman, cpuset delegation,
  replacing a bind-mounted directory's contents, reflink timing (Design §12).
- The perf-client image and the first canaries, in engine-suite.
