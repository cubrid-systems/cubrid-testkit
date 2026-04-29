# sql — Requirements (모듈이 해결하는 문제 + 외부 호출 형태)

**Source:** `cubrid-testtools/CTP/sql/`

**Companion docs:** `design.md` (run.sh + cqt 시퀀스), `io-contract.md`, `implementation-notes.md`, `test-corpus.md`

---

## 1. 이 모듈이 해결하는 문제

CUBRID 의 **SQL 의미 / 결과 / 쿼리 플랜** 을 *대규모 SQL 케이스 코퍼스* (17,411 .sql) 로 회귀 검증한다.

각 케이스는 *SQL 스크립트 + 정답 텍스트* 의 페어로, 같은 SQL을 실행했을 때 출력이 *.answer 와 동일한지* 결정. CUBRID 확장 SQL (object-oriented constructs, dba/db_root, serial, trigger 등) 을 광범위하게 다룸.

추가로 sql 모듈은 다음도 책임:
- **DB 셋업/cleanup 자동화** — createdb / loaddb / locale 빌드 / java SP 적재
- **3가지 인터페이스 모드** — JDBC (default), CCI (native C via sql_by_cci/ccqt), interactive (사용자 셸)
- **다중 charset/collation 매트릭스** — DB charset × Client charset × Collation 변형 정답 매칭
- **빌드 라인 차이 흡수** — `.<ver>_S64_patch` / `.<ver>_D_patch` 메커니즘
- **memory leak 검증** — `enable_memory_leak=true` 시 `run_memory.sh` 변형 실행
- **webconsole** — 결과 시각화 (별도 utility, sql/webconsole)

---

## 2. 외부 호출 형태

### 2-1. 사용자 측 CLI

```
ctp.sh sql        [-c <sql.conf>]
ctp.sh medium     [-c <medium.conf>]    # sql 의 suite 변형 (medium/design.md)
ctp.sh kcc        [-c <kcc.conf>]
ctp.sh neis05     [-c <neis05.conf>]
ctp.sh neis08     [-c <neis08.conf>]
ctp.sh sql_by_cci [-c <sql_by_cci.conf>]  # CCI 모드, sql_interface_type=cci 자동
ctp.sh webconsole {start|stop}             # 별도 utility 분기
```

### 2-2. 내부 진입 — `run.sh` 셸 출

```
CTP.executeSQL(config, suite, interactiveMode, useCCI)
  └─ shell-out: sh ${CTP_HOME}/sql/bin/{run.sh|run_memory.sh} -s <suite> -f <conf>
       (useCCI=true 면 export sql_interface_type=cci 선행)
```

### 2-3. run.sh 의 3-way 분기 (do_test)

| 모드 | 진입 |
|------|------|
| Interactive | `source ${CTP_HOME}/sql/bin/interactive.sh; bash --posix` |
| CCI | `${CTP_HOME}/sql_by_cci/ccqt <port> <db> <alias> <resultFolder> <repo> ${CTP_HOME} <urlproperty>` |
| JDBC (default) | `java cqt.console.ConsoleAgent runCQT <type> <alias> <bits> <charset_xml> <repo>?db=<db>_qa[&filter=<excl>]` |

---

## 3. 사용자 요구사항 (M0 + 가이드 기반 추정)

권위 있는 가이드: `doc/sql_guide.md` (17.6KB), `doc/cci_guide.md` (51KB), `doc/cci_compatibility_guide.md`, `doc/jdbc_compatibility_guide.md`. (정밀 인용은 후속.)

추정 핵심 요구:

1. **대규모 SQL 회귀 검증** — 17,411 케이스 (+ medium 970 + sample 11) 의 자동 실행
2. **인터페이스 비교** — 같은 SQL을 JDBC vs CCI 양쪽에서 실행 후 결과 비교 가능 (`.answer` vs `.answer_cci`)
3. **charset/collation 매트릭스** — DB/Client charset 조합 정답 자동 선택
4. **빌드 라인 차이 흡수** — 새 빌드(예: 11.x) 에서만 다른 결과를 *케이스 단위 patch* 로 흡수
5. **쿼리 플랜 회귀 검증** — `.queryPlan` 별도 비교
6. **인터랙티브 디버깅** — 실패 케이스를 사용자가 손으로 재현 가능
7. **DB 자동 관리** — 케이스 작성자가 DB 셋업 신경 안 쓰고 SQL 만 작성하면 됨
8. **부수 검증** — memory leak, java SP, locale build, debug mode

---

## 4. 비기능 요구

| 항목 | 현재 동작 | 새 시스템에서의 의미 |
|------|----------|---------------------|
| 단일 호스트 실행 | run.sh 가 *로컬* 에서 cubrid 와 cqt 모두 운영 | 분산 실행이 *없다* — sql 은 단독 호스트 모델 |
| stdout/main.info 결과 채널 | run.sh 가 grep, ConsoleAgent 가 작성 | in-band signaling 동결 |
| .answer 매트릭스 매칭 | cqt 측이 charset_file 보고 적합 .answer 변형 선택 | 매칭 규칙 동결 (구체 알고리즘 후속 분석) |
| memory leak 변형 | run_memory.sh 별도 entry | 새 시스템도 분리 또는 옵션 통합 결정 |
| interactive 모드 | bash --posix + #SCRIPTCONT 우회 | ad-hoc 메커니즘 — 새 시스템에서 first-class REPL 권고 |
| core file 처리 | run.sh do_summary_and_clean 의 find $CUBRID + file 검사 | core 정책 ADR (모든 모듈 공통) |
| webconsole 분리 | utility 분기 (cli-tree.md) | 새 시스템에서도 분리 (또는 별도 모듈) |

---

## 5. 의존하는 외부 자원 (요약)

상세는 `io-contract.md`.

- **CUBRID 설치**: `$CUBRID` 가 가리키는 dir + `cubrid` 명령 PATH
- **JAVA_HOME**: cqt 실행 + javac (java SP 빌드)
- **cqt 자체 SSH/SFTP 스택**: `cqt.common.SSHConnect/SFTP/...` (shell.common 과 별개)
- **testcases 레포**: cubrid-testcases/sql (17,411 .sql), cubrid-testcases/medium (970), cubrid-testcases/sample
- **scenario_repo_root**: default `$HOME/dailyqa` (config로 override 가능)
- **`<CTP_HOME>/sql_by_cci/ccqt`**: CCI 모드 native worker
- **`<CTP_HOME>/bin/ini.sh`**: 셸용 INI 파서 alias
- **`$CUBRID/conf/cubrid_broker.conf`**: CCI 모드의 BROKER_PORT 추출
- **local.properties**: cqt 의 isdebug / qaview 토글
- **<charset_file>.xml** (default test_default.xml): JDBC 연결 + charset 메타

---

## 6. 새 시스템 설계 입력 (요약)

sql 모듈 대체 시 *반드시 충족*:

1. ✅ stdout 라인 마커 (`Result Root Dir:`, `Fail:`, `Success:`, `Total:`, `CORE_FILE:`) 호환
2. ✅ `<resultDir>/main.info` 파일 포맷 호환 (run.sh do_summary_and_clean 가 grep)
3. ✅ DB 셋업 흐름 (do_init / do_clean / do_configure / do_create_db / do_test / do_summary_and_clean)
4. ✅ `<repo>?db=<dbname>_qa[&filter=<excl>]` URL-style scenario specifier 파싱
5. ✅ `--+ pragma` 주석 지시어 (예: holdcas on)
6. ✅ `=========================================...` 구분자 정답 포맷
7. ✅ `.answer_<DB>_<C>[_collation]` charset 매트릭스 매칭
8. ✅ `.queryPlan` 별도 비교
9. ✅ `.<ver>_<S64|D>_patch` / `_excluded_list` 빌드 라인 메커니즘
10. ✅ medium 의 db_name=mdb / locale skip / make_db_data 분기 (medium/design.md)
11. ✅ `enable_memory_leak` → run_memory.sh 분기

ADR 후보:
- 셸 (run.sh) 와 Java (cqt) 의 책임 분리 모델 — 유지 / 통합
- in-band signaling → 명시적 IPC 승격
- CCI 모드 (sql_by_cci) 와 sql 모듈의 결합 — 1차 대체 시 함께 / 우회
- interactive 모드 → first-class REPL
- charset/collation 매트릭스 표현 정형화
- `.<ver>_*_patch` → 조건부 정답 시스템 정형화
