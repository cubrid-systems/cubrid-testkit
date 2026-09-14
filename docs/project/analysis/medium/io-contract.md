# medium — I/O Contract (sql 의 suite 변형)

**Source:** medium 은 sql 모듈의 suite 변형 — 외부 표면 대부분이 `analysis/sql/io-contract.md` 와 동일.

본 문서는 **medium 만의 delta** 만 기록.

---

## 1. CLI 표면

```
ctp.sh medium [-c <medium.conf>] [-h] [-v]
```

- 미지정 conf: `$CTP_HOME/conf/medium.conf`
- `medium_dev.conf` 도 별도 존재 — 동일 흐름, 1 키 추가 (`create_table_reuseoid`)
- 종료 코드: sql 과 동일 (0 정상, 1 환경 부재)

내부 호출 시그니처:
```
CTP.executeSQL(config, "medium", interactiveMode, useCCI=false)
  → sh sql/bin/run.sh -s medium -f <conf>
       → java cqt.console.ConsoleAgent runCQT medium <alias> <bits> <charset> <repo>?db=mdb_qa[...]
```

---

## 2. medium.conf — 21 키 (sql.conf 와 차집합)

### 2-1. medium 에 *있고* sql 에 *없는* 키
```
data_file                     — 데이터 적재 입력 경로 (필수, medium 의 핵심 표면)
```

### 2-2. medium 에 *없고* sql 에 *있는* 키
```
APPL_SERVER_MAX_SIZE
SetAPPL_SERVER_MAX_SIZE
cubrid_createdb_opts
java_stored_procedure
sql_interface_type
```

→ medium 은 *broker memory / java SP / interface type 을 config 로 노출하지 않는다*. 시나리오가 디폴트 값으로 충분.

### 2-3. 공통 키 (sql.conf 와 medium.conf 모두에 존재)
- multi-instance 토폴로지 (default.* / env.instance{1,2}.*)
- scenario / testcase_retry_num / testcase_timeout_in_secs / testcase_exclude_from_file
- cubrid_download_url
- enable_memory_leak / db_charset / unicode_input_normalization / lock_timeout / max_plan_cache_entries / update_statistics_on_catalog_classes_yn / need_make_locale

(전체 비교는 `analysis/_overview/conf-matrix.md` 참조)

### 2-4. medium_dev.conf 의 추가 키
```
create_table_reuseoid          — DEV 환경 한정 옵션 (REUSE_OID 클래스 테이블 생성)
```

---

## 3. testcases 측 입력

### 3-1. 케이스 형식 (sql 동일)
- `<dir>/cases/<name>.sql` + `<dir>/answers/<name>.answer`
- `--+ pragma` 주석 지시어 보존
- CUBRID 확장 SQL 보존

### 3-2. medium 만의 변형 확장자
- `.api` (10) — sql 인터페이스 외 API 호출 케이스 (정확한 의미 후속)
- 그 외는 sql 의 변형 (`.0_S64_patch`, `.0_D_patch`, `.answer_win`, `.gz`) 의 *부분 집합*

### 3-3. 케이스 디렉터리 (case-formats.md §3)
```
cubrid-testcases/medium/
├── _04_full/{cases,answers}/
├── _06_fulltests/{cases,answers}/
├── _07_mc_dep/{cases,answers}/
├── _08_mc_ind/{cases,answers}/
└── config/
```

11 sub-dirs at depth 1, 모두 `cases/` + `answers/` 자매 디렉터리.

---

## 4. ConsoleAgent runCQT 인자 (sql 동일하나 type="medium")

```
java cqt.console.ConsoleAgent runCQT medium <typeAlias> <version> <charset_xml> <repo>?db=mdb_qa[...]
```

cqt 측에서 `type=="medium"` 의 분기는 *명시적 코드 없음* — sql 과 동일 경로. 결과 라벨링 (`testCategory = "medium_<bits>bit"`) 만 차이.

---

## 5. 출력

sql 과 동일:
- stdout 라인 마커 (Result Root Dir, Fail/Success/Total banner)
- `<resultDir>/main.info`
- `<resultDir>/summary_info` (core 발견 시)
- cqt.log

---

## 6. medium 만의 동결 표면 (sql 컨트랙트 + delta)

```
입력 (delta)
├── ctp.sh medium [-c <conf>]
├── medium.conf 21 키 (data_file 필수, broker/SP/interface 키 없음)
├── medium_dev.conf 22 키 (+ create_table_reuseoid)
├── testcases/medium/<...>/cases/*.sql + answers/*.answer
├── data_file 의 데이터 아카이브 (포맷 후속 분석)
└── ctp.sh 의 task name "medium" / "MEDIUM" (대소문자 무관)

출력 (sql 동일)
├── stdout: sql 모듈과 동일 마커
├── <resultDir>/main.info — testCategory="medium_<bits>bit"
└── medium-specific 차이 없음

원격 동작 (delta)
├── createdb mdb (sql 의 basic 대신)
├── make_db_data mdb (sql 의 make_sql_db_data 대신)
└── Windows 에서 locale 빌드 skip
```
