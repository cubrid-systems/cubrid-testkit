# E6 — Design (STUB)

**Status:** STUB — 정식 design 은 ADR-EXT-006 incubating 정식 진입 후. N13 pg-wire-compat selected 이상 권장.
**Source:** ROADMAP §6a-E6, `requirements.md`, `project/survey/dbms-testing-ecosystem.md` §8
**Companion:** `requirements.md` (FULL), `io-contract.md` (STUB), `test-corpus.md` (STUB)

---

## 1. 모듈 위치 (의제)

```
internal/runner/differential/
   ├── input/            # case corpus loader (E1 / E2 / 직접 작성)
   ├── runner/
   │     ├── cubrid/     # JDBC | CCI | pg-wire (N13 후)
   │     └── peer/       # PostgreSQL JDBC
   ├── rewrite/          # dialect rewrite layer (DATE / NULL / float / collation / JSON / overflow)
   ├── compare/          # row 단위 비교 + sort 처리
   └── classify/         # mismatch 분류: real wrong-result / known dialect / float / collation
```

## 2. mode 분기 (의제)

```
canonical mode:
  case → run on both → compare assert identical
  적용 범위: SQL-92 core 부분집합

rewrite mode:
  case → rewrite_for_peer(case) → run peer
       → run cubrid raw → compare
  적용 범위: dialect rewrite catalog 가 cover 하는 영역
```

## 3. dialect rewrite catalog (의제)

| 카테고리 | 차이 예 | rewrite 패턴 |
|---|---|---|
| DATE 함수 | `DATE_ADD` ↔ `+ INTERVAL` | 함수 매핑 |
| NULL 정렬 | NULLS FIRST 디폴트 차이 | 명시 추가 |
| float 정밀도 | precision / rounding 차이 | tolerance compare |
| collation | 디폴트 collation 차이 | COLLATE 명시 |
| JSON 함수 | 함수명 차이 | 함수 매핑 |
| 정수 overflow | wrap vs error 차이 | 범위 제한 |

ADR-EXT-006 에서 1차 카테고리 선정.

## 4. 외부 의존

- N13 pg-wire-compat (selected 이상 권장)
- PostgreSQL 인스턴스 + JDBC driver
- 케이스 코퍼스 (E1 / E2 / 직접)
- dialect rewrite catalog (testkit 자체 자산)

## 5. 결정 보류 항목 → ADR-EXT-006

- mode 1차 (canonical vs rewrite)
- dialect rewrite catalog 1차 카테고리
- peer DBMS 1차 (PostgreSQL 만 / + MySQL / + SQLite)
- 케이스 코퍼스 input source
- mismatch corpus 위치 (NG1 점검)

## 6. design 작성 트리거

N13 selected 진입 + ADR-EXT-006 후. 현 시점 stub.
