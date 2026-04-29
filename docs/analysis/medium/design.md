# medium — Design (sql 의 suite 인스턴스)

**Source:** `cubrid-testtools/CTP/` (medium 은 별도 디렉터리 없음 — sql 모듈의 suite alias)

**Phase 0 M0 #4 status:** 완료 (medium = sql 의 변형으로 확정)

---

## 1. 핵심 결론 — medium 은 별도 모듈이 아니다

3중 분석(cli-tree / conf-matrix / sql design)이 모두 일관되게 보여준다:

| 분석 | 발견 |
|------|------|
| cli-tree.md | CTP.executeSQL 의 case 분기 안에서 MEDIUM은 SQL과 같은 메서드 호출, suite="medium" 만 차이 |
| conf-matrix.md | medium.conf 21 키 모두 sql.conf 또는 medium_dev.conf와 공유 (exclusive 0). data_file 만 sql에 없음 |
| sql/design.md | run.sh 의 분기 3곳에서 `scenario_full_name == "medium"` 검사 — 별도 코드 경로 없음 |

따라서 medium 은 *별도 모듈 분석 산출물이 아닌* sql 모듈의 suite 변형으로 다루는 것이 맞다. 이 디자인 문서의 나머지는 medium 만의 차이점만 기술한다.

---

## 2. 디스패치 경로 (CTP.java)

`CTP.java` (cli-tree.md §switch dispatch 트리)에서:

```java
case MEDIUM:
    executeSQL(getConfigData(taskLabel, configFilename, "medium"),
               "medium", interactiveMode, false);
    break;
```

→ `executeSQL(config, "medium", interactive, useCCI=false)`. SQL 케이스(`"sql"`)와 같은 메서드, **suite 인자만 다르다**:

```sh
sh ${CTP_HOME}/sql/bin/run.sh -s medium -f <conf>
```

이후 흐름은 sql/design.md §3 `do_test` 의 default(JDBC) 분기와 동일:
```
java cqt.console.ConsoleAgent runCQT medium <alias> <bits> <charset> <repo>?db=<db>_qa
```

ConsoleAgent 입장에서 `type == "medium"` 만 sql과 다르며, 그 영향은 `testCategory = "medium_64bit"`(타입+bit 조합)으로 결과 디렉터리 라벨링 정도.

---

## 3. run.sh 안의 medium 분기 3곳

`sql/bin/run.sh` 안에서 `scenario_full_name == "medium"` (또는 `${scenario_category}`)으로 분기되는 지점은 정확히 3곳:

### 3-1. db_name 결정 (do_init, line 237-243)

```sh
if [ "$scenario_full_name" == "medium" ]; then
    db_name="mdb"
elif [ "$scenario_full_name" == "site" ]; then
    db_name="${scenario_category}"
else
    db_name="basic"
fi
```

→ medium 은 **DB 이름이 `mdb`** (sql은 `basic`). 결과 DB는 `${db_name}_qa` 패턴이므로 `mdb_qa`.

### 3-2. Windows에서 locale 빌드 스킵 (line 426)

```sh
if [ "$os_type" == "Windows" ]; then
    if [ "$scenario_full_name" != "medium" -a "$scenario_full_name" != "site" ]; then
        echo "make locale now"
        # ... cubrid_locales.* 백업 + make_locale.bat 호출
    fi
fi
```

→ medium 은 Windows 에서 **locale 빌드를 건너뛴다**. (sql 은 한다)

### 3-3. DB 데이터 적재 함수 (do_create_db, line 504-510)

```sh
if [ "${scenario_full_name}" == "medium" ]; then
    make_db_data mdb
elif [ "${scenario_full_name}" == "site" ]; then
    make_db_data ${scenario_category}
else
    make_sql_db_data
fi
```

→ medium 은 `make_db_data mdb` (테스트 데이터 dump 적재 함수), sql 은 `make_sql_db_data` (sql 셋업 함수). 두 함수의 내부 차이는 후속 분석에서 보강 (이번 design seed 범위 외).

---

## 4. conf 차이 (conf-matrix.md 와 일관)

medium.conf vs sql.conf 의 키 차집합:

| 키 | sql.conf | medium.conf | medium_dev.conf |
|----|---------|--------------|------------------|
| `data_file` | ✗ | ✓ | ✓ |
| `APPL_SERVER_MAX_SIZE` | ✓ | ✗ | ✗ |
| `SetAPPL_SERVER_MAX_SIZE` | ✓ | ✗ | ✗ |
| `cubrid_createdb_opts` | ✓ | ✗ | ✗ |
| `java_stored_procedure` | ✓ | ✗ | ✗ |
| `sql_interface_type` | ✓ | ✗ | ✗ |
| `create_table_reuseoid` | ✗ | ✗ | ✓ (medium_dev 전용) |

**해석:**
- **medium 은 `data_file` 키로 적재할 데이터 파일 경로를 받는다** (sql 은 함수 안에 하드코드)
- **medium 은 broker 메모리 한계, createdb 옵션, java SP, interface type 을 *config-driven 으로 노출하지 않는다*** — 디폴트 값으로 충분한 시나리오. medium 은 더 단순한 데이터-적재 검증 시나리오라는 의도.
- **medium_dev** 는 medium 과 동일하지만 `create_table_reuseoid` 한 키만 추가 — DEV 환경 한정 옵션 (REUSE_OID 클래스 테이블 생성).

---

## 5. cqt 안의 medium 처리

ConsoleAgent.runTest 안에서 `type == "medium"` 인 경우의 특수 처리는 **명시적이지 않다**. testCategory 라벨링 외엔 sql과 같은 코드 경로를 탄다.

→ Java 측에서는 **medium ≡ sql** 사실상 동일.

---

## 6. 새 시스템 설계 시 권고 (medium 관점)

1. **medium 을 별도 모듈로 두지 말 것**. sql 모듈의 *suite 변형* 으로 표현하는 것이 의도와 일치. 새 시스템의 디렉터리 구조에서 `analysis/sql/medium/` 또는 `sql/suites/medium/` 같은 위치가 자연스럽다.

2. **3가지 분기는 시나리오 메타데이터로 승격**. db_name(`mdb`), windows-locale-skip, data-file-driven 셋업을 `suite.toml`(또는 동등 메타) 한 파일로 표현하면 run.sh 의 if-ladder가 사라진다:
   ```toml
   [suite.medium]
   db_name = "mdb"
   skip_windows_locale = true
   data_loader = "make_db_data"
   data_file = "<from medium.conf>"
   ```

3. **medium_dev 는 medium 의 environment overlay** (1 키 차이). 새 시스템에서는 base + overlay 패턴을 명시화.

4. **`make_db_data` 함수 내부 차이 분석은 후순위**. M0 단계에서는 분기 위치 식별만으로 충분. Phase 1 (concept) 에서 새 시스템의 데이터 적재 모델을 결정할 때 정밀 분석.

5. **ROADMAP 갱신 권고**: M0 #4 가 완료되었으므로, `analysis/medium/` 의 5개 stub 중 design.md 이외 4개(requirements/implementation-notes/io-contract/test-corpus)도 sql 모듈의 동일 산출물 안에서 medium 변형 절을 두는 형태로 통합할 수 있다. 단, 즉시 폐기하기 보다는 Phase 0 의 후속 작업에서 정리.

---

## 7. medium 관련 외부 표면 (동결 대상)

sql 의 동결 표면을 그대로 상속하며, medium 만의 추가 약속:

- **DB 이름 규약**: medium → `mdb` 가 createdb 대상 (`${db_name}_qa` = `mdb_qa`)
- **`data_file` conf 키**: medium 시나리오의 데이터 적재 입력 경로
- **suite 식별자 `medium`**: `ctp.sh medium` 또는 `ctp.sh sql medium`처럼 task 이름으로 호출
- **Windows locale 빌드 스킵**: 사용자가 의존할 수 있는 동작 (Windows medium 실행 시 locale 빌드가 일어나지 않는다는 가정)

---

## ROADMAP 갱신

체크리스트의 `analysis/medium/design.md` 항목을 완료(`- [x]`)로 갱신.

**M0 #4 완료** — 4개 deep 모듈(isolation / shell / sql / medium) 모두 sequence-level seed 작성됨.

남은 M0 항목:
- M0 #5: `_overview/case-formats.md` (testcases 레포 케이스 파일 포맷 분포) — 5개 모듈 각각의 케이스 형식이 어떻게 다른지 분석
