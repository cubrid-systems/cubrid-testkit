# Reading a run's failures

Notes from classifying 279 failures across a 3,244-case corpus, written down
because the obvious method is wrong in a way that is not obvious, and it caught
this analysis three times.

## The trap

`feedback.log` keeps a case's console output **only when the case failed**. An OK
case is four lines; a NOK case is several hundred. So there is no control group,
and any string that CTP prints during teardown appears in 100% of failures and
0% of passes — which is exactly the shape of a cause.

Three that fooled this analysis in turn:

| string | in NOK | what it actually is |
|---|---|---|
| `cubrid manager is not running` | 75/75 | `cubrid service stop`, at the end of every case |
| `manager server is not installed` | 103/103 | the same, in the next run |
| `dos2unix: command not found` | 40/103 | a tool CTP calls and this machine lacks — and nothing depends on |

The first two are obvious in hindsight because they are near 100%. `dos2unix` was
not: 39% looks discriminating, the missing tool is real, and CTP calls it from
inside `compare_result_between_files`, which every comparison goes through. The
story was complete and wrong.

## What settles it

**Remove the suspected cause and re-run the same cases.** Nothing else does.

For `dos2unix`: a stand-in was put on the slot's PATH and the 40 cases re-run.
**39 still failed.** The tool was noise. One experiment, twenty minutes, and it
replaced a plausible story with an answer.

The same method sized the real ones:

| suspected cause | removed by | failures before | after |
|---|---|---|---|
| `--as-needed` dropping `-lcascci` | a linker shim | 112 | 2 |
| an inherited `databases.txt` | clearing it, and pruning on reclaim | 27 | 0 |
| `hostname -I` empty in a net namespace | an address on a dummy interface | 6 | 0 |

## Rules that follow

1. **A signature present in most failures is not a cause.** Discard anything over
   ~80% of NOK blocks without reading it. It is teardown.
2. **Frequency below that proves nothing either.** It ranks candidates; it does
   not confirm them. `dos2unix` sat at 39%.
3. **Order classification rules most-specific first, and check the residue.** A
   rule matching on `PERMANENT DATA` swallowed manager failures whose logs
   happened to contain a volume listing; the count it produced (9) was wrong by
   a factor of four.
4. **Grep tells you what a case mentions, not what it depends on.** 60 of 89
   remaining cases mention a volume size or `spacedb`. That is a list of
   candidates for an experiment, not a list of cases to patch.
5. **The first failing check beats any log line.** `<case>-N : NOK` in the
   console section, and the last real command before it, points at where the
   case gave up. Skip the shell trace of `write_nok` itself — it prints
   `+ '[' -z '' ']'`, which otherwise looks like the answer for 77 cases at once.

## What an experiment cannot do

It says a case's verdict depends on something. It does not say the dependence is
wrong. `bug_bts_5106` pins `--db-volume-size=20M` on its own `createdb` and still
reads the volume layout out of `spacedb`; a case that exists to test volume
extension is *supposed* to depend on the volume size. Sorting those two apart is
reading the case, one at a time, after the experiment has said which ones to
read.
