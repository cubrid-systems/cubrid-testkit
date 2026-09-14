# isolation — Implementation Notes (미묘 동작·재현 어려운 부분·깨지기 쉬운 가정)

**Source:** `cubrid-testtools/CTP/isolation/`

design.md 의 시퀀스 위에서, 새 시스템 작성 시 **잊으면 회귀가 일어나는** 미세한 동작들을 정리한다.

---

## 1. SSH 새 연결을 *매 케이스* 마다 — 의도된 안정성 비용

`Test.runAll` 은 케이스 루프 안에서 `resetSSH()` 를 호출 (Test.java:108):
```java
while (!shouldStop && !Dispatch.getInstance().isFinished()) {
    ...
    resetSSH();              // 매 케이스 새 SSH connection
    resultItemList.clear();
    ...
    testCaseSuccess = runTestCase();
    ...
}
```

**왜?** 케이스 실행 중 락 해소 실패 / SSH 채널 hang 발생 시 다음 케이스가 *오염되지 않도록* 의도적 분리. 성능 비용 (커넥션 수립 ~수백 ms × 6,778 케이스 × N env) 이 크지만 **결정론 / 안정성 우선 정책**.

새 시스템에서 **connection pooling 도입은 신중하게**: runone.sh 가 환경 mutation 을 포함할 가능성 있음. ADR 로 동작 호환성 검증 필요 (design.md §10-3 참조).

---

## 2. `processCoreFile` 는 코드에 있으나 *호출되지 않음*

`Test.processCoreFile()` 메서드 (Test.java:256~) 는 `find $CUBRID ${CTP_HOME} ${init_path} <testCaseDir> -name 'core.*'` 로 core 파일을 탐색하고 NOK 처리 + rm 한다.

```java
// processCoreFile();  ← 명시적으로 주석 처리됨
```

`runTestCase()` 끝에서 호출이 *주석 처리된 상태*. 즉 **로컬 측 자동 core 정리는 작동하지 않는다** — runone.sh 가 stdout 으로 `found core file` 만 보고. 정리는 후속 단계 (TestFactory.backupTestResults 의 tar 가 currentLogDir 만 백업) 또는 외부 운영자에 의존.

**위험:** core 파일이 원격 env 에 누적될 수 있음. `backup_core_file_yn=false` 시 runone.sh 가 자체적으로 core 를 제거하는 것으로 추정 — runone.sh 정밀 분석은 후속.

---

## 3. `TestMonitor` 도 비활성

`TestFactory.concurrentTest` 안에서 TestMonitor 워커를 만드는 코드는 *주석 처리됨* (TestFactory.java:191~):
```java
// final TestMonitor monitor = new TestMonitor(context, test);
// testPool.execute(...);  ← 주석
```

shell 모듈은 활성, isolation 만 비활성. **이유 추정:** runone.sh 의 `<timeout_sec>` 인자가 케이스 단위 타임아웃을 *내부에서* 다루므로 외부 모니터가 불필요. *경합 없음 보장* 측면에서 의도된 정책.

새 시스템에서 모니터 활성화 여부는 ADR — TestMonitor 의 정확한 책임을 분명히 한 후 결정.

---

## 4. `startConfigMonitor` 도 주석 처리 (env hot-reload 비활성)

`TestFactory.execute()` 끝부분:
```java
// startConfigMonitor();
```

shell 모듈은 활성. isolation 은 비활성. **이유 추정:** isolation 의 multi-instance 토폴로지는 정적이라는 가정 (test 시작 후 env 추가/제거 일이 드뭄). shell 은 분산 CI 에서 노드 추가/제거 일이 잦음.

새 시스템 isolation 도 동일 정책 유지가 안전.

---

## 5. `system.setProperty("sun.rmi.transport.connectionTimeout", "10000000")`

Main.exec 첫 줄에서 RMI 타임아웃을 *10,000,000 ms = ~2.78 시간* 으로 설정. **왜?** isolation 워커가 SSH 채널을 매우 오래 유지할 수 있고, 내부 RMI 호출 (어디?) 이 timeout 으로 죽는 것을 막기 위해 *사실상 무한대* 로 늘림.

**미묘:** 이 RMI 의 정체는 분명치 않음 — shell 의 ShellService 같은 명시적 RMI 가 isolation 에는 없는 것으로 보임. `cqt` 가 RMI 를 쓰는지 또는 jsch 내부 RMI 인지 추가 조사 후속.

새 시스템에서 *불필요* 할 가능성 — Java 외 언어로 가면 자동 제거.

---

## 6. `calcScenario` 의 SSH 핸드오버

```java
private static String calcScenario(Context context) throws Exception {
    SSHConnect ssh = IsolationHelper.createFirstTestNodeConnect(context);
    String homeDir = ssh.execute(new IsolationScriptInput("echo $(cd $HOME; pwd)")).trim();
    ...
    if (scenarioDir.startsWith(homeDir))  
        scenarioDir = scenarioDir.substring(homeDir.length() + 1);  // relative
    else
        return context.getTestCaseRoot().trim();                    // absolute
}
```

scenario 가 $HOME 의 자손이면 *상대경로* 로 변환, 아니면 *절대경로* 그대로. **Test.runTestCase 의 case 절대경로 normalize** 와 짝지어진 동작:
```java
String tc = testCaseFullName.trim();
if (tc.startsWith("/") == false) {
    tc = "$HOME/" + tc;
}
```

→ `$HOME` 이 case 측에 *prefix 로 다시 붙는다*. 즉 *Java 측 scenario 가 상대경로면 SSH 측 case 가 $HOME prefix 로 절대경로 복원* — 상대/절대 경로 변환을 두 곳에서 짝지어 처리하는 구조. 새 시스템에서도 같은 약속을 유지하거나, 한 쪽만 절대경로로 통일.

---

## 7. `extractItems` 의 *마지막 일치* 우선 규칙

Test.runTestCase 결과 판정:
```java
int p1 = result.lastIndexOf("flag: NOK");
int p2 = result.lastIndexOf("flag: OK");
if (p1 > p2) {
    passFlag = false;
}
```

**마지막 NOK 와 마지막 OK** 의 위치 비교. 즉 stdout 에 NOK 가 먼저 나왔어도 마지막에 OK 가 있으면 통과. retry 메커니즘 동작 (한 번 실패 → 재시도 → 통과 시 OK 가 마지막) 과 정합.

새 시스템에서 같은 정책 유지 또는 retry 자체를 *케이스 단위로 분리해서 마지막 결과만 보고* (단순화) — 후자 권고.

---

## 8. `addSkippedTestCases` 의 음수 elapseTime

```java
feedback.onTestCaseStopEvent(tc, false, -1, "", "", false, false, skippedType);
```

skip 케이스의 elapseTime 을 `-1` 로 보고 — *signal value*. 0 은 "instant" 의미를 갖기 때문에 부족. 새 시스템에서도 skip 의 시간은 명시적 magic value (또는 nullable) 로 표현.

---

## 9. `Dispatch.singleton` 의 process-local 한계

`Dispatch.instance` 는 *static* — JVM 단일 프로세스 안에서만 thread-safe. 다중 host 워커 (예: 다른 머신의 Java 프로세스) 가 동일 풀에서 pull 하려면 *외부 broker* 필요.

현재는 multi-instance = 같은 JVM 프로세스 안의 thread 들. ROADMAP §7 의 "분산 실행으로 가려면 file-backed dispatch 를 SQLite/Redis 로 승격" 권고와 일관.

---

## 10. `feedback_type=db` 의 비공식 의존

`FeedbackDB` 가 어떤 DB 스키마를 사용하는지 본 분석에서는 미확정 (impl/FeedbackDB.java 정밀 분석은 Phase 1 후속). cubridqa-feedback 같은 외부 도구가 이 DB 를 *읽는다면* 스키마는 동결 표면.

**위험:** 새 시스템이 DB 스키마를 *모르고* 변경하면 외부 모니터링 깨짐. ADR 로 명시화 필요.

---

## 11. `IsolationHelper.createTestNodeConnect` vs `createFirstTestNodeConnect`

두 메서드 분리 — `createFirstTestNodeConnect` 는 `envList[0]` 에 대해서만 (build 정보 / scenario 검증). `createTestNodeConnect(envId)` 는 임의 env. 가정: **envList[0] 가 항상 *대표 노드*** — multi-instance 에서 한 노드가 build 정보를 가질 수 있다는 가정.

새 시스템에서 same property 보장 (또는 *대표 노드 명시 키* 도입).

---

## 12. `system property TEST_ID` 누락 위험

Test.runTestCase 시작 시:
```java
script.addCommand("export TEST_ID=" + this.context.getFeedback().getTaskId());
```

원격 셸에 TEST_ID 환경 변수를 export. 이 변수를 *runone.sh 또는 testcase 가 사용* 한다면 동결 표면. 본 분석에서 정확한 사용처 미확정 — runone.sh 정밀 분석 시 확인.

---

## 13. `cubrid_drv.c` / `mysql_drv.c` / `oracle_drv.cpp` — 다중 DB driver 지원

ctltool 안에 CUBRID 외 MySQL/Oracle driver 도 존재. ctltool 자체는 *DB 타입을 인자로 받아 적절한 driver 로 dispatch*. CTP 본체는 CUBRID 만 사용하지만 ctltool 은 더 넓은 호환성 보유.

새 시스템에서 ctltool 을 *흡수* 하면 이 다중 driver 능력을 보존할지 결정 필요. 사용되지 않는 코드라면 cleanup, 사용된다면 ADR.

---

## 14. `[ENV START]` / `[ENV STOP]` / `[TESTCASE]` 콘솔 라인 형식

Test.runAll 이 콘솔에 출력하는 라인:
```
[ENV START] <envId>
[TESTCASE] <tc> EnvId=<envId> [OK]|[NOK]
[ENV STOP] <envId>
```

이 라인을 *외부 운영자나 CI 시스템이 grep* 할 가능성. 새 시스템에서 같은 형식 유지.

---

## 15. `cubrid service stop` 으로 워커 종료

Test.stopCUBRIDService() — 워커가 종료할 때 cubrid service 를 stop. **다음 워커가 같은 env 에 들어오면 다시 start 해야 함**. 이 start 는 deploy 또는 테스트 케이스 자체가 담당 (runone.sh 가 service start 를 cleanup 으로 호출하는지 후속 분석).

새 시스템에서 *서비스 라이프사이클 관리 책임 명시* 가 ADR 후보.
