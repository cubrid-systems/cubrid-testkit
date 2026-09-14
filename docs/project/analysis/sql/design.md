# sql — Design (Main 시퀀스 분석)

**Source:** `cubrid-testtools/CTP/sql/`

**Phase 0 M0 #4 status:** 완료 (sequence-level seed)

---

## 1. 모듈 구성 — 셸 지배 + 자체 Java 서브시스템

```
sql/
├── bin/                          ★ 실행기의 본체 (셸)
│   ├── run.sh           917 라인 — 본 entry (CTP.executeSQL이 호출)
│   ├── run_memory.sh             — memory leak 변형 (`enable_memory_leak=true` 시)
│   └── interactive.sh            — sourcing 대상 (대화형 모드)
│
├── src/                          ★ Java cqt 서브시스템 (71 파일)
│   └── com.navercorp.cubridqa.cqt
│       ├── common (9)            — 자체 SSH/SFTP 클라이언트 (shell.common과 별개)
│       │   SSHConnect, SFTP, SFTPDownload/Upload, RunRemoteScript,
│       │   ShellInput, SQLParser, LineScanner, CommonUtils
│       ├── console (2)           — ★ Java entry layer
│       │   ConsoleAgent          (run.sh 가 java로 호출)
│       │   Executor              (PRINT_STDOUT / PRINT_UI 상수, 출력 어댑터)
│       ├── console/bean (10)     — 데이터 모델
│       │   Test, CaseResult, Summary, SummaryInfo, Sql, SqlParam,
│       │   SystemModel, ProcessMonitor, DefTestDB, TestCaseSummary
│       ├── console/bo (1)        — ConsoleBO (runTest / checkAnswers 비즈니스 로직)
│       ├── console/dao (1)       — ConsoleDAO
│       ├── console/util (35)     — 풍부한 유틸 (DB connect, env, XML reader,
│       │                            CommandExecutor, ProcessMonitor, ErrorInterrupt 등)
│       ├── model (2)
│       ├── webconsole (7)        — Jetty 기반 웹콘솔
│       │   Starter, WebServer, ...   (CTP.executeWebConsole 가 호출)
│       └── webconsole/compare (3) — Compare (diff 시각화)
│
├── lib/                          jetty/jasper/avalon-framework/javax-el/jaxen/xstream/
│                                  cubridqa-cqt.jar
├── webconsole/                   웹 UI 정적 자산
├── configuration/                System.xml, Function_Db/, test_config/
├── function/                     stored procedure 함수 정의
├── memory/                       메모리 테스트 리소스
└── sample/                       cases/ + answers/ 템플릿
```

**중요한 관찰**: cqt 의 `common` 서브패키지는 `shell.common.*` 와 별개의 SSH 클라이언트 / 셸 명령 빌더를 가진다. → **sql 모듈은 shell.common.* 에 의존하지 않는다** (cli-tree에서 본 4개의 common 의존만: `CommonUtils`, `coreanalyzer.AnalyzerMain`, `IniData`, `ShareMemory`). 모듈 간 결합 측면에서 가장 자율적인 deep 모듈.

---

## 2. CTP → run.sh → ConsoleAgent 라우팅

```
ctp.sh sql -c sql.conf
   ▼
CTP.executeSQL(config, "sql", interactive=false, useCCI=false)
   │ 셸-아웃 (process exec):
   │  enable_memory_leak ? run_memory.sh : run.sh
   │  $CTP_HOME/sql/bin/run.sh -s sql -f <conf-path>
   │  (useCCI=true 면 export sql_interface_type=cci 선행)
   ▼
sql/bin/run.sh -s <suite> -f <conf>
   │
   ├── do_init           CTP_HOME/scenario_repo_root/jdbc_config_file_ext/log_dir 등 디폴트
   ├── do_clean          이전 결과/임시 정리
   ├── do_configure      cubrid 설정, locale 빌드, java_stored_procedure 등
   ├── do_create_db      $db_name (`basic` 디폴트), createdb / loaddb
   ├── do_test           ★ 핵심 분기 (§3)
   └── do_summary_and_clean
        ├── result/<...>/main.info 파싱 → fail/success/total/totalTime
        ├── core file 탐색 (find $CUBRID $CTP_HOME -name 'core*' + file 검사)
        └── core 발견 시 test_error=Y, 아니면 do_clean
```

run.sh의 시작/끝(900~917)이 평탄한 6단계 호출 — **strict pipeline, 분기 없음**. 각 단계 내부에서 conf 파라미터에 따른 분기가 발생.

---

## 3. `do_test` 의 3-way 분기

```
if [ "$sql_interactive" == "yes" ]; then
    # 모드 1: Interactive
    export CLASSPATH=...; export log_file_in_interactive=...
    cd $scenario_repo_root
    source $CTP_HOME/sql/bin/interactive.sh
    help
    bash --posix       # 사용자가 직접 셸에서 SQL 케이스를 손으로 실행

elif [ "$interface_type" == "cci" ]; then
    # 모드 2: CCI (C native)
    port=`ini -s "%BROKER1" $CUBRID/conf/cubrid_broker.conf BROKER_PORT`
    resultFolder="schedule_cdriver_${os_type}_${alias}_<ts>_${cubrid_ver}_${cubrid_bits}"
    $CTP_HOME/sql_by_cci/ccqt $port $db_name ${alias} ${resultFolder} \
        ${scenario_repo_root} $CTP_HOME ${cci_urlproperty} 2>&1 | tee -a $log_filename
    # → sql_by_cci 의 `ccqt` C 실행파일이 진짜 worker (CTP/sql_by_cci/ 모듈)

else
    # 모드 3: JDBC (default)
    java -Xms1024m -XX:+UseParallelGC \
         -classpath "${CLASSPATH}${separator}${CPCLASSES}" \
         com.navercorp.cubridqa.cqt.console.ConsoleAgent runCQT \
            ${scenario_category} ${scenario_alias} ${cubrid_bits} \
            $jdbc_config_file_ext \
            ${scenario_repo_root}?db=${db_name}_qa[&filter=$testcase_exclude_file] \
         2>&1 | tee -a $log_filename
fi
```

**모드 1 (Interactive)**: `bash --posix`를 띄워 사용자가 손으로 케이스 실행. CTP의 `#SCRIPTCONT` 메커니즘과 결합 — Java 측 ConsoleAgent 가 아닌, CTP.java 가 출력에서 `#SCRIPTCONT` 마커를 추출해 후속 셸 스크립트로 실행함 (cli-tree.md 참조).

**모드 2 (CCI)**: native C 실행 파일(`ccqt`)이 케이스를 수행. 이 부분은 sql 모듈이 아닌 `sql_by_cci/` 모듈의 영역으로, 이번 분석 범위 외(인벤토리 후속).

**모드 3 (JDBC, default)**: Java `ConsoleAgent.main(args)` 가 진짜 worker.

---

## 4. ConsoleAgent.main(args) 시퀀스

```
ConsoleAgent.main(["runCQT", type, typeAlias, version, charset_xml, file1, file2, ...])
   │
   ├── command = args[0]; if "runCQT":
   │     type = args[1]                  e.g. "sql", "medium"
   │     typeAlias = args[2]             e.g. "sql_ext"
   │     version = args[3]               "32" / "64"
   │     charset_xml = args[4]           "test_default.xml" (default)
   │     files[] = args[5..]             ["<repo>?db=basic_qa[&filter=...]"]
   │     comefrom = (version=="32") ? COME_FROM_CQT_32(=32) : COME_FROM_CQT_64(=64)
   │
   ▼
ConsoleAgent.runTest(files, type, typeAlias, printResult=false, comefrom, charset_xml)
   │
   ├── stdOutJob = new StdOutJob(System.out, START)  ← stdout intercept (로그 처리용)
   ├── stdOutJob.start()
   ├── bo = new ConsoleBO(useMonitor=false, saveEveryone=true)
   ├── Innerbo = bo
   │
   ├── scenarioTypeName = "schedule"
   ├── testCategory = "<type>_<bits>bit"   (e.g. "sql_64bit")
   ├── testId = TestUtil.getTestId(scenarioTypeName, testCategory)
   ├── charsetfile = TestUtil.getCharsetFile(charset_xml)
   ├── resDir = TestUtil.getResultDir(testId)
   │
   ├── test = new Test(testId)
   ├── test.setRunMode(MODE_RESULT)
   ├── test.setCodeset(DEFAULT_CODESET)
   ├── test.setTestType(type)
   ├── test.setResult_dir(resDir)
   ├── test.setTestTypeAlias(typeAlias)
   ├── test.setTestBit(bit)               // "32bit" / "64bit"
   ├── test.setCharset_file(charset_xml)
   │
   ├── PropertiesUtil.initConfig(charsetfile, test)
   │      ← local.properties 읽기 (isdebug, qaview)
   ├── test.setDebug(...)                 // local.properties[isdebug]
   ├── test.setQaview(...)                // local.properties[qaview]
   │
   ├── if comefrom==32 → version="32bits"
   │   elif comefrom==64 && OS=windows → version="64bits"
   │   else → version="Main"
   ├── test.setCases(files.clone())
   │
   ├── System.out.println("Result Root Dir:" + test.getResult_dir())   ⚠️ run.sh 가 grep
   ├── bo.setPrintType(PRINT_UI)         (printResult=false 이므로)
   │
   ├── summary = bo.runTest(test)        ★ 진짜 실행
   │
   ├── if (test.isNeedSummaryXML()) TestUtil.saveSummaryMainInfo(test, summary)
   │   ← <resultDir>/main.info 작성 — run.sh 의 do_summary_and_clean 가 grep
   │
   ├── for each caseFile:
   │     caseResult = test.getCaseResultFromMap(caseFile)
   │     caseMap.put(file, "ok"|"nok"|"")
   │
   ├── coreFileCaseMap = test.getCoreCaseMap()
   │   for each (case → core files):
   │     bo.saveCoreCallStackFile(case, files)
   │
   └── stdOutJob.stop()
```

`bo.runTest(test)` 의 내부는 `console.bo.ConsoleBO` + `console.util.*` 35개 클래스의 협업. Phase 0 깊이 분석 단계에서는 진입점 시퀀스만 식별, 내부 흐름은 design.md 갱신 시 보강.

### `Test` (cqt.console.bean.Test) 의 키 setter
- `runMode`: MODE_NORESULT(0), MODE_ANSWER(1), MODE_RESULT(2), MODE_RESULT_TO_SCENARIO(3)
- `testType`, `testTypeAlias`: e.g. "sql" / "sql_ext"
- `testBit`: "32bit"|"64bit"
- `version`: "Main"|"32bits"|"64bits"
- `result_dir`, `charset_file`, `codeset`
- `cases[]`: URL-style scenario specifier — `<repo_root>/<scenario_path>?db=<db>_qa[&filter=<file>]`

cases는 *URL 쿼리 스트링 형식*으로 케이스 위치 + DB + 필터를 하나의 문자열로 인코딩. ConsoleBO 가 이를 파싱.

---

## 5. 출력 채널 (run.sh가 grep으로 의존)

ConsoleAgent → ConsoleBO 가 **stdout에 라인 마커**로 메타정보를 흘려보내고, run.sh 가 이를 grep:

| 출력 라인 | 작성자 | run.sh 사용처 |
|-----------|--------|----------------|
| `Result Root Dir: <path>` | ConsoleAgent.runTest:156 | `do_summary_and_clean` 첫 grep |
| `total:`, `success:`, `fail:`, `totalTime:` | ConsoleAgent (printResult=true 시) | 미사용 (run.sh는 main.info를 직접 읽음) |
| `<resultDir>/main.info` 파일 | TestUtil.saveSummaryMainInfo | `success:`, `fail:`, `total:`, `totalTime:` 정규식 |
| `TOTAL_COUNT: <N>` / `TOTAL_ELAPSE_TIME: <ms>` (cqt.log) | (ConsoleBO 추정) | CCI 모드에서 `generate_summary_info` 가 grep |
| `CORE_FILE: <path>` | run.sh do_summary_and_clean 자체 출력 | summary_info에 `test_error=Y` |

⚠️ **In-band signaling 의존이 강하다**: stdout 텍스트 grep으로 Java↔셸을 잇는 패턴. 새 시스템에서는 명시적 IPC(JSON 라인, named pipe, 파일 락 등)로 승격할 가치가 있으나, **외부 표면 동결 의무 때문에 stdout 마커 자체는 보존 필요** (cubridqa-feedback 도구나 CI 파이프라인이 동일 grep을 쓸 가능성).

---

## 6. cqt 의 자체 SSH/SFTP — shell.common 과의 분리

cqt.common 패키지는 자체적인 원격 통신 클래스를 가진다:
- `cqt.common.SSHConnect` — jsch 래퍼 (shell.common.SSHConnect와 별개의 구현)
- `cqt.common.SFTP` / `SFTPDownload` / `SFTPUpload`
- `cqt.common.RunRemoteScript` — 원격 셸 스크립트 실행
- `cqt.common.ShellInput` — 셸 명령 빌더

**즉 sql 모듈은 자체 원격 통신 스택을 가진다** — shell 모듈의 SSHConnect를 import 하지 않는다 (cli-tree.md M0 #1, deps-of-common.md M0 #3 의 발견과 일치).

이는 strangler-fig 관점에서 다음 의미:
- **sql 모듈은 단독으로 대체 가능**한 후보 — shell.common.* 의존을 갖지 않으므로 isolation/ha_repl/cdc_repl 처럼 묶음으로 대체할 필요 없음.
- 단, sql 모듈은 *self-contained 하지만 큰 모듈* (Java 71 파일 + 셸 917 라인 + 관련 자산). 분량은 가장 크다.
- **CCI 모드의 `ccqt` (C 바이너리)** 는 `sql_by_cci/` 모듈의 산출 — sql 단독 대체 시 CCI 모드 처리 정책 ADR 필요.

---

## 7. medium 의 위치 — sql 의 변형

`medium`은 별도 디렉터리 없이 **sql/bin/run.sh -s medium -f medium.conf** 로 실행되는 *suite alias*. CTP.executeSQL 에서 분기되는 일이 없으며, 동일한 ConsoleAgent runCQT 흐름을 탄다 (type="medium").

차이는 conf만:
- `medium.conf`, `medium_dev.conf` 의 키들은 sql.conf 와 거의 같음 (medium.conf 는 sql 의 sub-set + 일부 변형)
- conf-matrix.md에서 확인: `medium 0 exclusive`, `medium_dev 1 exclusive (create_table_reuseoid)`

→ **medium 은 별도 모듈이 아니라 sql 의 *suite 인스턴스*** 라는 가설이 sql 분석으로 확정됨. cli-tree, conf-matrix, design 3중으로 일관 확인.

새 시스템 설계 시:
- medium 은 sql의 sub-suite로 표현 (별도 모듈 아님)
- `analysis/medium/` stub 들의 위치는 향후 `analysis/sql/medium/` 로 재배치 가능 (M0 진행 메모)

---

## 8. 외부 의존 표면 (동결 대상)

### 8-1. 환경 변수 (run.sh가 `do_init`에서 의존)
- `$CUBRID` — CUBRID 설치 dir (없으면 즉시 exit 1)
- `$JAVA_HOME` — `${JAVA_HOME}/bin/java`, `${JAVA_HOME}/bin/javac` 호출
- `$CTP_HOME` — `cd $(dirname $(readlink -f $0))/../..` 로 자동 산출
- `$HOME` — `scenario_repo_root=$HOME/dailyqa` 디폴트

### 8-2. 외부 자산
- `$CUBRID/jdbc/cubrid_jdbc.jar` (run.sh:575) — 자바 stored procedure 빌드 시 javac 의 -cp
- `$CUBRID/conf/cubrid_broker.conf` (run.sh:792) — CCI 모드에서 BROKER_PORT 추출 (`ini -s` 사용)
- `$CTP_HOME/bin/ini.sh` — 셸용 INI 파서 (alias로 등록)
- `$CTP_HOME/sql_by_cci/ccqt` — CCI 모드의 native worker

### 8-3. testcases 측 약속
- 케이스 디렉터리: `<scenario_repo_root>/<scenario_alias>/<...>/cases/`
- 각 케이스는 SQL 스크립트 + answer 파일
- 케이스 위치 specifier: `<root>?db=<dbname>_qa[&filter=<exclude_file>]`

### 8-4. config (sql.conf 25 키, sql_by_cci.conf 23 키)
- `scenario`, `scenario_alias`, `db_name`, `db_charset`, `cubrid_bits`, `jdbc_config_file`
- `enable_memory_leak`, `cubrid_createdb_opts`, `need_make_locale`, `java_stored_procedure`
- `lock_timeout`, `max_plan_cache_entries`, `unicode_input_normalization`, `update_statistics_on_catalog_classes_yn`
- `APPL_SERVER_MAX_SIZE`, `SetAPPL_SERVER_MAX_SIZE` (sql 전용)
- `sql_interface_type` (CTP.executeSQL이 export 하는 환경변수)
- `testcase_retry_num`, `testcase_timeout_in_secs`, `testcase_exclude_from_file`
- (전체는 conf-matrix.md)

### 8-5. cqt 입력 파일
- `local.properties` (cqt working dir) — `isdebug`, `qaview` 등
- `<charset_file>.xml` (default `test_default.xml`) — JDBC 연결/charset 설정
- `<resultDir>/main.info` 출력 (run.sh가 fail/success/totalTime 추출)

---

## 9. 데이터 흐름 (다이어그램)

```
              ctp.sh sql -c sql.conf
                       │
                       ▼  process exec (LocalInvoker)
              ┌───────────────────┐
              │ sql/bin/run.sh    │ (917 라인, linear 6단계 pipeline)
              │  do_init          │
              │  do_clean         │
              │  do_configure     │
              │  do_create_db     │
              │  do_test          ┼───────┐
              │  do_summary_and_  │       │
              │    clean          │       │
              └───────────────────┘       │
                                          │
                       ┌──────────────────┴────────────────────┐
                       │                                       │
                       ▼                                       ▼
              [interactive ?]                        [interface_type==cci ?]
                       │                                       │
              source interactive.sh                  ccqt (C binary in sql_by_cci/)
              bash --posix                             │
                                                       ▼
                                              [native C SQL test]
                       │
                       ▼ default (JDBC)
              ┌────────────────────────────────────┐
              │ java cqt.console.ConsoleAgent      │
              │   runCQT type alias bits cs files  │
              └─────────────┬──────────────────────┘
                            │
                            ▼
                   ┌────────────────────┐
                   │ ConsoleAgent       │
                   │   .runTest         │
                   │   ├ new ConsoleBO  │
                   │   ├ new Test       │
                   │   ├ initConfig     │ ←── local.properties
                   │   ├ bo.runTest(t)  │
                   │   │   └ (35 util)  │ ←── jdbc / cqt.common.SSHConnect
                   │   ├ saveSummaryXML │ ──► <resultDir>/main.info
                   │   └ saveCoreStack  │ ──► <resultDir>/<core>/...
                   └────────────────────┘
                            │
                            ▼ stdout (line markers)
                   "Result Root Dir: ..."
                   "total: N", "success: N"
                            │
                            ▼ run.sh greps
                   do_summary_and_clean
                            │
                            ▼
                   summary_info, core 검사, 종료 출력
```

---

## 10. 새 시스템 설계 시 권고 (sql 관점)

1. **sql 모듈은 가장 자율적**. shell.common.* 의존이 없고, cqt 자체 원격 통신 스택을 가짐. **단독 strangler-fig 1차 대체 후보로 강함**. 단, 자율성의 대가로 모듈 자체가 가장 큼 (Java 71 파일 + 셸 917 라인) — 1인 6~12개월 호라이즌에서 한 분기 통째로 소요될 가능성.

2. **셸 + Java 분리 모델은 유지 가치 있음**. run.sh 의 "DB 셋업/cleanup/locale 빌드" 영역과 ConsoleAgent 의 "케이스 실행/diff/summary" 영역은 책임이 다름. 새 시스템에서도 두 책임을 나누되, in-band signaling(`Result Root Dir:` 마커, `main.info` 파일)을 보다 명시적인 IPC로 승격하면 이득.

3. **CCI 모드는 sql_by_cci 모듈에 종속**. sql 단독 대체 시 CCI 모드를 동시에 다룰지, 일시적으로 기존 CTP 의 sql_by_cci 어댑터로 우회할지 ADR 필요.

4. **medium 은 sql 의 suite 인스턴스로 흡수**. 별도 디자인 산출물은 작아도 됨. ROADMAP의 `analysis/medium/` 5개 stub은 향후 `analysis/sql/medium/` 로 재배치하거나, 짧은 노트(case-formats 차이)로 축소 가능.

5. **stdout 라인 마커 호환은 동결 의무**. `Result Root Dir:`, `Fail:`, `Success:`, `Total:`, `CORE_FILE:` 라인 형식은 외부 도구가 grep할 수 있는 표면.

6. **`local.properties` 의존**은 새 시스템 설계 시 conf 단일화 대상. cqt 가 별도로 읽는 `local.properties` 와 메인 `sql.conf` 는 의도적이라기보다는 누적된 결과 — 새 시스템에서는 한 conf로 통합.

7. **35개의 console/util 클래스**는 deep dive 시 정리 후보가 다수일 가능성. 외부 표면(stdout/main.info)만 동결되면 내부 리팩토링은 자유.

8. **interactive 모드 (`bash --posix` + #SCRIPTCONT)** 는 매우 ad-hoc 한 메커니즘. 새 시스템에서는 first-class REPL 또는 별도 도구로 분리하는 것이 깨끗.

---

## ROADMAP 갱신

체크리스트의 `analysis/sql/design.md` 항목을 완료(`- [x]`)로 갱신.

**M0 #4 마지막 모듈:** medium — 별도 모듈이 아닌 sql의 suite 인스턴스임이 본 분석으로 확정. medium/design.md 는 짧게 "sql 의 변형, 차이만 기술" 형태로 채울 예정.
