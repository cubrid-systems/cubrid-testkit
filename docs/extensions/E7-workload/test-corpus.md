# E7 — Test Corpus (STUB)

**Status:** STUB — 코퍼스 정책은 ADR-EXT-007 incubating 정식 진입 후.
**Source:** `requirements.md` §5

---

## 1. 코퍼스 종류

| 코퍼스 | 입력/출력 | 용도 |
|---|---|---|
| invariant catalog | 입력 | 검증 invariant 정의 (testkit 자산) |
| seed corpus | 입력 | regression seed — 과거 violation 의 fault sequence |
| violation corpus | 출력 | invariant violation 누적 |
| (engine-suite handoff) | 입력 | benchbase / HammerDB 의 workload 정의 — *재사용* |

본 항목은 *입력 SQL 코퍼스 없음* — workload 가 random generation.

## 2. invariant catalog (testkit 자산)

```
catalog/
   ├── row_count_consistency.yaml
   ├── referential_integrity.yaml
   ├── sum_conservation.yaml
   ├── monotonicity.yaml
   └── no_phantom_after_failover.yaml
```

각 yaml: invariant 이름 / SQL 쿼리 / threshold / scope (per-table / per-tx) — ADR-EXT-007 후 schema 동결.

## 3. 보관 정책 (NG1 점검)

- ❌ testcases 레포에 두지 않음
- ✅ testkit 내부 (invariant catalog) + 외부 storage (violation corpus)
- ✅ E4 와 corpus 위치 정책 *공유 권장* (history 거대해질 가능성 — GC 정책 명시 필요)

## 4. violation 항목 구조 (의제)

```
violations/<invariant>/<witness-hash>/
   ├── seed
   ├── topology.json          # cluster config
   ├── workload_mix.json
   ├── fault_seq.json
   ├── state_dump.txt         # violation 시점의 DB 상태 일부
   ├── invariant_report.txt   # checker 출력
   └── reproducer.sh
```

## 5. engine-suite 자산 재사용 (C-004 결합)

| engine-suite 자산 | 재사용 가능성 | 비고 |
|---|---|---|
| benchbase TPCC schema | 가능 | sum_conservation invariant 와 정합 |
| HammerDB schema/workload | 가능 | randomized DDL/DML 와 결합 |
| 자체 생성 KV workload | 별도 | invariant 디버깅에 적합 |

ADR-EXT-007 에서 재사용 범위 결정.

## 6. 라이선스

- benchbase: Apache 2.0 (engine-suite 측 결정 사항)
- HammerDB: GPLv3 (engine-suite 측 — testkit 가 동일 license 의무 받지 않도록 *프로세스 경계* 권장)
- invariant catalog: testkit 자산

## 7. 후속 작성 트리거

C-004 결론 + ADR-EXT-007 후 본 문서 FULL 로 보강.
