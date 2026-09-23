![cubrid-testkit — finds the cases, runs them, judges them, records what happened. Every task it takes over keeps the same commands and the same output. Shown: a run marking cases OK and NOK, and the summary it writes out.](docs/assets/banner.svg)

**cubrid-testkit** runs CUBRID's functional tests. Find the cases, run them against an engine,
decide whether each one passed, write down what happened — and nothing else.

It is a drop-in for CTP's `bin/ctp.sh`: the same task names, the same config keys, the same markers
on stdout, the same result files, the same exit codes. Tasks that have been rewritten in Go run
here; the rest are handed to the original CTP as a subprocess, and from outside there is no way to
tell which is which. That is what lets the old system keep running while the new one takes over one
task at a time.

For engine developers and QA. Part of
[CUBRID Systems Research](https://github.com/cubrid-systems).

| | |
|---|---|
| [Quick start](#quick-start) | build it and run a suite |
| [What runs where](#what-runs-where) | which of the fourteen tasks is native yet, and what each gate needs |
| [Using it](#using-it) | the command line, the tasks, `run-shell`, and what a run leaves behind |
| [The categories](#the-categories) | the as-built guide for `shell`, `sql`, `isolation` and the two HA suites |
| [How it works](#how-it-works) | routing, the shim, the three decisions that are the whole design, and the frozen surface |
| [Why you can trust it](#why-you-can-trust-it) | what each gate has actually produced, family by family |
| [Layout](#layout) | the repository, and where to start reading |

## Quick start

```bash
go build -o bin/testkit ./cmd/testkit

TESTKIT_CONTAIN=1 TESTKIT_NATIVE=shell testkit shell -c shell.conf
```

That is the `shell` suite, run natively, in slots — as many as this machine has measured it can
hold ([below](#using-it)), and `testkit sizing shell` says what it would take and why. Drop
`TESTKIT_NATIVE` and the same command hands the task to CTP instead — the same invocation either
way, which is what lets the two be compared at all. Whether the output then matches is not claimed here; it is [gated](#what-runs-where), family by
family.

**What you need:**

| | Why |
|---|---|
| **Go 1.25+** | to build the binary (`go.mod`) |
| **A CTP checkout** | any task that has not been rewritten runs the original as a subprocess: `CTP_HOME`, and `JAVA_HOME` for that path |
| **A CUBRID build** | the engine under test, with its own install and `CUBRID_DATABASES` |
| **A testcases checkout** | the cases themselves, for the tasks that read a corpus |

Linux. Windows is stale and out of scope — the native runner refuses it and says so.

No root and no container runtime is needed: the namespaces and the overlay are both unprivileged.
It also runs inside Docker, which needs `--security-opt seccomp=unconfined` and
`--security-opt systempaths=unconfined` but not `--privileged`.

**The runner fixes `/bin/sh` for you.** The shell suite has always needed a bash-compatible
`/bin/sh` — CTP's own `init.sh` opens with `function get_os(){` — and the runner now arranges that
itself, binding bash over `/bin/sh` inside its own mount namespace. The machine is not changed and
nothing outside the run sees a different shell; on a machine that already has it right the bind is
skipped. Set `TESTKIT_CONTAIN_SH` to override. Without it, on a distribution where `/bin/sh` is
dash, every case dies on `init.sh`'s first line — measured: seventeen cases, seventeen blank
results, seventeen `Syntax error: "(" unexpected`.

**Building and testing:**

```bash
go build -o bin/testkit ./cmd/testkit
go test ./... -count=1
```

The tests under `internal/cli` are the frozen command line written down, so a failure there is a
contract change rather than a broken refactor. CI runs gofmt, `go vet`, the tests and the build.

## What runs where

This is the one place the migration's state is recorded; everything else in this README defers to it.

![Fourteen tasks in three states: eight still CTP's as a subprocess with argv and env byte for byte; five native but opt-in behind TESTKIT_NATIVE — shell and rqg, sql and medium, isolation; and unittest native with nothing to set. Below, what each switch is waiting for: ADR-013 open for shell, ADR-017 passed for sql, ADR-018 met for isolation.](docs/assets/status.svg)

| Task | Runs | Gate |
|---|---|---|
| `unittest` | natively, nothing to set | — |
| `shell` · `rqg` | natively behind `TESTKIT_NATIVE=shell` | [ADR-013](docs/project/adr/ADR-013-regression-equivalence.md) — gate **open** |
| `sql` · `medium` | natively behind `TESTKIT_NATIVE=sql` | [ADR-017](docs/project/adr/ADR-017-sql-equivalence.md) — gate **passed** |
| `isolation` | natively behind `TESTKIT_NATIVE=isolation`, `runone.sh` still executing every case | [ADR-018](docs/project/adr/ADR-018-isolation-equivalence.md) — gate **met** |
| `isolation`, with its controller too | `TESTKIT_ISOLATION_CTL=1` as well: testkit's own controller in place of ctltool's `qactl`, keeping `qacsql` and `runone.sh`. 15.7% faster over the whole corpus | [ADR-019](docs/project/adr/ADR-019-isolation-controller.md) — gate **does not pass**: 27 cases whose answers record the order `qactl`'s pauses produced. Six are carried as patches here; the rest need an answer re-recorded, and the gate is then run on the patched corpus ([ADR-018](docs/project/adr/ADR-018-isolation-equivalence.md) 3a, 6) |
| `ha_repl` | natively behind `TESTKIT_NATIVE=ha_repl`, against a pair `cluster-sandbox` stands up | **no parity gate, and the reason is recorded.** CTP reaches this corpus through a conversion that deletes every `CALL` and almost every `SELECT`, so parity would be a claim about a different corpus ([ADR-015](docs/project/adr/ADR-015-beyond-axis.md), amended 2026-09-23) |
| `kcc` `neis05` `neis08` `sql_by_cci` `cdc_repl` `jdbc` `webconsole` — and any family above whose switch is unset | CTP, as a subprocess, unchanged | — |

`TESTKIT_NATIVE` names the families, comma-separated, and `all` is every one of them.
`TESTKIT_NATIVE_<FAMILY>=1` — `TESTKIT_NATIVE_SHELL=1`, `TESTKIT_NATIVE_SQL=1`,
`TESTKIT_NATIVE_ISOLATION=1`, `TESTKIT_NATIVE_HA_REPL=1` — does the same for one family. Either turns the same registration on;
neither turns the other off.

A switch is opt-in until its **gate** clears. A gate is not an opinion about the code — it is a
comparison of two runners' *files* over a corpus, specified in an ADR and run by a harness. What
each one has produced so far is [the evidence](#why-you-can-trust-it).

Seven more names — `cci` `dots` `nbd` `sysbench` `tpcc` `tpcw` `ycsb` — are ones CTP accepted and
silently did nothing about. They now say they are retired and move on to the next task: the same
outcome, without the silence.

**Phases.** The rewrite is in phases, and the exit condition of each is written down in
[`project/ROADMAP.md`](docs/project/ROADMAP.md):

| Phase | | |
|---|---|---|
| 0 — analysis | **done** | 38 documents on what CTP actually does |
| 1 — concept and freeze | **done** | north star, the freeze spec with a 24-row old↔new mapping, non-goals NG1–NG11, migration exclusions |
| 2 — architecture | **done** | architecture, five contracts, four module documents |
| **3 — rewrite `shell`** | **in progress** | `run-shell` complete with its six axis-T options; slots, a corpus that cleans itself, per-case patches and a progress page are in and measured. What is left is ADR-013's gate above |
| **4 — the rest** | **in progress** | `sql`, `medium` and `isolation` rewritten and gated as above; the other eight tasks still CTP's |
| 5 — retire | — | isolate what is no longer called; decide what to keep |

## Using it

```
testkit <task>... [-c <conf>] [--interactive] [-h] [-v]
```

```bash
testkit shell -c shell.conf
testkit sql medium -c ~/CTP/conf/sql.conf     # several tasks, in the order given
TESTKIT_NATIVE=shell testkit shell -c shell.conf
```

Task names are matched case-insensitively. A name that is not a task prints help and skips that
task only — the tasks after it still run. `CTP_HOME` comes from the environment if it is set,
otherwise from the parent of the binary.

Four names are not tasks. `testkit sizing [suite] [mode]` prints what this machine would run at
once and the runs it read to decide, and runs nothing ([below](#using-it));
`testkit check-cases <scenario> [<init_path>]` reads a corpus and reports the cases that **cannot
fail** — a misspelt `write_nok`, a comparison of a file against itself, a case with no route to a
failure at all — and exits 1 when it finds one, so it can gate a branch of the cases; and
`isolation-ctl` and `isolation-ctl-install` are the isolation controller and its installer, which a
run invokes for itself and which are documented in
[ADR-019](docs/project/adr/ADR-019-isolation-controller.md).

`check-cases` needs no engine, no database and no containment — it reads text. Against the shell
corpus it finds **seven misspelt verdict calls and three self-comparisons** in 3,475 cases, and
against the HA corpus one in 373 (`design/module-ha.md` P7). Those eleven are staged as patches
rather than fixed here, because the corpus is not this repository's — the patch set's own README
has them case by case, and the runbook for clearing them upstream.

The runner runs on **one machine** ([ADR-014](docs/project/adr/ADR-014-one-machine.md)): local by
default, and a remote machine over SSH is still one machine. RMI worker mode is retired and asking
for it fails loudly rather than falling back.

A contained run sizes itself unless `parallel_slots` says otherwise
([ADR-020](docs/project/adr/ADR-020-sizing.md)). It starts where the suite was verified — four
slots for `isolation` and `sql`, one for `shell` and `medium` — and each later run may go to twice
what this machine has run, bounded by memory (what a slot cost in this machine's own runs, with
15% on top), by processors, and by the *knee*: when more slots were measured to be no faster, the
count stays where it was fastest. `parallel=conservative` or `aggressive` narrows or widens all of
that. The measurements are kept in `~/.local/state/testkit/sizing/`, outside the checkout, and
`testkit sizing <suite>` shows them and what they decide. An uncontained run is serial, as CTP's
was. A value you write is used as written, whatever the machine; either way the run says on stderr
which it chose and why. A slotted shell or isolation run writes the record files a serial run writes — slots share
an environment id on purpose — and only the console, with an `[ENV START]` for each slot, shows
how many there were.

### `run-shell` — one case, looped

The other CLI tree — one case, looped until it fails, which is what you reach for after the suite
has told you a case is unreliable:

```bash
testkit run-shell --loop --maxloop 200 _01_utility/_38_csql/csql1
testkit run-shell -h
```

`--loop`, `--maxloop`, `--maxtime`, `--extend-script`, `--prompt-continue` and `-h` are the six
axis-T options of CTP's `run_shell.sh`; the testcase argument may name the case directory, its
`cases/` subdirectory, or a file in either, and defaults to the working directory. Touching a file
named `STOP` in the case directory ends the loop after the attempt in flight. The seven
QA-operations options — `--update-build`, `--enable-report`, `--mailto` and the rest — say so by
name instead of being ignored.

### What a run leaves behind

The result files are part of the frozen surface, so they are the same whichever runner produced
them. A shell run leaves ten:

![A shell run in five stages — check the machine, find the cases, run each in a slot, judge its .result, write it down — and the ten frozen files they leave: six held to zero differences against CTP, four classified.](docs/assets/shell-run.svg)

The sql family writes CQT's own records instead — `main.info`, `summary_info`, `summary.xml`, the
JUnit report and a `.result` beside every case; [`category/sql/`](docs/category/sql/README.md) lists
them.

One file is this runner's own, and it is absent unless it has something to say: `patched.txt`, the
cases that did not run as the corpus has them. The patches themselves are in
[cubrid-testkit-patches](https://github.com/cubrid-systems/cubrid-testkit-patches), which is
private, and its README says what each one is for.

## The categories

Each family has an as-built guide — the stages, every configuration key with what it costs, how to
run it on a host and in Docker, and how to read a failure. `ha-shell` is the one that is still
shorter, and deliberately: the parts that are not built are named rather than described.

| | |
|---|---|
| **[`category/shell/`](docs/category/shell/README.md)** | the task this project rewrote first, and the one with options: parallel slots on one machine, a corpus that cleans itself up, per-case patches, a progress page, and the memory ceiling that fails a run when it is sized wrong |
| **[`category/sql/`](docs/category/sql/README.md)** | `sql` and `medium` — the stages and the executor, what parallel buys and what it costs on the machine you have, and how to read a failure that is the corpus's order rather than the engine's |
| **[`category/isolation/`](docs/category/isolation/README.md)** | the stages and what `runone.sh` does with a case, the `.ctl` language as `qactl` reads it, every key, and the cases CTP cannot reproduce either |
| **[`category/ha-shell/`](docs/category/ha-shell/README.md)** | the `shell` task against a pair — what it establishes, the verbs a case gets, and the three rules the frozen corpus breaks |
| **[`category/ha-repl/`](docs/category/ha-repl/README.md)** | the `sql` corpus run across a pair, with the case's own reads as the oracle — the nine verdicts, every key, running one corpus over several pairs from one process, the status page and `testkit watch` |
| **[`category/extensions/`](docs/category/extensions/README.md)** | testing axes CTP never had — E1–E10 over eight axes: sqllogictest, SQLancer, SQLsmith, distributed isolation, parser and storage fuzzing, differential, workload, XASL fixtures |

## How it works

The compatibility is not a promise made in prose; it is where the binary sits. The endpoint is that
a QA machine keeps calling `bin/ctp.sh` and gets this runner, and nothing that reads the output can
tell.

![How testkit routes a task: a command typed today, or by bin/ctp.sh once its shim is in place, reaches one registry, which sends unittest, shell and rqg to shellsuite, sql and medium to sqlsuite, isolation to isolationsuite — each family behind TESTKIT_NATIVE — and everything else to the original CTP as a subprocess; every path writes the same frozen output.](docs/assets/dispatch.svg)

**The shim is not in place yet** — the dashed box. `bin/ctp.sh` in `cubrid-testtools` is still the
original, and it should stay that way until the corpus comparison clears — the gate is what earns
the swap. Today you reach this runner by invoking it directly, which is what the comparison harness
does when it runs both sides on the same shard. Everything below the shim is built and running.

Three things follow, and they are the whole design.

**The entry scripts stay.** `bin/ctp.sh` becomes a shim that hands its arguments over unchanged, so
every Jenkins job, every `docker-entrypoint.sh`, every habit keeps working — no caller is edited and
nothing has to be migrated on a schedule. The command line is already frozen and tested as such:
`internal/cli` is that contract written down, which is what makes the eventual swap a one-line
change rather than a negotiation.

**A task is routed, not converted.** Legacy registers first and claims all fourteen tasks; a native
runner registered after it takes over the ones it names. The legacy path reproduces CTP's argv and
environment byte for byte for everything else. There is no jar-compatibility layer: old modules keep
their own jars, which keep being built, until their turn comes.

**The output belongs to neither.** Every path writes through the same result layer, because the
surface is a property of the output rather than of whichever implementation ran
([ADR-003](docs/project/adr/ADR-003-external-surface-freeze.md)). That is what lets a task move from
one side to the other without anything downstream noticing — and what makes the equivalence gate
meaningful, since it compares two runners' files rather than two runners' intentions.

[`project/design/architecture.md`](docs/project/design/architecture.md) and
[`project/design/contracts.md`](docs/project/design/contracts.md) are the specifications.

### The frozen surface

[`project/concept/external-surface-freeze.md`](docs/project/concept/external-surface-freeze.md) is
normative, and everything in it carries a grade you can rely on:

| Grade | Means |
|---|---|
| **F1** | byte-identical. Something out there greps this |
| **F2** | same meaning; order, spacing and extra detail may differ |
| **F3** | must be accepted as input; the internal representation is free |
| **NF** | not frozen |
| **unsettled** | no grade yet, with the date that gets resolved |

Some of it is deliberately ugly and stays that way. `main.info` separates with `:` and
`summary_info` with `=`; both are F1 and will not be harmonised. `-1` is returned as an exit code
and reaches the shell as 255; it will not be tidied into `1`.

**The freeze preserves what CTP did, not what CTP got wrong.** Where a behaviour is plainly
unintended and reproducing it would make someone trust something false, it is fixed and the decision
recorded. Four so far: the requirements check now actually fails on a missing command — it used to
match csh's wording and so reported `PASS` for everything absent; `dos2unix` is off the checked
list, because no answer file in the corpus has a CRLF — though CTP's `init.sh` still calls it, so a
machine without it gets a stand-in (`internal/contain/dos2unix.go`); a build with no commit
suffix is now just its version instead of `11.2.0.0000) (64bit release build for linux_gnu`; and a
server that dies of a signal now **fails its case**
([ADR-021](docs/project/adr/ADR-021-crash-reports.md)). CTP looks for `core.*` files and for
`FATAL ERROR`, and on a machine that hands cores to a crash handler there is neither — so a case
that killed a server was reported as an ordinary diff, which is how one of them passed unnoticed
through six whole-corpus runs. The run reads the report the engine writes itself, keeps it with the
results, and no longer copies the whole install into `~/error_backup` to do it.

### What is out of scope

Deciding *when* to run, telling people the result, and filing the issue
that comes out of it are a different job. They are listed with reasons in
[`project/concept/migration-exclusions.md`](docs/project/concept/migration-exclusions.md), and get
rebuilt later as a layer that consumes this system's output.

## Why you can trust it

A gate is not an opinion about the code. It is a comparison of two runners' *files* over a corpus,
specified in an ADR and run by a harness — which is why it can fail, and has.

**`sql` and `medium` — proven.** CTP and testkit over the whole of both corpora, serial, with the
engine, the cases and CTP all at upstream develop's head: medium identical in every file, and a
clean sql run byte-identical to a clean CTP run — 17,459 `.result` files and 2,762 record files
([`project/evidence/regression-sql.md`](docs/project/evidence/regression-sql.md)).

**`isolation` — met, and the caveat is the point.** CTP does not reproduce its own verdicts on this
corpus: two whole CTP runs in the same order disagree on seven of 6,772. That, not byte-identity, is
what the native runner had to be measured against — and every verdict it disagrees on belongs to a
case that flips under CTP too, with no rerun separating the two. Four slots also run the corpus
nearly four times faster than CTP alone. Where the hours actually go, and which fix belongs upstream
in `cubrid-testtools` rather than here, is in
[`project/evidence/isolation-baseline.md`](docs/project/evidence/isolation-baseline.md).

**`shell` — judged, gate still open.** Equivalence here cannot be byte-identity, because some of
what a run writes cannot match between two runs — paths, times, dates. So both sides are normalised,
and then held to different standards:

![Equivalence for one shard: the same cases run on CTP and on testkit, normalize.sh masks what may differ, the six files that carry verdicts are held to zero differences with no baseline, the other four are classified through baseline.txt, and whatever no rule explains is reported as NEW.](docs/assets/equivalence.svg)

At develop head 3,216 cases were judged, 3,184 passed, and every one of the 32 failures was
attributed; the first four-case comparison and the full-corpus run are
[`regression-shell.md`](docs/project/evidence/regression-shell.md) and
[`parallel-shell.md`](docs/project/evidence/parallel-shell.md). `TESTKIT_NATIVE=shell` stops being
opt-in when the comparison clears over the *whole* corpus
([ADR-013](docs/project/adr/ADR-013-regression-equivalence.md)) — too many hours to run by hand, so
[`project/evidence/compare/`](docs/project/evidence/compare/README.md) runs it shard by shard, with
`shard-clean.sh` putting the tree back between the two runs because a case that leaves a database
behind would otherwise hand it to whichever runner goes second. The 48 cases that cannot pass on any
machine but the one they were written on are patched **in the corpus**, before either runner, so both
read the same source and the report names which they were.

**`ha_repl` — no gate, and that is the finding.** Reading CTP's conversion showed it deletes every
statement beginning with `CALL` — 1,707 catalog-method calls and 10,257 procedure calls across the
corpus — and every `SELECT` that does not say `INCR` or `DECR`, which is to say the cases' own reads.
So there is no CTP verdict for several of these findings to be diffed against, and
[ADR-015](docs/project/adr/ADR-015-beyond-axis.md) was amended rather than waived: where the
baseline cannot express the question, parity is not the gate. What this runner ships on instead is
its own reproductions — each finding in
[`project/evidence/ha/`](docs/project/evidence/ha/README.md) carries one, small enough to re-run by
hand on a pair.

**Corrections are recorded where the mistake was made** — the CLI survey that justified the project,
the freeze specification, the corpus counts, the first difference this runner was wrongly cleared
of, and 33 more found while measuring sql, medium and isolation under CTP — and reading `qactl` —
before rewriting them
([`project/evidence/spec-corrections.md`](docs/project/evidence/spec-corrections.md)).

## Layout

```
CONTEXT.md               the glossary — task ≠ suite ≠ module ≠ runner
cmd/testkit/             the entry point
internal/                cli · conf · registry · dispatch · runshell · exec · result · feedback ·
                         topology · runner (legacy, shellsuite, sqlsuite, isolationsuite,
                         hareplsuite) · sandbox (drives cluster-sandbox, reads its JSON) ·
                         contain (namespaces per slot) · plan (case durations) ·
                         patch (per-case patches) · sizing (how many slots, from what this
                         machine measured) · coredump (a dead server's stack, and the crash
                         report it wrote itself) · ctl (the isolation controller) ·
                         status (the progress page)
overrides/               what this run does differently from the corpus as it stands
  patches/               fixes carried for the corpus until upstream takes them
  machine-exclusions/    cases this machine cannot run, each with the reason and what ends it
scripts/sizing.sh        what this machine measures — the disk, the cores — and how big a shell
                         ceiling to ask for. The slot count is the run's own (ADR-020):
                         `testkit sizing <suite>` prints that decision
extensions/              separate repositories testkit will drive
  cubrid-sqlancer/       submodule — a SQLancer provider for CUBRID
  cluster-sandbox/       submodule — the tool that provisions the multi-node
                         topologies HA testing needs. `internal/sandbox` drives
                         it as a subprocess and reads its JSON
docs/
  assets/                the diagrams these pages use
  category/              how to run each test category, and what to set
    shell/  sql/         the as-built guides
    isolation/           the same, for isolation
    ha-repl/             the sql corpus across a pair, with the case's own reads as the oracle
    ha-shell/            the shell task against a pair; its corpus is still CTP's
    extensions/          testing axes CTP never had
  project/               why the rewrite exists and how it is built
    ROADMAP.md           phases, exit conditions, risks, the re-evaluation gate
    adr/                 decisions, numbered; README.md is the numbering authority
    concept/             north star, the freeze, non-goals, exclusions
    design/              architecture, contracts, one doc per module
    analysis/            what CTP actually does, measured
    evidence/            regression evidence, the normaliser, and the corrections
      compare/           the harness that runs the corpus and classifies what differs
    survey/              the DBMS testing ecosystem, classified into eight axes
```

### Where to start reading

| If you want | Read |
|---|---|
| the vocabulary | [`CONTEXT.md`](CONTEXT.md) |
| how to run the shell suite | [`category/shell/`](docs/category/shell/README.md) |
| how to run sql and medium | [`category/sql/`](docs/category/sql/README.md) |
| how to run isolation | [`category/isolation/`](docs/category/isolation/README.md) |
| what this system is for | [`project/concept/north-star.md`](docs/project/concept/north-star.md) |
| what may never change | [`project/concept/external-surface-freeze.md`](docs/project/concept/external-surface-freeze.md) |
| what was left out, and why | [`project/concept/migration-exclusions.md`](docs/project/concept/migration-exclusions.md) |
| how it is built | [`project/design/architecture.md`](docs/project/design/architecture.md) · [`project/design/contracts.md`](docs/project/design/contracts.md) |
| how equivalence is decided, and run | [`project/adr/ADR-013`](docs/project/adr/ADR-013-regression-equivalence.md) · [`project/evidence/compare/`](docs/project/evidence/compare/README.md) |
| how a run is made parallel | [`project/concept/beyond-axis.md`](docs/project/concept/beyond-axis.md) B-T3, B-T12, B-T13 |
| where the run's hours go, and what to do next | [`project/concept/beyond-axis.md`](docs/project/concept/beyond-axis.md) B-T14 |
| what the old system could fix cheaply | [`project/evidence/ctp-improvements.md`](docs/project/evidence/ctp-improvements.md) |
| what happens next | [`project/ROADMAP.md`](docs/project/ROADMAP.md) · [`project/design/module-shell.md`](docs/project/design/module-shell.md) · [`project/design/module-isolation.md`](docs/project/design/module-isolation.md) |
| every decision so far | [`project/adr/README.md`](docs/project/adr/README.md) |

The guides for shell, sql and isolation, the evidence, and the later ADRs are in English. The
analysis, the concept and design documents, the roadmap, the extension notes under
`docs/category/extensions/` and the first ADRs are in Korean and stay that way — a freeze
specification is worth exactly what its sentences are worth, and re-writing six thousand lines of
analysis buys nothing but a chance to introduce errors.
