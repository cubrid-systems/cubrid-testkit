# ROADMAP — CUBRID Test Kit

- **날짜**: 2026-04-28
- **전략**: Strangler-fig 점진 대체 (1인 사이드 프로젝트, 6~12개월 호라이즌)
- **제약 요약**: 기존 testcases 레포 수정 불가 / 외부 인터페이스 동결 / 빌드 도구 미확정(ADR-002)
- **Analysis baseline**: cubrid-testtools @ 86992c1b334d55800f2700d60f9809c2ceca268d

---

## 0a. 레포지토리 전략

```
cubrid-testtools/         (기존, 동결 대상)
├── CTP/                   살아 있음 — Phase 0 ~ 4 동안 지속 가동
└── ROADMAP.md             고수준 의도만 유지, 상세는 새 레포로

cubrid-testkit/            (신규, 이번 작업의 결과물)
├── README.md              "CTP의 후계 — strangler-fig 진행 중" 명시
├── ROADMAP.md             이 명세에서 추출한 상세 로드맵
├── analysis/              Phase 0 산출물 (브라운필드 분석)
│   ├── _overview/         CLI 트리, conf 매트릭스, case 포맷 분포
│   ├── medium/  sql/  shell/  isolation/  common/   (심도 5종)
│   └── inventory/         jdbc/sql_by_cci/ha_repl/cdc_repl/cci_compat
├── concept/               Phase 1 — 컨셉/외부 표면 동결
├── design/                Phase 2 — 아키텍처/모듈 설계
├── impl/                  Phase 3+ — 모듈별 구현
├── adr/                   ADR-001 .. ADR-NNN
└── docs/                  최종 사용자 문서 (기존 doc/의 후계)
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
```

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
- ADR-001 *(트리거: M0 종료)* — 새 시스템의 구현 언어
- ADR-002 *(트리거: M0 종료)* — 빌드 도구 (Ant 유지 / Maven / Gradle / 비-JVM)
- ADR-003 *(트리거: 분석 종료)* — 외부 표면 동결 시점

---

## 2. Phase 1 — 신시스템 컨셉 + 외부 표면 동결 (1개월)

**Exit 조건**: 새 시스템이 *어떤 입력*을 받아 *어떤 출력*을 내야 하는지가 기존 CTP와 1:1 매핑된 표가 존재.

**산출물**:
- `concept/north-star.md` — 한 페이지로 새 시스템의 정체성 (모던 설계 / 확장성 / 호환성의 구체적 의미)
- `concept/external-surface-freeze.md` — *동결되는* CLI 인자, conf 키, 출력 파일, 종료 코드 명세
- `concept/non-goals.md` — 의도적으로 *재현하지 않는* 동작 목록 (이전엔 가능했지만 새 시스템에선 의도적으로 다르게 처리)
- ADR-001 / ADR-002 / ADR-003 확정본

---

## 3. Phase 2 — 아키텍처 + 모듈 설계 (1~2개월)

**Exit 조건**: "의존이 가장 적은 모듈 1개"가 단독으로 빌드/실행 가능한 새 시스템 골격이 결정됨.

**산출물**:
- `design/architecture.md` — 모듈 경계, 공통 레이어, 실행 모델(분산 실행/SSH 호환), 결과 처리 파이프라인
- `design/module-{module}.md` × 4 — 신구 매핑 표 포함 (구 클래스/스크립트 → 신 컴포넌트)
- `design/contracts.md` — 모듈 간 in-process 인터페이스 (이후 strangler-fig 대체의 경계)
- ADR-004 — 신시스템 골격이 적용될 *첫 번째 대체 대상 모듈* 선정 (1차 후보: isolation 또는 medium 중 의존이 적은 쪽)

---

## 4. Phase 3 — 1차 strangler-fig 대체 (2~3개월)

**Exit 조건**: 선정된 모듈이 새 시스템에서 동작하며, `bin/ctp.sh` 호환 진입점에서 기존 결과와 **회귀 동등성** 확인.

**산출물**:
- `impl/m1/` — 첫 모듈 구현 코드
- `impl/m1/migration-bridge.md` — 기존 ctp.sh가 새 구현으로 라우팅되는 방식
- `impl/m1/regression-evidence.md` — 동일 testcases 입력에 대한 신/구 출력 동등성 보고서
- ADR-005 — 신/구 공존 기간 동안의 *유지보수 정책*

---

## 5. Phase 4 — 2~3차 대체 + 인벤토리 모듈 통합 (2~3개월)

**Exit 조건**: 심도 4개 모듈이 모두 새 시스템에서 동작. 인벤토리 모듈은 *새 시스템 위의 얇은 어댑터*로 동작 가능.

**산출물**:
- `impl/m2/`, `impl/m3/`, `impl/m4/` — 모듈 단위 구현
- `impl/adapters/{inventory_module}.md` × N — 인벤토리 모듈에 대한 *호환 어댑터* 명세 (재작성 아님)
- ADR-006 — 인벤토리 모듈 중 *재작성 대상*과 *어댑터 유지 대상* 분류

---

## 6. Phase 5 — 잔여 정리 + 폐기 결정 (1개월)

**Exit 조건**: 기존 CTP 디렉터리에서 *더 이상 호출되지 않는 코드*가 식별되고 격리됨.

**산출물**:
- `cleanup/dead-code-inventory.md`
- `cleanup/legacy-archive-policy.md` — 기존 CTP 트리를 어떻게 보존/축소/제거할지
- `RETROSPECTIVE.md` — 6~12개월 회고

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

---

## Phase 0 분석 stub 체크리스트

Phase 0 진행 상황을 추적합니다. 각 항목은 `analysis/` 트리의 스텁 파일에 대응합니다.

### _overview (4)
- [x] analysis/_overview/cli-tree.md
- [x] analysis/_overview/conf-matrix.md
- [ ] analysis/_overview/case-formats.md
- [x] analysis/_overview/deps-of-common.md

### medium (5)
- [ ] analysis/medium/requirements.md
- [ ] analysis/medium/design.md
- [ ] analysis/medium/implementation-notes.md
- [ ] analysis/medium/io-contract.md
- [ ] analysis/medium/test-corpus.md

### sql (5)
- [ ] analysis/sql/requirements.md
- [ ] analysis/sql/design.md
- [ ] analysis/sql/implementation-notes.md
- [ ] analysis/sql/io-contract.md
- [ ] analysis/sql/test-corpus.md

### shell (5)
- [ ] analysis/shell/requirements.md
- [x] analysis/shell/design.md
- [ ] analysis/shell/implementation-notes.md
- [ ] analysis/shell/io-contract.md
- [ ] analysis/shell/test-corpus.md

### isolation (5)
- [ ] analysis/isolation/requirements.md
- [x] analysis/isolation/design.md
- [ ] analysis/isolation/implementation-notes.md
- [ ] analysis/isolation/io-contract.md
- [ ] analysis/isolation/test-corpus.md

### common (5)
- [ ] analysis/common/requirements.md
- [ ] analysis/common/design.md
- [ ] analysis/common/implementation-notes.md
- [ ] analysis/common/io-contract.md
- [ ] analysis/common/test-corpus.md

### inventory (5)
- [ ] analysis/inventory/jdbc.md
- [ ] analysis/inventory/sql_by_cci.md
- [ ] analysis/inventory/ha_repl.md
- [ ] analysis/inventory/cdc_repl.md
- [ ] analysis/inventory/cci_compat.md
