# DBMS Testing Ecosystem Survey

**Date:** 2026-05-08
**Status:** 외부 조사 (ROADMAP §6a 확장 영역 카탈로그 결정 근거)
**Audience:** hgryoo, testkit 외부 contributor
**Trigger:** roadmap repo의 testkit foundation 검토 중 *DBMS testing 8축 에코시스템* 정리 요청 (2026-05-08)

---

## Table of Contents

- [1. 목적](#1-목적)
- [2. 분류 — 8축](#2-분류--8축)
- [3. 축 1 — sqllogictest 계열 (정답 회귀)](#3-축-1--sqllogictest-계열-정답-회귀)
  - [3.1 sqllogictest (SQLite 원형)](#31-sqllogictest-sqlite-원형)
  - [3.2 sqllogictest-rs](#32-sqllogictest-rs)
  - [3.3 PostgreSQL `src/test/regress` / `pg_regress`](#33-postgresql-srctestregress--pg_regress)
  - [3.4 MySQL MTR — 참고](#34-mysql-mtr-mysql-test-runpl--참고)
  - [3.5 DuckDB test suite](#35-duckdb-test-suite)
  - [3.6 축 1 비교 + CUBRID 적용성](#36-축-1-비교--cubrid-적용성)
- [4. 축 2 — Random SQL generation](#4-축-2--random-sql-generation)
  - [4.1 SQLsmith](#41-sqlsmith)
  - [4.2 SQLancer (random generation 부분만)](#42-sqlancer-random-generation-부분만)
  - [4.3 SQLsmith vs SQLancer](#43-sqlsmith-vs-sqlancer-동일-random-부분)
  - [4.4 축 2 CUBRID 적용성](#44-축-2-cubrid-적용성)
- [5. 축 3 — Logic-bug / semantic testing](#5-축-3--logic-bug--semantic-testing)
  - [5.1 SQLancer (Manuel Rigger 외)](#51-sqlancer-manuel-rigger-외)
  - [5.2 SQLaser (clause-guided)](#52-sqlaser-clause-guided-fuzzing-최근-연구)
  - [5.3 SQLancer++ (adaptive grammar)](#53-sqlancer-adaptive-grammar-learning)
  - [5.4 축 3 CUBRID 적용성](#54-축-3-cubrid-적용성)
- [6. 축 4 — Isolation / transaction testing](#6-축-4--isolation--transaction-testing)
  - [6.1 PostgreSQL isolation tester](#61-postgresql-isolation-tester-srctestisolation)
  - [6.2 AWDIT](#62-awdit-anomaly-aware-diagnostic-isolation-testing--최근-연구)
  - [6.3 Jepsen](#63-jepsen)
  - [6.4 Hermitage / Elle](#64-hermitage--elle-참고)
  - [6.5 축 4 CUBRID 적용성](#65-축-4-cubrid-적용성)
- [7. 축 5 — Parser / compiler fuzzing](#7-축-5--parser--compiler-fuzzing)
- [8. 축 6 — Differential testing](#8-축-6--differential-testing)
- [9. 축 7 — Stateful / workload testing](#9-축-7--stateful--workload-testing)
  - [9.1 CockroachDB roachtest](#91-cockroachdb-roachtest-randomized-testing)
  - [9.2 FoundationDB simulation testing](#92-foundationdb-simulation-testing)
  - [9.3 CUBRID 적용성](#93-cubrid-적용성)
- [10. 축 8 — Hybrid (Materialize 패턴)](#10-축-8--hybrid-materialize-패턴)
- [11. CUBRID 적용성 종합 — §6a 카탈로그 확장 후보](#11-cubrid-적용성-종합--6a-카탈로그-확장-후보)
- [12. Open Questions](#12-open-questions-정식-incubating-진입-시-결정-owner-hgryoo)
- [13. References](#13-references)
- [14. 변경 이력](#14-변경-이력)

---

## 1. 목적

ROADMAP §6a "확장 영역(Beyond Strangler-fig)"의 카탈로그는 현재 **E1 = sqllogictest** 한 항목만 등록되어 있다. 본 survey는 DBMS testing 에코시스템을 **8축**으로 정리하고, 각 축의 대표 도구·연구를 비교한 뒤, **CUBRID 적용성**을 점수화하여 §6a 카탈로그 확장 후보(E2~En)를 식별한다.

본 문서는 *결정* 이 아닌 *근거(evidence base)*. 각 후보의 incubating 정식 진입은 분기 게이트(ROADMAP §7)와 충돌하지 않는 시점에서 별도 ADR-EXT-NNN로 결정한다.

**비-목표 (Non-Goal):**
- 본 survey가 직접 §6a 항목을 *추가*하지 않는다. ROADMAP.md §6a 갱신은 별도 ADR-EXT-NNN의 결정 결과.
- 도구별 통합 PoC는 본 survey의 범위 밖. PoC는 incubating 진입 후 구현 산출물.
- 비-CUBRID DBMS 호환 (ROADMAP NG4)은 건드리지 않음 — 본 survey의 모든 항목은 *CUBRID를 SUT로* 검증하는 도구만 다룬다.

---

## 2. 분류 — 8축

DBMS testing 에코시스템은 *목적·생성 방식·판정 기준 종류*가 직교한다. 8축으로 분류:

> **용어 메모.** 본 문서의 "판정 기준 / 판정 기법"은 software-testing 문헌의 *test oracle* (결과의 정답 여부를 결정하는 메커니즘) 을 가리킨다. Oracle Corporation 의 DBMS 와는 무관.

| # | 축 | 한 줄 정의 | 대표 판정 기준 |
|---|----|---|---|
| 1 | **sqllogictest 계열** | 결정적 input → 사전 기록된 expected output diff | expected file |
| 2 | **Random SQL generation** | 스키마 introspect 후 valid AST 무작위 생성 | crash / assert / segfault |
| 3 | **Logic-bug / semantic testing** | 의미 등가 query 쌍을 비교해 *결과가 다른* 버그 검출 | TLP / NoREC / PQS 판정 기법 |
| 4 | **Isolation / transaction testing** | 트랜잭션 interleaving을 systematic하게 강제 후 anomaly 탐지 | anomaly 카탈로그·history graph |
| 5 | **Parser / compiler fuzzing** | byte-level 또는 grammar-guided fuzzing으로 frontend 강건성 검증. *engine-internal structured 변종* 포함 (§7.4) | crash / UB |
| 6 | **Differential testing** | 같은 입력을 여러 DBMS에 돌려 결과 비교 | 외부 DBMS = 판정 기준 |
| 7 | **Stateful / workload testing** | schema mutation·node 재시작·partition을 randomize한 long-running 시나리오 | invariant violation·linearizability |
| 8 | **Hybrid (modern composition)** | 위 축 다수를 같은 CI 안에 결합 | 축별 판정 기준 합집합 |

각 축은 *서로 다른 종류의 버그*를 잡는다. 축 1만으로는 잘못된 결과(wrong-result)를 거의 못 잡고, 축 3만으로는 parser crash를 거의 못 잡는다. *조합* 이 모범 사례이며, §10에서 Materialize의 hybrid 패턴을 정리한다.

---

## 3. 축 1 — sqllogictest 계열 (정답 회귀)

### 3.1 sqllogictest (SQLite 원형)

- 출처: <https://www.sqlite.org/sqllogictest/>
- 철학: *deterministic input · expected-result-based · cross-DB validation 가능*
- 포맷:
  - 한 파일에 여러 record. record = `statement`(DDL/DML) 또는 `query` (SELECT + 결과 hash 또는 raw values)
  - hash 비교로 결과 표현이 짧음 → cross-DBMS 회귀에 적합
- 강점: SQLite·DuckDB·CockroachDB·RisingWave 등 다수가 채택 → *코퍼스 자체가 자산*
- 약점: 비결정적 결과(ORDER BY 없는 SELECT, FLOAT 정밀도 등) 표준화 어려움
- CUBRID 적용성: ★★★★ (E1로 이미 ROADMAP §6a 등록됨)

### 3.2 sqllogictest-rs

- 출처: <https://github.com/risinglightdb/sqllogictest-rs>
- Rust 기반 modern reimplementation. **library/runner 분리**.
- 채택: RisingLight, RisingWave, 일부 DuckDB 계열, 다수 modern Rust DBMS
- 강점:
  - async runner — 동시성 시나리오 (`statement async`)
  - hook/extension 풍부 — 커스텀 데이터타입·정렬·검증
  - sqllogictest spec 호환 + 확장 (label·hash·sleep·system shell escape)
- 약점: Rust dependency. testkit ADR-001(구현 언어) 미정인 단계에서는 *언어 선택을 강제*하는 도입은 위험
- CUBRID 적용성: ★★★ — testkit ADR-001이 Rust일 경우 ★★★★, JVM이면 ★★. **ADR-001 결정과 결합되는 후속 변수**

### 3.3 PostgreSQL `src/test/regress` / `pg_regress`

- 출처: <https://github.com/postgres/postgres/tree/master/src/test/regress>
- *PostgreSQL dialect* 중심. `.sql` ↔ `.out` expected diff
- 강점:
  - extension/contrib 모듈 테스트에 사실상 표준
  - `psql` 직결 — 환경 의존 풍부
- 약점:
  - infra-heavy: temp instance · port · locale 환경 의존
  - PostgreSQL 전용 기능 (regex notation, server-side fn) 가정 다수
- CUBRID 적용성: ★ — *직접 채택*은 부적합. CTP의 sql 모듈이 이미 동등한 *`.sql` ↔ expected diff* 패턴이라 학습 가치 정도

### 3.4 MySQL MTR (`mysql-test-run.pl`) — 참고

- Perl 러너 + `.test`/`.result`. multi-version matrix 강함
- CTP의 `medium`·`sql` 모듈과 가장 가까운 모델. 비교용 reference.
- CUBRID 적용성: ★ — Perl + 자체 protocol 가정. CTP가 이미 동일 패턴이라 신규 도입 가치 낮음

### 3.5 DuckDB test suite

- 출처: <https://github.com/duckdb/duckdb/tree/main/test>
- 한 suite 안에 다음이 *섞여 있음*:
  - SQL logic (sqllogictest 변종)
  - vectorized execution (chunk 경계 verification)
  - storage format (page format invariant)
  - parallel execution (race 검증)
  - optimizer verification (rewrite 등가성)
- 최근 DBMS testing 연구에서 *baseline corpus*로 자주 인용됨
- 강점: 단일 spec 안에 다축 결합 — *축 1 + 일부 축 3·축 7* 효과
- 약점: DuckDB 내부 추정(vectorized chunk size 등)에 결합된 케이스 다수
- CUBRID 적용성: ★★ — *코퍼스 자체*는 가져오기 어렵지만, *컨벤션* (한 파일에 logic + invariant 섞기)은 testkit 자체 코퍼스 작성 시 reference

### 3.6 축 1 비교 + CUBRID 적용성

| 도구 | format | runner | cross-DB | CUBRID 적용성 | testkit 매칭 |
|---|---|---|---|---|---|
| sqllogictest | hash + values | C/Tcl | ✅ | ★★★★ | E1 (등록됨) |
| sqllogictest-rs | spec + 확장 | Rust async | ✅ | ★★★ (ADR-001 종속) | E1 변종 |
| pg_regress | `.sql`/`.out` | C + psql | ❌ | ★ | sql 모듈이 동급 |
| MySQL MTR | `.test`/`.result` | Perl | ❌ | ★ | medium 모듈이 동급 |
| DuckDB suite | spec mix | C++ | ❌ | ★★ (참고) | corpus 컨벤션 reference |

**결론:** E1 (sqllogictest) 유지 + sqllogictest-rs는 ADR-001 결정 후 *implementation choice*로 검토. pg_regress / MTR / DuckDB suite는 *직접 채택*이 아니라 *코퍼스 컨벤션 reference*.

---

## 4. 축 2 — Random SQL generation

### 4.1 SQLsmith

- 출처: <https://github.com/anse1/sqlsmith>
- 철학: *Valid but insane SQL*. PostgreSQL 발 (Christian Oest 등). PostgreSQL 발견 버그 100+ 건의 출처.
- 동작:
  1. 대상 DB schema introspect (`information_schema`)
  2. random AST 생성 — type-correct
  3. *valid* SQL만 생성 (parser 통과)
  4. 임의 깊이의 nested expression / window / lateral / subquery 조합
- 잘 잡는 버그:
  - parser crash, planner assert
  - executor segfault, stack overflow, OOM
  - invalid internal state (catalog leak)
- 약점: 판정 기준 거의 없음 — *crash 중심*. 잘못된 결과(wrong-result)는 못 잡음
- CUBRID 적용성: ★★★★ — schema introspect 부분만 CUBRID dialect로 포팅하면 즉시 가동. 기대 ROI 매우 높음

### 4.2 SQLancer (random generation 부분만)

- SQLancer (§5에서 본격 다룸)는 판정 기준을 빼고 보면 *축 2 기능도 포함*. 같은 random AST 생성기 위에 판정 기법 여럿을 얹은 구조.
- SQLancer를 도입하면 SQLsmith의 가치 일부가 흡수됨. **단, SQLsmith의 "최대 deep nesting + extreme adversarial AST" 패턴은 별개의 자산** — SQLancer는 판정 효율을 위해 nesting을 제한하는 경향.

### 4.3 SQLsmith vs SQLancer (동일 random 부분)

| 차원 | SQLsmith | SQLancer |
|---|---|---|
| 주 목적 | crash 발견 | wrong-result 발견 |
| 판정 기준 | 없음 (signal·process 만) | TLP / NoREC / PQS |
| 생성 깊이 | 매우 깊음 | 중간 (판정 비용 고려) |
| 포팅 비용 | schema introspect만 교체 | schema + dialect + 판정 adapter |
| 결합 효과 | parser/planner 강건성 | optimizer/executor 정확성 |

### 4.4 축 2 CUBRID 적용성

**결론:** SQLsmith를 §6a-**E2 (random SQL fuzzing)** 후보로 등록 추천. 도입 비용 낮고(schema introspect 경로만 CUBRID화), parser·planner·executor 강건성에서 *지금 testkit이 비어 있는 영역*을 직접 채움. SQLancer(§5)와 *상보적* — 두 항목을 함께 등록하는 것이 모범사례.

---

## 5. 축 3 — Logic-bug / semantic testing

### 5.1 SQLancer (Manuel Rigger 외)

- 출처: <https://github.com/sqlancer/sqlancer> · 논문: PLDI'20 NoREC, OOPSLA'20 TLP, ICSE'21 PQS 등
- 현재 분야 *왕급* — PostgreSQL·MySQL·SQLite·CockroachDB·DuckDB·MariaDB·TiDB 등 메이저 DBMS에서 매년 100+ 버그 발견
- 4가지 판정 기법 제공:

#### 5.1.1 NoREC (Non-optimizing Reference Engine Construction)
- optimizer rewrite bug 검증
- 비교 쌍:
  ```
  Q1: SELECT * FROM t WHERE complex_predicate;
  Q2: SELECT COUNT(*) FROM (SELECT (complex_predicate) IS TRUE AS p FROM t) WHERE p;
  ```
  Q1.rowcount == Q2.scalar 이어야 함. optimizer가 Q1만 잘못 rewrite하면 발견됨.
- *CUBRID에 가장 즉시 적용 가능한 판정 기법* — 내부 plan을 알 필요 없이 SQL surface만 사용

#### 5.1.2 TLP (Ternary Logic Partitioning)
- WHERE p ↔ WHERE p IS TRUE / IS FALSE / IS NULL 의 결과 합집합 = 전체
- 3-valued logic 처리 버그 (특히 NULL/UNKNOWN)에 강함

#### 5.1.3 PQS (Pivoted Query Synthesis)
- 임의 row 하나 (pivot) 를 *반드시 포함하는* query를 생성
- generated query를 실행해 pivot이 결과에 없으면 → 버그
- 매우 영리하지만 판정 기법 구축 비용이 가장 높음

#### 5.1.4 Differential 판정 기법 (축 6과 중첩)
- 등가 query 쌍 또는 외부 DBMS와의 결과 비교
- 본 survey 축 6에서 별도 정리

### 5.2 SQLaser (clause-guided fuzzing, 최근 연구)

- 핵심: *bug-prone clause combination targeting*
- 위험 조합 (예: `GROUP BY + HAVING + DISTINCT + WINDOW`) 을 의도적으로 조합 — 실측 기반으로 *과거 버그가 모인 조합*에 가중치
- 효과: 동일 시간 내 발견 버그 수가 SQLancer 대비 의미 있게 증가했다는 보고
- CUBRID 적용성: SQLancer를 먼저 도입한 뒤, *CUBRID 자체 버그 history*로 weight를 학습시키면 ★★★. SQLancer 도입 없이 단독 도입은 비효율.

### 5.3 SQLancer++ (adaptive grammar learning)

- 핵심: *새 DBMS에도 빠르게 porting* 가능하도록 grammar adaptation
- 새 dialect를 manual로 명세하지 않고, syntax error 응답을 학습해 grammar를 adapt
- **CUBRID 같은 niche DBMS에 가장 의미 있는 방향성** — manual dialect 명세 비용을 절감
- 약점: 기법이 여전히 연구 단계. 안정성·재현성은 SQLancer mainline 대비 낮을 가능성

### 5.4 축 3 CUBRID 적용성

| 도구·판정 기법 | 도입 비용 | 즉시 ROI | 권장 단계 |
|---|---|---|---|
| SQLancer NoREC | 낮음 (SQL surface만) | ★★★★ | E3 1차 진입 |
| SQLancer TLP | 낮음 | ★★★ | E3 1차 진입 |
| SQLancer PQS | 중간 (pivot 판정 기법 구축) | ★★ | E3 2차 |
| SQLaser | 중간 (SQLancer 종속 + history) | ★★ | E3 후속 |
| SQLancer++ | 높음 (연구 도구) | ★★★ (장기) | E3 별도 |

**결론:** SQLancer (NoREC + TLP) 를 §6a-**E3 (logic bug detection)** 후보로 등록 추천. PQS·SQLaser·SQLancer++는 E3 1차 도입 후 별도 ADR로 확장. SQLsmith(E2) + SQLancer(E3) 결합은 PostgreSQL ecosystem의 *de facto* 표준 — testkit이 흉내낼 가치가 큼.

---

## 6. 축 4 — Isolation / transaction testing

### 6.1 PostgreSQL isolation tester (`src/test/isolation`)

- 출처: <https://github.com/postgres/postgres/tree/master/src/test/isolation>
- *legendary tool* — DBMS kernel 개발에서 사실상 표준
- 동작:
  - `.spec` 파일에 multi-session 트랜잭션 step 정의
  - permutation 자동 또는 명시
  - expected schedule output 과 diff
- 검증 대상:
  - serialization anomaly (write-skew, read-only anomaly 등)
  - deadlock, snapshot visibility
  - SSI 검증
- CUBRID 적용성: ★★★★ — CTP `isolation` 모듈이 이미 *유사한 schedule-based testing* 을 한다 (`.ctl` 문법, isolation/ctl-grammar.md 참조). PostgreSQL `.spec` 포맷 흡수 가능성은 isolation 모듈 strangler-fig 1차 대체 시(ROADMAP Phase 3 ADR-004 후보) 검토

### 6.2 AWDIT (Anomaly-aWare Diagnostic Isolation Testing — 최근 연구)

- 핵심:
  - *weak isolation anomaly* 자동 탐지
  - *huge history scalability* — 장기 운영 분산 DB의 거대한 history graph 처리
- 배경: Adya/Bailis taxonomy를 history graph 기반으로 검출하는 흐름 (Hermitage·Elle 계보 연장선)
- CUBRID 적용성: ★★★ — CUBRID의 *분산 / HA* 영역 검증에 의미 있음. 단, 단일 노드 RR/RC 안정성 우선이라면 후순위

### 6.3 Jepsen

- 출처: <https://jepsen.io/>
- 분산 DB의 network partition · clock skew · process kill 검증
- DBMS testing에서 거의 *독립 분야* — Cassandra·CockroachDB·Yugabyte·TiDB 등에서 표준
- 검증: linearizability·causal·snapshot isolation 등 외부 spec 위반
- CUBRID 적용성: ★★ (현재) ~ ★★★★ (HA·CDC·streaming-replication 진척에 비례). 본 repo의 N24 streaming-replication / N11 logical-replication-extension 진척과 강하게 결합

### 6.4 Hermitage / Elle (참고)

- Hermitage: anomaly *카탈로그* — 각 isolation level이 어느 anomaly를 허용/금지하는지 표 + 재현 SQL
- Elle: history graph cycle detection. Jepsen과 결합되어 사용
- 직접 도구 도입보다 *CTP isolation 모듈의 case 작성 가이드*로 활용

### 6.5 축 4 CUBRID 적용성

| 도구 | 단일 노드 | 분산/HA | 도입 비용 | 권장 |
|---|---|---|---|---|
| PostgreSQL isolation tester (포맷·아이디어) | ★★★★ | – | 낮음 | testkit `isolation` 모듈 internal 흡수 (§6a 외) |
| AWDIT | – | ★★★ | 중 | E4 후보 (HA 진척 연동) |
| Jepsen | – | ★★~★★★★ | 높음 (Clojure + 인프라) | E4 또는 별도 — N24/N11 graduation 후 |
| Hermitage / Elle | ★★★ | ★★★ | 낮음 (지식 자산) | isolation 모듈 case 가이드 |

**결론:** 축 4는 *§6a 외부 + §6a 내부*가 갈린다.
- PostgreSQL `.spec` 포맷 + Hermitage 카탈로그는 **strangler-fig isolation 모듈 자체에 흡수** (Phase 3 ADR-004 검토 시 함께 결정).
- AWDIT / Jepsen은 §6a-**E4 (distributed isolation testing)** 후보. *분산 axis가 충분히 익은 후* — N24 streaming-replication / N11 logical-replication-extension graduation에 맞춰 trigger.

---

## 7. 축 5 — Parser / compiler fuzzing

### 7.1 AFL / libFuzzer 적용 layer

- byte-level coverage-guided fuzzer. SQL parser 직접 입력에 적용 가능.
- DBMS frontend 단계별 layer:
  ```
  SQL text
   -> lexer
   -> parser
   -> binder / catalog resolution
   -> planner
   -> executor (binary protocol level)
  ```
  각 단계 모두 fuzzing target 가능.
- 잘 잡는 버그:
  - parser crash / UB
  - heap overflow / stack corruption
  - infinite recursion
  - binary protocol parser corruption (CUBRID의 CCI/JDBC 프로토콜 진입점)

### 7.2 grammar-guided 변종

- libFuzzer + grammar (e.g. `protobuf-mutator` 류) — random SQL을 grammar 안에서 mutate
- SQLsmith는 grammar-aware random generation, libFuzzer는 byte-level mutation. *상보적* — SQLsmith가 못 만드는 *문법 외* SQL을 libFuzzer가 만든다 (오히려 그래서 parser crash가 잘 잡힘).

### 7.3 축 5 CUBRID 적용성

- *엔진 사이드* 결합도가 높음. testkit이 fuzzer harness만 제공하고, fuzz target을 만드는 것은 *cubrid 본 레포 빌드 시스템* 책임.
- 본 repo의 cross-cutting C-004 (testkit × engine-suite 경계) 와 동일 분류 — *어디서 돌릴지* 자체가 결정 사항.
- CUBRID 적용성: ★★★ (가치 큼) · ★★ (testkit 단독 책임으로 두면 부정합 위험)
- **결론:** §6a-**E5 (parser/protocol fuzzing harness)** 후보로 등록하되, *cubrid 본 레포에 fuzz target 빌드 옵션* 추가가 전제. testkit은 corpus 보관 + replay + crash triage 만 책임.

---

### 7.4 축 5 확장 — engine-internal structured fuzzing (2026-09-03 추가)

§7.1~7.3 은 *frontend byte 진입점* 을 다룬다. 같은 fuzzer(libFuzzer)를 쓰지만 **대상과
입력 형태가 다른** 갈래가 하나 더 있다 — storage engine 내부 API 를 *구조화된 연산 열* 로
때리는 방식.

#### RocksDB `fuzz/` 패턴 (참조 구현)

- RocksDB 는 fuzz 코드를 저장소 안에 `fuzz/` 로 유지하고, README 에서 **LLVM libFuzzer** 를
  fuzz engine 으로 명시한다. structured fuzzing 에는 **protobuf + libprotobuf-mutator** 를 쓴다.
- 입력이 random byte 가 아니라 *연산 메시지* 다:

  ```protobuf
  message DBOperation {
    enum Type { PUT = 0; GET = 1; DELETE = 2; }
    Type type = 1;
    bytes key = 2;
    bytes value = 3;
  }
  ```

  libFuzzer 가 raw byte 를 만들면 libprotobuf-mutator 가 이를 *구조를 지키며* mutate 하고
  (`oneof` 전환 / `repeated` 삽입·삭제 / field 교체), harness 가 `db->Put()` / `db->Get()` 을
  **직접 호출** 한다. 결과적으로 `PUT / GET / DELETE / PUT / COMPACT / …` 같은 *유효한 연산
  열 자체* 가 fuzz 대상이 된다.
- 이 타깃들은 Google **OSS-Fuzz** 에서 지속 실행된다.

#### protobuf 는 프로토콜이 아니다 (혼동 방지)

여기서 protobuf 는 **fuzzer 내부의 입력 기술 언어(IR)** 이며 *wire format 이 아니다*.
RocksDB 도 protobuf 를 저장 포맷이나 통신 프로토콜로 쓰지 않는다. 따라서 자체 바이너리
프로토콜을 쓰는 DBMS(= CUBRID)에도 **호환성 문제 없이** 적용된다 — 엔진은 protobuf 바이트를
한 번도 보지 않고, 의존은 fuzz 바이너리에만 링크된다. protobuf 를 아예 쓰지 않는 대안으로
libFuzzer 의 `FuzzedDataProvider` 로 byte→op 디코더를 손으로 쓰는 방법도 있다(구조 인식
mutation 품질은 떨어짐).

#### CUBRID 적용성

- CUBRID 등가물: `INSERT / UPDATE / DELETE / SCAN / IDX_INSERT / VACUUM / COMMIT` 열을
  `heap_insert_logical` / `heap_update_logical` / `heap_get_visible_version` / `btree_insert`
  (`src/storage/heap_file.h`, `src/storage/btree.h`) 로 직접 번역.
- 잡는 버그: slotted page slot 재사용 × 가변길이 갱신, overflow record 승격/강등 경계,
  unique 위반 롤백 후 재삽입, MVCC 가시성 × vacuum 간섭, heap best-space 불일치 —
  *하나의 연산* 이 아니라 *연산 열이 만든 상태* 에서 터지는 결함들.
- **결정적 난제: state reset.** libFuzzer 는 한 프로세스에서 입력을 수만 번 반복하므로
  매 입력 경계마다 엔진 상태가 결정적으로 초기화돼야 한다. RocksDB 는 `DestroyDB` +
  재오픈으로 푼다. CUBRID 는 서버 부팅·page buffer·log volume·transaction table·vacuum
  워커가 얽혀 있어 그만큼 가볍지 않다. 이 문제가 풀리지 않으면 *접근 자체가 성립하지 않는다*.
- 축 7 (stateful workload) 과 대상이 겹쳐 보이지만 층이 다르다 — 축 7 은 SQL·노드 레벨
  long-running 시나리오, 본 갈래는 **단일 프로세스 내부 API 레벨**.
- CUBRID 적용성: ★★★ (가치 큼) · 도입 비용 **높음** (§7.1~7.3 의 인프라 + reset 설계)
- **결론:** §6a-**E9 (storage-engine structured fuzzing)** 후보로 등록. **E5 선행** 전제
  (같은 `-DENABLE_FUZZING` 인프라를 공유). 착수 순서는 ROADMAP §6a 사다리가 정한다.

---

## 8. 축 6 — Differential testing

### 8.1 핵심 패턴

- 같은 SQL · 같은 dataset · 다른 DBMS → 결과 비교
- 판정 기준 불필요 — *외부 DBMS = 판정 기준*
- 사례: RAGS (Microsoft), SQLancer differential mode, CockroachDB ↔ PostgreSQL differential CI

### 8.2 단점 — dialect mismatch 지옥

- DATE 함수, NULL 정렬, 부동소수점, 정수 overflow, 문자열 collation, JSON 함수 — 거의 모든 DBMS가 다름
- 우회:
  - **canonical subset** — 모든 DBMS가 동일하게 정의하는 subset만 비교 (SQL-92 core 등)
  - **rewrite layer** — 차이가 나는 syntax를 dialect별로 rewrite

### 8.3 CUBRID 적용성 — N13 pg-wire-compat과 직접 결합

- N13 pg-wire-compat 이 진행되면 *CUBRID를 PostgreSQL driver로 접속* 가능 → PostgreSQL 과의 differential 비용이 급감
- 즉, 축 6은 **N13의 검증 도구**로 자연스럽게 자리잡음
- 단독 differential CI (`canonical subset` 모드) 는 N13 없이도 가능하지만 ROI 작음
- **결론:** §6a-**E6 (differential testing)** 후보로 등록하되, *우선순위는 N13 selected 진입 이후*. roadmap repo cross-cutting 신설 추천 (testkit §6a-E6 × N13). **번호 미배정** — ~~C-014~~ 는 2026-05-13 에 다른 내용으로 등록되었다 (§13 번호 정정 참조).

---

## 9. 축 7 — Stateful / workload testing

### 9.1 CockroachDB roachtest (randomized testing)

- 출처: <https://github.com/cockroachdb/cockroach/tree/master/pkg/cmd/roachtest>
- 동작:
  - random schema mutation
  - node restart / kill
  - range split / merge / rebalance
  - failover · partition
  - 모두 *동시* 진행하며 long-running invariant 검증
- *crash/recovery 축 + 분산 axis* 가 같이 들어감

### 9.2 FoundationDB simulation testing

- 거의 *신급* 으로 인용되는 사례 (Apple FoundationDB · TigerBeetle 영향)
- 핵심: **deterministic simulation**
  - fake network · fake disk · fake clock — 전체 cluster를 *single-process simulation*
  - seed 고정 시 완벽 재현 가능
  - 1초 wall time = 수 시간 simulated time
- 발견하는 버그: race · partial failure · clock-related · ordering — *production에서 reproduce 불가능*한 결함을 *재현 가능* 하게 만듦
- CUBRID 적용성: ★★ (직접) ~ ★★★ (장기 비전). FoundationDB의 *simulation framework* 자체는 C++ 작성 + actor model 가정. 직접 흡수는 비현실적이지만, *deterministic harness* 라는 *컨셉* 은 testkit `isolation` / `medium` 모듈의 장기 진화 방향과 정합

### 9.3 CUBRID 적용성

- 축 7은 *engine-suite* (graduated 별 프로젝트) 영역과 겹친다. HammerDB·benchbase 가 long-running workload 발생기.
- testkit은 *correctness invariant 검증* 책임, engine-suite은 *throughput 측정* 책임 — 본 repo cross-cutting **C-004** 의 연속선.
- **결론:** §6a-**E7 (stateful/randomized workload)** 후보로 등록하되, *engine-suite과의 책임 경계 정의가 선결*. ROADMAP §7 분기 게이트에서 strangler-fig 우선원칙(F-cross 위험)에 따라 *후순위*.

---

## 10. 축 8 — Hybrid (Materialize 패턴)

### 10.1 Materialize 사례

- Materialize 는 다음을 *같은 CI* 안에 결합:
  - **SQLsmith** — random valid SQL, crash 검증
  - **SQLancer** — wrong-result 검증
  - **differential testing** — Materialize ↔ PostgreSQL
  - **Zippy** — 자체 random framework (workload·schema mutation)
- 하나의 PR 검증 단계에서 4축이 동시에 돌아간다

### 10.2 시사점 — 축은 *대체* 가 아니라 *합성*

- 본 survey의 축 1~7은 *어느 하나로 대체* 되지 않는다
- *처음부터 hybrid를 가정한 카탈로그 설계*가 모범
- **§6a 카탈로그 설계 원칙:** 각 E-항목이 *독립* 하면서도, 같은 *case-format ingestion 인터페이스* (ROADMAP §6a-E1에서 이미 명시) 를 공유하도록 — `design/contracts.md` 에 hybrid 합성 가능성을 주석으로 남길 것.

---

## 11. CUBRID 적용성 종합 — §6a 카탈로그 확장 후보

본 survey에서 도출한 §6a-E2~E7 후보 (E1은 기등록) + E9 (2026-09-03 §7.4 추가분):

| ID | 이름 | 근거 축 | 도입 비용 | 즉시 ROI | 의존·전제 |
|---|---|---|---|---|---|
| E1 | sqllogictest 적용 | 1 | 낮음 | ★★★★ | (등록됨) ADR-EXT-001 |
| **E2** | **Random SQL fuzzing (SQLsmith 포팅)** | 2 | **낮음** | **★★★★** | schema introspect 경로만 CUBRID화 |
| **E3** | **Logic bug detection (SQLancer NoREC+TLP)** | 3 | 낮음 | ★★★★ | dialect adapter |
| **E4** | **Distributed isolation testing (AWDIT/Jepsen)** | 4 | 높음 | ★★★ (조건부) | N24 streaming-replication / N11 graduation |
| **E5** | **Parser/protocol fuzzing harness (libFuzzer)** | 5 | 중 | ★★★ | cubrid 본 repo fuzz target build option (선결) |
| **E6** | **Differential testing (PostgreSQL pair)** | 6 | 중 | ★★★ | N13 pg-wire-compat selected 이상 |
| **E7** | **Stateful/randomized workload** | 7 | 높음 | ★★ | engine-suite과 책임 경계 정의 (C-004) |
| (E8) | (Hybrid CI 통합) | 8 | (메타) | – | E2~E7·E9 중 둘 이상 채택 후 |
| **E9** | **Storage-engine structured fuzzing (libFuzzer + libprotobuf-mutator)** | 5 확장 (§7.4) | **높음** | ★★★ | **E5 선행** + in-process boot 진입점 + state reset 훅 |

### 우선순위 권고 (ROADMAP §7 분기 게이트 충돌 방지)

1. **즉시 후보 (strangler-fig Phase 3·4 와 *병행* 가능):** E2 (SQLsmith), E3 (SQLancer NoREC+TLP)
   - 도입 비용 낮음, 의존 없음, *지금 testkit이 비어 있는 영역* 을 직접 채움
   - PostgreSQL ecosystem의 *de facto* 모범 (regress + isolation + SQLsmith + SQLancer)
2. **조건부 후보 (선결 의존 충족 후):** E5 (cubrid 본 repo fuzz target 옵션), **E9 (E5 선행)**, E6 (N13 selected), E4 (HA/streaming graduation), E7 (C-004 경계 정의)
   - fuzzing 계열(E3·E5·E9)과 미등록 후보 2건의 *착수 순서* 는 ROADMAP **§6a 사다리** 가 단일 출처
3. **장기 추적:** SQLancer++ (adaptive grammar — niche DBMS에 의미), FoundationDB simulation 컨셉

### Strangler-fig Phase 4 우선원칙 충돌 점검

- ROADMAP §8 risk 7: *§6a 확장 영역이 strangler-fig 진척을 잠식*
- E2·E3 도입은 **strangler-fig medium/sql/isolation 모듈 대체와 자원 충돌** 가능 — 분기 게이트(§7)에 §6a 진척을 별 행으로 분리 기재하고, 충돌 시 strangler-fig 우선

---

## 12. Open Questions (정식 incubating 진입 시 결정, owner: hgryoo)

1. **E2 (SQLsmith) 우선순위.** strangler-fig 1차 대체 모듈 (ADR-004) 이 medium/isolation 으로 결정된 후 E2를 *언제* 진입할지. 분기 게이트 답변에 명시.
2. **E3 판정 기법 우선순위.** NoREC, TLP, PQS 중 1차 진입 판정 기법 선정. NoREC 권장 (도입 비용 최저, ROI 최고).
3. **dialect adapter 위치.** SQLsmith / SQLancer 모두 dialect adapter 가 필요. testkit 안에 *공통 dialect 레이어* 를 둘지, 도구별로 분산할지.
4. **fuzz corpus 보관 정책.** crash corpus·regression seed 를 testkit 안에 둘지, 별도 storage 에 둘지. testcases 레포 동결(NG1) 위반 가능성 점검.
5. **E5 fuzz target build option.** cubrid 본 repo 에 `-DENABLE_FUZZING` 옵션 추가가 선결. *cubrid 본 repo* 에 PR 필요 — testkit 단독 결정 불가.
6. **E4·E6·E7 trigger 시점.** N24·N11·N13 selected/graduation 일정에 의존 — roadmap repo planning.md 와 동기화 필요.
7. **hybrid CI 통합 (E8).** E2·E3 *모두 도입* 후 한 PR 에 두 축이 모두 돌도록 묶을지, 별 CI lane 으로 분리할지.
8. **E9 state reset 전략.** libFuzzer 의 in-process 반복 실행에서 CUBRID 엔진 상태를 어떻게 결정적으로 초기화할지. abort+drop / volume 재생성 / fork 격리 / 전용 reset 훅 — 측정으로만 답할 수 있으며, 답이 없으면 E9 는 성립하지 않는다.
9. **E9 입력 IR.** libprotobuf-mutator (protobuf 신규 의존, fuzz 빌드 한정) vs libFuzzer `FuzzedDataProvider` (의존 0, mutation 품질 하락). cubrid 본 repo 3rdparty 정책과 함께 판단.
10. **license / vendoring.** SQLancer (MIT), SQLsmith (custom), libFuzzer (Apache 2.0). 외부 코퍼스 ingestion 시 ROADMAP §8 risk 6 (sqllogictest 코퍼스 라이선스) 와 동일 패턴 적용.

---

## 13. References

### Tools / projects (실제 확인됨)
- sqllogictest: <https://www.sqlite.org/sqllogictest/>
- sqllogictest-rs: <https://github.com/risinglightdb/sqllogictest-rs>
- pg_regress: <https://github.com/postgres/postgres/tree/master/src/test/regress>
- PostgreSQL isolation tester: <https://github.com/postgres/postgres/tree/master/src/test/isolation>
- DuckDB test suite: <https://github.com/duckdb/duckdb/tree/main/test>
- SQLsmith: <https://github.com/anse1/sqlsmith>
- SQLancer: <https://github.com/sqlancer/sqlancer>
- CockroachDB roachtest: <https://github.com/cockroachdb/cockroach/tree/master/pkg/cmd/roachtest>
- Jepsen: <https://jepsen.io/>
- RocksDB fuzzing: <https://github.com/facebook/rocksdb/tree/main/fuzz>
- libprotobuf-mutator: <https://github.com/google/libprotobuf-mutator>
- libFuzzer: <https://llvm.org/docs/LibFuzzer.html>
- OSS-Fuzz: <https://google.github.io/oss-fuzz/>

### Papers / 연구 (이름·아이디어 기반 — 정식 인용은 incubating 진입 시 보강)
- Manuel Rigger 외, NoREC (PLDI'20) / TLP (OOPSLA'20) / PQS (ICSE'21)
- AWDIT — weak isolation anomaly · history scalability (최근 연구, 정확한 venue 확인 보강 대상)
- SQLaser — clause-guided fuzzing (최근 연구)
- SQLancer++ — adaptive grammar learning (최근 연구)
- FoundationDB simulation testing — Apple FoundationDB 운영 사례 (Strange Loop 2014 talk 등)

### testkit 내부 연결
- ROADMAP §6a 확장 영역 — 본 survey 의 *대상 섹션*
- ROADMAP §7 분기 게이트 — §6a 진척 별 행 기재 의무
- ROADMAP §8 risk 7 — strangler-fig 우선원칙
- analysis/isolation/ctl-grammar.md — 축 4 CTP `.ctl` 현행 분석 (PostgreSQL `.spec` 흡수 검토 시 baseline)
- analysis/_overview/case-formats.md — 축 1·2 코퍼스 도입 시 referenced

### roadmap repo cross-cutting

> **번호 정정 (2026-09-03).** 본 절은 2026-05-08 작성 시점에 C-013·C-014·C-015 를 *신설 추천* 으로
> 적었으나, roadmap repo 는 **5일 뒤인 2026-05-13 에 같은 번호를 lock-manager 계열로 등록** 했다
> (C-013 = × wait-event-stats, C-014 = × maintenance-mode, C-015 = × shared-memory-arch).
> 따라서 아래 세 항목의 *번호* 는 무효였다. fuzzing 경계만 실제 등록되었고, 나머지 둘은 미등록이다.

- **C-055 (등록됨, 2026-09-03)** — testkit §6a-E5·E9 × **N66-fuzz-target-infrastructure** :
  fuzz target·빌드·sanitizer·state reset 훅은 엔진, corpus·replay·triage 는 testkit.
  엔진 쪽 작업은 roadmap repo 에 **N66 (00-pending-review)** 로 등록되어 있다 —
  E5·E9 의 "cubrid 본 repo 선결" 이 가리키는 실체가 그것이다.
- **(미등록)** — testkit §6a-E3 (logic bug) × {N27 lock-manager, N28 mvcc, N29 page-buffer, N30 log-buffer} :
  회귀가 아니라 *정합성 검증* 채널. 번호 미배정 — E3 가 실제로 필요로 할 때 신청한다.
- **(미등록)** — testkit §6a-E6 (differential) × N13 pg-wire-compat : pg 의미 등가성 검증을 어디서 돌릴지.
  번호 미배정 — N13 이 selected 에 진입할 때 신청한다.

---

## 14. 변경 이력

| Date | Author | 변경 |
|---|---|---|
| 2026-05-08 | Claude (외부 조사 작성) | 초기 작성 — 8축 분류, 도구·연구 catalog, §6a-E2~E7 후보 도출 |
| 2026-09-03 | Claude (사용자 지시) | §7.4 추가 — 축 5 확장(engine-internal structured fuzzing, RocksDB `fuzz/` 참조). §11 에 E9 등록, §12 Open Question 8·9 추가 |
| 2026-09-03 | Claude (roadmap repo 대조) | §13 cross-cutting 번호 정정 — 제안했던 C-013·C-014·C-015 는 2026-05-13 에 lock-manager 계열로 등록되어 무효였다. fuzzing 경계만 **C-055** 로 실제 등록하고 엔진 쪽 작업을 roadmap repo 에 **N66-fuzz-target-infrastructure** 로 신설. 나머지 둘은 *미등록* 으로 표기 |
| TBD | hgryoo | 검토·확정 |
