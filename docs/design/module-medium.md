# Module: medium — 매핑 표

- **Date:** 2026-09-02 · **Status:** Accepted (Phase 2, 매핑 수준)
- **담당 task:** `medium` (그리고 `medium_dev.conf` 변형)
- **Phase 3 에서의 처리:** **미대체 — `legacy` Runner**

---

## 1. medium 은 독립 모듈이 아니다

Phase 0 이 확정한 사실이다. **medium 은 sql 모듈의 suite 변형**이며 코드 경로가 완전히 같다.

- conf 관점: `medium.conf` 21키 중 **exclusive 0개** — 전부 `sql.conf` 또는 `medium_dev.conf` 와 공유
- 호출 경로: `CTP.executeSQL(config, "medium", ...)` → `sql/bin/run.sh -s medium -f <conf>`
- cqt 안에 `type=="medium"` 분기가 **없다**. 결과 라벨(`testCategory = "medium_<bits>bit"`)만 다르다

따라서 **별도 Runner 를 만들지 않는다.** `runner/sqlsuite` 가 suite 이름으로 처리한다.

## 2. delta 만 정리

| | medium 에 있고 sql 에 없음 | medium 에 없고 sql 에 있음 |
|---|---|---|
| conf 키 | `data_file` (필수) | `APPL_SERVER_MAX_SIZE` · `SetAPPL_SERVER_MAX_SIZE` · `cubrid_createdb_opts` · `java_stored_procedure` · `sql_interface_type` |
| DB | `createdb mdb` · `make_db_data mdb` | `createdb basic` · `make_sql_db_data` |
| 케이스 | `.api` 확장자 (10개, 의미 후속) | — |
| 그 외 | Windows 에서 locale 빌드 skip | — |

`medium_dev.conf` 는 `medium.conf` + `create_table_reuseoid` 1키.

## 3. 미결

| 항목 | 어디로 |
|---|---|
| `data_file` 데이터 아카이브 포맷 | Phase 4 |
| `.api` 확장자의 의미 | Phase 4 |
| `make_db_data` 의 **`tar -zxvf mdb.tar.gz` 하드코딩 버그** — site suite 를 깨뜨린다 | ADR-006. 새 시스템에서는 올바르게 구현한다 (NG9 위반 아님 — 출력 표면이 아니라 내부 동작) |
