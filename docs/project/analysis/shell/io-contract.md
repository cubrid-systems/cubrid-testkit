# shell — I/O Contract

**Source:** `cubrid-testtools/CTP/shell/`

shell 모듈의 외부 표면 동결 명세. shell 은 3가지 모드 + cross-module 인프라 호스트이므로 contract 가 가장 큼.

---

## 1. CLI 표면

```
ctp.sh shell    [-c <conf>] [-h] [-v]    # SSH 분산
ctp.sh rqg      [-c <conf>]              # SHELL 경로 + TEST_CATEGORY=rqg
ctp.sh unittest [-c <conf>]              # 로컬 (옵션 conf, null 가능)
```

종료 코드:
- 0 — task 완료
- -1 — 환경 점검 / build URL / scenario 부재 시 `System.exit(-1)`

CLI 인자가 isolation 과 동일한 스킴 — 다중 task 호출 가능 (`ctp.sh shell isolation`).

---

## 2. conf 스키마

### 2-1. shell.conf — 26 키 (M0 #2)

기본 키 카테고리:
- multi-instance: `default.{ssh,cubrid,broker1,broker2,brokercommon,cm,ha}.<property>`, `env.instance{1,2}.{ssh,cubrid,broker1,broker2}.*`
- 실행 제어: `scenario`, `testcase_retry_num`, `testcase_timeout_in_secs`, `testcase_exclude_from_file`
- CUBRID: `cubrid_download_url`, `cubrid_createdb_opts`
- 공통 동작: `enable_memory_leak`, `db_charset`, `need_make_locale`

### 2-2. shell_ci.conf — 42 키 (CI 변형)

shell.conf 의 모든 키 + **14개 CI 전용**:
```
test_platform                           — Windows / Linux 등
test_continue_yn                        — continue mode 활성
testcase_git_branch                     — testcases 레포의 branch
testcase_update_yn                      — git pull 활성
testcase_exclude_by_macro               — 매크로 기반 exclude
default.broker1.APPL_SERVER_SHM_ID      — port 충돌 회피용 SHM ID 강제 지정
default.broker1.BROKER_PORT             — 마찬가지
default.broker2.APPL_SERVER_SHM_ID
default.broker2.BROKER_PORT
default.cubrid.cubrid_port_id           — 프로세스 충돌 회피
default.ha.ha_port_id
delete_testcase_after_each_execution_yn — 실행 후 정리
enable_check_disk_space_yn              — 디스크 공간 검사
feedback_type                           — DB / file / null
```

### 2-3. ha_shell.conf — 28 키 (HA 변형)

shell.conf + master/slave 토폴로지 키 추가:
```
env.instance{1,2}.master.ssh.{host,user}
env.instance{1,2}.slave.ssh.{host,user}
env.instance{1,2}.ssh.relatedhosts        — 관련 호스트 (HA 노드)
```

### 2-4. dot-notation 와일드카드

isolation 과 동일. `default.cubrid.<property>=<val>` → 원격 cubrid.conf 의 `<property>=<val>` 전사.

### 2-5. UNITTEST conf

`unittest.conf` 는 옵션. 미지정 시 GeneralLocalTest.exec 가 null 인자로 호출.
- `TEST_TYPE` 시스템 프로퍼티가 더 우선 (config 보다 먼저 결정)
- `TEST_CATEGORY` 시스템 프로퍼티로 logDir 결정

---

## 3. 케이스 측 입력

### 3-1. 케이스 위치 (분산 모드)

```
<scenario_root>/<...>/cases/<name>.sh
<scenario_root>/<...>/answers/<name>.answer
```

규칙: case path 는 `cases/` 세그먼트를 *반드시* 포함 (Test.java 가 `lastIndexOf("cases")` 로 분리). 없으면 path 파싱 실패.

### 3-2. 케이스 init 컨트랙트

```bash
# 케이스 첫 번째 실행 라인:
. $init_path/init.sh
init test
set -x
# ... 케이스 본체 ...
```

`$init_path` 는 deploy 가 설정한 환경 변수 (CTP/shell/init_path/ 디렉터리의 원격 복사본 위치).

### 3-3. 결과 채널

케이스 종료 시:
- stdout / stderr → 워커가 capture (consoleOutput)
- `<name>.result` 파일에 케이스 작성자가 결과 기록 → 워커의 `collectGeneralResult` 가 회수
- 워커가 `.answer` 와 `.result` 를 diff (또는 stdout 의 패턴 검사)

### 3-4. UNITTEST plug-in 컨트랙트

`shell/local/<TEST_TYPE>.sh` 가 다음 4 함수를 정의해야 함:
```bash
init() {
    # 환경 셋업
}
list() {
    # 사용 가능한 테스트 케이스 한 줄당 한 개로 stdout
}
execute() {
    local testcase=$1
    # 케이스 실행
    IS_SUCC=true|false
}
finish() {
    # 정리
}
```

GeneralLocalTest 의 invoke() 가 stdout 의 `EEOOKK` 마커로 환경 변수 (예: IS_SUCC) 회수.

---

## 4. RMI 서비스 contract

### 4-1. 서비스 등록

```java
LocateRegistry.createRegistry(port);
registry.rebind("shellService", new ShellServiceImpl(props));
```

- port: `agent_login_port` (default 1099) from `<CTP_HOME>/conf/shell_agent.conf`
- 서비스 이름: `"shellService"` (RMI registry binding)

### 4-2. 클라이언트 접근

```java
// SSHConnect.SERVICE_TYPE_RMI 모드일 때 내부적으로 RMI 호출
// 정확한 인터페이스는 ShellService.java 에 정의됨
```

(ShellService 인터페이스 메서드 시그니처 — 정밀 분석은 후속.)

### 4-3. alive ping

Test.runAll 안에서 RMI 모드일 때:
```bash
echo HELLO
```
응답이 `HELLO` 이면 alive, 아니면 1초 대기 후 재시도.

---

## 5. 출력 파일 (currentLogDir)

isolation 과 유사하나 추가:

| 파일 | 용도 |
|------|------|
| `main_snapshot.properties` | 시작 conf snapshot (Main.exec가 직접) |
| `dispatch_tc_ALL.txt` | 전체 케이스 풀 |
| `dispatch_tc_FIN_<envId>.txt` | env별 완료 케이스 |
| `test_<envId>.log` | env별 워커 로그 |
| `<resultDir>/main.info` | summary (sql 모듈과 같은 형식?) |
| `*_fail_backup_package*.tar.gz` | generateFailBackupPackage — 실패 케이스만 묶음 (isolation의 통째 백업과 다름) |

---

## 6. 콘솔 출력 마커

isolation 과 같은 ENV/TESTCASE 마커:
```
[ENV START] <envId>
[TESTCASE] <tc> EnvId=<envId> [OK]|[NOK][, retry: <N>]
[ENV STOP] <envId>
```

retry 시 `, retry: <N>` 후미에 추가 (DispatchTicket).

---

## 7. Feedback 이벤트

isolation 의 7 이벤트 + retry 전용:

```java
onTaskStartEvent(buildUrl)
onTaskContinueEvent()
onTaskStopEvent()
setTotalTestCase(total, macroSkipped, tempSkipped)
onTestCaseStartEvent(tc, envIdentify)
onTestCaseStopEvent(tc, success, elapseMs, resultText, lastPassResultCont, envIdentify, isTimeOut, hasCore, skipType, retryCount)
onTestCaseStopEventForRetry(tc, success, elapseMs, resultText, envIdentify, isTimeOut, hasCore, skipType, retryCount)  ⭐ retry 한정
onStopEnvEvent(envId)
```

⭐ shell 의 onTestCaseStopEvent 는 **`lastPassResultCont` + `retryCount` 를 추가** (isolation 과 다름). 새 시스템에서 두 모듈의 Feedback 인터페이스를 *통합* 할지 *분리* 할지 ADR.

---

## 8. 외부 환경 의존

원격 환경에 *반드시* 존재해야 함:
- `$CTP_HOME` — deploy 가 셋업
- `$init_path` — deploy 가 CTP/shell/init_path/ 를 복사한 경로
- `$JAVA_HOME` (선택적: `$JAVA_HOME_<VERSION>` for multi-jdk)
- `$CUBRID` — CUBRID 설치 dir
- `$TEST_BIG_SPACE` (선택적, 큰 임시 디렉터리)
- `cubrid` 명령 PATH

윈도우 한정:
- `cygpath` 명령
- `cygwin` 환경
- registry 헬퍼 (`*Regedit.bat`)

---

## 9. 동결 표면 요약

```
입력
├── ctp.sh {shell|rqg|unittest} [-c <conf>]
├── {shell,shell_ci,ha_shell}.conf 26~42 키
├── unittest.conf (옵션)
├── testcases/<...>/cases/*.sh (cases/ 세그먼트 필수)
├── testcases/<...>/answers/*.answer
├── shell/local/<TEST_TYPE>.sh — UNITTEST 4 함수 컨트랙트
├── CTP/shell/init_path/init.sh — case가 source
└── (옵션) RMI 서비스 binding "shellService" + agent_login_port

출력
├── stdout: [ENV START/STOP], [TESTCASE] <tc> [OK/NOK][, retry: N]
├── currentLogDir/main_snapshot.properties
├── currentLogDir/dispatch_tc_{ALL,FIN_*}.txt
├── currentLogDir/test_*.log
├── currentLogDir/<resultDir>/main.info
├── *_fail_backup_package*.tar.gz (실패 한정)
├── 케이스 작성자가 쓴 <name>.result — 워커가 회수
└── Feedback 이벤트 (retry 인지)

종료 코드
├── 0  (정상)
└── -1 (환경 / scenario / build URL 부재)
```
