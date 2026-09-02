# Non-Goals — 의도적으로 하지 않는 것

- **Date:** 2026-09-02
- **Status:** Accepted (NG1·NG2·NG4·NG5·NG6 확정 / NG7~NG11 제안 — 해당 Phase 진입 시 확정. NG3 은 결번 확인 대기)
- **Inputs:** ROADMAP §0a·§6a·§8, ADR-003, `analysis/common/io-contract.md`, `analysis/_overview/orphan-enums.md`

> non-goal 은 "나중에 할 일"이 아니라 **"이 프로젝트의 범위가 아님을 결정한 일"** 이다.
> 각 항목은 위반 신호(무엇이 보이면 이 선을 넘은 것인가)를 함께 적는다.

---

## NG1 — testcases 레포를 수정하지 않는다 *(확정)*

`cubrid-testcases` / `cubrid-testcases-private` / `cubrid-testcases-private-ex` 3개 레포는 **읽기 전용 입력**이다.

- 케이스 형식이 불편해도 **새 시스템이 맞춘다**. 케이스를 고쳐서 새 파서에 맞추지 않는다.
- 새로 생성되는 코퍼스(fuzz seed / mismatch corpus / crash 재현 케이스, §6a E2·E3·E5·E6)를 이 레포에 넣지 않는다 — 외부 storage 또는 testkit 내부 별 트리.
- **위반 신호:** testcases 레포에 대한 커밋이 이 프로젝트의 산출물에 등장.

## NG2 — 외부 표면을 바꾸지 않는다 *(확정)*

`concept/external-surface-freeze.md` 에 F1/F2/F3 로 등급이 붙은 항목은 개선 대상이 아니다.

- `main.info` 는 `:` 구분이고 `summary_info` 는 `=` 구분인 비대칭을 **통일하지 않는다**.
- `-1` 종료 코드(셸에서 255)를 관례적인 `1` 로 정리하지 않는다.
- 마커 문자열의 오탈자·비일관 대소문자를 고치지 않는다.
- **위반 신호:** "이왕 재작성하는 김에 출력 포맷도 정리하자" 는 판단이 커밋에 반영됨.

## NG3 — *(예약 — 원 spec 유래, 본 레포에서 참조된 적 없음)*

ROADMAP 과 extensions 문서가 NG1·NG2·NG4 를 참조하지만 **NG3 은 어디에서도 참조되지 않는다**. 원 명세에 있었으나 이 레포로 옮겨지지 않은 것으로 보인다.

- 번호 재배정은 하지 않는다 — 기존 문서 11곳의 NG 참조가 깨진다.
- 원 명세를 확인해 내용을 복원하거나, 확인 불가로 판정되면 "결번" 으로 명시 확정한다.
- **해소 시점:** 다음 분기 게이트(ROADMAP §7).

## NG4 — 비-CUBRID DBMS 호환을 새로 추가하지 않는다 *(확정)*

- `isolation/ctltool/` 에 MySQL / Oracle 드라이버(`mysql_drv.c`, `oracle_drv.cpp`)가 존재하지만, 이를 **유지·확장하지 않는다**. 현 활성 여부 확인 후 ADR-007 에서 처리(Option C = CUBRID-only 도 후보).
- §6a-E6(differential testing)이 PostgreSQL 을 peer 로 쓰는 것은 **NG4 위반이 아니다** — CUBRID 에 호환성을 *추가* 하는 것이 아니라 CUBRID 를 *검증* 하기 위해 비교 대상으로 쓰는 것.
- **위반 신호:** 새 시스템의 conf 에 비-CUBRID DBMS 접속 설정이 1급 시민으로 등장.

## NG5 — Java API 와 jar 산출물 호환을 제공하지 않는다 *(확정 — ADR-003)*

- `cubridqa-common.jar` 등 7개 jar 를 **같은 이름·같은 위치로 산출하지 않는다**.
- `CommonUtils` 40+ 메서드, `IniData` 15, `ConfigParameterConstants` 82 상수의 **시그니처 호환 layer 를 만들지 않는다**. (`analysis/common/io-contract.md` §7 의 Option A "jar 호환 layer" 는 **기각**)
- 공존은 jar 호환이 아니라 **subprocess 호출**로 달성한다 (같은 문서 Option B/C).
- **결과:** 만약 외부 자산이 jar 이름을 직접 참조하는 것이 §11-1 확인에서 발견되면, 그것은 *호환 layer 를 만들 이유*가 아니라 *그 자산을 고칠 이유*다.
- **위반 신호:** **testkit 자체 빌드 산출물**에 `.jar` 가 등장.
- ⚠️ 예외 — `shell/init_path/commonforjdbc.jar` / `commonforjdbc_aix.jar` 는 *원격에 배포되는 자산*이지 testkit 의 빌드 산출물이 아니다. NG5 대상 아님 (`external-surface-freeze.md` §7-8). 공존 기간의 기존 Ant 빌드가 만드는 jar 들도 마찬가지.

## NG6 — QA 운영 축을 이번 마이그레이션에서 재현하지 않는다 *(확정 — 2026-09-02 프로젝트 방향성)*

**전체 목록과 사유는 `concept/migration-exclusions.md` 가 정본.** 요약:

- 스케줄러 / 메시지 큐 (ActiveMQ 5.8 + Quartz 2.2.1) — `common/sched/` 전체
- 메일 / 리포트 — `MailSender`, `RunShellMain --enable-report|--report-cron|--mailto|--mailcc`
- 이슈 등록 — `IssueMain`, `MergeTemplate` + `tpl/issue_*.tpl`, `RunShellMain --issue`
- 저장소 서비스 / 자가 업그레이드 — `common/grepo/`, `UpgradeMain`
- Feedback 의 **DB 백엔드**만 (인터페이스와 Null/File 은 축 T 로 보존)
- webconsole (진입점은 F1 유지하고 기존 자산 호출)

**제외 ≠ 삭제.** 세 가지 처리(동결 유지+기존 호출 / 미이관+존치 / 표면에서 제거)를 구분한다 — `migration-exclusions.md` §2.
**나중에 새로운 층으로 다시 세운다** — 의존 방향은 `운영 층 → testkit` 단방향. ADR-012 예약.

## NG7 — dead 표면을 이식하지 않는다 *(제안 — ADR-005)*

- orphan 7 task (`cci` `dots` `nbd` `sysbench` `tpcc` `tpcw` `ycsb`) 는 새 시스템에 **존재하지 않는다**. `orphan-enums.md` 가 7개 모두 진짜 dead 임을 확인했다.
- silent no-op 로 남기지 않는다 — 인식 불가 task 로 처리해 헬프를 출력한다.
- grace period 동안은 "폐기됨" 메시지를 명시적으로 출력하는 것을 권고.
- **확정 시점:** Phase 1 마무리 (ADR-005).

## NG8 — webconsole 을 재작성하지 않는다 *(제안)*

- `cqt.webconsole` (Jetty 기반 결과 뷰어)는 공존 기간 내내 **기존 자산 그대로 subprocess 호출**한다.
- UI 현대화·API 화·대시보드 확장은 이 프로젝트의 범위가 아니다.
- **확정 시점:** Phase 5 재평가. (축 분리상 webconsole 은 축 O 이므로 NG6 와도 정합)

## NG9 — 테스트 케이스의 내용을 판단하지 않는다 *(제안)*

- 실패하는 케이스를 고치거나, 중복 케이스를 정리하거나, 커버리지를 평가하지 않는다.
- 새 시스템은 **러너**다. 케이스의 품질은 별개 활동이다.
- 단, 회귀 동등성 검증 과정에서 발견한 **기존 시스템의 버그**(예: `make_db_data` 의 `tar -zxvf mdb.tar.gz` 하드코딩)는 기록만 남기고 새 시스템에서는 올바르게 구현한다 — 이것은 NG2 위반이 아니다(출력 표면이 아니라 내부 동작).
- **확정 시점:** Phase 3.

## NG10 — 성능 개선을 목표로 삼지 않는다 *(제안)*

- 새 시스템이 기존보다 **빨라야 한다는 요구는 없다**. 테스트 실행 시간은 SUT(CUBRID)와 케이스가 지배한다.
- 동시성 모델(goroutine)은 *코드 단순성*을 위한 선택이지 처리량을 위한 선택이 아니다.
- **위반 신호:** 벤치마크 수치가 설계 결정의 근거로 등장.

## NG11 — 다중 사용자·권한·멀티테넌시를 도입하지 않는다 *(제안)*

- 1인 운영 + CI 서비스 계정이 전부다. 사용자 모델, 권한 검사, 감사 로그를 만들지 않는다.
- **위반 신호:** conf 에 사용자/역할 개념이 등장.

---

## 부록 — non-goal 이 *아닌* 것 (자주 혼동되는 항목)

| 항목 | 판정 | 근거 |
|---|---|---|
| §6a 확장 E1~E7 | **범위 안** (additive) | ROADMAP §6a. 단 strangler-fig 우선(§8 risk 마지막 행) |
| Windows / cygwin 지원 | **범위 안** (유지) | 기존 케이스가 `cygpath`·`*Regedit.bat` 에 의존. 단 *네이티브* Windows 지원 추가는 범위 밖 |
| interactive 모드 | **범위 안** (동작 보존) | F1. `#SCRIPTCONT` **메커니즘**만 NF |
| ctltool native 자산 | **범위 안** (그대로 빌드·호출) | ADR-002 / ADR-007 Option B |
| core dump 분석 (`coreanalyzer`) | **범위 안** | `CORE_FILE:` 마커가 F1 이므로 기능 자체가 필요 |
