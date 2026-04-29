# sql — DB Setup Functions: `make_sql_db_data` vs `make_db_data` 정밀 비교

**Source:** `cubrid-testtools/CTP/sql/bin/run.sh` (line 569-672)

implementation-notes.md / medium/implementation-notes.md 에서 미해결로 남겨둔 *두 함수의 정확한 차이* 정밀 분석.

**Phase 4 정밀 분석 status:** 완료

---

## 1. 호출 위치 (run.sh do_create_db, line 504-510)

```bash
if [ "${scenario_full_name}" == "medium" ]; then
    make_db_data mdb                       # medium suite
elif [ "${scenario_full_name}" == "site" ]; then
    make_db_data ${scenario_category}      # site suite (legacy)
else
    make_sql_db_data                        # sql / sql_by_cci / kcc / neis05 / neis08
fi
```

→ medium/site 만 `make_db_data <name>`, 나머지는 `make_sql_db_data` (인자 없음).

---

## 2. `make_sql_db_data` (line 569-585) — Java SP 적재

```bash
function make_sql_db_data()
{
    curDir=`pwd`
    echo "Load Java Stored Procedure Classes"
    cd ${CTP_HOME}/sql/function/stored_procedure/src
    rm *.class 2>&1 > /dev/null
    "$JAVA_HOME/bin/javac" -cp $CUBRID/jdbc/cubrid_jdbc.jar *.java

    for clz in $(ls *.class); do
        echo "Load ${clz}..."
        loadjava $db_name $clz 2>&1 >> $log_filename
    done

    rm *.class 2>&1 >/dev/null
    cd $curDir
}
```

**역할:** **Java Stored Procedure 컴파일 + DB 적재**

흐름:
1. `${CTP_HOME}/sql/function/stored_procedure/src/` 로 이동
2. 기존 `.class` 정리
3. `javac -cp $CUBRID/jdbc/cubrid_jdbc.jar *.java` — 모든 .java 컴파일
4. 컴파일된 `.class` 들을 `loadjava $db_name <clz>` 로 DB 에 적재
5. `.class` 정리 후 디렉터리 복귀

**전제:**
- `$JAVA_HOME` 설정
- `$CUBRID/jdbc/cubrid_jdbc.jar` 존재
- `$db_name` 가 *이미 createdb 된 상태*
- `loadjava` 명령 PATH

**미묘:**
- 컴파일 실패 시 *조용히 진행* (rm `.class` 가 항상 성공) → loadjava 가 빈 set 으로 통과
- `cubrid_jdbc.jar` classpath 만 — 다른 dependency 가 .java 안에 있으면 컴파일 실패

---

## 3. `make_db_data` (line 629-672) — 사전 빌드된 데이터 아카이브 적재

```bash
function make_db_data()
{
    curDir=`pwd`
    dataFileName=$1
    echo "Load Initial Data..."
    if [ -z "$test_data_file" ]; then
        echo "Not found data file ... please set ... data_file in $config_file_main"
        exit 1
    fi

    data_file=""
    if [ -d $test_data_file ]; then
        data_file=${test_data_file}/${dataFileName}.tar.gz
        [ ! -f $data_file ] && echo "..." && exit 1
    elif [ -f $test_data_file ]; then
        data_file=$test_data_file
    else
        echo "Not found data file ..."
        exit 1
    fi

    cd $cubrid_root_dir/databases/$db_name
    cp $data_file .

    tar -zxvf mdb.tar.gz                     # ⚠️ 하드코드 (§6 참조)
    loaddb=`cubrid loaddb 2>&1`
    if [[ $loaddb =~ "--no-user-specified-name" ]]; then
        cubrid loaddb -s ${db_name}_schema -i ${db_name}_indexes -d ${db_name}_objects \
                      -u dba ${db_name} --no-user-specified-name >> $log_filename
    else
        cubrid loaddb -s ${db_name}_schema -i ${db_name}_indexes -d ${db_name}_objects \
                      -u dba ${db_name} >> $log_filename
    fi
    optimize_db $db_name

    rm *.gz 2>&1 >/dev/null
    cd $curDir
}
```

**역할:** **사전 빌드된 데이터 아카이브 (.tar.gz) 의 압축해제 + cubrid loaddb 적재**

흐름:
1. `$test_data_file` config 검증 (필수)
2. data_file 결정:
   - 디렉터리: `${test_data_file}/${dataFileName}.tar.gz`
   - 파일: `$test_data_file` 직접
   - 외: exit 1
3. `$cubrid_root_dir/databases/$db_name` 으로 이동 (예: `$CUBRID/databases/mdb`)
4. data_file 복사
5. `tar -zxvf mdb.tar.gz` (⚠️ 하드코드, §6)
6. `cubrid loaddb` 인자:
   - `-s ${db_name}_schema` — schema 파일
   - `-i ${db_name}_indexes` — index 파일
   - `-d ${db_name}_objects` — objects 파일
   - `-u dba` — 사용자
   - `${db_name}` — DB 이름
   - **(11.5+) `--no-user-specified-name`** — 신규 빌드 호환
7. `optimize_db $db_name` (= `cubrid optimizedb $db_name`)
8. 압축 파일 정리

**전제:**
- `$test_data_file` config 키 (medium.conf 의 `data_file=` 가 export 한 변수)
- 아카이브 안에 `<db_name>_schema`, `<db_name>_indexes`, `<db_name>_objects` 3 파일 존재
- `cubrid loaddb` PATH

**미묘:**
- 11.5+ 빌드에서 `--no-user-specified-name` 플래그 *runtime detection* (loaddb 의 help 출력을 grep)
- 이전 빌드와의 호환을 위한 분기

---

## 4. `make_site_db_data` (line 587-590) — TODO 스텁

```bash
function make_site_db_data()
{
    echo "todo"
}
```

→ *정의는 있으나 호출 안 됨*. site suite 도 `make_db_data ${scenario_category}` 로 처리. **dead code** — 새 시스템에서 제거.

---

## 5. 차이 매트릭스

| 항목 | `make_sql_db_data` | `make_db_data` |
|------|---------------------|----------------|
| 인자 | 없음 | `dataFileName` (e.g. "mdb") |
| 호출 suite | sql / sql_by_cci / kcc / neis05 / neis08 | medium / site |
| 핵심 작업 | Java SP **컴파일 + loadjava** | 데이터 아카이브 **압축해제 + cubrid loaddb** |
| 입력 | `${CTP_HOME}/sql/function/stored_procedure/src/*.java` | `$test_data_file` (config `data_file`) |
| 외부 명령 | javac, loadjava | tar, cubrid loaddb, cubrid optimizedb |
| 실패 정책 | 컴파일 실패 silent | 데이터 file 부재 즉시 exit 1 |
| DB 생성 가정 | 이미 createdb 됨 | 이미 createdb 됨 |
| 후처리 | `.class` 정리 | `.gz` 정리 |
| Java SP 활성 conf | `java_stored_procedure=true` (sql.conf 에만 키 존재) | (의미 없음) |
| 데이터 아카이브 | (없음 — SP 만 적재, 데이터 적재 없음) | `<dataFileName>.tar.gz` |
| OS 호환 | Linux/Windows | Linux/Windows (tar -z 의존) |

---

## 6. ⚠️ 발견된 hardcode 버그

```bash
tar -zxvf mdb.tar.gz   # ← line 660
```

**문제:** `make_db_data` 가 인자 `dataFileName` 으로 데이터 파일 위치를 결정하지만, **압축해제 시점에 파일 이름을 `mdb.tar.gz` 로 하드코드**.

호출 패턴:
```bash
make_db_data mdb                       # medium → cp ${test_data_file}/mdb.tar.gz . ; tar -zxvf mdb.tar.gz  ✓
make_db_data ${scenario_category}      # site → cp ${test_data_file}/<category>.tar.gz . ; tar -zxvf mdb.tar.gz  ✗
```

site suite 가 `${scenario_category}` 인자로 호출 시 **잘못된 파일을 압축 해제 시도** → tar fail → loaddb 도 fail.

**확인:** site suite 가 실제로 사용되는지 (`make_site_db_data` 가 todo 스텁인 점에서 site 도 dead code 추정). git log 확인 후 dead 면 제거 ADR.

→ **새 시스템에서는 인자 기반 path 사용:**
```bash
tar -zxvf "${dataFileName}.tar.gz"
```

---

## 7. 관계 정리 — 두 함수는 *대안* 이 아니라 *서로 다른 역할*

처음 design.md 작성 시 두 함수가 *분기되는 대안* 이라고 가정했으나, 실제로는:

- **`make_sql_db_data`** = SP 적재 (DB 안에 stored procedure 가 *코드로* 존재해야 하는 sql 시나리오)
- **`make_db_data`** = 데이터 적재 (DB 안에 *대량 데이터* 가 미리 있어야 하는 medium 시나리오)

→ 두 시나리오의 셋업 *전제가 다르다*. sql 케이스는 빈 DB 에 *SP 만* 적재하고 케이스 안의 SQL 이 데이터 INSERT. medium 케이스는 *대량 데이터가 미리 적재된 DB* 위에서 *읽기/조작* 검증.

새 시스템에서 *공통 setup 패턴* 으로 표현 가능:
```yaml
# suite metadata
suite.sql:
  setup:
    - load_stored_procedures: ${CTP_HOME}/sql/function/stored_procedure/src

suite.medium:
  setup:
    - load_data_archive: ${data_file}/mdb.tar.gz
    - cubrid_loaddb:
        schema: mdb_schema
        indexes: mdb_indexes
        objects: mdb_objects
        user: dba
    - optimize_db: mdb
```

→ *suite-level setup recipe* 로 정형화. plug-in 모델 (각 케이스 또는 suite 가 setup step 을 list 로 선언).

---

## 8. 새 시스템 ADR 권고

**ADR 후보 — DB Setup Recipe 정형화** (가칭 ADR-006):

1. **`make_sql_db_data` → `setup_stored_procedures` recipe**
   - 입력: `${CTP_HOME}/<module>/function/stored_procedure/src/`
   - 출력: DB 안에 SP 적재
   - 모듈 별로 독립 (sql 외 다른 모듈도 사용 가능)

2. **`make_db_data` → `load_data_archive` recipe**
   - 입력: `data_file` config (디렉터리 또는 파일)
   - 출력: tar 압축해제 + cubrid loaddb + optimize_db
   - **버그 수정:** dataFileName 인자를 tar 명령에도 사용

3. **`make_site_db_data` 제거** (dead code)

4. **suite metadata 도입** — 각 suite 가 *어떤 recipe 를 어떤 인자로* 사용하는지 명시

5. **현 세 가지 분기 (medium / site / 그 외) 통합** — 모든 suite 가 동일한 setup 모델을 사용하되 *recipe list* 로 차이 표현

6. **11.5+ 빌드 의 `--no-user-specified-name` 분기** — *빌드 버전 capability detection* 을 *명시적 빌드 메타* 로 승격 (runtime grep 보다 안전)

---

## 9. 미해결 후속

- `${CTP_HOME}/sql/function/stored_procedure/src/` 의 .java 파일 목록 (몇 개의 SP, 어떤 기능?)
- `loadjava` 명령의 정확한 동작 (CUBRID 의 자체 명령? bash alias?)
- `data_file=` config 값으로 사용되는 표준 데이터 아카이브 location (예: 빌드 자산 또는 별도 testcases-data 레포)
- site suite 의 *현재 활성 여부* (case-formats.md 에서 site 디렉터리 발견 안 됨 — 이미 dead 추정)
- `cubrid optimizedb` 의 실제 효과 (분석 통계 갱신 추정)
