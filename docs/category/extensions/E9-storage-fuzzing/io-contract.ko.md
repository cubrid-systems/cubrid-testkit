# E9 — I/O Contract (STUB)

*[English](io-contract.md) · 한국어*

**Status:** STUB — contract 동결은 ADR-EXT-009 incubating 정식 진입 후.
**Source:** `requirements.md` §3

---

## 1. CLI (제안)

```
testkit run fuzz --target storage [--time <sec>] [--ops <n>] [--corpus <dir>]
                                  [--reset abort|recreate|fork|hook]
```

E5 의 `--target` 값 공간을 공유한다. E5 현행 값은 `parser | cci | jdbc | record` 이며,
`storage` 값 추가는 **ADR-EXT-005 에서 확정** 한다 (E5 io-contract §2 에 확장 여지가 명시됨).

## 2. conf 스키마 (TBD)

| 키 (의제) | 값 | 출처 |
|---|---|---|
| `target` | `storage` | E5 와 공유하는 값 공간 |
| `mutator` | `lpm \| fdp` | 입력 IR (ADR-EXT-009) |
| `reset_strategy` | `abort \| recreate \| fork \| hook` | requirements §5 |
| `ops_per_input` | `<int>` | 한 입력의 최대 연산 수 |
| `op_vocabulary` | csv `heap,btree,vacuum,checkpoint` | 어휘 1차 범위 |
| `invariant_checks` | `bool` | crash 없이도 위반 검출 |
| `time_budget_sec` | `<int>` | CI 시간 통제 |
| `corpus_root` | `<dir>` | seed + crash corpus |
| `sanitizers` | csv `asan,ubsan` | 활성 sanitizer |

ADR-EXT-009 후 키/의미 동결.

## 3. 출력 포맷 (TBD)

```
<resultDir>/
   ├── main.info
   ├── crash/
   │     └── <stack-hash>/
   │           ├── input.bin            # libFuzzer 원본 입력
   │           ├── sequence.txt         # 사람이 읽는 op 열 (핵심 산출물)
   │           ├── stack.txt
   │           ├── sanitizer.txt
   │           └── reproducer.sh
   ├── invariant/
   │     └── <check-id>/                # crash 없는 위반
   ├── coverage/coverage.json
   └── stats.json                       # execs / resets / unique crashes / coverage delta
```

`sequence.txt` 예 (형식 미확정):

```
INSERT  oid=? len=137
INSERT  oid=? len=4021          # overflow 승격 경계
UPDATE  oid=#1 len=8
SCAN    hfid=#0
VACUUM
COMMIT
```

## 4. 종료 코드 (TBD)

| 값 | 의미 |
|---|---|
| 0 | 시간 내 crash·invariant 위반 없음 |
| 1 | crash 발견 |
| 2 | invariant 위반 (crash 없음) |
| ≥3 | infra 오류 (target 미빌드 / reset 훅 부재 / 비결정적 재현 감지) |

## 5. NG2 / NG4 점검

신규 진입점 — 충돌 없음.

## 6. cubrid 본 repo 측 contract (선결)

| 항목 | testkit 의제 | cubrid 본 repo 책임 |
|---|---|---|
| build option | `-DENABLE_FUZZING=ON` | E5 와 **동일 옵션 공유** |
| fuzz target binary | `cubrid-fuzz-storage` | 본 repo build 산출물 |
| harness signature | `int LLVMFuzzerTestOneInput(const uint8_t*, size_t)` | 본 repo 정의 |
| custom mutator (LPM 채택 시) | `LLVMFuzzerCustomMutator` | 본 repo 정의. protobuf 링크는 fuzz 빌드 한정 |
| state reset 훅 | 매 입력 경계 초기화 | **본 항목 고유 요구** — E5 에는 없음 |
| in-process boot/shutdown | 임시 volume 부팅 | 본 repo 진입점 필요 |

C-055 cross-cutting 으로 양 측 contract 동시 동결 (엔진 쪽 = N66-fuzz-target-infrastructure).
