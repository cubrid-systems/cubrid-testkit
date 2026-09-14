# sql — Test Corpus

**Source repos:**
- `cubrid-testcases/sql/` (public, 17,411 .sql)
- `cubrid-testcases/medium/` (public, 970 .sql) — sql 의 suite 변형
- `cubrid-testcases/sample/` (public, 11 .sql) — 작은 sample
- `cubrid-testcases-private` 의 sql 변형 (정밀 분석 후속)

case-formats.md §2 / §3 의 sql/medium 부분을 더 깊게 정리.

---

## 1. 규모 (sql 모듈 한정)

| 확장자 | 갯수 | 의미 |
|--------|------|------|
| `.sql` | 17,411 | 케이스 |
| `.answer` | 17,428 | 기본 정답 (+17 = 변형) |
| `.answer_cci` | 2,111 | CCI 인터페이스 정답 |
| `.queryPlan` | 934 | 쿼리 플랜 별도 비교 |
| `.diff_1` | 273 | 알려진 diff (무시 추정) |
| `.answer_D_utf8_C_utf8_bin` | 86 | DB=utf8, Client=utf8_bin |
| `.answer_D_iso88591_C_iso88591_en_ci` | 80 | DB=iso88591, Client=en_ci |
| `.answer_D_iso88591_C_utf8_bin` | 71 | mixed |
| `.answer_win` | 57 | Windows 변형 |
| `.todo` | 7 | 미완성 케이스 |
| `.answer_ci` | 6 | case-insensitive collation |
| `.0_S64_patch`, `.0_D_patch` | 6, 6 | 빌드 라인 0 한정 |
| `.0_S64_excluded_list`, `.0_D_excluded_list` | 5, 5 | 빌드 라인 0 한정 exclude |
| `.8_S64_patch` | 2 | 빌드 라인 8 한정 |
| `.bk`, `.back`, `.log`, `.conf` | 소수 | 운영 부산물 |

medium: 970 .sql + 982 .answer + 10 .api + 패치 변형. (medium/design.md)

---

## 2. 디렉터리 분류 (depth 1)

```
cubrid-testcases/sql/
├── _01_<topic>           ← 주로 issue 그룹 (issue 번호 기반 디렉터리)
├── _02_user_authorization
├── _03_<topic>
├── ...
├── _24_aprium_qa         ← 일부 카테고리 alias
└── (38 sub-dirs at depth 1)
```

→ **38개 카테고리** (38_*_<topic>) — 격리 수준 분류 (isolation 의 5종) 보다 훨씬 풍부. CUBRID 의 SQL 영역 전체 cover.

depth 2~5 까지 nested. 가장 깊은 케이스: `_24_aprium_qa/_05_non-ASCII/cases/<name>.sql` 또는 `_24_aprium_qa/_02_sql_extension/issue_<NNNN>_<feature>/cases/<name>.sql`.

---

## 3. cases / answers 자매 디렉터리 패턴

```
<scenario_dir>/
├── cases/
│   ├── <name1>.sql
│   ├── <name2>.sql
│   └── ...
└── answers/
    ├── <name1>.answer
    ├── <name1>.queryPlan          ← 같은 케이스의 플랜 정답
    ├── <name1>.answer_cci         ← 같은 케이스의 CCI 변형
    ├── <name1>.answer_D_utf8_C_utf8_bin   ← charset 매트릭스
    ├── <name2>.answer
    └── ...
```

→ 한 .sql 케이스에 *최대 6~7가지* answer 변형이 존재 가능 (`.answer`, `.answer_cci`, `.answer_win`, `.answer_<DB>_<C>_<coll>`, `.queryPlan`, `.diff_1`).

cqt 가 *config 의 db_charset / charset_xml / interface_type 보고 적합한 변형 선택*. 정확한 매칭 알고리즘은 cqt.console.util/PropertiesUtil + TestUtil 정밀 분석 후속.

---

## 4. .sql 케이스 형식

### 4-1. 표준 SQL + CUBRID 확장

```sql
--+ holdcas on;
--use index granted from DBA

call login('dba','') on class db_user;
create user user1;
create class xoo ( a int, b int);
create index idx1 on xoo(a,b);
create reverse index idx2 on xoo(b,a);
insert into xoo values(1,1);
create serial ser1;
grant select, insert on xoo to user1;
create trigger tri1
  after insert on xoo
  execute update object obj set a = a+100;

call login('user1','') on class db_user;

select * from dba.xoo;
select dba.ser1.next_value from db_root;
```

CUBRID 확장 SQL 사용:
- `call login(...) on class db_user` — 사용자 전환
- `create class` (table 의 OO 표현)
- `create reverse index`
- `create serial`
- `dba.<class>` (스키마 prefix)
- `db_root` pseudo-table

### 4-2. `--+` Pragma 주석 지시어

```sql
--+ holdcas on;
```

cqt 가 *pragma 지시어* 로 인식. holdcas 외 다른 pragma 후속 분석 (codebase grep 으로 추출 가능).

`--` 일반 주석은 케이스 설명용 (영문/주석 다양).

### 4-3. 케이스 파일 명명 규칙

- `CUBRID<NNN>.sql` (issue tracker 번호)
- `cbrd_<NNNNN>.sql` (Jira CBRD prefix 변형)
- `issue_<NNNN>_<feature>.sql`
- `<topic>_<variant>.sql`

→ 일관된 *issue 번호 매핑* 으로 회귀 검증의 추적성 우수.

---

## 5. .answer 형식

```
===================================================
    
null     

===================================================
0
===================================================
1
===================================================
```

- 구분자: `===================================================` (51 = 기호)
- 각 블록 = 하나의 SQL 결과 출력 (컬럼 정렬 공백 + 데이터)
- 빈 블록 = 결과 없음 (예: DDL)
- *컬럼 헤더 / 정렬 공백* 까지 그대로 비교 — 매우 strict

→ 새 시스템에서 *비교 알고리즘 동결* 필수. 단, 정렬 공백 차이 (terminal width 등) 가 회귀로 잡힐 수 있어 *환경 의존* 위험.

---

## 6. multi-build answer 매트릭스

### 6-1. `.<ver>_<S64|D>_patch`

```
.0_S64_patch       ← 빌드 라인 0, S64 한정
.0_D_patch         ← 빌드 라인 0, D 한정
.8_S64_patch       ← 빌드 라인 8, S64 한정
```

`<ver>` 는 빌드 메이저 (예: 0 = 11.x, 8 = 8.x, 9 = 9.x?). `S64` / `D` 는 빌드 라인 변형 (정확한 의미 후속 — `S64` = standalone 64bit?, `D` = development 라인?).

cqt 가 build version 보고 *patch 적용 후 정답 비교* 추정. 정확한 알고리즘 정밀 분석 후속.

### 6-2. `.<ver>_<S64|D>_excluded_list`

해당 빌드 라인에서 *실행 안 할 케이스 목록*. 빌드 의존 회귀 무시 정책.

### 6-3. `.diff_1`

알려진 diff 패턴. 273개 — *무시되는 차이* 또는 *수동 확인된 차이* 로 추정.

→ 새 시스템에서 *조건부 정답 시스템* 정형화 ADR 필요. 현재는 ad-hoc 명명 규칙으로 표현.

---

## 7. charset / collation 매트릭스

### 7-1. 명명 규칙

```
.answer_D_<dbcharset>_C_<clientcharset>[_<collation>]
```

예:
- `.answer_D_utf8_C_utf8_bin` — DB=utf8, Client=utf8, Collation=bin
- `.answer_D_iso88591_C_iso88591_en_ci` — DB=iso88591, Client=iso88591, Collation=en_ci
- `.answer_D_iso88591_C_utf8_bin` — mixed

### 7-2. cqt 의 매칭

`config.db_charset` + `charset_xml` 의 client charset 으로 매칭. 정확한 우선순위 (정확 매칭 → fallback `.answer`) 후속 분석.

### 7-3. `.answer_ci` (case-insensitive)

collation 만 다른 변형. (정확한 활성 조건 후속.)

---

## 8. medium / sample / kcc / neis05 / neis08 의 차이

(case-formats.md §3 + medium/design.md)

| Suite | 케이스 수 | 디렉터리 | 비고 |
|-------|----------|----------|------|
| sql | 17,411 .sql | cubrid-testcases/sql/ | 메인 |
| medium | 970 .sql | cubrid-testcases/medium/ | sql 의 sub-set + data_file 의존, db_name=mdb |
| sample | 11 .sql | cubrid-testcases/sample/ | 작은 샘플 (validation) |
| kcc | (확인 필요) | (확인 필요) | 도메인 전용 |
| neis05 | (확인 필요) | (확인 필요) | 도메인 전용 |
| neis08 | (확인 필요) | (확인 필요) | 도메인 전용 |

→ kcc / neis05 / neis08 는 conf-matrix.md 에서 별도 conf 가 보이지 않음 (sample.conf 로 통합되었거나 외부 존재). 정밀 후속.

---

## 9. config 디렉터리 (cubrid-testcases/sql/config/ ?)

(존재 여부 확인 후속 — isolation 과 medium 은 config/ 가짐)

---

## 10. 케이스 작성자 측 contract (동결 표면 요약)

새 시스템 sql 대체 시 *변경 없이 보여야 할 것*:

1. ✅ `<dir>/cases/<name>.sql` + `<dir>/answers/<name>.answer` 자매 디렉터리
2. ✅ `--+ pragma` 주석 지시어 (holdcas on 등)
3. ✅ CUBRID 확장 SQL (class/object/serial/db_root/dba.*)
4. ✅ `===...===` 구분자 정답 포맷 + strict 공백/정렬 비교
5. ✅ `.answer_cci` / `.queryPlan` / `.diff_1` / `.answer_win` 변형
6. ✅ `.answer_D_<dbcharset>_C_<clientcharset>[_<coll>]` charset 매트릭스
7. ✅ `.<ver>_<S64|D>_patch` / `_excluded_list` 빌드 라인 시스템
8. ✅ `<repo>?db=<dbname>_qa[&filter=...]` URL-style scenario specifier
9. ✅ 케이스 파일 명명 (`CUBRID<NNN>` / `cbrd_<NNNNN>` / `issue_<NNNN>_*` 등)
10. ✅ `.todo` 파일은 미완성 표시 (실행 skip 또는 보고만)

---

## 11. 새 시스템 corpus 운영 권고

1. **case metadata 표준화** — 디렉터리 위치로 표현된 카테고리/issue 를 *케이스 헤더 SQL 주석* 으로 명시:
   ```sql
   -- @category: user_authorization
   -- @issue: CUBRID215
   -- @topic: grant
   -- @timeout: 30
   --+ holdcas on;
   ```

2. **lint 도구**:
   - `.todo` 케이스 정리 후보 검출
   - `.bk`, `.back`, `.log` 운영 부산물 cleanup
   - cases/ + answers/ 페어링 검증
   - charset 변형 누락 검출 (특정 케이스만 일부 변형이 있고 다른 변형은 없음)

3. **조건부 정답 시스템 정형화** — `.<ver>_<S64|D>_patch` / `.diff_1` / `.answer_<DB>_<C>` 매트릭스를 통합하는 *조건 표현식* 도입:
   ```yaml
   # case.meta.yaml
   answers:
     - file: case.answer
       when: default
     - file: case.answer_cci
       when: { interface: cci }
     - file: case.answer_D_utf8_C_utf8_bin
       when: { db_charset: utf8, client_charset: utf8, collation: bin }
     - file: case.0_S64_patch
       when: { build: ">=11.0", line: S64 }
       apply: patch
   ```

4. **공백/정렬 비교 정책** — strict vs lenient 모드 선택 가능. 환경 의존 위험 감소.

5. **케이스 디렉터리 청소** — `.todo`, 일관성 깨진 명명, 38 카테고리의 의미 정리.

6. **kcc / neis05 / neis08 정체 확정** — Phase 1 진입 전 확인. 폐기 후보면 ADR.
