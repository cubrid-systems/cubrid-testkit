# Migration Exclusions — 마이그레이션 제외 목록

- **Date:** 2026-09-02
- **Status:** Accepted (프로젝트 방향성 결정, 2026-09-02)
- **관련:** `external-surface-freeze.md` (축 T 동결 명세) · `non-goals.md` · `analysis/_overview/cli-tree.md` 부록 A

---

## 0. 원칙 — 축 분리

> **테스트 실행과 QA 시스템 운영은 다른 축이다. 현재 CTP 는 이 둘이 한 덩어리로 뭉쳐 있다.
> 이번 마이그레이션은 축 T 만 옮긴다. 축 O 는 제외하고, 제외했다는 사실을 여기에 남기고,
> 나중에 새로운 층으로 다시 세운다.**

이것은 프로젝트 전체의 방향성이며, 개별 판단이 아니다. 새로운 표면을 만날 때마다 먼저 축을 판정한다.

| 축 | 정의 | 판정 질문 | 이번 마이그레이션 |
|---|---|---|---|
| **T (테스트 실행)** | 케이스를 고르고 · 돌리고 · 판정하고 · 결과를 파일로 남긴다 | *이 기능이 없으면 테스트 결과가 달라지는가?* | **대상** — `external-surface-freeze.md` 로 동결 |
| **O (QA 운영)** | 언제 돌릴지 · 누구에게 알릴지 · 어디에 등록할지 | *이 기능이 없어도 테스트는 정확히 같은 결과를 내는가?* | **제외** — 본 문서에 기록, 후속 층 |

**판정 규칙:** 축은 *패키지·파일·jar 로 나뉘지 않는다*. 같은 클래스 안에서도, 같은 CLI 옵션 목록 안에서도 갈린다. `coreanalyzer` 패키지의 `AnalyzerMain`(core dump 분석)은 축 T 이고 `IssueMain`(이슈 등록)은 축 O 다.

---

## 1. 제외 대상 — 축 O

### 1-1. 스케줄러 / 메시지 큐 (ActiveMQ + Quartz)

| 자산 | 진입 | 근거 |
|---|---|---|
| `common/sched/` 전체 | — | `cubridqa-scheduler.jar` |
| `common/script/start_producer.sh` | `scheduler.producer.Main` | cli-tree 부록 A O1 |
| `common/script/start_consumer.sh` | `scheduler.consumer.ConsumerAgent` / `ConsumerTimer` | O2 |
| `common/script/sender.sh` | `scheduler.producer.ManualSender` | O3 |
| `common/script/generate_build_test.sh` | `scheduler.producer.crontab.BuildMain` | O4 |

**제외 사유:** 언제·무엇을 돌릴지 결정하는 오케스트레이션. 테스트 결과에 영향이 없다.
**의존:** ActiveMQ 5.8 (2013) + Quartz 2.2.1 (2014) — 둘 다 노후.

### 1-2. 메일 / 리포트

| 자산 | 근거 |
|---|---|
| `common.MailSender` | `common/io-contract.md` §1-1 |
| `RunShellMain --enable-report` / `--report-cron` / `--mailto` / `--mailcc` | cli-tree 부록 A T5 |
| `shell.common.GeneralFeedback` 의 메일 경로 | — |
| **`check_disk_space` 의 메일 인자 2개** (2026-09-02 추가) | `Test.checkDiskSpace` |

**제외 사유:** 결과의 *전달*이지 *생성*이 아니다.

⚠️ **디스크 체크는 남긴다.** `check_disk_space <fs> <size> "<to>" "<cc>"` 는 **케이스마다** 돌면서
공간이 모자라면 메일을 보낸다 — 축 O 가 per-case 루프 한가운데까지 들어와 있는 자리다. 공간 부족은
케이스를 엉뚱하게 실패시키므로 **검사 자체는 축 T** 이고, 통지만 뺀다. CTP 도 검사 실패로 실행을
중단하지는 않았고(로그만 남긴다) 그 동작을 유지한다.
⚠️ `--report-cron` 이 `RunShellMain` 의 classpath 에 `cubridqa-scheduler.jar` 를 끌어들이는 유일한 이유다. 이 옵션을 제외하면 shell 모듈의 스케줄러 의존이 **함께 끊긴다** — ADR-004 Phase 3 범위가 그만큼 줄어든다.

### 1-3. 이슈 등록 (JIRA)

| 자산 | 진입 | 근거 |
|---|---|---|
| `common.coreanalyzer.IssueMain` | `common/script/issue.sh` | O7 |
| `common.MergeTemplate` + `tpl/issue_*.tpl` 3종 | `common/script/analyze_failure.sh` | O8 |
| `common/script/file_core_issue.sh` · `report_issue.sh` | — | `common/io-contract.md` §2-1 |
| `RunShellMain --issue` | — | T5 |

**제외 사유:** 발견된 결함의 *사후 처리*. 새 층에서 다시 설계한다 (지금은 JIRA 하드코딩, 새 층은 채널 추상화가 자연스럽다).

### 1-4. 저장소 서비스 / 자가 업그레이드

| 자산 | 진입 | 근거 |
|---|---|---|
| `common/grepo/` (RMI git 저장소 서비스) | `start_grepo_server.sh` → `RepoServiceImpl` | O5 |
| `common.grepo.UpgradeMain` | `common/script/upgrade.sh` | O6 |
| `common/script/run_grepo_fetch` · `run_git_update` | — | `common/io-contract.md` §2-1 |

**제외 사유:** git 저장소 접근을 RMI 서비스로 감싼 2016년식 우회. 새 층에서는 git CLI 또는 CI 가 직접 한다.

### 1-4a. 빌드 설치 (2026-09-02 추가)

| 자산 | 진입 | 축 판정 |
|---|---|---|
| `DeployOneNode.deploy_ctp` | `common/script/upgrade.sh` | **축 O** — 1-4 의 자가 업그레이드 그 자체 |
| `DeployOneNode.deploy_build_on_linux` | `run_cubrid_install <role> <url> <extra>` | **축 O — 제외** |
| `cubrid_download_url` · `cubrid_additional_download_url` · `cubrid_install_role` | conf | 경고 후 진행 |

**제외 사유:** *어떤 빌드를 언제 어디서 가져와 설치할지*는 QA 운영 결정이다. 테스트 실행기는
**이미 설치되어 있는 빌드를 시험한다**. 실무적으로도 `run_cubrid_install` 은 CTP 가 원격에 배포한
자기 환경 안에서만 존재하는 셸 함수라, 이걸 옮기면 "CTP 자산을 안 쓴다"는 전제가 깨진다.

**남기는 것:** `updateCUBRIDConfigurations`(인스턴스 파라미터를 `ini.sh` 로 conf 에 반영)와
`backup_linux`(`~/.CUBRID_SHELL_FM` 스냅샷)는 **축 T** 다. 전자는 인스턴스끼리 포트·공유메모리가
겹치지 않게 하는 유일한 장치이고, 후자는 모든 케이스가 같은 설치 상태에서 시작하게 하는 장치다.

**설정 키 정책 적용:** `cubrid_download_url` 이 설정됐는데 무시하면 *다른 빌드*를 시험하고 결과를
믿게 된다 — 실패시켜야 할 것 같지만, 그 키가 있다는 사실만으로는 현재 설치된 빌드가 틀렸다고 말할 수
없다. **경고하고, 설치된 빌드의 build id 를 출력해서** 사람이 대조할 수 있게 한다.

### 1-4b. 기계 플릿 (2026-09-03 추가 — ADR-014)

| 자산 | 축 판정 |
|---|---|
| `env.instanceN.*` **인스턴스 인벤토리** (호스트·계정·비밀번호) | **축 O — 제외** |
| `TestFactory.concurrentDeploy` — N대 동시 배포 | **축 O — 제외** |
| env 별 워커 fan-out, `Dispatch` 의 기계 간 분배 | **축 O — 제외** |
| `startConfigMonitor` — 실행 중 기계 추가/제거(`joinTest`) | **축 O — 제외** |
| `ssh.relatedhosts`, `getRelatedHosts` | **축 O — 제외** |
| **`exec.Channel` (C3)** | **축 T — 유지.** "DB 가 있는 곳에서 명령을 돌린다" |
| **`exec.SSH`** | **축 T — 유지하되 강등.** 그 '한 대'가 원격일 때 쓰는 구현 하나. 기본 경로 아님 |
| **`exec.Local`** | **축 T — 기본** |
| `updateCUBRIDConfigurations` (`ini.sh`) | **축 T — 유지.** 다만 이유가 바뀐다: *시험 대상 엔진 설정*이지 플릿 프로비저닝이 아니다. 한 대에 적용한다 |

**제외 사유:** *어느 기계들에서, 어떻게 접속해서, 어떻게 나눠 돌릴지*는 운영 결정이다. 근거 두 가지:

1. **출하되는 conf 14개 중 플릿을 실제로 설정한 파일이 없다.** SSH 키는 전부 placeholder
   (`env.instance1.master.ssh.host=<master ip>`)이거나 주석이다. `shell.conf` 의 live 키 3개 중
   기계를 지목하는 것은 없다. **CTP 는 로컬 러너로 출하되고 플릿은 QA 팀이 얹는다.**
2. **첫 side-by-side 실행이 이것 때문에 막혔다** (`evidence/regression-shell.md`). 러너가 기계를
   소유한다고 가정해서 사용자 프로세스 63개와 공유메모리 13개를 죽이려 했고, PID/IPC namespace 로
   가두고서야 돌았다. **격리된 기계를 내주는 건 운영 층이 러너에게 할 일**이지 러너가 공유 기계에
   할 일이 아니다.

**설정 키 정책 적용:** 인스턴스를 여러 개 적어두면 **첫 번째만 쓰고 나머지를 이름과 함께 경고**한다.
빼먹는 대가는 처리량이지 정확성이 아니므로 실패시키지 않는다 (§2a).

**HA 는 예외가 아니다.** `ha_repl` 의 master/slave 는 **시험 대상이 다중 노드인 것**이지 러너가
플릿을 가진 게 아니다. 러너는 여전히 한 곳에서 돌면서 여러 노드에 걸친 DB 를 구동한다.

### 1-5. Feedback 의 DB 백엔드

| 자산 | 축 판정 |
|---|---|
| `Feedback` **인터페이스와 이벤트 정의** | **축 T** — 테스트 진행 상태의 표현. 동결 (`external-surface-freeze.md` §6-2) |
| `FeedbackNull` | **축 T** — 기본 동작 |
| `FeedbackFile` | **축 T** — 결과 파일 |
| **`FeedbackDB`** (isolation / shell 각각) | **축 O** — QA 대시보드 적재. **제외** |

**제외 사유:** 이벤트를 *어디에 쌓을지*는 운영 관심사다. 이벤트 자체는 축 T 이므로 인터페이스는 보존한다.
**효과:** `external-surface-freeze.md` §11 의 "Feedback DB 스키마 확인" 항목이 **Phase 3 블로커에서 해제**된다.

### 1-6. webconsole

`sql/webconsole/` + `cqt.webconsole.{Starter,WebServer,compare.Compare}` + `conf/webconsole.conf`.

**축 판정:** 결과 *조회* UI — 축 O.
**처리:** 제외하되 **기존 자산을 그대로 subprocess 로 계속 호출**한다 (`ctp.sh webconsole start|stop` 은 F1 유지). 죽이지 않고 건드리지도 않는다.

---

## 2. 제외의 정확한 의미

제외는 **삭제가 아니다.** 세 가지 중 하나다:

| 처리 | 의미 | 해당 |
|---|---|---|
| **동결 유지 + 기존 자산 호출** | 새 시스템이 진입점만 호환 유지하고 기존 구현을 subprocess 로 부른다 | webconsole (1-6) |
| **미이관 + 기존 자산 존치** | 새 시스템이 관여하지 않는다. 기존 CTP 자산이 그대로 살아 별도로 운영된다 | scheduler (1-1), grepo (1-4) |
| **미이관 + 표면에서 제거** | 새 시스템의 CLI/동작에서 사라진다. 필요해지면 새 층이 제공한다 | `RunShellMain` 의 O 축 옵션 5개 (1-2, 1-3) |

**세 번째만 사용자 가시 변화다.** `--enable-report` / `--report-cron` / `--mailto` / `--mailcc` / `--issue` 를 새 `testkit` 이 받지 않는다는 뜻이므로, 이는 **NG2(외부 표면 동결)의 명시적 예외**이며 아래 §4 에 기록한다.

---

## 2a. 제외된 기능의 conf 키를 만나면 — 결과 신뢰성 기준 *(2026-09-02)*

제외했다고 키가 사라지지 않는다. F3 는 "기존 입력을 수용해야 한다"이므로 키는 계속 들어온다.
무엇을 할 것인가의 규칙은 하나다:

> **그 키를 무시했을 때 사용자가 테스트 결과를 잘못 믿게 되면 실패, 아니면 경고.**

| 키 / 옵션 | 처리 | 왜 |
|---|---|---|
| `testcase_update_yn=yes` · `testcase_git_branch` | **실패** | 갱신을 요청했는데 안 되면 **낡은 케이스로 통과했다는 거짓 신호**가 난다. 가장 위험한 종류다 |
| `agent_protocol=rmi` | **경고 후 ssh 진행** | ssh 가 원래 기본값이고 결과가 같다 |
| `--enable-report` · `--report-cron` · `--mailto` · `--mailcc` · `--issue` | **경고 후 진행** | 결과의 *전달*이지 *생성*이 아니다 |
| `feedback_type=db` | **경고 후 file 로 진행** | 이벤트는 남고 적재처만 바뀐다 |
| `conf/shell_agent.conf` | **해당 없음** | 배포되지 않는 파일이라 애초에 표면이 아니다 |

⚠️ 초안은 RMI 를 실패로, `testcase_update_yn` 을 경고로 적었다. **원칙 없이 사례별로 답한 결과였고,
위험도로 보면 정확히 반대였다.** 이 표가 정정본이다.

## 3. 나중의 "새로운 층"

축 O 를 다시 세울 때의 전제만 기록한다. 설계는 그 시점의 일이다.

- **경계:** 새 층은 `testkit` 의 *출력*(결과 디렉터리 · `main.info` · Feedback 이벤트)을 **입력으로 소비**한다. `testkit` 내부를 호출하지 않는다.
- **방향:** 의존은 `운영 층 → testkit` 단방향. 역방향 의존(테스트 러너가 메일을 보내는 것)이 지금 문제의 원인이다.
- **트리거:** strangler-fig Phase 5 완료 후, 또는 운영 필요가 실제로 발생했을 때. 둘 중 빠른 쪽.
- **ADR 자리:** ADR-012 *(예약 — `adr/README.md`)*.

---

## 4. NG2 예외 기록 *(감사 추적용)*

`non-goals.md` NG2 는 "외부 표면을 바꾸지 않는다" 이다. 본 문서 §1-2·§1-3 이 그 예외다.

| 제거되는 표면 | 등급이었던 것 | 사유 | 사용자 영향 |
|---|---|---|---|
| `RunShellMain --enable-report` | F1 후보 | 축 O | 리포트 메일이 발송되지 않음 |
| `RunShellMain --report-cron <expr>` | F1 후보 | 축 O | 정기 리포트 없음 + scheduler 의존 해제 |
| `RunShellMain --mailto` / `--mailcc` | F1 후보 | 축 O | 수신자 지정 불가 |
| `RunShellMain --issue <url>` | F1 후보 | 축 O | 메일 본문의 이슈 링크 없음 |
| `common/script/{issue,sender,start_*,generate_build_test,upgrade}.sh` | F2 후보 | 축 O | 해당 스크립트는 **기존 CTP 트리에 그대로 존재**하므로 계속 사용 가능 |

**결정 근거:** 이 5개 옵션은 전부 `doc/` 가이드에 문서화되어 있으나, 모두 *결과의 전달*이지 *테스트 실행*이 아니다. 유지하면 새 시스템이 SMTP·Quartz·JIRA 클라이언트를 떠안는다.

**되돌리기 조건:** 운영자가 `run_shell.sh --loop --enable-report` 조합에 실제로 의존하고 있음이 확인되면, `--enable-report` 만 새 층이 제공할 때까지 **기존 `run_shell.sh` 를 그대로 쓰도록 안내**한다 (새 시스템이 재현하지 않는다).
