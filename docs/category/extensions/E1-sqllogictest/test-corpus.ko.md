# E1 — Test Corpus (STUB)

*[English](test-corpus.md) · 한국어*

**Status:** STUB — 코퍼스 정책은 ADR-EXT-001 incubating 정식 진입 후.
**Source:** `requirements.md` §5, ROADMAP §8 risk 6

---

## 1. 입력 코퍼스 출처 (의제)

| 후보 | 규모 | 라이선스 | 비고 |
|---|---|---|---|
| SQLite sqllogictest 원형 | 5.8M rows | Public Domain | 원형 — baseline 우선순위 |
| DuckDB test suite (`.slt` 부분) | 다수 | MIT | DuckDB 내부 추정 결합 case 다수 |
| CockroachDB logictest | 다수 | Apache 2.0 | 분산 / SERIAL 의존 case 다수 |
| RisingWave logictest | 다수 | Apache 2.0 | streaming SQL — CUBRID 적용성 낮음 |

ADR-EXT-001 의 *spec target* 결정에 종속.

## 2. 라이선스 점검 (필수)

- SQLite Public Domain — vendoring 자유
- DuckDB MIT — vendoring 자유 (NOTICE 포함)
- CockroachDB Apache 2.0 — vendoring 자유 (NOTICE 포함)

각 코퍼스의 *부분집합* 만 vendor in 하는 경우에도 출처/라이선스 표기 의무.

## 3. 보관 정책 (NG1 점검)

testcases 레포 (cubrid-testcases / -private / -private-ex) 는 *동결 대상*. 본 코퍼스는:

- ❌ testcases 레포 안에 두지 않음 (NG1 위반)
- ✅ testkit 내부 별 트리 (예: `corpus/sqllogictest/`) 또는 외부 storage
- ✅ full mirror 대신 *의미 있는 부분집합 vendor in* (ROADMAP §8 risk 6)
- ✅ 외부 트리 자동 동기화 시 sync 스크립트로 운영

## 4. dialect-skip 정책 (의제)

CUBRID 가 지원하지 않는 SQL 기능 (예: PostgreSQL extension 의존, SQLite specific) 의 case 는 *skip* 분류. 동결 형식: ADR-EXT-001 후.

## 5. 후속 작성 트리거

ADR-EXT-001 의 *코퍼스 정책* 결정 후 본 문서 FULL 로 보강.
