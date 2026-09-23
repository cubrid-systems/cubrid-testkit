# E5 — Design (STUB)

*English · [한국어](design.ko.md)*

**Status:** STUB — the real design comes after formal entry into incubating through ADR-EXT-005.
`-DENABLE_FUZZING` in the cubrid repository comes first.
**Source:** ROADMAP §6a-E5, `requirements.md`, `project/survey/dbms-testing-ecosystem.md` §7
**Companion:** `requirements.md` (FULL), `io-contract.md` (STUB), `test-corpus.md` (STUB)

---

## 1. The responsibility boundary (cross-repo)

```
the cubrid repository                  testkit (this module)
─────────────────────                  ─────────────────────
fuzz target build option           →   keeping the corpus
sanitizer build (ASan/UBSan/MSan)  →   replay (regression)
in-process harness function        →   crash triage (stack hash dedup)
                                       coverage reporting
```

It cannot be started by testkit alone. It is registered as the **C-055** cross-cutting in the
roadmap repository, and the engine-side work is **N66-fuzz-target-infrastructure**
(00-pending-review).

## 2. Where the module sits (agenda)

```
internal/runner/fuzzharness/
   ├── runner/           # calls libFuzzer / AFL / honggfuzz
   ├── corpus/           # keeps the seeds and the crashes
   ├── triage/           # stack hash dedup + sanitizer classification
   └── coverage/         # compares coverage build by build
```

## 3. The fuzz target layer (agenda)

| Layer | target | Estimated ROI | Work in the cubrid repository |
|---|---|---|---|
| lexer | the `lex_consume` entrance | ★★ | little |
| parser | the `parser_main` entrance | ★★★★ | little |
| binder | catalog resolution | ★★★ | middling |
| planner | the optimizer entrance | ★★★ | middling |
| CCI protocol | binary message handler | ★★★★ | middling (the handler has to be split out) |
| JDBC protocol | wire protocol parser | ★★★★ | middling |
| record ser/unpack | `or_get_value` / `or_unpack_value` | ★★★ | little (pure functions) |

ADR-EXT-005 chooses the first layer (the parser is recommended — cost ↓, value ↑).
The order of starting is the ROADMAP **§6a ladder**: parser(1) → CCI/JDBC(3) → record ser/unpack(4).
Rank 5 after that (storage operation sequence) is not this entry but **E9**.

## 4. Data flow (agenda)

```
seed corpus → fuzzer (libFuzzer) → fuzz target (cubrid in-process)
                                         ├─ ok        → coverage update
                                         └─ CRASH (signal | sanitizer)
                                              └─ triage.dedupe(stack)
                                                   └─ corpus.save({input, stack, sanitizer})
```

## 5. External dependencies

- the `-DENABLE_FUZZING` build option in the cubrid repository (a prerequisite)
- the ASan / UBSan / MSan build outputs (the cubrid repository's responsibility)
- libFuzzer / AFL / honggfuzz
- a seed corpus (see test-corpus.md)

## 6. Held over for decision → ADR-EXT-005

- which fuzz target layer comes first
- the fuzzer itself (libFuzzer in-process / AFL subprocess / honggfuzz)
- where the corpus lives (the NG1 check)
- the C-055 responsibility boundary ADR (on the engine side, N66)

## 7. The trigger for writing the design

After the PR to the cubrid repository and ADR-EXT-005. A stub for now.
