# Module: sql — 2차 대체 대상 (sql · medium)

- **Date:** 2026-09-02 (매핑 수준) · **2026-09-11 (설계로 확장 — Phase 4 착수)**
- **Status:** Draft — P0(측정·문서) 진행 중. 확정되는 결정은 ADR 로 올린다
- **담당 task:** `sql` · `medium`. `kcc` · `neis05` · `neis08` · `sql_by_cci` · `webconsole` 은 **legacy Runner 유지**
- **관련:** ADR-016 (실행부) · ADR-017 (sql 동등성, 예약) · ADR-006 (DB 셋업 레시피, 예약) · ADR-015 (축 B) ·
  `evidence/sql-baseline.md` · `module-medium.md`
- **Inputs:** `analysis/sql/*` · `analysis/medium/*` · CTP 소스 직접 확인분 (§2 의 행 번호는 전부 `cubrid-testtools` develop `a1bec87` 의 `CTP/sql/src/com/navercorp/cubridqa/cqt/` 기준)

---

## 0. 세 갈래, 그리고 순서

| 갈래 | 무엇 | 끝나는 조건 |
|---|---|---|
| **호환 (축 T)** | CTP 가 sql·medium 을 돌리는 능력을 그대로 — 단계, 판정, 동결 출력, 종료 코드 | ADR-017 의 게이트: CTP 와 testkit 을 같은 코퍼스에 돌려 정규화 후 diff 0 |
| **향상 (축 B)** | shell 에서 만든 기능·성능·사용성을 **공통 모듈로 빼서** 적용 | 항목마다 스스로 선언한 증거 (ADR-015 기준 3) |
| **문서화** | shell 수준의 as-built 가이드 `docs/category/sql/` 와 evidence | 가이드가 코드와 어긋나지 않음 |

**목표는 sql 과 medium 을 병렬로 돌리는 것이다** (2026-09-11 사용자). 그래서 병렬은 나중에 붙는 향상
항목이 아니라 **처음부터 러너의 구조**다 — `sqlsuite` 는 shell 의 슬롯 위에 짓고, 슬롯 1개로 돈 것이 호환
게이트, N개로 돈 것이 병렬의 증거가 된다 (§3).

**켜는 순서는 ADR-015 기준 2 가 정한다.** 병렬은 코드로는 처음부터 있지만 **기본값은 off** 이고, 호환
게이트가 통과하기 전에는 켜지지 않는다. shell 의 슬롯·컨테이닝이 지금 그런 상태인 것과 같다.

**Phase 순서의 이탈을 기록한다.** ROADMAP 은 Phase 4 를 Phase 3 게이트(shell 전체 코퍼스) 뒤에 둔다.
2026-09-11 사용자 결정으로 병행한다. 자원이 충돌하면 ROADMAP §8 의 규칙대로 shell 게이트가 먼저다.

---

## 1. 범위

| | 이번 | 나중 |
|---|---|---|
| suite | `sql` · `medium` (`medium_dev` 포함) | `kcc` · `neis05` · `neis08` — 코퍼스 미상 (`analysis/sql/test-corpus.md`) |
| 인터페이스 | **JDBC 모드** — `jdbc` 실행부 | **CCI 모드** — `native` 실행부 (ADR-016). `sql_by_cci` task 도 그때 |
| interactive | legacy 로 넘긴다 | 동결 대상은 행동이지 메커니즘이 아니다 (`architecture.md` §5) |
| `enable_memory_leak` | legacy 로 넘긴다 (`run_memory.sh`, valgrind) | — |

**medium 은 별도 Runner 가 아니다** (`module-medium.md` §1). 코드 경로가 같고 `test_category` 와 DB 셋업만
다르다. `runner/sqlsuite` 가 `Tasks() = {sql, medium}` 으로 둘 다 받는다.

---

## 2. 호환 (축 T)

### 2-0. 구 ↔ 신 매핑

2026-09-02 의 매핑 표를 설계에 맞춰 갱신했다. 바뀐 행은 날짜를 붙인다.

| 구 | 신 |
|---|---|
| `sql/bin/run.sh` (917줄) — 6단계 파이프라인 | `runner/sqlsuite` (§2-2) |
| `ConsoleAgent` · `ConsoleBO` 의 탐색 · answer 선택 · 비교 · 기록 | `runner/sqlsuite` — Go |
| `ConsoleBO.executeSqlFile` · `SQLParser` · `ConsoleDAO` — 실행과 렌더링 | **실행부** (§2-4, ADR-016). `jdbc` 는 이 클래스들을 그대로 부르고, `native` 는 Go 로 옮긴다 *(2026-09-11)* |
| `--+ holdcas` · `--@queryplan` · `@conn:` · `$type,val` 바인드 | 실행부. `jdbc` 는 `SQLParser` 그대로, `native` 는 Go 파서가 같은 규칙으로 *(2026-09-11 — 2026-09-02 는 `caseformat/sql` 에 두었다)* |
| URL 형태 지정 `<repo>?db=X&filter=Y` | `sqlsuite` 가 conf 에서 직접 읽는다. URL 형태는 legacy 경로에만 남는다 |
| `sql_by_cci/ccqt` (C 바이너리) | ~~subprocess 유지~~ → **`native` 실행부 (ADR-016, 2026-09-11)** |
| `cqt.webconsole.*` | **축 O — 제외.** 진입점만 F1 로 두고 기존 자산 호출 |
| `common.coreanalyzer.AnalyzerMain` | `coreanalyze` (축 T — `CORE_FILE:` 가 F1) |

### 2-1. 진입

| | |
|---|---|
| 게이트 | `TESTKIT_NATIVE_SQL=1` 일 때만 sqlsuite 가 등록된다. 없으면 지금처럼 legacy (`cmd/testkit/main.go` 의 등록 순서) |
| conf | `cli.Task.Suite()` 가 이미 `sql.conf` / `medium.conf` 를 고른다. `-c` 우선 |
| 종료 코드 | **0** — 실패 케이스가 있어도. **1** — `$CUBRID` 없음, conf 없음 (freeze §6-1). shell 의 `quit()`(255)를 쓰지 않는다 |
| 넘기는 경우 | `sql_interface_type=cci`, `--interactive`, `enable_memory_leak=yes` → legacy Runner 로 위임. 조용히 무시하지 않는다 |

### 2-2. 단계 — `sql/bin/run.sh` 대응

`run.sh` 는 6단계를 게이트 없이 잇는다 (`analysis/sql/design.md` §2). 각 단계의 새 주인:

| run.sh | 하는 일 | 신 |
|---|---|---|
| `do_init` | DB 이름 `basic` (medium 은 `mdb`), 카테고리 | `sqlsuite` 설정 해석 |
| `do_clean` | DB 정지·삭제, `pkill cub`, `ipcrm` | 같은 스크립트. 컨테이닝(§3) 안에서는 자기 것만 죽는다 |
| `do_configure` | charset — **11.5 이상에서 sql 은 `en_US.utf8` 강제** (`run.sh:688-694`). conf 섹션을 `$CUBRID/conf` 에 기록. 첫 `SERVICE=ON` 브로커 포트를 `Function_Db/<db>_qa.xml` 에 기록 | `ini.sh` 경유는 유지. 브로커 포트는 실행부 연결 설정으로 넘긴다 |
| `do_create_db` | `createdb` (+`cubrid_createdb_opts`), `need_make_locale`, **sql:** `make_sql_db_data` (저장 프로시저 javac + `loadjava`) / **medium:** `make_db_data` (`mdb.tar.gz` 풀기 + `loaddb` + `optimizedb`) | ADR-006 (예약) 으로 정형화. `tar` 파일명 하드코딩 버그는 고친다 (NG9 위반 아님 — 출력 표면이 아니다) |
| `do_test` | `ConsoleAgent runCQT …` | **실행부 (ADR-016)** + Go 의 탐색·선택·비교·기록 |
| `do_summary_and_clean` | `main.info` 읽기, core 수색 (`CORE_FILE:`), DB 정리 | `sqlsuite` |

**medium 은 11.x 에서 `create_table_reuseoid=no` 가 없으면 적재가 깨진다** (2026-09-11 측정,
`evidence/sql-baseline.md` §3). 스키마의 `dba.picture` 가 REUSE_OID 테이블이 되어 참조 도메인이 될 수
없고, `loaddb` 가 스키마 단계에서 멈춘다. 그 뒤 975 중 579 가 `Error:-493/-494` 로 실패한다. `medium_dev.conf`
가 그 한 키를 가진 이유이며 파일에도 "It needs to CUBRID 11.x over" 라고 적혀 있다. 러너는 이 키를
만들어 넣지 않는다 — conf 의 선택이다. 대신 적재 실패를 **run 을 멈추는 사실로** 보고한다
(CTP 는 `loaddb` 의 출력을 로그 파일로만 보내고 계속 간다).

### 2-3. 케이스

| | CTP 의 규칙 (확인한 소스) | 신 |
|---|---|---|
| 탐색 | 확장자 `.sql` 하나 (`TestUtil.java:112`, `getCaseFilePostFix` `:342`). **medium 의 `.api` 10개는 케이스가 아니다** — `module-medium.md` §3 의 미결을 닫는다 | 같은 규칙. 정렬 (B-T1 과 같은 이유) |
| 제외 | `filter=` 파일의 줄마다, 시나리오 루트 기준 상대 경로에 `containPath` (`TestUtil.java:371-391`). **파일이 없으면 스택트레이스를 찍고 제외 없이 진행** (`:350-363`) | **같게.** shell 은 없는 제외 파일을 실패로 바꿨지만(2026-09-11) sql 은 그러면 안 된다 — 모든 conf 의 기본값 `${CTP_HOME}/conf/exclusions.txt` 가 **실제로 없다** |
| answer | `answers/<name>.answer`. `<run_mode>` 가 있으면 `_<run_mode>` → `_<run_mode_secondary>` → 기본 순 (`ConsoleBO.java:368-396`). `run_mode` 는 `jdbc_config_file` XML 의 `<run_mode>` 요소 — **freeze §11-7 을 닫는다.** `test_default.xml` 은 주석 처리되어 기본 run 은 기본 answer 만 쓴다. `_D_<db charset>_C_<collation>` 237개는 `test_D_*.xml` 들과 짝이다 | 같게 |
| answer 없음 | 실행하지 않는다 (`shouldRun=false`, `ConsoleBO.java:373-381`). 루프는 그 케이스를 **실패로 센다** (`processMonitor.setFailedFile`, `:474-478`). develop 코퍼스에는 해당 케이스가 없다 — `total` = `execute_case` = 17,459 | 같게 |
| 순서 | 한 JVM, 한 연결, 케이스를 차례로 (`ConsoleBO.java:482`) | 직렬 모드는 같게. 병렬은 §3 |

### 2-4. 실행부 (ADR-016)

```
type Executor interface {
    Open(ctx, conn ConnSettings) error              // run 동안 살아 있는 연결 — CQT 와 같다
    Run(ctx, caseFile string) (Rendered, error)     // 문장마다 렌더링된 텍스트
    Close() error
}
```

| 구현 | 어떻게 | 비고 |
|---|---|---|
| **`jdbc`** | CTP 트리의 `cubridqa-cqt.jar` 를 classpath 에 두고, CQT 와 **같은 XML 설정**으로 `Test` 를 만든 뒤, 케이스 경로를 받을 때마다 `ConsoleBO.executeSqlFile` 을 호출하는 작은 Java 프로세스. `executeSqlFile` 은 private 이라 리플렉션 — JDK 8 이라 막히지 않는다. **스파이크 (2026-09-11, `evidence/sql-baseline.md` §9)**: medium 975/975, sql 표본 1,658/1,658 의 `.result` 가 CTP 와 바이트 동일. CQT 가 non-daemon 스레드를 남기므로 `System.exit` 로 끝내야 하고, 작업 디렉터리는 CQT 처럼 `sql/lib` | 파싱·연결 재사용·리셋·렌더링이 **CQT 의 코드 그 자체**. 문장 경계는 렌더링의 51개 `=` 구분자로 가른다 |
| **`native`** | Go CAS 클라이언트 + 모드별 포매터. CCI 모드의 기대 출력은 `ccqt` 의 C 렌더링(`execute.c:266-510`)이고, `.answer_cci` 가 있으면 그것이 이긴다 | 2단계. CAS 클라이언트는 `cubrid-labs/cubrid-go`(MIT) 확장 vs `internal/cas` — P0 스파이크 뒤 결정 |

**비교는 Go 에 둔다.** `\r`·`\n` 을 양쪽에서 모두 지우고 같으면 통과 (`ConsoleBO.java:633-654`). CCI 모드의
`ccqt` 는 바이트 비교다 (`execute.c:2548-2581`) — 모드마다 비교 규칙도 다르다는 뜻이고, 둘 다 재현한다.

### 2-5. 기록 (동결 출력)

| 표면 | 내용 | 등급 |
|---|---|---|
| stdout | `Result Root Dir:` · 진행 줄 `[HH:MM:SS] Testing <case> (i/N p%) [OK\|NOK]` · `Fail/Success/Total` · `Testing End!` 등 (freeze §4). **run 이 띄운 서버가 이 stdout 을 물려받는다** — 세 번 중 한 번 `*** XASL generation failed ***` 가 진행 줄과 `[OK]` 사이에 끼어 판정이 다음 줄로 밀렸다 (2026-09-11, `evidence/sql-baseline.md` §6). 신 러너가 서버 출력을 다른 곳으로 돌리면 이 끼어듦이 사라지지만 그것도 표면의 변화다 — 게이트 전에는 CTP 와 같게 물려준다 | F1 |
| 결과 루트 | `$CTP_HOME/sql/result/y<Y>/m<M>/schedule_linux_<cat>_64bit_<ddHHmmss+난수>_<build>` | 경로 형태 F1, 시각·난수는 마스킹 |
| `main.info` | `key:value` — CQT 가 쓰는 12키(build, version, os, category, elapse_time, success, fail, total, execute_case, totalTime, end_time, result_path) 뒤에 `do_summary_and_clean` 이 붙이는 3키(cubrid_rel, user, machine — `run.sh:853-855`). freeze §5-2 의 키 목록은 이것과 다르다 (spec-corrections §7) | F1 |
| `summary_info` | **이름이 같은 파일이 둘이다** (2026-09-11 확인). JDBC 모드는 CQT 의 것 — `key:value`, 루트와 디렉터리마다 (total, success, fail, totalTime, SiteRunTimes). CCI 모드는 `run.sh` 의 `generate_summary_info` 가 쓰는 것 — `key=value` (freeze §5-3, `run.sh:804-850`) | F1 |
| `summary.info` · `summary.xml` | XStream XML · 케이스별 `<scenario><case><answer><elapsetime><result>` | F2 (P0 에서 확정) |
| 케이스 옆 `.result` | **코퍼스 트리 안에** 쓴다 — medium 한 번 돌린 뒤 사본 트리에 975개 (2026-09-11 확인). 실패 시 결과 디렉터리에 `.sql`·`.answer`·`.result` 사본 (`TestUtil.java:715-740`) | 경로 F1 |
| `CORE_FILE:` | `find $CUBRID $CTP_HOME -name core*` 결과 | F1 |
| 종료 코드 | §2-1 | F1 |

### 2-6. 공존 기간 (freeze §8)

sqlsuite 가 넘기는 경우(§2-1)와 게이트 전의 기본 경로는 legacy Runner 다. freeze §8 은 legacy 가
아래 argv/env 를 **바이트 단위로** 재현하길 요구한다.

```
sh $CTP_HOME/sql/bin/run.sh -s <suite> -f <conf>
  + export sql_interface_type=cci        (CCI 모드)
  + export sql_interactive=yes           (interactive)
  + export log_file_in_interactive=<path>
java cqt.console.ConsoleAgent runCQT <type> <typeAlias> <version> <charset_xml> <files...>
$CTP_HOME/sql_by_cci/ccqt <port> <db> <alias> <resultFolder> <repo> <CTP_HOME> <cci_urlproperty>
```

지금의 legacy Runner 는 이것을 스스로 조립하지 않고 CTP 의 Java 진입점(`CTP sql -c …`)에 통째로 넘긴다.
그러면 CTP 가 위 argv 를 만들므로 요구는 **위임으로 충족**된다 — 단 `architecture.md` §5-1 의 서술("재현한다")과는
다른 방식이라 여기 적어 둔다.

### 2-7. 동등성 (ADR-017, 예약)

ADR-013 을 sql 에 맞춘다. 코퍼스는 sql 17,459 · medium 975 (CQT 가 세는 수 — `evidence/sql-baseline.md`).
정규화는 시각·경로·난수·elapse 를 마스킹하고, **판정을 싣는 파일은 baseline 없이 diff 0**:
`main.info`(시각 제외) · `summary_info` · 케이스별 판정. 같은 러너 두 번의 차이를 noise floor 로 먼저 잰다
(ADR-013 의 "measured before judged").

---

## 3. 향상 (축 B) — shell 에서 가져오는 것

### 3-1. 공통 모듈: 필요할 때 뺀다

sqlsuite 가 처음 쓰는 시점에 `shellsuite` 에서 공용 패키지로 옮기고, 옮길 때마다 **shell 테스트가 그대로
통과하는 것**으로 동작 불변을 확인한다. 미리 한꺼번에 설계하지 않는다 — 두 번째 사용자가 생길 때가 경계를
긋기에 가장 싸고 가장 정확하다.

| 지금 | 쓰는 곳 | 옮길 곳 (안) | 시점 |
|---|---|---|---|
| `exec` · `conf` · `topology` · `registry` · `plan` · `dispatch` · `contain` | 그대로 | — | — |
| `runIn`/`probeIn`/`exitError` (`prologue.go`) | 단계 스크립트 | ~~`internal/script`~~ → **`exec.Check` · `exec.Result.Failure`** *(2026-09-11 완료)*. prologue 는 shell task 의 환경이라 shellsuite 에 남는다 | P1 ✓ |
| `safepath` · `buildinfo` · 제외 파싱 | 단계 · 탐색 | **옮기지 않는다** *(2026-09-11)*. 규칙이 suite 마다 다르다 — 버전은 shell 이 CTP Java 의 규칙, sql 은 `run.sh` 의 awk 이고 커밋 접미사가 없는 빌드에서 갈린다. 제외 파일이 없을 때 shell 은 실패, sql 은 제외 없음. sql 은 디렉터리를 비우지 않아 `safepath` 가 필요 없다 | — |
| `openSlots` · `slotTmp` · `channelPair` · 입장 정책 배선 | 병렬 | ~~`internal/slots`~~ → **`contain.OpenSlots` · `contain.Slot`** *(2026-09-11 완료)*. shell 의 레인별 코퍼스 오버레이는 mount 훅으로 남는다 | P1 ✓ |
| `Worker` 골격 (claim · finish · 보고) · `Monitor` | 병렬 · 타임아웃 | **옮기지 않는다** *(2026-09-11)*. 공유할 부분(큐·입장·affinity)은 이미 `dispatch` 에 있고 — `Queue.Affinity` 추가 — 남는 루프는 suite 마다 다르다: sql 은 케이스 사이 리셋·재시도·케이스 타임아웃이 없다 (CQT 에 없다) | — |
| `status.Board` 의 shell 전용 부분 (`familyOf`, `replay` 의 `\.sh` 정규식, 패치 표시) | 보드 | 일반화 | **일반화 없이 붙였다** *(2026-09-11)* — 핵심 API(`Begin`·`End`·`Expect`·`Lane`·`Setup`)가 이미 일반이고 `familyOf` 도 sql 경로를 그대로 묶는다. sql 은 `[sql] status_http` 로 켠다. shell 전용인 `Detail`(feedback.log)·`replay` 는 sql 에서 쓰지 않는다 |
| `result.Sink` | 기록 | **`result.SQL`** — sql 결과 트리는 shell 의 것과 모양이 달라 섹션이 아니라 타입 하나. "관찰되는 바이트는 전부 `result` 에서" 규칙은 그대로 | P1 |
| — | 슬롯 안에서 **오래 사는 프로세스** (실행부 JVM) | **`contain.Namespace.Command` · `Slot.Command`**, 그룹 kill 은 `exec.Command` *(2026-09-11 완료)* | P1 ✓ |

### 3-2. 적용 항목

각 항목은 `concept/beyond-axis.md` 에 등록하고 ADR-015 의 네 조건을 지킨다. 기준선 수치는
`evidence/sql-baseline.md`.

| 항목 | 이기는 것 (오늘) | 증거 | 끄는 법 |
|---|---|---|---|
| **컨테이닝** (B-T2 의 sql 판) | `do_clean` 의 `pkill cub`·`ipcrm` 이 사용자의 다른 CUBRID 까지 죽인다. 오늘 기준선도 namespace 래퍼 없이는 못 돌렸다 | 개발자가 일하는 기계에서 전체 run, namespace 안의 `ps -e` 는 run 뿐 | `TESTKIT_CONTAIN` |
| **깨끗한 코퍼스** (B-T12 의 sql 판) | CQT 는 케이스마다 `.result` 를 **testcases 체크아웃 안에** 쓴다 | run 뒤 체크아웃이 `git status` 로 깨끗 | `scenario_ram_mb` 와 같은 오버레이 |
| **병렬 슬롯** (B-T3 의 sql 판) | 한 JVM·한 연결·직렬 | 슬롯 1 과 N 의 판정이 케이스별로 **동일**. wall | `parallel_slots=1` |
| **셋업 한 번** (B-T8 의 sql 판) | 슬롯마다 `createdb`+로케일+SP 컴파일(스모크에서 약 40 s) | 슬롯 수와 무관한 셋업 시간 | 병렬을 끄면 없음 |
| **보드** (B-T11) | 진행은 로그로만 보인다 | 케이스·슬롯·실패가 실시간으로 보인다 | `status_http` |
| **플랜을 데이터로** (B-T7) | 플랜 비교가 숫자를 `?` 로 가려 `t1`·`t2` 가 같다 | 인덱스가 바뀐 플랜이 실패한다 | 두 번째 비교, 기본 off |
| **드라이버 비교** (§6a 후보) | 드라이버 차이는 `.answer_cci` 2,121 개로만 드러난다 | 같은 코퍼스를 `jdbc`·`native` 로 돌린 문장 단위 diff | 별도 모드 |

**슬롯 구조는 shell 의 것을 그대로 쓴다.** 슬롯마다 네트워크·PID·IPC·마운트 namespace (모든 슬롯이
1822/33120 을 그대로), `$CUBRID`·레지스트리 오버레이, 슬롯별 `CUBRID_TMP`, 큐와 입장 정책, `case_plan` 의
긴 것 먼저, 보드, 코퍼스 오버레이. 달라지는 것은 슬롯 위에 올라가는 네 곳뿐이다:

| | shell | sql |
|---|---|---|
| 셋업 | 케이스가 자기 DB 를 만든다 | 슬롯을 열기 **전에** 러너의 namespace 에서 한 번 — configure · `createdb` · 로케일 · SP 또는 `mdb` 적재. DB 는 `$CUBRID/databases/<db>` (`run.sh` `do_create_db`) 라 `$CUBRID` 오버레이의 lower layer 가 된다 |
| 슬롯을 열 때 | 없음 | 슬롯마다 자기 `cub_master`·서버·브로커를 띄우고, 오래 사는 `jdbc` 실행부(JVM 1개)를 슬롯 안에서 시작 |
| 케이스 1건 | 셸 스크립트 실행 → `.result` 판정 | 실행부에 케이스 경로 → 렌더링된 텍스트를 answer 와 비교 |
| 케이스 사이 | 프로세스 정리 + `$CUBRID` 복원 | 없다 — CQT 처럼 연결의 reset 만 |

그래서 공통 모듈 추출(§3-1)은 P1 의 첫 일이다: Worker 골격이 "케이스 1건"을 인터페이스로 받으면 두
러너가 같은 슬롯·큐·보드를 쓴다.

**병렬 설계의 요점 — 셋업은 lower layer 에서 한 번.** shell 은 케이스가 자기 DB 를 만들지만 sql 은 run 에
DB 하나를 모든 케이스가 공유한다. 그래서 슬롯을 열기 **전에** 러너 자신의 namespace 에서 `do_configure`·
`do_create_db` 를 한 번 하고, 슬롯은 그 설치·DB 를 lower layer 로 보는 오버레이를 받는다. 복사가 없고
(copy-on-write), 경로가 모든 슬롯에서 같아 `_vinf`·`_lginf`·`databases.txt` 를 고칠 필요도 없다 —
shell 이 `$CUBRID` 경로를 바꾸지 않은 이유(`beyond-axis.md` "A mount namespace per slot")가 여기서 또 이긴다.
각 슬롯은 자기 `cub_server` 와 브로커를 같은 포트로 띄운다 (네트워크 namespace).

**위험은 케이스 사이의 상태다.** 직렬에서는 케이스가 앞 케이스가 남긴 DB 상태 위에서 돈다. 병렬에서는
어떤 케이스들이 한 DB 를 공유하는지가 바뀐다. 디렉터리 단위로 한 슬롯에 묶는 것(`dispatch` 의 slot
affinity — shell 에서는 쓰이지 않던 기계)으로 시작하고, 판정 동일성으로 검증한다.

### 3-3. 만들고 재어 보니 (2026-09-11, `evidence/sql-native.md`)

| | 결정 | 근거 |
|---|---|---|
| 기록 | `internal/result/sql.go` 가 CQT 의 출력을 **바이트 그대로** 쓴다. Java `Hashtable` 순회와 `childList` 정렬의 버릇까지 흉내 낸다 | 실제 CQT run 6개를 그 run 의 판정으로 다시 만들어 모든 파일·모든 줄이 동일 (`sql/records.sh`) |
| 실행부 | 슬롯마다 JVM 하나. 프로토콜은 fd 3 (표준 출력의 스레드 덤프 한 줄이 이후 모든 판정을 한 칸씩 밀지 않게) | 리뷰 |
| 슬롯의 쓰기 | **디스크** (`TESTKIT_SLOT_ROOT`). 메모리 upper 는 뺐다 | sql 슬롯 하나가 3.3 GB 까지 자라 14 GB tmpfs 를 채우고 서버가 멈췄다. 결정: 디스크 레인 + 병렬만으로 (사용자, 2026-09-11) |
| 실행부 JVM 크기 | 슬롯이 여럿이면 `-Xms256m -Xmx1g`, 수집 스레드 2. 직렬은 CTP 그대로 | CTP 크기로 8개가 첫 케이스 전에 16 GB |
| CQT 의 server-message 플래그 | 슬롯 run 에서는 케이스마다 **CTP 순서에서의 값**으로 맞춘 뒤 실행 (`testkit.follow_order`) | 첫 8-슬롯 sql run 이 옮긴 판정 497 중 495 가 이것. 고친 뒤 11,137 케이스 중 0 |
| 코퍼스 자체의 순서 의존 | 남는다 — medium `sesnsch.sql` 이 PUBLIC 로 로그인한 채 끝나 뒤 케이스 답이 PUBLIC 기준, sql 은 카탈로그 목록·남은 테이블 | §6 |

---

## 4. 문서화 (shell 수준)

`docs/category/sql/` — shell 과 같은 뼈대, sql 에만 있는 장을 둔다.

| 장 | 내용 |
|---|---|
| README | 한 장 그림, 가장 짧은 run |
| 1. How a run works | 6단계, 실행부, 슬롯 |
| 2. Writing a case | `.sql`·`.answer`, 문장 분리 규칙(`SQLParser`), `--@queryplan`·`--+ holdcas`·`@conn:`·`$type,val` 바인드, answer 변형 |
| 3. Running it | 호스트·Docker, 두 실행부 |
| 4. Configuration | 모든 키와 비용 — `medium_dev` 의 `create_table_reuseoid` 포함 |
| 5. Answers and comparison | 렌더링 규칙(열 이름 +4칸, 값 +5칸, `null`), CR/LF 제거 비교, `run_mode` 변형, CCI 의 바이트 비교와 `.answer_cci` |
| 6. Keeping what failed | 실패 사본, 서버 로그, 코어 |

evidence: `evidence/sql-baseline.md` (P0) → `evidence/regression-sql.md` (게이트). 명세 오류는
`evidence/spec-corrections.md` 에 발견한 자리에서 기록한다.

---

## 5. 순서

| | 내용 | 끝 |
|---|---|---|
| **P0** | 샌드박스 기준 run (medium · sql, 세 저장소 모두 upstream develop), 포맷 census, 분석 공백 채우기, ADR-016, 이 문서 | `evidence/sql-baseline.md` |
| **P1** | 공통 모듈 추출(§3-1, shell 테스트로 불변 확인) → **슬롯 위의 `sqlsuite`** + `jdbc` 실행부, `TESTKIT_NATIVE_SQL` 뒤 *(2026-09-11 — 병렬을 P3 에서 당김)* | 슬롯 1개: 스모크·medium 전체가 CTP 와 동일. 슬롯 N개: 슬롯 1개와 판정·`.result` 동일. **진행 (2026-09-11):** 코드 완료. 기록은 CTP run 6개와 바이트 동일, 직렬 medium 은 `.result` 975/975 동일(샌드박스 핀). 병렬은 §3-3 |
| **게이트** | ADR-017 확정, sql 전체를 슬롯 1개로 CTP 와 비교 | diff 0 → 병렬 기본값 on 가능 (ADR-015) |
| **P2** | `native` 실행부 — CCI 모드 먼저, `jdbc` 를 오라클로 | CCI 모드가 `ccqt` 와 동일 |
| **P3** | 나머지 축 B: 플랜 데이터 · 드라이버 비교 | 항목별 증거 |
| **문서** | `docs/category/sql/` as-built | 코드와 일치 |

---

## 6. 미결

| 항목 | 어디로 |
|---|---|
| ~~answer 없는 케이스가 `total`·`fail` 에 어떻게 잡히는가~~ | **닫힘 (2026-09-11)** — 실패로 센다. §2-3 |
| ~~`ErrorInterrupt` cascade-abort 정책~~ | **닫힘 (2026-09-11)** — 도달 불가. `ErrorInterrupt.ERROR_INTER` 는 `false` 로 선언되고 CQT 어디서도 바뀌지 않는다. 재현하지 않는다 |
| `summary.info` · `summary.xml` 의 등급 | P0 → ADR-017 |
| CAS 클라이언트: 확장 vs 신규 | P2 착수 전 스파이크 |
| 보드: 러너마다 띄우는 라이브러리 vs 러너들이 보고하는 곳 (B-T11) | P3. 제안은 라이브러리 — 러너가 둘일 때 스키마를 공유하는 것으로 충분하고, 보고받는 곳은 운영 층(ADR-012)의 몫에 가깝다 |
| 병렬에서 케이스 사이 상태 의존의 실제 크기 | **P1** — 디렉터리 affinity 로 시작, 슬롯 1 과 N 의 판정·`.result` 동일성으로 측정. *(2026-09-11)* CQT 의 server-message 플래그는 고쳤다(§3-3). 남는 것은 코퍼스가 디렉터리를 넘어 남기는 상태 — 로그인 사용자, 테이블, 카탈로그 |
| 순서에 기대는 케이스를 병렬에서 어떻게 다룰지 | 게이트 뒤. 후보: (a) 앞 디렉터리와 한 슬롯에 묶는 목록(데이터, 증거 첨부), (b) 알려진 차이로 보고, (c) 코퍼스 수정을 upstream 에 제안 |
| 코어의 `.err` 호출 스택 파일 | 미구현 — CQT 는 실패한 케이스에 새 코어가 있으면 `<케이스>.err` 를 gdb 로 쓴다. sqlsuite 는 `hasCore` 까지만 |
| 서버의 표준 출력 | CTP 에서는 서버가 run 의 표준 출력을 물려받는다(§2-5). sqlsuite 에서는 서버 단계의 파이프에 묶였다가 버려진다 — 판정에는 무관, 표준 출력의 차이 |
| medium 의 병렬 한계 | 디렉터리 8개 중 하나가 444 케이스라 슬롯을 늘려도 wall 은 그 슬롯이 정한다. 디렉터리 안의 케이스가 서로 기대는지 재고 나서 케이스 단위를 허용할지. *(2026-09-11)* 지금은 직렬이 더 빠르다 — 4슬롯 157 s, 직렬 110–125 s. 서버 기동 하나(32–48 s)가 케이스 전체(CTP 22 s)보다 크다 |
| 슬롯 upper 를 둘 디스크 | 8-슬롯 sql 의 케이스는 CTP 직렬의 4.04배 걸린다. upper 가 있는 `/var/tmp`(sdc, SATA SSD, swap 도 여기)의 4 KB 동기 쓰기는 5.8 ms, CTP 의 DB 가 있는 `/data`(sdb)는 1.3 ms. *(2026-09-11 측정)* `/data` 로 옮기면 1,131 → 722 s, overlay 를 `volatile`(끝나면 버리는 층이라 sync 를 건너뛴다)로 올리면 338–394 s — CTP 직렬의 4.5–5.2배, 케이스 시간은 CTP 직렬의 절반. medium 직렬도 77 s 로 CTP(87–90 s)보다 빠르다. **결정 (사용자, 2026-09-11):** `volatile` 은 켜는 옵션 — `TESTKIT_SLOT_VOLATILE=1`, 기본은 CTP 와 같은 sync. **남은 것:** 기본 슬롯 루트, 8슬롯의 메모리(한 run 이 메모리 부족으로 중단 — 다음 작업), 슬롯 동시 기동 — `evidence/sql-native.md` §3 |
| `kcc` · `neis05` · `neis08` 코퍼스 | 이번 범위 밖 |
| `.sql` pragma 전수 목록, `@conn:` 의 정확한 문법 | freeze §11-3 — `jdbc` 에는 필요 없고(`SQLParser` 그대로) `native` 의 Go 파서에 필요. P2 |
| ~~`run_mode` 값의 출처~~ | **닫힘 (2026-09-11)** — `jdbc_config_file` XML 의 `<run_mode>`. §2-3 |
| `.diff_1` 이 입력인가 산출물인가 | freeze §11-12 |
| `jdbc_config_file` charset XML 스키마 | freeze §11-16 — `jdbc` 는 CQT 가 그대로 읽으므로 P1 에서는 필요 없다 |
| DB 셋업 레시피 정형화, `tar` 하드코딩 버그 | ADR-006 (예약 — 파일 없음, 2026-09-11 확인) |
