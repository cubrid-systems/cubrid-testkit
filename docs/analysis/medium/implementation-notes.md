# medium — Implementation Notes (sql 과의 delta)

**Source:** `cubrid-testtools/CTP/sql/bin/run.sh` (medium 분기 3곳)

medium 은 별도 코드가 없으므로 본 문서는 sql 의 implementation-notes 를 *상속* 하면서, **medium 에 한해 다르게 동작하는 미묘한 부분** 만 기술한다.

`analysis/sql/implementation-notes.md` 의 19개 노트는 모두 medium 에도 적용된다 (run.sh 같은 코드 경로). 추가:

---

## 1. db_name 결정의 조용한 분기 (run.sh:237-243)

```bash
if [ "$scenario_full_name" == "medium" ]; then
    db_name="mdb"
elif [ "$scenario_full_name" == "site" ]; then
    db_name="${scenario_category}"
else
    db_name="basic"
fi
```

- 사용자 conf 에 `db_name` 을 명시해도 *오버라이드되지 않을 수 있음* (run.sh 가 do_init 의 분기로 *덮어쓴다*).
- 즉 medium suite 로 호출하면 *db_name 이 자동으로 mdb*. 사용자가 conf 에서 다른 값을 주면 *어디서 적용되는지 명확하지 않음*.

→ 새 시스템에서 *우선순위 명시* (config > suite 디폴트). 또는 case-by-case 로 db_name 선택 가능하게.

---

## 2. Windows locale skip 의 *implicit 가정* (run.sh:426)

```bash
if [ "$os_type" == "Windows" ]; then
    if [ "$scenario_full_name" != "medium" -a "$scenario_full_name" != "site" ]; then
        # locale 빌드
    fi
fi
```

- medium 은 *locale 빌드를 안 한다는 가정* — 케이스 작성자가 의존할 수 있음 (locale 변경 없이 실행)
- *예외 케이스* (예: medium 안에 i18n 시나리오를 추가하려 할 때) 가 발생하면 *조용히 실패*

→ 새 시스템에서 *suite 별 locale 정책을 명시적 metadata* 로 표현.

---

## 3. `make_db_data mdb` vs `make_sql_db_data` — 함수 차이

run.sh 의 `do_create_db`:
```bash
if [ "${scenario_full_name}" == "medium" ]; then
    make_db_data mdb
elif [ "${scenario_full_name}" == "site" ]; then
    make_db_data ${scenario_category}
else
    make_sql_db_data
fi
```

- `make_db_data <db>` 와 `make_sql_db_data` 는 *서로 다른 함수* (둘 다 run.sh 안에 정의 추정)
- 두 함수의 정밀한 동작 차이는 본 분석에서 미파악 — Phase 1 진입 전 후속 분석 권고
- 추정: `make_db_data` 는 `data_file` config 키의 데이터 아카이브 적재, `make_sql_db_data` 는 sql suite 의 인라인 데이터 셋업

→ 새 시스템에서 *데이터 적재 모델 통일* (suite 별 dispatch 대신 plug-in 의 표준 인터페이스).

---

## 4. `data_file` 의 부재 시 동작 미정

medium.conf 에 `data_file` 이 *있어야 한다* 는 의무 — 그러나 run.sh 가 부재 시 *명시적 fail* 하는지 *조용히 진행* 하는지 미파악. `make_db_data mdb` 함수의 내부 동작에 의존.

→ 새 시스템에서 *명시적 검증* 권고.

---

## 5. medium_dev 의 `create_table_reuseoid` 적용 시점

medium_dev.conf 의 추가 키. *DB createdb 단계* 에서 `cubrid createdb --reuse_oid <db>` 로 적용되는 것으로 추정 (정확한 호출 위치는 후속).

REUSE_OID 옵션은 CUBRID 의 *클래스 OID 재사용 정책* — 일부 시나리오 (예: 대량 DDL 반복) 에서만 의미. 새 시스템에서 dev/prod 변형 처리 ADR.

---

## 6. medium 케이스의 `.api` 확장자 (10개)

testcases/medium 에만 존재하는 확장자 (case-formats.md §3). sql 케이스에는 없음. 추정:
- API 호출 (예: stored procedure 호출, native API) 케이스
- 정답 파일은 별도 형식 또는 일반 `.answer` 사용

→ 정밀 형식 후속 분석. 새 시스템에서 *.api 케이스의 dispatch 분기* 를 어떻게 표현할지 결정.

---

## 7. `medium 위치 미확정` 캐비어트 — Phase 0 결과로 확정

`docs/analysis/medium/` 의 stub 들이 작성될 때 spec 은 medium 위치가 *불확실* 하다고 명시했다 (M0 #4 시작 전).

본 분석으로 **확정**:
- medium 은 별도 디렉터리 / Java 코드 없음
- sql 모듈의 suite 변형
- 케이스는 `cubrid-testcases/medium/` 에 (별도 testcases 트리)
- 분기는 `sql/bin/run.sh` 안 3곳

→ ROADMAP 의 후속 cleanup 에서 `analysis/medium/` 위치를 `analysis/sql/medium/` 으로 *재배치 권고* (medium/design.md §6-1).

---

## 8. cqt 측 `type == "medium"` 분기 부재

`cqt.console.ConsoleAgent.runTest` 안에서 `type == "medium"` 의 *명시적 분기 없음*. 즉 ConsoleAgent 입장에서 medium 과 sql 은 *완전히 같은 코드 경로*. 차이는 testCategory 라벨링 (`"medium_64bit"` vs `"sql_64bit"`) 만.

→ 새 시스템에서도 *cqt 동등물 안에서 sql/medium 구분 무관* — suite metadata 만 다른 라벨로 보존.

---

## 9. medium suite 가 사용 빈도 / 중요도 측면에서 sql 보다 *작지만 critical*

- 케이스 수: 970 (sql 17,411 의 5.6%)
- *데이터 적재* 라는 핵심 시나리오에 집중
- run.sh 분기로 *별도 cleanup / setup 흐름* 이 있음

→ 작지만 *별도 정체성 보존* 가치 있음. 단순히 sql 안에 흡수하는 것보다 *suite 변형* 으로 명시 표현 권고.

---

## 10. site suite — medium 과 같은 분기 패턴

run.sh 의 3곳 분기 모두 `medium` 와 함께 `site` 도 다룬다. 즉 site suite 도 medium 과 비슷한 *별도 db_name + locale skip + make_db_data* 모델. 단 site 는 ComponentEnum 에 *없음*.

→ legacy / 외부 호출 가능 (run.sh 직접 실행) 추정. 새 시스템에서 site 의 정체 확정 후 폐기 또는 정식 enum 추가 ADR.
