# E5 — Parser / Protocol Fuzzing Harness (libFuzzer) (Requirements)

*English · [한국어](requirements.ko.md)*

**Source:** survey/dbms-testing-ecosystem.md §7 + §11
**Status:** incubating (conditional — the fuzz target build option in the cubrid repository comes first)
**Axis mapping:** axis 5 (Parser / compiler fuzzing)
**Place on the ladder:** ROADMAP §6a ladder ranks 1 (SQL parser), 3 (network packet decoder) and 4 (record serialize/unpack)
**The entry that follows:** `../E9-storage-fuzzing/` — *structured stateful fuzzing* laid on top of this entry's infrastructure (ladder rank 5)
**Companion docs (to follow):** `design.md`, `io-contract.md`, `test-corpus.md`, `implementation-notes.md`

---

## 1. The problem this extension solves

It verifies the robustness of **CUBRID's frontend (lexer / parser / binder / planner)** and of its
**binary protocol entry points (CCI / JDBC)** with *byte-level coverage-guided fuzzing*.

Every existing testkit module looks only at the *valid SQL surface*. What is fuzzed:
- SQL parser crash / UB
- heap overflow / stack corruption
- infinite recursion
- binary protocol parser corruption (CUBRID's CCI / JDBC protocol entry points — ladder rank 3)
- **the record serialize / unpack path** (`or_get_value` / `or_put_value` / `or_unpack_value`,
  `src/base/object_representation.h` — ladder rank 4). This is the point where bytes read from disk
  or off the wire are restored into a `DB_VALUE`, and its robustness against a *corrupted record*
  as input has never been verified

Every one of the targets above is **stateless byte-in** in form — they share the same
`LLVMFuzzerTestOneInput` signature and use the same corpus and triage infrastructure. A sequence of
storage operations, where *state accumulates*, was split out into a separate entry (**E9**) instead
— because a different design problem, state reset, comes attached to it.

§6a-E2 (SQLsmith) is random SQL that *passes the grammar*; this entry (libFuzzer) is byte-level
mutation *outside the grammar*. The two are *complementary*: libFuzzer makes the SQL SQLsmith
cannot, and *that is precisely why it catches parser crashes well*.

**The kind of bug it catches:** parser crash / heap overflow / stack corruption / infinite recursion
/ protocol parser corruption / an out-of-bounds when a corrupted record is unpacked.

---

## 2. How it is called from outside (proposed — incubating)

```
ctp.sh fuzz-harness [-c <fuzz.conf>] [--target parser|cci|jdbc|record] [--corpus <dir>]
   or
testkit run fuzz [--target parser|cci|jdbc|record] [--time <sec>] [--max-len <bytes>]
```

The internal entry point (agenda):
```
FuzzHarnessDriver.exec(config)
  ├─ start cubrid server (or in-process target)
  ├─ select target binary (built with -DENABLE_FUZZING by the cubrid repository)
  ├─ run libFuzzer / AFL with seed corpus
  ├─ on crash:
  │    save input bytes + stack hash
  │    triage: dedupe by stack hash
  └─ replay mode:
       feed past crash inputs to new build (regression seed)
```

**The limit of testkit's responsibility:** keeping the corpus, replay, and crash triage.
**The cubrid repository's responsibility:** adding the fuzz target build option (`-DENABLE_FUZZING`
and the like).

**Effect on the external surface freeze:** none (a new entry point).

---

## 3. What users need (an incubating estimate)

1. **Choosing the fuzz target layer** — one or more of lexer / parser / binder / planner / executor
   / CCI protocol / JDBC protocol / record serialize·unpack
2. **Managing the seed corpus** — an initial seed of meaningful SQL and meaningful protocol messages
3. **crash dedup** — duplicate crashes merged on the stack hash
4. **Accumulating regression seeds** — re-running past crash inputs against a new build
5. **Integrating coverage feedback** — comparing libFuzzer's or AFL's coverage information build by
   build
6. **A time / iteration budget** — running within a fixed time inside CI
7. **Triage reporting** — crashes classified (signal / address sanitizer / undefined behavior)

---

## 4. Non-functional requirements

| Item | Agenda | What it means in the new system |
|------|------|---------------------|
| Cost of adoption | middling (testkit alone) to high (including collaboration with the cubrid repository) | survey §7.3 |
| Immediate ROI | ★★★ | of great value, but taking it as testkit's responsibility alone *risks inconsistency* |
| Prerequisite | a build option such as `-DENABLE_FUZZING` in the cubrid repository | testkit cannot decide it alone |
| The responsibility boundary | testkit = corpus + replay + triage / cubrid = the fuzz target build | **C-055** (a roadmap cross-cutting, registered) — on the engine side, **N66-fuzz-target-infrastructure** |
| Licence | libFuzzer Apache 2.0 / AFL Apache 2.0 | free |
| Complementarity with §6a-E2 (SQLsmith) | grammar-aware vs byte-level — both detect parser crashes | together when hybrid CI (E8) comes |

---

## 5. External resources it depends on

- **The fuzz target build option in the cubrid repository** — `-DENABLE_FUZZING` or its equivalent.
  *A prerequisite*. testkit cannot decide it alone
- **libFuzzer / AFL / honggfuzz** — the coverage-guided fuzzer itself
- **ASan / UBSan / MSan** — the sanitizer build (the cubrid repository's responsibility)
- **A seed corpus** — the initial asset of meaningful SQL and protocol messages
- **Crash corpus storage** — inside testkit or outside it (the NG1 check)

---

## 6. The conditions for entering incubating (conditional)

Formal entry into incubating comes *once these are met* (owner: hgryoo):

1. **A PR to the cubrid repository first** — adding the `-DENABLE_FUZZING` build option and defining
   the sanitizer build's output. testkit cannot start on its own
2. **Choosing the first fuzz target layer** — the SQL parser only / plus the CCI and JDBC
   protocols / plus record serialize·unpack. **The recommended order is the ROADMAP §6a
   ladder** (1 → 3 → 4)
3. **Choosing the fuzzer itself** — libFuzzer (in-process) vs AFL (subprocess) vs honggfuzz
4. **The seed corpus policy** — whether to convert the existing sql module's cases into seeds, or to
   build a separate seed asset
5. **Where the crash corpus lives** — checked against the testcases repository freeze (NG1)
6. **C-055 (a roadmap cross-cutting, registered 2026-09-03)** — testkit §6a-E5 ×
   **N66-fuzz-target-infrastructure** — where the responsibility for the fuzz target build sits. The
   engine-side work is registered in the roadmap repository under
   `projects/00-pending-review/N66-fuzz-target-infrastructure/`, and that is the real thing
   prerequisite 1 above points at. It also touches the build-surface boundary of C-004
   (testkit × engine-suite)

**ADR placeholder:**
- ADR-EXT-005 — the fuzz target build option (on the cubrid repository's side), the choice of
  fuzzer, where the corpus lives, and the responsibility boundary

---

## 7. Notes on risk and consistency

- **The risk of breaking the responsibility boundary** — if testkit takes on the fuzz target build
  as well, the *module boundary becomes inconsistent*. A PR on the cubrid repository's side comes
  first
- **Possible conflict with NG1** — putting the crash corpus into testcases violates the freeze.
  External storage is recommended
- **No conflict with NG2 / NG4**
- **§6a-E2 (SQLsmith) together with E5 (libFuzzer)** — both detect parser crashes, but over
  different areas — adopting them together is of great value
- **The branch gate, §7** — the strangler fig comes first — lower priority where it competes for
  resources with phases 3 and 4
- **E9 (storage structured fuzzing) depends on this entry** — it reuses `-DENABLE_FUZZING`, the
  keeping of the corpus, the triage and the coverage reporting just as they are. This entry's
  infrastructure decisions bind E9, so ADR-EXT-005 leaves the target value space (`--target`) open
  *so that it can be extended*
