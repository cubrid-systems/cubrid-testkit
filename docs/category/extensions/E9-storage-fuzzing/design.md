# E9 — Design (STUB)

*English · [한국어](design.ko.md)*

**Status:** STUB — the real design comes after formal entry into incubating through ADR-EXT-009.
E5's `-DENABLE_FUZZING` infrastructure plus the state reset spike come first.
**Source:** ROADMAP §6a-E9, `requirements.md`, `project/survey/dbms-testing-ecosystem.md` §7.4
**Companion:** `requirements.md` (FULL), `io-contract.md` (STUB), `test-corpus.md` (STUB)

---

## 1. The responsibility boundary (cross-repo)

```
the cubrid repository                     testkit (this module)
─────────────                            ───────────────
the storage fuzz target              →   keeping the corpus (seed + crash)
in-process boot / shutdown entry     →   replay (regression)
the state reset hook                 →   crash triage (stack hash dedup)
sanitizer builds (ASan/UBSan)        →   op sequence → a reproducer dump a person can read
the LLVMFuzzerTestOneInput impl      →   coverage reporting
```

It **shares the same infrastructure** as E5. All this entry asks for on top is the *reset hook*.

## 2. The layers (agenda)

```
libFuzzer
   ├─ mutator: libprotobuf-mutator | FuzzedDataProvider   ← ADR-EXT-009
   ▼
StorageOpSequence (in-memory)
   ▼
translate()  ── op → calling heap_* / btree_* / log_* directly
   ▼
CUBRID storage engine (in-process, a temporary volume)
   ▲
reset()  ── called at every input boundary
```

protobuf exists **only in the mutator layer**. Below `translate()` there is no protobuf, and
CUBRID's own protocol and serialization are *not gone through at all* (`requirements.md` §2).

## 3. Where the module sits (agenda)

```
internal/runner/fuzzharness/          # shared with E5
   ├── runner/
   ├── corpus/
   ├── triage/
   ├── coverage/
   └── storage/                       # ← new in this entry
         ├── opdump/                  # op sequence → a text reproducer
         └── invariant/               # collecting the results of optional checkpoints
```

## 4. The state reset strategy — held over

The A/B/C/D comparison table in `requirements.md` §5. Decided by the result of the spike.
**The rest of this document depends on that decision and so cannot be written yet.**

## 5. Decisions held over → ADR-EXT-009

- the input IR (libprotobuf-mutator vs FuzzedDataProvider vs our own)
- the state reset strategy
- the first scope of the operation vocabulary (heap alone / +btree / +vacuum / +checkpoint)
- where the corpus lives (NG1 check)
- whether there are invariant hooks

## 6. The trigger for writing the design

After the E5 infrastructure, the state reset spike and ADR-EXT-009. A stub for now.
