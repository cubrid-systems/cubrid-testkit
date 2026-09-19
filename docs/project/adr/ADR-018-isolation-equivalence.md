# ADR-018: How isolation equivalence is proven

- **Date:** 2026-09-15
- **Status:** Accepted (2026-09-15) — with the decision to make parallel slots isolation's default, which
  `design/module-isolation.md` §0 had left until this gate was met. It is met with one slot and with four
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
| native, four slots | 14 |

- The two CTP runs dispatched the cases in the same order and **disagree on 7 verdicts**.
- Across the three runs, **10 cases** moved. Rerun alone, three times under each runner: five flip from run to run under
  each runner, and five pass every time under both and failed only inside a whole run. **None of the ten separates the
  runners.**
- Everything that does not depend on timing is the same: `check_local.log`, the dispatch sets, the configured keys of
  `main_snapshot.properties`, and the set of files written into the cases tree.
- Normalized results (`result/<name>.log`) differ for 9 of 6,772 cases: the 7 whose verdicts moved, and two cases that
  passed in both runs by matching different answers they both have (`.answer` and `.answer1`).

The cases wait on locks and on `sleep` — `delete_select_03` sleeps 30 seconds by its own script — and `qactl` gives a
blocked client 100 seconds and then 300 more before it calls the wait failed (`qactl.c:91-92`). A verdict that depends
on which client prints first is not a property of the runner, and a diff-zero rule on verdicts is a grade CTP itself
fails.

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
3a. **What a separation means depends on whether both sides ran the same executor** — added 2026-09-19.
   Rule 3 was written under ADR-007, where `runone.sh` and ctltool execute every case on both sides. The
   executor being common, the only thing left that could separate two runners was the runner, and
   *separation* and *runner difference* were one sentence. Under ADR-019 they are not. A controller
   without `qactl`'s two fixed 100 ms sleeps sends a client its next statement when the script says to,
   and a case that never ordered what its clients print then fails every attempt — three of three, which
   is what rule 3 calls a runner difference.

   So the rule says which it is, and the `.ctl` decides: **a separation is charged to the runner only
   where the case orders what it prints.** Where the case leaves two clients' output unordered and the
   answer records the order one controller happened to produce, the separation is the corpus's and is
   recorded as such — the 25 found at four slots and two more at eight and fourteen, each with its fix,
   in `evidence/isolation-corpus-races.md`. Everything else is the runner's, as before: a wrong verdict,
   a file written differently, a wait that a case with its waits in place still fails.

   This is not a softer grade, and it fails closed. The reading is made from the case's own text, not
   from the result, and a case that cannot be shown to leave its output unordered counts against the
   runner. What it stops doing is judging a controller by whether it reproduces another controller's
   latency.

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

With four slots (rule 5), in 2,978 s against 12,301 s: rule 1 holds. Four cases both CTP runs pass fail — catalog rows
in a different order, a `rows affected` in a different place, where each slot's `ctldb` has seen different cases —
and every one passes alone three times under both runners. `delete_select_14`, which failed in every other run,
passes, and alone fails five times of six. **No runner difference again; across the four runs, fifteen unstable cases
and seven that fail in every run.**

## Consequences

1. **The gate is about the runner, and it can pass.** A case that flips on timing is not a reason to keep CTP's loop, and
   under this rule it is not counted as one.
2. **The unstable fifteen and the stable seven are upstream's to know about.** They fail on a machine running the same
   engine and the same cases under CTP itself.
3. **Noise is measured before anything is judged**, as ADR-013 requires — here as two whole CTP runs, because one CTP run
   is not a reference for this corpus.
4. **Rule 3 costs minutes.** Ten cases three times under each runner took under twenty minutes; a whole CTP run takes three
   hours.
5. **Parallel slots are compared to one slot by the same rules.** What a slot changes is order and load, which are the
   two things this corpus is already sensitive to.
6. **A gate run with a replaced executor is run on the patched corpus** — added 2026-09-19. Rule 3a names the
   cases the corpus owns; `cubrid-testkit-patches/isolation` carries them as diffs and a run applies them through
   `case_patch_dir`, so the controller is measured against cases that order what they print. This is ADR-013's
   arrangement for shell (*"A patched case is judged, and named"*): the patched set is listed where the verdict
   is, and a patch that stops applying stops the run rather than the case.
7. **Sending those diffs upstream is a separate track** — added 2026-09-19. It was not, and saying so is the
   change: `evidence/isolation-corpus-races.md` and ADR-019 both held the controller off *until upstream fixed
   the cases*, which makes a gate here wait on someone else's review queue. A patch carried in this repository
   closes the gate; the pull request that deletes the patch is its own piece of work, and the patch deleting
   itself is how this repository finds out it landed.

## Alternatives considered

**Diff zero on verdicts against one CTP run**, as for shell and sql. Rejected: CTP against itself moves seven verdicts.

**An exclusion list of unstable cases.** Rejected: it hides the cases from the comparison and from upstream, and a list
drawn from one machine's runs goes stale on the next.

**Many CTP runs, to draw a band around every case.** Rejected for the gate: three hours a run, and the reruns of rule 3
answer the same question for the only cases that need it.
