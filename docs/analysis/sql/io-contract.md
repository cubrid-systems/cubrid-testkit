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
├── testcases/<...>/answers/*.queryPlan
├── testcases/<...>/answers/*.<ver>_<S64|D>_patch / _excluded_list
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
