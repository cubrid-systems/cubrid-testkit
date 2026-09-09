# Patches

Corpus changes a run needs and does not own.

Some cases fail for reasons that are neither the engine's nor this runner's.
They assume the layout of the machine that has always run them — that `$CUBRID`
sits under `$HOME`, that `[common]` is empty, that the linker resolves libraries
in an order it has not for years. Fixing the corpus is right, and it belongs
upstream, where it is one pull request per case and someone else's review. Until
then a run can either fail on them or carry the change.

Carrying it as a patch file, applied at the moment the case runs, keeps three
properties:

- **The corpus on disk is not modified.** Writes go to the run's overlay and are
  dropped when the directory retires, so the checkout comes out as it went in.
- **The change is a diff.** It can be read, reviewed, and sent upstream as it
  stands — and it stops applying the moment the case changes, which is the signal
  that upstream has moved and the patch should be deleted.
- **It is visible.** A verdict from patched source is not the same claim as a
  verdict about the corpus, and the run says which is which.

## Layout

The tree mirrors the corpus under a category directory, so finding the patch for
a case is finding the case:

```
patches/shell/_08_shard/_02_cubrid_broker01/cases/_02_cubrid_broker01.sh.patch
              └─────────── the case's path under the scenario root ──────────┘
```

The diff's own paths are relative to the **case directory**, so one patch may
touch the case script and its answer files together.

## Turning it on

```
case_patch_dir=/path/to/cubrid-testkit/patches/shell
```

Unset, nothing is patched. That is the default, and it is the right default for
a run whose job is to report the corpus as it stands.

## How a run says a case was patched

Four places, because a caveat that only appears in one is a caveat that gets
missed:

| where | what it says |
|---|---|
| standard output, before the first case | every case that will run patched, and any patch with no case in this corpus |
| the case's own log | `[PATCH] applied <file>`, so it is in `feedback.log` and in the page's case detail |
| the page's **finished** table | a `patched` badge beside the case name |
| the page's finished count | `… · N patched` |

## When a patch stops applying

The run refuses the case rather than running it unpatched, and says so in the
verdict. `TestTheShippedPatchesApplyToTheCorpus` catches the same thing at build
time against a checkout named by `TESTKIT_SHELL_CORPUS`.

Either way the answer is the same: check whether upstream fixed the case, and if
it did, delete the patch.

## What is here

| patch | why |
|---|---|
| `_08_shard/_02_cubrid_broker01` | the case normalises paths with `sed s@$HOME@/path@`, which assumes `$CUBRID` is under `$HOME`. Its answer expects `/path/CUBRID/...`, so the patch normalises `$CUBRID` directly |
| `_08_shard/_03_cubrid_broker02` | the same line, the same fix |
| `_06_issues/_17_1h/cbrd_20760_1` | `test1.answer` expects an empty `[common]` — `db_volume_size=512.0M (512.0M)`, the engine's own default. CUBRID does not ship that: the stock `cubrid.conf` sets `log_volume_size=20M`, so the case fails on a fresh install. The patch clears the two parameters the case is about, before the baseline it asserts |
