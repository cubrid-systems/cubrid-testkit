# E2 — Design (STUB)

**Status:** STUB — 정식 design 은 ADR-EXT-002 incubating 정식 진입 후.
**Source:** ROADMAP §6a-E2, `requirements.md`, `survey/dbms-testing-ecosystem.md` §4
**Companion:** `requirements.md` (FULL), `io-contract.md` (STUB), `test-corpus.md` (STUB)

---

## 1. 모듈 위치 (의제)

```
internal/runner/sqlsmith/
   ├── introspect/        # CUBRID system catalog → schema model (db_class / db_attribute / db_serial / db_method)
   ├── ast/               # type-correct random AST generator (CUBRID dialect 가산 옵션)
   ├── runner/            # SUT 구동 (JDBC | CCI) + crash detection (signal / core / server log)
   ├── triage/            # stack hash dedup
   └── corpus/            # crash query 보관 (재현 seed + 정규화 query text)
```

E3 (SQLancer) 와 *공통 dialect 레이어* 가능 — Open Question 3 (dialect adapter 위치) 의 결정에 종속.

## 2. 데이터 흐름 (의제)

```
seed → AST.generate(depth)
       └─ runner.execute(query)
            ├─ ok / empty / SQL error  → drop
            └─ CRASH (signal | core | conn drop)
                 └─ triage.dedupe(stack hash)
                      └─ corpus.save({seed, query, stack})
```

## 3. crash 판정 채널 (의제)

| 채널 | 검출 가능 | 비고 |
|---|---|---|
| process signal | SIGSEGV / SIGABRT / SIGFPE | in-process driver 시 적합 |
| core file | server-side crash | sql / isolation 모듈 core 정책과 공유 (analysis/sql §4) |
| connection drop | server hang / restart | timeout 결합 필요 |
| server log assert | logical assertion failure | log scraping |

ADR-EXT-002 에서 단일 채널 또는 우선순위 결정.

## 4. 외부 의존

- CUBRID system catalog (db_class / db_attribute / db_serial / db_method)
- ADR-001 결정 언어 + JDBC/CCI 클라이언트
- SQLsmith 본체 (재사용 시) 또는 자체 random AST 라이브러리
- core dump 인프라 (analysis/sql §4 ADR 와 공유)

## 5. 결정 보류 항목 → ADR-EXT-002

- 재사용 (SQLsmith C++ subprocess) vs 재구현
- CUBRID dialect 가산 범위 (path expression / serial / connect-by / method)
- crash 판정 채널 (signal / core / server log / 통합)
- corpus 위치 (NG1 점검)

## 6. design 작성 트리거

ADR-EXT-002 합의 후 본 문서 FULL 로 보강.
