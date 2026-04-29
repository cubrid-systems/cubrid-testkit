# shell — Implementation Notes (미묘 동작 + 깨지기 쉬운 가정)

**Source:** `cubrid-testtools/CTP/shell/`

design.md 의 시퀀스 위에서, 새 시스템 작성 시 회귀 위험이 큰 미묘한 동작들.

---

## 1. `cases/` path segment 필수 — 어기면 NPE

```java
p = testCase.lastIndexOf("cases");
testCaseDir  = testCase.substring(0, p + 5);
testCaseName = testCase.substring(p + 6);
```

`p == -1` 처리 없음 → 케이스 path 가 `cases` 세그먼트를 *반드시* 포함해야 함. 없으면 substring 에서 -1 인자로 NPE/IndexOutOfBounds.

→ 새 시스템에서 *명시적 검증 + 친절한 에러 메시지* 추가.

---

## 2. RMI alive ping 의 *busy-wait sleep(1)* 패턴

```java
if (serviceProtocolType.equals(SSHConnect.SERVICE_TYPE_RMI)) {
    aliveScript = new ShellScriptInput("echo HELLO");
    try {
        if (!aliveResult.trim().equals("HELLO")) throw new Exception(...);
    } catch (Exception e) {
        this.workerLog.println("ERROR: NOT ALIVE.");
        CommonUtils.sleep(1);
        continue;        // 케이스 pull 안 한 채 워커 루프 재시작
    }
}
```

RMI 서버가 죽었으면 **무한 1초 sleep + retry** — 운영자가 서버를 재시작할 때까지 케이스 pull 하지 않음. 이 모델의 위험: 영구 hang 가능. 새 시스템은 *backoff + max retries → fail with clear message*.

---

## 3. `JAVA_HOME_<VERSION>` 환경 변수 동적 선택

`runTestCase_linux`:
```bash
if [ "$JAVA_HOME_${VERSION}" ]; then
    export JAVA_HOME=$JAVA_HOME_${VERSION}
fi
```

VERSION 은 `context.getVersion()` (e.g. `64BITS`, `MAIN`). 즉 *원격에 multi-jdk* 가 있고 빌드 버전마다 다른 jdk 를 쓸 수 있어야 함. 환경변수 명명 컨벤션(`JAVA_HOME_<VERSION>`) 이 동결 표면.

→ 새 시스템에서 같은 명명 보존 또는 명시적 conf 키 (`java_home_per_version_<VERSION>=...`) 로 승격.

---

## 4. `TEST_BIG_SPACE` 의 *cleanup-on-write* 패턴

```bash
export TEST_BIG_SPACE=$(echo $TEST_BIG_SPACE)
export TEST_BIG_SPACE=`if [ "$TEST_BIG_SPACE" = '' ]; then echo $bigSpaceDir ; else echo $TEST_BIG_SPACE; fi`
if [ "$TEST_BIG_SPACE" != '' ]; then mkdir -p $TEST_BIG_SPACE; rm -rf $TEST_BIG_SPACE/*; fi
```

매 케이스 시작 시 `mkdir -p` + `rm -rf $TEST_BIG_SPACE/*` — 큰 임시 디렉터리를 케이스마다 정리. **위험:** TEST_BIG_SPACE 가 잘못 설정되면 *루트 디렉터리 삭제* 가능. 변수 비어있는지 검사가 *bash if* 만으로 — quoted check 누락 (`""` 가 vs 없는 경우 매끄럽지 않음).

→ 새 시스템에서 path validation 강화 (절대경로 + 안전 prefix 만 허용).

---

## 5. `cygpath` Windows 변환 — Linux 에서도 호출

`runTestCase_windows`:
```bash
if [ "${TEST_BIG_SPACE}" != '' ]; then export TEST_BIG_SPACE=`cygpath ${TEST_BIG_SPACE}`; fi
```

Windows 분기는 항상 `cygpath` 호출. 즉 Cygwin 의존이 *내장*. 새 시스템에서 Windows native (PowerShell / WSL) 로 가려면 cygpath 우회 필요.

---

## 6. `waitNetReady` — Windows 한정 60×4 sec 대기

```java
public void waitNetReady() throws Exception {
    int timeout = 60 * 4;  // 4 minutes
    ShellScriptInput script = "netstat -abfno | grep -E 'TIME_WAIT|FIN_WAIT1|FIN_WAIT2|CLOSING' | grep -E ':<port>...' | wc -l";
    while (true) {
        if (len > timeout) break;
        result = ssh.execute(script).trim();
        if (result.equals("0")) break;
    }
}
```

Windows 에서만 케이스 종료 후 4분 net 정리 대기. **1 케이스에 4분 추가 latency 가능** — 케이스 풀이 큰 윈도우 CI 의 큰 비용.

→ 새 시스템: 윈도우 native 가 SO_REUSEADDR 같은 모던 기법으로 대기 없앨 가능성. ADR 후보.

---

## 7. `EEOOKK` 마커 in-band signaling (UNITTEST)

```java
scripts = scripts + "; echo GPROPSTART\n";
for (String k : keys) {
    scripts = scripts + "echo G_PROPERTY_" + k + "=${" + k + "}EEOOKK\n";
}
```

GeneralLocalTest 가 셸 stdout 에 환경 변수를 마샬링. 패턴: `G_PROPERTY_<KEY>=<value>EEOOKK`. **EEOOKK 가 변수 값 자체에 들어있으면 파싱 깨짐** — 케이스 작성자가 우연히 EEOOKK 를 출력하지 않을 거라는 가정.

→ 새 시스템에서 *명시적 IPC* (named pipe / structured stdout JSON) 로 교체 ADR 후보.

---

## 8. `dispatchLog` 의 retry-aware 동작

```java
this.dispatchLog = new Log(
    CommonUtils.concatFile(currentLogDir, "dispatch_tc_FIN_" + currEnvId + ".txt"),
    false,                                          // append 가 아닌 truncate
    laterJoined ? true : context.isContinueMode()   // 단, laterJoined 이면 append
);
```

**의미:** continue mode 또는 hot-reload 로 추가된 env 는 기존 로그를 *append*. 신규 fresh run 은 *truncate*. 새 시스템에서 같은 정책 보존 (continue / hot-reload 모두 의존).

---

## 9. `addSshInfoScript` 의 비밀번호 노출 위험

`runTestCase_linux` 내부에서 호출되는 `addSshInfoScript(script)` (Test.java; method body 본 분석 미확인). 추정: env 의 SSH 비밀번호/사용자 정보를 케이스 환경에 export. **위험**: 케이스 stdout 에 비밀번호가 echo 될 가능성. `set -x` 가 켜져 있으면 더 위험.

→ 새 시스템에서 secret 은 *환경 변수가 아닌 별도 채널* (key-vault / per-process socket) 로 전달 ADR.

---

## 10. `enableSaveNormalErrorLog` 분기

```java
if (testCaseSuccess == false && hasCore == false && context.getEnableSaveNormalErrorLog() == true) {
    String saveErrorLogResult = doSaveNormalErrorLog();
    resultCont.append(saveErrorLogResult).append(...);
}
```

*core 없이 실패한* 케이스만 추가 로그 보관 — core dump 와 일반 실패를 구분. 새 시스템에서 같은 분류 (core / non-core fail / pass) 정책 유지.

---

## 11. `dropTestCaseAfterTest` 의 부수 효과

```java
this.needDropTestCase = context.needDeleteTestCaseAfterTest();
...
if (needDropTestCase) {
    dropTestCaseAfterTest();   // testcases 디렉터리에서 케이스 *물리 삭제*
    dispatchLog.println(...);
}
```

`delete_testcase_after_each_execution_yn=true` (shell_ci.conf 전용) 시 *케이스 파일 삭제*. CI 환경의 디스크 공간 절약 의도. **위험:** testcases 레포가 read-only 마운트면 fail; non-write user 면 silently fail.

→ 새 시스템에서 *별도 working copy* 모델 ADR — testcases 레포는 절대 수정하지 않고 working dir 에서 작업.

---

## 12. `startConfigMonitor` 의 5초 polling

```java
while (!Dispatch.isFinished()) {
    CommonUtils.sleep(5);
    context.reload();    // INI 다시 읽음
    // env 차집합 계산 → 추가/삭제 처리
}
```

5초 간격 polling. **위험:** conf 파일 쓰기 도중 부분 읽기 가능 (atomic write 의존). 또 conf 가 매우 자주 바뀌면 polling 누락 가능.

→ 새 시스템에서 inotify / fsnotify (filesystem event) 기반으로 교체 검토.

---

## 13. `concurrentTest` 가 `synchronized` 메서드

```java
private synchronized void concurrentTest(ArrayList<String> envList, boolean laterJoined) throws Exception
```

전체 메서드 락 — startConfigMonitor 가 joinTest → concurrentTest 호출할 때 fresh run 의 호출과 직렬화. 짧은 메서드라 영향 없으나 *직렬화 정책 의도* 명시.

→ 새 시스템에서 명시적 mutex 또는 single-writer 패턴.

---

## 14. `commonforjdbc.jar` — *별도 빌드 산출물*

build.xml 의 jar 타겟 안:
```xml
<jar jarfile="shell/init_path/commonforjdbc.jar" basedir="${build}">
    <include name="common/*.class" />
</jar>
```

basedir 의 `common/*.class` 를 jar 로 묶음 — 패키지 prefix 없는 *최상위 common* 클래스만 (deps-of-common.md 의 cubridqa-common.jar 와는 다른 영역). 이 jar 가 init_path 에 함께 deploy 되어 *케이스가 직접 사용* 하는 JDBC 헬퍼.

→ 새 시스템에서 케이스용 JDBC 헬퍼 라이브러리를 어떻게 제공할지 결정 (별도 maven artifact / classpath 동봉 / unbundled).

---

## 15. `Test.runTestCase` 가 *exception 잡지 않음*

```java
try {
    consoleOutput = runTestCase();
    doFinalCheck();
    collectGeneralResult();
} catch (Exception e) {
    this.addResultItem("NOK", "Runtime error (" + e.getMessage() + ")");
}
```

exception 발생 시 NOK 로 기록 후 *케이스 루프 계속*. 즉 한 케이스 SSH 실패가 전체 워커를 죽이지 않음 — 안정성 정책.

→ 새 시스템에서 같은 fault-isolation 정책 유지.

---

## 16. `lastPassResultCont` 의 부분 정보

```java
String lastPassResultCont = buildLastPassResultCont(resultContString, consoleOutput, testCaseSuccess);
```

성공한 retry 의 결과만 별도 보관 — fail → retry → pass 시나리오에서 *마지막 pass 의 출력만* Feedback 에 보냄. UI 측에서 retry 흔적을 어떻게 표현할지 결정. 새 시스템에서 retry 는 *별도 evt* 로 보내거나 *최종 결과만* 보내거나 결정 필요.

---

## 17. `ManualReportJob` — 정체 미확인

`shell/main/ManualReportJob.java` (12 main 파일 중 하나) — Quartz scheduler job 으로 추정 (sched 의존). 본 분석에서 정확한 진입점 / trigger 확인 미완. 후속 작업으로 분리.

---

## 18. `RunShellMain` — 정체 미확인

마찬가지로 `shell/main/RunShellMain.java` (Main.java 와 별개). 별도 진입점일 가능성 — 추가 조사 후속.
