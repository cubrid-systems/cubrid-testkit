# Module: sql — 매핑 표

- **Date:** 2026-09-02 · **Status:** Accepted (Phase 2, 매핑 수준 — ADR-004 Consequence 1)
- **담당 task:** `sql` · `medium` · `kcc` · `neis05` · `neis08` · `sql_by_cci` · `webconsole`
- **Phase 3 에서의 처리:** **미대체 — `legacy` Runner 가 기존 자산을 subprocess 로 호출**

---

## 1. 왜 1차가 아닌가

자율성이 가장 높아 회귀 검증은 가장 깨끗하지만 분량이 가장 크다 — 9-phase 파이프라인 + 35 클래스 협업,
그리고 CCI 모드(`ccqt` C 바이너리)를 즉시 떠안게 된다 (ADR-004 §7-3).

## 2. 신 ↔ 구 매핑 (개요)

| 구 | 신 (대체 시) |
|---|---|
| `sql/bin/run.sh` (917줄) — 6단계 파이프라인 | `runner/sqlsuite` |
| `cqt.console.ConsoleAgent` — 9-phase, 35 클래스 | `runner/sqlsuite` + `caseformat/sql` |
| answer variant 선택 (runMode / runModeSecondary) | `caseformat/sql` 의 `Case.Answers` 선택 (C2) |
| `--+ pragma` · `--@<connId>` · `--@queryplan` | `caseformat/sql` 파서 |
| URL-style specifier `<repo>?db=X&filter=Y` | `caseformat/sql` |
| `sql_by_cci/ccqt` (C 바이너리) | subprocess 유지 |
| `cqt.webconsole.*` | **축 O — 제외.** 진입점만 F1 유지하고 기존 자산 호출 |
| `common.coreanalyzer.AnalyzerMain` | `coreanalyze` (축 T — `CORE_FILE:` 가 F1) |

## 3. 공존 기간에 지켜야 하는 것 (freeze §8)

`legacy` Runner 가 **argv/env 를 바이트 단위로 재현**해야 한다.

```
sh $CTP_HOME/sql/bin/run.sh -s <suite> -f <conf>
  + export sql_interface_type=cci        (CCI 모드)
  + export sql_interactive=yes           (interactive)
  + export log_file_in_interactive=<path>
java cqt.console.ConsoleAgent runCQT <type> <typeAlias> <version> <charset_xml> <files...>
$CTP_HOME/sql_by_cci/ccqt <port> <db> <alias> <resultFolder> <repo> <CTP_HOME> <cci_urlproperty>
```

## 4. 미결

| 항목 | 어디로 |
|---|---|
| `.sql` pragma 전수 목록, `--@<connId>` 확인(추정 상태) | freeze §11-3 |
| answer variant 의 `runMode` **값 출처** (알고리즘은 확인됨) | freeze §11-7 |
| `.diff_1` 이 입력인가 산출물인가 | freeze §11-12 |
| `jdbc_config_file` charset XML 스키마 | freeze §11-16 |
| `ErrorInterrupt` cascade-abort 정책 | freeze §11-17 |
| DB setup recipe 정형화 (`make_sql_db_data` / `make_db_data`, tar 하드코딩 버그) | ADR-006 |
