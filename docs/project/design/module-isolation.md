# Module: isolation — 3차 대체 대상

- **Date:** 2026-09-02 (매핑 수준) · **2026-09-15 (설계로 확장 — Phase 4 의 두 번째 모듈 착수)**
- **Status:** Draft — P0(측정·문서) 진행 중. 확정되는 결정은 ADR 로 올린다
- **담당 task:** `isolation`
- **관련:** ADR-007 (실행부, 초안) · ADR-013 (동등성 — isolation 판은 ADR-018 로 예약) · ADR-014 (한 기계) ·
  ADR-015 (축 B) · `evidence/isolation-baseline.md` · `evidence/spec-corrections.md` §8
- **Inputs:** `analysis/isolation/*` · CTP 소스 직접 확인분. 행 번호는 전부 `cubrid-testtools` develop `a1bec87` 의
  `CTP/isolation/` 기준. 분석 문서와 소스가 다른 곳은 소스를 따르고 `spec-corrections.md` §8 에 적었다

---

## 0. 세 갈래, 그리고 순서

| 갈래 | 무엇 | 끝나는 조건 |
|---|---|---|
| **호환 (축 T)** | CTP 가 isolation 을 돌리는 능력 그대로 — 단계, 판정, 동결 출력, 종료 코드 | ADR-018 의 게이트: CTP 와 testkit 을 같은 코퍼스에 돌려 정규화 후 diff 0 |
| **향상 (축 B)** | shell·sql 에서 만든 것 — 컨테이닝, 깨끗한 코퍼스, 슬롯, 보드 — 을 isolation 에 | 항목마다 스스로 선언한 증거 (ADR-015 기준 3) |
| **문서화** | as-built 가이드 `docs/category/isolation/` 와 evidence | 가이드가 코드와 어긋나지 않음 |

**sql 과 같은 모양이다.** 케이스를 실행하는 것은 기존 자산 그대로(sql 의 CQT ↔ 여기의 `runone.sh`·ctltool), 그
둘레 — 탐색·제외·큐·슬롯·판정·기록 — 는 Go. 슬롯은 처음부터 구조이고, **기본값은 슬롯 4개다** (2026-09-15,
사용자 결정 — ADR-018 의 규칙으로 슬롯 1개와 4개 모두 러너 차이 0). 기계가 작으면 줄인다: CPU 하나에 슬롯 하나까지,
가용 메모리에서 2 GB 를 뺀 것을 슬롯당 1.5 GB 로 나눈 만큼까지. `parallel_slots` 를 쓰면 그 값, `1` 이면 CTP 처럼 직렬.

**컨테이닝은 선택이 아니다.** `runone.sh` 는 케이스마다 사용자의 `sleep`·`qactl`·`qacsql` 을, 셋업마다 사용자의
`cub` 전부를 `pkill -9` 한다(§2-3). 개발자가 일하는 기계에서 네임스페이스 없이 돌리면 그 사람의 다른 CUBRID 가 죽는다.
그래서 슬롯 1개로 도는 호환 run 도 네임스페이스 안이다 — shell 이 "모든 워커가 네임스페이스를 받는다"로 간 이유와 같다.

---

## 1. 범위

| | 이번 | 나중 / 제외 |
|---|---|---|
| DBMS | CUBRID (`qactl` + `qacsql`) | MySQL·Oracle 클라이언트(`qactlm`·`qamysql`·`qactlo`·`qaoracle`) — **지원하지 않는다.** `prepare_mysql`·`prepare_oracle` 은 CTP 에서도 `echo "TODO"` 다 (`prepare.sh:68-79`) |
| 기계 | 한 대, 로컬 (ADR-014) | `env.*` 인스턴스 여럿 — shell 과 같게 첫 번째만 쓰고 나머지를 이름으로 경고 |
| 빌드 설치 | 설치된 빌드를 테스트 | `cubrid_download_url` — 경고 후 무시 (`migration-exclusions.md` 1-4a) |
| 케이스 갱신 | — | `testcase_update_yn=yes` 는 거부 (shell 과 같다) |
| feedback | `file` | `database` 는 경고 후 file, 그 밖은 없음 (shell 과 같다) |
| TAP 출력 (`runone.sh -t`) | — | CTP 의 Java 가 넘기지 않는다. 표면이 아니다 |

---

## 2. 호환 (축 T)

### 2-0. 구 ↔ 신 매핑

2026-09-02 의 매핑 표를 소스로 고쳤다.

| 구 | 신 |
|---|---|
| `Main` · `Context` · `TestFactory` | `runner/isolationsuite` |
| `Dispatch` — `find` 순서, 제외, `dispatch_tc_ALL.txt` | `dispatch.Queue` + 이 러너의 탐색. 정렬은 shell 과 같은 변경(`spec-corrections.md` "Where CTP does not agree with itself") |
| `Test.runAll` — env 당 워커 | 슬롯마다 워커. **재시도는 러너에 없다** — `runone.sh -r` 가 안에서 한다 |
| `CheckRequirement` | `isolationsuite` 의 자기 checker — 목록·순서·문구가 모두 shell 과 달라, 매개변수로 만드는 것보다 짧다(§2-5) |
| `Deploy` · `DeployOneNode` | 프로세스 정리 + `inquire_on_exit=3` 추가 + conf 섹션 반영(`ini.sh`). 설치는 제외 |
| `TestCaseGithub` | 제외 |
| `FeedbackFile` | `feedback.File` 의 **isolation 방언** — 줄 머리·이벤트가 shell 과 다르다(§2-5) |
| `FeedbackDB` | 축 O — 제외 |
| `TestMonitor` · `startConfigMonitor` · `processCoreFile` | CTP 에서 **주석 처리되어 돌지 않는다** (`TestFactory.java:115`, `:191-205`, `Test.java:220`). 옮기지 않는다 |
| `ctltool/` (`runone.sh`·`prepare.sh`·`clean.sh`·`timeout3.sh`·`qactl`·`qacsql`) | **실행부 — 그대로 subprocess** (§2-4, ADR-007) |

### 2-1. 진입

| | |
|---|---|
| 게이트 | `TESTKIT_NATIVE` 가 `isolation` 을 이름으로 가질 때만 등록. 없으면 legacy |
| conf | `cli.Task.Suite()` 의 `isolation.conf`, `-c` 우선 |
| 종료 코드 | **0** — 실패 케이스가 있어도. **255** — 빌드 정보를 못 읽음, `CheckRequirement` 실패 (`System.exit(-1)`, `Main.java:67-71`, `TestFactory.java:310-314`). **scenario 디렉터리가 없으면 CTP 는 `[ERROR]` 를 찍고 0 으로 끝난다** (`Main.java:80-85` 의 `return`). 종료 코드는 동결 표면이라 같게 0, 이유는 같은 `[ERROR]` 한 줄로 |

### 2-2. 단계 — `TestFactory.execute` 대응 (새 run)

| CTP | 하는 일 | 신 |
|---|---|---|
| `Main.exec` | env 목록(없으면 `local`), 빌드 id·bits (`cubrid_rel` 경유), `calcScenario` | `topology` · `buildinfo` |
| `cleanFilesByDirectory` | `current_runtime_logs` 비움 | `result.Open` |
| `createSnapshotForConfiguration` | `main_snapshot.properties` — conf 와 **JVM 시스템 속성**, `AUTO_BUILD_ID`·`AUTO_BUILD_BITS` | `Sink.Snapshot` (키 이름이 shell 의 `AUTO_TEST_*` 와 다르다) |
| `checkRequirement` | `check_<env>.log`, 실패 시 exit -1 | §2-5 |
| `UPDATE TEST CASES` (`TestCaseGithub.update`) | 프로세스 정리, `upgrade.sh`(CTP 자기 갱신 — 로컬이면 건너뛰지만 **환경 변수 전체를 표준 출력에 찍는다**), git pull(설정 시), 그리고 **ctltool 에서 `chmod u+x *.sh`** — 스크립트가 저장소에 `100644` 로 있어, 이것 없이는 모든 케이스가 `timeout3.sh: Permission denied` (측정: 58/58) | 자기 갱신·pull 은 제외. chmod 는 슬롯 준비에서, 슬롯의 ctltool 오버레이 안에 |
| `Dispatch.init` | `find <scenario> -name "*.ctl" -type f -print`, 제외 파일(엔트리마다 **처음 일치하는 케이스 하나만** 뺀다 — `Dispatch.java:126-141`), `dispatch_tc_ALL.txt` | 같은 규칙, 정렬 |
| `setTotalTestCase` · `addSkippedTestCases` | 건너뛴 케이스를 elapse `-1` 로 보고 | 같게 |
| `DEPLOY` | `cubrid service stop` + `$USER` 의 `cub_admin`·`cub_master`·`cub_server` `kill -9` (`Constants.java:74-81`). 빌드 major ≥ 10 이면 **`echo inquire_on_exit=3 >> $CUBRID/conf/cubrid.conf` — 매 run 누적** (`DeployOneNode.java:75-79`, 두 run 뒤 두 줄 측정). conf 섹션을 `ini.sh` 로 | 같게, 슬롯의 `$CUBRID` 오버레이 안에서 |
| `TEST` | 케이스마다 §2-3, 끝나면 `cubrid service stop` 을 워커 로그에 | 같게 |
| `onTaskStopEvent` · `backupTestResults` | 요약, `TEST COMPLETE`, tar | §2-5 |

continue 모드(`test_continue_yn`)는 `dispatch_tc_ALL.txt` 에서 `dispatch_tc_FIN_*` 를 뺀다 — shell 과 같다.

### 2-3. 케이스 한 건 — `runone.sh`

Java 는 케이스마다 새 셸에서 이것 하나를 보낸다 (`Test.java:174-192`, `IsolationScriptInput.java:35-36`):

```
export ctlpath=${CTP_HOME}/isolation/ctltool; export PATH=${ctlpath}:$PATH
ulimit -c unlimited; export TEST_ID=0; cd $ctlpath
sh runone.sh [-n] -r <testcase_retry_num+1> <case.ctl> <testcase_timeout_in_secs> qacsql 2>&1
```

마지막 인자는 DB 이름이 아니라 **클라이언트 프로그램**이다 — `cubrid_testdb_name`(기본 `cubrid`)이 `DB_TEST_MAP` 으로
`qacsql` 이 된다. DB 는 `runone.sh` 가 `ctldb` 로 박아 두었다 (`runone.sh:122`).

`runone.sh` 가 한 번의 시도에서 하는 일 (`runone.sh:239-392`):

| | |
|---|---|
| 앞 정리 | `$ctlpath`·`$CUBRID`·케이스 디렉터리의 `core.*` 삭제, `~/CUBRID/log` 아래 파일 비우기 — **`$CUBRID` 가 아니라 `~/CUBRID`** |
| 셋업 (필요할 때만) | `qactl` 이 없거나 `ctldb` 서버가 안 떠 있으면 `prepare.sh`: `cubrid service stop`, **`pkill -9 -u $(whoami) cub`**, `deletedb`, `$CUBRID/databases/ctldb` 에 `createdb ctldb en_US --db-volume-size=50M --log-volume-size=50M`, `server start`, **`make clean qactl qacsql` — CTP 트리 안에서** |
| 사전 SQL | `<name>.sql` 이 있으면 csql 로 (코퍼스에 0개) |
| 실행 | `timeout3.sh -t <timeout> qactl ctldb <case> qacsql > result/<name>.result 2>&1` |
| 정규화 | `result/<name>.result` → `result/<name>.log` 로 복사 후 sed 15단계 (`runone.sh:48-73`) — F1 |
| 판정 | `<name>.sh` 가 있으면 그것이 판정(코퍼스에 0개). 아니면 `answer/<name>.answer*` 를 `ls` 순서로, **처음 같은 것이 이긴다** |
| 기록 | OK: `flag: OK`, `<casedir>/<name>.result`. NOK: `flag: NOK`, `result/<name>.result` 에 `diff -y` |
| 코어 (`-n` 아니면) | `core.*` 나 `$CUBRID/log/*` 의 `FATAL ERROR` → 코어를 `~/error_backup` 으로, `cubrid service stop`, `pkill cub`, **`$CUBRID` 통째 복사**, tar, `flag: NOK found core file on host …`, `prepare.sh` 로 DB 재생성 |
| 뒤 정리 | **`pkill -u $(whoami) -9 sleep`**, `clean.sh`: 사용자의 `qactl`·`qacsql` `pkill -9`, `$HOSTNAME` 의 `tranlist` `kill -9`, 사용자·FK 테이블·트리거·serial·함수(`sleep`·`sleep1`·`sleep2` 제외)·프로시저·뷰·테이블을 `csql -u dba` 왕복으로 drop |

재시도: 시도마다 `Testing <case> (retry count: i)` 를 `.test.log`(cwd)에, `flag: OK` 가 나오면 멈춘다 (`runone.sh:395-407`).

**DB 는 run 에 하나다.** 케이스 사이의 격리는 `clean.sh` 의 drop 이 전부이고, DB 를 새로 만드는 것은 서버가 죽었거나
코어가 난 뒤뿐이다. 슬롯은 각자 `ctldb` 를 갖는다.

**판정 (Java 쪽, `Test.java:195-218`)**: 마지막 `flag: NOK` 가 마지막 `flag: OK` 뒤면 실패. `found core file`·`found fatal error`
가 한 번이라도 있으면 실패 + `hasCore`. `flag: OK` 가 아예 없으면 실패, `Not found OK word.`. 실패하면 워커가
`diff -a -y -W 185 answer/<name>.answer result/<name>.log` 를 feedback 에 붙인다 — **기본 answer 만**, `.answer1` 이
맞춰진 케이스라도 (`Test.java:148-172`).

### 2-4. 실행부 (ADR-007, 초안)

**`runone.sh` 와 ctltool 을 바꾸지 않고 케이스마다 부른다.** 러너는 그 표준 출력에서 §2-3 의 Java 규칙으로 판정한다.

이유는 판정이 이미 그 안에 있어서다. sed 15단계, answer 선택 순서, `<name>.sh`·`<name>.sql` 훅, 재시도, 코어 백업이 전부
`runone.sh` 이고, `qactl`·`qacsql` 은 CUBRID 의 C 클라이언트 API(`db_restart`, `local_tm_isblocked`, `lock_dump`)로
락 대기를 본다 — Go 드라이버가 주지 않는 것이다. 6,865 개 answer 는 이 조합이 낸 바이트다. 흡수·재구현은 ADR-007 의 대안.

실행부가 하는 위험한 일은 슬롯이 가둔다:

| 하는 일 | 가두는 것 |
|---|---|
| 사용자의 `cub`·`sleep`·`qactl`·`qacsql` `pkill -9`, `tranlist` `kill -9`, `cubrid service stop` | PID·IPC·네트워크 namespace — 슬롯 안의 `whoami` 는 슬롯의 프로세스만 가진다 |
| `$CUBRID/databases/ctldb`, `$CUBRID/log`, `inquire_on_exit=3` 추가 | 슬롯의 `$CUBRID` 오버레이 |
| `make clean qactl qacsql`, cwd 의 `.test.log`·`runone.log`·`timeout.log`·`csql.err`·`deleteuser.sql` … | 슬롯마다 ctltool 디렉터리 — `$CTP_HOME/isolation/ctltool` 오버레이 |
| 케이스 트리의 `result/<name>.{result,log}`, `<name>.result` | 코퍼스 오버레이 (`scenario_disk` / `scenario_ram_mb`) |
| `~/error_backup`, `~/CUBRID/log` | 슬롯마다 자기 `~/error_backup` — 끝나면 슬롯이 남긴 것을 기계의 `~/error_backup` 으로 복사, 이름이 겹치면 슬롯 이름을 붙인다. 테스트 대상 설치가 `~/CUBRID` 가 아니면 슬롯에는 빈 `~/CUBRID/log` *(2026-09-15)* |

### 2-5. 기록 (동결 출력)

run 디렉터리의 파일은 **일곱 개**다 — shell 에 있는 `current_task_id`·`monitor_<env>.log`·`test-<category>.xml` 이 없다
(측정, `isolation-baseline.md` §2).

| 파일 | 내용 | shell 과 다른 점 | 비교 등급 (안) |
|---|---|---|---|
| `check_local.log` | 변수 `JAVA_HOME`·`CTP_HOME`·`CUBRID`, 명령 `java`·`diff`·`wget`·`find`·`cat`, 디렉터리 scenario·`${CTP_HOME}/isolation/ctltool` | 목록, 그리고 `==> Check ssh connection ` 문구 (`CheckRequirement.java:51-88`) | 엄격 |
| `dispatch_tc_ALL.txt` · `dispatch_tc_FIN_local.txt` | 케이스 절대경로 | — | 엄격 |
| `test_status.data` | 다섯 카운터 | — | 엄격 |
| `feedback.log` | `[TASK START] Current Time is <date>` · `[OK] <case> <ms> EnvId=local[local]` + 결과 텍스트 + 빈 줄 · 건너뜀은 `[SKIP_BY_BUG] <case> -1 ` · 요약 · `[TEST STOP]`·`Elapse Time:<ms>` | 머리에 콜론이 없다(`[OK]` vs `[OK]: `), `MSG Id` 없음, `[Task Id]` 없음 (`FeedbackFile.java:63-120`) | baseline |
| `test_local.log` | Deploy 의 출력, `[TESTCASE] <case>`, **`runone.sh` 의 `set -x` 추적 전체**, 결과 항목, `Stop service for EnvId=local[local]]` | 58 케이스에 7,497 줄 — 거의 추적 | baseline |
| `main_snapshot.properties` | conf + JVM 시스템 속성 + `AUTO_BUILD_ID`·`AUTO_BUILD_BITS` | 키 이름 | baseline |

| 그 밖 | |
|---|---|
| 표준 출력 | CTP 배너, `Available Env:`·`Build Id:`·`Build Bits:`, 단계 제목, `[ENV START]`, `[TESTCASE] <case> EnvId=local [OK]` — **재시도 접미사가 없다**, `[ENV STOP]`, 요약, `TEST COMPLETE` |
| tar | `isolation_result_<build>_<bits>_0_<Y.M.D_h.m.s>.tar.gz`, 12시간제 — shell 과 같은 버릇. **`cd <dir>; tar zvcf ../<name> .`** 이라 경로가 상대다 (`TestFactory.java:146`) — shell 은 디렉터리 경로를 담는다 | F2 |
| 케이스 트리 | `result/<name>.result`, `result/<name>.log`, `<name>.result` — 케이스마다 셋 | 판정 동일성의 재료 |

공통 코드에 필요한 변경은 둘이다: `feedback.OpenIsolation`(방언)과 `Sink.BackupIsolation`(이름 접두어와 상대 경로). check 는 목록·순서·문구가 모두 달라 `isolationsuite` 에 따로 둔다.

### 2-6. 동등성 (ADR-018, 초안)

**측정 (2026-09-15, `isolation-baseline.md` §4):** CTP 는 같은 순서로 전체를 두 번 돌려 판정 7개를 옮긴다. 세 run
에서 판정이 움직인 10 케이스를 두 러너로 따로 세 번씩 돌리면, 다섯은 각 러너 안에서 run 마다 뒤집히고 다섯은 둘 다
혼자서는 늘 통과한다 — **러너를 가르는 케이스는 없다.** 그래서 한 번의 CTP run 과 판정 diff 0 은 CTP 자신도 얻지
못하는 등급이고, ADR-018 은 "CTP 가 재현하는 판정은 같아야 한다"로 정의한다. 아래는 그 전의 안이다.

ADR-013 을 isolation 에 맞춘다. **엄격** — baseline 없이 diff 0: `dispatch_tc_ALL.txt`·`dispatch_tc_FIN_local.txt`·
`test_status.data`·`check_local.log`, 그리고 **케이스별 판정**과 **케이스별 `result/<name>.log`** (정규화된 결과). 나머지
셋은 분류. 실행부가 같은 코드라 `result/<name>.log` 는 같아야 하지만, 락 대기와 `sleep` 에 기대는 케이스는 부하에 흔들릴
수 있다 — **CTP 두 run 의 차이(noise floor)를 먼저 잰다** (ADR-013 "measured before judged").

---

## 3. 향상 (축 B)

| 항목 | 이기는 것 (오늘) | 증거 | 끄는 법 |
|---|---|---|---|
| **컨테이닝** | `pkill -9 -u $(whoami)` 가 사용자의 다른 CUBRID·`sleep` 을 죽인다 | 개발자가 일하는 기계에서 전체 run, 바깥 프로세스 무사 | 없음 — §0 |
| **깨끗한 코퍼스** | 케이스 트리에 케이스당 세 파일, 6,772 케이스면 2만 개 | run 뒤 체크아웃이 `git status` 로 깨끗 | `scenario_disk` / `scenario_ram_mb` |
| **깨끗한 CTP 트리** | run 마다 `make clean`, 로그가 `$CTP_HOME` 안에 쌓인다 | run 뒤 CTP 트리 불변 | 슬롯의 ctltool 오버레이 |
| **병렬 슬롯** | 직렬 — 전체 12,301 s. **측정 (2026-09-15):** 슬롯 4개 2,978 s, ADR-018 규칙으로 러너 차이 0. 슬롯마다 `ctldb` 이력이 달라 카탈로그 행 순서에 기대는 케이스 4개가 드러났고, 넷 다 혼자서는 두 러너 모두 통과 (`isolation-baseline.md` §4). **기본값 (2026-09-15):** `parallel_slots` 가 없으면 4, CPU 수와 (가용 메모리 − 2 GB) / 1.5 GB 로 줄인다 — 표본 슬롯 4개 최고 2,813 MB (§2) | 슬롯 1 과 N 의 판정·`result/<name>.log` 동일. wall | `parallel_slots=1` |
| **셋업 한 번** | 슬롯마다 `createdb` + `make` | 슬롯 수와 무관한 셋업 시간 | 병렬을 끄면 없음 |
| **보드** | 진행이 로그로만 | 케이스·슬롯·실패가 실시간 | `status_http` |

**`inquire_on_exit=3` 누적은 고치지 않는다.** 같은 키가 여러 줄이면 마지막이 이기고 값이 같다 — 틀린 결과를 믿게 하지
않는다 (`spec-corrections.md` 의 규칙). 슬롯의 오버레이가 run 마다 버려지니 누적 자체가 사라진다.

**후보 (게이트 뒤, 실행부를 바꾸는 것):** `clean.sh` 는 케이스마다 `csql -u dba` 를 열일곱 번 연다. DB 를 스냅숏에서
되돌리면 빠를 수 있지만, 그것은 실행부의 행동을 바꾸고 케이스 사이에 남는 것을 바꾼다 — `runone.sh` 를 오라클로 두고
잰 뒤에 정한다.

---

## 4. 문서화

`docs/category/isolation/` — shell·sql 과 같은 뼈대: README(한 장 그림, 가장 짧은 run), 1. How a run works, 2. Writing a
case (`.ctl` 문법·`answer/`·다중 answer·sleep 은 초), 3. Running it, 4. Configuration, 5. When a case fails.

evidence: `evidence/isolation-baseline.md` (P0) → `evidence/regression-isolation.md` (게이트).

---

## 5. 순서

| | 내용 | 끝 |
|---|---|---|
| **P0** | 샌드박스 기준 run (세 저장소 upstream develop), 코퍼스 census, 분석 공백 채우기, ADR-007, 이 문서 | `evidence/isolation-baseline.md` — CTP 전체 run 과 noise floor |
| **P1** | 공통 코드 두 곳(§2-5) → **슬롯 위의 `isolationsuite`** + `runone.sh` 실행부, `TESTKIT_NATIVE=isolation` 뒤 | 슬롯 1개: 표본·전체가 CTP 와 동일. **진행 (2026-09-15):** 코드 완료. 표본 60 케이스, 슬롯 1개 — 판정·동결 파일·`feedback.log`·`result/<name>.log` 58/58 이 CTP 와 동일 (`isolation-baseline.md` §2). 전체는 CTP 기준 run 뒤 |
| **게이트** | ADR-018 확정, 전체 코퍼스 슬롯 1개로 CTP 와 비교 | ADR-018 의 규칙 → 병렬 기본값 on 가능. **초안의 규칙을 현재 데이터에 적용하면 (2026-09-15):** 러너 파일 동일, 러너 차이 0, 불안정 10, 늘 실패 8. 슬롯 4개도 러너 차이 0 — 네 run 에 걸쳐 불안정 15, 늘 실패 7. **ADR-018 확정 (2026-09-15), 게이트 충족** |
| **P2** | 병렬 증거, 축 B 나머지, as-built 가이드 | 항목별 증거. **진행 (2026-09-15):** 슬롯 4개 전체 코퍼스 2,978 s, 러너 차이 0 → **병렬 기본값 on** (사용자 결정). 가이드 `docs/category/isolation/` |

---

## 6. 미결

| 항목 | 어디로 |
|---|---|
| ~~`~/error_backup` · `~/CUBRID/log` — 슬롯이 `$HOME` 을 공유한다~~ | **닫힘 (2026-09-15)** — `HOME` 을 통째로 바꾸지 않았다: 프로파일·`cd`·scenario 의 상대 경로가 모두 `$HOME` 이다. 두 경로만 슬롯마다 bind 한다 (§2-4) |
| 락 대기·`sleep` 케이스의 부하 민감도 — `load average` 16 인 기계에서 CTP 자신이 흔들리는가 | P0 noise floor |
| `hostname -i` (코어 보고), `$HOSTNAME` (`clean.sh` 의 `tranlist` 필터) 가 네트워크 namespace 안에서 | P1 — 코어 경로와 함께 확인 |
| ADR-008 (`.ctl` grammar) · ADR-009 (정규화 정형화) | 실행부를 그대로 쓰는 동안 필요 없다. 실행부를 Go 로 옮길 때 — P2 이후 |
| 오타 answer (`.asnwer` 2, `.amswer` 1) 와 `compare.log` — `answer*` glob 에 안 걸려 **읽히지 않는다**. 해당 케이스는 다른 answer 가 있다 | upstream 에 알릴 것 |
| 다중 answer 의 의미 — 59 케이스, `ls` 순서의 첫 일치 | 가이드(2장)에 적는다 |
