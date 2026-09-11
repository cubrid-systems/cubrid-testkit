# ADR-017: How sql equivalence is proven

- **Date:** 2026-09-11
- **Status:** Proposed
- **Related:** ADR-013 (the same question for shell), ADR-016 (the executor), ADR-015 (axis B waits
  for this), `evidence/sql-baseline.md` (every number below)

## Context

ADR-013 proves the shell runner equivalent by normalising the result files and requiring the ones
that carry verdicts to be identical. The sql family needs the same kind of proof, and three things
about it change what that proof can be.

**Every case leaves its whole output behind.** CQT writes a `.result` beside every `.sql` — 17,459
after one run — and it is the rendered text of every statement in the case. The verdict is only
that text compared with the answer. So sql can be compared on **what each case printed**, not just
on whether it passed. That is a stronger check than shell has, and it is the one that tests
ADR-016's claim that the `jdbc` executor renders identically by construction.

**The whole corpus is cheap.** sql is 29 minutes serially and medium under two (`sql-baseline.md`
§3, §4). ADR-013 needed 64 shards and a resumable loop because shell takes tens of hours a runner.
Here both runners can run the whole corpus back to back in about an hour.

**The runner's floor has been measured.** CTP against itself, twice over the same copies, with all
three repositories at upstream develop:

| | cases | verdicts that moved | `.result` files that differ |
|---|---:|---:|---:|
| medium | 975 | 0 | — |
| sql | 17,459 | **0** | **0** |

Every case passed both times. **The floor is not zero in principle, though.** On an earlier set — a
PR branch of the cases against an engine seven commits behind — one case moved once in three runs
(`_27_banana_qa/…/_04_select/cases/_09_view_dt.sql`). It sorts on a column that holds equal instants
in different zones, so the order of tied rows is the engine's choice. On the same set 38 cases failed
every time, the same way, because the engine returned something that branch's answers did not have
(`sql-baseline.md` §6).

## Decision

**Run CTP and testkit over the whole of both corpora, in the same sandbox, one after the other.
After normalising, four things must be identical, and no baseline of accepted differences is
allowed for them:**

1. **every case's verdict**
2. **every case's `.result`**, byte for byte
3. **`main.info`**, without `elapse_time`, `totalTime`, `end_time` and `result_path`
4. **the counts in `summary_info`** — `total`, `success` and `fail` — and the `Fail`/`Success`/`Total`
   lines on standard output

**Normalisation** masks what two runs of CTP already differ in: the time on each progress line, the
`ddHHmmss` and random suffix in the result directory's name, elapsed times wherever they appear, and
the path prefix of the sandbox. Nothing else is masked.

**A verdict is read across line breaks.** The server a run starts inherits the run's standard
output. In one run of three it wrote `*** XASL generation failed ***` between a case's progress line
and its `[OK]`, so the verdict landed on the next line (`sql-baseline.md` §6). The comparison
extracts each verdict from the progress line through to the next `[OK]` or `[NOK]`. Text that
lands in between is reported as a difference in standard output, not as a difference in verdicts.

**Measured before judged.** ADR-013's rule applies unchanged. A case whose verdict or `.result` one
runner does not reproduce against itself is not evidence that two runners differ. The self-check is
rerun on whatever machine runs the gate. It is cheap, and a floor measured on another machine is an
assumption.

**The corpus is pinned, and it is current when the run starts.** The engine, the cases and CTP
each move daily. So the gate starts only when `evidence/sql/sandbox.sh check` finds all three at
upstream develop's head. The comparison names the three commits it was pinned to, as
`sql-baseline.md` does. The sql run uses CTP's own `sql.conf`. medium uses `medium_dev.conf`,
because `medium.conf` cannot load its data on an 11.x engine (`sql-baseline.md` §3). The gate records
that choice rather than hiding it.

## Consequences

1. **The `jdbc` executor's claim becomes a measurement.** "It renders identically by construction"
   is checked on 17,459 `.result` files. Any byte that differs is a defect, found at the statement
   that produced it.
2. **Failures are part of the evidence, not noise.** Whatever the pinned commits make CTP fail, an
   equivalent runner fails the same cases with the same text, and a runner that happens to pass one
   of them is wrong. At develop that set is empty today. On the mismatched set it was 38 cases, and
   it will not stay empty.
3. **The gate runs in a sandbox.** CTP's `do_clean` kills every CUBRID process it can see and edits
   `$CUBRID/conf`, and the cases write into the testcases tree. The copies and the namespace in
   `sql-baseline.md` §1 are the gate's environment too, and they apply to both runners alike.
4. **CCI mode has its own gate.** Its reference is `ccqt`, not CQT. It compares bytes rather than
   text with the line breaks removed, and it reads `.answer_cci` first. The `native` executor (P2)
   will need a version of this ADR with those substitutions. It is not written until P2 starts.
5. **Axis B is compared against testkit, not CTP.** Parallel slots, the corpus overlay and the rest
   (`design/module-sql.md` §3) are measured serial-against-parallel within testkit, after this gate
   has passed (ADR-015, criterion 2).

## Alternatives considered

**Compare verdicts only**, as ADR-013 does for shell. Rejected. Two runners can both fail a case
with different output, and the verdicts would agree. The `.result` files are already there, and
comparing them costs nothing more.

**Shard the corpus and resume**, as ADR-013's harness does. Not needed at 29 minutes a run. If a
future corpus or a slower machine changes that, `evidence/compare/`'s harness is the one to adapt.
