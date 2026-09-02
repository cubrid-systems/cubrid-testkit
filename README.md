![cubrid-testkit — goal: replace CTP one task at a time while everything the outside can see stays byte-identical, and separate what tests the engine from what merely runs QA around it](docs/assets/banner.svg)

**cubrid-testkit** is the successor to CTP, the functional test runner for the CUBRID engine.
It replaces CTP **one task at a time**, and while that is going on, nothing anyone outside can
observe is allowed to change: the same commands, the same config keys, the same markers on stdout,
the same result files, the same exit codes.

The second thing it does is subtract. CTP grew a scheduler, a mailer, an issue filer and a message
queue inside the test runner. None of that decides whether a test passes, so none of it is being
carried over.

For engine developers and QA. Part of
[CUBRID Systems Research](https://github.com/cubrid-systems).

> **Where it is:** design, not code. Phase 0 (analysis) and Phase 1 (concept and surface freeze) are
> done, Phase 2 (architecture) is done, and Phase 3 — rewriting the `shell` task in Go — has not
> started. See [Status](#status).

## Why replace CTP at all

CTP works. The problem is what it became on the way there, and Phase 0 measured it rather than
asserting it:

| What analysis found | Where |
|---|---|
| Three different ways to invoke a task: shell-out, in-process reflection, and a special case that escapes the loop before it starts | `analysis/_overview/cli-tree.md` |
| 21 declared task names, of which **7 are silent no-ops** — no branch, no error, nothing happens | `analysis/_overview/orphan-enums.md` |
| Commands smuggled to the parent shell as stdout lines carrying a `#SCRIPTCONT` suffix | `cli-tree.md` |
| Third-party dependencies from 2005–2016, including log4j 1.2, which is end-of-life | `ADR-001` |
| **15 entry points**, not one — Phase 0's own CLI map covered a single branch and missed nine | `cli-tree.md` Appendix A |

That last row is the honest one. The analysis that justified this project had a hole in it, found
later by re-running the survey properly. It is written down where it happened rather than quietly
fixed.

## Architecture

![Architecture: frozen entry scripts call one Go binary, which routes each task either to a native runner or to the old CTP as a subprocess; both produce the same frozen output, and QA operations are excluded](docs/assets/architecture.svg)

One Go binary. Every entry script becomes a shim that hands its arguments over unchanged. Inside,
a single registry decides where a task goes:

- a **native runner** if that task has been rewritten, or
- the **legacy runner**, which runs the original CTP as a subprocess, reproducing its argv and
  environment byte for byte.

Both paths write the same frozen output. That is the whole trick: the surface is a property of the
output layer, not of which implementation happened to run.

There is no jar-compatibility layer and there never will be — old modules do not call into new code.
They keep their own jars, which keep being built, until their turn comes.

## The two axes

CTP mixes two things that have no reason to be in the same program.

| | |
|---|---|
| **Axis T — running tests** | Pick cases, run them, judge them, write results. If it changes the *result*, it is axis T. |
| **Axis O — running QA** | Decide when to run, tell people, file the issue. If the result is the same without it, it is axis O. |

Only axis T is being migrated. Axis O is excluded, and the exclusions are written down with reasons
in [`concept/migration-exclusions.md`](docs/concept/migration-exclusions.md) rather than deleted
quietly. It gets rebuilt later as a separate layer that *consumes* this system's output — one
direction only, which is exactly what the current design gets wrong.

The split is not a matter of files or packages. `coreanalyzer` holds both: analysing a core dump is
axis T because the `CORE_FILE:` marker is part of the frozen surface, while filing the resulting
issue is axis O. `run_shell.sh` splits down the middle of a single option list — eight flags stay,
five go.

## The frozen surface

[`concept/external-surface-freeze.md`](docs/concept/external-surface-freeze.md) is the normative
document. Everything in it carries a grade:

| | |
|---|---|
| **F1** | byte-identical. Something out there greps this. |
| **F2** | same meaning; order, spacing and extra detail may differ |
| **F3** | must be accepted as input; the internal representation is free |
| **NF** | not frozen |
| **unsettled** | no grade yet — and it says when that gets resolved, rather than defaulting to NF |

Some of it is deliberately ugly and stays that way. `main.info` uses `:` as its separator and
`summary_info` uses `=`; both are F1 and they will not be harmonised. `-1` is returned as an exit
code and reaches the shell as 255; it will not be tidied into `1`.

Grades come from evidence. `found core file` is F1 because two test cases in a frozen repository
grep for it — `bug_bts_22449.sh` and `cbrd_21070.sh`. Other markers are F1 for the weaker reason
that no consumer was found and the conservative call was made.

## Layout

```
cubrid-testkit/
├── CONTEXT.md              the glossary. task ≠ suite ≠ module ≠ runner, and that distinction
│                           has already caught one design error
├── docs/
│   ├── ROADMAP.md          phases, exit conditions, risks, the quarterly re-evaluation gate
│   ├── adr/                decisions, numbered, with README.md as the numbering authority
│   ├── concept/            Phase 1 — north star, the freeze, non-goals, exclusions
│   ├── design/             Phase 2 — architecture, contracts, one doc per module
│   ├── analysis/           Phase 0 — what CTP actually does, measured
│   ├── extensions/         §6a — testing axes CTP never had
│   └── survey/             the DBMS testing ecosystem, classified into eight axes
├── ext/
│   └── cubrid-sqlancer/    submodule — §6a-E3, a SQLancer provider for CUBRID
└── impl/                   Phase 3+ — empty until the shell rewrite starts
```

## Status

| Phase | | |
|---|---|---|
| 0 — analysis | **done** | 39 documents. Appendix A of the CLI map was added later, after the first survey turned out to have covered one entry point out of fifteen |
| 1 — concept and freeze | **done** | north star, the freeze spec with a 24-row old↔new mapping, non-goals NG1–NG11, migration exclusions |
| 2 — architecture | **done** | architecture, five contracts, four module documents |
| **3 — rewrite `shell`** | **not started** | `shell` · `rqg` · `unittest`, Linux only |
| 4 — the rest | — | `sql` family, `isolation`, `ha_repl`, `cdc_repl`, `jdbc` |
| 5 — retire | — | isolate what is no longer called; decide what to keep |

Settled, and worth knowing before reading further:

- **Go**, one binary, `go build` plus a Justfile ([ADR-001](docs/adr/ADR-001-implementation-language.md), [ADR-002](docs/adr/ADR-002-build-tool.md))
- **`shell` goes first** ([ADR-004](docs/adr/ADR-004-first-replacement-candidate.md)) — it absorbs the infrastructure the other three shell-family modules share
- **`jdbc` does not come along.** It shares a jar with `shell`, but it is a separate entry point that calls that jar directly, so it keeps working untouched. An earlier draft claimed otherwise
- **RMI worker mode is retired.** It is unreachable with what ships: the config key defaults to `ssh`, appears in none of the 13 config files, its agent config is not shipped, and nothing launches its server. Asking for it now fails loudly instead of silently falling back
- **Windows is stale** and out of scope. The native runner refuses it and says so
- Regression equivalence is proven by **comparing normalised output over the whole shell corpus** — 3,722 cases, with the 204 in `_25_unstable` counted separately because their own readme says they depend on machine load and elapsed time

## Where to start reading

| If you want | Read |
|---|---|
| the vocabulary | [`CONTEXT.md`](CONTEXT.md) |
| what this system is for | [`concept/north-star.md`](docs/concept/north-star.md) |
| what may never change | [`concept/external-surface-freeze.md`](docs/concept/external-surface-freeze.md) |
| what was left out, and why | [`concept/migration-exclusions.md`](docs/concept/migration-exclusions.md) |
| how it is built | [`design/architecture.md`](docs/design/architecture.md) · [`design/contracts.md`](docs/design/contracts.md) |
| what happens next | [`ROADMAP.md`](docs/ROADMAP.md) · [`design/module-shell.md`](docs/design/module-shell.md) |
| every decision so far | [`adr/README.md`](docs/adr/README.md) |

## Conventions

This README and all commit messages are written in **English**, matching the other repositories in
this organisation.

The working documents under `docs/` are in **Korean** and stay that way — they were written that
way through Phases 0–2 and translating six thousand lines would buy nothing but a chance to
introduce errors into the one place where precision matters most. New material added at the repo
level is English; `docs/` continues in Korean.

Every decision that was hard to reverse gets an ADR, numbered in
[`docs/adr/README.md`](docs/adr/README.md), which is the single authority on numbering — two
documents once assigned `ADR-005` to different things, and that is how it was caught.

When something turns out to be wrong, the correction says so where the mistake was made. This
README's Status section names two: a CLI survey that covered one entry point out of fifteen, and a
claim that `jdbc` had to be rewritten alongside `shell`.
