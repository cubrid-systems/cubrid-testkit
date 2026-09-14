# medium — Requirements (sql 의 suite 변형)

**Source:** medium 은 별도 디렉터리 없음 — `cubrid-testtools/CTP/sql/bin/run.sh -s medium` 으로 동작

**Companion docs:** `design.md` (sql 변형 확정), `analysis/sql/requirements.md` (모든 베이스 요구는 여기로 위임)

---

## 1. medium 이 해결하는 문제

sql 모듈의 *medium suite* 는 SQL 검증 중에서도 **데이터 적재 / mass-data 시나리오** 에 초점을 둔 변형이다.

design.md 에서 확인했듯 medium 은 별도 모듈이 아니라 sql 의 suite 변형. 따라서 핵심 요구는 `analysis/sql/requirements.md` 와 동일하며 **medium 만의 delta 만 본 문서에 기록**.

---

## 2. medium 만의 추가/변형 요구

### 2-1. 데이터 적재 시나리오 우선

medium 케이스 (970 .sql) 는 sql 케이스 (17,411 .sql) 와 비교해 *작은 코퍼스* 이지만, 케이스 디렉터리 구조 (`_04_full` / `_06_fulltests` / `_07_mc_dep` / `_08_mc_ind`) 에서 보이듯 **mass-data load + table dependency / independence 검증** 에 초점.

추정 의도: SQL 의미가 아니라 **데이터 모델 + 적재 + 일관성** 검증.

### 2-2. db_name = `mdb` (medium database) 약속

run.sh do_init 의 분기에서 medium 은 *db_name="mdb"* 로 강제. 사용자 conf 에 db_name 을 안 적어도 자동 결정. → 케이스 작성자가 *mdb* 라는 약속된 DB 이름을 신뢰할 수 있음.

### 2-3. Windows 에서 locale 빌드 skip

medium 은 *locale 빌드 없이* Windows 에서 실행 — 빠른 시작. 이는 medium 이 *locale-sensitive 케이스가 거의 없다* 는 가정. 새 시스템에서 같은 가정 유지.

### 2-4. `data_file` conf 키

medium 의 데이터 적재 입력은 *config-driven*:
```ini
data_file=<path-to-data-archive>
```

→ sql 은 함수 안에 하드코드된 데이터 적재 흐름인데, medium 은 *config 키* 로 노출. 새 시스템에서 *데이터 적재 입력의 first-class 표현* 으로 승격.

---

## 3. 외부 호출 형태

```
ctp.sh medium [-c <medium.conf>]
```

- 미지정 시 `$CTP_HOME/conf/medium.conf` 자동
- `medium_dev.conf` 도 별도 존재 (1키 차이: `create_table_reuseoid`)

내부 진입은 sql 과 동일:
```
CTP.executeSQL(config, "medium", interactive, useCCI=false)
  └─ sh sql/bin/run.sh -s medium -f <conf>
       └─ ConsoleAgent runCQT medium <alias> <bits> <charset> <repo>?db=mdb_qa[&filter=...]
```

---

## 4. 비기능 요구

sql 과 동일하나 다음 차이:

| 항목 | sql | medium |
|------|-----|--------|
| db_name 디폴트 | basic | **mdb** |
| Windows locale 빌드 | 활성 | **skip** |
| 데이터 적재 함수 | `make_sql_db_data` | **`make_db_data mdb`** |
| broker memory 한계 | `APPL_SERVER_MAX_SIZE` config | (config 키 없음 — 디폴트 사용) |
| Java SP | `java_stored_procedure` config | (config 키 없음 — 비활성) |
| createdb 옵션 | `cubrid_createdb_opts` config | (config 키 없음) |
| 인터페이스 | `sql_interface_type` config (jdbc/cci) | (config 키 없음 — JDBC 고정) |

---

## 5. 의존하는 외부 자원

sql 과 동일. 추가:
- `data_file` conf 키가 가리키는 데이터 아카이브 (path)

---

## 6. 새 시스템 설계 입력

medium 대체 시 *반드시 충족* (sql 의 contract 위에서):

1. ✅ `ctp.sh medium [-c <conf>]` CLI
2. ✅ db_name=mdb 디폴트
3. ✅ Windows locale 빌드 skip
4. ✅ `make_db_data mdb` 동등 데이터 적재
5. ✅ `data_file` config 키 의미 보존
6. ✅ medium_dev 의 `create_table_reuseoid` config 키 보존
7. ✅ medium suite 의 testcases 디렉터리 (`cubrid-testcases/medium/`) 호환

→ 새 시스템에서 medium 은 *별도 모듈로 표현하지 말고 sql 의 suite 변형* 으로 유지 (medium/design.md §6).

ADR 후보:
- medium / medium_dev / sql / sql_by_cci / sample / kcc / neis05 / neis08 의 *suite-variant 표현* 정형화 (현재는 case 분기로 표현)
- `data_file` 처럼 *suite 별 추가 conf 키* 의 first-class 표현
