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
