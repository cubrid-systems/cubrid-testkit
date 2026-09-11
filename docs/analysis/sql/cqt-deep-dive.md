# sql — cqt Deep Dive: ConsoleBO.runTest 와 35 utility 클래스 협업

**Source:** `cubrid-testtools/CTP/sql/src/com/navercorp/cubridqa/cqt/`

`sql/design.md` §4 / `sql/implementation-notes.md` §8 에서 미해결로 둔 *cqt 의 35+ utility 클래스 깊이* 정밀 분석. ConsoleBO.runTest 의 1528 라인을 *9 phase pipeline* 으로 정리.

**Phase 4 정밀 분석 status:** 완료

---

## 1. ConsoleBO 의 9-phase pipeline

`ConsoleBO.runTest(Test test): Summary` (line 131-252) 의 정밀 흐름:

```
1. ProcessMonitor 초기화
2. init()                   ← dbVersion/dbBuild 수집 (line 1493)
3. new ConsoleDAO(test)     ← DB 접근 객체
4. buildTest(test)          ← case 목록 + 메타 구성 (§2)
5. checkDb(test)            ← DB 연결 검증 — fail 시 "-10000" 출력 + return null
6. createResultDirs(test)
7. (옵션) summary XML head 작성
8. execute(test)            ← ★ 메인 루프 (§3)
9. (사후) saveResults / saveAnswers / saveTempResults / saveResultSummary
   + finally: dao.release()
```

**runMode 분기 (Test.MODE_*):**
- `MODE_NO_RESULT` (0) — 답안 없이 실행 (smoke test)
- `MODE_MAKE_ANSWER` (1) — 결과를 *.answer 로 저장 (answer 생성 모드)
- `MODE_RESULT` (2) — 답안 보유 + 결과를 result/ 에 저장 (회귀 검증 모드, default)
- `MODE_RESULT_TO_SCENARIO` (3) — 결과를 scenario 디렉터리에 저장
- `MODE_RUN` — 일부 분기에서 사용

→ run.sh do_test 의 default JDBC 분기는 `MODE_RESULT` (회귀 검증). MODE_MAKE_ANSWER 는 *answer 갱신* 시 별도로 사용.

---

## 2. buildTest — case 목록 + 메타 구성

`buildTest` (line 290) = `getCaseFiles` + `build` 두 단계.

### 2-1. `getCaseFiles` (line 302) — URL specifier 파싱

```java
// Test.cases[] 의 각 entry 형식:
//   <scenario_path>?db=<dbId>&filter=<filter_path>
String[] files = test.getCases();
for (each file) {
    if file contains "?db=":
        scenario_path = file.substring(0, "?db=" position)
        dbId = ...
        filter = ... (optional)
        test.setDbId(dbId)
        test.setCaseFilter(filter)
        test.setScenarioRootPath(scenario_path)
        dao.addDb(dbId)
    
    String[] postFixes = TestUtil.getCaseFilePostFix(file)
    TestUtil.getCaseFiles(test, file, test.getCaseFileList(), postFixes)
    TestUtil.filterExcludedCaseFile(test.getCaseFileList(), filter, file)
}
```

**해석:**
- URL-style scenario specifier (sql/io-contract.md §4-1) 파싱
- `getCaseFilePostFix(file)` — 케이스 확장자 결정 (`.sql`, `.api`, `.groovy`?)
- `getCaseFiles` — 디렉터리 트리에서 모든 case 파일 수집
- `filterExcludedCaseFile` — exclusion 적용

**미묘:** `getCaseFiles` 메서드에는 `@deprecated` 표기 — 옛 메커니즘이 *deprecated 되었지만 여전히 호출* 됨. 새 시스템에서 *정리/대체* 후보.

### 2-2. `build` (line 341) — CaseResult 메타 구축

각 case 파일에 대해 `CaseResult` 객체 생성:

```java
caseResult.setShouldRun(...)
caseResult.setType(testType)      // SQL / GROOVY / ...
caseResult.setCaseFile(...)
caseResult.setHasAnswer(...)
caseResult.setCaseName(...)
caseResult.setResultDir(...)
caseResult.setCaseDir(...)
caseResult.setAnswerFile(...)     // ★ 변형 매칭 결과
caseResult.setPrintQueryPlan(TestUtil.isPrintQueryPlan(caseFile))
```

#### Answer 변형 매칭 — 발견된 알고리즘

```java
String answerFile = TestUtil.getAnswerFile(caseFile);
boolean hasAnswer = new File(answerFile).exists();

if (test.getRun_mode() != null && test.getRun_mode().length() > 0) {
    String runModeExtensionAnswer = answerFile + "_" + test.getRun_mode();
    String runModeExtSecondaryAnswer = answerFile + "_" + test.getRunModeSecondary();
    
    boolean hasRunModeExtAnswer = new File(runModeExtensionAnswer).exists();
    boolean hasRunModeSecondaryAnswer = new File(runModeExtSecondaryAnswer).exists();
    
    if (hasRunModeExtAnswer) {
        answerFile = runModeExtensionAnswer;       // 1순위: <answer>_<runMode>
    } else if (hasRunModeSecondaryAnswer) {
        answerFile = runModeExtSecondaryAnswer;    // 2순위: <answer>_<runModeSecondary>
    }
    // else: fallback to <answer>
}
```

→ **Answer 매트릭스 매칭의 핵심 알고리즘** (sql/test-corpus.md / sql/io-contract.md 의 미해결 항목).

`run_mode` / `runModeSecondary` 가 charset / build / cci 등을 어떻게 표현하는지가 PropertiesUtil + TestUtil 에 있을 것 — 후속 분석.

추정 매핑:
- `.answer_cci` ← `runMode = "cci"`
- `.answer_win` ← `runMode = "win"`
- `.answer_D_utf8_C_utf8_bin` ← `runMode = "D_utf8_C_utf8_bin"` (composite)

→ 새 시스템에서 *조건부 정답 매트릭스* (sql/test-corpus.md §11-3 권고) 를 명시적 구조로 표현.

### 2-3. shouldRun 결정

```java
boolean shouldRun = true;
if (testType == CaseResult.TYPE_SQL || testType == CaseResult.TYPE_GROOVY) {
    hasAnswer = new File(answerFile).exists();
    if (!hasAnswer) {
        if (runMode == MODE_RESULT || runMode == MODE_NO_RESULT) {
            shouldRun = false;   // answer 없으면 회귀 검증 모드에서는 skip
        }
        // MODE_MAKE_ANSWER 면 answer 없어도 실행 (=> answer 생성)
    }
}
```

→ MODE_MAKE_ANSWER 가 *answer 가 없는 케이스도 실행* 하는 special mode. answer 자동 생성용.

---

## 3. execute — 메인 루프

`execute(Test test)` (line 430) 의 핵심:

```
for (i = 0; i < totalCaseCount; i++) {
    // ProcessMonitor 상태 체크 (Stoping 이면 break)
    
    String caseFile = caseFileList.get(i);
    printMessage("Testing <caseFile> (i/total ratio%)")
    
    CaseResult caseResult = test.getCaseResultFromMap(caseFile);
    if (caseResult == null || !caseResult.isShouldRun()) {
        processMonitor.setCompleteFile(+1);
        processMonitor.setFailedFile(+1);
        continue;     // skip
    }
    
    executeSqlFile(test, caseResult);     // ★ 케이스 1건 실행 (§4)
    processMonitor.setCompleteFile(+1);
    
    if (saveEveryone) {
        saveTempResults(caseFile)
        if (FUNCTION) {
            if (RESULT or NO_RESULT) saveResults(caseFile)
            else if (MAKE_ANSWER) saveAnswers(caseFile)
        }
        caseResult.setResult("");           // 메모리 free
    }
    
    boolean isSucc = caseResult.isSuccessFul();
    if (!isSucc) {
        // core 파일 탐색
        coreFileList = CommonFileUtile.getCoreFiles(CubridUtil.getCubridPath(), test.getAllCoreList());
        if (coreFileList.size() > 0) {
            test.putCoreCaseIntoMap(caseFile, coreFileList);
            caseResult.setHasCore(true);
        }
    }
    printMessage(isSucc ? " [OK]" : " [NOK]");
    
    // Error interrupt 검사
    if (ErrorInterruptUtil.isCaseRunError(this, caseFile)) {
        onMessage("[ERROR]: Run case interrupt error!");
        break;       // 전체 중단
    }
}
```

**미묘:**
- `saveEveryone` (생성자 인자) — 모든 케이스의 결과를 *케이스마다 저장* vs *batch 저장*. ConsoleAgent.runTest 가 `saveEveryone=true` 로 설정 (cli-tree.md / sql/design.md §4)
- `core 파일 탐색이 케이스 단위` — 실패 시 *바로* core 검색 (run.sh 의 do_summary 와 다른 channel)
- `ErrorInterrupt` — *전체 실행 중단* trigger (특정 에러 패턴 발생 시 후속 케이스 skip). CI 환경에서 cascade fail 방지.

---

## 4. executeSqlFile — 케이스 1건 실행

`executeSqlFile(Test test, CaseResult caseResult)` (line 977) 의 핵심:

```
sqlList = SQLParser.parseSqlFile(caseFile, codeset, isNeedDebugHint)

for (run = 0; run < test.getSqlRunTime(); run++) {  // 다중 실행 (default 1)
    test.setDbId(test.getDbId(caseFile))
    String dbId = test.getDbId();
    String connId = test.getConnId();             // default connection
    CubridConnection cubridConnection = dao.getCubridConnection(dbId, connId, type)
    test.getConnIDList().put(connId, cubridConnection)
    
    if (test.isNeedCheckServerStatus()) checkServerStatus(cubridConnection)
    resetConnection(cubridConnection, test)        // charset 등 리셋
    
    long startTime = System.currentTimeMillis()
    for (k = 0; k < sqlList.size(); k++) {
        Sql sql = sqlList.get(k);
        
        // ★ 다중 connection 지원: 각 Sql 이 자체 connId 보유
        String thisConnId = sql.getConnId();
        if (!thisConnId.equals("") && !thisConnId.equals(connId)) {
            cubridConnection = dao.getCubridConnection(dbId, thisConnId, type);
            if (!test.getConnIDList().containsKey(thisConnId)) {
                test.getConnIDList().put(thisConnId, cubridConnection);
                resetConnection(cubridConnection, test);
            }
        }
        
        String script = sql.getScript();
        cubridConnection.isAvlible();
        Connection conn = cubridConnection.getConn();
        ConnList.add(conn);
        
        // <SQL 실행 + 결과 캡처 — 본 분석에서 line 1056+ 정밀 후속>
        ...
    }
}
```

> **후속 (2026-09-11).** 실행 · 렌더링 · 비교를 소스로 확인했다 — `evidence/sql-baseline.md` §7.
> 요지: 한 연결을 run 내내 재사용(`CubridConnManager.java:132`), 케이스마다 reset 과 autocommit
> (`ConsoleBO.java:812-848`), 결과는 열 이름 +4칸 · 값 +5칸 · 값은 `rs.getObject().toString()` 기반
> (`ConsoleDAO.java:873-934`, `957-1019`), 판정은 CR/LF 를 모두 지운 뒤 `String.equals`
> (`ConsoleBO.java:633-654`). 이 사실들이 ADR-016 의 근거다.

### 4-1. 다중 connection 지원 — sql 이 *isolation 같은 동시성 시나리오* 도 표현 가능!

`Sql.getConnId()` 가 빈 문자열 외 값이면 *다른 connection 으로 전환*. 즉:
- 하나의 .sql 파일 안에 *여러 connection* 의 statement 가 섞여 있을 수 있음
- 각 statement 가 어느 connection 으로 실행될지 SQL 안의 *주석 지시어* 로 표현 (추정 — `--@<connId>` 같은 패턴)

**이는 isolation 모듈의 .ctl DSL 과 부분적으로 겹치는 기능** — 단, isolation 은 *동시 실행* 이고 sql 다중 connection 은 *순차 실행*. sql 도 *간단한 다중 client 시나리오* 검증 가능.

→ case-formats.md / sql/test-corpus.md 의 pragma 시스템 보강:
- `--+ holdcas on;` 외에도 `--@<connId>` 같은 connection 지시어가 SQL 파서가 인식하는 메타로 추정

후속 — `SQLParser.parseSqlFile` 와 `Sql.connId` 추출 로직 정밀 분석.

### 4-2. `--@queryplan` 발견

```java
// isQueryPlan from two ways, one is "--@queryplan" in sql file,
//                          another is "XXX.queryPlan" file whose name is same as sql file
```

→ *2 가지 query plan 검증 채널*:
1. SQL 안의 `--@queryplan` 주석 지시어 → 그 SQL 의 query plan 출력
2. `<case>.queryPlan` 파일 (case-formats.md §2 의 934 .queryPlan) → 별도 비교 대상

새 시스템에서도 *두 채널* 모두 보존.

### 4-3. `sqlRunTime` — 다중 실행

`test.getSqlRunTime()` 회 반복. **default 1** 로 추정. *성능/안정성 검증* 시 N 회 반복 활용 가능 (SiteRunTimes 와 연관).

---

## 5. 35 utility 클래스 분류

cqt.console.util 35 클래스를 *역할별* 그룹화:

### 5-1. DB 연결 관리 (5)
- **CubridConnection** — JDBC connection 래퍼 (isAvlible / getConn / reset)
- **CubridConnManager** — connection pool 관리
- **CubridDBCenter** — multi-DB 지원
- **CubridUtil** — utility (getCubridPath / version 추출 등)
- **DatabaseInfo** — DB 메타 모델

### 5-2. JDBC 헬퍼 (3)
- **MyDataSource** — DataSource impl
- **MyDriverManager** — DriverManager 래퍼
- **DatabaseXMLReader** — DB conf XML 파서

### 5-3. 명령 실행 (3)
- **CommandExecutor** — 외부 명령 실행
- **CommandUtil** — 명령 utility
- **ShellFileMaker** — 셸 스크립트 생성

### 5-4. 환경 관리 (4)
- **EnvGetter** — 환경변수 조회
- **EnvSetter** — 환경변수 설정
- **EnvironmentCheck** — 환경 사전 검사
- **SystemConst** — 시스템 상수

### 5-5. 시스템 모니터링 (3)
- **SystemHandle** — 시스템 핸들러 (main 보유 — standalone 사용)
- **SystemModel** — 시스템 메타
- **SystemUtil** — OS 식별

### 5-6. 파일 / 리포트 (5)
- **FileUtil** — 파일 I/O
- **CommonFileUtile** — 공통 파일 (typo: Utile → Util) — core file 탐색 등
- **RepositoryPathUtil** — repo 경로
- **LogUtil** — 로깅
- **StreamGobbler** — Process I/O drain

### 5-7. XML / Conf (4)
- **ConfigurationXMLReader** — config XML 파서
- **ConfigureUtil** — config 관리 (local.properties 읽기 포함)
- **XMLDocument** — XML DOM
- **XMLReader** — XML SAX
- **XmlUtil** — XML utility
- **XstreamHelper** — XStream serialization

### 5-8. Test 메타 / Result 처리 (4)
- **TestUtil** — *가장 큰 utility*. getCaseFiles / getCaseFilePostFix / filterExcludedCaseFile / getResultDir / getCatMap / makeSummary / saveResultSummary / 등
- **PropertiesUtil** — local.properties 처리
- **ErrorInterrupt** — 에러 인터럽트 모델
- **ErrorInterruptUtil** — 인터럽트 검사

### 5-9. stdout 인터셉트 / 문자열 (3)
- **StdOutJob** — System.out hijack (sql/implementation-notes.md §6)
- **StringUtil** — 문자열 utility

### 5-10. SystemHandle (옵션 main)
- 자체 main(String[]) 보유 — 정밀 후속

---

## 6. 데이터 모델 (cqt.console.bean)

10 bean 클래스 (sql/design.md §1 참조):

```
Test            ← 최상위 — testId / cases[] / runMode / dbId / connIDList / catMap / coreCaseMap / summary
CaseResult      ← 케이스 1건 — caseFile / type / hasAnswer / answerFile / shouldRun / hasCore / result / isSuccessFul
Summary         ← 디렉터리/카테고리 단위 요약 — totalCount / successCount / failCount / siteRunTimes / totalTime / type / catPath
SummaryInfo     ← Summary 의 메타
TestCaseSummary
Sql             ← SQL 1 statement — script / connId / queryPlan flag
SqlParam        ← SQL 인자
SystemModel     ← (util 와 별개)
DefTestDB
ProcessMonitor  ← 진행 상태 (Status_Starting/Started/Stoping/Stoped, completeFile/failedFile, allFile)
```

---

## 7. ConsoleDAO (cqt.console.dao)

`ConsoleDAO(test, configureUtil)` — 데이터 접근 객체 (DAO).

핵심 메서드 (line 152, 236, 1014):
- `addDb(dbId)` — DB 등록
- `getCubridConnection(dbId, connId, type)` — connection 획득
- `isDbOk()` — DB 가용성 검증 (checkDb 가 사용)
- `release()` — finally 에서 호출 (모든 connection 정리)

→ **DAO 패턴**: BO 가 DAO 만 알고 직접 JDBC 호출 안 함. 새 시스템에서도 같은 분리 권고.

---

## 8. 데이터 흐름 (다이어그램)

```
                     run.sh sql/bin/run.sh
                            │
                            ▼ shell-out
                     java cqt.ConsoleAgent runCQT ...
                            │
                            ▼
                     ConsoleAgent.runTest(...)  
                            │
                            ▼
                     ConsoleBO.runTest(test)
              ┌────────┬────┴────┬─────────┬────────────┐
              │        │         │         │            │
              ▼        ▼         ▼         ▼            ▼
            init()   DAO     buildTest  checkDb     execute(test)
                      │      ┌───┴───┐               │
                      │      │       │               │
                  ConsoleDAO getCaseFiles  build      │
                      │      (URL parse + (CaseResult │
                      │       file walk + answer matrix)│
                      │       filter)                 │
                      │                               │
                      │   ┌───────────────────────────┘
                      │   ▼ (per case)
                      │ executeSqlFile(test, caseResult)
                      │   ├─► SQLParser.parseSqlFile → List<Sql>
                      │   ├─► dao.getCubridConnection(dbId, connId)
                      │   ├─► (옵션) checkServerStatus / resetConnection
                      │   ├─► for each Sql:
                      │   │     - 다중 connection 지원 (Sql.connId 변경 시 switch)
                      │   │     - --@queryplan 처리
                      │   │     - JDBC 실행 + 결과 캡처
                      │   ├─► CommonFileUtile.getCoreFiles (실패 시)
                      │   └─► caseResult.setSuccessFul / setHasCore
                      ▼
                   saveTempResults / saveResults / saveAnswers
                      │
                      ▼
                   makeSummary + makeSummaryInfo + saveResultSummary
                            │
                            ▼ stdout
                   "Result Root Dir: <path>"
                   "total: N", "success: N", "fail: N"
                            │
                            ▼ <resultDir>/main.info
                   (run.sh do_summary_and_clean 가 grep)
```

---

## 9. 새 시스템 ADR 후보

### ADR 후보 1: cqt 추상화의 *재사용 vs 폐기*

cqt 의 BO/DAO/util 분리는 *2010년대 Java enterprise* 스타일. 모던 언어로 가면 *전체 재설계* 가 자연스러움. 단:
- *외부 표면* (URL specifier / answer matrix / `--@queryplan` 등) 은 동결 의무
- *내부 추상화* (ConsoleBO / ConsoleDAO / 35 utility) 는 자유

→ 새 시스템에서 *cqt 외부 인터페이스* 는 보존, *내부* 는 그 언어의 idiom 으로 재작성.

### ADR 후보 2: SQL 안의 pragma 시스템 정형화

발견된 pragma:
- `--+ holdcas on;`
- `--@queryplan` (queryplan 출력)
- `--@<connId>` (multi-connection switch, 추정)

새 시스템에서 *명시적 grammar* + 명시적 list:
```
SQL pragmas (frozen):
  --+ holdcas on|off
  --@queryplan
  --@<connId>            (e.g. --@C1, --@C2)
  --@autocommit on|off   (주석 처리된 코드에서 발견)
```

`SQLParser.parseSqlFile` 의 정밀 grammar 후속 분석 후 *모든 활성 pragma list* 확정.

### ADR 후보 3: 다중 connection per .sql 의 위치

sql 의 multi-connection 은 isolation 의 .ctl 과 *기능 중첩*. 단:
- sql 의 multi-connection 은 *순차* 실행 (한 connection 씩 차례로)
- isolation 의 .ctl 은 *동시* 실행 (N processes)

→ 두 모델의 *의도된 차이* 임을 새 시스템에서 명확히 표현. *SQL 안의 가벼운 multi-connection* + *.ctl 의 무거운 multi-process* 모두 first-class.

### ADR 후보 4: answer matrix 알고리즘 정형화

발견된 알고리즘 (§2-2):
1. base = `<case>.answer`
2. runMode 있으면 `<case>.answer_<runMode>` 우선
3. fallback `<case>.answer_<runModeSecondary>`
4. 그래도 없으면 fallback `<case>.answer`

새 시스템에서 *조건부 정답* 을 명시적 *predicate + answer file path* 로 표현 (sql/test-corpus.md §11-3 권고와 결합).

### ADR 후보 5: ErrorInterrupt 정책

`ErrorInterruptUtil.isCaseRunError` — 특정 에러 발생 시 *전체 실행 중단*. 어떤 패턴이 trigger 인지 후속 분석.

CI 환경에서 *cascade fail 방지* 의도. 새 시스템에서 같은 정책 보존 (또는 명시적 conf 로 노출).

---

## 10. 미해결 후속 분석

- `SQLParser.parseSqlFile` 의 정밀 동작 — pragma 인식 / Sql.connId 추출 / debug hint 처리
- `TestUtil.getResultDir / getCatMap / makeSummary / saveResultSummary` 정밀 흐름
- `PropertiesUtil.initConfig(charsetfile, test)` 가 charset_xml 에서 무엇을 읽고 test 에 어떻게 매핑하는지
- `runMode` / `runModeSecondary` 의 *값 출처* (config / 시스템 프로퍼티 / charset XML)
- `ConfigurationXMLReader` / `DatabaseXMLReader` 가 읽는 XML 스키마
- `CubridConnection.isAvlible` (typo: Available) 의 정확한 검증 로직
- `ConsoleDAO.getCubridConnection` 의 connection pool 정책
- `ErrorInterruptUtil.isCaseRunError` 가 trigger 하는 에러 패턴
- `SystemHandle` 의 main() — standalone 도구로서의 사용처
- `CommandExecutor` / `StreamGobbler` 가 호출되는 곳 (외부 cubrid 명령 실행?)

이들은 ADR-001/002/004 결정 후 Phase 1 (concept) 진입 시 우선순위 따라 처리.

---

## 11. 결론 — 새 시스템 cqt 후계자 설계의 입력

```
보존해야 할 외부 표면:
- URL-style scenario specifier (?db=X&filter=Y)
- runMode-driven answer 변형 매칭 (algorithm §2-2)
- SQL pragma (--+ holdcas, --@queryplan, --@<connId>)
- multi-connection per .sql
- stdout 마커 (Result Root Dir, total/success/fail)
- main.info 포맷
- core file 자동 탐색

내부 재설계 가능:
- BO/DAO/Util 35+ 클래스 → 모던 언어 idiom (예: Go의 패키지 설계)
- StdOutJob System.out hijack → 명시적 logger
- ProcessMonitor → 모던 telemetry
- XML conf (test_default.xml) → modern conf format

다중 모드 제외 후보 (1차 strangler-fig):
- multi-DB (ConsoleDAO.addDb 가 다중 DB 지원, 그러나 사용처 미파악)
- SystemHandle main() 도구
- TestCaseSummary / SummaryInfo 의 일부 — XML serialization 위주
```
