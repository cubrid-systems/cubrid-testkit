# Module: shell — 1차 대체 대상

- **Date:** 2026-09-02
- **Status:** Accepted (Phase 2 산출물 — ADR-004 Consequence 1 에 따라 4개 모듈 중 가장 깊게)
- **담당 task:** `shell` · `rqg` · `unittest`  *(2026-09-02: `jdbc` 는 범위에서 제외 — §7-5)*
- **어휘:** 이 문서는 **구 module** 을 **신 Runner** 로 옮기는 매핑이다. task / suite / module / Runner 의 정의는 `CONTEXT.md`
- **Inputs:** `analysis/shell/{requirements,design,io-contract,implementation-notes,test-corpus}.md` · `analysis/_overview/cli-tree.md` 부록 A · `design/{architecture,contracts}.md`

---

## 1. 왜 이 모듈이 1차인가 (ADR-004 요약)

모듈 경계가 명확하고, `shell.common.*` 를 흡수하면 후속 3모듈(isolation·ha_repl·cdc_repl)의 대체 비용이
크게 줄며, shell-format 케이스가 가장 광범위하다. 그리고 **같은 jar 를 공유하는 task 가 셋 더 딸려 온다**.

---

## 2. 담당 task 와 진입점

```
ctp.sh shell     ──▶ reflection → shell.main.Main.exec(conf)
ctp.sh rqg       ──▶ 같은 경로 + TEST_CATEGORY=rqg
ctp.sh unittest  ──▶ reflection → shell.main.GeneralLocalTest.exec(conf)
ctp.sh jdbc      ──▶ sh jdbc/bin/run.sh <conf> → shell.main.JdbcLocalTest   ⚠️ 범위 밖 (§7-5)
shell/init_path/run_shell.sh ──▶ shell.main.RunShellMain                    ⚠️ 두 번째 CLI 트리
```

⚠️ 아래 둘은 Phase 0 의 `cli-tree.md` 본문에 없었고 부록 A 조사에서 드러났다. **ADR-004 §7-2 의
"≈5000 LoC" 추정은 이 둘을 포함하지 않는다** (Consequence 7 — 재산정 대상, §7).

---

## 3. 신 ↔ 구 매핑 (35 클래스)

축 판정이 먼저다. **축 O 는 옮기지 않는다** (`migration-exclusions.md`).

### 3-1. common/ — 11 클래스

| 구 클래스 | 축 | 신 컴포넌트 |
|---|---|---|
| `CommonUtils` | T | 흩어짐 — `conf` · `result` · 표준 라이브러리. 1:1 대응 없음 |
| `Constants` | T | 각 패키지의 상수. `SKIP_TYPE_*` 는 `feedback` 에 보존 (C5) |
| `SSHConnect` | T | `exec.ssh` (`golang.org/x/crypto/ssh`) |
| `LocalInvoker` | T | `exec.local` (`os/exec`) |
| `Log` | T | 표준 `log/slog` |
| `ScriptInput` · `ShellScriptInput` · `GeneralScriptInput` | T | `exec` 의 명령 조립. 인터페이스 계층은 유지하지 않는다 |
| `HttpUtil` | T | build URL 확인 — `net/http` |
| `SyncException` | T | Go 에러 값 |
| **`GeneralFeedback`** | **T/O 혼재** | 진행 이벤트는 `feedback` (T). **메일 경로는 제외** (O) |

### 3-2. deploy/ — 5 클래스

| 구 클래스 | 축 | 신 컴포넌트 |
|---|---|---|
| `Deploy` · `DeployOneNode` | T/O 혼재 | `runner/shellsuite/deploy` — 인스턴스 파라미터를 `ini.sh` 로 conf 에 반영, `~/.CUBRID_SHELL_FM` 스냅샷. **`deploy_ctp`(CTP 자기 업그레이드)와 `deploy_build`(`run_cubrid_install`)는 축 O — 제외** (2026-09-02). 어떤 빌드를 언제 설치할지는 운영 결정이고, 러너는 이미 설치된 빌드를 시험한다 |
| `DeployHA` | T | 동상. ⚠️ **이식하되 미검증** — HA 트리 162 케이스는 master/slave 토폴로지가 없어 회귀 증거에서 제외된다 (ADR-013). `evidence/regression-shell.md` 에 미검증으로 명시할 것 |
| **`TestCaseGithub` · `TestCaseSVN`** | **O — 제외 (2026-09-02)** | 케이스 코퍼스를 언제 갱신할지는 운영 결정이다. 단 `testcase_update_yn=yes` 는 **실패**시킨다 — 갱신을 요청했는데 조용히 안 되면 낡은 케이스로 통과했다는 거짓 신호가 난다 (`migration-exclusions.md` §2a) |

### 3-3. dispatch/ · main/ — 13 클래스

| 구 클래스 | 축 | 신 컴포넌트 |
|---|---|---|
| `Dispatch` | T | `dispatch` — 케이스 풀과 **재시도 순서**. `dispatch_tc_*.txt`. **env 별 분배는 축 O** (2026-09-03, ADR-014): 한 대 = 워커 하나다 |
| `Main` | T | `runner/shellsuite` 의 `Run` |
| `Test` | T | 워커 루프. `extractItems` 의 *"마지막 flag 가 이긴다"* 규칙 보존 |
| `TestFactory` | T/O 혼재 | 워커 생성 · 결과 백업은 T. **N대 동시 배포·env 별 워커 fan-out·실행 중 기계 추가(`joinTest`·`startConfigMonitor`)는 축 O — 제외** (2026-09-03, ADR-014) |
| `TestMonitor` | T | `monitor` — per-case 타임아웃 감시. **`context.WithTimeout` 이 아니다** (2026-09-02 정정): 타임아웃은 케이스 명령을 취소하지 않고 **케이스가 기다리는 원격 프로세스를 죽인다**. 취소하면 채널만 닫히고 원격 프로세스는 살아남아 다음 케이스가 포트/락을 못 잡는다. 그래서 모니터는 **자기 SSH 세션**을 따로 가진다 |
| `Context` | T | `conf.Config` + `topology` 로 대체. **암묵적 전역 제거 = M4** |
| `CheckRequirement` | T | `Runner.Validate` (C1) |
| `ShellHelper` | T | 헬퍼 — 흩어짐 |
| `Feedback` (인터페이스) | T | `feedback.Feedback` (C5) |
| `GeneralLocalTest` | T | `runner/shellsuite` 의 unittest 경로. **4함수 컨트랙트 + `EEOOKK` 보존** |
| `JdbcLocalTest` | T | `runner/shellsuite` 의 jdbc 경로 |
| **`RunShellMain`** | **T/O 혼재** | 옵션 13개 중 **T 8개만**. O 축 5개는 **경고 후 진행** (`migration-exclusions.md` §2a) |
| **`ManualReportJob`** | **O** | **제외** — 리포트 잡 |

### 3-4. result/ — 3 클래스

| 구 클래스 | 축 | 신 컴포넌트 |
|---|---|---|
| `FeedbackNull` · `FeedbackFile` | T | `feedback` 의 두 구현 |
| **`FeedbackDB`** | **O** | **제외** (`migration-exclusions.md` §1-5) |

### 3-5. service/ — 3 클래스

| 구 클래스 | 축 | 신 컴포넌트 |
|---|---|---|
| `Server` · `ShellService` · `ShellServiceImpl` | — | **이식하지 않는다 (2026-09-02).** RMI 워커 모드는 배포 자산만으로 도달 불가능함이 확인되어 폐기 (freeze §7-7). `agent_protocol=rmi` 는 경고 후 ssh 로 진행 |

---

## 4. 동결 표면 (이 모듈이 지켜야 하는 것)

| 표면 | 내용 |
|---|---|
| CLI | `ctp.sh {shell\|rqg\|unittest\|jdbc}` + `run_shell.sh` 의 축 T 옵션 8개 |
| conf | `shell.conf` 26 · `shell_ci.conf` 42 · `ha_shell.conf` 28 · `unittest.conf`(옵션) · **`shell_agent.conf`** |
| 케이스 | `<name>/cases/<name>.sh` — 스크립트 이름이 **두 단계 위 디렉터리 이름과 같아야** 한다 (2026-09-02 정정, `evidence/spec-corrections.md`). `answers/` 는 케이스가 쓰는 관례이지 러너가 읽는 파일이 아니다 |
| 케이스 prologue | `. $init_path/init.sh` → `init test` → `set -x` |
| unittest plug-in | `shell/local/<TEST_TYPE>.sh` 의 `init`/`list`/`execute`/`finish` + **`EEOOKK`** 마커 |
| stdout | `[ENV START/STOP]` · `[TESTCASE] <case> EnvId=<env> [OK\|NOK][, TRY-><N>]`. **`CORE_FILE:` 는 이 모듈 표면이 아니다** — sql/medium 의 `run_sql.sh` 소유 (2026-09-02 정정) |
| 결과 파일 | `main_snapshot.properties` · `dispatch_tc_{ALL,FIN_*}.txt` · `test_<env>.log` · **`monitor_<env>.log`** · **`feedback.log`** · **`test_status.data`** · **`current_task_id`** · **`test-<category>.xml`** (2026-09-02 추가 — 뒤 넷은 기본 백엔드인 `FeedbackFile` 이 쓴다. `monitor_*` 는 `Log` 생성자가 파일을 즉시 만들어 리눅스에서는 보통 빈 파일). `main.info` 는 이 모듈 것이 아니다 |
| 원격 자산 | `init_path/` 통째 복사. `commonforjdbc.jar` 포함 (배포 자산이지 빌드 산출물 아님) |
| 종료 코드 | 0 / 255 |
| **로컬 모드** | `env.instanceN.*` 키가 하나도 없으면 **에러가 아니라 로컬 실행**이다 (2026-09-02 정정). env id 는 `local`, 채널은 `exec.Local`, kill 스크립트는 `*.sh` 쓸어담기를 뺀 로컬 형태. `Main.exec` 의 `Not found any environment instance` 는 **도달 불가능한 분기**다 — `Context` 생성자가 이미 `local` 을 넣어 놨다 |

---

## 4a. 범위: 기계 한 대 (2026-09-03, ADR-014)

이 러너는 **한 대에서 돈다**. 로컬이 기본이고, 원격이어도 '한 대'다.

| | |
|---|---|
| 축 T | "DB 가 있는 곳에서 이 명령을 돌려라" — `Channel` 하나 (C3) |
| 축 O | "기계 8대가 있고, 3,452개를 나눠 뿌리고, 각각에 빌드를 깔고, 죽으면 빼라" — 플릿 |

`env.instanceN` 이 여럿이면 **첫 번째만 쓰고 나머지를 이름과 함께 경고**한다. 아래 §5·§6 의
"env 마다" 는 이 결정으로 대체되었다 — 워커는 하나다.

`exec.SSH` 는 삭제가 아니라 **강등**이다. 그 한 대가 원격일 때 쓰는 구현으로 남는다.

---

## 5. 실행 흐름

```
Validate   conf · scenario 디렉터리 · build URL 확인          실패 → 255
deploy     인스턴스마다 init_path/ 복사, $init_path 셋업       exec.ssh
discover   <name>/cases/<name>.sh 수집 → 정렬                shellsuite.IsCase
           skip 매크로 · exclusion 적용 → macroSkipped / tempSkipped 분리
dispatch   dispatch_tc_ALL.txt 기록 → env 별 워커 goroutine
  워커 루프 (env 마다)
     ├ 케이스 하나 꺼냄 (dispatch.Queue)
     ├ CUBRID 초기화 → 원격 실행 (타임아웃 = testcase_timeout_in_secs)
     ├ do_check_more_errors → <name>.result 회수 → NOK 줄 유무로 판정
     ├ 실패 + core 없음 + 재시도 남음 → 재시도 큐 (1차 패스 완료 후 처리)
     └ [TESTCASE] 마커 · dispatch_tc_FIN_<env>.txt · Feedback
report     실패 케이스 백업 tar.gz                             종료 코드 0
```

**continue mode** 는 `ALL − ⋃FIN` 차집합으로 재개한다.

**판정은 diff 가 아니다** (2026-09-02 정정). 러너는 `.answer` 를 읽지 않는다 — shell 모듈 전체
소스에 `answer` 라는 문자열이 없다. 케이스가 스스로 `<name>.result` 에 판정을 쓰고, 러너는 그 파일을
`cat` 해서 `NOK` 부분문자열이 든 줄이 하나라도 있으면 실패로 본다. `.answer` 대조는 케이스가
`$init_path` 헬퍼로 직접 하는 일이다. `main.info` 도 이 모듈이 쓰지 않는다 (sql/cqt 소유).

**재시도는 1차 패스가 전부 끝난 뒤에 시작한다.** 워커가 3번 케이스를 일찍 실패시켜도 3000번이
도는 동안 재시도하지 않는다. 재시도의 목적이 "불안정한 케이스"와 "깨진 빌드"를 가르는 것이므로,
빌드가 전 범위를 한 번 받아본 뒤라야 그 구분이 의미를 가진다. **core 를 남긴 실패는 재시도하지
않는다** — 다시 돌리면 찾으러 온 증거를 덮어쓴다.

---

## 6. 이 모듈이 남기는 것 (후속 3모듈을 위한 자산)

`shell.common.*` 가 4모듈의 공통 인프라였다는 M0 발견이 여기서 현금화된다.

| 신 패키지 | 후속에서 그대로 쓰는 모듈 |
|---|---|
| `exec` (local · ssh) | isolation · ha_repl · cdc_repl |
| `dispatch` | isolation · ha_repl · cdc_repl |
| `result.Sink` | 전부 |
| `feedback` | 전부 |
| `topology` | 전부 |
| `coreanalyze` | isolation · sql |

⚠️ **어댑터를 만들지 않는다.** 구 3모듈은 기존 jar 를 그대로 subprocess 로 쓰고, 신 구현과 서로를
호출하지 않는다 (ADR-004 §7-4). 구 `cubridqa-shell.jar` 는 그들을 위해 계속 빌드된다.

---

## 7. 결정 기록 — 2026-09-02 인터뷰에서 해소

| # | 항목 | 결정 |
|---|---|---|
| 7-1 | RMI 워커 모드 | **폐기.** 배포 자산만으로 도달 불가능(freeze §7-7). `service/` 3클래스 미이식. `agent_protocol=rmi` 는 경고 후 ssh |
| 7-2 | `TestCaseGithub` / `TestCaseSVN` | **축 O — 제외.** 단 `testcase_update_yn=yes` 는 실패시킨다 (`migration-exclusions.md` §2a) |
| 7-3 | shell fail-backup 의 Windows 동작 | **질문 소멸.** Windows 가 공식 stale 이라 범위 밖 |
| 7-4 | `shell_ci` exclusive 키 14 vs 16 | **미해소 — Phase 3 착수 시 확인** (freeze §11-11). 범위 산정에만 영향 |
| **7-5** | **`jdbc` 를 범위에 넣는가** | **제외.** `jdbc/bin/run.sh` 는 `shell.main.JdbcLocalTest` 를 **jar 에서 직접** 띄우고 testkit 을 경유하지 않는다. 구 `cubridqa-shell.jar` 는 다른 3모듈 때문에 어차피 계속 빌드되므로 jdbc 는 손대지 않아도 그대로 돈다. **초안이 "딸려 온다"고 단정한 것은 근거가 없었다** |
| **7-6** | **Windows** | **범위 밖.** 공식 stale. native runner 는 명시적으로 거부한다 |
| **7-7** | **회귀 동등성의 정의** | **정규화 후 diff 0.** 코퍼스·마스킹 목록·제외 항목은 **ADR-013** |

### 7-8. 범위 재산정 (ADR-004 Consequence 7)

| 증감 | 항목 |
|---|---|
| **+** | `RunShellMain` 축 T 8옵션 (loop / maxloop / maxtime / update-build / next-build-url / extend-script / prompt-continue / help) |
| **+** | `JdbcLocalTest` · `GeneralLocalTest` |
| **+** | `shell_agent.conf` 처리 |
| **−** | `ManualReportJob` · `FeedbackDB` · `GeneralFeedback` 의 메일 경로 |
| **−** | `--report-cron` 제외로 **`cubridqa-scheduler.jar` 의존이 통째로 사라진다** |
| **−** | `service/` 3클래스 — RMI 폐기로 미이식 |
| **−** | `TestCaseGithub` · `TestCaseSVN` — 축 O 제외 |
| **−** | `JdbcLocalTest` — 범위 밖 |
| **−** | Windows 경로 분기 전반 |

**7-1·7-2·7-5·7-6 이 모두 제외 방향으로 정해져 순증감은 음수다.** 그래도 ≈5000 LoC 라는 숫자 자체는
근거가 약하므로(원래 `Main.exec` 한 갈래만 가정한 값이다) **재추정하지 않고 폐기한다.**
범위는 LoC 가 아니라 위 표의 항목 목록으로 표현한다.
