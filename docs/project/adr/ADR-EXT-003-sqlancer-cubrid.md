# ADR-EXT-003: SQLancer 를 CUBRID 에 적용 (E3)

- **Status:** **Accepted** (2026-09-02) — 사용자 결정으로 **동시 트랙 승격**
- **Date:** 2026-09-02
- **Trigger:** E3 incubating 정식 진입 (`extensions/E3-sqlancer/requirements.md` §6)
- **Depends on:** ADR-001 (Go — Consequence 4: 확장은 subprocess + 아티팩트 ingest)
- **구현체:** `cubrid-sqlancer` 저장소 (별도)

---

## 1. Context

`extensions/E3-sqlancer/requirements.md` 는 NoREC/TLP 로 CUBRID 의 **wrong-result** 영역을 검증하자고 제안했다.
testkit 의 sql 모듈은 *정답이 미리 알려진* 케이스만 보므로 이 영역이 사각지대다.

로드맵상 E3 는 strangler-fig(Phase 0~5) *이후 또는 병행* 의 additive 작업이고, ROADMAP §8 의 마지막 risk 행은
*"우선순위 충돌 시 strangler-fig 우선"* 을 규정한다.

**2026-09-02 사용자 결정: E3 를 동시 트랙으로 우선 진행한다.** 본 ADR 은 그 결정과 그에 따라 확정된 사항을 기록한다.
strangler-fig 우선 규칙은 *자원 충돌이 실제로 발생했을 때* 적용되며, 두 트랙이 병행 가능한 동안에는 유예된다.

## 2. 결정 전에 확인한 사실 *(추정이 아니라 검증된 것)*

| # | 확인 사항 | 결과 |
|---|---|---|
| F1 | 상류 `sqlancer/sqlancer` 에 CUBRID provider 가 있는가 | **없다.** provider 23개(citus·clickhouse·cockroachdb·databend·datafusion·doris·duckdb·h2·hive·hsqldb·mariadb·materialize·mysql·oceanbase·postgres·presto·questdb·spark·sqlite3·tidb·yugabyte 등) 중 미포함. 코드 검색 0건 |
| F2 | 외부 jar 로 provider 를 공급할 수 있는가 | **가능하다.** `Main.java` 가 `ServiceLoader.load(DatabaseProvider.class)` 를 쓰고, 자체 Javadoc 이 *"This allows SQLancer to pick up providers in other JARs on the classpath"* 라고 명시. `@AutoService` 로 `META-INF/services` 자동 생성 |
| F3 | Maven Central 의 `com.sqlancer:sqlancer:2.0.0` 을 의존할 수 있는가 | **불가.** 게시일 2022-01-13 로 4년 이상 정체. `NoRECGenerator` · `NoRECOracle` · `Reproducer` 가 **없다**. 소스 빌드 후 로컬 설치 필요 |
| F4 | 공용 NoREC 구현이 있는가 | **있다.** `common/oracle/NoRECOracle` + `common/gen/NoRECGenerator` 인터페이스. provider 는 질의 생성만 구현하면 된다 |
| F5 | provider 1개의 규모 | HSQLDB 19파일 ≈1,420 LoC / DuckDB 39파일 ≈2,780 LoC |
| F6 | 라이선스 | MIT — vendoring·派生 자유 |

## 3. Decision

### 3-1. 재사용 vs 재구현 → **재사용**

SQLancer 본체를 그대로 쓰고 **CUBRID provider 만 새로 작성**한다. 공용 NoREC/TLP oracle(F4)을 그대로 활용하므로
재구현 대비 비용이 압도적으로 낮다.

### 3-2. 코드 거처 → **별도 저장소 + 라이브러리 의존**

`cubrid-sqlancer` 저장소가 `com.sqlancer:sqlancer` 를 의존하고, `@AutoService` 로 등록된 provider 를 classpath 에 얹는다.

- **채택 이유:** F2 로 공식 지원이 확인됨. fork 유지보수 부채 없음. 경계 명확.
- **기각한 대안:** *fork 유지* — 상류 변경마다 rebase 부채. *상류 PR* — 지금 단계에서 상류 리뷰 사이클에 종속될 이유가 없다.
  단, 성숙 후 upstream PR 은 **열어둔다** (provider 가 상류 트리 구조를 그대로 따르므로 이식 비용이 낮다).
- ⚠️ **fat-jar 금지.** shade 로 묶으면 `META-INF/services` 병합이 깨져 provider 가 조용히 사라진다
  ([sqlancer#799](https://github.com/sqlancer/sqlancer/issues/799)). 평범한 classpath 실행을 표준으로 한다.

### 3-3. 1차 oracle → **NoREC**

requirements §3 의 권고대로 NoREC 먼저. TLP(WHERE)는 코드 경로만 배선해두고 검증은 후속. PQS 는 범위 밖.

### 3-4. dialect 지식 공유 → **코드가 아니라 데이터**

ADR-001 §4a-4 의 결론을 그대로 적용한다. E2(SQLsmith, C++) · E1(sqllogictest) 과 provider 코드를 공유할 수 없으므로,
CUBRID 스키마·타입·함수 카탈로그를 **기계 판독 파일**로 뽑아 각 도구 어댑터가 읽게 한다. *(구현은 E2 진입 시)*

### 3-5. corpus 위치 → **testcases 레포 밖** (NG1)

mismatch/regression seed 는 `cubrid-testcases*` 3개 레포에 넣지 않는다. 1차는 `cubrid-sqlancer` 저장소 내부
(`logs/`, git 제외) 이며, 축적이 필요해지면 별도 storage 를 정한다.

### 3-6. testkit 과의 통합 → **후속**

ADR-001 Consequence 4 대로 *subprocess 구동 + 결과 아티팩트 ingest* 형태가 될 것이다. 다만 **지금은 통합하지 않는다** —
testkit 은 아직 Phase 1 이고 `design/contracts.md`(Phase 2)의 case-format ingestion 인터페이스가 없다.
E3 는 그때까지 **독립 실행 도구**로 운영한다.

## 4. 구현하며 확정된 CUBRID 고유 설계

| # | 사실 | 설계 귀결 |
|---|---|---|
| C1 | CUBRID 에 `CREATE DATABASE` SQL 이 **없다** (OS 명령 `cubrid createdb` 만) | provider 가 DB 를 만들지 않는다. 준비된 DB 에 접속해 **사용자 테이블/뷰만** 지운다. 결과적으로 **`--num-threads 1` 강제** |
| C2 | CUBRID 에 저장 가능한 **BOOLEAN 타입이 없다** | `BOOLEAN` 은 식 생성 전용 의사 타입. 컬럼 타입으로 절대 생성하지 않음. boolean 상수는 `(1 = 1)` / `(1 = 0)` |
| C3 | NoREC 의 unoptimized 질의 | `SELECT SUM(count) FROM (SELECT CASE WHEN <pred> THEN 1 ELSE 0 END AS count FROM t) AS res`. **WHERE 를 반드시 지운다** |
| C4 | 스키마 조회 | `INFORMATION_SCHEMA` 대신 `db_class` / `db_attribute`. `def_order` 정렬 필수 |
| C5 | 한 머신의 CUBRID 설치들이 master(1523)·broker(33000)·SHM 기본값을 공유 | **격리 실행 환경 필수.** 아니면 명령이 남의 master 에 붙어 내 DB 가 조용히 사라진다 |
| C6 | `count` 가 **예약어** | NoREC 별칭을 `norec_count` 로. `SUM(count)` 는 `Syntax error: unexpected ')', expecting '('` |
| C7 | CUBRID 서버 코어가 **10~20GB** (공유메모리) | 크래시 유도 워크로드에서 몇 분 만에 디스크가 찬다. 코어는 기본 off(`ulimit -c 0`)로 두고 `$CUBRID/log/coredump/*.coredump` 의 스택으로 원인을 본다 |

### ⚠️ 상류 두 provider 의 NoREC 구현을 따르지 않은 이유

- **DuckDB**: `generateUnoptimizedQueryString` 에서 WHERE 를 지우지 않는다 → 두 질의가 같은 것을 세어 oracle 이 무력화된다.
- **HSQLDB**: 술어를 `COUNT(*)` 로 대체해 **술어 자체를 잃는다**.

둘 다 CUBRID provider 에 복제하지 않았다. 상류에 보고할 후보 (§6).

## 5. Consequences

1. **새 저장소 `cubrid-sqlancer` 가 생긴다.** testkit 과 별개 생명주기. ROADMAP §0a 의 레포 전략에 3번째 항목으로 추가된다.
2. **bootstrap 이 2단계다** — SQLancer 소스 빌드(F3) + CUBRID JDBC 로컬 설치. `scripts/bootstrap.sh` 가 담당.
3. **격리된 CUBRID 실행 환경이 산출물이 된다** (`scripts/cubrid-env.sh`). 이는 E3 만의 필요가 아니라
   *공유 머신에서 CUBRID 를 쓰는 모든 작업*에 유용하므로, engine-suite 쪽으로 옮길지 후속 검토.
4. **분기 게이트(ROADMAP §7)에 E3 진척을 별 행으로 기재한다** — strangler-fig 잠식 여부를 관측하기 위해.
5. **ADR-EXT-002(E2/SQLsmith)의 입력이 확정된다** — dialect 공유 모델이 §3-4 로 정해졌다.

## 6. 후속 (미결)

- TLP(WHERE) oracle 검증
- 타입 확장: NUMERIC · BIT · BLOB/CLOB · ENUM · JSON · SET/컬렉션
- 인덱스 · 뷰 생성 액션 추가 (현재 CREATE TABLE / INSERT / UPDATE 만)
- `CUBRIDErrors` 목록 보강 — **논리 버그를 감출 수 있는 광범위한 패턴은 금지**
- 상류 SQLancer 에 DuckDB/HSQLDB 의 NoREC 결함 보고
- CI lane 편입 시점과 형태
- 발견된 버그의 CBRD 이슈 등록 경로 *(축 O — `migration-exclusions.md` 와 정합되게 testkit 밖에서)*
