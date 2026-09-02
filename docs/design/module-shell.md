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
| `Deploy` · `DeployOneNode` | T | `runner/shellsuite/deploy` — `init_path/` 복사, `$init_path` 셋업 |
| `DeployHA` | T | 동상. ⚠️ **이식하되 미검증** — HA 트리 162 케이스는 master/slave 토폴로지가 없어 회귀 증거에서 제외된다 (ADR-013). `impl/m1/regression-evidence.md` 에 미검증으로 명시할 것 |
| **`TestCaseGithub` · `TestCaseSVN`** | **O — 제외 (2026-09-02)** | 케이스 코퍼스를 언제 갱신할지는 운영 결정이다. 단 `testcase_update_yn=yes` 는 **실패**시킨다 — 갱신을 요청했는데 조용히 안 되면 낡은 케이스로 통과했다는 거짓 신호가 난다 (`migration-exclusions.md` §2a) |

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
