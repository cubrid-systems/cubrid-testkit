# E5 — Design (STUB)

**Status:** STUB — 정식 design 은 ADR-EXT-005 incubating 정식 진입 후. cubrid 본 repo `-DENABLE_FUZZING` 선결.
**Source:** ROADMAP §6a-E5, `requirements.md`, `survey/dbms-testing-ecosystem.md` §7
**Companion:** `requirements.md` (FULL), `io-contract.md` (STUB), `test-corpus.md` (STUB)

---

## 1. 책임 경계 (cross-repo)

```
cubrid 본 repo                          testkit (이 모듈)
─────────────                          ───────────────
fuzz target build option           →   corpus 보관
sanitizer 빌드 (ASan/UBSan/MSan)   →   replay (regression)
in-process harness 함수            →   crash triage (stack hash dedup)
                                       coverage 보고
```

testkit 단독으로는 시작 불가. *C-015 cross-cutting* (roadmap repo) 후보.

## 2. 모듈 위치 (의제)

```
impl/fuzz-harness/
   ├── runner/           # libFuzzer / AFL / honggfuzz 호출
   ├── corpus/           # seed + crash 보관
   ├── triage/           # stack hash dedup + sanitizer 분류
   └── coverage/         # build 별 coverage 비교
```

## 3. fuzz target layer (의제)

| Layer | target | 추정 ROI | cubrid 본 repo 작업량 |
|---|---|---|---|
| lexer | `lex_consume` 입구 | ★★ | 적음 |
| parser | `parser_main` 입구 | ★★★★ | 적음 |
| binder | catalog resolution | ★★★ | 중 |
| planner | optimizer 입구 | ★★★ | 중 |
| CCI protocol | binary message handler | ★★★★ | 중 (handler 분리 필요) |
| JDBC protocol | wire protocol parser | ★★★★ | 중 |

ADR-EXT-005 에서 1차 layer 선정 (parser 권장 — 비용 ↓, 가치 ↑).

## 4. 데이터 흐름 (의제)

```
seed corpus → fuzzer (libFuzzer) → fuzz target (cubrid in-process)
                                         ├─ ok        → coverage update
                                         └─ CRASH (signal | sanitizer)
                                              └─ triage.dedupe(stack)
                                                   └─ corpus.save({input, stack, sanitizer})
```

## 5. 외부 의존

- cubrid 본 repo 의 `-DENABLE_FUZZING` build option (선결)
- ASan / UBSan / MSan 빌드 산출물 (cubrid 본 repo 책임)
- libFuzzer / AFL / honggfuzz
- seed corpus (test-corpus.md 참조)

## 6. 결정 보류 항목 → ADR-EXT-005

- fuzz target layer 1차 선정
- fuzzer 본체 (libFuzzer in-process / AFL subprocess / honggfuzz)
- corpus 위치 (NG1 점검)
- C-015 책임 경계 ADR

## 7. design 작성 트리거

cubrid 본 repo PR + ADR-EXT-005 후. 현 시점 stub.
