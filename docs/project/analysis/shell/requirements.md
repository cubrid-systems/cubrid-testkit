# shell — Requirements (모듈이 해결하는 문제 + 외부 호출 형태)

**Source:** `cubrid-testtools/CTP/shell/`

**Companion docs:** `design.md` (3-mode 시퀀스), `io-contract.md`, `implementation-notes.md`, `test-corpus.md`

---

## 1. 이 모듈이 해결하는 문제

CUBRID 의 *임의의 셸 명령으로 표현되는 시나리오* 를 분산 환경에서 실행한다. 케이스 자체가 bash 스크립트이므로 SQL/JDBC/CCI/expect/native build/HA topology setup 등 *광범위한 행위* 를 한 케이스 안에 묶을 수 있다.

또한 shell 모듈은 다음 *4개 모듈의 공통 인프라 호스트* 역할도 겸한다 (M0 #4 발견):
- `shell.common.SSHConnect/LocalInvoker/...` 는 isolation/ha_repl/cdc_repl 가 모두 import
- 즉 shell 은 **단순 모듈이 아니라 *cross-module 인프라 + 자체 실행 모듈*** 의 이중 정체성을 갖는다.

---

## 2. 외부 호출 형태 (3가지 모드)

shell 모듈은 한 jar 안에 *3가지 진입점* 을 갖는다.

### 2-1. SSH 분산 모드 (SHELL / RQG)

```
ctp.sh shell [-c <shell.conf>]
ctp.sh rqg   [-c <rqg.conf>]    # 시스템 프로퍼티 TEST_CATEGORY=rqg
```

- CTP.executeShell → URLClassLoader(shell/lib/cubridqa-shell.jar) → reflection
- `com.navercorp.cubridqa.shell.main.Main.exec(configFilename: String)`
- 분산 SSH 워커, 케이스 = 셸 스크립트 (`<dir>/cases/<name>.sh`)

### 2-2. 로컬 모드 (UNITTEST)

```
ctp.sh unittest [-c <unittest.conf>]
```

- CTP.executeUnitTest → reflection
- `com.navercorp.cubridqa.shell.main.GeneralLocalTest.exec(configFilename | null)`
- *SSH 없이* 로컬에서 실행. `cd ${CTP_HOME}; source shell/local/<TEST_TYPE>.sh; init/list/execute/finish`
- shell/local/<TEST_TYPE>.sh 가 4 함수를 정의해야 함 (plug-in 컨트랙트)

### 2-3. RMI 서비스 모드 (옵션 데몬)

```
java com.navercorp.cubridqa.shell.service.Server
```

- 원격에 데몬으로 띄움. `../conf/shell_agent.conf` 의 `agent_login_port` (default 1099) 에 RMI registry 생성
- 클라이언트 (다른 host의 SSHConnect) 가 SERVICE_TYPE_RMI 로 셸 명령을 invoke
- 사용 시나리오: SSH 차단 / sudo interactive 회피 / 폐쇄망

---

## 3. 사용자 요구사항 (M0 분석으로 추정)

핵심 가이드: `doc/shell_guide.md` (30KB), `doc/shell_ext_guide.md` (43KB), `doc/shell_heavy_guide.md`, `doc/shell_long_guide.md`, `doc/ha_shell_guide.md`. (정밀 인용은 Phase 1 후속.)

추정 핵심 요구:

1. **임의의 bash 시나리오 실행** — 케이스 작성자가 자유롭게 셸 명령으로 시나리오 구성
2. **다중 instance 분산 실행** — env.instance{1,2}.* 토폴로지로 N 노드 워커 병렬
3. **OS 차이 흡수** — Linux 와 Windows (cygwin) 모두에서 동일 케이스 실행 (설계 의도)
4. **HA 토폴로지 지원** — DeployHA + master/slave 인스턴스
5. **Hot env reload** — 실행 중에 conf 변경 → env 추가/삭제 동적 반영 (CI 운영 우호)
6. **retry 인지 디스패치** — 실패한 케이스 자동 재시도 (DispatchTicket.retryCount)
7. **케이스 단위 결과 회수** — `<case>.result` 파일을 워커가 collectGeneralResult
8. **fail 백업 패키지** — generateFailBackupPackage 로 실패 케이스 묶음
9. **다중 case 소스** — Git (TestCaseGithub) + SVN (TestCaseSVN) 둘 다 지원
10. **agent (RMI) 모드** — 옵션. SSH 못 쓰는 환경 우회

---

## 4. 비기능 요구

| 항목 | 현재 동작 | 새 시스템에서의 의미 |
|------|----------|---------------------|
| OS 호환 | runTestCase_linux / runTestCase_windows 분기 | 새 시스템은 *내부 분기* 또는 *별도 worker* 결정 |
| init.sh 함수 컨트랙트 | shell.local/<TYPE>.sh + EEOOKK marshalling | 더 깨끗한 IPC (named pipe / structured stdout) ADR 후보 |
| 다중 case 소스 | Git / SVN 양쪽 지원 | SVN 사용 흔적 있는지 확인 후 폐기 검토 |
| TestMonitor | 활성 (isolation은 비활성) | 새 시스템도 활성 — 케이스가 길거나 hang 잦음 |
| RMI 데몬 | 옵션 컴포넌트 | 1차 strangler-fig 에서 *제외 가능* |
| `<case>.result` 파일 회수 | collectGeneralResult | 동결 표면 — 케이스 작성자 컨트랙트 |
| fail 백업 | generateFailBackupPackage | 새 시스템에서도 fail-only 백업 정책 유지 |

---

## 5. 의존하는 외부 자원 (요약)

상세는 `io-contract.md`.

- **CUBRID 설치**: 원격 + 로컬 모두 (UNITTEST 는 로컬에 cubrid 필요)
- **testcases 레포**: cubrid-testcases-private-ex/shell (3,658 .sh + 3,090 .answer + 다중 보조), cubrid-testcases-private/HA/shell (482 .sh), shell_heavy/shell_perf/shell_ext (각 변형)
- **CTP/shell/init_path/**: init.sh + shell_utils.sh + ha_*.sh + commonforjdbc.jar + commonforc/ + ccidb.sql + Windows 레지스트리 헬퍼 (HA.properties, *.bat) — deploy 가 원격으로 복사
- **공통 인프라**: CTP/common (CommonUtils, ConfigParameterConstants, Constants, LocalInvoker, Log, MailSender, MakeFile)
- **환경 변수**: $CTP_HOME, $JAVA_HOME, $JAVA_HOME_<VERSION> (multi-jdk), $CUBRID, $init_path, $TEST_BIG_SPACE, $TEST_TYPE, $TEST_CATEGORY, $CUBRID_CHARSET, $EXCLUDED_CORES_BY_ASSERT_LINE

---

## 6. 새 시스템 설계 입력 (요약)

shell 모듈 대체 시 *반드시 충족*:

1. ✅ 케이스 = `<dir>/cases/<name>.sh` 형식 (path 의 `cases/` 세그먼트 필수)
2. ✅ 케이스가 `. $init_path/init.sh` 를 source — init.sh 의 4 함수 (init test / 그 외) 컨트랙트 보존
3. ✅ `<case>.result` 파일을 워커가 회수해서 `.answer` 와 비교
4. ✅ 케이스 stdout 을 워커 로그 + console 에 기록
5. ✅ DispatchTicket.retryCount + complete(success, hasCore) → needRetry 모델
6. ✅ env hot reload (config monitor)
7. ✅ multi-instance + relatedHosts 점검
8. ✅ Linux/Windows 분기 (OS 별 케이스 .answer / .answer_win 변형 인지)

ADR 후보:
- RMI 서비스 모드 — 1차 대체에서 제외할지 (ADR-004)
- TestCaseSVN 폐기 (현 시점 사용 여부 확인)
- shell.local/<TEST_TYPE>.sh 의 EEOOKK 마샬링 — 깨끗한 IPC 로 교체
- RQG / shell_ext / shell_heavy / shell_perf / longcase 의 *suite 변형* 표현 (medium ↔ sql 패턴 차용)
- shell.common.* 추출 — 4 모듈 공통 인프라 (ADR-004)
