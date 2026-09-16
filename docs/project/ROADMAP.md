# ROADMAP — CUBRID Test Kit

- **날짜**: 2026-09-04 (E9 Tier 1 오라클 완비 — 정합성 검사 3종 + TSan/UBSan 베이스라인) / 2026-09-03 (§6a 부록 — fuzzing 우선순위 사다리 + E9 신설; 저녁: 순위 4 완료 · E9 state reset 게이트 통과) / 2026-09-02 (Phase 1 진입 반영) / 2026-05-06 (§6a 확장 영역 추가) / 2026-04-28 (초안)
- **현재 위치** *(2026-09-15)*: **Phase 3 진행 중** — shell·rqg native Runner 가 main 에 있고 `TESTKIT_NATIVE=shell` 뒤에 있다. develop head 에서 전체 코퍼스를 돌렸고 실패는 전부 원인이 붙었지만, 게이트(ADR-013 전체 코퍼스 비교)는 아직. **Phase 4 진행 중** — sql·medium 은 native 이고 ADR-017 게이트 통과(2026-09-12). **isolation native** (2026-09-15, 사용자 결정) — 설계 `design/module-isolation.md`, 실행부 ADR-007, 게이트 ADR-018 (슬롯 1개·4개 모두 러너 차이 0), 병렬 슬롯이 기본값, 가이드 `category/isolation/`. 자원이 충돌하면 §8 의 규칙대로 shell 게이트가 먼저
- **2026-09-11 의 위치**: Phase 3 진행 중 — shell native Runner 가 `TESTKIT_NATIVE_SHELL` 뒤에. Phase 4 병행 착수 — sql·medium P0(측정·문서), 설계는 `design/module-sql.md`, 실행부는 ADR-016
- **이전 위치**: Phase 2 완료 (2026-09-02) — Phase 0 완료(2026-04-29) · Phase 1 게이트 통과 및 산출 완료 · Phase 2 설계 5종 완료
- **동시 트랙**: **§6a-E3 (SQLancer) 진행 중** — 사용자 결정으로 우선 승격 (ADR-EXT-003). 구현체는 별도 저장소 `cubrid-sqlancer`
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

cubrid-sqlancer/           (신규, §6a-E3 — cubrid-testkit 이 submodule 로 참조)
└── SQLancer provider. 독립 실행 도구이며 Phase 2 contracts 이후 통합 검토

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
│   ├── extensions/        §6a 확장 E1~E10 (E8 은 Hybrid CI 메타 자리로 예약)
│   └── survey/            DBMS 테스팅 생태계 조사
├── extensions/
│   └── cubrid-sqlancer/   submodule — §6a-E3 SQLancer provider (별도 private 저장소)
├── go.mod                 모듈 루트는 저장소 루트다 (Go 표준 배치)
├── cmd/testkit/           진입점
└── internal/              구현 (Phase 3+)
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
- [x] `concept/phase0-retrospective.md` — 게이트 통과 기록 (구 `concept/phase0-retrospective.md`(구 `PHASE0_EXIT.md`))

**잔여 (Phase 2 진입 전 해소)**: `external-surface-freeze.md` §11-1(ext/script 외부 호출자 전수 확인) · §11-2(CI grep 대상 확인) · ADR-005(orphan task 폐기 정책) · NG3 결번 확인

---

## 3. Phase 2 — 아키텍처 + 모듈 설계 (1~2개월)

**Exit 조건**: "의존이 가장 적은 모듈 1개"가 단독으로 빌드/실행 가능한 새 시스템 골격이 결정됨.
→ **충족.** `design/architecture.md` §9 — `shellsuite` Runner 는 기존 CTP 자산을 컴파일 시점에 요구하지 않고,
나머지 task 는 `legacy` Runner 가 subprocess 로 처리하므로 shell 하나만으로 빌드도 실행도 성립한다.

**산출물** *(2026-09-02 완료)*:
- [x] `design/architecture.md` — 패키지 구조, dispatcher→worker 실행 모델, 결과 파이프라인, M1~M5 대응, **Exit 조건 충족 근거(§9)**
- [x] `design/module-shell.md` — 1차 대체 대상. **35 클래스 신구 매핑 + 축 T/O 판정**, 범위 재산정
- [x] `design/module-{sql,isolation,medium}.md` — 매핑 표 수준 (ADR-004 Consequence 1)
- [x] `design/contracts.md` — 계약 5개(`Runner` `Format` `Channel` `Sink` `Feedback`)와 **계약이 아닌 것**의 명시

**Phase 2 진입 전 해소 항목** *(완료)*: freeze §11-1(ext/run_*.sh 외부 호출자 — testcases 3레포에서 0건) ·
§11-2(마커 소비자 — `found core file` 은 케이스 2곳이 직접 grep, 확정 F1) · §11-9(baseline 스팟체크 — 변경 없음)
- ~~ADR-004 — 첫 번째 대체 대상 모듈 선정~~ → **Phase 0→1 게이트에서 조기 결정 완료. Accepted: Option C' (shell 단독)**. 사유는 ADR-004 §7-1

---

## 4. Phase 3 — 1차 strangler-fig 대체 (2~3개월)

**Exit 조건**: 선정된 모듈(**shell / rqg / unittest**, ADR-004)이 새 시스템에서 동작하며, `bin/ctp.sh` 호환 진입점에서 기존 결과와 **회귀 동등성** 확인. 판정 기준은 `external-surface-freeze.md` 의 등급별 — F1 은 diff 0, F2 는 필드 단위 비교, F3 는 수용 여부 (ADR-003 Consequence 3).

**산출물**:
- `internal/runner/shellsuite/` — 첫 Runner 구현 코드
- `design/migration-bridge.md` — 기존 ctp.sh가 새 구현으로 라우팅되는 방식 *(코드가 아니라 문서이므로 docs/ 아래)*
- `evidence/regression-shell.md` — 동일 testcases 입력에 대한 신/구 출력 동등성 보고서 (판정 기준은 ADR-013)
- ADR-010 — 신/구 공존 기간 동안의 *유지보수 정책* *(구 초안의 ADR-005 — 번호 충돌로 재배정, `adr/README.md` 참조)*

---

## 5. Phase 4 — 2~3차 대체 + 인벤토리 모듈 통합 (2~3개월)

**Exit 조건**: 심도 4개 모듈이 모두 새 시스템에서 동작. 인벤토리 모듈은 *새 시스템 위의 얇은 어댑터*로 동작 가능.

**산출물**:
- `internal/runner/{sqlsuite,isolation,replication}/` — Runner 단위 구현
- `design/adapters/{inventory_module}.md` × N — 인벤토리 모듈에 대한 *호환 어댑터* 명세 (재작성 아님)
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
- *어댑터 위치*: `internal/runner/sqllogictest/` 신 Runner vs 인벤토리 모듈 — Phase 2
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

**ADR**: **ADR-EXT-003 — Accepted (2026-09-02).** 사용자 결정으로 동시 트랙 승격.
- 재사용(SQLancer 본체) + CUBRID provider 신규 작성. 상류에는 CUBRID provider 가 **없다**
- 별도 저장소 `cubrid-sqlancer` + 라이브러리 의존 (SQLancer 가 ServiceLoader 로 외부 jar provider 를 공식 지원)
- 1차 oracle = NoREC. dialect 지식은 코드가 아니라 **데이터(카탈로그 파일)** 로 공유
- corpus 는 testcases 레포 밖 (NG1)
- testkit 과의 통합은 Phase 2 의 `design/contracts.md` 이후. 그때까지 **독립 실행 도구**

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

**왜 testkit인가**: testkit 은 corpus + replay + crash triage 책임. *fuzz target build option* 은 cubrid 본 repo 책임 — 본 항목은 *cross-repo 협업* 이 필수 (**C-055** cross-cutting; 엔진 쪽 작업 = roadmap repo `N66-fuzz-target-infrastructure`).

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

**Phase 정합 (조건부)**: N13 pg-wire-compat *selected 이상* — selected 이후 비용이 급감. cross-cutting 신설 필요 (번호 미배정 — ~~C-014~~ 는 이미 다른 내용으로 등록됨).

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

### E9 — Storage-engine Concurrency Fuzzing (schedule × operation interleaving)

> **재정의 (2026-09-03).** 본 항목은 처음에 *단일 스레드 operation sequence* fuzzing 으로
> 적혔다. 그 형태는 값이 거의 없다는 것이 확인되어 재정의한다. 근거 두 가지.
>
> **(1) 찾으려는 결함이 거기 없다.** slot 재사용 × 동시 갱신, latch 획득 순서, vacuum 과
> reader 의 간섭 — storage 결함은 *연산 열* 이 아니라 *인터리빙* 이 만드는 상태에 있다.
> 한 스레드로는 도달하지 않는다.
>
> **(2) 단일 스레드로 잰 것은 목표 구성에 대해 말해주는 바가 없다.** 저장 계층은 모드별로
> 갈라진 코드다 — `page_buffer.c` 에 `SERVER_MODE` 분기 **146** 개, `log_manager.c` **53** 개,
> `vacuum.c` **26** 개. `vacuum_Master_daemon` 은 `cubthread::daemon` 이다. SA 모드(단일 스레드)
> 에서의 관측은 SERVER_MODE 서버에 이어지지 않는다.

**목표**: storage engine 내부 API (heap / B-tree / slotted page / MVCC) 를 **여러 스레드의
연산 열과 그 인터리빙(schedule)** 으로 fuzzing — 동시성이 만든 상태에서 터지는 결함 검출.

**왜 testkit인가**: E5 와 동일한 근거 (corpus + replay + triage 는 testkit, fuzz target
build 는 cubrid 본 repo). 본 항목이 추가로 요구하는 것은 reset 훅이 아니라 **schedule 을
제어할 지점** 이다 — 그리고 그것은 엔진에 이미 있다 (아래).

**참조 구현**: RocksDB `fuzz/` 는 구조화 입력의 형태(libFuzzer + protobuf + libprotobuf-mutator)에
대해서만 참조 가치가 있다. *대상* 은 다르다 — RocksDB 의 `PUT/GET/DELETE/COMPACT` 열은 단일
스레드이고, 본 항목은 그 위에 **스케줄 축** 을 하나 더 얹는다. 입력은 연산 열이 아니라
`(스레드별 연산 열 × 인터리빙)` 쌍이다.

**엔진에 이미 있는 것 (절반) — 그리고 빠진 하나**: `src/base/fault_injection.c` 의
`fi_handler_hold` 와 `fi_handler_hang` 은 지정된 지점에서 **창을 넓히거나 멈추는** 핸들러이고,
FI 상태가 `thread_p->fi_test_array` 에 있어 **스레드별로 무장** 된다. 그러나 `hold` 는
`sleep(seconds)` 로 시간 기반이고 `hang` 은 `while(true) sleep(1)` 로 해제 경로가 없다 —
둘 다 대기 중 상태를 다시 보지 않으므로 **"A 는 B 가 Y 에 도달할 때까지 기다린다" 를 표현할 수
없다.** 즉 창 넓히기(확률적 노출)까지가 오늘 가능한 전부이고, *결정적 재생* 에는 조건 변수에서
대기하고 하네스가 깨우는 **rendezvous 핸들러 하나** 가 더 필요하다. 기존 네 핸들러 옆에 하나를
더하는 규모이며 CBRD-27198 이 그 관례를 이미 보여준다. requirements §5.4.
2026-09-02 머지된 CBRD-27198 이 `disk_reserve_sectors_in_volume` 에 `fi_handler_hold` 훅을
추가한 목적이 정확히 그것이었다 — "The race lasts microseconds … a test can only hit that
window by luck. Add a hold-style fault injection handler and a hook … to widen it on demand."
즉 **스케줄 주입 원시 도구가 이미 있고 실제로 그 용도로 쓰이고 있다.** 본 항목은 그 위에
스케줄 생성기와 불변식 검사를 얹는 일이지, 새로 만드는 일이 아니다.

**protobuf 오해 해소 (선결 확인 완료 — 2026-09-03)**: CUBRID 는 자체 바이너리 프로토콜을 쓰고
protobuf 를 쓰지 않는다. **그래도 무관하다** — 여기서 protobuf 는 *fuzzer 내부의 입력 기술
언어(IR)* 일 뿐 wire format 이 아니다. harness 가 protobuf 메시지를 받아 `heap_insert_logical()`
같은 **내부 C API 를 직접 호출** 하므로, CUBRID 는 protobuf 바이트를 한 번도 보지 않는다.
의존은 `cubrid-fuzz-storage` **fuzz 바이너리에만** 링크되고 배포 산출물은 불변.
protobuf 없이 가는 대안(`FuzzedDataProvider`)도 ADR-EXT-009 비교 대상.
근거·비교표: `extensions/E9-storage-fuzzing/requirements.md` §2.

**Phase 정합 (조건부)**: **E5 선행**. `-DENABLE_FUZZING` 인프라와 crash triage 위에 얹힌다.
SERVER_MODE 엔진의 in-process 기동은 **2026-09-03 확인됨** — 리스너 없이 뜨고 데몬 15 스레드가
살아 있으며 깨끗이 내려간다 (requirements §5.3).

**무엇이 연산을 만드는가 (2026-09-04 확정)**: 합성하지 않는다. `heap_insert_logical` 등은
호출자가 세워 준 전제 위에서 돌기 때문에, 임의로 조합하면 *실제 호출자가 만들지 않는 순서* 를
만들고 거기서 나온 crash 는 전제 위반에 대한 정당한 반응일 수 있다. 대신 **미리 컴파일된
XASL 을 재생** 한다 — 질의 실행이 전제를 다 세우므로 모든 경로가 구성상 도달 가능하고,
퍼저는 **스케줄만** 탐색한다. 서버는 SQL 을 컴파일하지 않으므로(`parser_main` ·
`xts_map_xasl_to_stream` 이 `libcubrid.so` 에 없음) 픽스처가 필요하고, 그 생산·보관 설비가
**§6a-E10 (신설)** 이다. 내부 API 어휘 모델링은 *future work*.

**2026-09-04 측정이 이 항목의 전제를 바꿨다 — Tier 1 이 주력이다.** 입력을 고정하고 반복하는
것만으로 4 스레드 순열 24 가지가 **전부, 거의 균등하게** 나온다(최빈 5.2%, 균등 4.17%).
엔진 작업(`file_create_heap`) 유무와 무관하고 실제 작업 쪽이 오히려 더 균등하다 — "엔진
latch 가 순서를 좁힌다" 는 가설은 틀렸다. 따라서 **스케줄 통제로 얻는 것은 탐색 커버리지가
아니라 재현** 이고, Tier 2 는 탐색 도구가 아니라 **triage 도구** 로서 Tier 1 *뒤* 에 온다.
libFuzzer 가 스케줄을 탐색한다는 구상은 폐기한다. 근거: requirements §5.3a · roadmap
`N66/10-design_fi-rendezvous.md` §7.2

**그 다음 개선 — 오라클 — 은 같은 날 끝냈다 (requirements §5.3b).** 정합성 검사는 비용이
위치를 정한다: `disk_check` 0.000 s 와 `file_tracker_check` 0.007 s 는 매 입력,
`xboot_check_db_consistency` 는 **7.0 s** 이므로 세션 경계에만. TSan 빌드는 플래그
하나(`-DFUZZ_SANITIZERS=thread`)였고, ASan/UBSan 과 **택일이 아니라 같은 하네스의 두 실행**
이다. 다만 베이스라인 없이는 새니타이저가 오라클이 못 된다 — 깨끗한 실행이 TSan 222 건,
UBSan 10 건을 매번 낸다. 억제 파일(42 + 3 규칙)로 **0 건** 이 되고, 그 침묵이 값을 했다:
워크로드에 heap drop 을 넣자 `vacuum_add_dropped_file ()` 에 닿아 새 race 40 회가 나왔는데
기존 179 건 사이였다면 안 보였을 것이다. **베이스라인은 판정이 아니다** — 적부는 하나도
판정하지 않았고, 목적은 침묵이 의미를 갖게 하는 것뿐이다.

**스코프 (incubating — Tier 2 기준, 보류 중)**:
- 입력 IR — 연산이 아니라 **스케줄과 참가자 수** 를 기술한다. libprotobuf-mutator vs
  `FuzzedDataProvider` vs 자체 — 미정
- **재현성의 정의가 바뀐다.** 단일 스레드 전제에서는 "같은 입력 → 같은 상태" 였다. 멀티스레드에서
  그것은 얻을 수도 없고 원할 것도 아니다 — 비결정성이 요점이기 때문이다. 필요한 것은
  **스케줄을 입력에 포함시켜 `(연산, 스케줄)` 쌍이 재현되게** 하는 것이다. reset 은 게이트가
  아니라 *알려진 시작 DB 를 만드는 준비 단계* 로 격하된다
- **본 repo 에 요구하는 것은 rendezvous 핸들러 하나** (§5.4). 지점 확대가 아니다 — 기존
  `hold`/`hang` 은 대기 중 상태를 보지 않아 깨울 수 없고, 그래서 재생이 안 된다
- 불변식 카탈로그 — crash 없이도 위반을 잡을 검사점 — 미정
- operation 어휘 1차 범위 (heap 단독 / +btree / +vacuum / +checkpoint) — 미정

**측정 이력**:
- 2026-09-03, SA 모드 reset 스파이크 — 전략 A 가 교차 실행 10,000 회에서 결정적(126 iter/sec).
  단, SA 는 단일 스레드라 대상 구성이 아니므로 게이트가 아니다 (requirements §5.2)
- 2026-09-03, SERVER_MODE in-process 기동 — 리스너 없이 뜨고 데몬 15 스레드 (requirements §5.3)
- **2026-09-04, 순서 공간 포화** — 위 문단. 엔진 수정 0 줄짜리 하네스 실험 하나가 엔진 패치와
  XASL 픽스처 설비를 *즉시 작업* 에서 내렸다 (requirements §5.3a)

**Open Questions**: requirements §9 참조.

**ADR 자리표시자**: ADR-EXT-009 *(트리거: incubating 정식 진입 시)*.
> **번호 주의**: `E8` / `ADR-EXT-008` 은 축 8 *Hybrid CI 통합* 자리로 이미 예약됨. 본 항목이
> `E9` 인 이유다. 사다리 순번(5)과 카탈로그 ID(9)는 별개 번호 공간.

**참조**:
- RocksDB fuzzing: <https://github.com/facebook/rocksdb/tree/main/fuzz>
- libprotobuf-mutator: <https://github.com/google/libprotobuf-mutator>
- Requirements: `extensions/E9-storage-fuzzing/requirements.md`

### E10 — XASL Fixture 생산·보관 (보조 설비)

**목표**: 질의로부터 XASL 픽스처를 만들어 보관하고, **버전을 식별하고**, 재생 가능한 형태로
내주는 설비.

**왜 필요한가**: **서버는 SQL 을 컴파일하지 않는다** — `libcubrid.so`(SERVER_MODE) 에
`parser_main` · `pt_compile` · `do_prepare_select` · `xts_map_xasl_to_stream` 이 없다.
컴파일과 직렬화는 클라이언트 몫이고 서버는 스트림을 받아 `stx_map_stream_to_xasl ()` 로
되돌린다. 즉 **소비 절반은 엔진에 있고 생산·보관 절반은 트리 어디에도 없다.**

**왜 독립 항목인가**: 소비자가 E9 하나가 아니다 — 플랜 회귀 비교, 플랜 안정성 검증이 같은
자산을 원한다. E3 와는 **생산자–소비자** 관계다: E3 가 질의를 만들고, E10 이 픽스처로
굳히고, E9 가 재생한다.

**핵심 요구 — 버전 거부**: `stx_map_stream_to_xasl ()` 은 포인터 non-null 과
`xasl_stream_size > 0` 만 확인하고 **포맷·버전을 전혀 검사하지 않는다.** 다른 빌드의
스트림은 거부되지 않고 의미가 달라진 오프셋으로 역직렬화된다 — 실패가 아니라 *조용한
오작동* 이다. 빌드 식별자를 픽스처에 기록하고 불일치 시 거부하는 것이 본 항목의 핵심이며,
엔진이 아니라 **픽스처 계층에서** 막는다.

**Phase 정합 (즉시 후보)**: 엔진 변경이 없고 선결 의존도 없다. E9 Tier 2 의 선결이면서
독립 실행 가능.

**스코프 (incubating)**: 생산 경로(csql / CCI / JDBC) · 픽스처 포맷 · 빌드 식별자 정의 ·
보관 위치(NG1) · 질의 1차 목록 — 모두 미정.

**ADR 자리표시자**: ADR-EXT-010.

**참조**: `extensions/E10-xasl-fixtures/requirements.md`

### §6a 부록 — 엔진 강건성 fuzzing 우선순위 사다리

fuzzing 계열 항목(E3·E5·E9)과 *아직 카탈로그에 없는* 후보를 하나의 우선순위로 정렬한
사다리. 카탈로그 ID 와 **별개 번호 공간** 이다 — 사다리는 *착수 순서*, 카탈로그는 *항목 식별*.

| 순위 | 원안 | 대상 | 도구 | 카탈로그 매핑 | 상태 |
|---|---|---|---|---|---|
| 1 | 1 | SQL parser | libFuzzer + ASan/UBSan | **E5** | 착수 가능 — `-DENABLE_FUZZING` 이 2026-09-03 부터 존재 |
| 2 | 3 | SQL correctness | SQLancer | **E3** | **진행 중** (ADR-EXT-003 Accepted) |
| 3 | 4 | network packet decoder | libFuzzer | **E5** (CCI/JDBC target) | 착수 가능 — 1 과 같은 선결이 해소됨 |
| 4 | 5 | record serialize / unpack | libFuzzer | **E5** (target 추가 — `or_get_value`) | **완료 (2026-09-03)** — 빌드·실행되고 첫 실행에서 결함 검출 |
| 5 | 6 | **storage concurrency** — Tier 1(반복 실행) / Tier 2(재생) | libFuzzer + schedule 주입(FI) | **E9** | **Tier 1 가동 중 (2026-09-04)** — 오라클 완비 · Tier 2 는 **보류** (포화 측정) |
| 6 | 7 | recovery / crash | 별도 crash·stress framework | *미등록* — E7 / `cluster-sandbox` 와 경계 정리 필요 | 사다리에만 기재 |
| 7 | 8 | concurrency (SQL·isolation 레벨) | schedule / model-based fuzzing | *미등록* — 축 4 isolation 모듈과 경계 정리 필요 | **storage 내부 부분은 순위 5(E9)로 이관** (2026-09-03). 남은 것은 SQL 레벨 |

**사다리의 의미**: **1·3·4 는 모두 E5** — *같은 선결 조건 하나*(`-DENABLE_FUZZING`)만
풀리면 순차 착수 가능하고 인프라(corpus·triage·coverage)를 공유한다. **2 (E3) 는 계열이
다르다** — SQLancer 는 Java 로직버그 도구라 `-DENABLE_FUZZING` 도 libFuzzer 인프라도
필요 없고 이미 진행 중이다. 사다리에는 *상대적 우선순위를 기록하기 위해서만* 올린다.
**5 (E9)** 는 1·3·4 의 인프라 위에 *스케줄 제어* 라는 다른 축이 얹히므로 그 다음이다. 6·7 은 도구 계열 자체가 달라 별도 판단이 필요하며, 지금은 *등록만* 하고 확장
항목으로 신설하지 않는다.

**제외 기록**: 원안 2행 *Spatial WKT/WKB (libFuzzer)* 는 **2026-09-03 사용자 결정으로
사다리에서 제외** — 현재 대상이 아니다. spatial 축은 별도 트랙(`spatial-stack`,
engine-suite `feat/spatial-probes` 브랜치)에 귀속되며, 그 트랙이 재개될 때 사다리에 다시
올릴지 판단한다.

**미등록 2건(순위 6·7)의 성격**:
- *순위 6 (recovery/crash)* — coverage-guided fuzzing 이 아니라 **fault injection + 재기동
  검증**. `cluster-sandbox` 가 이미 노드 재기동·split-brain·lag 주입을 다루고, E7 이
  long-running invariant 를 다룬다. 신설 전에 **C-004 책임 경계** 결론이 필요하다.
  **다만 그린필드가 아니다 (2026-09-03 확인)** — 엔진에 `src/base/fault_injection.c` 가
  51 개 호출 지점(btree · disk_manager · file_io · log_manager)과 함께 이미 있고,
  `#if !defined (NDEBUG)` 게이트 + 시스템 파라미터(`fault_injection_ids` /
  `fault_injection_test`)로 동작하며, CTP 의 `rqg_init.sh` 가 이미
  `fault_injection_test=recovery` 를 설정한다. 즉 이 항목은 "만든다" 가 아니라 "기존 FI 에
  하네스를 붙인다" 이고, 기록해 둔 것보다 싸다.
- *순위 7 (concurrency)* — schedule/model-based (systematic interleaving 탐색). **2026-09-03
  재분할**: storage 엔진 내부의 인터리빙은 순위 5(E9)가 가져갔고, 여기 남은 것은 **SQL·
  isolation 레벨** 이다. 축 4 (isolation) 의 *단일노드 다중 클라이언트* 와 대상이 겹치지만
  *탐색 방식* 이 다르다. 현행 isolation 모듈 대체(Phase 3·4)가 끝나기 전에는 착수 근거가 약하다.

### §6a 카탈로그 출처 / 인덱스

| ID | 진입성 | 선결 | requirements |
|---|---|---|---|
| E1 | 즉시 후보 | — | extensions/E1-sqllogictest/ |
| E2 | 즉시 후보 | — | extensions/E2-sqlsmith/ |
| E3 | **진행 중 (동시 트랙)** | — (ADR-EXT-003 로 해소) | extensions/E3-sqlancer/ + `cubrid-sqlancer` 저장소 |
| E4 | 조건부 | N24 / N11 graduation | extensions/E4-distributed-isolation/ |
| E5 | 조건부 | cubrid 본 repo `-DENABLE_FUZZING` | extensions/E5-parser-fuzzing/ |
| E6 | 조건부 | N13 pg-wire-compat selected 이상 | extensions/E6-differential/ |
| E7 | 조건부 | C-004 책임 경계 정의 | extensions/E7-workload/ |
| E9 | 조건부 | **E5 선행** + SERVER_MODE in-process 기동(확인됨) + **E10** | extensions/E9-storage-fuzzing/ |
| E10 | 즉시 후보 | — (엔진 변경 없음) | extensions/E10-xasl-fixtures/ |

근거: `survey/dbms-testing-ecosystem.md` (8축 분류, §11 카탈로그 확장 후보).

> `E8` 은 축 8 *Hybrid CI 통합* 의 메타 자리로 예약되어 있어 비어 있다 (`extensions/README.md` 카탈로그 참조). E9 가 E8 을 건너뛴 이유다.
> 착수 순서는 위 표가 아니라 **§6a 사다리** 가 정한다.

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
| §6a-E5 cubrid 본 repo 의 fuzz target build option 미진척 | E5 incubating 정식 진입 시도 | testkit 단독 시작 금지; cubrid 본 repo PR (`-DENABLE_FUZZING` 등) 선결. **2026-09-03: 엔진 쪽 작업이 roadmap repo 에 `N66-fuzz-target-infrastructure` (00-pending-review) 로 등록되고 경계는 C-055 로 기록됨** — 이 행의 위험은 N66 의 status 결정까지 열려 있다 |
| §6a-E4/E6 선결 의존 (N24·N11·N13) 미진척 | E4·E6 incubating 진입 검토 시 | roadmap repo planning 과 동기화; 선결 graduation 전에 진입 시도 금지 |
| §6a 확장 영역이 strangler-fig 진척을 잠식 | 분기 게이트에서 Phase 4·5 지연 vs E1~E10 진척이 역전 | 분기 게이트 답변에 §6a 진척을 별 행으로 분리 기재; 우선순위 충돌 시 strangler-fig 우선. **E3 는 2026-09-02 사용자 결정으로 동시 트랙이며, 이 규칙은 자원 충돌이 실제 발생할 때 적용된다** |
| §6a-E9 의 재현성을 SERVER_MODE 에서 확보하지 못한다 | E9 incubating 진입 시 | **2026-09-03 정정**: SA 모드 스파이크로 "해소"라 적었던 것은 범위를 넘은 주장이었다. SA 는 단일 스레드이고 저장 계층은 모드별로 갈라진 코드다(`page_buffer.c` SERVER_MODE 분기 146 개). 멀티스레드에서 재현성은 상태를 되돌려서가 아니라 **스케줄을 입력에 포함시켜 재생** 함으로써 얻어야 한다 — 그 형태가 성립하는지는 미측정 |
| §6a-E9 의 protobuf 신규 의존이 cubrid 본 repo 3rdparty 정책에 막힘 | ADR-EXT-009 결정 시 | fuzz 빌드 전용 링크임을 명시(배포 산출물 불변). 거부 시 `FuzzedDataProvider` 경로로 fallback — 의존 0, mutation 품질 하락 감수 (E9 requirements §2.1) |
| §6a 사다리 순위 5(E9)를 순위 1·3·4(E5) 보다 먼저 시도 | 착수 순서 역전 | 인프라(corpus·triage·coverage) 중복 구축이 된다. E5 선행 원칙을 사다리에 명시 |
| §6a 사다리 순위 6·7 (recovery / concurrency) 이 경계 정리 없이 신설됨 | 확장 항목 신설 시도 | 순위 6 은 `cluster-sandbox`·E7 과, 순위 7 은 축 4 isolation 모듈과 겹친다. C-004 결론 전 신설 금지 — 사다리에 *등록만* |
| CUBRID 서버 코어(10~20GB)로 인한 디스크 고갈 | E3 실행 중 서버 크래시 반복 | 코어 기본 비활성(`ulimit -c 0`) + `$CUBRID/log/coredump` 스택으로 원인 파악 (ADR-EXT-003 C7) |

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
