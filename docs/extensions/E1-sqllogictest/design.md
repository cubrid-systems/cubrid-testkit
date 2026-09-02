# E1 — Design (STUB)

**Status:** STUB — 정식 design 은 ADR-EXT-001 incubating 정식 진입 후.
**Source:** ROADMAP §6a-E1, `requirements.md`, `survey/dbms-testing-ecosystem.md` §3
**Companion:** `requirements.md` (FULL), `io-contract.md` (STUB), `test-corpus.md` (STUB)

---

## 1. 모듈 위치 (의제)

```
internal/runner/sqllogictest/        # 신 모듈 — case-format ingestion 인터페이스 통해 testkit 골격에 결합
   ├── parser/            # .slt record 파서 (statement / query)
   ├── runner/            # SUT 구동 (JDBC | CCI | cubrid-cli)
   ├── compare/           # hash 비교 + values 비교 + sort 옵션 처리
   └── report/            # pass / fail / hash mismatch / dialect-skip 분류
```

ADR-001 (구현 언어) 결정에 따라 sqllogictest-rs 를 직접 의존할지(`Rust`) / 자체 구현할지 결정됨.

## 2. 데이터 흐름 (의제)

```
.slt file
  └─ parser
       └─ record stream { Statement(ok|error) | Query(type, sort, [hash|values]) }
            └─ runner.execute(record)
                 └─ compare.diff(actual, expected)
                      └─ report.classify
```

## 3. 외부 의존

- ADR-001 결정 언어 + JDBC/CCI 클라이언트
- 외부 sqllogictest 코퍼스 (test-corpus.md 참조)
- Phase 2 `design/contracts.md` 의 case-format ingestion 인터페이스

## 4. 결정 보류 항목 → ADR-EXT-001

- spec variant (SQLite / DuckDB / CockroachDB)
- 결과 비교 모드 (hash vs values vs CUBRID expected 추가)
- SUT 구동 클라이언트 (JDBC / CCI / cubrid-cli)
- sqllogictest-rs 재사용 vs 자체 구현

## 5. design 작성 트리거

ADR-EXT-001 합의 후 본 문서를 *FULL* 로 보강. 현 시점에는 의제 보관소.
