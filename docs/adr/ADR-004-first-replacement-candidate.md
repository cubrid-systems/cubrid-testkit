# ADR-004: First Replacement Module (Strangler-Fig 1차 대체 단위)

- **Status:** Proposed (Phase 0 M0 출력. Phase 2 (아키텍처/모듈 설계) 완료 시점에 결정)
- **Date:** 2026-04-29
- **Trigger:** Phase 2 종료
- **Depends on:** ADR-001 (언어), ADR-002 (빌드)
- **Related:** ROADMAP §3 Phase 2 ~ Phase 4

---

## 1. Context

ROADMAP은 Strangler-fig 점진 대체를 채택했다. **새 시스템 cubrid-testkit 이 첫 번째로 대체할 단위** 를 결정해야 한다 (Phase 3 의 Exit 조건: 선정 모듈이 새 시스템에서 동작 + ctp.sh 호환 진입점에서 회귀 동등성).

M0 분석을 통해 *4개의 자연스러운 후보*가 식별되었다 (case-formats.md §5b 추가 후 5개로 갱신).

---

## 2. Decision Drivers

1. **1인 6-12개월 한도** — 6개월 안에 Phase 3 exit 가 가능한 *작동 단위* 여야 한다.
2. **외부 표면 동결 의무** — 1차 대체 모듈은 그 모듈에 해당하는 conf 키, CLI task 이름, 출력 포맷, 테스트 케이스 인터페이스가 *완전 동일* 해야 한다.
3. **회귀 동등성 검증 가능성** — 동일 testcases 입력에 대해 신/구 시스템 출력이 같은지 확인 가능 해야 한다 (testcases-private/private-ex 까지 확보됨, M0 #5).
4. **후속 모듈 대체 비용** — 1차 단위가 추출되면 *2차 이후 모듈* 대체가 쉬워져야 한다. 즉 1차는 *공통 인프라* 또는 *모범 사례* 가 되는 것이 이상적.
5. **신/구 공존 안정성** — 1차 대체 후 기존 CTP 의 다른 모듈이 *영향 없이* 계속 작동해야 한다.
6. **학습 곡선** — ADR-001 의 언어 선택과 결합. 새 언어로 처음 작성하는 코드는 *작은 단위* 가 안전.

---

## 3. Options Considered

### Option A: `shell.common.*` 추출 → "remote-exec" 공통 모듈

**범위:**
- shell 모듈의 11개 `common.*` 클래스 (SSHConnect, LocalInvoker, ScriptInput, ShellScriptInput, GeneralScriptInput, HttpUtil, CommonUtils, Constants, Log, GeneralFeedback, SyncException) 를 새 시스템 동등물로 작성.
- 새 시스템이 *원격 셸 명령 실행 + 로컬 프로세스 spawn + SSH/SFTP* 인터페이스를 제공.
- 기존 jar 들은 그대로 살아있고, *shell.common 만 새 시스템 라이브러리로 점진 교체*.

**규모 추정:**
- 11 Java 클래스 ≈ 1500~2500 LoC. 새 언어로 동등물 작성 시 다소 작아질 가능성 (Go: ssh/sftp 표준 라이브러리 풍부).

**Pros**
- ⭐⭐⭐⭐⭐ **최대 공통 인프라**. 한 번에 4 모듈 (shell, isolation, ha_repl, cdc_repl) + shell-format 케이스 다수 (shell_ext, shell_heavy, longcase, RQG, HA cases) 가 영향 권에 들어옴.
- 테스트가 명확 — SSHConnect 의 jsch 래퍼는 *입력 명령 → 출력 문자열* 로 단순한 인터페이스. 새 동등물의 회귀 테스트 작성 쉬움.
- 1차 대체 후 isolation/ha_repl/cdc_repl 의 Java 측은 *그대로 두고 새 SSH 라이브러리만 갈아 끼우는* 식의 점진 대체 가능 (단, 이는 strangler-fig가 아닌 *내부 라이브러리 교체*).

**Cons**
- ⚠️ **모듈 단위가 아닌 라이브러리 단위 대체**. 엄밀한 strangler-fig는 *동등 모듈*을 새 시스템 안에 만든다. 라이브러리만 교체하면 "기존 시스템의 일부가 새 시스템의 일부를 호출"하는 *cross-system 의존*이 생김 — 운영 복잡.
- 사용자가 보기에 *새 시스템의 가시적 산출물이 없음*. Phase 3 exit 의 *회귀 동등성* 입증이 모호 (어떤 케이스의 어떤 출력이 새 시스템 책임인지 경계가 흐림).
- 기존 jar가 새 라이브러리를 호출하려면 *언어간 IPC* 필요 (ADR-001이 Java 외 언어인 경우 = subprocess 또는 gRPC).

**Phase 3 Exit 가능성:** 어려움 — 회귀 동등성 단위가 모듈이 아닌 함수.

---

### Option B: `sql` 모듈 단독 대체

**범위:**
- `sql/bin/run.sh` (917 라인) + `cqt/*` (Java 71 파일) 의 책임을 새 시스템이 그대로 흡수.
- ConsoleAgent 의 외부 stdout 마커 (`Result Root Dir:`, `total:`, `success:`, ...) 와 `<resultDir>/main.info` 포맷 동결.
- 새 시스템이 `ctp.sh sql -c sql.conf` 호출을 받아 동등 결과를 내야 함.

**규모 추정:**
- 셸 917 라인 + Java 약 5000~8000 LoC = 매우 큼.

**Pros**
- ⭐⭐⭐ **모듈 자율성 최강**. shell.common.* 의존 없음 (sql/design.md §6). 단독 대체로 cross-system 의존이 깔끔.
- 회귀 동등성 검증 명확 — 같은 conf + 같은 testcases-public/sql 입력에 대해 새/구 main.info 비교.
- *가장 사용 빈도 높은 모듈* (testcases 17,411 .sql) — 가치가 큼.
- ADR-002 의 *셸 + Java 분리 모델* 을 새 시스템에서 어떻게 다룰지 (셸 보존 / 모두 흡수) 에 대한 입력 사례가 됨.

**Cons**
- ⚠️ **분량이 가장 큼**. 1인 6-12개월 호라이즌의 거의 전체를 sql 한 모듈에 쓸 가능성.
- CCI 모드 의존 (`sql_by_cci/ccqt` C 바이너리) — 1차에서 다룰지 우회할지 결정 필요.
- 35개 console/util 클래스의 deep dive 비용 미지수.
- *처음 작성하는 코드*로는 *너무 큼* — 1차에 실패 위험.

**Phase 3 Exit 가능성:** 가능하지만 일정 위험 — 6-12개월 한도에서 sql 단독에 모든 시간을 쓸 가능성.

---

### Option C: `shell` + `isolation` 묶음 (혹은 `shell` 단독)

**범위:**
- shell 모듈의 35 Java 파일 + isolation 의 17 Java 파일 + ctltool native C 처리.
- shell.common.* 가 자연스럽게 함께 흡수됨 (Option A 의 결과를 포함).
- ha_repl/cdc_repl 은 *기존 shell.jar 의 shell.common.* 의존이 끊어지므로 1차 작업에서 함께 새 시스템으로 옮기거나 *어댑터 layer* 필요.

**규모 추정:**
- shell.jar (35 파일 ≈ 4000~6000 LoC) + isolation.jar (17 파일 ≈ 1500~2500) + ctltool 처리 = 매우 큼.

**Pros**
- ⭐⭐⭐⭐ **strangler-fig 의 정통 패턴**. 모듈 경계가 명확하므로 회귀 동등성 단위가 깔끔.
- shell.common.* 가 자동 흡수되므로 ha_repl/cdc_repl 도 동시에 대체 *후보*가 됨.
- shell-format 케이스가 다중 suite (shell, shell_ext, shell_heavy, shell_perf, longcase, HA) 에서 사용되므로 1차 대체의 *영향 범위가 가장 큼*.

**Cons**
- ⚠️ **isolation 의 native C ctltool 동반 처리** — 새 시스템이 ctltool 파서를 *흡수* (FFI / 재작성) 또는 *외부 subprocess 로 호출* 결정 필요.
- shell의 *3가지 모드* (SSH 클라이언트 / 로컬 / RMI 서버) 를 1차에 다룰지 분리할지 결정 부담.
- 분량은 sql 단독과 비슷하거나 더 큼 (1인 한도 위협).

**Phase 3 Exit 가능성:** 어려움 — shell 단독으로 축소하지 않으면 일정 초과.

---

### Option C': `shell` 단독 (옵션 C 의 축소판)

**범위:** 위에서 isolation 을 빼고 shell + shell.common 만.
- shell 모듈의 35 Java 파일.
- shell-format 케이스들 회귀 동등성 검증.
- isolation/ha_repl/cdc_repl 은 *어댑터*로 기존 shell.jar 를 그대로 사용 (shell.common.* 만 새 시스템이 제공하는 동등물로 호출하는 thin shim).

**Pros**
- 분량 축소 — 4000~6000 LoC 정도로 1인 6개월 안에 가능.
- shell-format 케이스 (가장 많은 suite) 가 동시에 영향 권.

**Cons**
- 어댑터 layer 작성 비용 (작지 않을 수 있음 — Java reflection 트릭 또는 jar repackaging).

---

### Option D: `webconsole` 단독 — 가장 작은 단위

**범위:**
- `cqt/webconsole/*` (Jetty 기반 웹 UI, 7 파일 + compare 3 파일).
- `executeWebConsole(start|stop)` utility 분기를 새 시스템이 처리.
- 모든 케이스 실행 로직과 분리됨 (webconsole은 *결과를 시각화* 만 함).

**규모 추정:** 1500~3000 LoC + Jetty 자산.

**Pros**
- ⭐⭐⭐⭐ **가장 작은 단위 — 1인 첫 모듈로 안전**. 새 언어 학습 + 배포 파이프라인 수립의 검증 슬라이스로 적합.
- 케이스 실행 경로와 *완전 분리* — 회귀 위험 거의 없음.
- 모던 UI 도입 기회 (예: React + Go API).

**Cons**
- ⚠️ **strangler-fig 의 *작동 단위*가 아님** — webconsole 은 utility, 핵심 가치(케이스 실행 → 결과 보고) 영역 밖.
- 1차 대체 후 *공통 인프라가 형성되지 않음* → 2차 이후 모듈이 처음부터 다시 시작.
- Phase 3 Exit 의 "회귀 동등성" 정의가 약함 (UI는 시각적 검증).

**Phase 3 Exit 가능성:** 매우 빠름. 단, 의미 약함.

---

### Option E: `RQG 래퍼` 단독

**범위:**
- CTP.executeShell 의 RQG 분기 (`TEST_CATEGORY=rqg`) 와 testcases-private/random_query_generator/ 케이스를 다루는 thin layer.
- 외부 OSS RQG 프레임워크 호환 (.yy/.zz) 보존.

**규모 추정:** 작음 — RQG 자체는 외부 도구이므로 새 시스템은 *invocation wrapper* 만.

**Pros**
- ⭐⭐⭐ 매우 작음. 1차 검증 슬라이스로 빠른 진행.
- 외부 표준 (.yy/.zz) 호환만 신경쓰면 되므로 표면 동결 의무 작음.

**Cons**
- 핵심 가치 영역 밖. 학습된 인프라가 sql/shell 등 본 영역에 직접 적용되지 않음.
- 1인 운영자에게 *흥미가 적은* 단위 (단조로운 wrapper).

---

## 4. Comparison Matrix

| 기준 | A. shell.common 추출 | B. sql 단독 | C. shell+isolation | C'. shell 단독 | D. webconsole | E. RQG |
|------|---------------------|-------------|--------------------|----|-----|-----|
| 분량 (1인 한도 fit) | ⭐⭐⭐⭐ | ⭐⭐ | ⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| Strangler-fig 정합성 | ⭐⭐ (lib 교체) | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐ | ⭐⭐ |
| 영향 범위 (커버 모듈) | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐ | ⭐ |
| 회귀 동등성 명확성 | ⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐ | ⭐⭐⭐ |
| 후속 모듈 대체 용이 | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐ | ⭐⭐ |
| 1차 작업 위험 | ⭐⭐⭐ | ⭐⭐ | ⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| 신/구 공존 안정성 | ⭐⭐ (cross-system call) | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |

---

## 5. Recommendation Tree

### 사용자 목표가 *학습/검증 우선* 이라면:

**1순위: D (webconsole)** — 1차로 새 언어/빌드/배포 파이프라인 검증.
**2순위: E (RQG 래퍼)** — 역시 작음.
- 후속에 A 또는 C' 진입.

### 사용자 목표가 *strangler-fig 정통 + 영향 범위 우선* 이라면:

**1순위: C' (shell 단독)** — 정통 모듈 단위, 분량 ≤ sql, 후속 isolation/ha_repl 대체 비용 큰 폭 감소.
**2순위: B (sql 단독)** — 자율성 최강, 회귀 검증 가장 깨끗, 단 분량 큼.
- A 는 단독 1차로는 약함 (작동 단위 부재) — C' 또는 B 선택 후 *사실상 함께 진행* 됨.

### 사용자 목표가 *공통 인프라 우선 + 점진적 라이브러리 교체* 라면:

**1순위: A (shell.common.* 추출)** — 4 모듈 동시 영향, 단 작동 단위가 부재하므로 *strangler-fig가 아닌 internal refactor* 로 간주해야 함.
- 이 경우 ROADMAP 의 Phase 3 정의를 *회귀 동등성* 대신 *라이브러리 교체 후 회귀 안정* 으로 재해석.

### 균형형 추천 (작성자의 1인 한도 + 정통 strangler-fig 균형):

**최우선 후보: Option C' (shell 단독)**
- 모듈 경계 명확 + shell.common.* 자동 흡수 + shell-format 케이스 다수 영향.
- 분량 적정 (≈ 5000 LoC). 6개월 안에 Phase 3 Exit 가능.
- isolation/ha_repl/cdc_repl 의 어댑터 layer 작성 비용은 추가지만, 어댑터 자체가 좋은 학습.

**Backup: Option D (webconsole)**
- C' 에 진입하기 전에 *0.5개월 분량의 학습 슬라이스* 로 webconsole 을 먼저 옮기는 안.
- 1차 = D (학습/배포 검증), 2차 = C' (본격 strangler-fig).
- 이는 ROADMAP의 Phase 0 → Phase 3 진행을 *D-then-C'* 의 2단계로 조정하는 것을 의미.

---

## 6. Open Questions

- **사용자가 1차 산출의 가시성을 얼마나 중시?** 가시성 우선이면 C' 또는 B; 학습 우선이면 D 또는 E.
- **신/구 공존 기간의 기존 jar 처리 정책** — 어댑터 layer 작성을 받아들일 수 있는가? (C' 가 어댑터 의존)
- **ctltool native 자산** 처리 — 1차에서 isolation 을 함께 다루지 않더라도, 어댑터가 ctltool subprocess 호출을 어떻게 다룰지 결정 필요.
- **CCI 모드 (sql_by_cci/ccqt)** 처리 — sql 1차이면 즉시, 아니면 후순위.
- **ADR-001 의 언어** — 학습 곡선이 큰 언어(Rust)를 골랐다면 D (webconsole)을 1차로 권고. 친숙한 언어(Java/Kotlin/Go)면 C' 가능.

---

## 7. Decision

(미정. Phase 2 (아키텍처 + 모듈 설계) 의 종료 시점에 결정. 그 시점엔 ADR-001/002 가 결정되어 있어야 한다.)

## 8. Consequences (결정 후 채움)

- Phase 3 의 작업 범위 확정
- 어댑터 layer 필요성 / 설계
- 신/구 공존 기간의 운영 정책
- 후속 모듈 대체 순서 (Phase 4 의 입력)
