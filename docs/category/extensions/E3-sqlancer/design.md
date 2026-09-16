# E3 — Design (STUB)

**Status:** STUB — 정식 design 은 ADR-EXT-003 incubating 정식 진입 후.
**Source:** ROADMAP §6a-E3, `requirements.md`, `project/survey/dbms-testing-ecosystem.md` §5
**Companion:** `requirements.md` (FULL), `io-contract.md` (STUB), `test-corpus.md` (STUB)

---

## 1. 모듈 위치 (의제)

```
extensions/cubrid-sqlancer/   # 확정: 별도 저장소 (ADR-EXT-003)
   ├── oracle/
   │     ├── norec/      # WHERE p ↔ COUNT(*) WHERE (p IS TRUE) rowcount 비교
   │     ├── tlp/        # WHERE p ↔ p IS TRUE / IS FALSE / IS NULL 합집합
   │     └── pqs/        # pivot 보존 (2차 후속, 비용 ↑)
   ├── ast/              # E2 와 공유 가능 — 의제 (Open Question 3)
   ├── runner/           # SUT 구동 (JDBC | CCI)
   └── corpus/           # mismatch 보관
```

## 2. dialect adapter 위치 (의제)

```
internal/catalog/                # 후보 1 — testkit 내부 공통 레이어 (E2/E3 공유)
   ├── catalog.{rs,go,java} # CUBRID system catalog 추상화
   ├── grammar.*            # CUBRID dialect 가산 (path / serial / connect_by / method)
   └── adapter.*            # SQL emitter

# vs

internal/runner/sqlsmith/dialect/       # 후보 2 — 도구별 분산
extensions/cubrid-sqlancer/   # 확정: 별도 저장소 (ADR-EXT-003)dialect/
```

ADR-EXT-003 (Open Question 3) 에서 결정.

## 3. 데이터 흐름 (의제 — NoREC)

```
seed → ast.generate(predicate p, table t)
       ├─ Q1 = "SELECT * FROM t WHERE p"
       └─ Q2 = "SELECT COUNT(*) FROM (SELECT (p) IS TRUE p FROM t) WHERE p"
            └─ runner.execute(Q1).rowcount == runner.execute(Q2).scalar
                 └─ if mismatch: corpus.save({Q1, Q2, schema, seed})
```

## 4. 데이터 흐름 (의제 — TLP)

```
seed → ast.generate(predicate p, base Q)
       └─ Q_true   = base WHERE p IS TRUE
          Q_false  = base WHERE p IS FALSE
          Q_null   = base WHERE p IS NULL
          Q_total  = base
            └─ row_set(Q_total) == row_set(Q_true) ⊎ row_set(Q_false) ⊎ row_set(Q_null)
```

## 5. 외부 의존

- E2 와 동일한 schema introspect (가능하면 dialect adapter 공유)
- ADR-001 결정 언어 + JDBC/CCI
- SQLancer 본체 (재사용 시, MIT)

## 6. 결정 보류 항목 → ADR-EXT-003

- 1차 oracle 선정 (NoREC + TLP 권장)
- dialect adapter 위치 (E2 와 공유 vs 도구별)
- 재사용 (SQLancer Java) vs 재구현
- mismatch corpus 위치 (NG1 점검)

## 7. design 작성 트리거

ADR-EXT-003 합의 후 본 문서 FULL 로 보강.
