# E9 — I/O Contract (STUB)

*English · [한국어](io-contract.ko.md)*

**Status:** STUB — the contract is frozen after formal entry into incubating through ADR-EXT-009.
**Source:** `requirements.md` §3

---

## 1. CLI (proposed)

```
testkit run fuzz --target storage [--time <sec>] [--ops <n>] [--corpus <dir>]
                                  [--reset abort|recreate|fork|hook]
```

It shares E5's value space for `--target`. E5's current values are
`parser | cci | jdbc | record`, and adding the value `storage` is **settled in ADR-EXT-005** (E5's
io-contract §2 states the room for the extension).

## 2. conf schema (TBD)

| Key (agenda) | Value | Where it comes from |
|---|---|---|
| `target` | `storage` | the value space shared with E5 |
| `mutator` | `lpm \| fdp` | the input IR (ADR-EXT-009) |
| `reset_strategy` | `abort \| recreate \| fork \| hook` | requirements §5 |
| `ops_per_input` | `<int>` | the maximum number of operations in one input |
| `op_vocabulary` | csv `heap,btree,vacuum,checkpoint` | the first scope of the vocabulary |
| `invariant_checks` | `bool` | detecting a violation even without a crash |
| `time_budget_sec` | `<int>` | controlling CI time |
| `corpus_root` | `<dir>` | the seed + crash corpus |
| `sanitizers` | csv `asan,ubsan` | which sanitizers are active |

Keys and meanings are frozen after ADR-EXT-009.

## 3. Output format (TBD)

```
<resultDir>/
   ├── main.info
   ├── crash/
   │     └── <stack-hash>/
   │           ├── input.bin            # libFuzzer's original input
   │           ├── sequence.txt         # the op sequence a person reads (the key artefact)
   │           ├── stack.txt
   │           ├── sanitizer.txt
   │           └── reproducer.sh
   ├── invariant/
   │     └── <check-id>/                # a violation with no crash
   ├── coverage/coverage.json
   └── stats.json                       # execs / resets / unique crashes / coverage delta
```

An example of `sequence.txt` (the format is not settled):

```
INSERT  oid=? len=137
INSERT  oid=? len=4021          # the overflow promotion boundary
UPDATE  oid=#1 len=8
SCAN    hfid=#0
VACUUM
COMMIT
```

## 4. Exit codes (TBD)

| Value | Meaning |
|---|---|
| 0 | no crash and no invariant violation within the time budget |
| 1 | a crash found |
| 2 | an invariant violation (with no crash) |
| ≥3 | an infrastructure error (the target is not built / the reset hook is absent / non-deterministic reproduction detected) |

## 5. NG2 / NG4 check

A new entry point — no conflict.

## 6. The contract on the cubrid repository side (comes first)

| Item | testkit's agenda | The cubrid repository's responsibility |
|---|---|---|
| build option | `-DENABLE_FUZZING=ON` | **the same option shared** with E5 |
| fuzz target binary | `cubrid-fuzz-storage` | a build artefact of that repository |
| harness signature | `int LLVMFuzzerTestOneInput(const uint8_t*, size_t)` | defined in that repository |
| custom mutator (if LPM is adopted) | `LLVMFuzzerCustomMutator` | defined in that repository. protobuf is linked only into the fuzz build |
| the state reset hook | initialization at every input boundary | **specific to this entry** — E5 does not have it |
| in-process boot/shutdown | booting on a temporary volume | an entry point is needed in that repository |

The contracts on both sides are frozen together as cross-cutting C-055 (on the engine side,
N66-fuzz-target-infrastructure).
