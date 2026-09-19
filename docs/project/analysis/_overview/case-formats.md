# Case Formats — testcases 레포 케이스 파일 포맷 분포

**Source repos:**
- `cubrid-testcases` (public — sql, medium, isolation, tool)
- `cubrid-testcases-private-ex` (private extension — shell, shell_heavy, shell_perf, scripts)
- `cubrid-testcases-private` (private — HA, interface, longcase, manually, random_query_generator, shell_ext)

또한 CTP 레포 안의 다음 디렉터리가 *케이스 실행 시 원격 환경에 배치되어야* 하는 자산:
- `cubrid-testtools/CTP/isolation/ctltool/` — isolation .ctl 케이스의 native 실행기 + runone.sh
- `cubrid-testtools/CTP/shell/init_path/` — shell .sh 케이스가 source 하는 init.sh 등 헬퍼

**Phase 0 M0 #5 status:** 완료

---

## 1. 핵심 발견 — 모듈별로 케이스 형식이 완전히 다르다

| 모듈 | 케이스 확장자 | 짝 | 패턴 |
|------|---------------|-----|------|
| sql | `.sql` | `.answer` | `cases/` + `answers/` 자매 디렉터리 |
| medium | `.sql` | `.answer` | sql과 동일 |
| isolation | `.ctl` (multi-client DSL) | `.answer` | 자매 디렉터리 없음, 같은 디렉터리 안 |
| shell | `.sh` (실 bash) | `.answer` | `cases/` + `answers/` (보조 .sql/.java/.c 동봉) |

**전 모듈 공통**: 결과 비교의 정답은 `.answer` 텍스트 파일. 단, 케이스 입력 형식은 4가지(SQL/SQL/concurrency-DSL/bash) — 새 시스템에서 *동일한 실행기로 모두 처리* 하기 어렵다는 뜻. 모듈 경계가 곧 *케이스 형식 경계*.

---

## 2. sql 모듈 — `.sql` + `.answer` (depth 4~5 분류 트리)

**규모:** 17,411 `.sql` + 17,428 `.answer` (≈ 1:1 페어, +17은 charset 변형)

**구조 예:**
```
cubrid-testcases/sql/
├── _02_user_authorization/
│   └── _02_authorization/
│       └── _001_grant/
│           ├── cases/CUBRID215.sql
│           └── answers/CUBRID215.answer
├── _24_aprium_qa/
│   └── _02_sql_extension/
│       └── issue_9688_ntile/
│           ├── cases/...
│           └── answers/...
├── _01_*, _02_*, ..., _24_* (38 sub-dirs at depth 1)
└── ...
```

**.sql 포맷 예:**
```sql
--+ holdcas on;
--use index granted from DBA

call login('dba','') on class db_user;
create user user1;
create class xoo ( a int, b int);
create index idx1 on xoo(a,b);
create reverse index idx2 on xoo(b,a);
...
```

특징:
- 표준 SQL + **CUBRID 확장 SQL** (object-oriented constructs, e.g., `create class`, `dba.xoo`, `db_root` pseudo-table)
- **`--+ pragma` 지시어**: `--+` 로 시작하는 주석은 sql 실행기에 대한 메타지시 (예: `holdcas on` — 케이스 간 connection 유지)
- 일반 `--` 주석은 케이스 설명 (영문/주석 다양)

**.answer 포맷:**
```
===================================================
    
null     

===================================================
0
===================================================
1
===================================================
```

- 표준 구분자: `===================================================` (51 = 기호)
- 각 블록은 *하나의 SQL 결과 출력* — 컬럼 정렬 공백 포함
- diff 비교 대상

### sql 의 변형 확장자 (ConsoleAgent/cqt가 인지)

| 확장자 | 갯수 | 의미 |
|--------|------|------|
| `.answer` | 17,428 | 기본 정답 |
| `.answer_cci` | 2,111 | CCI 인터페이스 모드 정답 (sql_by_cci 변형) |
| `.queryPlan` | 934 | 쿼리 플랜 검증용 — sql과 별도 비교 |
| `.diff_1` | 273 | 알려진 diff 패턴 (검증된 차이를 무시?) |
| `.answer_D_utf8_C_utf8_bin` | 86 | DB charset = utf8, Client charset = utf8_bin |
| `.answer_D_iso88591_C_iso88591_en_ci` | 80 | DB charset = iso88591, Client = en_ci collation |
| `.answer_D_iso88591_C_utf8_bin` | 71 | mixed |
| `.answer_win` | 57 | Windows 전용 정답 |
| `.todo` | 7 | 미완성 케이스 표시 |
| `.answer_ci` | 6 | case-insensitive collation 정답 |
| `.0_S64_patch` / `.0_S64_excluded_list` | 6 / 5 | 빌드 버전 S64 (≈ 11.0) 한정 패치/제외 |
| `.0_D_patch` / `.0_D_excluded_list` | 6 / 5 | 빌드 버전 D (≈ 다른 라인) 한정 |
| `.8_S64_patch` | 2 | 빌드 8.x_S64 한정 패치 |
| 기타 | (소수) | `.bk`, `.back`, `.log`, `.conf` 등 운영 부산물 |

**해석:**
- charset/collation 변형은 `<DB>_<C>` 명명 규칙으로 다중 정답을 보유. cqt가 conf의 db_charset / charset_file 를 읽어 적합한 .answer 변형 선택
- `.queryPlan` 은 별도 비교 대상 — *결과 정답*과 *플랜 정답* 두 차원 검증
- `.<version>_S64_patch` / `.<version>_D_patch` 시스템은 **빌드 라인별 정답 차이**를 케이스 단위로 흡수하는 메커니즘. 새 시스템에서는 *조건부 정답* 또는 *snapshot diff* 패턴으로 정형화 후보

---

## 3. medium 모듈 — sql과 동일 형식

**규모:** 970 `.sql` + 982 `.answer` (≈ 1:1, +12 변형)

**구조:**
```
cubrid-testcases/medium/
├── _04_full/{cases,answers}/
├── _06_fulltests/{cases,answers}/
├── _07_mc_dep/{cases,answers}/
├── _08_mc_ind/{cases,answers}/
├── config/
└── ...
```

11 sub-dirs at depth 1, 모두 `cases/` + `answers/` 자매 패턴. **sql 모듈과 케이스 형식이 동일** — 디렉터리만 분리.

variant: `.api` (10) — sql과 다른 인터페이스 호출 케이스로 추정. `.0_S64_patch` (3), `.0_D_patch` (3), `.answer_win` (2), `.gz` (1) — sql 변형의 부분 집합.

**확정**: medium = sql 의 suite 변형 (medium/design.md §1 의 결론과 일관, 케이스 차원에서도 한 번 더 검증).

---

## 4. isolation 모듈 — `.ctl` (concurrency DSL) + `.answer`

**규모:** 6,778 `.ctl` + 6,853 `.answer` (≈ 1:1, +75 = multi-stage answer 변형)

**구조 (depth 3 분류):**
```
cubrid-testcases/isolation/
├── _01_ReadCommitted/...
├── _02_RepeatableRead/{
│     foreign_key_column, primary_key_column, catalog,
│     dml_ddl, issues, serial, trigger, partition_table,
│     no_index_column, function
│   }
├── _03_*, _04_*, ...
├── _06_features/cbrd_22705_online_index_parallel/
└── config/
```

isolation은 **격리 수준(_01_ReadCommitted, _02_RepeatableRead, _Serializable, ...)** 을 1차 분류로, 그 아래 **시나리오 토픽**(foreign_key, dml_ddl, ...)을 2차로 두는 트리. sql/medium 처럼 cases/answers 자매 디렉터리는 *없다* — 같은 디렉터리 안에 .ctl 과 .answer 가 짝지어 둠.

### .ctl 포맷 — multi-client concurrency DSL

```
MC: setup NUM_CLIENTS = 4;

C1: set transaction lock timeout INFINITE;
C1: set transaction isolation level read committed;
C2: set transaction lock timeout INFINITE;
...

/* preparation */
C1: DROP TABLE IF EXISTS t1;
C1: create table t1 (a int, b int auto_increment, c char(10));
C1: insert into t1(b,c) values (1,'a'),(2,'b'),...;
C1: COMMIT;
MC: wait until C1 ready;

/* transaction mix */
C1: describe t1;
MC: wait until C1 ready;

C2: create index i on t1(b,c) with online parallel 2;
MC: wait until C2 blocked;
```

**DSL 문법:**
- `<actor>: <statement>;` — actor 가 SQL 또는 제어 명령을 실행
- `MC` (Master Controller) — 동기화/오케스트레이션 actor
- `C1`, `C2`, ..., `Cn` — 클라이언트 (concurrent transactions)
- `MC: setup NUM_CLIENTS = N;` — N개 클라이언트 풀 초기화
- `MC: wait until <Cn> ready;` — `<Cn>`이 직전 명령을 끝낼 때까지 대기
- `MC: wait until <Cn> blocked;` — `<Cn>`이 락 대기 상태에 진입할 때까지 대기 (concurrency 핵심)
- `/* ... */` 주석

**파서/실행기**: `cubrid-testtools/CTP/isolation/ctltool/` 가 **native C 구현** (parse.c/h, common.c/h, cubrid_drv.c, mysql_drv.c, oracle_drv.cpp). Makefile 으로 빌드 → 원격 환경에 배포되는 단일 실행 파일. **isolation 케이스 실행은 Java가 아니라 native binary가 한다** (isolation/design.md 의 §5-1 갱신 필요).

### `runone.sh` 의 위치 — 정정

isolation/design.md 의 §5-1 에서 *"runone.sh는 CTP repo에 없음"* 이라고 적었으나, **이는 오류**. 실제 위치:

```
/data/cub_sys/cubrid-testtools/CTP/isolation/ctltool/runone.sh
```

`runone.sh`는 ctltool 디렉터리 안에서 native binary와 함께 배포됨. Deploy 단계에서 ctltool 디렉터리 전체가 원격 env로 복사되며, isolation worker가 SSH로 `cd $ctlpath; sh runone.sh ...` 호출 시 이 파일을 실행.

→ isolation/design.md 의 외부 의존 표면 표 갱신 필요 (M0 후속 cleanup commit으로 처리).

### .answer 변형
- `.answer1` (58), `.answer2` (6) — multi-stage 케이스에서 단계별 정답
- `.answer_1` (3) — 동일하나 underscore 변형
- `.asnwer` (2), `.amswer` (1) — 명백한 오타 (data quality 노이즈, 새 시스템에서는 lint 대상)

---

## 5. shell 모듈 — `.sh` + `.answer` + 보조 자산 폭주

**규모 (cubrid-testcases-private-ex/shell):**
- 3,658 `.sh` (실 bash 케이스) + 3,090 `.answer`
- 1:1 매핑이 아님 — `.sh` 파일 중 일부는 hierarchical sub-script (다른 .sh 가 source 하는)

**보조 자산 (1 케이스가 여러 파일을 묶음):**

| 확장자 | 갯수 | 용도 |
|--------|------|------|
| .sh | 3,658 | 케이스 본체 (bash 실행) |
| .answer | 3,090 | 정답 (1:1 짝짓기 약 80%) |
| .sql | 2,179 | 케이스가 cubrid에 적재할 SQL 스니펫 |
| .java | 1,107 | Java stored procedure / 클라이언트 |
| .txt | 575 | 시드 데이터, 입력 파일 |
| .c | 279 | CCI native 클라이언트 (rebuild on test) |
| .exp | 235 | expect 스크립트 (TUI 자동화) |
| .gz | 138 | 압축 시드 데이터 |
| .cpp | 72 | C++ stored procedure |
| .log | 81 | (이전 실행 로그? 또는 reference) |
| .result | 100 | (CTP shell.Test의 collectGeneralResult 가 회수하는 형식) |
| .conf | 40 | 케이스별 추가 설정 |
| .answer_WIN / .answer_win | 52 / 39 | Windows 변형 (대소문자 일관성 깨짐) |

**케이스 예** (issue_10709_statistic_1.sh):
```bash
#!/bin/bash
#1 Verify example is ok
#2 When there are one sql include two or more index, the sel is right.
...

. $init_path/init.sh
init test
set -x

db_name=10709
cubrid service stop
cubrid server stop $db_name
cubrid deletedb  $db_name
rm -rf  $db_name

mkdir $db_name
cd $db_name
cubrid_createdb -r $db_name
...
```

**핵심 약속:**
- 케이스 시작 시 `. $init_path/init.sh` 로 helpers source — `$init_path` 는 deploy 단계에서 원격에 설정한 경로 (`CTP/shell/init_path/` 가 복사된 위치)
- `init test` — init.sh 가 정의한 함수 호출, 환경 셋업
- 케이스 본체: `cubrid` 명령 자유 사용, DB 생성/삭제, native 컴파일, expect 스크립트 실행 등 임의 셸 작업
- 출력 → CTP shell.Test 가 `<case>.result` 파일을 회수, `.answer` 와 비교

### `init_path` 자산 (CTP 안에 있고 deploy로 원격 복사됨)

```
CTP/shell/init_path/
├── init.sh                       ★ 모든 shell 케이스가 source
├── shell_utils.sh                보조 함수 라이브러리
├── ha_common.sh, make_ha*.sh     HA 셋업 헬퍼
├── HA.properties                 HA 설정
├── rqg_init.sh                   RQG 변형용
├── run_shell.sh                  (sched.jar 와 연계되는 진입점, deps-of-common 참조)
├── ccidb.sql                     CCI 테스트용 DB 시드
├── commonforjdbc.jar             JDBC 클라이언트 라이브러리 묶음
├── commonforjdbc_aix.jar         AIX 변형
├── commonforjdbc_src/            소스
├── commonforc/                   C 클라이언트 헬퍼
├── shell_config.xml              설정
├── recovery_ignore_item.conf     복구 시 무시할 항목
├── cubrid                        (실행 파일? 또는 wrapper)
├── *Regedit.bat, dropCubridParamOnRegedit.bat ... — Windows 레지스트리 헬퍼
```

**즉 shell 모듈의 외부 표면은 단순히 cubridqa-shell.jar 가 아니라**:
- shell.jar (Java 35 파일) +
- shell/init_path/* (셸 helper + jar + bat 20+ 자산) +
- testcases-private-ex/shell/* (bash 케이스 + 보조)

새 시스템 설계 시 init_path 의 helper 계약은 **케이스 작성자에게 가장 가깝게 노출된 표면** — 동결 의무가 강함.

---

## 5b. ha_repl / cdc_repl — shell-format 의 HA 변형 (private 레포)

**Source:** `cubrid-testcases-private/HA/`

```
HA/
└── shell/
    ├── _23_ha_enhancement
    ├── _25_features_844
    ├── _26_features_845
    ├── _28_features_930
    ├── _29_banana_qa
    ├── _38_fig
    ├── _39_fig_cake
    └── config
```

**규모:** 482 `.sh` + 328 `.answer` + 218 `.sql` + 117 `.java` + 18 `.exp` + 9 `.result`

**핵심 발견 — ha_repl/cdc_repl 케이스는 별도 형식이 아니다.** HA 디렉터리 아래에 *shell-format 그대로* 의 케이스가 들어있다(`HA/shell/...`). 즉 ha_repl/cdc_repl 모듈은 *shell 모듈의 실행 모델 + HA 토폴로지 셋업 헬퍼* 조합으로 동작.

이는 다음 결합 관계를 명확히 설명한다:
- **why** ha_repl/cdc_repl 모듈 (Java) 이 `shell.common.SSHConnect/LocalInvoker/...` 를 import 하는가 → *케이스 자체가 shell-format이라 shell 의 실행 인프라를 그대로 쓴다*
- **why** conf-matrix.md 에서 ha_repl/cdc_repl 이 *cluster B* (shell/process family) 인가 → 위와 동일

→ **strangler-fig 1차 대체 단위 확정 입력**:
- shell.common.* 추출이 1차 작업이면, ha_repl/cdc_repl 자동 동반 대체 가능
- 즉 *4개 모듈(shell + isolation + ha_repl + cdc_repl) 한 묶음* 이 자연스러운 첫 번째 작동 단위

---

## 5c. interface — JDBC/CCI/PHP/Perl 호환성 (private 레포)

**Source:** `cubrid-testcases-private/interface/`

**규모:** 483 `.c` + 394 `.java` + 365 `.sh` + 331 `.answer` + 45 `.cpp` + 42 `.sql`

**구조:**
```
interface/
├── CCI/
│   ├── performance_scenario
│   ├── open_issue_cases_blocked
│   └── shell
├── Perl/
└── PHP/
```

**확장자 특이점:**
- `.0` (129), `.3` (63), `.1` (59) — **빌드 라인 번호로 추정되는 정답 변형** (예: `case.answer.0`, `case.answer.1`, `case.answer.3` — 빌드 0/1/3 별 정답)
- `.output` (26) — stdout/stderr 캡처 결과로 추정
- 언어별 디렉터리 — 각 클라이언트 라이브러리(JDBC/CCI/PHP/Perl) 호환성 케이스

**해석:**
- 인벤토리 모듈 `jdbc`, `cci_compat`, `sql_by_cci` 가 이 레포의 케이스를 사용
- C/C++/Java/PHP/Perl 다언어 케이스 → *케이스 자체가 빌드 산출물*. 새 시스템에서 이 케이스들의 *빌드/링크 시점* 정책 ADR 필요
- `.0`/`.1`/`.3` 명명 시스템은 sql 모듈의 `.<ver>_S64_patch` 와 다른 mechanism — ADR 후보

---

## 5d. random_query_generator (RQG) — `.yy` 문법 + 셸 래퍼

**Source:** `cubrid-testcases-private/random_query_generator/`

**규모:** 104 `.sh` + 77 `.yy` + 45 `.zz` + 6 `.txt` + 5 `.java`

**핵심 형식 — MySQL/MariaDB RQG 표준 호환**:
- **`.yy`** — RQG 문법 파일 (yacc-like rule grammar). 랜덤 SQL 생성 규칙을 정의
- **`.zz`** — RQG 데이터 파일 (테스트 데이터 시드 정의)
- **`.sh`** — 케이스 진입 셸 스크립트 (RQG runner 호출)

→ RQG는 **외부 OSS RQG 프레임워크(MariaDB)와 호환되는 케이스 포맷**. CTP 측은 셸 래퍼로 호출하고 결과를 일반 shell-format 처럼 다룬다 (cli-tree.md: RQG 분기는 SHELL과 같은 메서드 + `TEST_CATEGORY=rqg` 시스템 프로퍼티).

→ 새 시스템에서 RQG 형식 자체는 외부 표준이므로 *그대로 동결*. 호출 래퍼만 새 시스템 동등물로 제공.

---

## 5e. shell_ext / longcase / manually (private 레포)

| suite | 핵심 자산 | 정체 |
|-------|-----------|------|
| `shell_ext` | 342 .sh + 286 .conf + 164 .java + 156 .sql + 127 .txt | shell 형식의 *configuration-heavy* 변형. **.conf 가 286개로 매우 많음** — 케이스마다 별도 설정. cli-tree.md 의 SHELL_EXT 가 enum에 없음 → CTP shell의 카테고리 기능을 통한 것으로 추정 (`shell_ext_guide.md` 가 doc/에 존재) |
| `longcase` | 185 .java + 131 .sh + 88 .jar + 55 .sql | shell-format + **사전 빌드된 88개 .jar** (장시간 부하 케이스, jar 컴파일을 케이스 작성자 측에서 수행). shell_heavy 와 유사 패턴 |
| `manually` | 27 .txt + 20 .sh | **수동 실행 가이드** (.txt 가 절차 문서, .sh 는 보조 스크립트). 자동화 대상이 아닌 *문서화된 인간 케이스* |

→ shell_ext / longcase 는 shell 모듈의 *suite 변형* (medium ↔ sql 관계와 동일).
→ manually 는 *비-자동화 자산* — 새 시스템에서는 별도 디렉터리(`docs/manual/`)로 분리 권고.

---

## 6. shell_heavy / shell_perf — shell 변형

shell_heavy:
- 253 `.java` + 142 `.sh` + 52 `.class` + 41 `.sql` + 52 `.answer`
- Java 위주 — JDBC 부하 시나리오에서 미리 컴파일된 .class 까지 케이스 일부로 보관

shell_perf:
- 60 `.sql` + 50 `.sh` + 23 `.java` + 12 `.answer` + 분할 .gz_aa/_ab/_ac (split archives)
- 성능 측정 시나리오, 대용량 시드 데이터를 split 하여 보관

→ 두 변형 모두 **shell.conf 위에서 동작하는 shell 변형 suite** 이며 같은 `.sh` + 보조 자산 모델. 새 시스템에서는 `shell` 모듈의 *suite 인스턴스* 로 표현 가능 (medium ↔ sql 관계와 유사).

---

## 7. 모듈별 *동결 표면* 요약 (외부 인터페이스 동결 명세 입력)

| 모듈 | 케이스 형식 | 보조 자산 약속 | 정답 형식 | 결과 채널 |
|------|-------------|----------------|-----------|-----------|
| sql / medium | `.sql` (CUBRID SQL + `--+` pragma) | `cases/` + `answers/` 자매 디렉터리 | `.answer` (= 구분자, 블록당 SQL 결과) | stdout 마커 + `<resultDir>/main.info` |
| isolation | `.ctl` (MC/C1..Cn DSL) | `ctltool/` (native C parser + runone.sh) deploy 됨 | `.answer` (+ `.answer1`/`.answer2` for multi-stage) | runone.sh stdout: `flag: OK`/`flag: NOK`/`found core file` |
| shell | `.sh` (bash) + 보조 (.sql/.java/.c/.exp/.txt/.gz/.cpp/.conf) | `init_path/init.sh` + 헬퍼 source 컨트랙트 | `.answer` + `.answer_win` (OS 변형) | `<case>.result` 파일 + Test.collectGeneralResult |
| ha_repl / cdc_repl | `.sh` (shell-format) — *별도 형식 없음* | shell 의 init_path/* + HA 토폴로지 헬퍼 | `.answer` | `<case>.result` (shell 동일) |
| jdbc / cci_compat / sql_by_cci | `.c` / `.java` / `.cpp` / `.sh` (interface/) | 빌드 산출물을 case가 보유 | `.answer` + `.0`/`.1`/`.3` (빌드 라인 변형) | shell 모델 또는 ccqt native worker stdout |
| RQG | `.yy` (문법) + `.zz` (데이터) + `.sh` (래퍼) | 외부 OSS RQG 프레임워크 호환 | `.answer` (래퍼가 비교) | shell 모델 + `TEST_CATEGORY=rqg` |
| shell_ext / longcase / shell_heavy / shell_perf | `.sh` + suite-별 보조 | shell 변형 | `.answer` | shell 모델 |
| webconsole | (테스트 없음, utility) | sql/webconsole + Jetty | — | 웹 UI |
| manually | `.txt` (절차 문서) + `.sh` (보조) | — (자동화 대상 아님) | — | 인간 검증 |

### 다중 .answer 변형 시스템 (sql/medium 한정)
- `.answer_<DB>_<C>[_<collation>]` — DB charset / Client charset / Collation 매트릭스
- `.answer_cci` — CCI 인터페이스 모드
- `.answer_win` — Windows
- `.<buildversion>_S64_patch` / `.<buildversion>_D_patch` — 빌드 라인 한정
- `.diff_1` — 알려진 diff (무시 정책 추정)

→ 새 시스템에서 *조건부 정답 매칭*은 first-class 기능으로 설계 필요.

---

## 8. 새 시스템 설계 시 권고 (case-format 관점)

1. **모듈별 케이스 형식이 다르므로 새 시스템도 *케이스 파서 plugin* 모델이 자연스럽다**. 단일 실행기로 4가지(SQL/SQL/concurrency-DSL/bash)를 다루려 하면 인터페이스가 깨끗하지 않음.

2. **isolation `.ctl` DSL 은 native C 파서에 종속**. 새 시스템에서 동일 DSL을 처리하려면 (a) C 파서를 그대로 사용 (작은 격리), (b) DSL 을 보존하면서 호스트 언어로 다시 구현 (큰 작업), (c) DSL 자체를 결정론적 동시성 테스트 프레임워크로 대체 (가장 큰 결정). ADR 필요.

3. **shell 의 `init_path/init.sh` 컨트랙트** 는 가장 표면적인 약속. 케이스 작성자 수만큼 의존이 있을 가능성. 새 시스템에서도 같은 함수 시그니처를 유지하는 호환 init.sh 를 제공해야 무중단 마이그레이션 가능.

4. **sql 의 다중 .answer 매트릭스**(charset / OS / build-version) 는 인기 있는 메커니즘이지만 *케이스 단위로 분산 관리* 되어 추적이 어려움. 새 시스템에서는 *케이스 메타데이터 + 정답 파일 명명 규칙* 을 명시화한 규약이 좋음.

5. **`.todo`, `.bk`, `.log`, `.asnwer` (오타)** 같은 운영 부산물은 새 시스템 진입 시 lint 단계에서 차단. 단, 기존 testcases 레포는 수정 불가 자산이므로 *lint 보고는 하되 fail 시키지 않는* 전략 필요.

6. **shell_heavy 의 컴파일된 .class 보관** 은 어색함 — 새 시스템 마이그레이션 기간 중 빌드 산출물을 케이스 트리에 두는 관행을 ADR 로 명시 정리.

7. **`runone.sh` 의 위치를 isolation/design.md 에 정정 commit** — `cubrid-testtools/CTP/isolation/ctltool/runone.sh` 가 정답.

---

## 9. 발견 사항으로 인한 isolation/design.md 정정 사항

isolation/design.md 의 §5-1 에 적은:
> "runone.sh — 원격 측 스크립트 — CTP repo에 없음. testcases 레포 또는 deploy 단계가 배치한다 (확정 필요)"

→ **정정**:
> "runone.sh — `cubrid-testtools/CTP/isolation/ctltool/runone.sh`. ctltool 디렉터리에 native C 파서(parse.c, cubrid_drv.c 등) + Makefile + runone.sh 가 함께 들어 있고, deploy 단계가 이 디렉터리 전체를 원격 env로 복사한다. 즉 isolation 케이스 실행은 Java가 아닌 ctltool의 native binary가 수행한다."

이 정정은 isolation 모듈의 strangler-fig 1차 대체 비용을 더 무겁게 만든다 — Java만 대체하면 안 되고, native 파서/드라이버까지 함께 다뤄야 한다 (또는 그대로 우회).

---

## ROADMAP 갱신

체크리스트의 `analysis/_overview/case-formats.md` 항목을 완료(`- [x]`)로 갱신.

**Phase 0 M0 전체 완료** — 5/5 ActionableSlice 모두 작성됨:
- ✅ M0 #1 cli-tree.md
- ✅ M0 #2 conf-matrix.md
- ✅ M0 #3 deps-of-common.md
- ✅ M0 #4 4개 deep 모듈 design.md
- ✅ M0 #5 case-formats.md (본 문서)

**Phase 0 다음 단계 권고:**
1. M0 발견 사항을 토대로 **ADR-001 (구현 언어), ADR-002 (빌드 도구), ADR-004 (1차 대체 모듈 선정)** 의 입력 문서 작성
2. 4개 deep 모듈의 나머지 stub 4개씩 (requirements / implementation-notes / io-contract / test-corpus) 채우기 — 또는 design.md 안에서 통합
3. inventory 5개 stub 채우기 (jdbc / sql_by_cci / ha_repl / cdc_repl / cci_compat) — 본 case-formats.md 의 §5b/§5c 가 이미 입력 자료
4. ✅ **cubrid-testcases-private 레포 접근 확보됨** (HA / interface / longcase / manually / random_query_generator / shell_ext)
