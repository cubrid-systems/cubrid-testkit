# common — I/O Contract (라이브러리 API + 셸 자산 + 옵션 데몬)

**Source:** `cubrid-testtools/CTP/common/`

common 은 *호출되는 모듈* 이 아니므로 일반 모듈의 contract 와 다르다. 다음 4 가지 표면이 동결 대상:

1. **Java API** (다른 모듈이 import 하는 클래스/메서드)
2. **셸 자산** (script/ 의 헬퍼, ext/ 의 entry, tpl/ 의 템플릿)
3. **CTP CLI** (ctp 패키지 — 모든 ctp.sh 호출의 진입)
4. **옵션 데몬 contract** (grepo RMI / scheduler ActiveMQ)

---

## 1. Java API 표면

### 1-1. 다른 모듈이 *반드시* import 하는 클래스 (deps-of-common.md §4)

| 클래스 | 사용 모듈 | 메서드 호출 빈도 |
|--------|-----------|------------------|
| `common.CommonUtils` | shell, isolation, ha_repl, cdc_repl, sql, sched | 매우 높음 — 거의 모든 utility 호출의 origin |
| `common.IniData` | sql (ConsoleAgent), CTP.java | conf 파싱 |
| `common.ConfigParameterConstants` | shell, isolation, ha_repl, cdc_repl | conf 키 type-safe 참조 |
| `common.Constants` | shell, ha_repl, cdc_repl | LINE_SEPARATOR / ENV_CTP_HOME_KEY 등 |
| `common.Log` | shell, isolation, ha_repl, cdc_repl, sql, sched | 파일 로깅 |
| `common.LocalInvoker` | shell (TestFactory.backupTestResults), ha_repl, cdc_repl, GeneralLocalTest | 로컬 셸 실행 |
| `common.coreanalyzer.AnalyzerMain` | sql | core dump 분석 |
| `common.coreanalyzer.CommonUtil` | isolation | 분석 utility |
| `common.ShareMemory` | sql (cqt) | Java NIO IPC |
| `common.MailSender` | shell | 이메일 통보 |
| `common.MakeFile` | shell | (사용처 정밀 후속) |

### 1-2. CommonUtils 의 *주요 method 시그니처* (40+)

```java
// Path & OS
public static boolean isWindowsPlatform()
public static boolean isCygwinPlatform()
public static String getLinuxStylePath(String path)
public static String getLinuxStylePath(String path, boolean useCygPath)
public static String getWindowsStylePath(String path)
public static String getFixedPath(String path)

// String / data
public static boolean isEmpty(String s)
public static String replace(String src, String from, String to)
public static String rightTrim(String str)
public static boolean convertBoolean(String str)
public static boolean convertBoolean(String value, boolean defaultValue)
public static int getRadomNum(int max)            // (typo: Random)

// File I/O
public static String concatFile(String p1, String p2)
public static String getFileContent(String filename)
public static String getFileContent(String filename, boolean trimBlankSpace)
public static ArrayList<String> getLineList(String filename)
public static Reader readFile(String file)

// Properties / conf
public static Properties getProperties(String filename)
public static Properties getPropertiesWithPriority(String filename)
public static void writeProperties(String filename, Properties props)
public static Properties getConfig(String configFile)
public static Properties getConfig(InputStream is)
public static Properties parsePropertiesByPrefix(Properties cfg, String prefix)
public static Properties parsePropertiesByPrefix(Properties cfg, String prefix, String defaultPrefix)
public static String parsePropertiesStringByPrefix(Properties cfg, String prefix, String defaultPrefix)
public static String parseInstanceParametersByRole(Properties cfg, String instancePrefix, String role)
public static String getSystemProperty(String key, String defaultValue, Properties props)

// Time
public static String dateToString(Date date, String fm)
public static String getCurrentTimeStamp(String format)
public static Timestamp getCurrentTimestamp()
public static void sleep(int sec)

// Build / URL
public static boolean isAvailableURL(String urlStr)
public static String getSimplifiedBuildId(String cubridPackageUrl)
public static String getBuildId(String url_or_text)
public static String getBuildBits(String url_or_text)
public static boolean isNewBuildNumberSystem(String testBuild)

// Misc
public static String getEnvInFile(String var)
public static String getExportsOfMEKYParams()
public static int getShellType(boolean supportPureWindows)
public static String getExactFilename(String fullFilename)
public static ArrayList<String[]> extractTableToBeVerified(String input, String flag)

// Cleanup
public static void cleanFilesByDirectory(String dir)
```

→ 새 시스템에서 *동등 함수 일대일 매핑*. 단, 메서드 명명 (camelCase typo `getRadomNum`) 정정은 ADR — 호환 유지 vs cleanup.

### 1-3. IniData 의 시그니처 (15)

```java
public IniData(String fileName) throws InvalidFileFormatException, IOException
public IniData(File file) throws InvalidFileFormatException, IOException
public String get(String sectionName, String key)
public String getAndTrans(String sectionName, String key)
public Section getSection(String sectionName)
public void put(String sectionName, String key, String value, boolean store)
public void remove(String sectionName, boolean store)
public void remove(String sectionName, String key, boolean store)
public String getFilename()
public void saveAs(String newFilename, String oldPath, String newPath)
public String toString()
public static String translateValue(String value)

// Inner Section class
public Section.getName()
public Section.put(key, value)
public Section.get(key)
public Section.getAndTrans(key)
public Section.remove(key)
public Section.getData()  // returns HashMap<String, String>
```

→ 인터페이스 좁고 명확. 새 시스템에서 동등 추상화 보존.

### 1-4. ConfigParameterConstants 의 82 키 — 동결

```java
public static final String SCENARIO = "scenario";
public static final String TESTCASE_RETRY_NUM = "testcase_retry_num";
public static final String TESTCASE_TIMEOUT_IN_SECS = "testcase_timeout_in_secs";
public static final String TEST_INSTANCE_HOST_SUFFIX = ".ssh.host";
public static final String ROLE_ENGINE = "cubrid";
public static final String ROLE_BROKER1 = "broker1";
public static final String ROLE_BROKER2 = "broker2";
public static final String ROLE_HA = "ha";
public static final String ROLE_CM = "cm";
public static final String ROLE_BROKERCOMMON = "brokercommon";
// ... 72 more
```

새 시스템에서 *동일 한 type-safe 키 카탈로그* 보존. 단순 string literal 으로 회귀하면 모듈 간 일관성 깨짐.

---

## 2. 셸 자산 표면

### 2-1. `script/` — CI 운영 헬퍼 (셸 명령으로 직접 호출)

```
script/analyze_failure.sh             — 실패 분석 + tpl/issue_*.tpl 머지
script/analyzer.sh                    — 일반 분석
script/commit_config_file             — config 를 DB 적재 (CommitConfigFileIntoDB 의 셸 wrapper?)
script/convert_to_git_url.sh          — repo URL 변환
script/crash_template_ha.sh           — HA crash 처리 템플릿
script/crash_template_single.sh       — single crash
script/file_core_issue.sh             — core 발견 시 issue 등록
script/generate_build_test.sh         — 빌드 테스트 생성
script/issue.sh                       — issue 보고 wrapper
script/prepare_memory_env.sh          — memory test 환경 준비 (run_memory.sh 보조?)
script/process_safe.sh                — 안전한 프로세스 처리
script/report_issue.sh                — issue 보고
script/run_action_files               — action file 실행
script/run_coverage_collect_and_upload — gcov/gcov 호출 + 업로드
script/run_cubrid_install             — CUBRID 설치
script/run_download                   — 다운로드 도구
script/run_general_feedback           — 일반 feedback
script/run_git_update                 — git pull 도구
script/run_grepo_fetch                — grepo 서비스 호출
```

→ 외부 CI / 운영자 / 다른 셸 스크립트 가 grep / source / direct invoke 가능. **셸 명령 시그니처 = 동결 표면**. 정확한 사용처 후속 분석.

### 2-2. `tpl/` — 템플릿

```
tpl/issue_create.tpl
tpl/issue_create_desc.tpl
tpl/issue_comment.tpl
```

`script/analyze_failure.sh` 가 변수 치환으로 사용. 새 시스템에서도 동일 텍스트 템플릿 보존 또는 모던 템플릿 엔진 (Mustache / Tera) 으로 교체.

### 2-3. `ext/` — 외부 모듈 진입 (10+ 셸 스크립트)

```
ext/run_sql.sh
ext/run_shell.sh
ext/run_jdbc.sh
ext/run_isolation.sh
ext/run_ha_repl.sh
ext/run_cdc_repl.sh
ext/run_unittest.sh
ext/run_coverage.sh
ext/run_compat_cci.sh
ext/run_compat_jdbc.sh
ext/run_sql_by_cci.sh
```

→ CTP/ 안 어디에서도 호출되지 않음 (deps-of-common.md §1). *외부 CI / 수동 실행* 의 진입점으로 추정. **셸 시그니처 = 동결 표면**.

### 2-4. `gcov/` — 외부 바이너리

```
gcov/gcov              — gcov 실행 파일 (어떤 버전?)
```

`script/run_coverage_collect_and_upload` 가 호출. 새 시스템에서 *시스템 gcov* 사용 또는 별도 동봉 정책 ADR.

---

## 3. CTP CLI 표면 (ctp 패키지)

(cli-tree.md 의 정밀 분석 참조)

```
ctp.sh <task>... [-c <conf>] [--interactive] [-h] [-v]

task ∈ ComponentEnum.values():
  SQL / MEDIUM / KCC / NEIS05 / NEIS08
  SHELL / RQG
  ISOLATION / HA_REPL / CDC_REPL
  JDBC / SQL_BY_CCI
  WEBCONSOLE / UNITTEST
  CCI / DOTS / NBD / SYSBENCH / TPCC / TPCW / YCSB  ← orphan (분기 없음)
```

→ 21 enum 동결. 7 orphan 의 폐기는 ADR.

종료 코드:
- 0 — 정상
- 1 — `JAVA_HOME` 미설정
- 비-0 — Java 측 Exception

`#SCRIPTCONT` 마커 — interactive SQL 모드의 in-band 셸 명령 forwarding (cli-tree.md §6).

---

## 4. 옵션 데몬 contract

### 4-1. grepo RMI 서비스

```
RMI binding: <unknown> (정밀 후속)
Server: common.grepo.service.RepoServiceImpl
Client: common.grepo.RepoClient
Interface: com.nhncorp.cubrid.common.grepo.RepoService (legacy nhn ns)
```

새 시스템에서 git CLI subprocess 또는 go-git 으로 대체 ADR.

### 4-2. scheduler

```
ActiveMQ broker connection: scheduler.common.ActiveMQFactory
Quartz scheduler: scheduler.producer.crontab.SchedularMain
Producer entry: scheduler.producer.Main
Consumer entry: scheduler.consumer.ConsumerAgent
```

ActiveMQ broker URL / Quartz job 정의는 별도 conf 에 있을 것 (정밀 후속). 옵션 컴포넌트 — 1차 제외.

---

## 5. cubridqa-common.jar 의 *jar 이름 자체* 가 동결 표면일 가능성

다른 모듈의 build/runtime classpath 에 `common/lib/cubridqa-common.jar` 가 *고정 이름* 으로 참조됨:
- ctp.sh: `JAVA_CPS=$CTP_HOME/common/lib/cubridqa-common.jar`
- 모듈 jar 의 MANIFEST Class-Path (정밀 후속)
- 외부 CI 스크립트 (사용처 후속)

→ 새 시스템이 이 jar 를 *완전히 대체* 하려면 같은 위치 + 같은 이름의 호환 jar 또는 *호환 layer* 제공.

---

## 6. 동결 표면 요약

```
입력 (모듈/외부가 common 에 의존)
├── Java import: 11+ 클래스 (CommonUtils/IniData/Log/Constants/ConfigParameterConstants/
│                  LocalInvoker/SSHConnect/SFTP/coreanalyzer.*/ShareMemory/MailSender/MakeFile)
├── ConfigParameterConstants 82 키 — type-safe 카탈로그
├── ext/run_<module>.sh — 외부 CI 진입 (10+)
├── script/* — CI 헬퍼 (analyze_failure / run_coverage_*/run_grepo_fetch 등)
├── tpl/issue_*.tpl — 템플릿
├── CTP CLI (ctp.sh) — 21 task name (7 orphan)
└── (옵션) grepo RMI / scheduler ActiveMQ

출력 (common 이 제공)
├── cubridqa-common.jar (common/lib/) — 핵심 산출물
├── cubridqa-scheduler.jar (common/sched/lib/) — 옵션
├── 셸 자산 그대로 deploy
└── stdout / 파일 로그 (Log 클래스)

종료 코드
└── 라이브러리이므로 N/A. CTP CLI 만 종료 코드 (위 §3)
```

---

## 7. 새 시스템 전환 시 contract 호환 전략

새 시스템이 common 을 *부분적으로 흡수* 하면서 기존 모듈과 공존하려면:

### Option A: jar 호환 layer
- 새 시스템이 *동일한 cubridqa-common.jar* 산출 (인터페이스 유지)
- 내부 구현은 새 언어/시스템 호출
- *bridge jar* 패턴

### Option B: subprocess 모델
- 새 시스템 binary 가 `cubrid-testkit` 으로 실행
- ctp.sh 가 *조건부* 로 새/구 시스템 호출
- 모듈은 기존대로 동작 (common 도 그대로)

### Option C: 점진 교체 (strangler-fig 정통)
- 새 시스템이 *처음부터* 일부 모듈을 대체
- common 의 *교체된 모듈에 한해서만* 새 동등물 사용
- 나머지는 기존 jar 그대로

**ADR-004 (1차 대체 모듈) 의 결정에 따라 전략 결정**. 본 io-contract.md 가 그 결정의 입력.
