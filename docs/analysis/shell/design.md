# shell — Design (Main 시퀀스 분석)

**Source:** `cubrid-testtools/CTP/shell/`

**Phase 0 M0 #4 status:** 완료 (sequence-level seed)

---

## 1. 클래스 구성 (35 Java 파일, 6 sub-package)

```
com.navercorp.cubridqa.shell
├── common (11)               ⭐ 다른 모듈(isolation/ha_repl/cdc_repl)이 의존하는 진짜 공통 인프라
│   ├── SSHConnect            jsch 기반 + RMI 모드 듀얼. 모든 원격 통신의 게이트웨이
│   ├── ScriptInput           추상 빌더
│   ├── ShellScriptInput      셸 명령 묶음 빌더 (cd/export/...)
│   ├── GeneralScriptInput    범용 명령 빌더
│   ├── LocalInvoker          로컬 셸 실행 (Process)
│   ├── HttpUtil              build URL 다운로드 등
│   ├── CommonUtils           getBuildVersionInfo(ssh) 등 — Main.exec에서 직접 호출
│   ├── Constants             retry/skip type
│   ├── Log                   파일 로거
│   ├── GeneralFeedback       feedback 헬퍼
│   └── SyncException         예외 타입
│
├── main (12)                 모듈 본체 — 두 개의 entry point + 워커/오케스트레이션
│   ├── Main                  ★ entry: SHELL / RQG (executeShell이 호출)
│   ├── GeneralLocalTest      ★ entry: UNITTEST (executeUnitTest이 호출, 로컬 전용)
│   ├── JdbcLocalTest         (UNITTEST 변형으로 추정 — getProperty TEST_TYPE 분기)
│   ├── Context               config 모델 (envList, scenario, retry/timeout, isWindows, hot-reload)
│   ├── TestFactory           오케스트레이션 (check → update → deploy → test, hot env 추가/삭제)
│   ├── Test                  env 1개 워커, runTestCase_linux/_windows 분기, retry 지원
│   ├── TestMonitor           ⭐ 활성 (isolation은 비활성) — 타임아웃/하트비트 모니터
│   ├── ShellHelper           SSH 빌더, instance 메타 추출
│   ├── CheckRequirement      env 사전 점검 (relatedHosts 포함)
│   ├── Feedback              인터페이스
│   ├── RunShellMain          (별도 entry로 추정, 추가 조사 필요)
│   └── ManualReportJob       수동 리포트 작업 (Quartz job?)
│
├── deploy (5)
│   ├── Deploy                기본 배포 오케스트레이터
│   ├── DeployOneNode         단일 노드 단위
│   ├── DeployHA              ⭐ HA 토폴로지 전용 (master/slave 동기 시작)
│   ├── TestCaseGithub        git 기반 testcases 동기화
│   └── TestCaseSVN           ⭐ SVN 기반 (legacy 케이스 레포 호환)
│
├── dispatch (1)
│   └── Dispatch              singleton + DispatchTicket(retryCount) — retry 인지 디스패치
│
├── result (3)                Feedback 구현체
│   ├── FeedbackFile
│   ├── FeedbackDB
│   └── FeedbackNull
│
└── service (3)               ⭐ RMI 서비스 모드 (shell 모듈만 보유)
    ├── Server                main: ../conf/shell_agent.conf 읽고 RMI rebind
    ├── ShellService          인터페이스
    └── ShellServiceImpl      구현 — 원격 클라이언트가 셸 명령을 invoke
```

설계 패턴 요약:
- **isolation과 동일한 골격**: Pipeline + Pool + Singleton dispatcher + Strategy(Feedback)
- **추가 능력**: retry-aware dispatch, hot env reload, RMI 데몬 모드, HA-aware 배포, SVN/Git 양쪽 케이스 소스, OS-별 분기 (linux/windows)

---

## 2. 두 개의 진입점 — 같은 모듈, 다른 모델

### 2-1. `Main.exec(configFilename)` — SHELL / RQG (분산 SSH)

isolation의 Main과 거의 동일 구조 — 다음 차이:
- `setLogDir("shell")` (RQG는 isolation 위에서 시스템 프로퍼티 `TEST_CATEGORY=rqg` 로 분기)
- Build URL 미지정 시 *envList[0]* 의 SSH로 buildInfo 조회 — `shell.common.CommonUtils.getBuildVersionInfo(ssh)` 호출
- scenario 디렉터리 검증 (`calcScenario`, isolation과 동일 로직)
- `main_snapshot.properties` 작성 (Main이 직접 — isolation은 TestFactory가 함)

```
ctp.sh shell -c shell.conf
   ▼
CTP.executeShell → URLClassLoader(shell.jar) → reflection
   ▼
shell.main.Main.exec(configFilename)
   ├── new Context(configFilename)
   ├── envList 검증 → exit(-1) on empty
   ├── build URL 검증 또는 envList[0] SSH로 buildInfo 조회
   ├── calcScenario(context) → SSH로 scenario 절대경로 검증
   ├── main_snapshot.properties 기록
   └── new TestFactory(context).execute()
```

### 2-2. `GeneralLocalTest.exec(configFilename)` — UNITTEST (로컬)

UNITTEST는 SSH/원격 환경 *없이* 동작. 메커니즘:

1. `Context` 로드 (config는 nullable — `executeUnitTest` 안에서 `null` 허용)
2. `TEST_TYPE` 시스템 프로퍼티 로딩 (없으면 `"general"`)
3. `setLogDir(TEST_TYPE)`, `setTestCategory(TEST_CATEGORY)`
4. **invoke()** 패턴:
   ```
   bash -c "cd ${CTP_HOME}; source shell/local/${TEST_TYPE}.sh; <action> [args]"
   ```
   액션은 `init`, `list`, `execute <tc>`, `finish` 4가지. `shell/local/<TEST_TYPE>.sh` 가 이 4 함수를 정의해야 함.
5. **결과 추출 trick**: `<action>; echo GPROPSTART; echo G_PROPERTY_KEY=${KEY}EEOOKK` 패턴으로 환경 변수를 stdout에 마샬링하고 Java 측에서 `EEOOKK` 표지로 파싱.

이 모델은 cli-tree에서 본 *reflection 로드* 동일하지만, **외부 통신은 LocalInvoker만 사용**. SSH/원격 의존 없음 → testcases가 *현재 머신에 이미* 존재해야 한다.

`shell/local/` 디렉터리는 cli-tree에서 보지 못한 영역 — UNITTEST/JdbcLocalTest의 핵심 자산. 새 시스템에서 동결 표면.

---

## 3. TestFactory.execute() — isolation과의 차이

```
execute()
  ├── continueMode 분기는 isolation과 같음 (Dispatch.init → finished cases 차감 등)
  ├── checkRequirement(context) — env마다 + 각 env의 relatedHosts에 대해서도 검사 ⭐
  ├── feedback.onTaskStartEvent
  ├── concurrentSVNUpdate ⭐ — context.isScenarioInGit() 분기로 TestCaseGithub 또는 TestCaseSVN
  ├── Dispatch.init
  ├── concurrentDeploy(envList, false)
  ├── concurrentTest(envList, false)
  ├── startConfigMonitor() ⭐ — config 파일 watch, env 추가/삭제 감지
  │      ├── 삭제된 env: test.stop() + 종료 대기
  │      └── 추가된 env: checkRequirement → joinTest (하위 deploy + test)
  ├── while !isAllTestsFinished: sleep
  ├── feedback.onTaskStopEvent
  └── CommonUtils.generateFailBackupPackage(context) ⭐ — 실패 케이스만 모아서 백업 파일 생성
```

isolation 대비 5가지 추가 능력:
1. ⭐ **relatedHosts** 점검 — env의 main host 외 부속 호스트(브로커 분리/HA slave)도 사전 검사
2. ⭐ **TestCaseSVN** 지원 — git 외 svn 레거시 호환
3. ⭐ **startConfigMonitor** — 5초마다 reload, env 풀 동적 변경
4. ⭐ **TestMonitor 워커 활성** — concurrentTest에서 Test 와 Monitor를 같은 풀에 push
5. ⭐ **generateFailBackupPackage** — isolation은 currentLogDir 통째 백업, shell은 실패 모음 추출 백업

---

## 4. Test.runAll() — retry-aware 워커 루프

```
runAll() — currEnvId 단일 워커
  while (!shouldStop && !Dispatch.isFinished()):
    │
    ├── if (RMI 프로토콜): aliveScript = "echo HELLO"; ssh.execute → "HELLO" 검증
    │       └── 실패 시 sleep(1) continue (재시도 루프, 대기)
    │
    ├── ticket = Dispatch.claimNext()      ← DispatchTicket(testCase, retryCount)
    ├── ticket == null → break
    │
    ├── 케이스 메타 파싱:
    │       p = path.lastIndexOf("cases")
    │       testCaseDir  = path[0..p+5]    ← .../cases
    │       testCaseName = path[p+6..]     ← cases/ 이하 상대경로
    │       testCaseResultName = name.replace(".sh", ".result")
    │
    ├── feedback.onTestCaseStartEvent(tc, env)
    ├── try:
    │     resetProcess()                   ← 잔존 cubrid 프로세스 정리
    │     resetCUBRID()                    ← cubrid service stop/clean
    │     resetSSH()                       ← 매 케이스 새 SSH
    │     if (enableCheckDiskSpace) checkDiskSpace()
    │     consoleOutput = runTestCase()    ← OS 분기 (linux/windows)
    │     doFinalCheck()                   ← 사후 검증
    │     collectGeneralResult()           ← <tc>.result 파일 회수
    │   catch:
    │     addResultItem("NOK", "Runtime error (...)")
    │   finally:
    │     resultItems → workerLog
    │     hasCore = ("NOK found core file" or "NOK found fatal error" 발견 시)
    │     needRetry = Dispatch.complete(ticket, success, hasCore) ⭐
    │     if !success && !hasCore && enableSaveNormalErrorLog:
    │         resultCont += doSaveNormalErrorLog()
    │     if !success: resultCont += "===== CONSOLE OUTPUT =====" + consoleOutput
    │     if needRetry:
    │         feedback.onTestCaseStopEventForRetry(...)        ← retry는 별도 이벤트
    │     else:
    │         feedback.onTestCaseStopEvent(..., lastPassResultCont, ...)
    │         if needDropTestCase: dropTestCaseAfterTest()     ← 케이스 삭제 옵션
    │         dispatchLog.println(tc)
  ├── close() / feedback.onStopEnvEvent
```

### runTestCase 의 OS 분기

**Linux (`runTestCase_linux`)**:
```sh
cd <testCaseDir>
ulimit -c unlimited
if [ "$JAVA_HOME_<VERSION>" ]; then
  export JAVA_HOME=$JAVA_HOME_<VERSION>
fi
export TEST_BIG_SPACE=...
[export CUBRID_CHARSET=...]
[export EXCLUDED_CORES_BY_ASSERT_LINE=...]
echo > <testCaseResultName>
addSshInfoScript(script)        # SSH 인포를 스크립트 환경에 주입
sh <testCaseName> 2>&1
```

**Windows (`runTestCase_windows`)**:
```sh
cd <testCaseDir>
TEST_BIG_SPACE 처리 (cygpath 변환 포함)
sh <testCaseName> 2>&1
waitNetReady()                  # netstat로 TIME_WAIT/FIN_WAIT 정리 대기 (60*4 sec)
```

⚠️ **isolation과 결정적 차이**: shell은 *케이스 자체가 셸 스크립트* (`<testCaseName>.sh`). isolation은 *runone.sh가 ctl 파일을 처리*. 다른 호출 규약이다.

stdout 파싱 규약: shell은 result 파일(`<tc>.result`)을 별도로 읽음 (`collectGeneralResult`) — isolation처럼 stdout의 `flag: OK` 만 보지 않는다. 케이스 작성자가 result 파일에 기록하는 약속.

---

## 5. RMI 서비스 모드 (`service/Server.java`) — shell 모듈 고유

```
java com.navercorp.cubridqa.shell.service.Server
   ├── load ../conf/shell_agent.conf
   ├── port = AGENT_LOGIN_PORT (default 1099)
   ├── LocateRegistry.createRegistry(port)
   ├── new ShellServiceImpl(props)
   ├── registry.rebind("shellService", impl)
   └── "Service Start!"
```

→ 원격에 데몬으로 띄워 둔 shell agent가 RMI를 통해 셸 명령을 받아 실행. 클라이언트(SSHConnect의 `SERVICE_TYPE_RMI`)는 SSH 대신 이 RMI 채널로 명령을 보낸다.

**용도 추정**: SSH 접속이 비용/제약(예: 폐쇄망에서 SSH 차단, 또는 sudo 비밀번호 인터랙션 회피) 인 환경에서 사전 합의된 데몬을 통해 셸 실행. `Test.runAll`의 RMI alive ping (`echo HELLO`) 이 이 모드에서만 활성.

**새 시스템 의미**: shell 모듈은 단일 jar이지만 사실상 *3가지 모드*로 동작 — (a) SSH 클라이언트로서의 분산 실행, (b) 로컬 단독 실행 (UNITTEST), (c) RMI 서버. 새 시스템에서 이를 어떻게 분리할지(또는 통합할지) ADR 후보.

---

## 6. cross-module 결합의 진실 (isolation/design.md §6 의 후속)

shell 모듈의 `shell.common.*` 11 클래스 중 다음 7개가 isolation/ha_repl/cdc_repl에서 import됨:
- `SSHConnect`, `LocalInvoker`, `ShellScriptInput`, `GeneralScriptInput`, `HttpUtil`, `CommonUtils`, `SyncException`

이 분석으로 cli-tree.md / deps-of-common.md 의 그림이 갱신된다:

```
                         ┌──────────────────┐
                         │    common module │
                         │  (CommonUtils,   │
                         │   IniData, ...)  │
                         └─────────▲────────┘
                                   │
                ┌──────────────────┼──────────────────────┐
                │                  │                      │
        ┌───────┴────┐    ┌────────┴───────┐     ┌────────┴────────┐
        │ shell.main │    │  isolation     │     │  ha_repl/cdc_repl│
        │ ↓          │    │     ↓          │     │     ↓            │
        │ shell.     │◀───┤ also imports   │     │ also imports     │
        │ common.*   │    │ shell.common.* │     │ shell.common.*   │
        └────────────┘    └────────────────┘     └──────────────────┘
                ▲
                │ runtime 의존이지만 빌드 산출물 측면에서는
                │ cubridqa-shell.jar 를 isolation/ha_repl/cdc_repl
                │ 클래스패스에 끌어들여야 한다 (조사 필요)
```

**이는 "isolation을 단독으로 strangler-fig 1차 대체" 가 사실상 불가능함을 의미**. 1차 대체 단위는:
- **A. shell.common.* 만 별도 모듈로 추출** → 의존성 축소, 점진적 대체 가능
- **B. shell + isolation 묶음** → 묶음 하나가 새 시스템에서 첫 번째 작동 단위
- **C. shell.main 의 ssh/rmi/local 3모드 분리** → 큰 작업이지만 후속 모듈 대체가 쉬워짐

ADR-004 ("1차 대체 모듈 선정") 의 결정 근거 강화.

---

## 7. 외부 의존 표면 (동결 대상)

### 7-1. testcases 측 약속
- 케이스는 `<base>/cases/<...>/<name>.sh` 형태 (path에 "cases" 세그먼트 필수)
- 케이스 실행 결과는 `<name>.result` 파일에 기록 (Test.collectGeneralResult가 회수)
- 케이스 자체가 실행 가능한 shell script (`sh <name>.sh`)

### 7-2. 실행 환경에 *반드시* 존재해야 하는 것
- `${CTP_HOME}` 환경 변수 (UNITTEST: `cd ${CTP_HOME}; source shell/local/<TEST_TYPE>.sh`)
- `shell/local/<TEST_TYPE>.sh` 파일 — UNITTEST/JdbcLocalTest의 4 함수 (init/list/execute/finish) 정의
- (HA/dist 모드) ports: cubrid_port_id, broker1.BROKER_PORT, broker2.BROKER_PORT, ha_port_id, cm_port (waitNetReady에서 검사)
- `JAVA_HOME_<VERSION>` 환경 변수 (선택적, 다중 빌드 지원)

### 7-3. RMI 서비스 모드
- `<CTP_HOME>/conf/shell_agent.conf` 파일 (Server.java)
- agent_login_port (기본 1099)

### 7-4. config 표면 (shell.conf 26 / shell_ci.conf 42 키)
- 공통: env.instance*.* / default.* / scenario / testcase_*
- shell_ci 추가: test_platform, test_continue_yn, testcase_git_branch, testcase_update_yn, feedback_type, ...
- (전체는 `_overview/conf-matrix.md`)

---

## 8. 데이터 흐름 (다이어그램)

```
                    ctp.sh
                      │
          ┌───────────┼───────────────┐
          │           │               │
       SHELL/RQG   UNITTEST         WEBCONSOLE
       (Main)   (GeneralLocalTest)  (별도 utility)
          │           │
          ▼           ▼
     ┌─────────┐  ┌──────────────────┐
     │ Context │  │ Context (옵션)   │
     │  envList│  │  TEST_TYPE,       │
     │  ssh+   │  │  TEST_CATEGORY     │
     │  scenario│  │  실제 환경: 로컬  │
     └────┬────┘  └─────┬─────────────┘
          │             │
          ▼             ▼
   ┌──────────┐  ┌─────────────────────┐
   │TestFactory│ │GeneralLocalTest     │
   │ pipeline │ │ invoke("init")      │
   │  ▼ ▼ ▼ ▼ │ │ list = invoke("list")│
   │ check    │ │ for tc in list:     │
   │ update   │ │   invoke("execute") │
   │ deploy   │ │ invoke("finish")    │
   │ test     │ │                     │
   └────┬─────┘ └─────────┬───────────┘
        │                  │
        │ env마다           │ LocalInvoker.exec
        ▼                  ▼
   ┌──────────┐       ┌────────────┐
   │  Test    │       │ 로컬 셸:   │
   │ +Monitor │       │ source     │
   │  per env │       │ shell/local│
   │   ↓      │       │ /<TEST_   │
   │ runTest  │       │  TYPE>.sh  │
   │   _linux │       │            │
   │   _win   │       └────────────┘
   └────┬─────┘
        │
        ▼ via SSH or RMI
   ┌────────────┐
   │ remote env │
   │  cd cases/ │
   │  sh tc.sh  │
   │  → result  │
   │  → .result │
   └────────────┘
        │
        ▼
   ┌────────────┐
   │ Feedback   │
   │ File/DB/   │
   │ Null       │
   └────────────┘
```

---

## 9. 새 시스템 설계 시 권고 (shell 관점)

1. **`shell.common.*` 추출이 strangler-fig 1차 작업**. SSHConnect/LocalInvoker/ScriptInput 계층은 모듈 경계가 아닌 *공유 인프라 레이어*이므로 별도 모듈로 승격해야 한다 (가칭 `remote-exec`). isolation/ha_repl/cdc_repl/shell이 모두 이를 의존하면 strangler 진행 시 한 번만 새 시스템 동등물을 제공하면 된다.

2. **shell 모듈의 3모드(ssh/rmi/local)는 분리 가능**. RMI 서비스(`service/`) 는 *옵션* 컴포넌트로, 새 시스템에서 처음부터 포함할 필요 없음. UNITTEST의 로컬 모드는 entry point만 다를 뿐 실행 모델은 단순(LocalInvoker 만) → 새 시스템에서 자연스럽게 분리.

3. **케이스 = 셸 스크립트** 라는 약속은 동결. 케이스 디렉터리 규약 (`*/cases/*.sh`) 도 동결.

4. **`<tc>.result` 파일 채널** 도 동결. shell 케이스 작성자가 의존하는 result 파일 포맷/위치 규약은 새 시스템에서 그대로 받는다.

5. **`shell/local/<TEST_TYPE>.sh`**는 UNITTEST의 plug-in 인터페이스. 새 시스템에서도 *4 함수 (init/list/execute/finish) 컨트랙트* 를 보존하는 것이 호환성 있는 선택. 단 `EEOOKK` 마커 문자열 같은 in-band signaling은 더 깨끗한 IPC (named pipe / structured stdout) 로 대체 후보.

6. **retry semantics는 dispatcher에 내재화**. `DispatchTicket(retryCount)` + `complete(success, hasCore) → needRetry` 모델은 새 시스템에서도 유지할 만한 좋은 패턴. retry는 testcase 단위가 아니라 *디스패치 단위*에서 관리.

7. **TestMonitor가 활성**이라는 점은 isolation 분석과 비교해서 주목 — shell의 케이스는 더 길거나 행이 잦아 능동적 모니터가 필요하다는 운영적 신호. 새 시스템도 모니터/타임아웃을 first-class로.

8. **TestCaseSVN 지원은 현 시점에서 폐기 후보**. 모든 testcases 레포가 git으로 이주했다면 SVN 경로 제거 ADR.

---

## ROADMAP 갱신

체크리스트의 `analysis/shell/design.md` 항목을 완료(`- [x]`)로 갱신.

**M0 #4 다음 모듈:** sql (Java entry 없음, `sql/bin/run.sh` 27KB가 진짜 entry — 셸 지배 모듈) → medium (sql 변형, 별도 design 분량 적음)
