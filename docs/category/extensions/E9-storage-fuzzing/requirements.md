# E9 — Storage-engine Concurrency Fuzzing (schedule × operation interleaving) (Requirements)

*English · [한국어](requirements.ko.md)*

**Source:** ROADMAP §6a-E9 + the §6a appendix (the fuzzing priority ladder) · survey §7.4
**Status:** incubating (conditional — E5 first, plus SERVER_MODE in-process startup)
**Axis mapping:** axis 5 extended (engine-internal) × axis 8 (schedule/model-based) — a different layer from axes 4 and 7
**Ladder position:** §6a ladder rank 5 (row 6 of the original)
**Companion docs (to follow):** `design.md`, `io-contract.md`, `test-corpus.md`, `implementation-notes.md`
**Reference implementation:** RocksDB `fuzz/` — only the *shape of the structured input* is referenced. The target is different (§1.0)

> **Redefinition (2026-09-03, the user's decision).** This document was first written as fuzzing of
> a *single-threaded operation sequence*. That shape has almost no value — §1.0 below. Which of the
> judgements from the edition before the redefinition are kept and which are discarded is marked in
> each section.

---

## 1. The problem this extension solves

### 1.0 Why not a single-threaded operation sequence (the 2026-09-03 redefinition)

Two things changed the shape of this entry.

**(1) The defects being looked for are not in a single thread.** Most of the states §1.1 below
lists come about only when two or more flows of execution overlap. However many orders one thread
lays its operations out in, it never arrives at latch ordering or at vacuum interference.

**(2) The storage layer is code that forks by mode.** There are **146** `SERVER_MODE` branches in
`page_buffer.c`, **53** in `log_manager.c` and **26** in `vacuum.c`, and `vacuum_Master_daemon` is
a `cubthread::daemon`. An observation made in single-threaded mode (SA) does not carry over to a
SERVER_MODE server. The target library is **`libcubrid.so`** (SERVER_MODE) too —
`heap_insert_logical`, `btree_insert` and `boot_restart_server` are all exported from there — and
not the SA library.

So the input is not one operation sequence but a
**`(per-thread operation sequence × interleaving)`** pair.

CUBRID's **storage engine internal API** (heap / B-tree / slotted page / overflow / MVCC
visibility) is fuzzed with a *structured operation sequence*. The input is not a lump of bytes but
**a sequence of valid DB operations**, and it is that sequence itself that is mutated.

### 1.1 The area the existing axes cannot catch

| Existing axis | What it reaches | The blind spot |
|---|---|---|
| E5 (parser/protocol fuzz) | the frontend byte entry points | *stateless* — no engine-internal state accumulates |
| E3 (SQLancer) | wrong-result in valid SQL | only the combinations SQL can express. It cannot call an internal API directly |
| E2 (SQLsmith) | random SQL that passes the grammar | the same |
| the strangler-fig modules as a whole | cases written by hand | it does not explore the combinatorial explosion of operation *order* |

A storage engine bug goes off not in the state made by *one operation* but in the state made by
**the interleaving of operations running at the same time**:

- slot reuse in a slotted page × a **concurrent** update of a variable-length record
- the order in which two transactions take the same page at the overflow record
  promotion/demotion boundary
- the window where a B-tree unique violation rollback overlaps another thread's re-insertion
- MVCC visibility — the update chain interfering with the **vacuum worker**
- the order in which latches are acquired, and the races whose window is microseconds
- the state where the heap best-space cache has drifted from the actual free space

The SQL layer can reach such a state *by accident*, but it does not explore it *systematically*.
And **a single thread does not reach it at all** — that is the point of §1.0.

### 1.2 The kinds of bug it catches

Internal assert violations / page corruption / a slot index out of range / a dangling OID /
an overflow chain leak / a wrong MVCC visibility decision / heap best-space disagreement /
ASan heap-buffer-overflow and use-after-free / UBSan misaligned access.

---

## 2. Clearing up the misunderstanding about protobuf (to be confirmed first)

> **CUBRID uses its own binary protocol and does not use protobuf.
> So why libprotobuf-mutator? — because it has nothing to do with the protocol.**

protobuf here is only **the fuzzer's internal language for describing input (its IR)**, *not a wire
format*. CUBRID **never sees a protobuf byte.**

```
libFuzzer
   │  (raw bytes)
   ▼
libprotobuf-mutator          ← protobuf exists up to here and no further
   │  (structure-aware mutation: replacing a field / switching a oneof / inserting into and deleting from a repeated)
   ▼
StorageOpSequence  (an in-memory C++ object)
   │  the harness translates it directly
   ▼
heap_insert_logical() / btree_insert() / heap_get_visible_version() / ...
   ↑
   calling CUBRID's internal C/C++ API directly — no network, no protocol, no serialization on the way
```

RocksDB's `fuzz/db_fuzzer.cc` has exactly this shape — it takes a `DBOperation` protobuf message
and calls `db->Put()` / `db->Get()` / `db->Delete()` *directly*. RocksDB does not use protobuf as a
storage format or as a protocol either.

**So no compatibility problem arises:**

| The worry | What is actually so |
|---|---|
| Does CUBRID's protocol have to be changed to protobuf? | No. The protocol is not touched |
| Does a protobuf dependency appear in the server or the client? | No. It is linked **only into the `cubrid-fuzz-storage` fuzz binary** |
| Does protobuf have to be vendored into 3rdparty? | Only on the fuzz build (`-DENABLE_FUZZING=ON`) path. The default build artefacts do not change |
| Do the shipped artefacts get bigger? | No. The fuzz target is not shipped |

*(Confirmed 2026-09-03: there is **no** protobuf dependency in the cubrid repository — a search
over the whole source tree returns nothing, and there is no mention in `CMakeLists.txt`, `cmake/`
or `3rdparty/` either. The current 3rdparty is libedit · libexpat · libjansson · libodbc ·
libopenssl · libtbb · lz4 · rapidjson · re2, and nothing more. So protobuf would be **a new
fuzz-only dependency**, and whether that conflicts with that repository's 3rdparty policy is a
decision item for ADR-EXT-009.)*

### 2.1 The alternatives that do not use protobuf (compared in ADR-EXT-009)

| Approach | New dependency | Structure-aware mutation | Crossover quality | Note |
|---|---|---|---|---|
| **libprotobuf-mutator** | protobuf + LPM (fuzz-only) | ★★★★ | ★★★★ | the path RocksDB has proven. The corpus can be read by a person (TextFormat) |
| **FuzzedDataProvider** (the libFuzzer header alone) | none | ★★ | ★★ | zero dependencies. The byte→op decoder is written by hand. Mutation easily breaks the structure |
| our own IR + our own mutator | none | ★★★ | ★★★ | maximum control, maximum cost to build and to keep |

**The recommendation (not settled):** first verify the skeleton of the harness and the *state
reset* with `FuzzedDataProvider`, then promote to libprotobuf-mutator once the operation vocabulary
has settled. The reason is §7 risk 1.

---

## 3. The form of the external call (proposed — incubating)

```
testkit run fuzz --target storage [--ops <n>] [--time <sec>] [--corpus <dir>]
```

The internal entry (agenda):

```
StorageFuzzDriver.exec(config)          # testkit (Go) — orchestration only
  ├─ run cubrid-fuzz-storage as a subprocess (ADR-001 Consequence 4)
  │
  │   ┌ cubrid-fuzz-storage (a binary of that repository, libFuzzer in-process) ───┐
  │   │ once: create a temporary volume + boot (in-process)                        │
  │   │ for every input:                                                           │
  │   │    reset()                       ← the §5 state reset contract             │
  │   │    for op in sequence:                                                     │
  │   │       translate(op) -> call heap_* / btree_* / log_* directly              │
  │   │       check invariants (optional)                                          │
  │   │    rollback or drop                                                        │
  │   └────────────────────────────────────────────────────────────────────────────┘
  │
  └─ ingest the artefacts produced: crash input → an op sequence text dump + stack hash dedup
```

**The limit of testkit's responsibility:** keeping the corpus + replay + crash triage +
orchestrating the run.
**The cubrid repository's responsibility:** the fuzz target build option, the in-process
boot/shutdown entry points, the `LLVMFuzzerTestOneInput` implementation, the state reset hook.

**Effect on the frozen external surface (NG2):** none (a new entry point).

---

## 4. The operation vocabulary (the first agenda — grounded in the actual API)

> **Redefinition 2 (2026-09-04).** The vocabulary table below was written on the premise that
> *operations are synthesized*. That premise has been withdrawn — because of **the
> calling-convention problem**. `heap_insert_logical` and the rest run on top of premises the
> caller has set up: an open transaction and isolation, locks and latches already taken, a
> `HEAP_OPERATION_CONTEXT` filled in by `heap_create_insert_context ()`, and correctly nested
> `log_sysop_start`/`end`. Synthesizing them produces **an order no real caller ever makes**, and a
> crash out of that may not be a defect but a legitimate response to a violated premise. Every
> finding then comes with a reachability triage.
>
> So operations are **not synthesized; a pre-compiled XASL is replayed instead** (§4a). Executing
> the query sets up every premise, so every path is reachable by construction and the fuzzer
> explores **only the schedule**. The table below is left as the starting point for Q16(b) *future
> work* — modelling the internal API vocabulary.

As confirmed against the CUBRID source (`src/storage/`, `src/base/`):

| op | The internal API it maps to | Note |
|---|---|---|
| `INSERT` | `heap_insert_logical` (`storage/heap_file.h`) | a `HEAP_OPERATION_CONTEXT` has to be built |
| `UPDATE` | `heap_update_logical` | it branches into the in-place / move / overflow promotion paths |
| `DELETE` | `heap_delete_logical` | |
| `GET` | `heap_get_visible_version` | takes an MVCC snapshot argument |
| `SCAN` | `heap_scancache_start` → `heap_next` → `heap_scancache_end` | |
| `IDX_INSERT` | `btree_insert` (`storage/btree.h`) | unique / non-unique |
| `IDX_SCAN` | `btree_range_scan` | |
| `COMMIT` / `ABORT` | the transaction boundary | directly bound up with the §5 reset |
| `VACUUM` | a vacuum request | the heart of exploring MVCC interference |
| `CHECKPOINT` | log checkpoint | the point of contact with rank 6 (recovery) |

`record serialize/unpack` (`or_get_value` / `or_put_value` / `or_unpack_value`,
`src/base/object_representation.h`) is **a stateless byte-in target**, so it belongs not to this
entry but to **E5's target layer** (§6a ladder rank 4).

**Undecided:** the first scope of the vocabulary (heap only / heap+btree / +vacuum / +checkpoint),
the key/value domain (a fixed schema vs a variable domain), and whether scan results are verified.

---

## 4a. What makes the operations — XASL replay (2026-09-04)

**The server does not compile SQL.** `libcubrid.so` (SERVER_MODE) **has no** `parser_main`,
`pt_compile`, `do_prepare_select` or `xts_map_xasl_to_stream` — compilation and serialization are
the client's job. `xqmgr_prepare_query (thrd, compile_context *, xasl_stream *)` only registers an
already-made stream in the cache or checks that it is there; it does not compile.

Deserialization (`stx_map_stream_to_xasl`) is in the server. That is, **the consuming half is there
and the producing and keeping half is not.** The harness takes a pre-compiled XASL fixture, puts it
in the cache and runs it repeatedly by `XASL_ID`.

```
query generation (E3 SQLancer / picked by hand)
      ↓ client-side compilation + serialization
  the XASL fixture   ← §6a-E10 handles this (new). The part the engine does not have
      ↓ stx_map_stream_to_xasl (the engine has this)
  E9 replay — exploring the schedule only
```

**It does not overlap with E3.** E3 explores *the shape of a query* and generates a new one every
time. E9 needs no query generator and uses as its corpus **a small number of plans picked by hand
to create contention**. The axis of exploration differs (query vs interleaving) and so does the
oracle (wrong-result vs crash and corruption).

**The fixture schema** — the canonical form is what the client puts on the execution request (the
unpack of `sqmgr_execute_query ()`): `sql_user_text` (not the hash text — that is a rewritten hash
key), the XASL stream, the host variable bundle + `data_size`, `query_flag`, `query_timeout`, and
**the engine build identifier**. Why the last of those is mandatory is §4b.

## 4b. The engine does not check the stream's version

What `stx_map_stream_to_xasl ()` checks is a non-null pointer and `xasl_stream_size > 0`, and
nothing else. It goes straight on to read the header size with `or_unpack_int` and compute the
offsets. **There is no format check and no version check.** A stream from a different build is not
rejected; it is deserialized with offsets whose meaning has changed — it behaves oddly, silently.
So recording the build identifier and refusing on a mismatch is not optional, and it is **E10's to
implement**.

---

## 5. Reproducibility — not a problem of putting state back but of replaying a schedule

> **Redefinition (2026-09-03).** This section was first "state reset — this entry's central design
> difficulty". That frame came out of the single-threaded premise, and the redefinition changed the
> problem itself.

libFuzzer **repeats an input tens of thousands of times inside one process**. Had it been
single-threaded, the proposition needed would have been "the same input → the same state". **In a
multi-threaded setting that is neither obtainable nor wanted** — the schedule being
non-deterministic is precisely what is to be explored.

What is needed has a different shape:

- **Make the schedule part of the input.** Reproducing the `(operation, schedule)` pair is enough;
  the state does not have to be identical bit for bit.
- **reset drops from a gate to a preparation step** — the job of making a known starting DB.
- **The unit of crash triage changes** — what is stored is not the input bytes alone but the
  schedule that made that input reproduce.

### 5.1 The schedule control points are already in the engine

`fi_handler_hold` / `fi_handler_hang` in `src/base/fault_injection.c` widen the window or stop at a
named point. That is exactly why CBRD-27198 (merged 2026-09-02) put an `fi_handler_hold` hook into
`disk_reserve_sectors_in_volume` — *"The race lasts microseconds … a test can only hit that window
by luck."* **So a primitive for injecting a schedule exists and is already being used for that
purpose.** This entry is the job of laying a generator and invariant checks on top of it.

The limit is recorded alongside: the FI points are **a static enum**, so coverage of the schedule
space is limited to where a hook has been driven in. Adding points means work in that repository at
that moment, and CBRD-27198 already shows the convention for it (an NDEBUG gate plus a reserved
range per module).

### 5.2 The SA-mode spike (2026-09-03) — evidence about the preparation step, not about the gate

The spike: `fuzz/spike/reset_spike.cpp` on the cubrid `feat/fuzz-target-infrastructure` branch. It
boots the engine in-process in SA mode (`db_login` / `db_restart` — the path `compactdb` and
`checksumdb` already use), runs an operation sequence, resets, and turns everything that run
observed into a digest. One kind of digest means it is deterministic.

**Repeating only the same sequence is weak verification, and was not used.** Every input a fuzzer
sends differs from the one before, so the proposition that has to hold is *whatever comes before
it, the same sequence yields the same digest*. So two sequences that leave different residual state
were put between the variant 0 runs — a wide row that is not updated in place, and a rolled-back
unique violation.

| Strategy | Runs | Determinism | iter/sec | median | Note |
|---|---:|---|---:|---:|---|
| **A. abort + emptying the table** | **10,000** | **deterministic** (5,000 variant-0 runs, one kind of digest) | **126.2** | **7.6 ms** | uses only the public API. The cheapest and the fastest |
| A. (repeating the same sequence) | 10,000 | deterministic | 72.0 | 12.7 ms | weak verification. For reference |
| B. recreating the volume (`DROP`/`CREATE`) | 500 | deterministic | 41.2 | 23.6 ms | slower than A, as expected |
| B'. a full `db_shutdown` + `db_restart` | 50 | deterministic | 2.4 | 321.8 ms | 1/50 of A |
| C. fork() isolation | — | not measured | — | — | no longer needed for securing determinism. See §10 |
| ~~D. a dedicated reset hook~~ | — | — | — | — | **withdrawn.** A is already deterministic and faster. The amount of work in that repository becomes 0 |

**This is as far as the measurement goes: a preparation step that winds back to a transaction
boundary works, and is cheap.** The "deterministic reproduction" gate that was set as a condition
of entry is **not met by this measurement** — SA is single-threaded and is not this entry's target
configuration (§1.0). Reproducibility in the target configuration has to be obtained in the shape
of §5, that is by replaying a schedule, and nothing about it has been measured yet.

**What the measurement does not cover:**
- **It was measured in a single thread (SA).** This entry targets multi-threaded SERVER_MODE. That
  is the largest gap.
- The reset was measured **at the SQL level**. This entry hits internal APIs such as
  `heap_insert_logical` directly. Since the reset is per transaction it looks as though it will
  carry over, but that is an *inference*.
- 126 a second falls short of the thousands libFuzzer expects. Most of that is not the reset but
  the cost of six SQL statements going round the whole stack, and this measurement did not separate
  the two.
- Three kinds of operation sequence are not arbitrary operation sequences.

**And the shape of the harness is settled** — unlike E5's stateless target, this one **cannot be
self-contained.** `db_restart` demands an installed `$CUBRID` tree and a DB on disk. What is being
wound back is not a structure but files and a booted engine.

### 5.3 SERVER_MODE in-process startup — it works (2026-09-03)

`fuzz/spike/server_boot_spike.cpp`. It is `net_server_start()`'s order exactly, with the network
half taken out — upstream too, `boot_restart_server()` comes **before** `css_init()`. Only two
things were taken out: `net_server_init()` (static, and it only fills the request dispatch table
that in-process does not use) and `css_init()` (opening the socket).

```
boot_restart_server                            rc=0
BOOTED -- SERVER_MODE engine is up in-process with no listener.
live threads while booted                      15
xboot_shutdown_server                          ok=1
```

**The fifteen threads are the point.** Had `rc=0` come back with one thread, it would have been the
SA shape under a different name. The daemons are up, and that is the reason this library was
chosen.

### 5.3a The space of orderings is already saturated (2026-09-04) — the measurement that changed this entry's premise

`noise_floor_spike.cpp`. The input is **fixed** and repeated, and each participant draws a ticket
from a shared counter immediately before its operation. The ticket order is the observed
interleaving, and the number of distinct orders is counted.

| Mode | Threads | Repetitions | Distinct orders | Most frequent |
|---|---:|---:|---|---|
| control (tickets only, no engine work) | 4 | 2000 | **24 / 24 — 100%** | 5.8 · 5.3 · 4.9 · 4.8 · 4.7 % |
| `file_create_heap` | 4 | 1000 | **24 / 24 — 100%** | 5.2 · 5.1 · 5.1 · 4.9 · 4.9 % |

Uniform would be 4.17%, and both are near it. The engine-work side is if anything *more* uniform —
**the hypothesis that "the engine's latches narrow the order" is wrong.**

**The implication**: what schedule control buys is not exploration coverage — repetition alone gives
that for free. What it buys is **reproduction**. So **Tier 1 is the main effort** (time is
discovery), **Tier 2 is a triage tool** to be started *after* Tier 1 has produced a defect that
will not reproduce, and **the idea of libFuzzer exploring the schedule is discarded** — without
control points there is no correlation between the schedule encoded in the input and the actual
interleaving.

*The limits of the scope*: one kind of work, four participants, and the observation point is the
ticket rather than an event inside the engine. Work that puts real contention on a single hot page
may be more constrained.

### 5.3b The oracle was widened (2026-09-04) — the next improvement §5.3a pointed at

§5.3a ended with "the next improvement is the oracle, not the schedule". This is the result of that
work. The implementation is on `feat/fuzz-target-infrastructure` in the `cubrid` repository, and
the design rationale is the roadmap's `N66/10-design_fi-rendezvous.md` §9.2 and §9.3.

**The consistency checks — cost decides where they can go.**

| Check | Cost | Where it runs |
|---|---:|---|
| `disk_check ()` | 0.000 s | every input |
| `file_tracker_check ()` | 0.007 s | every input |
| `xboot_check_db_consistency (CHECKDB_ALL_CHECK_EXCEPT_PREV_LINK)` | **7.0 s** | the start and the end of a session |

The target is tens of runs a second, so one full check is worth hundreds of inputs. It goes **only
at the session boundaries**, not "every N runs". The check is run **both before and after the
workload** — run only afterwards, and the defect this input made cannot be told apart from what the
DB already had.

**A TSan build is one flag** — `-DFUZZ_SANITIZERS=thread`. It is exclusive with ASan so the engine
has to be built separately, but the source tree is one, and the spike is built by reading the
compile flags back out of the target build's `flags.make`. **ASan/UBSan and TSan are not a choice
between two; they are two runs of the same harness.**

**Without a baseline a sanitizer is not an oracle.** A clean four-thread run produces 222 TSan
reports and 10 UBSan reports every time. `sanitizer_triage.py` groups them by kind and by site (for
TSan, by the topmost *engine* frame) and `--suppress` pulls out a suppression file.

| File | Rules | With the baseline applied |
|---|---:|---|
| `tsan-baseline.supp` | 42 | 222 → **0** |
| `ubsan-baseline.supp` | 3 | 10 → **0** |

**A baseline is not a verdict.** Not one entry was judged sound or unsound — the page buffer and
the log append use hand-written atomics, so TSan has no reason to be believed there. The only
purpose is **to make silence mean something**, and it was worth that in practice: widening the
workload to bring in `xheap_destroy` reached `vacuum_add_dropped_file ()`, and TSan reported a new
race there 40 times. What would have been invisible among the 179 existing reports was, on top of
silence, the one thing on the screen. ASan is **deliberately left out of the baseline** — an ASan
report is a memory error that stops the run, so suppressing it hides the defect.

**Two traps (both fail silently).**
- `DEBUGINFOD_URLS=` has to be emptied. Otherwise the process stops on the first report **with zero
  CPU time** — `llvm-symbolizer` goes off to fetch debug info over HTTP and blocks, and TSan holds
  the trace-part semaphore while it symbolizes, so every other thread piles up behind it. It is
  indistinguishable from an engine deadlock.
- UBSan suppression has **no line granularity.** A rule is `<check>:<file>`, so one rule covers
  that whole file. A baseline that silently widens is worse than none, so it is stated here that
  the precise tool is the compile-time `-fsanitize-ignorelist`.

**A harness thread that calls itself `TT_WORKER` has to actually be one.** The engine reads the
type back as a promise — `log_tran_table.c:2896` records in a comment that "Only TT_WORKER threads
use pl_session", and in a `TT_WORKER` with no connection entry **every `log_sysop_start ()` sets
`ER_SES_SESSION_EXPIRED` as a side effect**. Most paths ignore it, but `heap_insert_logical ()`
returns on it, so every insert fails. The fix is not to pick a different type but **to keep the
promise**: the `CSS_CONN_ENTRY` array is allocated inside `boot_restart_server` and
`css_initialize_conn ()` does not touch a socket, so `css_make_conn (INVALID_SOCKET)` gives a real
connection entry with no connection. This is **a constraint that applies to in-process harnesses in
general**, which is why it is recorded here.

**How it is run — `soak.sh`.** §5.3a ended with "there is nothing to control", so Tier 1's
exploration is just time, and the only question left is how that time is spent. It is spent **in
sessions** — boot · the before check · the workload · the after check · shutdown. Both reasons it is
not one long run come out of the table above: the full check takes 7 s so it can only go at the
boundaries, and a check taken *while* four threads are changing things reports not a wrong state
but a **torn** one. Repeating sessions has the side effect of re-testing boot and shutdown
themselves every time. The stopping conditions are just three — an abnormal exit, an oracle
failure, and a sanitizer report **the baseline does not cover** — and the rest is silence.

**The first soak result (2026-09-04, two hours).** 205 sessions · 164,000 heaps · 1,312,000
records · **0 reports outside the baseline**. The oracle passed before and after in every session.
This is not a conclusion about the engine but a negative result — **this workload catches nothing
at this scale** — and it is the number the next judgement has to answer to: **time is no longer the
lever.**

Two things the soak did confirm. **The workload does not give the DB back** — it grows by 199 MB a
session and is not reclaimed (dropping a heap defers reclamation). Against the 8 GB cap the runner
recreated five times, about once every 41 sessions. And **that growth is what made the oracle
slow**: `file_tracker_check` was 1.359 s at session 190 and came back to **0.012 s** at session 191,
right after a recreation. The cost tracks the accumulation exactly and resets with it, so the linear
rise observed earlier is files piling up, not a performance regression.

The 32-session soak before it filled the host filesystem and stopped, and **reported that as though
it were a finding.** It was not a finding. The runner now tells the two apart, but the lesson is
that distinction itself — an unattended runner that cannot tell "the engine failed" from "this
machine has no room" ends up crying wolf, and it did.

### 5.4 But FI cannot *replay* a schedule (2026-09-03)

§5.1 recorded that "a primitive for injecting a schedule already exists". That is only half right.
Corrected after reading the API.

**What is there:**
- **Arming per thread.** The FI state lives in `thread_p->fi_test_array` (`fi_thread_init`,
  SERVER_MODE only). `fi_set (thread_p, code, state)` hooks *one particular thread* and no other.
  "Only thread A stops at this point" can be expressed
- **Named injection points** — the FI_TEST_CODE enum

**What is not there — and this is what blocks the gate:**

| Handler | Implementation | As a schedule primitive |
|---|---|---|
| `fi_handler_hold` | `sleep (seconds)` | **time-based**. It widens the window; it does not fix the order |
| `fi_handler_hang` | `while (true) sleep (1);` | **a permanent stop. There is no path to release it** |

Neither looks at the state again while sleeping, so another thread cannot wake it with `fi_set`.
That is, **"A waits here until B reaches point Y" cannot be expressed.**

**The conclusion — it splits into two tiers:**

- **Tier 1 (possible today).** Widening the window + invariant checks. It exposes a race
  *probabilistically*. That is exactly what CBRD-27198 does, and it is genuinely useful. But there
  is no replay, so triage stays at the level of recording the FI settings and the seed
- **Tier 2 (once one handler is added to the engine).** A rendezvous handler that waits on a
  condition variable and is woken by the harness — something like `fi_handler_wait`. Once that
  exists, an interleaving can be replayed deterministically. **The amount of work is about adding
  one more beside the four existing handlers, and CBRD-27198 already shows the convention for it
  (adding a handler plus a hook)**

So what this entry asks of the engine is neither a reset hook nor more schedule points but **one
rendezvous handler**. That becomes the central decision item of ADR-EXT-009.

---

## 6. User requirements (incubating estimate)

1. **operation sequence generation** — mutate a structurally valid operation sequence
2. **deterministic reproduction** — the same input → the same crash. The reset contract guarantees
   it
3. **a reproducer a person can read** — dump the crashing input as `INSERT/UPDATE/SCAN/COMMIT` text
4. **invariant hooks** — checkpoints that can catch a violation even without a crash (page
   integrity / OID validity)
5. **crash dedup** — based on the stack hash
6. **accumulating regression seeds** — re-run past crash sequences against a new build
7. **a time / iteration budget** — controlling CI time
8. **sharing the infrastructure with E5** — keeping the corpus, triage and coverage reporting use
   the same thing

---

## 7. Non-functional requirements

| Item | Agenda | What it means in the new system |
|------|------|---------------------|
| Cost of adoption | **high** | higher than E5 (medium) — the state reset design is added |
| Immediate ROI | ★★★ | great value once reached. But the cost that comes first is large, so it is low on the ladder |
| Prerequisites | E5's `-DENABLE_FUZZING` infrastructure + the in-process boot entry point | testkit cannot decide alone |
| Responsibility boundary | testkit = corpus + replay + triage / cubrid = the target + the reset hook | **C-055** — on the engine side, **N66-fuzz-target-infrastructure** |
| Licence | libFuzzer Apache 2.0 · protobuf BSD-3 · libprotobuf-mutator Apache 2.0 | free. Linked fuzz-only |
| Throughput | dominated by the reset cost per input | the choice of strategy in §5 is the throughput decision |
| Relation to E3 (SQLancer) | complementary — E3 is wrong-result at the SQL layer, this entry is internal API state | not a duplicate |

---

## 8. External resources it depends on

- **The fuzz infrastructure in the cubrid repository** — `-DENABLE_FUZZING` (shared with E5) + the
  storage fuzz target + the reset hook. *Comes first*
- **Sanitizer builds** — ASan / UBSan (that repository's responsibility). Whether MSan needs the
  whole server rebuilt is to be examined
- **libFuzzer** — the in-process coverage-guided fuzzer
- **protobuf + libprotobuf-mutator** — structure-aware mutation (decided after the comparison with
  the alternatives in §2.1)
- **A seed corpus** — meaningful operation sequences (§ test-corpus.md)
- **Crash corpus storage** — NG1 check (outside the testcases repository)

---

## 9. Conditions for entering incubating (conditional)

Formal entry into incubating *after* the following are met (owner: hgryoo):

1. **E5 first** — `-DENABLE_FUZZING` and the crash triage infrastructure have to stand first. This
   entry is laid on top of them.
   **Partly met (2026-09-03)**: `-DENABLE_FUZZING` and the first target (ladder rank 4,
   `or_get_value ()`) exist and work on cubrid `feat/fuzz-target-infrastructure`. The corpus,
   replay and triage are not there yet
2. ~~**SERVER_MODE in-process startup**~~ → **met (2026-09-03, §5.3).** It starts with no listener,
   fifteen daemon threads come up and it goes down cleanly. ~~SA-mode `db_restart`~~ is not the
   target (§1.0)
3. **Schedule replay — not possible with FI as it stands (confirmed 2026-09-03, §5.4).**
   Deterministic replay needs a rendezvous primitive, and FI does not have one. **Adding one
   handler to the engine resolves it** — until then only Tier 1 (widening the window, probabilistic
   exposure) is possible. Strategy D (a dedicated reset hook) is left withdrawn — the problem
   stopped being reset
4. **The XASL fixture equipment (§6a-E10) comes first** — since the server does not compile SQL
   (§4a), Tier 2 cannot be started without it. **Tier 1 can go ahead regardless of this condition**
   and is in fact going ahead — but Tier 1 calls the storage internal APIs directly, so it carries
   the calling-convention problem of §4 as it stands, and its findings come with a reachability
   triage.
   **That cost was measured on 2026-09-04**: widening the workload to a heap's whole life (create →
   8 records that straddle pages → drop or rollback) took three calling-convention violations, and
   **all three were harness defects rather than engine ones** — calling `file_create_heap ()`
   directly (→ `xheap_create ()`), a NULL class OID (→ the root class), and a `TT_WORKER` with no
   session (§5.3b). The oracle caught each one, and each time "is it the engine or is it me?" had
   to be settled against the source. Details: roadmap N66 §5
5. **The cap on participants settled** — the thread count is part of the input but is **clamped**
   (not rejected) to the cap the engine enforces. The cap is the connection share of
   `m_max_threads` that goes unused in this configuration, and with no listener there is room. No
   configuration change is needed still, but **corrected 2026-09-04**: each participant uses one
   real `CSS_CONN_ENTRY` (§5.3b), so `max_clients` is not an irrelevant value but **the cap on
   participants** — `css_make_conn ()` returns `NULL` above it
6. **The input IR decided** — it describes **the schedule and the number of participants**, not the
   operations. libprotobuf-mutator vs FuzzedDataProvider (§2.1)
7. **Where the corpus lives** — NG1 check. The schedule is part of the input, so the unit of
   storage gets bigger
8. **The scope of the schedule control points** — whether to use only the existing FI enum or to
   add points (§5.4)
9. **C-055** — registered in the roadmap's cross-cutting list (2026-09-03). The work on the engine
   side is **N66-fuzz-target-infrastructure**. Its §9 Q1 (*a testing-only reset hook*) **has become
   moot** — the measurements pass the gate without the hook, so the answer to it does not decide
   how far this entry reaches

**ADR placeholder:**
- **ADR-EXT-009** — the input IR (protobuf/LPM vs FDP) + the state reset strategy + the first scope
  of the operation vocabulary + where the corpus lives + the responsibility boundary in that
  repository

> **A note on the numbering.** `E8` / `ADR-EXT-008` is already reserved for the axis 8 slot,
> *Hybrid CI integration* (the `extensions/README.md` catalogue). That is why this entry is `E9` —
> the ladder's rank (5) and the catalogue ID (9) are **separate number spaces**.

---

## 10. Risk and consistency notes

1. **Schedule replay may not work (the biggest risk).** To reproduce the `(operation, schedule)`
   pair, the points where FI hooks are driven in have to be enough to determine the interleaving.
   The stretches between hooks are still the OS scheduler's to decide, so reproduction may only be
   probabilistic. Crash triage then becomes meaningless — which is why condition of entry 3 is a
   gate.
1b. **A correction recorded 2026-09-03.** What stood here was "state reset may not work", and the
   result of the SA spike was written up as "resolved". **That was a claim beyond its scope** — SA
   is single-threaded and is not the target configuration. All that measurement leaves is the fact
   that the preparation step is cheap (§5.2).
2. **Resistance in that repository to a new protobuf dependency.** Even linked fuzz-only it may be
   refused on 3rdparty policy → the FuzzedDataProvider path in §2.1 is the fallback.
3. **Trying this entry first and skipping E5.** It means building the infrastructure twice → keep
   to the ladder's order.
4. **NG1 conflict.** Putting the crash corpus in the testcases repository violates the freeze →
   external storage.
5. **No conflict with NG2 / NG4.**
5b. **Folding fault injection into the operation sequence makes `fork()` isolation mandatory.** The
   engine's FI handlers (`fi_handler_exit` / `fi_handler_hang`, `src/base/fault_injection.c`) end
   the process. That is incompatible with libFuzzer's in-process model, so exploring the crash
   point × the operation sequence together makes strategy C not a second best but a precondition.
   That is a separate matter from §5.1 having made C unnecessary for the purpose of determinism.
6. **Confusing the boundary with axis 7 (E7 stateful workload).** E7 is a long-running scenario at
   *the SQL/node level*; this entry is a single process at *the internal API level*. To be recorded
   together in the C-004 boundary definition.
7. **The branch gate, §7** — the strangler fig comes first. Lower priority where it conflicts with
   phases 3 and 4 for resources.

---

## 11. References

- RocksDB fuzzing: <https://github.com/facebook/rocksdb/tree/main/fuzz>
- libprotobuf-mutator: <https://github.com/google/libprotobuf-mutator>
- libFuzzer: <https://llvm.org/docs/LibFuzzer.html>
- libFuzzer `FuzzedDataProvider`: <https://llvm.org/docs/LibFuzzer.html#fuzzer-friendly-build-mode>
- OSS-Fuzz: <https://google.github.io/oss-fuzz/>
- Internal: `../E5-parser-fuzzing/requirements.md` · `../../../project/ROADMAP.md` §6a ladder · `../../../project/survey/dbms-testing-ecosystem.md` §7.4
