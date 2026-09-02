# Module: shell — 1차 대체 대상

- **Date:** 2026-09-02
- **Status:** Accepted (Phase 2 산출물 — ADR-004 Consequence 1 에 따라 4개 모듈 중 가장 깊게)
- **담당 task:** `shell` · `rqg` · `unittest` · **`jdbc`**
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
ctp.sh jdbc      ──▶ sh jdbc/bin/run.sh <conf> → shell.main.JdbcLocalTest   ⚠️ 뒤늦게 식별
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
| `Deploy` · `DeployOneNode` | T | `runner/shellsuite/deploy` — `init_path/` 복사, `$init_path` 셋업 |
| `DeployHA` | T | 동상 (HA 토폴로지 분기) |
| **`TestCaseGithub` · `TestCaseSVN`** | **미결 (§7-2)** | 케이스 레포를 git pull/svn up 하는 코드. 축 판정 보류 |

### 3-3. dispatch/ · main/ — 13 클래스

| 구 클래스 | 축 | 신 컴포넌트 |
|---|---|---|
| `Dispatch` | T | `dispatch` — 케이스 풀, env 별 분배, `dispatch_tc_*.txt` |
| `Main` | T | `runner/shellsuite` 의 `Run` |
| `Test` | T | 워커 루프. `extractItems` 의 *"마지막 flag 가 이긴다"* 규칙 보존 |
| `TestFactory` | T | 워커 생성 · 결과 백업 |
| `TestMonitor` | T | per-case 타임아웃 감시 → `context.WithTimeout` |
| `Context` | T | `conf.Config` + `topology` 로 대체. **암묵적 전역 제거 = M4** |
| `CheckRequirement` | T | `Runner.Validate` (C1) |
| `ShellHelper` | T | 헬퍼 — 흩어짐 |
| `Feedback` (인터페이스) | T | `feedback.Feedback` (C5) |
| `GeneralLocalTest` | T | `runner/shellsuite` 의 unittest 경로. **4함수 컨트랙트 + `EEOOKK` 보존** |
| `JdbcLocalTest` | T | `runner/shellsuite` 의 jdbc 경로 |
| **`RunShellMain`** | **T/O 혼재** | 옵션 13개 중 **T 8개만**. `--enable-report` `--report-cron` `--mailto` `--mailcc` `--issue` 는 제외 |
| **`ManualReportJob`** | **O** | **제외** — 리포트 잡 |

### 3-4. result/ — 3 클래스

| 구 클래스 | 축 | 신 컴포넌트 |
|---|---|---|
| `FeedbackNull` · `FeedbackFile` | T | `feedback` 의 두 구현 |
| **`FeedbackDB`** | **O** | **제외** (`migration-exclusions.md` §1-5) |

### 3-5. service/ — 3 클래스

| 구 클래스 | 축 | 신 컴포넌트 |
|---|---|---|
| `Server` · `ShellService` · `ShellServiceImpl` | T | **미결** — RMI 워커 모드의 존치 여부가 freeze §11-6. `Channel` 의 세 번째 구현이 되거나 사라진다. **Phase 3 착수 전 결정** |

---

## 4. 동결 표면 (이 모듈이 지켜야 하는 것)

| 표면 | 내용 |
|---|---|
| CLI | `ctp.sh {shell\|rqg\|unittest\|jdbc}` + `run_shell.sh` 의 축 T 옵션 8개 |
| conf | `shell.conf` 26 · `shell_ci.conf` 42 · `ha_shell.conf` 28 · `unittest.conf`(옵션) · **`shell_agent.conf`** |
| 케이스 | `<...>/cases/*.sh` + `<...>/answers/*.answer`. **`cases/` 세그먼트 필수는 이 모듈 한정** |
| 케이스 prologue | `. $init_path/init.sh` → `init test` → `set -x` |
| unittest plug-in | `shell/local/<TEST_TYPE>.sh` 의 `init`/`list`/`execute`/`finish` + **`EEOOKK`** 마커 |
| stdout | `[ENV START/STOP]` · `[TESTCASE] ... [OK\|NOK][, retry: N]` · `CORE_FILE:` |
| 결과 파일 | `main_snapshot.properties` · `dispatch_tc_{ALL,FIN_*}.txt` · `test_<env>.log` · `main.info` |
| 원격 자산 | `init_path/` 통째 복사. `commonforjdbc.jar` 포함 (배포 자산이지 빌드 산출물 아님) |
| 종료 코드 | 0 / 255 |

---

## 5. 실행 흐름

```
Validate   conf · scenario 디렉터리 · build URL 확인          실패 → 255
deploy     인스턴스마다 init_path/ 복사, $init_path 셋업       exec.ssh
discover   scenario 아래 cases/*.sh 수집                      caseformat "sh"
           exclusion 적용 → macroSkipped / tempSkipped 분리
dispatch   dispatch_tc_ALL.txt 기록 → env 별 워커 goroutine
  워커 루프 (env 마다)
     ├ 케이스 하나 꺼냄
     ├ 원격 실행 (타임아웃 = testcase_timeout_in_secs)
     ├ <name>.result 회수 → .answer 와 diff
     ├ core 발견 시 CORE_FILE: 출력
     ├ 실패 + 재시도 남음 → 다시 큐로 (retry: N)
     └ [TESTCASE] 마커 · dispatch_tc_FIN_<env>.txt · Feedback
report     main.info · 실패 케이스 백업 tar.gz                 종료 코드 0
```

**continue mode** 는 `ALL − ⋃FIN` 차집합으로 재개한다.

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

## 7. 미결 — Phase 3 착수 전에 답해야 하는 것

| # | 항목 | 왜 지금 필요한가 |
|---|---|---|
| 7-1 | **RMI 워커 모드 존치/폐기** (freeze §11-6) | `service/` 3클래스의 운명과 `Channel` 구현 수가 갈린다 |
| 7-2 | **`TestCaseGithub` / `TestCaseSVN` 의 축 판정** | 케이스 레포 갱신은 운영(O)에 가깝지만, `testcase_update_yn` · `testcase_git_branch` 가 F3 수용 대상이라 무시하면 조용한 동작 변화가 된다. **수용하되 무시**할지, **축 T 로 옮길**지 결정 필요 |
| 7-3 | **shell `*_fail_backup_package*.tar.gz` 의 Windows 동작** (freeze §11-14) | isolation 과 달리 OS 조건이 소스에 없다. 잘못 빼면 Windows 레인에서 실패 아티팩트가 조용히 사라진다 |
| 7-4 | **`shell_ci` exclusive 키 14 vs 16 불일치** (freeze §11-11) | 범위 산정에 직접 영향 |
| 7-5 | **jdbc 출력 표면 미분석** (freeze §11-8) | inventory stub 이 비어 있다. jdbc 가 이 모듈에 딸려 오므로 Phase 3 범위다 |

### 7-6. 범위 재산정 (ADR-004 Consequence 7)

| 증감 | 항목 |
|---|---|
| **+** | `RunShellMain` 축 T 8옵션 (loop / maxloop / maxtime / update-build / next-build-url / extend-script / prompt-continue / help) |
| **+** | `JdbcLocalTest` · `GeneralLocalTest` |
| **+** | `shell_agent.conf` 처리 |
| **−** | `ManualReportJob` · `FeedbackDB` · `GeneralFeedback` 의 메일 경로 |
| **−** | `--report-cron` 제외로 **`cubridqa-scheduler.jar` 의존이 통째로 사라진다** |
| **?** | `service/` 3클래스 — 7-1 에 종속 |
| **?** | `TestCaseGithub` · `TestCaseSVN` — 7-2 에 종속 |

**순증감은 7-1·7-2 가 정해져야 확정된다.** 그 전까지 ≈5000 LoC 추정은 근거가 약한 숫자로 취급한다.
