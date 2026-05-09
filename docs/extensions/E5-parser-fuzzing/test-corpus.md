# E5 — Test Corpus (STUB)

**Status:** STUB — 코퍼스 정책은 ADR-EXT-005 incubating 정식 진입 후.
**Source:** `requirements.md` §5

---

## 1. 코퍼스 종류

| 코퍼스 | 입력/출력 | 용도 |
|---|---|---|
| seed corpus | 입력 | 의미 있는 SQL / protocol 메시지의 초기 seed (coverage 부트스트랩) |
| crash corpus | 출력 | fuzz run 의 crash 누적 — stack hash 기반 dedup |
| coverage corpus | 출력 | edge coverage 갱신 입력 (libFuzzer 가 자동 관리) |

## 2. seed corpus 출처 (의제)

| 후보 | 비용 | 비고 |
|---|---|---|
| 기존 sql 모듈 케이스 → byte 입력 변환 | 낮음 | 17,411 .sql 의 *부분집합* 을 fuzz seed 로 |
| sqllogictest (E1) 코퍼스 | 낮음 | external corpus 차용 |
| cubrid 자체 grammar 기반 합성 seed | 중 | hand-crafted edge case |
| CCI / JDBC capture replay | 중 | client 통신 capture (privacy 점검) |

ADR-EXT-005 에서 seed 정책 결정.

## 3. 보관 정책 (NG1 점검)

- ❌ testcases 레포에 crash corpus 두지 않음
- ✅ testkit 내부 별 트리 또는 외부 storage
- ✅ seed corpus 는 *생성* 가능 (sql 모듈 케이스 변환) — 보관 불필요 시 cache 처리

## 4. crash 항목 구조 (의제)

```
crash/<stack-hash>/
   ├── input.bin               # raw bytes (libFuzzer 형식)
   ├── input.repr              # human-readable 표현 (가능한 경우)
   ├── stack.txt
   ├── sanitizer.txt
   ├── target_layer            # parser / cci / jdbc
   └── reproducer.sh
```

## 5. 라이선스

- libFuzzer: Apache 2.0
- AFL: Apache 2.0
- honggfuzz: Apache 2.0
- 외부 seed corpus 사용 시 출처 license 점검

## 6. 후속 작성 트리거

cubrid 본 repo PR + ADR-EXT-005 후 본 문서 FULL 로 보강.
