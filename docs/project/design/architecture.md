# Architecture — cubrid-testkit

- **Date:** 2026-09-02
- **Status:** Accepted (Phase 2 산출물)
- **Inputs:** `concept/north-star.md` · `concept/external-surface-freeze.md` · `concept/migration-exclusions.md` · ADR-001/002/003/004 · `analysis/_overview/cli-tree.md` 부록 A
- **Exit 조건 (ROADMAP §3):** *"의존이 가장 적은 모듈 1개가 단독으로 빌드/실행 가능한 새 시스템 골격이 결정됨"* → §9

---

## 1. 한 장 요약

```
                    ┌─────────────────────────────────────────────┐
  사용자 / CI  ──▶  │  bin/ctp.sh  ·  shell/init_path/run_shell.sh │   동결된 진입점 (F1)
                    │  sql/bin/run.sh  ·  jdbc/bin/run.sh  ·  ...  │   전부 얇은 shim
                    └───────────────────────┬─────────────────────┘
                                            │  exec testkit "$@"
                    ┌───────────────────────▼─────────────────────┐
                    │              testkit (단일 Go 바이너리)      │
                    │                                             │
                    │  cli ──▶ conf ──▶ registry ──▶ Runner       │
                    │                                  │          │
                    │        ┌─────────────────────────┴───────┐  │
                    │        ▼                                 ▼  │
                    │   native Runner                    legacy Runner
                    │   (shell/rqg/unittest/jdbc)        (그 외 task)
                    │        │                                 │  │
                    │        │  exec.Channel                   │  │
                    │        │  (local · ssh)                  │  subprocess
                    │        ▼                                 ▼  │
                    └────────┬─────────────────────────────────┬──┘
                             │                                 │
                    ┌────────▼─────────┐            ┌──────────▼──────────┐
                    │  원격 워커        │            │  기존 CTP 자산       │
                    │  runone.sh       │            │  cubridqa-*.jar     │
                    │  init.sh         │            │  sql/bin/run.sh     │
                    │  ctltool         │            │  cqt · ccqt         │
                    └────────┬─────────┘            └──────────┬──────────┘
                             │                                 │
                    ┌────────▼─────────────────────────────────▼──────────┐
                    │  result — 동결된 출력 (F1)                           │
                    │  stdout 마커 · main.info · summary_info              │
                    │  dispatch_tc_*.txt · test_<env>.log · 종료 코드      │
                    └─────────────────────────────────────────────────────┘

                    축 O (스케줄러 · 메일 · 이슈 등록 · 큐)는 이 그림에 없다.
                    제외되었고, 나중에 이 출력을 소비하는 별도 층이 된다.
```

---

## 2. 설계를 지배하는 세 가지 제약

이 아키텍처의 거의 모든 선택은 아래 셋에서 나온다. 새로운 판단이 필요할 때도 이 순서로 묻는다.

1. **축 T 만 옮긴다** (`migration-exclusions.md`) — 스케줄·메일·이슈·큐는 여기 없다.
2. **외부 표면은 동결이다** (`external-surface-freeze.md`) — 내부는 전부 바꿔도 되지만 F1 표면은 바이트 단위로 같아야 한다.
3. **미대체 task 는 기존 자산을 subprocess 로 부른다** (ADR-003) — jar 호환 layer 는 만들지 않는다 (NG5).

---

## 3. 패키지 구조

저장소 루트가 곧 Go 모듈 루트다. `src/` 같은 중간 디렉터리를 두지 않는 것이 Go 관행이고,
gopls · golangci-lint · CI 액션이 모두 모듈 루트를 가정한다.

```
go.mod                           모듈 루트 = 저장소 루트
cmd/testkit/main.go              진입점. 인자를 cli 에 넘기고 종료 코드를 정한다
internal/
    ├── cli/          동결된 CLI 문법 파싱 · task 이름 해석 · conf fallback
    ├── conf/         INI 로딩 · dot-notation 전개 · 우선순위(default < env.instanceN)
    ├── topology/     env.instance* 를 인스턴스/역할 모델로. SSH 자격과 원격 경로
    ├── registry/     task 이름 → Runner 매핑. 단 하나의 dispatch 지점
    ├── runner/
    │   ├── shellsuite/   1차 대체 — shell · rqg · unittest · jdbc
    │   └── legacy/       미대체 task — 기존 CTP 를 subprocess 로 호출
    ├── caseformat/   케이스 발견 · 파싱 · 정답 선택. §6a 확장의 유일한 슬롯
    ├── exec/         실행 채널 추상화 — local · ssh
    ├── dispatch/     케이스 풀 · env 별 분배 · 재시도 · continue mode
    ├── result/       결과 디렉터리 · 마커 출력 · main.info · dispatch_tc_*
    ├── feedback/     실행 이벤트 (Null · File). DB 백엔드는 축 O 라 없다
    └── coreanalyze/  core dump 판정 — `CORE_FILE:` 마커가 F1 이므로 축 T 다
```

**의존 방향은 한쪽이다.** `cli → conf → registry → runner → {exec, dispatch, caseformat} → result`.
`result` 는 아무것도 import 하지 않는다 — 동결 표면을 만드는 곳이 다른 레이어를 알면, 표면을 바꾸지 않고
내부를 바꾸는 일이 불가능해진다.

---

## 4. 실행 모델

### 4-1. dispatcher → worker

기존 shell 모듈의 구조를 그대로 옮긴다. 바꾸는 것은 *동시성 표현*이지 *동작*이 아니다.

```
Dispatch.load()          케이스 풀 구성 → dispatch_tc_ALL.txt
   │                     (exclusion 적용, macro/temp skip 분리 집계)
   ├─ env1 워커 goroutine ─┐
   ├─ env2 워커 goroutine ─┤ 각자 케이스를 하나씩 가져가 실행
   └─ ...                  │ 완료마다 dispatch_tc_FIN_<envId>.txt 에 append
                           ▼
                    [TESTCASE] <tc> EnvId=<env> [OK|NOK][, retry: N]
```

- 워커 수 = conf 의 `env.instance*` 개수. 채널로 케이스를 나눠주고, 워커는 자기 env 에만 붙는다.
- **continue mode** 는 `ALL − ⋃FIN` 차집합으로 재개한다. 이 파일 포맷이 곧 재개 계약이므로 F1 이다.
- 재시도는 `testcase_retry_num` 만큼. shell 은 `retry: N` 을 마커에 붙이고 isolation 은 붙이지 않는다 —
  **이 비대칭을 통일하지 않는다** (NG2).

### 4-2. 실행 채널

```go
type Channel interface {
    Run(ctx context.Context, cmd string) (Result, error)  // stdout/stderr/exit
    Put(ctx context.Context, local, remote string) error
    Get(ctx context.Context, remote, local string) error
    Close() error
}
```

구현은 `local` (os/exec) 과 `ssh` (`golang.org/x/crypto/ssh`) 둘. 케이스 실행은 원격이 기본이고,
`unittest` 만 local 이다.

**RMI 모드는 여기서 결정하지 않는다.** 기존 shell 은 SSH 외에 RMI 워커 모드를 갖는데, 존치 여부가
`external-surface-freeze.md` §11-6 으로 미결이다. `Channel` 인터페이스가 있으므로 나중에 세 번째 구현으로
붙이거나, 폐기하고 끝낼 수 있다. **Phase 3 착수 전에 답을 내야 하는 항목**이다 (ADR-004 Consequence 4).

### 4-3. 원격 자산 배포

`shell/init_path/` 를 원격에 통째로 복사하고 `$init_path` 를 셋업하는 동작은 그대로다.
그 안의 `commonforjdbc.jar` 도 그대로 간다 — **testkit 이 만드는 산출물이 아니라 배포되는 자산**이다
(NG5 대상 아님, freeze §7-8).

원격에 `$JAVA_HOME` 을 계속 셋업한다. 케이스가 쓰기 때문이고, testkit 이 Go 인 것과 무관하다.

---

## 5. Runner — 단 하나의 dispatch 지점

M1 이 없애려는 것은 *호출 모델 3종 혼재*다. 밖에서 보이는 것은 그대로 두고 안쪽만 하나로 만든다.

```go
type Runner interface {
    // task 이름들. 하나의 Runner 가 여러 task 를 담당할 수 있다 (shell/rqg/unittest).
    Tasks() []string
    // conf 를 검증한다. 여기서 실패하면 실행 전 환경 오류다.
    Validate(cfg *conf.Config) error
    // 실행. 케이스 실패는 error 가 아니다 — 종료 코드 0 이 F1 이기 때문.
    Run(ctx context.Context, cfg *conf.Config, out *result.Sink) error
}
```

| Runner | 담당 task | Phase |
|---|---|---|
| `shellsuite` | `shell` `rqg` `unittest` `jdbc` | **3 (1차 대체)** |
| `sqlsuite` | `sql` `medium` | **4** |
| `isolationsuite` | `isolation` | **4** |
| `hareplsuite` | `ha_repl` | **4** |
| `legacy` | `kcc` `neis05` `neis08` `sql_by_cci` `cdc_repl` `webconsole` — 그리고 위 Runner 중 스위치가 꺼진 것 | 3 (공존) |

> **2026-09-23 갱신.** 이 표는 `shellsuite` 하나만 있던 시점의 것이었다. `sqlsuite` ·
> `isolationsuite` · `hareplsuite` 가 뒤따랐고, 각각 `TESTKIT_NATIVE` 뒤에 있다. 어떤 게이트를
> 통과했고 무엇이 남았는지는 루트 `README.md` 의 *What runs where* 가 유일한 기록처이며, 이
> 표는 거기에 양보한다. `hareplsuite` 는 홀로 parity 게이트를 갖지 않는데, 그 이유는
> [ADR-015](../adr/ADR-015-beyond-axis.md) 에 2026-09-23 개정으로 적혀 있다.

`jdbc` 가 `shellsuite` 에 있는 이유는 `jdbc/bin/run.sh` 가 `shell.main.JdbcLocalTest` 를 부르기 때문이다
(`cli-tree.md` 부록 A T4). shell 모듈을 옮기면 함께 온다.

### 5-1. legacy Runner

미대체 task 는 기존 CTP 를 그대로 부른다. **argv 와 환경변수를 바이트 단위로 재현**해야 한다
(freeze §8). 예:

```
sql        → sh $CTP_HOME/sql/bin/run.sh -s <suite> -f <conf>
             + export sql_interface_type / sql_interactive / log_file_in_interactive
isolation  → java -cp isolation.jar ... isolation.Main.exec(conf)
webconsole → java cqt.webconsole.Starter <conf> <webRoot> {start|stop}
```

⚠️ **용어** — 이 라우팅을 담는 것이 `design/migration-bridge.md` 의 "브리지"다.
NG5 가 금지하는 "jar 호환 layer"(구 모듈이 새 구현을 라이브러리로 호출)와 다른 것이다.

---

## 6. 결과 처리 파이프라인

동결 표면이 만들어지는 곳. **여기만 바꾸면 표면이 바뀌고, 여기만 안 바꾸면 표면은 안전하다.**

```
워커 실행 결과
   │
   ├─▶ Sink.TestCase(tc, env, ok, retry)   ──▶ stdout  [TESTCASE] ...
   ├─▶ Sink.EnvStart/EnvStop(env)          ──▶ stdout  [ENV START|STOP] ...
   ├─▶ Sink.Core(path)                     ──▶ stdout  CORE_FILE:<path>
   │                                            (케이스가 직접 grep 한다 — freeze §11-2)
   ├─▶ Sink.Worker(env, text)              ──▶ test_<env>.log  (+ DIFF 블록)
   ├─▶ Sink.Finished(tc, env)              ──▶ dispatch_tc_FIN_<env>.txt
   └─▶ Sink.Summary(counts, times)         ──▶ <resultDir>/main.info   (`:` 구분)
                                                <resultDir>/summary_info (`=` 구분)
```

`main.info` 는 `:`, `summary_info` 는 `=` — **이 비대칭은 의도적으로 보존한다**. 외부 파서가 각각에
맞춰져 있다 (NG2).

`Sink` 는 인터페이스가 아니라 **구조체 하나**다. 표면이 하나뿐이므로 추상화할 이유가 없고,
추상화하면 "어디서 마커가 나오는가"를 추적하기 어려워진다.

### 6-1. Feedback

축 T 로 남은 이벤트만 만든다 (`Null` · `File`). **`FeedbackDB` 는 축 O 로 제외**되었으므로
`feedback` 패키지에 DB 백엔드가 없다 (`migration-exclusions.md` §1-5).
`skipType` 상수(`SKIP_TYPE_NO` / `BY_MACRO` / `BY_TEMP`)는 보존한다.

---

## 7. 케이스 형식 — 확장의 유일한 슬롯

`north-star.md` §3 이 약속한 단 하나의 확장점. §6a(E1 sqllogictest ~ E7)가 들어오는 자리다.

```go
type Format interface {
    Name() string
    // scenario 아래에서 이 형식의 케이스를 찾는다.
    Discover(scenario string, excl Exclusions) ([]Case, error)
    // 한 케이스를 실행하고 판정한다. 실행 채널은 주입받는다.
    Run(ctx context.Context, c Case, ch exec.Channel) (Verdict, error)
}
```

**모듈마다 레이아웃이 다르다**는 점이 이 인터페이스의 존재 이유다 (freeze §3-1):

| 형식 | 레이아웃 | 판정 |
|---|---|---|
| `sh` (shell 계열) | `<...>/cases/*.sh` + `<...>/answers/*.answer` — 자매 디렉터리 | `.result` 회수 후 diff, stdout 패턴 |
| `sql` (sql 계열) | 같은 자매 디렉터리 + **answer variant 선택 알고리즘** | variant diff |
| `ctl` (isolation) | `_NN_<level>/<topic>/` 안에 `.ctl` 과 `.answer` **동거** | `runone.sh` 의 `flag: OK/NOK` |

⚠️ `cases/` 세그먼트 필수 규칙은 **shell 의 `Test.java` 한정**이다. isolation 에 적용하면 안 된다.

⚠️ `runone.sh` 의 **sed 정규화 체인**을 통과한 뒤에 비교가 일어난다 (freeze §7-6).
`ctl` 형식 구현은 이 체인을 보존해야 하며, 전수 목록은 §11-15 로 미결이다.

---

## 8. M1~M5 와의 대응

`north-star.md` §2 가 약속한 것이 이 설계의 어디서 이루어지는지.

| 목표 | 어디서 |
|---|---|
| **M1** 호출 모델 3종 → 단일 plug-in | §5 `registry` + `Runner`. dispatch 지점이 하나뿐이다 |
| **M2** in-band signaling 제거 | `#SCRIPTCONT` 컨베이어가 `testkit` 안으로 들어온다. interactive *동작*은 F1, 전달 메커니즘은 NF (freeze §10 행 1) |
| **M3** 노후 의존 제거 | Go 표준 + `x/crypto/ssh`. log4j·commons-io·dom4j·jgit·ActiveMQ·Quartz 가 전부 사라진다 (뒤 넷은 축 O 제외로도 사라진다) |
| **M4** 암묵적 전역 상태 | `conf.Config` 와 `topology.Instance` 를 명시적으로 전파. `CTP_HOME` 해석은 `conf` 한 곳에 격리 |
| **M5** dead surface | orphan 7 task 는 registry 에 없다. 폐기 정책은 **ADR-005 (미결)** — 무단으로 hard error 로 바꾸지 않는다 |

---

## 9. Exit 조건 — shell 모듈이 단독으로 빌드/실행 가능한가

ROADMAP §3 의 Exit 조건에 대한 답.

**빌드**: 저장소 루트에서 `go build ./cmd/testkit` 하나로 끝난다. `shellsuite` Runner 가 의존하는 것은
`cli` `conf` `topology` `registry` `exec` `dispatch` `caseformat` `result` `feedback` `coreanalyze` 뿐이고,
그중 어느 것도 기존 CTP 자산을 컴파일 시점에 필요로 하지 않는다. `legacy` Runner 도 `os/exec` 만 쓴다.

**실행**: `ctp.sh shell -c shell.conf` 가 다음을 거친다.

```
ctp.sh (shim) → testkit
  cli      : task="shell", conf fallback 적용
  conf     : shell.conf 26키 로딩, dot-notation 전개
  topology : env.instance1..N → 인스턴스 목록
  registry : "shell" → shellsuite Runner
  shellsuite:
    deploy   : init_path/ 를 각 인스턴스에 복사, $init_path 셋업        (exec.ssh)
    discover : scenario 아래 cases/*.sh 수집, exclusion 적용            (caseformat.sh)
    dispatch : dispatch_tc_ALL.txt 기록 후 env 별 워커 기동
    run      : 케이스마다 원격 실행 → .result 회수 → answer diff        (exec.ssh)
    report   : [TESTCASE]/[ENV] 마커, test_<env>.log, main.info         (result.Sink)
  종료 코드 : 0 (케이스 실패 포함) / 255 (환경·scenario·build URL 부재)
```

**나머지 task 는 이 골격을 건드리지 않고 `legacy` Runner 로 동작한다.** 그래서 shell 하나만으로
빌드도 실행도 성립한다 — Exit 조건 충족.

---

## 10. 이 문서가 미루는 것

| 항목 | 어디로 |
|---|---|
| 모듈별 신구 매핑 상세 | `design/module-*.md` |
| Runner / Format / Channel 의 확정 시그니처 | `design/contracts.md` |
| RMI 모드 존치 여부 | freeze §11-6 → **Phase 3 착수 전** |
| ctltool 통합 형태 (흡수 / subprocess / CUBRID-only) | ADR-007 |
| `.ctl` grammar 정형화 | ADR-008 |
| result normalization 정형화 (`runone.sh` sed 체인) | ADR-009 |
| orphan task 폐기 정책 | ADR-005 |
| 신/구 공존 기간 유지보수 정책 | ADR-010 |
