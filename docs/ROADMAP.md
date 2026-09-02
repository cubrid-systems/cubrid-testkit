# ROADMAP — CUBRID Test Kit

- **날짜**: 2026-09-02 (Phase 1 진입 반영) / 2026-05-06 (§6a 확장 영역 추가) / 2026-04-28 (초안)
- **현재 위치**: **Phase 1 진행 중** — Phase 0 완료(2026-04-29), 게이트 통과(2026-09-02, `concept/phase0-retrospective.md` §11)
- **전략**: Strangler-fig 점진 대체 (1인 사이드 프로젝트, 6~12개월 호라이즌)
  + 확장 영역(외부 테스트 포맷 흡수, §6a)
- **제약 요약**: 기존 testcases 레포 수정 불가(NG1) / 외부 인터페이스 동결(NG2, 범위는 ADR-003) / 구현 언어 = Go(ADR-001) / 빌드 = `go build` + Justfile(ADR-002)
- **Analysis baseline**: cubrid-testtools @ 86992c1b334d55800f2700d60f9809c2ceca268d

---

## 0a. 레포지토리 전략

```
cubrid-testtools/         (기존, 동결 대상)
├── CTP/                   살아 있음 — Phase 0 ~ 4 동안 지속 가동
└── ROADMAP.md             고수준 의도만 유지, 상세는 새 레포로

cubrid-testkit/            (신규, 이번 작업의 결과물)
├── docs/                  모든 문서
│   ├── README.md          "CTP의 후계 — strangler-fig 진행 중" 명시
│   ├── ROADMAP.md         이 명세에서 추출한 상세 로드맵
│   ├── adr/               ADR-000 .. ADR-NNN + README (번호 단일 출처)
│   ├── analysis/          Phase 0 산출물 (브라운필드 분석)
│   │   ├── _overview/     CLI 트리(+진입점 전수 부록 A), conf 매트릭스, case 포맷
│   │   ├── medium/ sql/ shell/ isolation/ common/   (심도 5종)
│   │   └── inventory/     jdbc/sql_by_cci/ha_repl/cdc_repl/cci_compat (0/5 미착수)
│   ├── concept/           Phase 1 — north-star / 동결 명세 / non-goals / 마이그레이션 제외
│   ├── design/            Phase 2 — 아키텍처/모듈 설계 (미착수)
│   ├── extensions/        §6a 확장 E1~E7
│   └── survey/            DBMS 테스팅 생태계 조사
└── impl/                  Phase 3+ — 모듈별 구현
```

**레포 이름 결정 근거 (ADR-000 자리)**:
- `cubrid-testkit` — "kit"이 단일 runner를 넘어 분석/실행/리포트/생성 도구를 포괄. CTP라는 약어와 결별하여 새 정체성을 강조하면서, "cubrid-" 접두로 CUBRID 생태계 소속을 분명히.
- 거부된 대안: `cubrid-ctp-next`(레거시 단어를 영구 동결), `cubrid-testrunner`(역할을 좁게 고정)

**기존 레포 처리**:
- Phase 0~4: `cubrid-testtools`는 *그대로 가동*. 새 레포는 분석/설계/구현이 모이는 장소.
- Phase 5: `cubrid-testtools` 안의 *호출되지 않는 코드* 격리. 폐기 시점은 별도 ADR로 결정.

---

## 0. 개관

```
[Phase 0] 분석 ── deep ──> medium / sql / shell / isolation / common
                └─ inventory ──> jdbc / sql_by_cci / ha_repl / cdc_repl / cci_compat
        │
        ▼
[Phase 1] 신시스템 컨셉 + 외부 표면 동결 결정
        │
        ▼
[Phase 2] 아키텍처 + 모듈 설계 (의존이 적은 조각 우선)
        │
        ▼
[Phase 3] 1차 대체: 한 모듈을 strangler-fig로 교체 (가장 의존 적은 모듈)
        │
        ▼
[Phase 4] 2~3차 대체 + 인벤토리 모듈 표층 통합
        │
        ▼
[Phase 5] 잔여 모듈 정리 + 기존 CTP 격리/폐기 결정

[§6a 확장 영역] (Phase 4·5와 병행 가능, 또는 그 이후)
   └─ E1: sqllogictest CUBRID 적용  ── (다른 외부 포맷 후속 가능)
```

확장 영역(§6a)은 strangler-fig 외부에 있는 *additive 작업*. 외부 표면 동결
(NG2)·testcases 레포 동결(NG1) 밖이라 신규 결정 자유도가 큼.

---

## 1. Phase 0 — 기존 시스템 분석 (즉시 착수 가능, 1~2개월)

**Exit 조건**: 4개 심도 모듈 + common에 대해 아래 산출물이 모두 작성되고, 인벤토리 모듈에 대해 표층 표가 채워졌을 때.

**산출물 템플릿 (심도 모듈 1개당)**:
- `analysis/{module}/requirements.md` — 이 모듈이 *해결하는 문제*와 *외부에서의 호출 형태*
- `analysis/{module}/design.md` — 현재 클래스/스크립트 구조, 데이터 흐름, 외부 의존(SSH/conf/temp dir)
- `analysis/{module}/implementation-notes.md` — 미묘한 동작/재현하기 어려운 부분, 깨지기 쉬운 가정
- `analysis/{module}/io-contract.md` — CLI 인자 / conf 스키마 / 출력 파일 / 종료 코드의 정확한 형식
- `analysis/{module}/test-corpus.md` — 어느 testcases 디렉터리를 입력으로 받는지, 케이스 형식

**산출물 (인벤토리 모듈 1개당)**:
- `analysis/inventory/{module}.md` 한 장: CLI 진입점, conf 키 목록, 출력 포맷, 외부 의존, *추정 위험*

**즉시 착수 가능한 ActionableSlice (M0 마일스톤)**:
1. `bin/ctp.sh` 정독 → "어떤 모듈이 호출되는지" 트리 작성 → `analysis/_overview/cli-tree.md`
2. `conf/` *.conf 파일들 → 키 교집합/차집합 표 → `analysis/_overview/conf-matrix.md`
3. common 하위 디렉터리들(src/script/sched/grepo/gcov/tpl) → 의존 방향 그래프 → `analysis/common/deps.svg`
4. medium/sql/shell/isolation 각각의 `Main.java`(또는 동급) → "외부 호출 → 내부 호출" 1쪽 시퀀스
5. testcases 레포의 케이스 디렉터리 구조 → 케이스 파일 포맷의 *현존 분포* → `analysis/_overview/case-formats.md`

**Phase 0의 진단적 ADR 자리표시자**:
- ADR-001 — 새 시스템의 구현 언어 → **Accepted: Go** (2026-09-02)
- ADR-002 — 빌드 도구 → **Accepted: `go build` + `go.mod` + Justfile 메타 + `ctltool/Makefile` 유지**
- ADR-003 — 외부 표면 동결 범위 → **Accepted: CLI/conf/출력/종료코드/원격컨트랙트만. jar·Java API 제외. F1/F2/F3/NF 4등급**

---

## 2. Phase 1 — 신시스템 컨셉 + 외부 표면 동결 (1개월)

**Exit 조건**: 새 시스템이 *어떤 입력*을 받아 *어떤 출력*을 내야 하는지가 기존 CTP와 1:1 매핑된 표가 존재.

**산출물** *(2026-09-02 완료)*:
- [x] `concept/north-star.md` — 정체성. M1~M5 재설계 목표 / 확장점 1개 / 호환성 정의 / 성공 기준 3개
- [x] `concept/external-surface-freeze.md` — 동결 명세. **§10 이 본 Phase 의 Exit 조건인 신↔구 1:1 매핑 표(22행)**
- [x] `concept/non-goals.md` — NG1~NG11
- [x] ADR-001 / ADR-002 / ADR-003 확정본
- [x] `concept/phase0-retrospective.md` — 게이트 통과 기록 (구 `PHASE0_EXIT.md`)

**잔여 (Phase 2 진입 전 해소)**: `external-surface-freeze.md` §11-1(ext/script 외부 호출자 전수 확인) · §11-2(CI grep 대상 확인) · ADR-005(orphan task 폐기 정책) · NG3 결번 확인

---

## 3. Phase 2 — 아키텍처 + 모듈 설계 (1~2개월)

**Exit 조건**: "의존이 가장 적은 모듈 1개"가 단독으로 빌드/실행 가능한 새 시스템 골격이 결정됨.

**산출물**:
- `design/architecture.md` — 모듈 경계, 공통 레이어, 실행 모델(분산 실행/SSH 호환), 결과 처리 파이프라인
- `design/module-{module}.md` × 4 — 신구 매핑 표 포함 (구 클래스/스크립트 → 신 컴포넌트)
- `design/contracts.md` — 모듈 간 in-process 인터페이스 (이후 strangler-fig 대체의 경계)
- ~~ADR-004 — 첫 번째 대체 대상 모듈 선정~~ → **Phase 0→1 게이트에서 조기 결정 완료. Accepted: Option C' (shell 단독)**. 사유는 ADR-004 §7-1

---

## 4. Phase 3 — 1차 strangler-fig 대체 (2~3개월)

**Exit 조건**: 선정된 모듈(**shell / rqg / unittest**, ADR-004)이 새 시스템에서 동작하며, `bin/ctp.sh` 호환 진입점에서 기존 결과와 **회귀 동등성** 확인. 판정 기준은 `external-surface-freeze.md` 의 등급별 — F1 은 diff 0, F2 는 필드 단위 비교, F3 는 수용 여부 (ADR-003 Consequence 3).

**산출물**:
- `impl/m1/` — 첫 모듈 구현 코드
- `impl/m1/migration-bridge.md` — 기존 ctp.sh가 새 구현으로 라우팅되는 방식
- `impl/m1/regression-evidence.md` — 동일 testcases 입력에 대한 신/구 출력 동등성 보고서
- ADR-010 — 신/구 공존 기간 동안의 *유지보수 정책* *(구 초안의 ADR-005 — 번호 충돌로 재배정, `adr/README.md` 참조)*

---

## 5. Phase 4 — 2~3차 대체 + 인벤토리 모듈 통합 (2~3개월)

**Exit 조건**: 심도 4개 모듈이 모두 새 시스템에서 동작. 인벤토리 모듈은 *새 시스템 위의 얇은 어댑터*로 동작 가능.

**산출물**:
- `impl/m2/`, `impl/m3/`, `impl/m4/` — 모듈 단위 구현
- `impl/adapters/{inventory_module}.md` × N — 인벤토리 모듈에 대한 *호환 어댑터* 명세 (재작성 아님)
- ADR-011 — 인벤토리 모듈 중 *재작성 대상*과 *어댑터 유지 대상* 분류 *(구 초안의 ADR-006 — 번호 충돌로 재배정)*

---

## 6. Phase 5 — 잔여 정리 + 폐기 결정 (1개월)

**Exit 조건**: 기존 CTP 디렉터리에서 *더 이상 호출되지 않는 코드*가 식별되고 격리됨.

**산출물**:
- `cleanup/dead-code-inventory.md`
- `cleanup/legacy-archive-policy.md` — 기존 CTP 트리를 어떻게 보존/축소/제거할지
- `RETROSPECTIVE.md` — 6~12개월 회고

---

## 6a. 확장 영역 (Beyond Strangler-fig)

Phase 0~5는 *기존 CTP의 strangler-fig 대체*에 한정된다. 본 절은 strangler-fig
완료 후(또는 Phase 4·5와 병행 가능한 *additive 작업*으로) 새 시스템 위에 얹는
*확장 모듈* 목록. 본 절의 항목은 NG1(testcases 레포 동결)·NG2(외부 표면 동결)
*밖*에 있어 신규 결정 자유도가 크지만, 그만큼 *왜 testkit 안에서 하는가*에 대한
근거가 매번 필요.

### E1 — sqllogictest 적용 (CUBRID를 SUT로)

**목표**: SQLite 발 sqllogictest 포맷의 테스트 케이스를 CUBRID에서 실행하도록
testkit이 sqllogictest 러너를 새 모듈로 흡수.

**왜 testkit인가**: 케이스 형식 ingestion · diff/hash 회귀 · 결과 보고는
testkit의 핵심 역량과 동일. testtools/CTP에는 없던 *신규 모듈*이므로 strangler-
fig 외부이며, NG1·NG2와 충돌하지 않음 (CUBRID가 SUT라는 점은 NG4와도 무관 —
"비-CUBRID DBMS 호환의 신규 추가"가 아니라 "CUBRID를 외부 표준 포맷으로
검증").

**Phase 정합**:
- Phase 1 (외부 표면 동결): 영향 없음. sqllogictest는 새 진입점.
- Phase 2 (아키텍처): 신 모듈 슬롯이 일반 *case-format ingestion 인터페이스*로
  열리도록 `design/contracts.md`에 반영해야 함 — 이것이 본 항목의 Phase 2에
  미치는 *유일한* 영향.
- Phase 3 1차 대체 후보 *비교군*: sqllogictest는 CTP 의존이 0이라 "의존이
  가장 적은 모듈" 후보로 적격 — 단, 입력 코퍼스가 외부에 있어 testcases 레포
  외부 의존 관리라는 *새 변수*가 생김. ADR-004(1차 대체 모듈 선정) 시 후보로
  비교 대상에 포함.
- Phase 4·5와 병행: strangler-fig 진척과 독립적으로 진행 가능.

**스코프 (incubating 단계 — 확정 전)**:
- *대상 spec*: SQLite 원형 / DuckDB 확장 / CockroachDB 변형 중 primary target
  — 미정.
- *입력 코퍼스 정책*: 외부 트리 import vs mirror vs 자체 작성 — 미정.
  라이선스 점검 포함.
- *어댑터 위치*: `impl/sqllogictest/` 신 모듈 vs 인벤토리 모듈 — Phase 2
  contracts 결정에 종속.
- *결과 비교 모드*: sqllogictest 표준의 hash 기반 vs CUBRID expected 파일
  추가 — 미정.
- *SUT 구동 클라이언트*: JDBC / CCI / cubrid-cli 중 어느 경로 — 미정.

**Open Questions (incubating 정식 진입 시 결정 — owner: hgryoo)**:
1. *Pain point*: 왜 지금 sqllogictest? (외부 표준 진입 / 다른 DBMS와의 회귀
   비교 / 테스트 코퍼스 확장 / 특정 RND·CBRD 티켓?)
2. *Spec target*: 어느 변종을 baseline으로?
3. *코퍼스 정책*: 외부 트리 import 또는 mirror — 어느 트리, 어떤 라이선스?
4. *Acceptance*: 통과 case 수 / hash 일치율 / coverage 등 측정 기준?
5. *Phase 정합 재확인*: Phase 4·5 병행이 1인 가용성을 초과하지 않는지
   (분기 게이트 §7와 직접 결합).

**ADR 자리표시자**:
- ADR-EXT-001 *(트리거: 본 항목 incubating 정식 진입 시)* — sqllogictest
  spec variant 선정 + 입력 코퍼스 import 정책 + 결과 비교 모드 + SUT 구동
  클라이언트.

**참조**:
- SQLite sqllogictest 원형: <https://www.sqlite.org/sqllogictest/>
- DuckDB sqllogictest 확장: github.com/duckdb/duckdb (`test/sqllogictest`)
- CockroachDB logictest: github.com/cockroachdb/cockroach (`pkg/sql/logictest`)
- Requirements: `extensions/E1-sqllogictest/requirements.md`

### E2 — Random SQL Fuzzing (SQLsmith 포팅)

**목표**: SQLsmith 류 grammar-aware random SQL generator 를 CUBRID 에 포팅 — parser/planner/executor 강건성 검증 (crash 중심).

**왜 testkit인가**: testkit 의 *random generation 축이 부재* — 손으로 만든 케이스만으로는 deep nesting / lateral / window 조합에서의 internal state corruption 을 못 잡음.

**Phase 정합**: E1 과 동일 — Phase 4·5 와 병행 가능, contracts.md 의 case-format ingestion 인터페이스 공유.

**스코프 (incubating)**:
- 재사용 (SQLsmith C++ subprocess) vs 재구현 (ADR-001 결정 언어) — 미정
- CUBRID dialect 가산 범위 (path expression / serial / connect-by / method) — 미정
- fuzz corpus 위치 (testkit / cubrid 본 repo / 외부) — 미정

**Open Questions**: requirements §6 참조.

**ADR 자리표시자**: ADR-EXT-002 *(트리거: incubating 정식 진입 시)*.

**참조**:
- SQLsmith: <https://github.com/anse1/sqlsmith>
- Requirements: `extensions/E2-sqlsmith/requirements.md`

### E3 — Logic Bug Detection (SQLancer NoREC + TLP)

**목표**: SQLancer 의 NoREC (optimizer rewrite bug) + TLP (3-valued logic) oracle 을 CUBRID 에 적용 — wrong-result 검증.

**왜 testkit인가**: testkit 은 *expected file diff* 만 — *정답을 모르는* random query 의 wrong-result 영역이 사각지대.

**Phase 정합**: E1·E2 와 동일 — case-format ingestion 인터페이스 공유. dialect adapter 는 E2 와 *공통 레이어* 가능 (Open Question 3).

**스코프 (incubating)**:
- 1차 oracle = NoREC (도입 비용 최저, ROI 최고) + TLP — 미확정
- PQS / SQLaser / SQLancer++ 는 후속 ADR
- 재사용 (SQLancer Java) vs 재구현 — 미정

**Open Questions**: requirements §6 참조.

**ADR 자리표시자**: ADR-EXT-003 *(트리거: incubating 정식 진입 시)*.

**참조**:
- SQLancer: <https://github.com/sqlancer/sqlancer>
- Requirements: `extensions/E3-sqlancer/requirements.md`

### E4 — Distributed Isolation Testing (AWDIT / Jepsen)

**목표**: 분산 / HA / streaming-replication 환경에서의 isolation anomaly 검증.

**왜 testkit인가**: 기존 isolation 모듈은 *단일 노드 다중 클라이언트* 만. 분산 axis 는 별 도구가 필요. *단일노드 축 4* (PostgreSQL `.spec` + Hermitage) 는 strangler-fig isolation 모듈 자체에 흡수 (ADR-004 검토 시).

**Phase 정합 (조건부)**: N24 streaming-replication 또는 N11 logical-replication-extension *graduation 후*. roadmap repo planning 과 동기화.

**스코프 (incubating)**: AWDIT vs Jepsen 1차 선택 / fault injection 채널 / engine-suite 책임 경계 (C-004) — 모두 미정.

**Open Questions**: requirements §6 참조.

**ADR 자리표시자**: ADR-EXT-004.

**참조**:
- Jepsen: <https://jepsen.io/>
- PostgreSQL isolation tester: github.com/postgres/postgres (`src/test/isolation`)
- Requirements: `extensions/E4-distributed-isolation/requirements.md`

### E5 — Parser / Protocol Fuzzing Harness (libFuzzer)

**목표**: byte-level coverage-guided fuzzing 으로 SQL parser / CCI·JDBC binary protocol 강건성 검증.

**왜 testkit인가**: testkit 은 corpus + replay + crash triage 책임. *fuzz target build option* 은 cubrid 본 repo 책임 — 본 항목은 *cross-repo 협업* 이 필수 (C-015 cross-cutting).

**Phase 정합 (조건부)**: cubrid 본 repo 의 `-DENABLE_FUZZING` 등 build option 추가 *선결*.

**스코프 (incubating)**: fuzz target layer (parser / CCI / JDBC) / fuzzer 본체 (libFuzzer / AFL / honggfuzz) / corpus 위치 — 모두 미정.

**Open Questions**: requirements §6 참조.

**ADR 자리표시자**: ADR-EXT-005.

**참조**:
- libFuzzer: <https://llvm.org/docs/LibFuzzer.html>
- Requirements: `extensions/E5-parser-fuzzing/requirements.md`

### E6 — Differential Testing (PostgreSQL Pair)

**목표**: 같은 SQL 을 CUBRID 와 PostgreSQL 양쪽에 실행해 *결과 차이* 로 wrong-result 검증. 판정 기준 = 외부 DBMS 자체.

**왜 testkit인가**: oracle 비용이 0 — peer DBMS 가 oracle. 단, dialect mismatch 노이즈 통제가 핵심.

**Phase 정합 (조건부)**: N13 pg-wire-compat *selected 이상* — selected 이후 비용이 급감. C-014 cross-cutting.

**스코프 (incubating)**: canonical subset vs rewrite layer / dialect rewrite catalog / peer DBMS 범위 — 모두 미정.

**Open Questions**: requirements §6 참조.

**ADR 자리표시자**: ADR-EXT-006.

**참조**:
- Requirements: `extensions/E6-differential/requirements.md`

### E7 — Stateful / Randomized Workload

**목표**: schema mutation + node restart + partition + failover 가 동시 진행되는 long-running 시나리오에서 invariant 검증.

**왜 testkit인가**: invariant 검증 = correctness 영역 = testkit 책임. throughput 은 engine-suite (HammerDB / benchbase) 책임. C-004 책임 경계 정의가 *선결*.

**Phase 정합 (조건부)**: C-004 cross-cutting 결론 *선결*. 분기 게이트 §7 후순위.

**스코프 (incubating)**: scenario (roachtest 류 randomized vs FoundationDB 류 deterministic simulation) / invariant 카탈로그 / engine-suite 자산 재사용 범위 — 모두 미정.

**Open Questions**: requirements §6 참조.

**ADR 자리표시자**: ADR-EXT-007.

**참조**:
- CockroachDB roachtest: github.com/cockroachdb/cockroach (`pkg/cmd/roachtest`)
- FoundationDB simulation: <https://apple.github.io/foundationdb/testing.html>
- Requirements: `extensions/E7-workload/requirements.md`

### §6a 카탈로그 출처 / 인덱스

| ID | 진입성 | 선결 | requirements |
|---|---|---|---|
| E1 | 즉시 후보 | — | extensions/E1-sqllogictest/ |
| E2 | 즉시 후보 | — | extensions/E2-sqlsmith/ |
| E3 | 즉시 후보 | dialect adapter 위치 (E2 와 공유) | extensions/E3-sqlancer/ |
| E4 | 조건부 | N24 / N11 graduation | extensions/E4-distributed-isolation/ |
| E5 | 조건부 | cubrid 본 repo `-DENABLE_FUZZING` | extensions/E5-parser-fuzzing/ |
| E6 | 조건부 | N13 pg-wire-compat selected 이상 | extensions/E6-differential/ |
| E7 | 조건부 | C-004 책임 경계 정의 | extensions/E7-workload/ |

근거: `survey/dbms-testing-ecosystem.md` (8축 분류, §11 카탈로그 확장 후보).

---

## 7. 분기별 재평가 게이트 (1인 사이드 프로젝트 완충)

매 분기 종료 시 다음 3개 질문에 *서면으로* 답변:
1. 이번 분기 실제 투입 시간은 계획 대비 어땠는가?
2. Phase 진행이 의존관계 그래프 상의 *현재 위치* 어디까지 도달했는가?
3. 다음 분기에 *어느 Phase의 어느 산출물*을 끝낼 것인가?

답이 부정적이면 다음 분기는 신규 모듈에 손대지 말고 *기존 산출물의 깊이*를 보강하는 분기로 사용.

---

## 8. 위험 목록 (우선순위 순)

| Risk | Trigger | Mitigation |
|------|---------|------------|
| 1인 가용성 변동으로 Phase 정체 | 분기 게이트 미통과 | 신규 진입 금지, 깊이 보강 분기로 전환 |
| 외부 표면 동결 위반 | testcases가 새 동작에 의존 | Phase 1의 freeze 명세 + 회귀 동등성 게이트 |
| common 의존 그래프가 예상보다 복잡 | M0의 deps 그래프 단계에서 발견 | Phase 2 진입 전 설계 1주 보강 |
| testcases 포맷의 *문서화되지 않은 변형* | 테스트 코퍼스 분석 단계에서 발견 | 신시스템 파서가 관용적(lenient)이어야 한다는 비기능 요구로 승격 |
| Java 외 언어 선택 시 학습 곡선 | ADR-001 결정 시점 | 1차 대체 모듈은 *언어 결정의 검증 슬라이스*로 활용 |
| §6a-E1 외부 sqllogictest 코퍼스 라이선스·동기화 부채 | E1 incubating 정식 진입 | ADR-EXT-001로 import 정책 명시; full mirror 대신 의미 있는 부분집합만 vendor in 또는 자동 동기화 스크립트로 운영 |
| §6a-E2/E3 fuzz·logic-bug corpus 의 testcases 레포 동결(NG1) 위반 | E2·E3 incubating 정식 진입 | 외부 storage 또는 testkit 내부 별 트리에 보관; ADR-EXT-002·003에서 corpus 위치 명시 |
| §6a-E5 cubrid 본 repo 의 fuzz target build option 미진척 | E5 incubating 정식 진입 시도 | testkit 단독 시작 금지; cubrid 본 repo PR (`-DENABLE_FUZZING` 등) 선결, C-015 cross-cutting 트래킹 |
| §6a-E4/E6 선결 의존 (N24·N11·N13) 미진척 | E4·E6 incubating 진입 검토 시 | roadmap repo planning 과 동기화; 선결 graduation 전에 진입 시도 금지 |
| §6a 확장 영역이 strangler-fig 진척을 잠식 | 분기 게이트에서 Phase 4·5 지연 vs E1~E7 진척이 역전 | 분기 게이트 답변에 §6a 진척을 별 행으로 분리 기재; 우선순위 충돌 시 strangler-fig 우선 |

---

## Phase 0 분석 stub 체크리스트

Phase 0 진행 상황을 추적합니다. 각 항목은 `analysis/` 트리의 스텁 파일에 대응합니다.

### _overview (4)
- [x] analysis/_overview/cli-tree.md
- [x] analysis/_overview/conf-matrix.md
- [x] analysis/_overview/case-formats.md
- [x] analysis/_overview/deps-of-common.md

### medium (5)
- [x] analysis/medium/requirements.md
- [x] analysis/medium/design.md
- [x] analysis/medium/implementation-notes.md
- [x] analysis/medium/io-contract.md
- [x] analysis/medium/test-corpus.md

### sql (5)
- [x] analysis/sql/requirements.md
- [x] analysis/sql/design.md
- [x] analysis/sql/implementation-notes.md
- [x] analysis/sql/io-contract.md
- [x] analysis/sql/test-corpus.md

### shell (5)
- [x] analysis/shell/requirements.md
- [x] analysis/shell/design.md
- [x] analysis/shell/implementation-notes.md
- [x] analysis/shell/io-contract.md
- [x] analysis/shell/test-corpus.md

### isolation (5)
- [x] analysis/isolation/requirements.md
- [x] analysis/isolation/design.md
- [x] analysis/isolation/implementation-notes.md
- [x] analysis/isolation/io-contract.md
- [x] analysis/isolation/test-corpus.md

### common (5)
- [x] analysis/common/requirements.md
- [x] analysis/common/design.md
- [x] analysis/common/implementation-notes.md
- [x] analysis/common/io-contract.md
- [x] analysis/common/test-corpus.md

### inventory (5)
- [ ] analysis/inventory/jdbc.md
- [ ] analysis/inventory/sql_by_cci.md
- [ ] analysis/inventory/ha_repl.md
- [ ] analysis/inventory/cdc_repl.md
- [ ] analysis/inventory/cci_compat.md
