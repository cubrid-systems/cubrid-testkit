# E5 — I/O Contract (STUB)

*English · [한국어](io-contract.ko.md)*

**Status:** STUB — the contract is frozen after formal entry into incubating through ADR-EXT-005.
**Source:** `requirements.md` §2

---

## 1. CLI (proposed)

```
ctp.sh fuzz-harness [-c <fuzz.conf>] [--target parser|cci|jdbc|record] [--corpus <dir>]
   or
testkit run fuzz [--target parser|cci|jdbc|record] [--time <sec>] [--max-len <bytes>] [--fuzzer libfuzzer|afl|honggfuzz]
```

## 2. conf schema (TBD)

| Key (agenda) | Value | Source |
|---|---|---|
| `target` | `parser \| cci \| jdbc \| record` | the fuzz target layer. **The value space is left open so that it can take E9's `storage`** (settled in ADR-EXT-005) |
| `fuzzer` | `libfuzzer \| afl \| honggfuzz` | the fuzzer itself |
| `time_budget_sec` | `<int>` | controlling CI time |
| `max_len` | `<int>` | the maximum length of an input |
| `corpus_root` | `<dir>` | where the seed and crash corpus lives |
| `sanitizers` | csv `asan,ubsan,msan` | which sanitizers are active |
| `regression_only` | `bool` | replay past crashes only |

Keys and meanings are frozen after ADR-EXT-005.

## 3. Output format (TBD)

```
<resultDir>/
   ├── main.info
   ├── crash/
   │     └── <stack-hash>/
   │           ├── input.bin           # raw input bytes
   │           ├── stack.txt
   │           ├── sanitizer.txt       # ASan / UBSan / MSan report
   │           └── reproducer.sh
   ├── coverage/
   │     └── coverage.json             # per-build edge coverage
   └── stats.json                      # execs / unique crashes / coverage delta
```

## 4. Exit codes (TBD)

| Value | Meaning |
|---|---|
| 0 | no crash within the time |
| 1 | a crash was found |
| ≥2 | an infrastructure error (the fuzz target is not built, no sanitizer is active, and so on) |

## 5. The NG2 / NG4 check

A new entry point — no conflict.

## 6. The contract on the cubrid repository's side (a prerequisite)

| Item | testkit's agenda | The cubrid repository's responsibility |
|---|---|---|
| build option | `-DENABLE_FUZZING=ON` | added to that repository's cmake / configure |
| fuzz target binary | `cubrid-fuzz-{target}` | an output of that repository's build |
| in-process harness signature | `int LLVMFuzzerTestOneInput(const uint8_t*, size_t)` | the C function defined in that repository |
| sanitizer build | `-DSANITIZE=asan \| ubsan \| msan` | a build option of that repository |

Both sides' contracts are frozen at once as the C-055 cross-cutting (on the engine side,
N66-fuzz-target-infrastructure).
