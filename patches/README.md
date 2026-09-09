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

One flat directory per category. The file name is the case's path under the
scenario root, flattened:

```
_06_issues/_17_1h/cbrd_20760_1/cases/cbrd_20760_1.sh
  ->  patches/shell/_06_issues~_17_1h~cbrd_20760_1.patch
```

Flat rather than a mirror of the corpus, because the corpus is five levels deep
and this set is not: `ls patches/shell` should show everything a run carries, on
one screen. Two segments come out on the way — `cases`, which every case has, and
a file name that repeats its directory, which almost every case has. Nothing
collides: a case always lives under `cases/`, so no case maps to the name another
case would. A directory holding two cases keeps both names
(`_06_issues~_25_1h~cbrd_24741~01_basic`).

The diff's own paths are relative to the **case directory**, so one patch may
touch the case script and its answer files together.

## The corpus comes out as it went in

The patch is applied when the case starts and reverted when it finishes.

Behind the corpus overlay the revert is redundant — the writes are in the upper
layer and go when the directory retires. It is there because that is a property
of how the run was configured and not of the patch, and *does the corpus come out
as it went in* must not have "it depends" as its answer. A run without
`scenario_ram_mb` writes straight into the checkout, and a patch left behind
there would be applied to a case the next run reads from git.

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
| `patched.txt` in the result directory | every case that actually ran patched, and which patch. Written only when there was one, so the file's presence is itself the answer |

The startup line says what a run *intends* to patch; `patched.txt` says what it
*did*. They differ when a patch refuses to apply — and `feedback.log` keeps a
case's console output for failures only, so an OK case that ran patched leaves
no trace there at all.

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
| `_01_utility/_16_restoredb/itrack_10001` | `char(10000)` is over the engine's 2048-byte limit, so the class is never created and all fifteen restore checks fail behind `Unknown class "dba.x"`. The widths are arbitrary — the loader binds the integers 1, 2 and 3, and no answer file mentions them. CBRD-21637, and on the daily exclusion list |
| `_06_issues/_11_1h/bug_bts_5106`<br>`_06_issues/_11_1h/bug_bts_5200`<br>`_06_issues/_16_2h/cbrd_20683`<br>`_06_issues/_17_1h/cbrd_20966` | the answer records a **volume layout**, and the volumes the load auto-extends into take their size from `cubrid.conf` — a setting the case never meant to depend on. `bug_bts_5106` even pins `--db-volume-size=20M` on its own `createdb`, which does not reach the extension. Lowering `db_volume_size` globally turns one 128 M extension volume into two of 64 M: same total space, different listing, and `spacedb` prints the listing. Pinning to what CUBRID ships turns all four from NOK back to OK |
| `_10_plcsql/support_commit_rollback` | the case walks its 18 sub-tests with `find test_sql -name "*.sql"`, which returns readdir order -- the order of the directory on whatever filesystem it sits on. The `.answer` files pin an absolute serial that only comes out right in name order: test_01 expects 1, test_02 expects 7, test_03 expects 13. Measured: this run's serials were 12(2), 17(7), 09(13), 16(19), 10(26)…, exactly the checkout's readdir order, and all 18 comparisons failed. The patch sorts |
| `_06_issues/_17_1h/cbrd_20760_2` | the case's own comment says "do not set db_volume_size and log_volume_size (use the defaule value)", then checks that the volume is 512M. This run lowers both so 24 slots fit in memory, so the case measures the run's setting instead of the engine's default. The patch deletes the two lines |
| `_06_issues/_12_2h/bug_bts_9419` | one of its three creations is `cubrid_createdb -r 9419` with no size, so it takes both sizes from `cubrid.conf`. All thirteen locale answers record what CUBRID ships — "512.0M size … needed is 1.5G" — and got "64.0M … 104.0M". Same fix as above |
| `_06_issues/_20_2h/cbrd_22803` | the answer records `Pool_size : 32768` — 32768 pages of 16K, which is `data_buffer_size=512M`, the shipped value. The case pins its own volume sizes on `createdb` but never names the buffer, so it reports this run's 64M. The patch restores it |
| `_39_fig_cake/cbrd_24044_enhance_optimizer/cbrd_25366` | `match($0, /re/, arr)` — the three-argument form is a GNU awk extension. Where `/usr/bin/awk` is mawk, which Debian and Ubuntu ship as the default, the whole program is `awk: line 3: syntax error at or near ,`, `val_sha1` comes out empty and `cubrid plandump -s ""` fails the case on something it never meant to test. Rewritten with two-argument `match()` and `RSTART`/`RLENGTH`, which is POSIX and yields the same string. The only case in the corpus that needs GNU awk |
| `_28_features_844/issue_10986_eventlog/_03_eventlog_tempvolume` | the case pins `--db-volume-size` on `createdb`, which sizes the permanent volume only; the **temporary** volumes it is about take their size from `cubrid.conf`, which it never names, and it then counts DISK_ADD_VOLUME / DISK_EXTEND / TEMPORARY_VOLUME exactly (4, 6, 10). Lowering `db_volume_size` makes the temp volumes smaller, so two more get created than the answer records. The patch restores the shipped size |
| `_06_issues/_11_1h/bug_bts_5136_4` | step 1 greps `cubrid.conf` for the literal `db_volume_size = 512M` and `log_volume_size = 512M` — the shipped values, which the case exists to confirm before it changes them. This run lowers both, so step 1 measures the run instead of the engine. The four steps after it set their own sizes and are unaffected |

## What is deliberately not here

`_01_utility/_38_csql/csql2` (CBRD-23602) compares against an answer file last
touched in 2016, with a query that has no `ORDER BY`. Making it pass means
regenerating the answer, and an answer regenerated from today's engine asserts
today's behaviour — which is not a compatibility patch, it is deleting the test.
It needs someone who knows what the query was meant to prove.
