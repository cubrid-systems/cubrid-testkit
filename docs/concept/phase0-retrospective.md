# Phase 0 회고 — Phase 0 → Phase 1 전이 게이트 *(통과 기록)*

**Date:** 2026-04-29 (게이트 작성) / 2026-09-02 (통과)
**Status:** **PASSED** — Phase 1 진입 완료
**Analysis baseline:** cubrid-testtools @ 86992c1b334d55800f2700d60f9809c2ceca268d

> 본 문서는 `PHASE0_EXIT.md` 였다. ROADMAP §10 의 예정대로 Phase 1 진입 후
> `concept/phase0-retrospective.md` 로 이름을 바꿔 *진행 기록* 으로 보존한다.
> §9 의 "다음 액션" 과 §7 의 체크리스트는 **§11 에 기록된 실제 결정** 으로 대체되었다.

---

## 1. 게이트의 의미

ROADMAP §1 의 Phase 0 (기존 시스템 분석) 의 Exit 조건을 모두 충족했는지 확인하고, **Phase 1 (컨셉 + 외부 표면 동결) 진입에 필요한 결정** 을 사용자가 내리는 시점.

Phase 0 산출물은 *분석 기반 (evidence base)* — 모든 *결정* 은 Phase 1 진입 시점에 사용자가 직접 내려야 함.

---

## 2. Phase 0 산출물 체크리스트 ✅

### 2-1. ROADMAP Phase 0 ActionableSlice (M0) — 5/5 ✅

| # | 작업 | 산출물 | 커밋 |
|---|------|--------|------|
| 1 | bin/ctp.sh + CTP.main 호출 트리 | `_overview/cli-tree.md` | `3fc7260` |
| 2 | conf 13개 파일 키 매트릭스 | `_overview/conf-matrix.md` | `f3627aa` |
| 3 | common 8개 하위 의존 그래프 | `_overview/deps-of-common.md` | `7947f5d` |
| 4 | 4개 deep 모듈 Main 시퀀스 | `{isolation,shell,sql,medium}/design.md` | `99deeba`/`3ac39ad`/`5c0609f`/`b918005` |
| 5 | testcases 케이스 포맷 분포 | `_overview/case-formats.md` | `c3e9621` + `d09569d` |

### 2-2. 4 deep 모듈 × 5 stubs — 20/20 ✅

| 모듈 | requirements | design | implementation-notes | io-contract | test-corpus |
|------|:---:|:---:|:---:|:---:|:---:|
| isolation | ✅ | ✅ | ✅ | ✅ | ✅ |
| shell | ✅ | ✅ | ✅ | ✅ | ✅ |
| sql | ✅ | ✅ | ✅ | ✅ | ✅ |
| medium | ✅ | ✅ | ✅ | ✅ | ✅ |

### 2-3. common 5 stubs — 5/5 ✅

requirements / design / implementation-notes / io-contract / test-corpus 모두 채워짐 (`22e4717`).

### 2-4. inventory 5 stubs — 0/5 ⏸ (낮은 우선순위, 보류)

`jdbc / sql_by_cci / ha_repl / cdc_repl / cci_compat` 의 표층 인벤토리. 사용자 결정으로 *우선순위 낮음* — Phase 1 시작 후 필요 시 채움.

⚠️ 단, case-formats.md §5b/§5c 가 *입력 자료* 로 이미 풍부 — Phase 1 어디서든 참조 가능.

### 2-5. ADR drafts — 3 신규 + 4 정밀 분석에서 ADR 후보 5개 ✅

기존:
- ADR-000 (repo name) — Accepted (`e86a891`)
- ADR-001 (구현 언어) — Proposed (`4ae5137`)
- ADR-002 (빌드 도구) — Proposed
- ADR-004 (1차 대체 모듈) — Proposed

Phase 4 정밀 분석에서 새로 도출된 *ADR 후보 5개* (`dd37383`):
- ADR-005 (orphan ComponentEnum 폐기)
- ADR-006 (DB setup recipe 정형화)
- ADR-007 (ctltool 처리: subprocess vs 흡수 vs CUBRID-only)
- ADR-008 (.ctl DSL grammar 정형화)
- ADR-009 (result normalization 정형화)

→ **총 9 ADR 가 Phase 1 진입 시점의 결정 매트릭스**.

### 2-6. 정밀 분석 (Phase 4) — 4/4 ✅

| 분석 | 산출물 |
|------|--------|
| 7 orphan ComponentEnum 사용 흔적 | `_overview/orphan-enums.md` |
| make_sql_db_data vs make_db_data | `sql/db-setup-functions.md` |
| ctltool .ctl native parser grammar | `isolation/ctl-grammar.md` |
| ConsoleBO.runTest 35 utility 협업 | `sql/cqt-deep-dive.md` |

---

## 3. M0 누적 발견 — Phase 1 입력의 핵심

### 3-1. CTP 의 *내부 구조* 분석 결과

**CLI dispatch (cli-tree.md):**
- 21 ComponentEnum 중 14 active dispatch + 7 silent orphans
- 3가지 호출 패턴 혼재: shell-out (sql/jdbc) / reflection (shell/isolation/ha_repl/cdc_repl/unittest) / utility (webconsole)
- shell.jar 가 isolation/ha_repl/cdc_repl/unittest 의 공통 인프라 호스트

**Conf 구조 (conf-matrix.md):**
- 81 unique 키, `scenario` 만이 거의 모든 conf 에 공통
- 3개 자연 cluster: SQL/data, Shell/process, Replication
- shell_ci 의 14 exclusive 키 = CI 오버레이 정체성

**Common 의존 (deps-of-common.md):**
- 모듈→common 의존이 좁음 (4-7 심볼)
- coreanalyzer / ShareMemory / ctp 패키지 / sched 등 의 *common 거주는 빌드 트릭*
- third-party JAR 모두 노후 (2007~2016)

**모듈별 Main 시퀀스 (4 design.md):**
- shell.common.* 가 4 모듈 (shell + isolation + ha_repl + cdc_repl) 의 *진짜 공통 인프라* — strangler-fig 의 1차 후보
- sql 은 가장 자율적 (cqt 자체 SSH 스택)
- medium 은 sql 의 suite 변형 (확정)
- isolation 은 native C ctltool 동반 의존 (대체 비용 큼)

**케이스 형식 (case-formats.md):**
- 모듈마다 케이스 형식 다름 (`.sql` / `.ctl` / `.sh`)
- shell-format 이 가장 광범위 (shell + HA + RQG + shell_ext + shell_heavy + longcase + manually)
- testcases 3 레포 모두 접근 확보 (HA/interface/longcase/shell_ext + 공개 + private-ex)

### 3-2. Phase 4 정밀 분석 결과

**Orphan enums (orphan-enums.md):**
- 7 enum (CCI/DOTS/NBD/SYSBENCH/TPCC/TPCW/YCSB) 모두 진짜 dead
- 새 시스템에서 *grace period 후 제거* 권고 (ADR-005)

**DB setup (db-setup-functions.md):**
- make_sql_db_data + make_db_data 는 *대안이 아닌 보완* (SP 적재 vs 데이터 적재)
- `make_db_data` 의 *tar 명령 hardcode 버그* (`tar -zxvf mdb.tar.gz`) — site suite 를 깨뜨림
- 새 시스템에서 *suite recipe* 로 정형화 (ADR-006)

**ctltool (ctl-grammar.md):**
- 2-binary 아키텍처 (qactl MC + qacsql Client)
- 다중 DBMS 호환 (CUBRID/MySQL/Oracle) — 현 활성 여부 확정 필요
- 8 DSL 토큰 + Unix socket IPC + 10+ sed normalization
- 새 시스템에서 *Option B (subprocess 호출)* 가 1차에 안전 (ADR-007)

**cqt deep dive (cqt-deep-dive.md):**
- 9-phase pipeline + 35-class 협업
- Answer matrix 알고리즘 발견 (runMode + runModeSecondary 우선순위)
- SQL 안 다중 connection 지원 (--@<connId>) — isolation 와 부분 중첩
- pragma 시스템 (--+ holdcas / --@queryplan / --@<connId>) 은 동결 표면

---

## 4. Phase 1 진입 *전제 결정* — 9 ADR 의사결정 매트릭스

Phase 1 (concept + 외부 표면 동결) 에 진입하려면 다음 결정이 *최소한 잠정적으로* 필요:

### 4-1. 핵심 결정 (Phase 1 시작 전 필수)

| ADR | 의사결정 | 입력 | 영향 |
|-----|---------|------|------|
| **ADR-001** | 구현 언어 | 사용자 친숙도 + M0 (shell/SSH 도미넌스) | 모든 후속 결정의 기반 |
| **ADR-002** | 빌드 도구 | ADR-001 종속 | CI / IDE / 의존 관리 결정 |
| **ADR-004** | 1차 대체 모듈 | M0 (shell.common.* 추출 / sql 단독 / shell+isolation 묶음 / webconsole) | Phase 2/3 의 작업 범위 |

→ 이 3개 결정 후 Phase 1 진입 가능. 나머지 6개는 *Phase 1/2 진행 중* 결정.

### 4-2. 후속 결정 (Phase 1/2 진행 중)

| ADR | 의사결정 | 시점 |
|-----|---------|------|
| ADR-005 | orphan ComponentEnum 폐기 정책 | Phase 1 (concept) |
| ADR-006 | DB setup recipe 정형화 | Phase 2 (아키텍처) |
| ADR-007 | ctltool 처리 (subprocess / 흡수 / CUBRID-only) | Phase 2 |
| ADR-008 | .ctl DSL grammar 정형화 | Phase 2 |
| ADR-009 | result normalization 정형화 | Phase 2 |

---

## 5. Phase 1 (Concept) 산출 권고

ROADMAP §3 Phase 1 의 산출물 (Exit 조건):
- `concept/north-star.md` — 한 페이지 새 시스템 정체성 (모던 설계 / 확장성 / 호환성의 *구체적 의미*)
- `concept/external-surface-freeze.md` — 동결 CLI / conf / 출력 / 종료 코드 명세
- `concept/non-goals.md` — 의도적 *재현 안 함* 목록
- `adr/ADR-001-implementation-language.md` 확정본 (Status: Accepted)
- `adr/ADR-002-build-tool.md` 확정본
- `adr/ADR-003-external-surface-freeze.md` 신규

→ 본 게이트 통과 후 Phase 1 의 *3개 concept 문서 + 3 ADR 확정* 이 다음 마일스톤.

---

## 6. 가능한 *3 가지 진입 시나리오*

### 시나리오 A — *큰 결정 먼저, 빠른 진입* (권장)

1. 사용자가 ADR-001 / ADR-002 / ADR-004 *지금 결정* (각 ADR draft 의 Recommendation Tree 참고)
2. Phase 1 즉시 시작 — concept/ 3 문서 + ADR 확정 작성
3. ADR-005 ~ 009 는 Phase 1 진행 중 자연스러운 시점에 결정

**적합 조건:** 사용자가 큰 방향을 결정할 의지가 있고, 후속 발견이 큰 결정을 뒤집지 않을 confidence 보유.

### 시나리오 B — *분석 더, 결정 나중* (보수적)

1. inventory 5 stubs (jdbc/sql_by_cci/ha_repl/cdc_repl/cci_compat) 채우기
2. 미해결 후속 (cqt deep dive 의 §10, ctl-grammar 의 §9, db-setup 의 §9 등) 정밀화
3. 그 후 ADR-001/002/004 결정

**적합 조건:** 큰 결정에 추가 데이터가 도움된다고 판단. 1인 사이드 6-12개월 한도 측면에서는 위험.

### 시나리오 C — *prototype-first* (실험적)

1. ADR-001 의 *상위 2 후보* (예: Go / Kotlin) 로 *각각 1주 prototype* 만들기
2. webconsole 또는 RQG wrapper 같은 *작은 작동 단위* 를 두 언어로 구현
3. prototype 결과로 ADR-001 확정

**적합 조건:** 1-2주 추가 시간 투자 가능 + 결정의 confidence 가 prototype 으로만 얻어질 때.

---

## 7. 게이트 통과 체크리스트

Phase 1 진입 직전 사용자가 *서면으로 답변* 하면 좋은 질문:

```
Q1. 사용자(1인 운영자)가 평소 가장 자주 사용하는 언어 1순위와 2순위는?
   (1순위)  ____________________
   (2순위)  ____________________

Q2. ADR-001 의 6 옵션 중 *현 시점 선호 1순위* 와 *왜*?
   ____________________________________________________________

Q3. ADR-004 의 6 옵션 중 *1차 대체 단위 선호 1순위* 와 *왜*?
   ____________________________________________________________

Q4. 새 시스템이 *기존 cubridqa-common.jar 를 호환 jar 로 산출* 해야 하는가?
   (예/아니오) + 이유 ___________________________________________

Q5. *ctltool native 자산* 을 새 시스템에서 어떻게 다룰 것인가?
   ( ) Option A 흡수 / 재구현
   ( ) Option B subprocess (현 자산 그대로 빌드 + 호출)
   ( ) Option C CUBRID-only (다중 DBMS 폐기)

Q6. *분기별 재평가 게이트* (ROADMAP §7) 의 첫 가동 시점은?
   __________ (예: 2026-Q3 종료)

Q7. inventory 5 stubs 채움이 Phase 1 진입에 *필수* 인가?
   ( ) 예 — 시나리오 B
   ( ) 아니오 — 시나리오 A 또는 C

Q8. prototype-first 시나리오 (C) 를 고려하는가?
   (예/아니오)
```

---

## 8. 수치 요약

```
산출 문서 수:      33 분석 문서 (29 stubs + 4 정밀)
                   +  4 ADR (000 + 001 + 002 + 004)
                   +  1 ROADMAP
                   +  1 README
                   =  39 markdown 파일

git 커밋 수:        ~20 (bootstrap → M0 #1~#5 → ADR drafts → 16 stubs → common 5 + 4 정밀)

총 분석 라인:        대략 8,000+ 줄 (markdown)

미해결 후속:
- inventory 5 stubs (선택)
- cqt 35 utility 정밀 (선택)
- ctltool runtime classpath 메커니즘 (관심)
- script/ + ext/ 외부 호출자 식별 (Phase 1 진입 권고 전 처리)
- common.SSHConnect vs shell.common.SSHConnect 통합 분석 (선택)
- HA suite + multi-DBMS ctltool 의 현 활성 여부 (관심)
```

---

## 9. 다음 액션

다음 중 하나를 선택:

**A. ADR 결정 → Phase 1 시작** (시나리오 A)
   - 사용자가 ADR-001/002/004 *결정 입력*
   - Phase 1 으로 즉시 진입
   - concept/ 3 문서 작성 + ADR 확정 시작

**B. inventory 채움** (시나리오 B 일부)
   - jdbc / sql_by_cci / ha_repl / cdc_repl / cci_compat 표층 인벤토리
   - 그 후 ADR 결정

**C. prototype 시작** (시나리오 C)
   - 사용자가 *1-2주 prototype 시간* 약속
   - ADR-001 후보 2-3개로 작은 작동 단위 (webconsole 또는 RQG wrapper) 구현

**D. 추가 정밀 분석**
   - script/ + ext/ 외부 호출자 식별
   - cqt 정밀 후속 (cqt-deep-dive.md §10)
   - ctl-grammar.md §9 의 미해결

**E. Phase 0 산출 retrospective**
   - 본 게이트 문서 + 모든 산출물의 *peer review*
   - 발견된 ad-hoc 패턴 / 이름 충돌 / typo 정리 ADR

각 시나리오의 *추정 소요 시간* (1인 사이드 기준):
- A: 1-2주 (concept + ADR 확정)
- B: 1주 (inventory) + 1주 (ADR) = 2주
- C: 2-4주 (prototype) + 1주 (ADR) = 3-5주
- D: 1주 (정밀) + 1주 (ADR) = 2주
- E: 0.5주 (review) + B/A 중 선택

---

## 10. 결론

```
Phase 0 = 완료 ✅
- 5 ActionableSlice (M0)
- 25 stubs (4 deep 모듈 × 5 + common × 5)
- 4 ADR drafts
- 4 정밀 분석
- 게이트 통과 체크리스트 준비

Phase 0 → Phase 1 게이트 = 사용자 결정 대기
- ADR-001 (언어)
- ADR-002 (빌드)
- ADR-004 (1차 모듈)
- 시나리오 A/B/C/D/E 중 선택

다음 트리거:
- 사용자가 시나리오 선택 + Q1-Q8 답변 → Phase 1 진입
```

본 게이트 문서는 Phase 1 시작 후 `concept/phase0-retrospective.md` 로 이름 변경 + Status 업데이트하여 *진행 기록* 으로 보존.


---

## 11. 게이트 통과 기록 *(2026-09-02)*

### 11-1. 선택된 시나리오

**시나리오 A — 큰 결정 먼저, 빠른 진입.** inventory 5 stubs(§2-4)와 정밀 후속(§8)은 채우지 않고 Phase 1 에 즉시 진입.

### 11-2. §7 체크리스트에 대한 실제 답변

| Q | 답변 |
|---|---|
| Q2. ADR-001 선호 | **Go** — ADR 권고 1순위 그대로. 근거는 ADR-001 §7-1 |
| Q3. ADR-004 선호 | **Option C' (shell 단독)** — 균형형 추천 그대로. 근거는 ADR-004 §7-2 |
| Q4. `cubridqa-common.jar` 호환 산출 의무 | **아니오.** "CLI/conf/출력만 동결" → ADR-003, NG5 |

⚠️ ADR-003 이 확정한 동결 범위는 사용자 답변보다 넓다 — *CLI/conf/출력* 에 **종료 코드·원격 실행 컨트랙트**를 더했다. 이는 ADR-003 §1/§4 의 저자 판단이며 사용자가 명시적으로 답한 것이 아니다. 종료 코드와 `runone.sh`/`init.sh` 컨트랙트를 빼면 회귀 동등성 판정이 불가능해지기 때문이다.
| Q5. ctltool 처리 | **미기록.** ADR-002 §7-1 이 `ctltool/Makefile` 유지(빌드 결정)를 확정했을 뿐이고, 흡수/subprocess/CUBRID-only 라는 *런타임 통합 형태*는 ADR-007 로 이월 |
| Q7. inventory 필수 여부 | **아니오** (시나리오 A) |
| Q8. prototype-first | **아니오** — Phase 3 자체를 언어 검증 슬라이스로 사용 (ADR-001 §7-3) |
| Q1 / Q6 | 미기록. Q6(분기 게이트 첫 가동 시점)은 다음 분기 게이트에서 확정 |

### 11-3. 결정된 ADR

| ADR | 결정 | Status |
|---|---|---|
| ADR-001 | Go | Accepted |
| ADR-002 | `go build` + `go.mod` + Justfile 메타 + `ctltool/Makefile` 유지 | Accepted |
| ADR-003 | 동결 범위 = CLI/conf/출력/종료코드/원격컨트랙트. jar·Java API 제외. F1/F2/F3/NF 4등급 | Accepted (신규) |
| ADR-004 | Option C' (shell 단독) — Phase 2 종료 트리거였으나 조기 결정 | Accepted |
| ADR-005 ~ 009 | 미결 — Phase 1/2 진행 중 결정 (§4-2 유지) | — |

### 11-4. Phase 1 산출물

- `concept/north-star.md` — 정체성. M1~M5 / 확장점 1개 / 호환성 정의 / 성공 기준 3개
- `concept/external-surface-freeze.md` — 동결 명세. §10 이 ROADMAP Phase 1 Exit 조건인 신↔구 1:1 매핑 표(22행)
- `concept/non-goals.md` — NG1~NG11 (NG3 은 결번 확인 대기)
- ADR-001/002/003/004 확정본

### 11-5. Phase 1 에서 새로 발생한 미해결 항목

`external-surface-freeze.md` §11 의 7건. 이 중 §11-1·§11-2 는 **Phase 2 진입 전**, §11-5·§11-6 은 **Phase 3 착수 전** 해소가 필수로 승격되었다 (ADR-003 Consequence 4, ADR-004 Consequence 4).

또한 **NG3 결번 확인** — ROADMAP·extensions 문서가 NG1·NG2·NG4 를 참조하지만 NG3 은 어디에서도 참조되지 않는다. 원 명세 확인 또는 결번 확정이 다음 분기 게이트 안건.


### 11-6. 후속 정정 *(2026-09-02, 같은 날 이후)*

Phase 1 산출물에 대한 별도 검토 패스(critic)와 `§11-1` 확인 작업에서 다음이 드러나 문서를 개정했다:

| 발견 | 영향 |
|---|---|
| **`cli-tree.md` 가 `ctp.sh` 한 갈래만 추적** — 실제 java launcher 는 15개 | `cli-tree.md` 부록 A 신설 (진입점 전수) |
| `jdbc/bin/run.sh` 가 **`shell.main.JdbcLocalTest`** 호출 | ADR-004 Phase 3 범위에 `jdbc` 추가 |
| `shell/init_path/run_shell.sh` = **두 번째 CLI 트리** (옵션 13개, 가이드 3곳에 문서화) | freeze §1-4 신설 |
| isolation `.ctl` 은 `cases/`·`answers/` 자매 디렉터리가 **없다** | freeze §3-1 정정 (치명) |
| `.ctl` DSL 은 4토큰이 아니라 **8토큰** | freeze §3-3 정정 (치명) |
| **`runone.sh` 의 sed normalization** 이 명세에서 통째로 누락 — 모든 isolation 판정이 여기 의존 | freeze §7-6 신설 (치명) |
| `conf/shell_agent.conf` 누락 | freeze §2-1 |
| answer variant 패턴 오기 + `.diff_1` / `.answer_WIN` 누락 | freeze §3-2 |
| **축 T / 축 O 분리** (사용자 결정) | `migration-exclusions.md` 신설, north-star §1a, freeze 전면 개정 |

**교훈:** Phase 0 의 "완료" 판정이 이르렀다. `cli-tree.md` 가 단일 진입점만 추적했다는 사실을 게이트 체크리스트가 잡지 못했다. 향후 Phase Exit 판정에는 *"이 분석이 커버하지 않은 영역을 한 문장으로 적으라"* 는 항목을 넣는다.
