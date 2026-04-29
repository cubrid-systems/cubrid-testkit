# isolation — Design (Main 시퀀스 분석)

**Source:** `cubrid-testtools/CTP/isolation/`

**Phase 0 M0 #4 status:** 완료 (sequence-level seed)

---

## 1. 클래스 구성 (17 Java 파일)

```
com.navercorp.cubridqa.isolation
├── Main                  ← 진입점 (CTP에서 reflection 호출)
├── Context               ← config 모델, env list, scenario 경로
├── TestFactory           ← 오케스트레이션 (전체 라이프사이클)
├── Test                  ← 환경(env) 1개당 워커 — 케이스 루프
├── TestMonitor           ← 워커 모니터 (현재 비활성: TestFactory 안 주석 처리됨)
├── CheckRequirement      ← deploy 직전 환경 사전 검사
├── IsolationHelper       ← SSH 커넥션 빌더, env 메타데이터 추출
├── IsolationScriptInput  ← 다중 라인 SSH 명령 빌더 (shell.common.ScriptInput 확장)
├── Constants             ← 결과 코드, 라인 분리자, skip type 등
├── Feedback              ← 결과 보고 인터페이스
├── deploy/Deploy             ← 환경별 CUBRID 설치/구성
├── deploy/DeployOneNode      ← 단일 노드 배포 단위
├── deploy/TestCaseGithub     ← testcases 레포 git 동기화
├── dispatch/Dispatch         ← 케이스 풀 (singleton, thread-safe nextTestFile)
├── impl/FeedbackFile         ← Feedback: 파일 출력 구현
├── impl/FeedbackDB           ← Feedback: DB 출력 구현
└── impl/FeedbackNull         ← Feedback: no-op 구현
```

설계 패턴 요약:
- **Pipeline + Pool**: TestFactory가 큰 단계(check → update → deploy → test)를 차례로 실행, 각 단계는 env 리스트에 대한 ExecutorService 병렬 처리.
- **Singleton dispatcher**: `Dispatch` 가 process 단위 단일 인스턴스로 케이스 풀을 보유. 워커 `Test` 가 `Dispatch.getInstance().nextTestFile()` 로 thread-safe pull.
- **Strategy**: `Feedback` 인터페이스 + 3개 구현체. config 의 `feedback_type` 으로 선택.

---

## 2. 진입점 시퀀스 (Main.exec)

```
ctp.sh sql ... isolation -c isolation.conf
   │
   ▼
CTP.main → executeIsolation(config, "isolation")
   │ (URLClassLoader: isolation/lib/cubridqa-isolation.jar)
   ▼
isolation.Main.exec(configFilename)
   │
   ├── system property: sun.rmi.transport.connectionTimeout = 10_000_000
   │
   ├── new Context(configFilename)
   │      ├── reload() → load INI, init envList, ctpHome, flags
   │      ├── setLogDir("isolation")     // log 루트 결정
   │      └── envList 비면 isExecuteAtLocal=true, envList=["local"]
   │
   ├── envList.size() == 0  → throw "Not found any environment instance"
   │
   ├── if (cubridPackageUrl 존재):
   │      ├── isAvailableURL(buildUrl)  ← HTTP/FTP 확인, 실패 시 System.exit(-1)
   │      ├── context.setBuildId / setBuildBits  ← URL에서 파싱
   │      └── reInstallTestBuildYn = true
   │   else:
   │      ├── envList[0] 의 SSH로 접속  ← IsolationHelper.createTestNodeConnect
   │      ├── shell.common.CommonUtils.getBuildVersionInfo(ssh)
   │      │    ⚠️  shell 모듈의 utility 호출 (cross-module dependency, §6 참조)
   │      └── reInstallTestBuildYn = false
   │
   ├── calcScenario(context)  ← SSH로 scenario 디렉터리 절대경로 해석
   │      └── 실패 시 [ERROR] 프린트 후 return (exit code 0)
   │
   ├── new TestFactory(context)
   │      ├── testPool   = newFixedThreadPool(100)   ← env당 1워커이지만 풀 크기 100
   │      ├── configPool = newFixedThreadPool(1)
   │      └── feedback   = context.getFeedback()
   │
   └── factory.execute()
```

---

## 3. TestFactory.execute() 라이프사이클

```
execute()
  │
  ├── if (continueMode):
  │      ├── createSnapshotForConfiguration  ← Properties → main_snapshot.properties
  │      ├── feedback.onTaskContinueEvent
  │      ├── Dispatch.init(context)          ← 이전 dispatch_tc_ALL.txt 재로드
  │      │                                     - dispatch_tc_FIN_<env>.txt 들과 차집합
  │      ├── totalTbdSize == 0 → "NO TEST CASE TO TEST WITH CONTINUE MODE!" return
  │      ├── checkRequirement(context)       ← env별 CheckRequirement (병렬 X, 순차)
  │      └── concurrentUpdateScenarios       ← cachedThreadPool, env마다 TestCaseGithub
  │
  │   else (fresh run):
  │      ├── cleanFilesByDirectory(currentLogDir)
  │      ├── createSnapshotForConfiguration
  │      ├── checkRequirement                ← deploy 전에 사전 점검 (실패 시 System.exit(-1))
  │      ├── feedback.onTaskStartEvent(buildUrl)
  │      ├── concurrentUpdateScenarios       ← env마다 testcases 레포 git pull
  │      └── Dispatch.init(context)
  │             ├── findAllTestCase()        ← env[0]에 SSH로 `find <root> -name *.ctl`
  │             ├── findExcludedList()       ← config의 testcase_exclude_from_file 적용
  │             └── dispatch_tc_ALL.txt 작성
  │
  ├── feedback.setTotalTestCase(total, macroSkipped, tempSkipped)
  ├── if (!continueMode): addSkippedTestCases(macroSkipped, tempSkipped)
  │
  ├── concurrentDeploy(envList)              ← env마다 Deploy.deploy() 병렬 실행
  │       ⚠️ Deploy.java가 env에 CUBRID 설치 (실제 작업은 SSH 통한 셸 스크립트 호출)
  │
  ├── concurrentTest(envList)
  │       └── env마다 Test 인스턴스 → testPool.execute(test::runAll)
  │
  ├── while (!isAllTestsFinished()) sleep(1)  ← polling
  │       └── Dispatch.isFinished() && 모든 Test.isStopped() 검사
  │
  ├── testPool.shutdown(); configPool.shutdown()
  │
  ├── feedback.onTaskStopEvent
  │
  └── backupTestResults                      ← !Windows 만, tar zcf isolation_result_*.tar.gz
```

---

## 4. Test.runAll() — 워커 루프 (env 1개당)

```
runAll() — currEnvId 단일 워커
  │
  └── while (!shouldStop && !Dispatch.isFinished()):
        │
        ├── tc = Dispatch.nextTestFile()        ← thread-safe singleton pull
        ├── tc == null → break
        │
        ├── feedback.onTestCaseStartEvent(tc, envIdentify)
        ├── resetSSH()                          ← 매 케이스 SSH 새로 연결
        ├── runTestCase()
        │      ├── IsolationScriptInput script:
        │      │     ulimit -c unlimited
        │      │     export TEST_ID=<taskId>
        │      │     cd $ctlpath
        │      │     sh runone.sh [-n] -r <retries+1> <tc> <timeout> <db>  ⚠️ §5
        │      ├── result = ssh.execute(script)
        │      ├── parse stdout:
        │      │     last "flag: NOK" vs last "flag: OK" → passFlag
        │      │     "found core file" / "found fatal error" → hasCore=true, fail
        │      ├── return passFlag
        │
        ├── if (!success):
        │       └── showDifferenceBetweenAnswerAndResult(tc)
        │           └── SSH로 `diff -a -y -W 185 <tc>.answer <tc>.log`
        │
        ├── feedback.onTestCaseStopEvent(tc, success, elapsed, resultText, env, isTimeOut, hasCore, skipType)
        └── dispatchLog.println(tc)             ← dispatch_tc_FIN_<env>.txt 추가
  │
  ├── stopCUBRIDService()                       ← `cubrid service stop` SSH 명령
  ├── close() → SSH/log 닫기
  └── feedback.onStopEnvEvent(currEnvId)
```

---

## 5. 외부 의존 (반드시 새 시스템에서 호환 유지해야 할 표면)

### 5-1. 원격 환경에 *반드시* 존재해야 하는 것
| 자원 | 용도 | 비고 |
|------|------|------|
| `runone.sh` | 케이스 1건 실행 (`sh runone.sh [-n] -r <retry> <tc> <timeout> <db>`) | 원격 측 스크립트 — CTP repo에 없음. **testcases 레포 또는 deploy 단계가 배치한다** (확정 필요) |
| `$ctlpath` | runone.sh 가 cd 하는 작업 디렉터리 | 원격 셸 환경변수 |
| `$CUBRID`, `$CTP_HOME`, `$init_path` | core dump 탐색 시 사용 (Test.processCoreFile) | 단, processCoreFile은 현재 호출되지 않음 (주석) |
| `<tc>.answer`, `<tc>.log` | diff 비교 대상 | 케이스 디렉터리에 함께 위치 |
| `cubrid service stop` 커맨드 | 워커 종료 시 호출 | CUBRID 설치 PATH 필요 |
| `*.ctl` 파일 | 테스트 케이스 (find로 수집) | 케이스 포맷의 정의는 별도 분석 (M0 #5 case-formats.md) |

### 5-2. 결과 파일 (currentLogDir 안)
- `main_snapshot.properties` — 시작 시점 config 스냅샷
- `dispatch_tc_ALL.txt` — 전체 케이스 목록 (continueMode 입력)
- `dispatch_tc_FIN_<envId>.txt` — env별 완료 케이스 (continueMode resume용)
- `test_<envId>.log` — env별 워커 로그 (케이스별 result + diff)
- `isolation_result_<buildId>_<bits>_<taskId>_<ts>.tar.gz` — currentLogDir 통째 백업

### 5-3. Feedback 출력
- `FeedbackFile` — 로컬 파일에 기록
- `FeedbackDB` — DB에 적재 (어떤 DB인지 config로 결정)
- `FeedbackNull` — 비활성

선택 기준은 config 의 `feedback_type`. 새 시스템에서도 이 키 의미는 동결.

### 5-4. config 키 (isolation.conf 27개) — 동결 대상 표면
- multi-instance: `env.instance{1,2}.{ssh,broker,cubrid}.*`, `default.{ssh,cubrid,broker,cm,ha}.<property>`
- 실행 제어: `scenario`, `testcase_retry_num`, `testcase_timeout_in_secs`
- 동작 플래그: `backup_core_file_yn` (isolation 전용)
- (전체 목록은 `_overview/conf-matrix.md` 참조)

---

## 6. 숨겨진 cross-module 의존: `shell.common.*`

isolation 모듈은 표면적으로 `cubridqa-isolation.jar` 한 개로 빌드되지만, 다음 7개 클래스를 **shell 모듈에서 가져온다**:

| isolation에서 import | shell의 위치 | 용도 |
|---------------------|--------------|------|
| `shell.common.SSHConnect` | shell/src/.../shell/common/SSHConnect.java | jsch 기반 SSH 클라이언트. **모든 원격 통신의 진짜 진입점** |
| `shell.common.LocalInvoker` | shell/src/.../shell/common/LocalInvoker.java | 로컬 셸 실행 (TestFactory.backupTestResults 의 tar 호출) |
| `shell.common.ShellScriptInput` | shell/src/.../shell/common/ShellScriptInput.java | 다중 라인 셸 명령 빌더 (IsolationScriptInput의 부모) |
| `shell.common.GeneralScriptInput` | 〃 | (deploy/CheckRequirement에서 사용) |
| `shell.common.HttpUtil` | 〃 | (deploy 단계에서 build URL 다운로드) |
| `shell.common.CommonUtils` | shell/src/.../shell/common/CommonUtils.java | `getBuildVersionInfo(ssh)` — Main.exec 안에서 직접 호출 |
| (잠재) `shell.common.SyncException` | (ha_repl/cdc_repl는 사용, isolation은 직접 없음) | — |

⚠️ **이는 cli-tree.md 분석 시 드러나지 않은 결합** — CTP.executeIsolation 은 `URLClassLoader(isolation/lib/cubridqa-isolation.jar, parentCL)` 로 격리해서 로드하지만, isolation jar 의 클래스들이 `shell.common.*` 를 참조한다. 따라서 *런타임 classpath 에 cubridqa-shell.jar 가 있어야* 실제로 로드된다.

이 classpath 가 어떻게 충족되는지는 추가 조사 필요 (M0 후속 항목으로 분리):
- 가능성 1: isolation/lib 의 MANIFEST.MF 의 `Class-Path:` 헤더에 `../shell/lib/cubridqa-shell.jar` 가 적혀 있다
- 가능성 2: ctp.sh 가 build 산출 시 모든 모듈 jar를 한 디렉터리에 모은다
- 가능성 3: parent ContextClassLoader 가 jvm 시작 시 `-cp` 로 모든 jar를 받았다 (그러나 ctp.sh 는 common jar 1개만 -cp에 둠 → 모순)

→ **새 시스템에서는 `shell.common.*` 영역이 isolation/ha_repl/cdc_repl 의 진짜 공통 인프라**라는 점이 명확하므로, 이를 **별도 "remote-exec" 공통 모듈**로 승격하는 것이 strangler-fig 1차 분리 시점의 자연스러운 경계가 된다.

---

## 7. 데이터 흐름 (다이어그램)

```
                    ┌──────────────┐
                    │  CTP.main    │
                    └──────┬───────┘
                           │ reflection
                           ▼
                    ┌──────────────┐
                    │ Main.exec()  │
                    └──────┬───────┘
            ┌──────────────┴──────────────┐
            │                             │
            ▼                             ▼
    ┌──────────────┐              ┌──────────────┐
    │  Context     │◀─────────────│ INI config   │
    │  (envList,   │              │  (multi-     │
    │  feedback,   │              │  instance,   │
    │  scenario)   │              │  scenario)   │
    └──────┬───────┘              └──────────────┘
           │
           ▼
    ┌──────────────┐
    │ TestFactory  │
    │ .execute()   │
    └──┬─┬─┬─┬─┬───┘
       │ │ │ │ └─► concurrentUpdateScenarios → TestCaseGithub × N envs (parallel)
       │ │ │ └───► concurrentDeploy → Deploy × N envs (parallel)
       │ │ └─────► Dispatch.init → SSH find *.ctl → tbdList
       │ │
       │ └───────► concurrentTest → Test × N envs (testPool, runAll)
       │                              │
       │                              ▼
       │                    ┌────────────────┐
       │                    │ Test.runAll    │ (env 1개)
       │                    │   while not    │
       │                    │   finished:    │
       │                    │     pull tc    │ ←──┐
       │                    │     SSH exec   │    │ Dispatch
       │                    │     parse out  │    │ singleton
       │                    │     feedback   │    │
       │                    └───────┬────────┘    │
       │                            │             │
       │                            ▼ via SSH    │
       │                    ┌────────────────┐    │
       │                    │ remote env     │    │
       │                    │ runone.sh      │    │
       │                    │ runs *.ctl     │    │
       │                    └────────────────┘    │
       │                                          │
       └─► polling loop until all Tests stopped ──┘
                       │
                       ▼
                 backupTestResults (tar.gz)
```

---

## 8. 새 시스템 설계 시 권고 (isolation 관점)

1. **Dispatch → Test 의 producer/consumer 패턴은 재사용 가능**. queue/channel 추상화 + thread-safe pull 은 모던 언어/플랫폼에서 자연스럽게 표현됨.

2. **`runone.sh` 인터페이스는 동결 대상**. CTP가 testcases 레포의 `runone.sh` 에 의존(원격 측 위치)하므로, 새 시스템에서도 동일 호출 시그니처를 보내야 함:
   ```
   sh runone.sh [-n] -r <retry+1> <tc> <timeout> <db>
   ```
   stdout 파싱 규약(`flag: OK`/`flag: NOK`/`found core file`/`found fatal error`)도 동결.

3. **per-test SSH reset은 안정성 보장이지만 성능 비용이 큼**. 새 시스템에서는 케이스 그룹 단위 connection pooling을 도입할 가치가 있음 — 단, runone.sh가 환경 mutation을 포함할 가능성이 있으므로 ADR로 동작 호환성 확인 필요.

4. **Dispatch singleton 은 multi-process 실행 시 한계**. 새 시스템이 분산 실행(여러 호스트의 worker)으로 가려면 file-backed dispatch (현재 dispatch_tc_ALL.txt 가 이미 그렇게 동작) 를 SQLite/Redis 같은 명시적 broker로 승격하는 것이 자연스러움.

5. **Feedback 인터페이스 분리는 잘 되어 있음**. File/DB/Null 3구현은 그대로 살리고, 새 시스템에서는 +Slack/+S3 등 추가 구현을 plug-in 형태로.

6. **`shell.common.*` 의존을 받아들여야 한다** — 분석 직후 immediate insight (§6). isolation 단독 strangler-fig 대체는 어렵고, **isolation + shell.common 한 묶음**이 자연스러운 첫 번째 대체 단위. 이는 향후 ADR-004 ("1차 대체 모듈 선정") 의 강력한 근거.

7. **`ulimit -c unlimited` + core dump 탐색** 은 디버깅용이지만 현재 processCoreFile은 비활성. 새 시스템에서는 명시적 core dump 정책 (자동 압축/보관/통보)을 ADR로 표면화.

---

## ROADMAP 갱신

체크리스트의 `analysis/isolation/design.md` 항목을 완료(`- [x]`)로 갱신.

**M0 #4 다음 모듈:** shell (다음 라운드) — 본 분석에서 드러난 `shell.common.*` 가 진짜 공통 인프라라는 점을 검증해야 한다.
