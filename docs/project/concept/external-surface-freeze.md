# External Surface Freeze — 축 T(테스트 실행) 동결 명세

- **Date:** 2026-09-02 (rev.2 — 축 분리 + 검토 반영)
- **Status:** Accepted (ADR-003 의 규범 본문)
- **Scope 결정:** **축 T(테스트 실행)** 표면만 동결. 축 O(QA 운영)는 `concept/migration-exclusions.md` 로 제외. 산출물 jar 이름과 Java API 는 비동결 (ADR-003)
- **Inputs:** `analysis/_overview/{cli-tree,conf-matrix,case-formats,deps-of-common,orphan-enums}.md`, `analysis/{shell,sql,isolation,medium,common}/{io-contract,design,requirements,implementation-notes}.md`, `analysis/isolation/ctl-grammar.md`, `analysis/sql/{cqt-deep-dive,db-setup-functions,test-corpus}.md`
- **Analysis baseline:** cubrid-testtools @ 86992c1b334d55800f2700d60f9809c2ceca268d
  ⚠️ 로컬 체크아웃은 `feature/ai_support @ 8fb0925` — baseline 과 다르다. Phase 2 진입 전 `ComponentEnum.java` / `Test.java` / `runone.sh` 스팟 체크 필요 (§11-9)

> ROADMAP Phase 1 Exit 조건 — *"새 시스템이 어떤 입력을 받아 어떤 출력을 내야 하는지가 기존 CTP 와 1:1 매핑된 표"* — 는 §10 이 충족한다.

---

## 0. 읽는 법

### 0-1. 축 판정이 먼저다

`migration-exclusions.md` §0 의 원칙에 따라, 모든 표면은 **먼저 축을 판정**한 뒤 등급을 받는다.

- **축 T** — 없으면 테스트 *결과*가 달라진다 → 본 문서에서 동결
- **축 O** — 없어도 테스트 결과는 같다 → `migration-exclusions.md` 로 제외

축은 패키지·파일·jar 로 나뉘지 않는다. **같은 CLI 옵션 목록 안에서도 갈린다** (§1-4 참조).

### 0-2. 동결 등급

| 등급 | 의미 | 검증 방법 |
|---|---|---|
| **F1 (strict)** | 바이트 단위 동일 | 신/구 출력 diff 가 0 |
| **F2 (semantic)** | 의미 동일. 순서·공백·부가 정보는 자유 | 필드 단위 비교 |
| **F3 (compat-input)** | 기존 입력을 수용해야 함. 내부 표현은 자유 | 오류 없이 수용되는지 |
| **NF** | 동결하지 않음 | — |
| **미결** | 등급을 아직 못 붙임 — §11 에 해소 시점 기재 | — |

### 명백한 버그는 축 T 에서 고친다 (2026-09-03)

동결은 *CTP 가 하던 대로* 를 지키기 위한 것이지 *CTP 가 실수한 대로* 를 지키기 위한 것이 아니다.
어떤 동작이 **명백한 버그** — 작성자가 명백히 의도하지 않았고, 그대로 두면 사람이 틀린 것을 믿게
되는 것 — 이면 **사용자에게 묻고 축 T 에서 고친다.** 축 B 로 미루지 않는다: 축 B 는 CTP 에 대응물이
없는 개선이고, 이건 대응물이 *잘못된* 경우다.

| | |
|---|---|
| **고친다** | 의도하지 않았음이 명백하고, 그대로 두면 틀린 결과를 믿게 된다 |
| **재현한다** | 보기 흉하지만 의도된 것이거나, 무해하거나, 고치면 더 나빠진다 |
| **판단이 갈리면** | 사용자에게 묻는다. 판정을 혼자 내리지 않는다 |

고친 것은 회귀 증거에서 *의도한 차이* 로 기록된다 (`evidence/spec-corrections.md`). 그러므로 이
예외는 드물어야 하고, 각 건마다 근거가 남아야 한다 — 그러지 않으면 "정규화 후 diff 0" 이 다시
변론이 필요한 산문이 된다.

**결정 기록:**

| 대상 | 결정 | 근거 |
|---|---|---|
| `checkCommand` 가 없는 명령을 PASS | **고침** | csh 문구(`no <cmd> in ...`)로 판정 → bash 기계에선 항상 PASS. `check_local.log` 가 `dos2unix ...... PASS` 라 해놓고 그 실행이 `command not found` 로 4번 깨졌다 (`evidence/regression-shell.md`) |
| `run_shell.sh` 가 실패해도 0 반환 | **고침** | `System.exit(0)` 무조건. `QUIT(NOK)` 를 찍고도 0. `run_shell.sh ... && ...` 가 통과한다 |
| 빌드 id 가 버전을 넘어감 | **고침** | 커밋 접미사가 없으면 다음 `)` 까지 집어서 `11.2.0.0000) (64bit release build for linux_gnu` 가 build id 가 된다. `main_snapshot.properties` 와 모든 케이스 환경에 들어간다 |
| `dos2unix` 필수 명령 | **뺌** | Windows 용이고 Windows 는 범위 밖. shell 코퍼스의 `.answer` 51개 중 CRLF 를 가진 것 0개 |
| JVM sweep 구문 오류 | **고치되 격리와 함께** | 버그는 맞지만 단독으로 고치면 파괴 범위가 넓어진다. **B-T2(namespace 격리)의 범위 안에서** 고친다 — §11-23, `beyond-axis.md` |
| `TEST_BUIILD_ID` 오타 | 재현 | 무해하고 읽는 곳이 없다 |

---

**기본값: 의심스러우면 F1.** 단 이 보수성은 *확인하지 않은 것*에도 적용되어야 한다 — 등급을 못 붙이면 **NF 가 아니라 "미결"** 이고 §11 에 등록한다.

---

## 1. CLI 표면

### 1-1. `ctp.sh` 문법 — **F1**

```
ctp.sh <task>... [-c <conf>] [--interactive] [-h] [-v]
```

| Short | Long | Arg | 의미 | 등급 |
|---|---|---|---|---|
| `-c` | `--config` | yes | 설정 파일 경로 | F1 |
| | `--interactive` | no | 대화형 실행 (SQL 계열만 유효, 그 외 무시) | F1 |
| `-h` | `--help` | no | 사용법 출력 | F1 |
| `-v` | `--version` | no | 버전 출력 | F1 |

- 위치 인자는 모두 task 이름, **다중 나열 가능** (`ctp.sh sql medium shell`) — 선언 순서대로 순차 실행. F1
- task 이름 **대소문자 무관** (`valueOf(toUpperCase())`). F1
- 인식 불가 task 는 **헬프 출력 후 해당 task 만 skip**. F1
- `-c` 미지정 시 `$CTP_HOME/conf/<suite>.conf` fallback. F1

### 1-2. task 이름 — 14 active **F1**

`sql` `medium` `kcc` `neis05` `neis08` `sql_by_cci` `shell` `rqg` `unittest` `isolation` `ha_repl` `cdc_repl` `jdbc` `webconsole`

`webconsole` 만 두 번째 위치 인자를 받는다 — `ctp.sh webconsole {start|stop}`.

**orphan 7** (`cci` `dots` `nbd` `sysbench` `tpcc` `tpcw` `ycsb`) — 현재 switch 분기 없는 silent no-op. **등급: 미결.** 폐기 정책은 **ADR-005 (미결)** 이며, `orphan-enums.md` §치는 *grace period 를 둔 단계적 제거*를 권고한다. 무단으로 hard error 로 바꾸지 않는다.

### 1-3. 그 밖의 축 T 진입점 — `cli-tree.md` 부록 A

`ctp.sh` 는 **6개 축 T 진입점 중 하나일 뿐이다.**

| # | 진입 | main | 등급 |
|---|---|---|---|
| T1 | `bin/ctp.sh` | `ctp.CTP` | F1 (§1-1) |
| T2 | `sql/bin/run.sh -s <suite> -f <conf>` | `cqt.console.ConsoleAgent runCQT` | F1 |
| T3 | `sql/bin/interactive.sh` | 동상 (interactive 변수) | F1 |
| T4 | `jdbc/bin/run.sh <conf>` | **`shell.main.JdbcLocalTest`** | F1 |
| T5 | `shell/init_path/run_shell.sh` | **`shell.main.RunShellMain`** | §1-4 |
| T6 | `bin/ini.sh` | `ctp.IniCommand` (conf 편집 CLI) | 미결 (§11-10) |

⚠️ **T4 가 `cubridqa-shell.jar` 의 클래스를 부른다** — `unittest`(`GeneralLocalTest`)에 이어 shell.jar 를 공유하는 두 번째 task. ADR-004 Phase 3 범위에 `jdbc` 가 딸려 온다.

### 1-4. `run_shell.sh` (T5) — 축이 섞인 CLI

`RunShellMain` 의 옵션 13개는 **두 축에 걸쳐 있다**. 축 O 5개는 제외한다 (`migration-exclusions.md` §1-2·§1-3·§4).

| 옵션 | arg | 축 | 등급 |
|---|---|---|---|
| `--loop` | no | T | F1 |
| `--maxloop` | yes | T | F1 |
| `--maxtime` | yes | T | F1 |
| `--update-build` | no | ~~T~~ **O** | **제외 (2026-09-03)** — 루프 사이에 새 빌드를 설치한다. §1-4a 가 빌드 설치를 제외했으므로 그 결정을 여기까지 끌고 온다 |
| `--next-build-url` | yes | ~~T~~ **O** | **제외 (2026-09-03)** — 빌드 서버 HTML 을 긁어 다음 빌드를 찾는다. `RunShellMain` 의 메서드 9개가 여기 딸려 있다 |
| `--extend-script` | yes | T | F1 |
| `--prompt-continue` | yes | T | F1 |
| `-h` / `--help` | no | T | F1 |
| `--enable-report` | no | **O** | **제외** |
| `--report-cron` | yes | **O** | **제외** |
| `--mailto` | yes | **O** | **제외** |
| `--mailcc` | yes | **O** | **제외** |
| `--issue` | yes | **O** | **제외** |

`--report-cron` 제외로 **`cubridqa-scheduler.jar` 의존이 끊긴다** — Phase 3 범위 축소.
문서화 위치: `doc/rqg_guide.md` §2.7 · `doc/cci_guide.md` · `doc/shell_heavy_guide.md`.

**2026-09-03 정정 — 축 T 는 8개가 아니라 6개다.** `loop` · `maxloop` · `maxtime` ·
`extend-script` · `prompt-continue` · `help`. 위 표에서 두 개가 축 O 로 옮겨갔고, 그와 함께
`RunShellMain` 의 절반이 범위 밖이 된다 — 메일/리포트 8개 메서드 + `CheckThread`(메일 상태 신호),
그리고 빌드 탐색·설치 9개 메서드(HTML 스크래핑 포함).

**같은 날 발견한 세 가지 (§11-20~22):**

| | |
|---|---|
| **`--config` 는 14번째 옵션이고 죽어 있다** | `options.addOption` 이 **주석 처리**되어 있는데 `cmd.getOptionValue("config")` 는 그대로 호출된다. 등록 안 된 옵션이라 항상 `null` 이고, 파라미터 배너는 매 실행 `config : null` 을 찍는다 |
| **`STOP` 파일이 제어 표면이다** | 케이스 디렉터리에 `STOP` 이라는 파일이 있으면 루프가 그 회차 끝에 멈춘다. 명세에 없다. 도는 루프를 밖에서 세우는 유일한 수단 |
| **종료 코드가 항상 0 이다** | `run()` 끝의 `System.exit(0)` 은 무조건이다. `QUIT(NOK)` 를 찍고도 0 을 반환한다 |

`run_shell.sh` 자체는 얇은 런처다 (`java -cp ... RunShellMain "$@"`). CTP 안에서 이걸 호출하며
`$?` 를 보는 곳은 없다 — 사람이 직접 쓰는 도구다. ⚠️ `common/ext/run_shell.sh` 는 **이름만 같은
다른 파일**이다 (스위트 러너 함수 3개). 혼동 주의.

### 1-5. `CTP_HOME` — **F3**

환경변수(미설정 시 실행 파일의 부모)를 입력으로 수용. 내부 경로 해석은 NF.

---

## 2. conf 표면

### 2-1. 파일명 규약 — **F1**

`$CTP_HOME/conf/` 의 **13개 배포 템플릿**:
```
sql.conf  sql_by_cci.conf  medium.conf  medium_dev.conf  sample.conf  jdbc.conf
shell.conf  shell_ci.conf  ha_shell.conf  isolation.conf
ha_repl.conf  cdc_repl.conf  webconsole.conf
```

⚠️ **이것이 `conf/` 디렉터리의 전부라는 뜻은 아니다.** 런타임 conf 가 추가로 있다:

| 파일 | 용도 | 등급 |
|---|---|---|
| ~~`conf/shell_agent.conf`~~ | RMI 에이전트용 | **등급 대상 아님 (2026-09-02)** — 이 파일은 **배포되지 않는다**. `Server.java:40` 이 `../conf/shell_agent.conf` 를 상대경로로 읽지만 13개 배포 conf 에 없다. 배포되지 않는 파일은 외부 표면이 아니다. RMI 모드 폐기(§7-7)와 함께 소멸 |
| `sql/` 작업 디렉터리의 `local.properties` | cqt 의 `isdebug` / `qaview` | NF — 메인 conf 로 통합 권고 |

### 2-2. 키 스키마 — 81 고유 키 **F3**

정본은 `conf-matrix.md`. 클러스터: A(SQL/data) sql 25 · sql_by_cci 23 · medium 21 · medium_dev 22 · sample 11 · jdbc 6 / B(shell/process) shell 26 · shell_ci 42 · ha_shell 28 · isolation 27 / C(replication) ha_repl 21 · cdc_repl 21 / D webconsole 2.

A↔B 교집합은 `scenario` **1개뿐** — 전역 설정 영역은 사실상 없다.

### 2-3. dot-notation 와일드카드 — **F1**

```
default.<role>.<property>          → 모든 instance 의 <role> 에 적용
env.instance<N>.<role>.<property>  → instance<N> 오버라이드 (default 를 이김)
```
`<role>` ∈ `ssh` `cubrid` `broker1` `broker2` `brokercommon` `ha` `cm` `master` `slave`.
`<property>` 는 **열거되지 않은 임의 키** — `default.cubrid.async_commit=on` → 원격 `cubrid.conf` 의 `async_commit=on` 으로 전사. 이 전사 규칙이 F1 이다.

### 2-4. CI 오버레이 — **F3**

`shell_ci.conf` 는 `shell.conf` 42−26 = **16 키**가 더 많다. `conf-matrix.md` §1-3 은 그중 **14개**만 열거했다 (§11-11). 전부 F3 로 수용한다.

**2026-09-03 해소: 16 이 맞다.** 파일에서 직접 세면 shell.conf 는 live 3 키, shell_ci.conf 는 live 19 키이고 차집합이 16 이다. 누락된 둘은 `testcase_exclude_from_file` 과 `test_category`. 둘 다 이미 구현되어 있으므로 범위 산정에는 영향이 없다.

---

## 3. 케이스 입력 컨트랙트 (NG1 동결 — testcases 레포는 수정 불가)

### 3-1. 디렉터리 규약 — 모듈마다 다르다 **F1**

⚠️ **단일 규약이 아니다.**

| 모듈 | 레이아웃 | 근거 |
|---|---|---|
| sql / medium | `<...>/cases/<name>.<ext>` + `<...>/answers/<name>.answer` — **자매 디렉터리** | `case-formats.md` §2·§3 |
| **shell** | `<name>/cases/<name>.sh` — 스크립트 이름 = **두 단계 위 디렉터리 이름**. `answers/` 는 케이스가 쓰는 관례일 뿐 **러너는 읽지 않는다** (2026-09-02 정정) | `Dispatch.getAllTestCaseScripts` · `evidence/spec-corrections.md` |
| **isolation** | `_NN_<isolation_level>/<topic>/<...>/` 안에 `.ctl` 과 `.answer` 가 **같은 디렉터리에 co-located**. `cases/`·`answers/` 자매 디렉터리 **없음** | `case-formats.md`:22,152 · `isolation/io-contract.md` §3-1 |

`cases/` 세그먼트 필수 규칙은 **shell 모듈의 `Test.java` 한정**이다 (`lastIndexOf("cases")` 로 경로 분리 — `shell/io-contract.md` §3-1). isolation 에 적용하면 안 된다.

**2026-09-02 정정 — shell 의 케이스 판별.** 위 표의 shell 행은 원래 "`cases/` 아래 아무 `*.sh`" 였고,
그건 CTP 의 규칙이 아니다. `Dispatch` 는 `find` 결과를 awk 로 거른다:

```
awk -F "/" '{ if( $(NF-2)".sh"== $NF) print }'
```

즉 **파일 이름이 두 단계 위 디렉터리 이름 + `.sh`** 여야 하고, `cases` 라는 세그먼트는 검사조차
하지 않는다. 이유는 `init_path/shell_utils.sh` 의 `do_check_more_errors` 가 결과 파일 이름을
**디렉터리 이름**에서 만드는데(`${case_name}.result`) 러너는 **스크립트 이름**에서 만들기 때문이다 —
둘이 같을 때만 일치한다. 이 규칙 때문에 `cases/` 안의 `PrintInfo.sh`·`common.sh` 같은 270개
헬퍼가 케이스가 아니며, 코퍼스 크기도 3,722 이 아니라 **3,452** 다 (ADR-013 재계수).

### 3-2. answer variant — **F1**

```
<name>.answer                          기본
<name>.answer_cci                      CCI 인터페이스
<name>.answer_win                      Windows
<name>.answer_WIN                      Windows (대문자 변형 — 52 파일)
<name>.answer_<DB>_<C>[_<COLL>]        charset/collation
                                       예: .answer_D_utf8_C_utf8_bin
<name>.queryPlan                       query plan 검증
<name>.<ver>_S64_patch / _D_patch      버전·비트 패치
<name>.<...>_excluded_list             제외 목록
<name>.diff_1                          알려진 diff 패턴 (273 파일)
<name>.answer1 / .answer2              isolation multi-stage
<name>.answer_1                        isolation 언더스코어 변형 (3 파일)
```

- `.answer_win` (39) 과 `.answer_WIN` (52) 의 **대소문자 비일관은 보존한다** — NG2 의 "비일관 대소문자를 고치지 않는다" 의 직접 사례.
- `.diff_1` 이 *입력*인지 *산출물*인지는 미확정 (§11-12).
- **variant 선택 알고리즘**: 1순위 `<answer>_<runMode>`, 2순위 `_<runModeSecondary>`, fallback base (`cqt-deep-dive.md` §). **알고리즘 자체는 F1**이나 `runMode` 의 *값 출처*가 미확인 (§11-7).

### 3-3. 케이스 안의 지시어 — **F1**

**`.sql` (sql 계열)**

| 지시어 | 상태 |
|---|---|
| `--+ holdcas on;` | 확인됨 |
| `--@queryplan` | 확인됨 (`.queryPlan` 파일명과 대소문자 비대칭 — 보존) |
| `--@<connId>` | **추정** — `cqt-deep-dive.md` 가 3곳에서 추정으로 표기. 확인은 §11-3 |

전수 목록 미확정 (§11-3).

**`.ctl` (isolation) — 8 토큰, `qactl.c` 하드코딩** (`ctl-grammar.md` §4)

| 토큰 |
|---|
| `<actor>: <statement>;` (actor ∈ MC, C1…Cn) |
| `MC: setup NUM_CLIENTS = N;` |
| `MC: wait until <Cn> ready;` |
| `MC: wait until <Cn> blocked;` |
| `MC: wait until <Cn> unblocked;` |
| `MC: wait until <Cn> finished;` |
| `MC: sleep <ms>;` |
| `MC: pause for deadlock resolution;` |
| 주석 `/* ... */` |

grammar 정형화는 ADR-008 (미결, §11-13).

**`.sh` (shell 계열)** — prologue `. $init_path/init.sh` → `init test` → `set -x` (§7-2).

### 3-4. `.answer` 내부 블록 형식 — **F1**

`=` 51개 구분선으로 케이스 블록을 나눈다 (`case-formats.md` §2). `MODE_MAKE_ANSWER`(정답 생성 모드)에서 새 시스템이 이 형식을 *생성*해야 한다.

### 3-5. exclusion 파일 — **F1**

`testcase_exclude_from_file` 이 가리키는 파일: 한 줄당 한 케이스 prefix/substring. 첫 글자 `#` 또는 `--` 는 주석. excluded 는 `dispatch_tc_ALL.txt` 에 불포함, `tempSkipped` 로 분리 집계.

### 3-6. URL-style scenario specifier — **F1**

```
<scenario_repo_root>/<scenario_path>?db=<db_name>_qa[&filter=<exclude_file>]
```
예: `/home/user/dailyqa/sql/_02_user_authorization?db=basic_qa&filter=exclusions.txt`

`ConsoleAgent.runTest` 의 `files[]` 인자 형식 (`sql/io-contract.md` §4-1). `cqt-deep-dive.md` 가 "보존해야 할 외부 표면" 목록의 **첫 항목**으로 든다.

---

## 4. stdout 마커 — 전부 **F1**

### 4-1. sql 계열

```
Result Root Dir:<path>
total:<N>
success:<N>
fail:<N>
SiteRunTimes:<N>
totalTime:<ms>
TOTAL_COUNT:<N>
TOTAL_ELAPSE_TIME:<ms>
CORE_FILE:<path>
-----------------------
Fail:<N>
Success:<N>
Total:<N>
Elapse Time:<sec>
Test Log:<path>
Test Result Directory:<path>
-----------------------
Testing End!
```

### 4-1a. 모든 task 를 감싸는 dispatcher 배너 — **F1**

> **Added 2026-09-02, found by running the first old-vs-new comparison.** The spec had no
> dispatcher output at all. Every task is bracketed by `CTP.java`'s own lines, whichever module
> ends up running.

```
(blank line)
====================================== <TASK IN CAPITALS> ==========================================
[<TASK IN CAPITALS>] TEST STARTED (Wed Sep 02 21:03:13 KST 2026)
(blank line)
   ... the task's own output ...
[<TASK IN CAPITALS>] TEST END (Wed Sep 02 21:05:44 KST 2026)
[<TASK IN CAPITALS>] ELAPSE TIME: <n> seconds
```

- The opening rule is printed **before** the task name is resolved, so an unrecognised name gets a
  banner and then help (`CTP.java:132-137`).
- The date is `java.util.Date.toString()` — `EEE MMM dd HH:mm:ss zzz yyyy`.
- Elapsed is **whole seconds, truncated**: CTP divides milliseconds by `1000.0` and casts to `long`.
- `webconsole` returns before the task loop, so it gets none of this.

### 4-1b. unittest 의 출력 — **F1**

> **Added 2026-09-02.** The spec described unittest's *plug-in contract* but never its output, which
> resembles nothing else in the system.

```
=> Init Step: 
<init's own output>
(blank)
=> List Step: 
Test Category:<category>
The Number of Test Cases: <n> (macro skipped: <a>, bug skipped: <b>)
=> Execute Step: 
[TESTCASE-1] <case> [SUCC]
[TESTCASE-2] <case> [FAIL]
=> Finish Step: 
<finish's own output>
(blank)
```

- Each heading carries a **trailing space** after the colon.
- The index is **one-based**, and the verdict is `[SUCC]` / `[FAIL]` — not `[OK]` / `[NOK]`.
- The verdict lands on the **same line** as the case name (`print`, then `println`).
- An empty list prints `[ERROR] Not found any test cases.` and the run **ends normally**: CTP returns
  from `start()` and the exit code stays 0.
- The two `Test Category:` / `The Number of Test Cases:` lines come from **`FeedbackFile`**, which
  writes them to its own file *and* to standard output. They are console surface even though a
  feedback backend produces them (`FeedbackFile.java:134-137`).

### 4-2. shell / isolation 계열

```
[ENV START] <envId>
[TESTCASE] <tc> EnvId=<envId> [OK]
[TESTCASE] <tc> EnvId=<envId> [NOK]
[TESTCASE] <tc> EnvId=<envId> [NOK], TRY-><N>
[ENV STOP] <envId>
```

> **Corrected 2026-09-02, while implementing `internal/result`.** The draft above read
> `, retry: <N>` and said the suffix appears "only when a retry happened". Both were wrong,
> and this is an F1 marker, so a consumer would have broken on either.
>
> - The literal is `TRY->`, from `Constants.RETRY_FLAG` in the shell module — not `retry: `.
> - The suffix hangs on `maxRetryCount != 0`, that is on retries being **configured**, not on a
>   retry having occurred (`shell/main/Test.java:189`). A first-attempt failure therefore prints
>   `, TRY->0` as soon as `testcase_retry_num` is set.
> - It never appears on `[OK]`, and never in the isolation module at all
>   (`isolation/Test.java:135` has no suffix).

### 4-3. 원격 `runone.sh` 마커 (isolation)

```
flag: OK
flag: NOK
found core file
found fatal error
```
판정 규칙 — **마지막 `flag: OK` 와 마지막 `flag: NOK` 의 위치 비교**(`Test.extractItems`). "마지막 것이 이긴다" 규칙 자체가 F1.

---

## 5. 출력 파일

### 5-1. 결과 디렉터리 레이아웃 — **F1**

```
<CTP_HOME>/result/<category>/current_runtime_logs/     ← currentLogDir
├── main_snapshot.properties        (conf 스냅샷 + AUTO_BUILD_ID / AUTO_BUILD_BITS)
├── dispatch_tc_ALL.txt             (전체 케이스 절대경로, 한 줄당 하나 — continue mode 입력)
├── dispatch_tc_FIN_<envId>.txt     (env 별 완료 — ALL 과의 차집합이 재개 대상)
└── test_<envId>.log                (워커 로그)
```

> **Corrected 2026-09-02, while implementing `internal/result`.** Two errors here.
>
> - **There is no timestamp in the path.** `Context.java:172-173` builds it as
>   `getToolHome() + "/result/" + category + "/current_runtime_logs"` — a fixed directory name.
>   `jdbc/bin/run.sh:66` spells out the same literal. The draft said `result/<task>/<timestamp>/`,
>   and ADR-013 consequently listed a timestamp among the values to mask during regression
>   comparison. There is nothing there to mask.
> - **`main.info` is not part of this layout.** It belongs to the sql/cqt side
>   (`cqt/console/util/TestUtil.java:108`, `TOTAL_SUMMARY_FILE = "/main.info"`), and the shell
>   family never writes one. It stays frozen — for the sql runner, in §5-2.

### 5-2. `main.info` — **F1** (`:` 구분)

```
total:<N>
success:<N>
fail:<N>
totalTime:<ms>
SiteRunTimes:<...>
cubrid_rel:<...>
user:<...>
machine:<...>
```

> **정정 (2026-09-11).** 위 키 목록은 실제 `main.info` 가 아니다. 실제는 CQT 가 쓰는 12키(`build`,
> `version`, `os`, `category`, `elapse_time`, `success`, `fail`, `total`, `execute_case`, `totalTime`,
> `end_time`, `result_path`) 뒤에 `do_summary_and_clean` 이 붙이는 3키(`cubrid_rel`, `user`, `machine` —
> `run.sh:853-855`). `SiteRunTimes` 는 CQT 의 `summary_info` 에 있다. `evidence/sql-baseline.md` §8.

### 5-3. `summary_info` — **F1** (`=` 구분, CCI 모드 또는 core 발견 시)

> **정정 (2026-09-11).** 아래는 **CCI 모드에서 `run.sh` 가 쓰는** `summary_info` 다 (`generate_summary_info`,
> `run.sh:804-850`). JDBC 모드 run 에는 **이름이 같은 다른 파일**이 있다 — CQT 가 루트와 디렉터리마다 쓰는
> `key:value` 형식(`total`, `success`, `fail`, `totalTime`, `SiteRunTimes`). 이 문서는 그것을 싣지 않았다.

```
cubrid_build_id=<ver>      execute_date=<...>       Num_total=<N>
Num_test_total=<N>         Num_success=<N>          Num_fail=<N>
Test_cat=<alias>           Test_upcat=function      OS=<os>
Bit=<32|64>bits            Elapse_time=<sec>        test_error=Y   ← core 시에만
```

`main.info` 는 `:`, `summary_info` 는 `=` — **이 비대칭은 의도적으로 보존한다** (NG2).

### 5-4. 워커 로그의 diff 블록 — **F1** (isolation)

`test_<envId>.log` 안에 실패 케이스마다:
```
=================================================================== D I F F ===================================================================
<diff -a -y -W 185 의 출력>
```
구분선 폭과 `diff` 옵션(`-a -y -W 185`)까지 F1. *stdout 이 아니라 워커 로그 파일*이다.

### 5-5. 백업 아카이브 — **F2**

| 모듈 | 파일명 | 내용 | OS 조건 |
|---|---|---|---|
| isolation | `isolation_result_<buildId>_<bits>_<taskId>_<ts>.tar.gz` | currentLogDir 통째 | **Linux/macOS 한정, Windows skip** (`isolation/design.md`) |
| shell | `*_fail_backup_package*.tar.gz` | 실패 케이스만 | **미확인** — shell 소스에 OS 조건 기술 없음 (§11-14) |

### 5-6. 로그 파일 — **F2**

`${CTP_HOME}/sql/log/cqt.log`, `test_<envId>.log`. 경로는 F1, 라인 포맷은 F2(§4 마커 포함 제약만).

---

## 6. 종료 코드 / 실행 상태

### 6-1. 종료 코드

| task 군 | 코드 | 조건 | 등급 |
|---|---|---|---|
| 전체 | `0` | task 완료. **케이스 실패가 있어도 0** | F1 |
| sql 계열 | `1` | `$CUBRID` 없음 또는 conf 부재 | F1 |
| shell / isolation | `-1` (셸에서 **255**) | 환경 점검 / build URL / scenario 부재 | F1 |
| ha_repl / cdc_repl | `-1` 추정 | shell·isolation 로부터 **유추** — io-contract 없음 (§11-8) | 미결 |
| ctp.sh launcher | `1` | `JAVA_HOME` 미설정 | **F2** — Go 에는 JAVA_HOME 검사가 없다. *실행 전 환경 오류는 1* 로 매핑 (§10 row 1) |

Go 구현은 `-1` 을 `os.Exit(255)` 로 명시한다.

### 6-2. Feedback 이벤트 — **F2** (축 T)

```java
onTaskStartEvent(buildUrl) / onTaskContinueEvent() / onTaskStopEvent()
setTotalTestCase(total, macroSkipped, tempSkipped)
onTestCaseStartEvent(tc, envIdentify)
onTestCaseStopEvent(tc, success, elapseMs, resultText, [lastPassResultCont,] envIdentify,
                    isTimeOut, hasCore, skipType [, retryCount])
onTestCaseStopEventForRetry(...)   ← shell 한정
onStopEnvEvent(envId)
```

- `skipType` 상수 **`SKIP_TYPE_NO` / `SKIP_TYPE_BY_MACRO` / `SKIP_TYPE_BY_TEMP`** 보존 — F1.
- shell 은 `lastPassResultCont` + `retryCount` 를 추가로 갖는다 (isolation 과 다름).
- **백엔드 중 `FeedbackDB` 는 축 O — 제외** (`migration-exclusions.md` §1-5). `Null` / `File` 은 축 T.

---

## 7. 원격 실행 컨트랙트 — 전부 **F1**

### 7-1. `runone.sh` 호출 시그니처 (isolation)

```
sh runone.sh [-n] -r <retry+1> <tc> <timeout_sec> <db_name> 2>&1
```
`-n` = `backup_core_file_yn=false`. `<tc>` 가 `/` 로 시작 안 하면 `$HOME/<tc>` prefix.

### 7-2. `init_path/init.sh` 케이스 prologue (shell)

```bash
. $init_path/init.sh
init test
set -x
```
`$init_path` 를 deploy 가 셋업한다는 사실 자체가 계약.

### 7-3. UNITTEST plug-in 4 함수 (`shell/local/<TEST_TYPE>.sh`)

```bash
init()      # 환경 셋업
list()      # 케이스를 한 줄당 하나씩 stdout
execute()   # $1 = testcase, 결과를 IS_SUCC=true|false
finish()    # 정리
```

> **Expanded 2026-09-02, while implementing `internal/runner/shellsuite`.** The draft named one
> marker; the protocol has three, and it also constrains the shell.
>
> **How the plug-in is driven** (`GeneralLocalTest.invoke`). Each call builds one script:
>
> ```
> cd ${CTP_HOME}; source shell/local/<TEST_TYPE>.sh; <command>
> ; echo GPROPSTART
> echo G_PROPERTY_<K>=${<K>}EEOOKK      # one line per variable wanted back
> ```
>
> `${CTP_HOME}` is left for the shell to expand — CTP does not substitute it here.
> Sourcing is what lets the four entry points be *functions* rather than four scripts, and it is
> why `execute` can see what `init` exported.
>
> **Three frozen literals, not one:** `GPROPSTART` ends the plug-in's own output; each value is
> what sits between `G_PROPERTY_<K>=` and the next `EEOOKK`, trimmed.
>
> **The verdict is a variable, not an exit code.** `execute` sets `IS_SUCC`; a failing case leaves
> the shell exit status untouched. A plug-in that never sets it has *not* passed.
>
> **The contract requires a bash-compatible shell.** `shell/local/unittest.sh`, the only plug-in
> CTP ships, declares `function init { ... }` — bash/ksh syntax that dash rejects — and the
> protocol uses `source`, which is likewise not POSIX. CTP runs the script with `sh <tmpfile>`
> (`common/LocalInvoker.exec`), so this has only ever worked because its machines have
> `/bin/sh` → bash.
>
> **Recorded deviation:** testkit says `bash <tmpfile>` rather than `sh <tmpfile>`. It reproduces
> what happens wherever CTP works, and additionally works where `/bin/sh` is dash — there CTP fails
> with `source: not found` and `function: not found`. No frozen output changes. A test asserts the
> shipped `function` syntax runs, so a change back to `sh` fails in CI rather than only on Debian.

### 7-3a. 원격 실행의 프레이밍 — **F1**

> **Added 2026-09-02, while implementing the SSH channel.** The spec described what runs remotely
> but not how output is delimited.

`SSHConnect` sends the script through one `ChannelExec` and keeps only what lies between two
markers (`ScriptInput`):

```
echo ALL_${NOTEXIST}STARTED
   … the script …
echo ALL_${NOTEXIST}COMPLETED
```

- The markers the reader looks for are `ALL_STARTED` and `ALL_COMPLETED`.
- **The indirection is the mechanism, not an accident.** `$NOTEXIST` is unset, so the shell prints
  `ALL_STARTED` while the script *text* says `ALL_${NOTEXIST}STARTED`. A shell that echoes its input
  — `set -x`, a login banner, a replayed `sudo` line — therefore cannot be mistaken for the frame.
  A rewrite that "simplifies" this to `echo ALL_STARTED` reintroduces the bug it was written to
  avoid, and only under `set -x`.
- The reader stops at `ALL_COMPLETED`; anything after it is discarded. Only stdout is read.
- Connection settings: `StrictHostKeyChecking=no`, and
  `PreferredAuthentications=password,publickey,keyboard-interactive`.

### 7-4. 원격 환경 전제 — **F3** (Windows 항목은 **NF**)

`$CTP_HOME` · `$init_path` · `$JAVA_HOME`(+ multi-jdk `$JAVA_HOME_<VERSION>`) · `$CUBRID` · `$TEST_BIG_SPACE`(선택) · PATH 의 `cubrid`.
~~Windows: `cygpath` · cygwin · `*Regedit.bat`~~ → **NF (2026-09-02)** — Windows 는 공식 stale 이므로
지킬 의무가 없다. native runner 는 Windows 를 **명시적으로 거부**한다.

⚠️ 단 testcases 에는 Windows 자산이 남아 있다 — `cygpath` 를 참조하는 케이스 **59개**,
`.answer_win` / `.answer_WIN` 파일 **155개**. NG1 동결 자산이라 사라지지 않는다.
**"동결된 입력이지만 실행하지 않는 것"** 으로 분류한다. 안 그러면 나중에 왜 안 도는지가 미궁이 된다.
⚠️ `$JAVA_HOME` 은 **케이스가 사용**하므로 새 시스템이 Go 여도 원격에 계속 셋업해야 한다.

### 7-5. 케이스 결과 회수 채널

케이스가 `<name>.result` 에 기록 → 워커가 `collectGeneralResult` 로 회수. stdout/stderr 는 워커가 capture. 워커가 `.answer` 와 `.result` 를 diff 또는 stdout 패턴 검사.

### 7-6. ⭐ `runone.sh` 의 sed normalization — **F1**

`runone.sh` 는 비교 **전에** 10+ 줄의 sed 필터 체인으로 결과를 정규화한다 (`ctl-grammar.md` §).

> *"세상의 모든 `.ctl` answer 가 이 sed normalization 후에 비교됨. 새 시스템에서 동일 normalization 반드시 보존"* — `ctl-grammar.md`

**시그니처만 동결하고 끝내면 안 된다.** 이 체인이 다르면 모든 isolation 케이스의 판정이 달라진다. 정규화 패턴 전수 목록은 §11-15, 정형화는 ADR-009.

### 7-7. RMI 워커 모드 — **폐기 (2026-09-02)**

**배포 자산만으로는 도달할 수 없다**는 것이 확인되어 폐기한다.

| 근거 | |
|---|---|
| conf 키 | `Context.java:163` 이 `agent_protocol` 을 읽고 **기본값이 `"ssh"`**. 이 키는 배포되는 13개 conf 어디에도 없다 |
| 에이전트 conf | `conf/shell_agent.conf` 가 **배포되지 않는다** |
| 서버 기동 | `service/Server` 를 띄우는 launcher 가 15개 진입점 조사에 **없다** |

**처리:** `agent_protocol=rmi` 가 conf 에 오면 **경고 후 ssh 로 진행**한다. 조용히 폐기하지 않는 이유는
NG7(dead surface 를 침묵하는 no-op 으로 남기지 않는다)이고, 실패가 아니라 경고인 이유는 §2-5 의 규칙이다 —
ssh 는 원래 기본값이므로 **테스트 결과가 달라지지 않는다**.

alive-ping 규약(`echo HELLO` → `HELLO`)도 함께 소멸한다. §11-6 은 이것으로 **해소**.

### 7-8. 원격 배포 자산 — **F1 (배포물)**

`shell/init_path/` 는 원격에 통째로 복사된다. 그 안에는 **`commonforjdbc.jar` / `commonforjdbc_aix.jar`** 가 포함된다 (`case-formats.md` §, `shell/implementation-notes.md` §14).

⚠️ 이것은 **testkit 이 빌드하는 산출물이 아니라 배포되는 원격 자산**이다. NG5("jar 를 산출하지 않는다")의 대상이 아니다 — NG5 는 *testkit 자체 빌드 산출물*에 한정된다. 기존 Ant 빌드가 계속 만들거나, 빌드된 바이너리를 그대로 vendoring 한다.

---

## 8. 하위 진입 시그니처 (subprocess 호출 시 재현 대상) — **F1**

새 바이너리가 미대체 task 를 처리할 때 **정확히 이 argv/env 를 내보내야 한다**.

### 8-1. `ConsoleAgent runCQT`

```
java cqt.console.ConsoleAgent runCQT <type> <typeAlias> <version> <charset_xml> <files...>
```
`<type>`=suite / `<typeAlias>`=conf 의 `scenario_alias` / `<version>`="32"|"64" / `<charset_xml>`=default `test_default.xml` / `<files...>`=§3-6 의 URL-style specifier 1개 이상.

### 8-2. `sql/bin/run.sh`

```
sh sql/bin/run.sh -s <scenario_category> -f <conf_absolute_path>
```
`CTP.executeSQL` 이 추가로 export 하는 환경변수 — **F1**:
```
sql_interface_type=cci             (CCI 모드)
sql_interactive=yes                (interactive 모드)
log_file_in_interactive=<path>     (interactive 모드)
```

### 8-3. `ccqt` (CCI 모드 C 바이너리)

```
$CTP_HOME/sql_by_cci/ccqt <port> <db_name> <scenario_alias> <resultFolder> \
                          <scenario_repo_root> <CTP_HOME> <cci_urlproperty>
```

### 8-4. `webconsole`

```
java cqt.webconsole.Starter <webconsole.conf> <webRoot> <start|stop>
```
(축 O 이지만 진입점은 F1 유지 — `migration-exclusions.md` §1-6)

### 8-5. `jdbc_config_file` charset XML

default `test_default.xml`. 스키마 미분석 (§11-16).

---

## 9. 셸 자산 진입점

### 9-1. `common/ext/run_*.sh` (11) — **F1 (조건부)**

CTP 내부에서는 **어디서도 호출되지 않는다**. 외부 CI/수동 실행 진입점으로 추정 → 보수적으로 F1. 실제 호출자 확인 후 하향 가능 (§11-1).

### 9-2. `common/script/*` (19) — 축이 갈린다

| 스크립트 | 축 | 등급 |
|---|---|---|
| `analyzer.sh` → `coreanalyzer.AnalyzerMain` | **T** (core dump 분석은 판정의 일부) | F2 |
| `run_cubrid_install` · `run_download` · `prepare_memory_env.sh` · `process_safe.sh` · `run_action_files` · `crash_template_*.sh` | T | F2 |
| `run_coverage_collect_and_upload` (+ `gcov/gcov`) | T | F2 |
| `issue.sh` · `analyze_failure.sh` · `report_issue.sh` · `file_core_issue.sh` · `sender.sh` · `start_producer.sh` · `start_consumer.sh` · `generate_build_test.sh` · `start_grepo_server.sh` · `upgrade.sh` · `run_grepo_fetch` · `run_git_update` · `commit_config_file` · `convert_to_git_url.sh` | **O** | **제외** |

### 9-3. `common/tpl/issue_*.tpl` — **축 O, 제외**

---

## 10. 신 ↔ 구 1:1 매핑 표 *(Phase 1 Exit 조건)*

| # | 표면 | 입력 | 출력 | 기존 CTP | cubrid-testkit | 등급 |
|---|---|---|---|---|---|---|
| 1 | 진입점 | argv | 종료 코드 + stdout | `ctp.sh` → `java -cp cubridqa-common.jar ctp.CTP "$@"`. 추가로 **`JAVA_HOME` 검사**, `tee` 캡처, **`#SCRIPTCONT` 라인 추출 후 셸 실행** | `ctp.sh` → `exec testkit "$@"`. JAVA_HOME 검사는 **실행 전 환경 검사로 대체**, `#SCRIPTCONT` 컨베이어는 **`testkit` 내부로 흡수** | F2 |
| 2 | 인자 파싱 | §1-1 | — | `CTP.main` + commons-cli 1.2 | Go CLI 파서 | F1 |
| 3 | task 해석 | task 이름 | — | `ComponentEnum` 21개 | Runner 레지스트리 14개 (+ orphan 7 미결) | F1 / 미결 |
| 4 | conf 로딩 | §2-2·§2-3 | — | `IniData` dot-prefix | Go INI 파서, 동일 의미 | F1(의미) / NF(구현) |
| 5 | conf fallback | `-c` 부재 | — | `$CTP_HOME/conf/<suite>.conf` | 동일 | F1 |
| 6 | sql 계열 실행 | §8-2 argv + env | §4-1 + §5-2·5-3 | `run.sh` → `ConsoleAgent runCQT` (§8-1) | **미대체 — §8-1·§8-2 그대로 subprocess** | F1 |
| 7 | **shell 실행** | §2-2 conf + §3-1 케이스 | §4-2 + §5-1 | reflection → `shell.main.Main.exec(conf)` | **네이티브 Go Runner (ADR-004 1차)** | F1(외부 표면) |
| 8 | rqg 실행 | 동상 | 동상 | 동일 jar + `TEST_CATEGORY=rqg` | 동일 Runner + 내부 카테고리 | F1 |
| 9 | **unittest 실행** | §7-3 4함수 | `EEOOKK` 회수 | reflection → `GeneralLocalTest.exec(conf)` | **Go Runner (shell 과 코드 공유)** | F1 |
| 10 | **jdbc 실행** | `jdbc.conf` (6키) | 미분석 | `jdbc/bin/run.sh` → **`shell.main.JdbcLocalTest`** | **shell.jar 의존 — Phase 3 에 딸려 옴** | 미결 (§11-8) |
| 11 | isolation | `isolation.conf` + §3-1 flat `.ctl` | §4-3 + §5-1·5-4·5-5 | reflection → `isolation.Main.exec(conf)` | **미대체 — 기존 jar subprocess** | F1 |
| 12 | ha_repl / cdc_repl | 각 conf | 미분석 | reflection → 각 jar `Main.exec` | **미대체 — 기존 jar subprocess** | 미결 (§11-8) |
| 13 | webconsole | §8-4 | HTTP UI | reflection → `Starter.exec` | **미대체 — §8-4 그대로** (축 O) | F1(진입점) |
| 14 | `run_shell.sh` (T5) | §1-4 축 T 8옵션 | 루프 실행 | `RunShellMain` (+scheduler.jar) | Go — **축 O 5옵션 제외**, scheduler 의존 해제 | F1(T) / 제외(O) |
| 15 | 원격 실행 채널 | SSH 자격 | 원격 stdout | `SSHConnect`(jsch) / RMI 모드 | `golang.org/x/crypto/ssh`. **RMI 모드 존치 여부 미결** | F1(SSH 동작) / 미결(RMI, §11-6) |
| 16 | 로컬 프로세스 실행 | — | — | `LocalInvoker` | `os/exec` 래퍼 | NF |
| 17 | ctltool native | `.ctl` | §4-3 + §7-6 | `ctltool/` C 바이너리 + `Makefile` | **그대로 빌드해 subprocess 호출**. 통합 형태는 ADR-007 (미결) | F1 |
| 18 | 결과 디렉터리 | — | §5-1 | `<CTP_HOME>/result/<task>/<ts>/` | 동일 | F1 |
| 19 | stdout 마커 | — | §4 전체 | 상동 | 동일 문자열 | F1 |
| 20 | 결과 파일 | — | §5-2·5-3 | `main.info` / `summary_info` / `dispatch_tc_*` | 동일 포맷 | F1 |
| 21 | Feedback | 실행 이벤트 | Null/File | §6-2 (DB 백엔드는 축 O 제외) | 동일 이벤트 + `skipType` 상수 보존 | F2 |
| 22 | 종료 코드 | — | 0 / 1 / 255 | §6-1 | 동일 | F1 |
| 23 | 배포 형태 | — | — | JVM + 7 jar + 셸 자산 | **단일 바이너리 + 셸 자산 + ctltool + `init_path/` 원격 자산** | NF |
| 24 | 빌드 | — | — | Ant `build.xml` + `ctltool/Makefile` | `go build` + Justfile + `ctltool/Makefile` 유지 | NF |

**공존 원칙 (행 6·11·12·13):** 미대체 task 는 새 `testkit` 이 **기존 CTP 자산을 subprocess 로 호출**한다. 이 라우팅 규칙을 담는 문서가 `design/migration-bridge.md`(Phase 3)다.

> ⚠️ **용어 주의** — 여기서 "브리지"는 *task 라우팅 shim* 이다. NG5 가 금지하는 "jar 호환 layer"(구 모듈이 새 구현을 라이브러리로 호출하는 것)와 **다른 것**이다. 두 개를 혼동하면 Phase 3 실행자가 ROADMAP §4 가 요구하는 산출물을 NG5 위반으로 오인한다.

---

## 11. 미확정 — 해소 시점

| # | 항목 | 왜 필요한가 | 해소 시점 |
|---|---|---|---|
| 11-1 | ~~`common/ext/run_*.sh` 의 외부 호출자~~ | **해소 (2026-09-02)** — testcases 3개 레포에서 호출자 **0건**. CTP 의 CI 워크플로(`.github/workflows/check.yml`)도 code-style 만 돌린다. 남은 가능성은 QA 조직 내부 자동화뿐이며 이 레포들에는 없다 → **F1 유지하되 근거는 "미확인"이 아니라 "이 범위에서는 호출자 없음"** | 완료 |
| 11-2 | 외부 CI 가 grep 하는 stdout 라인 | **부분 해소 (2026-09-02)** — 아래 표 참조 | 완료 |
| 11-3 | `.sql` pragma 전수 목록 + `--@<connId>` 확인 | 케이스 파서 범위 | Phase 4 (sql 대체 전) |
| 11-4 | 백업 tar.gz 파일명 정확한 구분자 | F2 → F1 승격 여부 | Phase 4 |
| 11-5 | ~~Feedback DB 스키마~~ | **해제됨** — `FeedbackDB` 는 축 O 제외 | — |
| 11-6 | ~~RMI 모드 존치/폐기~~ | **해소 (2026-09-02) — 폐기.** 근거는 §7-7 | 완료 |
| 11-7 | answer variant 선택의 **`runMode` 값 출처** | 알고리즘은 확인됨. 값이 어디서 오는지가 미상. **닫힘 (2026-09-11)** — `jdbc_config_file` XML 의 `<run_mode>` 요소. `test_default.xml` 은 주석 처리, `test_D_*.xml` 이 `_D_*` answer 와 짝 (`ConsoleBO.java:368-396`) | Phase 4 |
| 11-8 | **jdbc / ha_repl / cdc_repl 출력 표면 미분석** — inventory stubs 0/5 | §10 행 10·12 와 §6-1 의 `-1` 이 유추 | **Phase 4** — jdbc 가 Phase 3 범위에서 빠져(`module-shell.md` §7-5) 더 이상 Phase 3 블로커가 아니다 |
| 11-9 | ~~로컬 체크아웃과 baseline 차이~~ | **해소 (2026-09-02)** — `ComponentEnum.java` · `Test.java` · `bin/ctp.sh` 모두 **변경 없음**. 본 명세의 근거는 유효하다 | 완료 |
| 11-10 | `bin/ini.sh` / `IniCommand` 의 CLI 표면 + 외부 사용자 | 등급 부여 | Phase 2 |
| 11-11 | ~~`shell_ci` exclusive 키 14 vs 16 불일치~~ | **해소 (2026-09-03)** — **16 이 맞다.** `conf-matrix.md` §1-3 이 `testcase_exclude_from_file` 과 `test_category` 를 빠뜨렸다. 둘 다 구현되어 있어 범위에는 영향 없음 | 완료 |
| 11-12 | ~~`.diff_1` 이 입력인가 산출물인가~~ | **부분 해소 (2026-09-03)** — `queryPlan`·`excluded_list`·`patch` 는 **전부 입력**. `analysis/sql/io-contract.md` 에 기전 기록. `.diff_1` 자체는 Phase 4 로 남는다 | 부분 |
| 11-13 | `.ctl` grammar 정형화 | ADR-008 | Phase 2 |
| 11-14 | ~~shell fail-backup 의 Windows 동작~~ | **소멸 (2026-09-02)** — Windows 가 범위 밖이 되어 질문 자체가 사라졌다 | 완료 |
| 11-15 | **`runone.sh` sed 정규화 패턴 전수 목록** | §7-6 — 모든 isolation 판정이 여기 의존 | Phase 4 (ADR-009) |
| 11-16 | `jdbc_config_file` charset XML 스키마 | §8-5 | Phase 4 |
| 11-17 | `ErrorInterrupt` cascade-abort 정책 | 실행 중단 동작이 관측 가능. **닫힘 (2026-09-11) — 도달 불가.** `ErrorInterruptUtil.isCaseRunError` 는 `ErrorInterrupt.ERROR_INTER` 가 참일 때만 run 을 멈추는데, 그 값은 `false` 로 선언되고 CQT 어디서도 바뀌지 않는다. 동결할 행동이 없다 | Phase 4 |
| 11-20 | `run_shell.sh --config` 가 죽어 있다 | §1-4 — 재현할지 되살릴지 | Phase 3 |
| 11-21 | `run_shell.sh` 의 `STOP` 파일 | §1-4 — 명세에 없던 제어 표면 | Phase 3 |
| 11-22 | ~~`run_shell.sh` 가 실패해도 0 을 반환한다~~ | **해소 (2026-09-03)** — 명백한 버그로 판정, 축 T 에서 고친다 | 완료 |
| 11-23 | ~~JVM sweep 구문 오류를 고칠 것인가~~ | **해소 (2026-09-03)** — 고친다. 단 **B-T2 격리와 함께** 낸다. 마이그레이션이 끝나면 CTP 의 JVM 이 사라지므로 `com.navercorp.cubridqa` 필터가 아무것도 안 거르게 되고, 고쳐진 sweep 은 *시간이 갈수록 더* 파괴적이 된다 | 완료 |
| 11-24 | `.answer_cci`(2,119) · `.answer_win`(59) · charset 변형을 **읽는 코드가 CTP 에 없다** | `analysis/sql/io-contract.md` — 누가 고르는지 확인 | Phase 4 |

### 11-2 상세 — 마커별 실제 소비자 *(2026-09-02 조사)*

| 마커 | CTP 밖에서 참조하는 곳 | 판정 |
|---|---|---|
| **`found core file`** | **`cubrid-testcases-private-ex` 의 케이스 2곳** — `shell/_06_issues/_18_2h/bug_bts_22449/cases/bug_bts_22449.sh`, `shell_heavy/cbrd_21070/cases/cbrd_21070.sh` | **F1 확정. 완화 불가** — NG1 동결 자산이 직접 grep 한다 |
| `Result Root Dir` | `cubrid-testcases` 의 `ConsoleBO.log` 3곳 — 실행이 남긴 *산출물*이지 소비자가 아니다 | F1 유지 (소비자 미발견, 보수적) |
| `flag: OK` | `doc/isolation_guide.md` — 문서 | F1 유지 (소비자 미발견, 보수적) |
| `Testing End!` · `TOTAL_ELAPSE_TIME` | 없음 | F1 유지 (소비자 미발견, 보수적) |

**결론:** 하향 조정은 하지 않는다. 소비자를 하나라도 찾은 `found core file` 은 확정 F1 이고,
나머지는 *조사 범위 안에서* 소비자가 없을 뿐 QA 조직 내부 자동화까지 확인한 것은 아니다.
다만 이제 "확인하지 않아서 F1" 이 아니라 "이 범위에서는 소비자가 없지만 보수적으로 F1" 이다.

> 이 17개는 **동결 명세의 구멍**이지 Phase 1 의 미완이 아니다. Phase 1 Exit 은 §10 으로 충족되며, 각 항목은 표기된 Phase 의 진입/착수 조건으로 이월한다.
