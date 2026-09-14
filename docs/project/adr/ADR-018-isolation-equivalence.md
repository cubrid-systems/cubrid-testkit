# ADR-018: How isolation equivalence is proven

- **Date:** 2026-09-15
- **Status:** Proposed — for review. The numbers below are measured; the rule is the proposal
- **Related:** ADR-013 (shell), ADR-017 (sql), ADR-007 (the executor), `design/module-isolation.md` §2-6,
  `evidence/isolation-baseline.md` §4

## Context

ADR-013 and ADR-017 hold the files that carry verdicts to zero differences against CTP. Isolation cannot be held to
that, and the reason is measured rather than supposed.

### The executor is CTP's, so what can differ is small

ADR-007 keeps `runone.sh` and ctltool as they are. A case's bytes come from the same script and the same binaries under
either runner. What a comparison has to establish is that the runner around them — discovery, order, the records, the
verdict read from the script's output — is CTP's, and how much the executor varies on its own.

### CTP does not reproduce its own verdicts

The whole corpus at upstream develop (6,790 cases, 18 excluded, 6,772 run), on this machine:

| run | NOK |
|---|---:|
| CTP, alone | 13 |
| CTP again, same order, alongside the native run | 16 |
| native, one slot | 11 |

- The two CTP runs dispatched the cases in the same order and **disagree on 7 verdicts**.
- Across the three runs, **10 cases** moved. Rerun alone, three times under each runner: five flip from run to run under
  each runner, and five pass every time under both and failed only inside a whole run. **None of the ten separates the
  runners.**
- Everything that does not depend on timing is the same: `check_local.log`, the dispatch sets, the configured keys of
  `main_snapshot.properties`, and the set of files written into the cases tree.
- Normalized results (`result/<name>.log`) differ for 9 of 6,772 cases: the 7 whose verdicts moved, and two cases that
  passed in both runs by matching different answers they both have (`.answer` and `.answer1`).

The cases wait on locks and on `sleep`, and `qactl` gives a wait 30 seconds before moving on. A verdict that depends on
which client prints first is not a property of the runner, and a diff-zero rule on verdicts is a grade CTP itself fails.

## Decision

A native isolation run is equivalent to CTP when, over the whole corpus and with one slot:

1. **The runner's own files are the same, with no baseline**: `check_local.log`; `dispatch_tc_ALL.txt` and
   `dispatch_tc_FIN_local.txt` as sets (the runner sorts; CTP's order is `find`'s); the configured keys and
   `AUTO_BUILD_*` of `main_snapshot.properties`; and the set of files written into the cases tree.
2. **Every verdict CTP reproduces is the same.** A verdict is *reproduced* when two whole CTP runs over the same corpus,
   in the same order, agree on it. For those cases the native verdict must match.
3. **A disagreement on a reproduced verdict is rerun before it is judged**: the case alone, three times under each
   runner. It is a difference in the runner only if the runners separate — one of them passes all three and the other
   fails all three. Otherwise it is a case CTP does not reproduce either, and it is reported with the unstable ones.
4. **Results follow verdicts**: where both runners pass a case, its normalized result matches one of the case's answers
   under each; where both fail it, the results are reported, not compared, because a failing interleaving is the part
   that varies.
5. **Unstable cases are reported, never excluded**: listed with their verdicts in every run, so that the list is
   evidence about the corpus and the engine rather than a filter on the comparison.
6. **Not compared**: `test_local.log`, whose trace lines arrive in a different order between two runs of the same
   script; elapsed times; the console's order; and the summary counts, which follow the verdicts.

Applied to the runs above: rule 1 holds. Rule 2 finds three disagreements on reproduced verdicts —
`groupby/delete_select_06`, `catalog/db_index_key_03` and `aggregate/insert_select_02_1`, NOK in both CTP runs and OK in
the native one. Rule 3 separates none of them: the first two fail alone under both runners, the third passes alone under
both. **No runner difference; ten unstable cases; eight cases that fail in every run.**

## Consequences

1. **The gate is about the runner, and it can pass.** A case that flips on timing is not a reason to keep CTP's loop, and
   under this rule it is not counted as one.
2. **The unstable ten and the stable eight are upstream's to know about.** They fail on a machine running the same
   engine and the same cases under CTP itself.
3. **Noise is measured before anything is judged**, as ADR-013 requires — here as two whole CTP runs, because one CTP run
   is not a reference for this corpus.
4. **Rule 3 costs minutes.** Ten cases three times under each runner took under twenty minutes; a whole CTP run takes three
   hours.
5. **Parallel slots are compared to one slot by the same rules.** What a slot changes is order and load, which are the
   two things this corpus is already sensitive to.

## Alternatives considered

**Diff zero on verdicts against one CTP run**, as for shell and sql. Rejected: CTP against itself moves seven verdicts.

**An exclusion list of unstable cases.** Rejected: it hides the cases from the comparison and from upstream, and a list
drawn from one machine's runs goes stale on the next.

**Many CTP runs, to draw a band around every case.** Rejected for the gate: three hours a run, and the reruns of rule 3
answer the same question for the only cases that need it.
