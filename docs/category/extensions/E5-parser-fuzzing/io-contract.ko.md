# E5 — I/O Contract (STUB)

*[English](io-contract.md) · 한국어*

**Status:** STUB — contract 동결은 ADR-EXT-005 incubating 정식 진입 후.
**Source:** `requirements.md` §2

---

## 1. CLI (제안)

```
ctp.sh fuzz-harness [-c <fuzz.conf>] [--target parser|cci|jdbc|record] [--corpus <dir>]
   또는
testkit run fuzz [--target parser|cci|jdbc|record] [--time <sec>] [--max-len <bytes>] [--fuzzer libfuzzer|afl|honggfuzz]
```

## 2. conf 스키마 (TBD)

| 키 (의제) | 값 | 출처 |
|---|---|---|
| `target` | `parser \| cci \| jdbc \| record` | fuzz target layer. **E9 의 `storage` 를 받을 수 있도록 값 공간을 열어 둔다** (ADR-EXT-005 에서 확정) |
| `fuzzer` | `libfuzzer \| afl \| honggfuzz` | fuzzer 본체 |
| `time_budget_sec` | `<int>` | CI 시간 통제 |
| `max_len` | `<int>` | 입력 최대 길이 |
| `corpus_root` | `<dir>` | seed + crash corpus 위치 |
| `sanitizers` | csv `asan,ubsan,msan` | 활성 sanitizer |
| `regression_only` | `bool` | 과거 crash 만 replay |

ADR-EXT-005 후 키/의미 동결.

## 3. 출력 포맷 (TBD)

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

## 4. 종료 코드 (TBD)

| 값 | 의미 |
|---|---|
| 0 | 시간 내 crash 없음 |
| 1 | crash 발견 |
| ≥2 | infra 오류 (fuzz target 미빌드 / sanitizer 미활성 등) |

## 5. NG2 / NG4 점검

신규 진입점 — 충돌 없음.

## 6. cubrid 본 repo 측 contract (선결)

| 항목 | testkit 의제 | cubrid 본 repo 책임 |
|---|---|---|
| build option | `-DENABLE_FUZZING=ON` | 본 repo 의 cmake / configure 추가 |
| fuzz target binary | `cubrid-fuzz-{target}` | 본 repo build 산출물 |
| in-process harness signature | `int LLVMFuzzerTestOneInput(const uint8_t*, size_t)` | 본 repo C 함수 정의 |
| sanitizer 빌드 | `-DSANITIZE=asan \| ubsan \| msan` | 본 repo build option |

C-055 cross-cutting 으로 양 측 contract 동시 동결 (엔진 쪽 = N66-fuzz-target-infrastructure).
