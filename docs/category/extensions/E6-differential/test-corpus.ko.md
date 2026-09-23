# E6 — Test Corpus (STUB)

*[English](test-corpus.md) · 한국어*

**Status:** STUB — 코퍼스 정책은 ADR-EXT-006 incubating 정식 진입 후.
**Source:** `requirements.md` §5

---

## 1. 코퍼스 종류

| 코퍼스 | 입력/출력 | 용도 |
|---|---|---|
| input case corpus | 입력 | 비교 대상 SQL — E1 (.slt) / E2 (generated) / 직접 작성 |
| dialect rewrite catalog | 입력 | dialect 차이 → rewrite 패턴 매핑 |
| seed corpus | 입력 | regression seed — 과거 *real wrong-result* 의 query |
| mismatch corpus | 출력 | mismatch 분류별 누적 |

## 2. input case 출처 (의제)

- **E1 (.slt) 차용** — sqllogictest 가 cross-DB 적용을 가정 — 직접 활용 가능
- **E2 (sqlsmith generated)** — random valid SQL — dialect skip 비율 ↑
- **canonical subset 직접 작성** — SQL-92 core 의 *PostgreSQL 와 동일* SQL — 수가 적음

ADR-EXT-006 에서 1차 input source 결정.

## 3. dialect rewrite catalog 보관 (의제)

```
catalog/
   ├── date.yaml          # DATE 함수 매핑
   ├── null.yaml          # NULL 정렬 / IS NULL 처리
   ├── float.yaml         # 부동소수점 tolerance / rounding
   ├── collation.yaml     # 디폴트 collation 차이
   ├── json.yaml          # JSON 함수 매핑
   └── overflow.yaml      # 정수 overflow wrap vs error
```

testkit 자체 자산. 외부 license 의무 없음.

## 4. 보관 정책 (NG1 점검)

- ❌ testcases 레포에 두지 않음
- ✅ testkit 내부 (catalog) + 외부 storage (mismatch corpus)
- ✅ E1 / E2 의 corpus 위치 정책과 일관

## 5. mismatch 분류 (의제)

| classification | 의미 | 처리 |
|---|---|---|
| `real_wrong_result` | dialect 외 차이 | 종료 코드 1 — 진짜 버그 |
| `dialect_known` | catalog 매칭됨 | warning |
| `float_tolerance` | tolerance 초과 | 분류 후 사람이 결정 |
| `collation_known` | catalog 매칭됨 | warning |
| `unknown` | 분류 불가 | inspection 필요 |

ADR-EXT-006 에서 분류 정책 동결.

## 6. 라이선스

- PostgreSQL JDBC: BSD-2-Clause
- 입력 corpus 가 외부 (E1) 인 경우 — E1 의 라이선스 정책 상속
- dialect catalog: testkit 자산

## 7. 후속 작성 트리거

N13 selected + ADR-EXT-006 후 본 문서 FULL 로 보강.
