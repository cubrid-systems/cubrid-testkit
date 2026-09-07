# The comparison harness

- **Date:** 2026-09-07
- **What this is:** the thing that turns ADR-013 from a definition into a run
  you can leave going. Four cases were compared by hand; 3,452 cannot be.

ADR-013 says how to judge — normalise, then require the rest to be identical —
and says which corpus. It does not say how to *run* it, and at roughly 18
seconds a case on each runner the whole corpus is somewhere between thirty and
a hundred hours. That gap is what these four files fill.

```
shards.sh       the corpus, cut into units of work and of resume
shard.sh        one unit: run both runners, keep both trees, compare them
selfcheck.sh    one unit, one runner, twice: what the corpus cannot reproduce
compare.sh      the comparison itself
classify.awk    sort the differences into named buckets
baseline.txt    the buckets, each with the reason it exists
deviations.txt  the verdict files' only exemption, matched literally
```

## Running it

```sh
export COMPARE_ENV=/path/to/env.sh     # CTP_HOME, CUBRID, JAVA_HOME, PATH, ...
export TESTKIT=/path/to/testkit
export CONF=/path/to/shell.conf        # scenario= is replaced per shard
export WRAP=/path/to/in-ns.sh          # optional, and in practice required

./shards.sh "$CORPUS" 250 | while read -r n path; do
  ./shard.sh "$path" /somewhere/out
done
```

`shard.sh` skips a shard whose report already ends in `COMPLETE`, so the loop
can be killed and restarted. Reports land in `<out>/<slug>/report.txt` beside
both result trees and both runners' stdout, so a finding can be re-examined
without re-running anything — which is how the first comparison's artifacts
were still usable five days later.

To compare two trees you already have, without running anything:

```sh
./compare.sh <ctp-result-dir> <testkit-result-dir> [label]
```

**Measure the corpus before judging the runners.** A case whose verdict one
runner cannot reproduce against itself cannot be evidence that two runners
differ -- the argument that stopped `dispatch_tc_ALL.txt` from being gradeable,
moved from a file to a verdict. `selfcheck.sh` runs one runner twice and reports
what it disagrees with itself about:

```sh
./selfcheck.sh "$CORPUS/_01_utility" /somewhere/out testkit
```

Skipping this step makes every unstable case read as a runner difference, which
is what the first 217-case report did with five of them.

## What a report means

**`VERDICTS` comes first and answers the question.** It lists the cases the two
sides disagree about, by name. Everything below it is detail: two runs that
disagree about five cases disagree about everything those five printed, and
reading forty thousand difference lines as forty thousand findings is how a real
one gets missed.

Exit 0 and a `COMPLETE clean` line mean: every case agrees, the six
verdict-carrying files are identical apart from anything named in
`deviations.txt`, and every difference in the other four matched a rule in
`baseline.txt`. Anything else is exit 1 and a `COMPLETE dirty` line, with the
unmatched differences printed in full under `NEW`.

## Two decisions worth knowing about

**The verdict files are never classified.** `dispatch_tc_ALL.txt`,
`dispatch_tc_FIN_<env>.txt`, `test_status.data`, `check_<env>.log`,
`current_task_id` and `monitor_<env>.log` are held to zero differences with the
baseline switched off. They were byte-identical on the first comparison and
there is no reason they should ever not be; a rule that let one of them differ
quietly would be hiding the only thing this exercise is for.

This is the same line E9 drew when it built sanitizer baselines and left ASan
out of them: suppress a report that stops the program and you have hidden a
defect, not filtered noise. A baseline earns its keep by making silence mean
something, and it only means something if the loud things are still loud.

**The operator is `comm`, not `diff`.** Both inputs are sorted — `normalize.sh`
ends with `LC_ALL=C sort` — and on sorted input `comm` is the exact multiset
difference. `diff` agreed with it line for line on the four-case comparison,
but `diff` also pairs lines inside a hunk, and reading its output it is not
possible to tell a pair that differs from a pair that was merely aligned. That
mattered: ten lines were written off as alignment artifacts on 2026-09-06 and
were nothing of the kind (see below).

## What it found the first time it was pointed at anything

Run against the four-case artifacts of 2026-09-02, with the console-duplication
defect fixed: 154 differences, 143 of them classified, **11 new**. Ten are one
finding.

The `_38_csql/csql_help` case drives `csql` through an `expect` script, and the
two runs disagree about the shape of its output:

```
CTP      \tCUBRID SQL Interpreter
testkit  \t\tCUBRID SQL Interpreter\r
```

An extra leading tab, and a carriage return — which is what a pseudo-terminal
produces and a pipe does not. `expect` allocates one, so both sides have a pty
somewhere; what differs is the environment around it.

**The standard-input hypothesis is wrong, and it was measured rather than
argued.** CTP reaches a case through `Runtime.getRuntime().exec`, whose child
gets a pipe nothing ever writes to; `internal/exec` never sets `Stdin`, which Go
documents as the null device. That difference is real. It is not this one:

```
expect spawning a printf, stdin = /dev/null : \t C U B R I D ... \r \n
expect spawning a printf, stdin = a pipe    : \t C U B R I D ... \r \n
```

Byte for byte the same, so what expect is given on its own standard input does
not change what the pty it allocates produces. The stdin difference stays worth
knowing -- a case that reads standard input can still tell -- but it does not
explain these ten lines.

The ten lines are **still open**, and deliberately not in `baseline.txt`. What is
known: the case drives `csql` through `expect`, a pty always adds the carriage
return, and CTP's recording has neither the return nor the second tab -- so on
CTP's side something either did not use a pty or removed what a pty added.
Deciding between those needs the case itself, which needs the machine to itself.
