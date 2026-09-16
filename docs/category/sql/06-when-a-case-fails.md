# 6. When a case fails

[← back to the sql category](README.md)

- [What a verdict is](#what-a-verdict-is)
- [Where to look](#where-to-look)
  - [A case that dumped core](#a-case-that-dumped-core)
- [Failures that are not the engine's](#failures-that-are-not-the-engines)
- [Carrying a corpus fix as a patch](#carrying-a-corpus-fix-as-a-patch)
- [What is unstable at the moment](#what-is-unstable-at-the-moment)

## What a verdict is

A case is a file of statements. CQT runs them on one connection and renders the result — the rows,
the column headers, the error codes — into text, separated by lines of fifty-one `=` characters. That
text is written next to the case as `<name>.result`, and compared with `answers/<name>.answer` after
carriage returns and newlines are removed from both. Same, and the case is OK.

Two consequences worth holding on to:

- **An answer is a recording, not an assertion.** It was produced by a run of this corpus against
  some engine, and it carries whatever that run produced — including a listing's row order, a plan,
  and an error that a leftover object caused.
- **A case with no answer is counted and never run.** CQT keeps it in the list and in `N` without
  executing it, and its summary counts it in neither column — so `Total` is not `Success + Fail`,
  and `main.info`'s `execute_case` is the two added up.

## Where to look

```
cubrid-testcases/sql/<family>/cases/<name>.result     what this run produced
cubrid-testcases/sql/<family>/answers/<name>.answer   what it is judged against
$CTP_HOME/sql/result/…/sql/<family>/<name>.result     a copy, kept because it failed
$CTP_HOME/sql/result/…/sql/<family>/<name>.answer     the answer, beside it
$CTP_HOME/sql/result/…/sql/<family>/<name>.err        gdb's stack, if the case dumped core
$CTP_HOME/sql/result/…/summary.xml                    every case, its time, its verdict
```

`diff` the pair in the result tree: those two are the run's own, and the pair in the corpus is
overwritten by the next run.

The status page (`status_http=on`) answers the same question while the run is going: a failing case
shows the first line where the result leaves the answer, with the lines around it.

### A case that dumped core

After a case fails, the run looks under `$CUBRID` for a `core.<digits>`, and a core it has not seen
before is that case's. It then asks `gdb` for a `bt full` and writes the answer as `<name>.err`
beside the failure copies:

```
SUMMARY:
CORE_DIR:/home/q/CUBRID
core.410914 [cub_server] Core dumped in pt_check_where at src/parser/semantic_check.c:9814

==================core.410914==================
#0  0x00007f1f in __pthread_kill_implementation () from /lib/x86_64-linux-gnu/libc.so.6
…
```

The one-line summary names the innermost frame in CUBRID's own source, skipping the error machinery
every crash leaves on the way out. It is empty when the binary carries no symbols, which leaves the
header line ending at the program's name.

Two things are worth knowing. The analysis runs **in the slot the core is in**, as the case ends
rather than after the run — a slot's `$CUBRID` and the `cub_server` that dumped the core go when the
slot does. And a run that found any core skips the final clean, so the cores are still there
afterwards; a slot's are copied out to `<result root>/slot_cores/` before its overlay is discarded.

## Failures that are not the engine's

A serial run gives every case the same predecessors CTP's run gave it. A slotted run does not — and a
case that read what an earlier case left changes its verdict with it. What that looks like:

| what you see | what it is |
|---|---|
| `Error:-494` where the answer has a result | the case created a table whose name another case left behind |
| a row in a catalog listing that the answer does not have | an object another case left, and a listing that is not scoped to the case |
| the same rows in a different order | a listing with no `ORDER BY`, ordered by the catalog's history |
| `-1071 Too many session variables` | CUBRID holds twenty per connection, and cases before this one left theirs |
| a `Query Plan:` or `Trace Statistics:` block missing | what a trace prints depends on the server's state, and sometimes on nothing the case controls |

Measured, with the patches this repository ships: **0 to 2 cases of 17,459 per six-slot run**, against
0 to 2 per run for CTP by itself at the same pins. The families that could be fixed are fixed; what
is left is in [what is unstable](#what-is-unstable-at-the-moment).

## Carrying a corpus fix as a patch

A case that leans on the order is a case worth fixing, and the fix belongs upstream. Until it gets
there, a run can carry it:

```
[sql]
case_patch_dir=/path/to/cubrid-testkit/patches/sql
```

The patches are unified diffs, one per case, applied before the first case and reverted at the end —
the slots share one corpus, so every slot has to read the same source. A patch that no longer applies
stops the run, because that means the case has moved and running it unpatched answers a question
nobody asked. What was patched is written to `patched.txt` in the result tree, announced on standard
output, and shown on the page: a verdict from patched source is a claim about the patched case.

The principle behind every one of them is the same — **a case says what it needs and cleans up what
it leaves** — and [`overrides/patches/README.md`](../../../overrides/patches/README.md) says what each does and why. To
write one: fix the case in a copy of the corpus, run it, take the `.result` it produces as the new
answer rather than writing one by hand, and diff both files with paths relative to the case
directory.

## What is unstable at the moment

Two things are known not to be fixable in the corpus, and are written down rather than papered over:

- **A leftover table with a name somebody else wants.** 1,922 cases create a table and never drop it;
  245 of those names are created by more than one directory, and `t1` alone is left behind by 61
  directories and created by 423. That is a corpus-wide rule, not a patch set.
- **A trace whose plan the engine does not always print.** The cases already ask for `recompile`; the
  blocks that go missing are the plans of queries the server parallelises or rewrites internally. CTP's
  own two runs disagree about `cbrd_25542`, and a serial run here disagrees about `cbrd_25708_eq_in`.

When a failure looks like either, compare it with a second run before reading it as a regression.
[`project/evidence/regression-sql.md`](../../project/evidence/regression-sql.md) is the same comparison done
carefully, twice per side.
