# ADR-013: How regression equivalence is proven

- **Status:** **Accepted** (2026-09-02)
- **Trigger:** Phase 3 exit criterion needs an operational definition
- **Depends on:** ADR-003 (freeze grades) · ADR-004 (`shell` first)

---

## Context

ROADMAP Phase 3 exits on *"regression equivalence against the existing result"*. That phrase has no
operational meaning on its own, and `design/contracts.md` only narrowed it to *"compare `Sink`
output"*. Sink output contains things that **cannot** match between two runs: the timestamp in the
result directory name, absolute paths, elapsed times, and the order in which cases finish, since
workers pull from a shared queue.

Without a definition, "equivalent" gets decided case by case while looking at a diff, which is how a
freeze quietly stops meaning anything.

## Options

1. **Compare per-case verdicts only** — the map `{case → OK|NOK}`. Cheap, and blind: every F1
   surface could be broken and this still passes.
2. **Compare selected fields** — markers and counts, no whole-file diff. Turns into an argument about
   which fields count, every time.
3. **Normalise, then diff to zero.** Define exactly what may vary, mask it, require the rest to be
   identical.

## Decision

**Option 3.** The normalisation rules are a committed artifact, and that file becomes the executable
definition of what is frozen and what is not.

**Masked** — may differ between runs:

1. the timestamp in `<CTP_HOME>/result/<task>/<ts>/`
2. absolute path prefixes — `CTP_HOME`, the scenario root
3. elapsed values — `totalTime`, `Elapse Time`, per-case durations
4. case completion order (sort before comparing; `dispatch_tc_FIN_<env>` keeps its per-env membership,
   so no information is lost)
5. hostnames, pids, ports

**Not masked, and fixed as a precondition instead:** build id, bit width, charset. Masking these
would leave the evidence unable to state that both sides ran against the same build.

**Corpus.** The exit evidence runs the whole shell corpus — `cubrid-testcases-private-ex/shell`,
3,722 cases. The development loop uses `_01_utility` (234 cases) as a smoke set: deterministic,
low external dependency, and chosen for a reason that can be stated.

Excluded from the evidence, each with a reason:

| Excluded | Why |
|---|---|
| `_25_unstable` (204) | Counted separately, not dropped. Its own `readme.txt` says these depend on elapsed time, memory size and whether anyone else is using the machine. Mixing them in lets noise bury the signal; dropping them silently would be hiding the inconvenient part |
| HA tree (162) | Needs a master/slave topology that does not exist yet. `DeployHA` is still ported — it is axis T — but it ships **unverified**, and that is recorded rather than glossed |
| `manually` (12) | Documented as human cases, not automation targets |
| Windows | Officially stale (ADR-003 revision). The native runner refuses it |

## Consequences

1. `impl/m1/normalization.md` (or its executable equivalent) becomes a Phase 3 deliverable, and
   changing it is a change to the freeze.
2. Phase 3 exit needs two runs over 3,722 cases, old and new. That cost is the point: a smaller
   corpus would make "equivalent" mean less.
3. `_25_unstable` produces a second, separate report. A difference there is a finding to look at,
   not a failure.
4. `DeployHA` ships unverified. It must be listed as such in `impl/m1/regression-evidence.md`, not
   left for someone to discover.
