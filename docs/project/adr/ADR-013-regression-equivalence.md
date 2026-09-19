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

1. ~~the timestamp in the result directory name~~ — **struck 2026-09-02.** There is no timestamp:
   the directory is `result/<category>/current_runtime_logs`, a fixed name. The freeze spec said
   otherwise and this list inherited the error. Nothing to mask, and one fewer rule to write.
2. absolute path prefixes — `CTP_HOME`, the scenario root
3. elapsed values — `totalTime`, `Elapse Time`, per-case durations
4. case completion order (sort before comparing; `dispatch_tc_FIN_<env>` keeps its per-env membership,
   so no information is lost)
5. hostnames, pids, ports
6. **case dispatch order, and which environment ran which case** — added 2026-09-02. `dispatch_tc_ALL.txt`
   records the list in `find` order, and `find` order is `readdir` order: three consecutive runs over
   the same unchanged tree produced three different lists. CTP cannot reproduce this file against
   itself, so comparing it byte for byte would only measure the filesystem. Sort both sides; the set
   is the contract. The new runner sorts at discovery, which makes its own reruns identical —
   a deviation in CTP's favour, recorded in `evidence/spec-corrections.md`

7. **dates, wherever they appear** — added 2026-09-06. The rules masked a date only when a time
   followed it, and the weekday form only with a zero-padded day and no meridiem. Two runs an hour
   and a half apart across midnight therefore differed on every `date` a case calls: 48 lines in the
   first comparison, none of them about the runners. The server error log's `_<YYYYMMDD>_<HHMM>`
   name goes with them.

**Not masked, and fixed as a precondition instead:** build id, bit width, charset. Masking these
would leave the evidence unable to state that both sides ran against the same build.

**Not masked, and left standing:** the environment a case inherits. A case under CTP sees three JVM
library directories at the head of `LD_LIBRARY_PATH` that a case under testkit does not, because
CTP's entry point is `java` and the JDK 8 launcher reorders the variable (`regression-shell.md`
§3-5). The comparison harness avoids provoking it, but the difference is real on any machine whose
profile puts a JVM directory mid-path, and masking it would hide the one place where the two
runners genuinely hand a case something different.

**Measured before judged** — added 2026-09-08. A difference is not a finding until the cases it
involves are known to reproduce. Before a difference between the two runners can be attributed to
the runners, **one runner is run twice over the same shard**, and the cases that disagree with
themselves are counted separately -- as `_25_unstable` already is, and for the same reason rather
than as a convenience.

This is the argument that removed `dispatch_tc_ALL.txt` from being gradeable, moved from a file to a
verdict: **a case whose verdict one runner cannot reproduce against itself cannot be evidence that
two runners differ.** Skipping the step is not hypothetical. The first 217-case comparison reported
five disagreements as runner differences; none of them was one.

**It is a gate on findings, not a third full run.** Running the whole corpus a third time would cost
as much as the comparison itself. The self-check is owed only where the comparison actually found
something: a shard that compares clean needs no alibi, and a shard that does not is not evidence
until its cases have produced one. `evidence/compare/selfcheck.sh` is the instrument.

**What a clean self-check licenses is narrow.** Over `_01_utility` it found every case agreeing --
173 OK and 44 NOK twice, the same cases in each column. That is one observation about one runner in
one window; it does not make the corpus stable, and in the same window CTP disagreed with *itself*
on `itrack01`. What it supports is only this: the differences that remain are not explained by a
corpus that cannot reproduce itself.

**The by-product is the more useful half.** Two runs of one runner are the floor under which no
difference means anything -- 24 new lines in `feedback.log` against 2,874 between the runners, and
the four files that carry verdicts identical even across runs. Without that number, "2,874 lines
differ" is a quantity with nothing to compare it to.

**A patched case is judged, and named** — added 2026-09-19. Some cases cannot pass on this machine for
reasons that are neither runner's: they assume `$CUBRID` sits under `$HOME`, that `[common]` is empty,
that the linker resolves libraries in an order it has not for years. `cubrid-testkit-patches/shell` carries
the 48 diffs that fix them, and **the gate is run on the patched corpus**.

Where the patch is applied is the decision, not whether. `evidence/compare/shard.sh` puts it in the
**tree**, before CTP runs and out again after testkit has, and drops `case_patch_dir` from the conf it
generates. A patch is a diff against a case; which runner executes the patched case is not part of it.
Left to the conf key only testkit reads, one side would run patched source and the other would not, and
every patched case would come back as a difference between the runners — a difference the harness would
be reporting about itself.

Unpatched, these cases fail on **both** sides for the same reason, so they compare clean and the gate
would pass with them in. Nothing about equivalence is lost by leaving them broken; what is lost is the
corpus. A case that fails for the machine exercises neither runner, and 3,452 cases of which some number
were never executed is not the number this gate claims.

So the patched set is named where the verdict is: the report carries a `PATCHED` block with the count and
the case names, and the evidence quotes it. A verdict from patched source is not the same claim as a
verdict about the corpus, and a gate that cannot list which is which is not a gate.

**Corpus.** The exit evidence runs the whole shell corpus — `cubrid-testcases-private-ex/shell`,
3,452 cases. The development loop uses `_01_utility` (217 cases) as a smoke set: deterministic,
low external dependency, and chosen for a reason that can be stated.

Every count on this page is what this command yields, so it can be checked rather than trusted:

```sh
find <tree> -name "*.sh" -type f -print | awk -F "/" '{ if( $(NF-2)".sh"== $NF) print }' | wc -l
```

**Revised 2026-09-02.** These numbers were first written as 3,722 and 234, counting every `*.sh`
under a `cases/` directory. That is not CTP's rule: a case is `<name>/cases/<name>.sh`, and the 270
other scripts are helpers the cases call. Every corpus size below was recounted
(`evidence/spec-corrections.md`).

Excluded from the evidence, each with a reason:

| Excluded | Why |
|---|---|
| `_25_unstable` (195) | Counted separately, not dropped. Its own `readme.txt` says these depend on elapsed time, memory size and whether anyone else is using the machine. Mixing them in lets noise bury the signal; dropping them silently would be hiding the inconvenient part |
| HA tree (367) | Needs a master/slave topology that does not exist yet. `DeployHA` is still ported — it is axis T — but it ships **unverified**, and that is recorded rather than glossed |
| `manually` (2) | Documented as human cases, not automation targets |
| Windows | Officially stale (ADR-003 revision). The native runner refuses it |

## Consequences

1. `docs/project/evidence/normalization.md` (or its executable equivalent) becomes a Phase 3 deliverable, and
   changing it is a change to the freeze.
2. Phase 3 exit needs two runs over 3,452 cases, old and new. That cost is the point: a smaller
   corpus would make "equivalent" mean less.
3. `_25_unstable` produces a second, separate report. A difference there is a finding to look at,
   not a failure.
4. `DeployHA` ships unverified. It must be listed as such in `docs/project/evidence/regression-shell.md`, not
   left for someone to discover.
5. **A shard that differs owes a self-check before its differences are reported** — added 2026-09-08.
   The comparison harness produces the difference; the self-check decides whether it is about the
   runners. Reporting one without the other is what the first 217-case run did.
6. **The patch set is part of the evidence** — added 2026-09-19. `cubrid-testkit-patches/shell` is pinned by
   the run that used it: the report's `PATCHED` block lists every case, and a patch that stops applying
   stops the shard rather than the case, because upstream having moved is a thing to find out about
   rather than compare past. Sending these diffs upstream is a separate track and not a precondition of
   this gate — a case fixed upstream deletes its patch, and the gate is run again on a corpus with one
   fewer.
