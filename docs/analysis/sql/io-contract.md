# sql — I/O Contract

**Source:** `cubrid-testtools/CTP/sql/`

---

## 1. CLI 표면

```
ctp.sh sql        [-c <conf>] [--interactive] [-h] [-v]
ctp.sh medium     [-c <conf>]         # suite=medium 변형
ctp.sh kcc        [-c <conf>]         # suite=kcc
ctp.sh neis05     [-c <conf>]
ctp.sh neis08     [-c <conf>]
ctp.sh sql_by_cci [-c <conf>]         # useCCI=true (sql_interface_type=cci)
ctp.sh webconsole {start|stop}        # 별도 utility 분기
```

종료 코드:
- 0 — task 완료 (run.sh 의 6단계 pipeline 정상 종료)
- 1 — `$CUBRID` 없음 또는 conf 부재 (do_init 에서 즉시 exit)

---

## 2. run.sh 인자

```
sh sql/bin/run.sh -s <scenario_category> -f <conf_path>
```

- `-s` : suite name (sql / medium / kcc / neis05 / neis08 / sql_by_cci / rqg)
- `-f` : config 파일 절대경로

추가 환경변수 (CTP.executeSQL이 export):
- `sql_interface_type=cci` (CCI 모드일 때)
- `sql_interactive=yes` + `log_file_in_interactive=...` 등 (interactive 모드)

---

## 3. conf 스키마

### 3-1. sql.conf — 25 키 (M0 #2)

```
scenario                      — testcases 케이스 디렉터리 절대경로
scenario_alias                — alias (cqt 측 testCategory 라벨)
db_name                       — DB 이름 (default basic, run.sh do_init 에서 결정)
db_charset                    — DB charset
cubrid_bits                   — 32 / 64
jdbc_config_file              — charset/JDBC 메타 XML (default test_default.xml)
enable_memory_leak            — true 면 run_memory.sh 분기
cubrid_createdb_opts          — createdb 추가 옵션
need_make_locale              — locale 빌드 활성
java_stored_procedure         — Java SP 활성
lock_timeout
max_plan_cache_entries
unicode_input_normalization
update_statistics_on_catalog_classes_yn
APPL_SERVER_MAX_SIZE          — broker memory 한계 (sql 전용)
SetAPPL_SERVER_MAX_SIZE
sql_interface_type            — jdbc / cci (env var 로도 export)
testcase_retry_num
testcase_timeout_in_secs
testcase_exclude_from_file    — exclusion list path
cubrid_download_url           — build URL (옵션)
scenario                      — repo root (예: $HOME/dailyqa)
data_file                     — (medium 전용 키, sql 에는 없음)
...
```

### 3-2. sql_by_cci.conf — 23 키

sql.conf 와 거의 동일. CCI 모드에서 sql_interface_type=cci 가 *기본값*.

### 3-3. medium.conf — 21 키 (medium/design.md)

sql.conf 의 sub-set + `data_file` 추가. `APPL_SERVER_MAX_SIZE`/`SetAPPL_SERVER_MAX_SIZE`/`cubrid_createdb_opts`/`java_stored_procedure`/`sql_interface_type` 미포함.

### 3-4. dot-notation 와일드카드

isolation/shell 과 동일 의미. `default.cubrid.<property>` 등.

### 3-5. local.properties (cqt 별도 conf)

cqt working directory 의 `local.properties` 를 cqt 가 추가 로드:
```
isdebug=false              # cqt debug 모드
qaview=false               # qaview UI 통합
```

→ 새 시스템 conf 통합 시 *두 파일을 한 conf 로 합치는 것* 권고 (sql/design.md §10-6).

---

## 4. testcases 측 입력

### 4-1. URL-style scenario specifier

ConsoleAgent.runTest 의 files[] 인자:
```
<scenario_repo_root>/<scenario_path>?db=<db_name>_qa[&filter=<exclude_file>]
```

예: `/home/user/dailyqa/sql/_02_user_authorization?db=basic_qa&filter=exclusions.txt`

cqt 가 이 URL 을 파싱해서 *디렉터리 + DB + 필터* 를 결정.

### 4-2. 케이스 형식 (case-formats.md §2 참조)

- `<dir>/cases/<name>.sql`
- `<dir>/answers/<name>.answer`
- `<dir>/answers/<name>.<variant>` (cci/win/charset/queryPlan/patch/excluded_list)

### 4-3. SQL pragma 지시어

`--+ pragma` 형식 — 표준 SQL 주석으로 해석되지만 cqt 가 메타 인지:
- `--+ holdcas on;` — connection 유지
- (다른 pragma 후속 조사 — Phase 1)

### 4-4. testcase exclusion

config `testcase_exclude_from_file` (path) → 한 줄당 한 케이스 prefix/substring. 첫 글자 `#`/`--` 인 라인은 주석.

---

## 5. ConsoleAgent runCQT 인자

```
java cqt.console.ConsoleAgent runCQT <type> <typeAlias> <version> <charset_xml> <files...>
```

- args[0] = `"runCQT"` (커맨드)
- args[1] = type (suite name: sql / medium / kcc / ...)
- args[2] = typeAlias (config 의 scenario_alias)
- args[3] = version ("32" / "64")
- args[4] = charset_xml (default test_default.xml)
- args[5..] = files[] (URL-style scenario specifier 한 개 이상)

---

## 6. 출력 파일

### 6-1. log_filename
- run.sh 의 cqt.log (기본 `${CTP_HOME}/sql/log/cqt.log`)
- run.sh 가 stdout 을 `tee -a $log_filename` 로 기록
- do_summary_and_clean 가 `Result Root Dir`, `TOTAL_COUNT`, `TOTAL_ELAPSE_TIME` grep

### 6-2. <resultDir>/main.info (cqt 가 작성)
```
total:<N>
success:<N>
fail:<N>
totalTime:<ms>
SiteRunTimes:<...>
cubrid_rel:<...>          ← run.sh do_summary_and_clean 가 추가
user:<...>                ← 동일
machine:<...>             ← 동일
```

### 6-3. <resultDir>/summary_info (CCI 모드 또는 core 발견 시)
```
cubrid_build_id=<ver>
execute_date=<...>
Num_total=<N>
Num_test_total=<N>
Num_success=<N>
Num_fail=<N>
Test_cat=<alias>
Test_upcat=function
OS=<os>
Bit=<32|64>bits
Elapse_time=<sec>
test_error=Y              ← core 발견 시만
```

### 6-4. core dump
```
CORE_FILE:<path>          ← run.sh stdout
test_error=Y              ← summary_info 에 기록
```

---

## 7. stdout 마커 (외부 grep 대상)

```
Result Root Dir:<path>             ← cqt 가 작성, run.sh 가 첫 grep
total:<N>
success:<N>
fail:<N>
SiteRunTimes:<N>
totalTime:<ms>
TOTAL_COUNT:<N>                    ← cqt.log
TOTAL_ELAPSE_TIME:<ms>             ← cqt.log
CORE_FILE:<path>                   ← run.sh do_summary_and_clean
-----------------------
Fail:<N>
Success:<N>
Total:<N>
Elapse Time:<sec>
Test Log:<path>
Test Result Directory:<path>
-----------------------
Testing End!
```

→ 외부 CI / 모니터링 도구가 이 라인을 *그대로 grep* 할 가능성. 새 시스템 동결 표면.

---

## 8. CCI 모드 (sql_by_cci/ccqt) 인자

```
$CTP_HOME/sql_by_cci/ccqt <port> <db_name> <scenario_alias> <resultFolder> <scenario_repo_root> <CTP_HOME> <cci_urlproperty>
```

C 바이너리. 출력은 sql 모듈 일반 stdout 마커 호환 + summary.info 에 NOK 카운트로 기록.

---

## 9. webconsole 인자

```
java cqt.webconsole.Starter <webconsole.conf> <webRoot> <start|stop>
```

- `<webconsole.conf>`: `${CTP_HOME}/conf/webconsole.conf` (web_port + sql_result_root 2 키)
- `<webRoot>`: `${CTP_HOME}/sql/webconsole`

---

## 10. 동결 표면 요약

```
입력
├── ctp.sh {sql|medium|kcc|neis05|neis08|sql_by_cci|webconsole} [-c <conf>]
├── sql.conf 25 / medium.conf 21 / sql_by_cci.conf 23 키
├── local.properties (cqt working dir, isdebug/qaview)
├── jdbc_config_file (charset XML, default test_default.xml)
├── testcases/<...>/cases/*.sql + answers/*.answer{,_cci,_win,_<DB>_<C>[_coll]}
├── testcases/<...>/cases/*.queryPlan          ← cases/ 다. answers/ 아니다 (2026-09-03 정정)
│                                             sql/medium 합쳐 940개. answers/ 에 3개 있지만 죽었다
├── <module>/config/daily_regression_test_exclude_list_compatibility/
│       <ver>_<S64|D>_excluded_list           ← 22개. sql/medium 이 아니라 **호환성 스위트**의 입력
│       patch_files/<ver>_<S64|D>_patch       ← 40개. 위와 동상
├── URL-style scenario specifier <repo>?db=<dbname>_qa[&filter=...]
├── --+ pragma 주석 지시어
└── webconsole.conf (web_port, sql_result_root)

출력
├── stdout: Result Root Dir / total / success / fail / SiteRunTimes / totalTime / TOTAL_* / Fail/Success/Total summary banner
├── <resultDir>/main.info (total/success/fail/totalTime/cubrid_rel/user/machine)
├── <resultDir>/summary_info (CCI 또는 core 발견 시)
├── ${CTP_HOME}/sql/log/cqt.log
├── ${CTP_HOME}/sql/result/<resultFolder>/...
├── core dump 발견 시 CORE_FILE:<path> + test_error=Y
└── webconsole HTTP UI (web_port)

종료 코드
├── 0  (정상; 케이스 실패 포함)
└── 1  ($CUBRID 부재 / conf 부재)
```

## answer 선택 — cqt 는 `.answer` 하나만 읽는다 (2026-09-03)

`TestUtil.getAnswerFile(caseFile)` 이 만드는 경로는 하나다:

```
<케이스 경로의 /cases 를 /answers 로 치환>/<name>.answer
```

**변형 접미사를 조립하는 코드가 cqt 에 없다.** `.answer_cci`(2,119개) 와 `.answer_win`(59개) 는
코퍼스에 있지만 CTP 트리 전체의 `.java`·`.sh` 어디에서도 그 이름을 만들지 않는다. `sql_by_cci` 는
`ComponentEnum` 에 없어 `ctp.sh` task 가 아니고 `common/ext/run_sql_by_cci.sh` 가 직접 몬다 —
그쪽이 어떻게 답을 고르는지는 **Phase 4 에서 확인할 것** (§11-24).

### `answers32` — 기전은 있고 대상이 없다

디렉터리 이름을 고르는 경로가 둘 겹쳐 있다:

1. `ConsoleDAO` 가 `<db>.xml` 의 `version` 을 보고 **전역 가변 static** `TestUtil.OTHER_ANSWERS_32` 를
   `"answers32"`(32bits) 또는 `"answers"` 로 세팅한다
2. `getAnswer4SQLAndOther` 가 그 이름의 디렉터리가 케이스 옆에 **있는지** 보고, 있으면 쓰고 없으면
   `answers` 로 떨어진다

**코퍼스에 `answers32/` 디렉터리가 0개다.** 즉 이 기전은 현재 고를 대상이 없다.

⚠️ 그리고 그 static 은 **DB 마다 덮어써진다.** 한 실행에 DB 가 둘 이상이면 마지막 것이 이기고
케이스별로 결정되지 않는다. 지금은 대상이 없어 드러나지 않는다.

### charset 변형

`answer_D_<db charset>_C_<client charset>[_<collation>]` 형태가 코퍼스에 있다
(`answer_D_iso_C_iso_en_ci`, `answer_D_utf_C_utf_bin`, …). cqt 쪽에서 charset 을 다루는 것은
`getCharsetFile` 인데 그건 **`$CTP_HOME/<config>/<test_config>/<name>`** 의 XML 을 가리킬 뿐
answer 파일 이름과 무관하다 (기본값 `test_default.xml`). 이 변형들도 §11-24 대상이다.

## queryPlan · excluded_list · patch — 전부 입력이다 (2026-09-03 해소, freeze §11-12)

### queryPlan — 스위치이자 비교 대상

`TestUtil.isPrintQueryPlan(caseFile)` 이 켜는 방법이 **두 가지**다 (`ConsoleBO.java:1037` 주석):

1. `.sql` 안의 **`--@queryplan`** 프라그마 (`SQLParser.java:76`, 줄 전체가 정확히 그것)
2. **`<case>.queryPlan` 파일이 `.sql` 옆에 존재** — `caseFile` 의 확장자를 갈아끼운 경로다.
   그래서 `cases/` 에 있어야 하고 `answers/` 가 아니다
3. 그리고 `sql/configuration/System.xml` 의 전역 `queryPlan` 이 참이면 **모든 케이스가** 켜진다

**숫자 마스킹은 전역 스위치를 본다.** `StringUtil.replaceQureyPlan` (메서드 이름의 오타도 CTP 것):

| 전역 `System.xml` | 출력 |
|---|---|
| **참** | 그대로. 숫자 마스킹 없음 |
| **거짓** | `[0-9]+` → `?` 전체 치환. 그리고 `Query plan:` 구간은 전부, `Query stmt:` 구간은 첫 줄과 `/` 로 시작하는 줄만 남기고 나머지(`msg`)는 **버린다** |

⚠️ **켜는 판단(per-case)과 마스킹 판단(전역)이 서로 다른 스위치다.** 이름이 비슷해서 하나로 읽기 쉽다.
per-case `.queryPlan` 파일로 켠 경우, 전역은 거짓이므로 **숫자가 마스킹된 채 비교된다.**

`replaceJoingraph` 는 `Join graph` 구간만 남기고 `sel <숫자>.<숫자>` → `sel ?` 로 선택도를 지운다.

### excluded_list — sql/medium 것이 아니다

`cqt` 도 제외 목록을 읽지만 그건 **`testcase_exclude_from_file`** 이고, 두 conf 모두
`${CTP_HOME}/conf/exclusions.txt` 를 가리킨다 (`TestUtil.filterExcludedCaseFile` → 상대경로 부분일치).

`<ver>_<S64|D>_excluded_list` 22개와 `<ver>_<S64|D>_patch` 40개는 전부
`<module>/config/daily_regression_test_exclude_list_compatibility/` 아래에 있고, 읽는 곳은
`common/ext/run_compat_jdbc.sh` · `run_compat_cci.sh` — **호환성 스위트**다. `cases/` 나 `answers/` 에는
하나도 없다. 이 문서가 answer variant 로 분류했던 것은 오분류다.

### 죽은 파일 3개

`answers/*.queryPlan` 3개 (`where_clause` · `_08_enum_index` · `_016_insert_index`) 는 전부
`cases/` 에 살아 있는 쌍둥이가 있고, `isPrintQueryPlan` 은 `cases/` 만 본다. **읽히지 않는다.**

