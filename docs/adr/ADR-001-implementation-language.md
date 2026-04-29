# ADR-001: Implementation Language

- **Status:** Proposed (Phase 0 M0 출력. 결정은 Phase 1 컨셉 수립 시점)
- **Date:** 2026-04-29
- **Trigger:** Phase 0 M0 종료
- **Supersedes:** —
- **Inputs:** M0 분석 (cli-tree.md / conf-matrix.md / deps-of-common.md / 4개 모듈 design.md / case-formats.md)

---

## 1. Context

기존 CTP 는 Java 6+ + Apache Ant + 셸 스크립트의 혼합 시스템이다 (M0 발견):

- **Java 자산**: 8개 모듈에 약 130개 .java 파일, cubridqa-common.jar / cubridqa-shell.jar / cubridqa-isolation.jar / cubridqa-ha_repl.jar / cubridqa-cdc_repl.jar / cubridqa-cqt.jar / cubridqa-scheduler.jar 7개 jar 산출
- **셸 자산**: `bin/ctp.sh`, `sql/bin/run.sh` (917줄), `shell/init_path/init.sh`, `common/script/*` 다수, `common/ext/run_*.sh` (모듈별 외부 진입)
- **native 자산**: `isolation/ctltool/` 의 C 파서 + Makefile (parse.c, cubrid_drv.c, mysql_drv.c, oracle_drv.cpp), `sql_by_cci/ccqt` C 실행기
- **third-party deps**: 모두 노후 — commons-cli 1.2 (2009), log4j 1.2.16 (2010), commons-io 1.3 (2007), dom4j 1.6.1 (2005), jgit 4.3 (2016), ActiveMQ 5.8 (2013), Quartz 2.2.1 (2014). log4j 1.2 는 EOL/보안 이슈 누적.

새 시스템 `cubrid-testkit` 의 구현 언어를 결정해야 한다.

## 2. Decision Drivers

ROADMAP.md 의 제약 + M0 발견을 합쳐 우선순위:

1. **1인 사이드 프로젝트, 6~12개월 호라이즌** — 학습 곡선이 결정의 가장 큰 변수. 새 언어 학습 시간이 직접 진행 시간을 갉아먹는다.
2. **shell/SSH/process exec 도미넌스** — 케이스 형식 자체가 셸 스크립트 (shell/HA/RQG) 또는 셸로 wrapping (sql/jdbc/medium). SSH 클라이언트, 원격 셸 실행, 로컬 프로세스 spawn 이 핵심 기능.
3. **외부 표면 동결 의무** — `bin/ctp.sh` CLI 인자, `conf/<suite>.conf` 81 키, stdout 마커 (`Result Root Dir:`, `flag: OK/NOK`, `found core file`, `CORE_FILE:`), `<resultDir>/main.info` 포맷, `runone.sh` 호출 시그니처, `init_path/init.sh` 함수 컨트랙트 — 모두 새 시스템이 *호환 출력* 해야 함.
4. **공존 기간 안정성** — strangler-fig 4~5 단계 동안 기존 CTP의 jar 들이 살아있어야 함. 새 시스템이 *기존 jar 와 IPC* 할 가능성 (특히 cqt.console.ConsoleAgent) — 그러나 IPC는 stdout/파일 기반으로만 가도 충분.
5. **native 자산 재사용** — `isolation/ctltool/` 의 C 파서를 새 시스템에서 *그대로 호출* 할 수 있어야 처음부터 다시 만들지 않음 (FFI 또는 subprocess).
6. **배포 단순성** — 1인 프로젝트는 jar/JVM 의존 대신 single-binary 배포가 운영 비용이 작음.
7. **moder ecosystem fit** — 노후 third-party를 모던 동등물로 교체할 수 있어야 함 (특히 SSH/로깅).

## 3. Options Considered

### Option A: Java 17/21 (현재 언어 모던화)

**Pros**
- 기존 Java 자산 *그대로 재사용* 가능. 7 jar 산출물이 새 시스템 안에 그대로 살 수 있음.
- 학습 곡선 0 — 사용자가 이미 Java 사용자.
- jsch (SSH), apache commons (CLI/IO), Quartz (스케줄), JDBC 의 풍부한 생태계.
- jUnit / Testcontainers 로 새 시스템 자체 테스트 가능.
- `cqt.webconsole` Jetty 위에서 Java가 자연스러움.

**Cons**
- 기존 시스템의 한계(*ad-hoc dispatcher, in-band signaling, RMI 데몬*)가 그대로 따라옴. *기존 코드 재사용 유혹*이 강해 진정한 재설계가 어려움.
- 노후 의존(log4j 1.2 → log4j2 / slf4j+logback) 마이그레이션은 Java 안에서 해도 별도 작업.
- single-binary 배포 어려움 (GraalVM native-image 가능하지만 jsch/jgit 같은 reflection-heavy 라이브러리에서 추가 설정 필요).
- 1인 운영에서 Maven/Gradle 빌드 + JVM 실행 + 의존 jar tree 관리는 비용.

**Maturity / Risk**: 매우 안정적. 위험은 *과거 회귀*.

---

### Option B: Kotlin (JVM 호환 + 모던 문법)

**Pros**
- Option A 의 장점(기존 Java 자산 호환) + 코루틴(coroutines)으로 동시성 코드가 간결.
- 멀티플랫폼 (Kotlin/JVM, Kotlin/Native) 가능 — long-term 이주 경로 다양.
- `Result<T>`, sealed class, data class 등 모던 패턴이 *오케스트레이션 도메인에 잘 맞음*.
- 학습 곡선 작음 (Java → Kotlin은 빠름).

**Cons**
- JVM 의존 → single-binary 배포 동일한 한계.
- 코틀린 native는 SSH 라이브러리 생태계가 빈약.
- 사용자가 Kotlin 사용 경험이 적을 가능성.

**Maturity / Risk**: JVM 위 Kotlin은 매우 안정. 단, 새로운 영역에 도구가 익숙해야 빠름.

---

### Option C: Go

**Pros**
- **single-binary 배포** — 1인 운영에 최적. CTP_HOME 한 디렉터리에 단일 실행 파일 + conf + 자산.
- **`golang.org/x/crypto/ssh` 표준 라이브러리 — jsch 와 동등한 SSH 기능, 더 모던**.
- 동시성 (goroutines + channels)이 dispatcher → worker 패턴에 자연.
- 빠른 컴파일, 짧은 빌드 사이클 → 1인 사이드에서 iteration cost 작음.
- cobra/spf13 같은 CLI 라이브러리로 ctp.sh 외부 표면 깔끔히 호환.
- C interop (cgo) 이 잘 동작 → `isolation/ctltool/` native 파서 호출 가능.

**Cons**
- 기존 Java 자산을 *직접 재사용 불가* — Java jar는 별도 subprocess 로 실행해야 함 (단, 공존 기간엔 받아들일 만한 비용).
- 사용자가 Go 사용 경험에 따라 학습 곡선 발생.
- Generics 도입(1.18) 이후 일반 표현은 가능하지만 자바/코틀린 만큼 풍부하지 않음.
- jUnit 같은 강력한 테스트 프레임워크가 없음 (testify 정도).

**Maturity / Risk**: 생태계 매우 안정. SSH/CLI/process 영역에서 best-fit.

---

### Option D: Rust

**Pros**
- 가장 안전한 메모리 모델 + 강한 타입 시스템 → orchestrator 의 race condition 회피.
- single-binary 배포.
- C interop 자연 (FFI) — ctltool 파서 호출 가능.
- async/tokio 로 고성능 분산 실행.

**Cons**
- **학습 곡선 매우 높음** — 1인 6-12개월 한도에서 코드 작성보다 차용 검사기와 싸우는 시간이 길어질 위험.
- SSH 라이브러리 (russh, openssh) 가 jsch / Go ssh 보다 미성숙.
- 빌드 시간 길음 → iteration cost 큼.
- ecosystem 변동이 커서 6-12개월 동안 의존이 깨질 가능성.

**Maturity / Risk**: 언어 자체는 안정. *프로젝트 위험은 학습 곡선*.

---

### Option E: Python

**Pros**
- 개발 속도 가장 빠름. 1인이 짧은 시간에 큰 면적 커버 가능.
- paramiko/fabric → SSH; click → CLI; rich → 출력. 풍부한 라이브러리.
- 셸과의 인터페이스가 자연 (subprocess + os.system).
- 케이스 작성자 도구 (pytest)가 친숙.

**Cons**
- single-binary 배포 어려움 (PyInstaller 가 있지만 production-quality는 한계).
- 동적 타입 → 큰 코드베이스에서 리팩토링 리스크. mypy 도입은 추가 작업.
- 성능 (오케스트레이션 자체는 IO 바운드라 큰 문제 아님).
- 의존 관리 (pip / poetry / uv) 가 Java 만큼 표준화되지 않음.

**Maturity / Risk**: 안정적. 단, 큰 시스템에서 *유지보수성* 위험.

---

### Option F: Polyglot (셸 + 다언어, Just/Make 기반)

**Pros**
- 기존 셸 자산 재사용 극대화.
- 점진적 도입 가능 — 모듈마다 다른 언어 가능.
- "재작성"보다 "조립"에 가까움 → 1인에 우호적.

**Cons**
- *진정한 재설계 목표*와 정면 충돌 — 기존 시스템의 디자인 부재(ad-hoc dispatcher 등)가 그대로 살아남음.
- 모듈 간 인터페이스 명세가 더 어려움.
- 새 시스템이 *재작성된 모던 시스템*이라기보다 *셸 매크로 모음*으로 보임.

**Maturity / Risk**: 가장 빠른 시작, 가장 약한 종착점.

---

## 4. Comparison Matrix

| 기준 | Java 17/21 | Kotlin | Go | Rust | Python | Polyglot |
|------|-----------|--------|----|------|--------|----------|
| 학습 곡선 (1인 우호) | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| 기존 자산 재사용 | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐ (subprocess) | ⭐⭐ | ⭐⭐ | ⭐⭐⭐⭐⭐ |
| SSH/process 생태계 | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐ |
| single-binary 배포 | ⭐⭐ | ⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐ | ⭐⭐ |
| C interop (ctltool) | ⭐⭐⭐ (JNI) | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ (cgo) | ⭐⭐⭐⭐⭐ (FFI) | ⭐⭐⭐⭐ (ctypes) | — |
| 모던 동시성 | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐ | — |
| 진정한 재설계 가능성 | ⭐⭐ (회귀 위험) | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐ |
| iteration cost | ⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ |
| 사용자 친숙도 | (사용자 입력 필요) | (사용자 입력 필요) | (사용자 입력 필요) | (사용자 입력 필요) | (사용자 입력 필요) | — |

## 5. Recommended Top-3 (의사결정자 검토용)

**1순위: Go**
- *재설계 의도 + single-binary + SSH 생태계 + 1인 운영*의 균형이 가장 좋다.
- C interop 으로 ctltool 흡수, subprocess 로 기존 jar 공존, cobra 로 ctp.sh 호환 layer 구현.
- 위험: 사용자의 Go 친숙도가 분명하지 않다면 1주 정도의 spike 권고.

**2순위: Kotlin**
- *Java 자산 재사용 + 모던 문법* 의 절충. 진정한 재설계는 약간 약하지만 빠른 진행 가능.
- single-binary 부재가 운영 비용으로 누적될 가능성.

**3순위: Java 17/21**
- *최소 위험* 옵션. 단, 기존 시스템의 디자인 한계가 따라올 위험을 ADR-004 (1차 대체 모듈) 결정으로 보완 필요.

**고려 후 후순위:**
- Rust: 학습 곡선이 1인 6-12개월 한도와 맞지 않음. 단, 핵심 인프라(remote-exec)만 Rust로 작성하고 상위는 다른 언어로 가는 *부분 도입*은 별개로 고려할 수 있음.
- Python: 큰 면적 빠르게 커버하려면 좋지만, 대규모 리팩토링/타입 안정성 부족이 6-12개월 끝나는 시점에 누적 비용으로 누적될 가능성.
- Polyglot: "재작성" 목표와 충돌 — 단기 prototyping 외엔 비추천.

## 6. Open Questions (결정 전 해소 필요)

- 사용자(1인 운영자) 의 언어별 친숙도 / 선호도?
- 새 시스템이 *기존 cqt.webconsole* 을 흡수할 것인지 분리할 것인지?
- 기존 jar 와 *공존 기간 동안의 IPC 모델* (stdout 만 / 파일 만 / RMI 우회 / gRPC)?
- ctltool 파서를 새 시스템에 *흡수*할 것인지 *외부 subprocess 로 유지*할 것인지?
- 새 시스템 자체의 *테스트 전략* (예: 새 시스템의 unit test 인프라가 케이스 실행 인프라와 별개인가?)

## 7. Decision

(미정. Phase 1 (concept) 진입 시점에 결정. 결정 후 본 ADR의 Status 를 Accepted 로 변경하고 Decision/Why/Consequences 섹션을 채운다.)

## 8. Consequences (결정 후 채움)

- 빌드 도구 결정 (ADR-002) 의 입력
- 1차 대체 모듈 선정 (ADR-004) 시 언어 학습 곡선이 결정 변수
- 기존 jar 와의 공존 기간 IPC 모델 결정 트리거
