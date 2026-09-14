# Where the parallel shell run stands

- **Date:** 2026-09-09
- **What this is:** what a full-corpus run of the shell suite does on one machine at 8 and 24
  slots, what was wrong with it, and what is still open. Written as a handover: everything below
  is either measured or explicitly marked as unmeasured.
- **Corpus:** `cubrid-testcases-private-ex` at `afab9435`, engine `11.5.0.2545-b50dcc3`.
  `_25_unstable` excluded — upstream does not judge it — leaving 3,244 cases after the daily
  exclusion list and the `LINUX_NOT_SUPPORTED` macro.

---

## 1. Where the numbers are

| run | slots | ceiling | wall | OK | NOK |
|---|---|---|---|---|---|
| baseline, before any of today's fixes | 8 | 20 GB | 130 min | 2,965 | 279 |
| the same 279 failures, re-run with the fixes | 8 | 20 GB | 23 min | 176 | 103 |
| full corpus | 24 | 22.5 GB | — (stopped) | 3,112 | 131 |

The 24-slot run was stopped twice: once for the admission bug in §3, once for a case that hung
past its plan by a factor of 25. Its 3,243 judged cases are the figures above, and they are
sound; the wall clock is not, and is not quoted.

**183 cases went from NOK to OK** between the two 8-slot runs. That is the whole of today's work
in one number.

---

## 2. What was actually wrong, by size

Every row was established by removing the suspected cause and re-running the same cases.

| cause | failures | what it was |
|---|---:|---|
| `--as-needed` dropping `-lcascci` | 112 → 2 | CTP's `xgcc` names the library before the object. The default here, not on the CI image. A driver shim for `gcc`/`g++`/`cc`/`c++` |
| an inherited `databases.txt` | 27 → 0 | `$CUBRID_DATABASES` is a per-slot overlay whose **lower layer is shared**, so an entry from a run weeks ago sat in all eight slots. `make_tz -g extend` walks every name in it |
| `db_volume_size` the case never named | 20 → 3 | 6 cases take the size from the conf; 13 name it on `createdb` and still depend on the conf for the volumes they extend into |
| `log_volume_size` the case never named | 8 → 1 | same shape, a different parameter |
| `hostname -I` empty in a net namespace | 6 → 0 | it reports every interface *except* loopback. 108 case scripts read it |
| `tar` cannot restore ownership | 5 → 2 | a user namespace maps one uid; `tar -x` extracts and exits 2. 51 scripts unpack something |
| `CUBRID_TMP` outside `$CUBRID` | 1 → 0 | cases normalise output against `${CUBRID}` |

**Not a cause, though it looked like one:** `dos2unix` is missing from this machine and CTP calls
it from inside `compare_result_between_files`, so it appears in 40% of failures. A stand-in on the
PATH and a re-run of those 40 cases left **39 still failing**. See `reading-failures.md`.

---

## 3. What the runner was doing to itself

Found today, all fixed, all with tests:

- **The reset's `find ${CUBRID}/ -name "core"` ran as `find /`** with the variable unset and
  deleted 69 directories across the machine, one of them inside a VS Code server. Guarded now, and
  `safeToEmpty` checks any path before it becomes a destructive command.
- **The guard did nothing** for its first hours: `Run` reports a non-zero exit in the `Result` and
  keeps `err` for not being able to run at all, and `quietly` checked `err` alone.
- **`Enter()` spelled two things -1** — "nothing to contain" and Go's "killed by a signal" — so an
  OOM-killed contained child sent the parent on to run the suite uncontained.
- **`ClaimFor`'s `false` grew a second meaning** when the admission policy arrived, and the worker
  read it as the first: ten of twenty-four slots closed in the first seconds of a run.
- **The stagger became the tail.** `heavy_in_flight_max` skips heavy cases while lighter ones are
  available, so they accumulate at the front of the remaining window; the last 150 cases ran six at
  a time with eighteen slots idle. A constraint and a preference are now different things — see
  `internal/dispatch/policy.go`.
- **The status page had stopped rendering** below the headline figures, twice, both times from a
  helper that was called and not reachable.

A pattern worth naming: **four of these were one value meaning two things.**

---

## 4. What is open

### 4a. The 47

47 cases fail in a full-corpus run at 24 slots and pass at 8. What that means is **not** settled:

| what was run | result |
|---|---|
| full corpus, 24 slots | 47 fail |
| those 47 alone, 24 slots | **3** fail |
| those 47 alone, 8 slots, three times | **0** fail, no verdict moved |
| their source against 300 cases that pass | no pattern separates them |

So the variable is the **pressure of a full run**, not the slot count, and not anything visible in
the case's source. An earlier claim in this session that they are "slot-count dependent" was
wrong — the 8-slot control also changed the load.

**Next:** run the full corpus twice at 24 slots and intersect the two failure lists. Overlap means
a deterministic response to pressure and the cases are worth reporting upstream; no overlap means
flakiness, whose rate was separately measured at **2.8%** (6 of 213, three identical runs at 16
slots) against 47/3,120 = 1.5% here.

### 4b. Where the ceiling and the slots stop

Measured on `_01_utility` (213 cases) with `data_buffer_size=64M`, `log_buffer_size=4M`:

| slots | wall | peak load (16 cores) |
|---|---|---|
| 8 | 336 s | 3.4 |
| 12 | 230 s | 4.7 |
| 16 | 190 s | 8.7 |
| 24 | 191 s | 9.7 |

16 is where this subset hits its makespan floor — its longest case is 195 s against 184 s of work
per slot. The full corpus's floor is much further out (46,217 case-seconds, longest 524 s), so 24
is not the machine's limit; the tmpfs is. At 24 slots the full corpus peaked at **19,645 MB**.

Shrinking the buffers is what made 16+ slots possible at all: 24 × 768 MB of engine buffers plus a
20 GB ceiling does not fit in 31.7 GB. It cost no verdicts that survived repetition.

### 4c. Not yet measured

- **The review's systemic finding**: `Run` reports failure through `Result.ExitCode`, and roughly
  eighteen call sites in `shellsuite` check only `err`. Two were fixed by hand. Making `runIn`
  return an error on non-zero exit, with an explicit opt-out for the few probes that want status as
  data, would turn the rest into compile-time problems. **Done 2026-09-11:** `runIn` now errors on a
  non-zero exit, and nine call sites opt out through `probeIn` -- seven whose status is an answer,
  and two walks over the corpus, discovery's `find` and the workspace `cp -r`, which warn and go on.
  Those two are lenient because three of five corpora sampled on this machine hold a root-owned
  0700 directory a run as root left under a case's `cases/`, and `find` exits 1 on each.
- **The isolation audit's remaining claims**: `$CTP_HOME` written by three cases that `make clean`
  in it; `/tmp` shared (`Namespace.Private` exists and has never been called); `$HOME` shared;
  cases that size the engine from `free -g`; seven that assert on `nproc`. None reproduced yet, and
  none of them separates the 47.
- **`patched.txt` has never been seen from a full run** — both 24-slot runs were stopped before the
  run's own tail wrote it. 35 patches match the corpus and all 36 apply, but that they were applied
  in a full run is unconfirmed.
- **Slot roots are keyed on the namespace-local pid** (`/var/tmp/testkit-slots/3`), which is small
  and reused, so a later run can inherit an earlier run's `$CUBRID` upper layer. Confirmed on disk.
  **Fixed 2026-09-11:** each run makes its root with `MkdirTemp` and removes it when the slots
  close. The fallback `CUBRID_TMP` (`/var/tmp/tk<pid>`, used only when `$CUBRID` is too deep for a
  socket path) is still keyed on the pid.
- **`cbrd_26328` hangs** on `csql ... call [CHANGE].test_proc()` — an unsubstituted placeholder —
  for 25 minutes until the timeout. Plan says 43 s.
- **20 `cases/` directories hold a `.sh` whose name does not match the directory**, so those cases
  are never discovered. A coverage hole, measured at 13 in this corpus.
- **The comment in `dispatch.go` and `corpus.go` claiming "15 of 217 directories hold more than one
  case" is false** — 0 of 3,494 do. The slot-affinity machinery is dormant.

---

## 5. Volatile slot overlays (2026-09-11)

> **The engine in this section is 11.3.5.1275-0e31336, not the 11.5 in the header.** The `regr`
> sandbox held an install of its own and nobody checked it; every run below says
> `Build Number: 11.3.5.1275` in its own output. The wall clocks and the I/O compare a switch
> against itself and are sound. **The verdict columns are not** — 61 of 217 failing is that
> engine against a corpus written for 11.5, not a property of `volatile`. §6 is the same
> measurement at develop head.

`TESTKIT_SLOT_VOLATILE=1` took eight sql slots from 1,131 s to 338–394 s, by making the log flush
every commit waits on a no-op (`sql-native.md` §3). The same switch on shell, in the `regr` sandbox:
`_01_utility`, eight slots, the slot root on `/data`, no `scenario_ram_mb`, run twice.

| | wall | OK / NOK | writes to sdb | flushes | sdb busy |
|---|---:|---:|---:|---:|---:|
| without | 1,426 s | 156 / 61 | 188 GB | 139,115 | 95% |
| `volatile` | 1,401 s | 156 / 61 | 187 GB | 139,155 | 95% |

**No effect, and none possible in this configuration.** Without `scenario_ram_mb` there is no corpus
overlay: a case runs in place in the corpus, as under CTP, and the databases it creates there are
plain files on `/data`. The slot overlays cover `$CUBRID` and the registry, which a shell case barely
writes; the flushes did not move. The verdicts did not either — the same 61 cases fail in both.

Where the switch can reach shell is the disk lane of a split (`lane_slow_*`), whose corpus overlay is
mounted per slot with its upper on disk; with `scenario_ram_mb` alone the upper is a tmpfs, whose
syncs cost nothing already. What this run also shows: `_01_utility` writes 188 GB in 24 minutes —
`createdb`'s volumes, 512 MB each here — and keeps the disk 95% busy, so it is bound by how much it
writes as well as by the syncs.

**So every slot got a corpus overlay on disk: `scenario_disk=on`.** Each slot sees the corpus
through an overlay of its own, its upper in the slot's directory under `TESTKIT_SLOT_ROOT`; the
corpus on disk is unchanged. The same cases, slots and disk, two more runs:

| `_01_utility`, 8 slots | wall | OK / NOK | writes to sdb | flushes | sdb busy |
|---|---:|---:|---:|---:|---:|
| in place | 1,426 s | 156 / 61 | 188 GB | 139,115 | 95% |
| in place, `volatile` | 1,401 s | 156 / 61 | 187 GB | 139,155 | 95% |
| `scenario_disk` | 1,454 s | 156 / 61 | 190 GB | 137,773 | 95% |
| `scenario_disk`, `volatile` | **540 s** | **157 / 60** | **64 GB** | **2,978** | 83% |

- **The overlay alone changes nothing** — 1,454 s, the same writes, the same 61 failures. It is what
  lets `volatile` reach the databases, and `volatile` is what does the work.
- **Two thirds of what the cases wrote never reached the disk.** A database a case creates and drops
  again before the kernel writes it back leaves nothing to write: 188 GB became 64, and the syncs
  went from 139,000 to 3,000. 2.6 times faster, the lowest available memory 15.3 GB, and at most
  3.4 GB waiting to be written.
- **Verdicts.** No new failure. One case, `_15_backupdb/itrack_10002`, fails in the three slow runs
  and passes in the fast one; it fails at its client-server multi-threaded backups (`-C -t 2`,
  steps 55–59). Why it passes when the run is faster is not established.

---

## 6. The whole corpus at develop head (2026-09-14)

§5 left one thing unmeasured: `scenario_disk` and `volatile` over the **whole** shell corpus rather
than `_01_utility`. This is that run, and the first time everything under it is at upstream's head.

| | |
|---|---|
| engine | `11.5.0.2569-dcfe798` — built here from `cubrid/cubrid` develop head |
| corpus | `cubrid-testcases-private-ex` `2a22a74c` — develop head |
| CTP | `shell/init_path` byte-identical to the upstream checkout |
| `cubrid.conf` | the four lines in [the configuration](../category/shell/04-configuration.md): 20M/20M/64M/4M |
| run | 8 slots, `scenario_disk=on`, `TESTKIT_SLOT_VOLATILE=1`, 47 patches applied |

| | |
|---|---:|
| judged | **3,216** (261 excluded: `_25_unstable`, the daily list, this machine's two) |
| **passed / failed** | **3,184 / 32** |
| wall | **9,722 s** (162 min) |
| writes to sdb | 560 GB |
| flushes | **20,935** |
| disk busy | 61% |
| dirty peak | 3,595 MB — against a 20% hard throttle at roughly 3.9 GB |
| lowest available memory | 5,197 MB |
| lowest `/data` free | 19.7 GB |

**`patched.txt` from a full run, which §4c wanted.** 47 of the 49 shipped patches matched a case and
applied; the two that did not are cases no longer in the corpus.

### 16 slots buys nothing on this machine

The same corpus, same everything, at 16 slots, stopped after 1,137 cases:

| slots | cases/min at the same point in the corpus | dirty peak | lowest available |
|---|---:|---:|---:|
| 8 | ~35 | 2.1 GB | 16.8 GB |
| 16 | ~34, falling to 28 | 3.2 GB | 13.2 GB |

`tools/sizing.sh` had already said why: this machine delivers **4–5 cores' work at once** of the 16
it counts, because other work runs on it. Doubling the slots doubles the waiting. The dirty-page
limit is the other wall — at 16 slots the run sits against `dirty_ratio`, where a write blocks and
the sync avoidance stops paying. So the answer to "does volatile let us raise the slot count" is
**yes in principle and no here**: the tmpfs ceiling is gone, but the kernel's writeback throttle and
the CPU arrive first.

### The 32 failures

Re-run alone, one slot, no volatile, they split three ways:

| | cases | |
|---|---:|---|
| reproduce alone | 7 | counters, page counts and statistics: `cbrd_20145_1` (`Num_page_locks_acquired` 27 vs 25), `cbrd_20149_xasl`, `cbrd_24644`, `bug_bts_11649`, `bug_bts_8934_3`, `bug_bts_9411_5`, `CUBRID_LANG_EUCKR`. Engine and answers are both at develop head, so this is upstream's to settle |
| unstable on their own | 1 | `cbrd_25080` failed 4 of 5 runs **alone**: the optimizer picks `temp(order by)` over the index's order, a cost decision, and costs come from the sampled statistics `CBRD-26959` introduced |
| not reproduced in a small tree | 1 | `cbrd_23732` passes alone and at 8 slots in a 9-case tree; it fails only in the full run |
| the machine's own | 2 | `cbrd_24911`, `cbrd_26501` drive CUBRID Manager, which this machine cannot build. Now on `exclusions/no-cubrid-manager.txt` |
| timing and state | the rest | empty variables in comparisons, an `expect` pager interaction, a `sleep` loop's log |

**A wrong turn worth recording.** The first read blamed the 16-slot load, because the failures
appeared there first. The 8-slot run failed on the same cases. Load was not the cause, and the
cheap check that settled it — the same case alone, five times — should have come first.

### Where the corpus spends its sleep

`sleep` written in the case scripts, for the 3,486 cases a full run judges:

| | |
|---|---:|
| total written | **21,449 s (6.0 h)** in 2,521 calls |
| cases with any | 704 (20%) |
| in `_06_issues` | 16,351 s — **76%** |
| top 5 cases | 38% |
| top 25 | 57% |
| **top 100** | **81%** |

Two cases sleep an hour each (`bug_bts_14506`, `bug_bts_14441`) against a 720 s timeout, so they are
killed mid-sleep and hold a slot for the whole of it. A static count is a floor — a `sleep` in a loop
is counted once — and CTP's own `init_path` sleeps in 13 more places that every case pays.

Six hours of sleep over 8 slots is 45 minutes before a single query runs, which is what puts a
30-minute full corpus out of reach on this machine. **The lever is 100 cases**, not a setting.

---

## 7. What the shard comparison measures, and what it measured instead (2026-09-14)

Re-running ADR-013 on `impl_sql` produced a dirty report and, next to `main` on the same shard, a
difference that looked like a regression:

| `_02_sqlx_init`, 8 cases | `main` | `impl_sql` |
|---|---|---|
| verdicts | every case agrees | **1 disagrees** (`_06_media_failures_supported/itrack_10001`) |
| `feedback.log`, new lines | 6-11 | **510** |

It was not one. `shard.sh` runs the two runners **over the same tree, in place**, CTP first and
testkit second, so a case that leaves a database behind hands it to testkit — and contamination and
a difference between the runners produce the same report. The 510 lines say so in their own words:
`Could not connect to master server` (98), `cubrid broker stop: fail` (36),
`Database "qadb" is unknown` (13) on testkit's side, against
`Volume "…mydb_vinf" already exists` on CTP's.

[`shard-clean.sh`](compare/shard-clean.sh) is `shard.sh` with the scenario restored from a tar
between the two runs, and the registry emptied with it. Same branch, same shard, same order:

| `_02_sqlx_init`, 8 cases | `impl_sql`, in place | `impl_sql`, restored |
|---|---|---|
| verdicts | 1 disagrees | **every case agrees** |
| `feedback.log`, new lines | 510 | **6 / 0 / 6 / 11** |

What is left is the same on both binaries and therefore older than this branch: one line in
`check_local.log` (testkit checks for `expect` and CTP does not) and `GetParameter.exp` noise.

**The lesson is about the harness, not the branch.** An in-place comparison cannot tell a runner's
behaviour from what the previous runner left, and it always blames the second one. Use
`shard-clean.sh` where the shard's cases create databases; `shard.sh` remains right for a corpus
whose cases clean up after themselves, and cheaper.

**Still not clean, and not claimed to be.** Both binaries report `COMPLETE dirty` on this shard for
the two older differences above, and one shard is not the corpus.
