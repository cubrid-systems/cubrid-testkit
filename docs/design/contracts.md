# Contracts — 모듈 간 in-process 인터페이스

- **Date:** 2026-09-02
- **Status:** Accepted (Phase 2 산출물)
- **Inputs:** `design/architecture.md` · `concept/external-surface-freeze.md` · ADR-004
- **왜 필요한가:** ROADMAP §3 — *"모듈 간 in-process 인터페이스 (이후 strangler-fig 대체의 경계)"*.
  여기 그은 선이 곧 다음에 무엇을 통째로 갈아끼울 수 있는지의 경계다.

---

## 0. 읽는 법

**계약은 다섯 개뿐이다.** 이 문서에 없는 것은 계약이 아니라 구현이며, 자유롭게 바꿔도 된다.

| # | 계약 | 무엇의 경계인가 | 안정성 |
|---|---|---|---|
| C1 | `Runner` | task 하나를 통째로 대체하는 단위 | **높음** — Phase 3·4 의 대체가 전부 이 선에서 일어난다 |
| C2 | `Format` | 케이스 형식 하나를 추가하는 단위 | **높음** — §6a 확장의 유일한 슬롯 |
| C3 | `Channel` | 명령을 어디서 실행하는가 | 중간 — 구현 추가는 잦고 시그니처 변경은 드물다 |
| C4 | `Sink` | **동결 표면이 만들어지는 곳** | **최고** — 여기가 바뀌면 F1 이 깨진다 |
| C5 | `Feedback` | 실행 진행 이벤트 | 중간 — 축 O 백엔드가 빠져 좁아졌다 |

`Config` 와 `Topology` 는 계약이 아니라 **자료**다. 구조체를 그대로 넘긴다.

---

## C1. Runner — task 하나

```go
package runner

// Runner 는 ctp.sh 의 task 하나 이상을 담당한다.
// 대체는 이 단위로 일어난다: legacy Runner 가 맡던 task 를 native Runner 가 가져간다.
type Runner interface {
    // 담당하는 task 이름. registry 가 이것으로 dispatch 한다.
    // 하나의 Runner 가 여러 task 를 맡을 수 있다 — shellsuite 는 shell/rqg/unittest/jdbc.
    Tasks() []string

    // 실행 전 환경 검증. conf 누락, scenario 디렉터리 부재, build URL 무효 등.
    // 여기서 error 를 돌려주면 종료 코드 255 다 (freeze §6-1).
    Validate(cfg *conf.Config) error

    // 실행.
    //
    // ⚠️ 케이스 실패는 error 가 아니다. 케이스가 몇 개 깨지든 종료 코드는 0 이어야 하고
    //    (freeze §6-1), 결과는 out 과 Feedback 으로만 보고된다. error 는 *실행 자체가
    //    불가능했을 때* 만 돌려준다.
    Run(ctx context.Context, cfg *conf.Config, out *result.Sink, fb feedback.Feedback) error
}
```

**등록**

```go
package registry

func Register(r Runner)              // init() 에서 호출
func Lookup(task string) (Runner, bool)   // 대소문자 무관 (freeze §1-1)
```

`Lookup` 이 실패하면 **헬프를 출력하고 그 task 만 건너뛴다** — 전체 실패가 아니다 (F1).

---

## C2. Format — 케이스 형식 하나

**§6a 확장(E1 sqllogictest ~ E7)이 들어오는 유일한 자리.** `north-star.md` §3 이 약속한 확장점이 이것 하나다.

```go
package caseformat

type Case struct {
    Path     string   // 케이스 파일 절대경로
    Name     string   // 확장자 없는 이름
    Answers  []string // 정답 후보 경로. 선택 알고리즘은 Format 이 안다
    Meta     map[string]string
}

type Verdict struct {
    Passed    bool
    TimedOut  bool
    HasCore   bool
    CoreFiles []string
    Detail    string // 실패 시 워커 로그에 들어갈 본문 (DIFF 블록 등)
}

type Exclusions interface {
    Excluded(path string) (bool, SkipType)  // SKIP_TYPE_BY_MACRO / BY_TEMP
}

type Format interface {
    Name() string   // "sh" · "sql" · "ctl" · "sqllogictest" · ...

    // scenario 트리에서 이 형식의 케이스를 찾는다.
    //
    // ⚠️ 레이아웃은 형식마다 다르다 (freeze §3-1).
    //    sh/sql : <...>/cases/*.ext + <...>/answers/*.answer  (자매 디렉터리)
    //    ctl    : _NN_<level>/<topic>/ 안에 .ctl 과 .answer 가 동거. cases/ 없음
    //    cases/ 세그먼트 필수 규칙은 shell 의 Test.java 한정이다.
    Discover(scenario string, excl Exclusions) ([]Case, error)

    // 한 케이스를 실행하고 판정한다. 실행 채널은 주입받는다 — Format 은
    // "어디서 실행되는가"를 몰라야 한다.
    Run(ctx context.Context, c Case, ch exec.Channel) (Verdict, error)
}
```

### C2-1. §6a 확장이 이 계약을 쓰는 방식

확장은 **`Format` 을 구현하지 않는다.** ADR-001 Consequence 4 대로 외부 도구를 subprocess 로 돌리고
결과 아티팩트를 ingest 한다. 따라서 확장의 접점은 `Format` 이 아니라 **`Runner`** 다.

```
E1 sqllogictest → Runner  (외부 러너 subprocess + 레코드 파싱)
E2 SQLsmith     → Runner  (C++ 바이너리 subprocess + crash 판정)
E3 SQLancer     → Runner  (java -cp ... sqlancer.Main + 로그 ingest)
```

`Format` 은 *testkit 이 직접 케이스를 읽고 판정할 때* 쓴다. 외부 도구가 자체 코퍼스와 판정기를 가지면
`Runner` 가 맞는 층이다. **이 구분을 흐리지 않는다** — 흐리는 순간 `Format` 이 무엇이든 담는 자루가 된다.

⚠️ **dialect 지식은 코드가 아니라 데이터로 공유한다** (ADR-001 §4a-4). testkit 이 CUBRID 스키마·타입·함수
카탈로그를 기계 판독 파일로 내보내고, 각 도구의 어댑터가 그것을 읽는다. `Format` 이나 `Runner` 인터페이스로
공유하려 들면 언어가 다른 도구들 사이에서 성립하지 않는다.

---

## C3. Channel — 어디서 실행하는가

```go
package exec

type Result struct {
    Stdout   string
    Stderr   string
    ExitCode int
}

type Channel interface {
    Run(ctx context.Context, cmd string) (Result, error)
    Put(ctx context.Context, local, remote string) error
    Get(ctx context.Context, remote, local string) error
    Close() error
}
```

구현: `local` (os/exec) · `ssh` (`golang.org/x/crypto/ssh`).

**RMI 모드는 세 번째 구현 자리로 비워둔다.** 존치 여부가 미결이며(freeze §11-6),
`Channel` 이 있으므로 붙이든 폐기하든 `Runner` 와 `Format` 은 영향받지 않는다.
alive-ping 규약(`echo HELLO` → `HELLO`, 1초 후 재시도)은 RMI 구현이 생길 때 그 안에 들어간다.

---

## C4. Sink — 동결 표면이 만들어지는 곳

**가장 중요한 계약.** F1 표면의 모든 바이트가 여기서 나온다.

```go
package result

// Sink 는 인터페이스가 아니라 구조체다.
// 표면이 하나뿐이라 추상화할 이유가 없고, 추상화하면 "어느 마커가 어디서
// 나오는가"를 추적할 수 없게 된다.
type Sink struct { /* ... */ }

func Open(ctpHome, task string, now time.Time) (*Sink, error)  // result/<task>/<ts>/

// stdout 마커 (freeze §4) — 전부 F1
func (s *Sink) EnvStart(envID string)
func (s *Sink) EnvStop(envID string)
func (s *Sink) TestCase(tc, envID string, ok bool, retry int)  // retry>0 이면 ", retry: N"
func (s *Sink) Core(path string)                               // "CORE_FILE:<path>"

// 결과 파일 (freeze §5)
func (s *Sink) Snapshot(cfg *conf.Config, buildID, bits string) error  // main_snapshot.properties
func (s *Sink) All(cases []string) error                               // dispatch_tc_ALL.txt
func (s *Sink) Finished(tc, envID string) error                        // dispatch_tc_FIN_<env>.txt
func (s *Sink) Worker(envID, text string) error                        // test_<env>.log
func (s *Sink) MainInfo(m MainInfo) error                              // ':' 구분
func (s *Sink) SummaryInfo(m SummaryInfo) error                        // '=' 구분
```

### 지켜야 하는 것

- `main.info` 는 `:`, `summary_info` 는 `=`. **통일하지 않는다** (NG2).
- `dispatch_tc_ALL.txt` 와 `dispatch_tc_FIN_*.txt` 의 포맷이 곧 **continue mode 의 재개 계약**이다.
- `[TESTCASE]` 의 `, retry: N` 은 **shell 계열에만** 붙는다. isolation 에는 없다.
- 실패 케이스의 DIFF 블록은 폭 51 의 `=` 구분선과 `diff -a -y -W 185` 출력. 폭까지 F1.
- `Core()` 가 내는 `CORE_FILE:` 는 **testcases 의 케이스가 직접 grep 한다** (freeze §11-2 상세).
  NG1 동결 자산이 소비자이므로 이 마커는 확정 F1 이고 논의 대상이 아니다.

### 회귀 동등성의 검증 지점

Phase 3 Exit 의 증거(`evidence/regression-shell.md`)는 **`Sink` 의 출력만 비교**하면 된다.
그것이 `Sink` 를 인터페이스가 아니라 하나의 구조체로 둔 이유다.

---

## C5. Feedback — 실행 진행 이벤트

```go
package feedback

type Feedback interface {
    TaskStart(buildURL string)
    TaskContinue()
    TaskStop()
    TotalTestCase(total, macroSkipped, tempSkipped int)
    CaseStart(tc, envID string)
    CaseStop(ev CaseStopEvent)
    CaseStopRetry(ev CaseStopEvent)   // shell 한정
    EnvStop(envID string)
}

type CaseStopEvent struct {
    TC, EnvID          string
    Success            bool
    Elapsed            time.Duration
    ResultText         string
    LastPassResultCont string  // shell 한정
    TimedOut, HasCore  bool
    SkipType           SkipType
    RetryCount         int     // shell 한정
}
```

구현은 **`Null` 과 `File` 둘뿐이다.**

⚠️ **`FeedbackDB` 는 없다.** 이벤트를 어디에 쌓을지는 운영 관심사이므로 축 O 로 제외되었다
(`migration-exclusions.md` §1-5). 이벤트 정의 자체는 축 T 라 보존한다.
그 결과 freeze §11-5(Feedback DB 스키마 확인)가 **Phase 3 블로커에서 해제**되었다.

`SkipType` 상수 `SKIP_TYPE_NO` / `SKIP_TYPE_BY_MACRO` / `SKIP_TYPE_BY_TEMP` 는 이름 그대로 보존한다.

---

## 6. 계약이 아닌 것 — 자유롭게 바꿔도 되는 것

혼동을 막기 위해 명시한다.

| | 왜 계약이 아닌가 |
|---|---|
| `conf.Config` · `topology.Instance` 구조체 | 자료다. 필드 추가·개명 자유 |
| `dispatch` 의 내부 (채널·워커 풀·재시도 루프) | 관측 가능한 것은 `Sink` 출력뿐이다 |
| `coreanalyze` 의 분석 방식 | 관측 가능한 것은 `CORE_FILE:` 마커뿐이다 |
| `legacy` Runner 의 subprocess 조립 방식 | 관측 가능한 것은 **내보내는 argv/env** 다 (freeze §8) |
| 로깅 라이브러리·에러 래핑·컨텍스트 전파 | 전부 내부 |

---

## 7. 미결

| 항목 | 계약에 미치는 영향 | 어디로 |
|---|---|---|
| RMI 모드 존치 | `Channel` 구현 하나가 늘거나 안 늘거나 | freeze §11-6 — **Phase 3 착수 전** |
| ctltool 통합 형태 | `ctl` Format 이 subprocess 를 쓰는지 cgo 를 쓰는지 | ADR-007 |
| `runone.sh` sed 정규화 전수 | `ctl` Format 의 판정이 여기 의존한다 | freeze §11-15 · ADR-009 |
| answer variant 의 `runMode` 값 출처 | `sql` Format 의 `Answers` 선택 | freeze §11-7 |
| jdbc / ha_repl / cdc_repl 출력 표면 | 해당 `Runner` 의 `Sink` 사용 범위 | freeze §11-8 — jdbc 는 **Phase 3** |
