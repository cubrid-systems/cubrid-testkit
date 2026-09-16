# Repository Layout — cubrid-testkit

- **Date:** 2026-09-16
- **Status:** 적용됨 (Applied, 2026-09-16) — ADR-019 로 올린다
- **왜 지금:** 최상단이 구조화되어 있지 않다는 지적. 축약어와 비축약어가 섞여 있고
  (`ext` ↔ `internal`), 직접 구현하는 코드가 한 디렉터리 아래 모여 있지 않다는 것.

이 문서는 최상단 항목을 하나씩 검토하고, 무엇을 바꾸고 무엇을 그대로 둘지와 그 근거를 적는다.

**판정 기준은 하나다: 이름이 그것이 무엇인지 정확히 말하는가.** 특히 이 저장소 안에서 같은
단어가 다른 것을 가리키고 있지 않은가. "코드가 그 경로를 참조한다" 는 판정 근거가 아니다 —
그것은 비용이고, 비용은 코드를 고치면 없어진다.

---

## 0. 먼저, 바꿀 수 없는 것과 바꾸면 안 되는 것

### `internal/` 은 이름이 아니라 컴파일러 규칙이다

Go 1.5 부터 `internal/` 아래의 패키지는 **`internal/` 의 부모를 루트로 하는 트리 안에서만**
import 된다. 이름이 곧 접근 제어라 다른 이름으로 바꾸면 그 보호가 사라진다. 관례가 아니라
언어 기능이므로 선택지가 없다.

따라서 "`ext` 는 축약어인데 `internal` 은 아니다" 라는 비대칭은 **`internal` 쪽을 건드려
해소할 수 없다.** 지적은 타당하고, 고칠 곳은 `ext` 쪽이다.

### `src/` 는 Go 에서 후퇴다

`src/` 는 모듈 이전 GOPATH 시대의 잔재(`$GOPATH/src/<import-path>`)이고, 모듈 도입과 함께
의도적으로 버려졌다. 지금 이 저장소의 `cmd/` + `internal/` 조합이 표준 레이아웃 그 자체이며
Kubernetes · Docker · etcd 가 쓰는 형태다.

| 항목 | `src/` 도입 시 |
|---|---|
| Go 파일 | 137 개 이동 |
| import 경로 | 152 곳 수정 |
| 얻는 것 | 없음 — 덜 관용적인 구조가 된다 |

**기각.** Java · JS 감각으로는 자연스러운 직관이지만 Go 에서는 역방향이다.

---

## 1. 최상단 전수 검토

| 항목 | 지금 | 판정 | 근거 |
|---|---|---|---|
| `cmd/` | 진입점 (`testkit/main.go`) | **유지** | Go 표준 레이아웃. main 패키지는 `cmd/<binary>/` |
| `internal/` | 구현 15 패키지 | **유지** | 컴파일러가 강제하는 이름 (§0) |
| `docs/` | 문서 트리 | **유지** | 이름이 정확하고 이미 최상단 |
| `patches/` | 업스트림이 받아가기 전까지 들고 있는 코퍼스 수정 | **이동** | §4 — 이름은 그대로, `overrides/` 아래로 |
| `go.mod` `go.sum` | 모듈 정의 | **유지** | 위치 고정 |
| `.github/` | 워크플로 2 개 (build, bump-sqlancer-submodule) | **유지** | 위치 고정 |
| `CONTEXT.md` | 용어집 — task ≠ suite ≠ module ≠ runner | **유지** | 최상단에 있어야 먼저 읽힌다 |
| `README.md` | 루트 문서 | **유지** | — |
| **`ext/`** | 서브모듈 `cubrid-sqlancer` 하나 | **개명** | §2 — 축약어이고, 이 저장소에서 이미 다른 뜻이다 |
| **`exclusions/`** | 이 **기계가** 못 도는 케이스 | **개명** | §3 — `ext` 와 같은 결함. 이 저장소에서 `exclusion` 은 세 가지를 가리킨다 |
| **`tools/`** | `sizing.sh` | **개명** | §5 — 관례상 `scripts/` 가 맞다 |
| **`lob/`** | **비어 있고 git 에 없다** (추적 파일 0) | **삭제** | 잔재. 이름이 무엇을 뜻했는지도 남아 있지 않다 |
| **`bin/`** | 빌드 산출물 `testkit` | **정리** | §6 — 무시되는 이유가 의도가 아니다 |
| `.gitignore` | Java/Maven/Gradle/Ant/Eclipse 보일러플레이트 | **재작성** | §6 |

### 개명 비용에 대하여

`patches/` 와 `exclusions/` 의 경로는 **하드코딩이 아니라 conf 키다.**

```
case_patch_dir              patch.Load 가 받는 값 (shell.go:563, sql.go:175)
testcase_exclude_from_file  CTP 로부터 물려받은 F1 키
```

Go 코드에 남은 `patches/` `exclusions/` 문자열은 전부 **주석**이다 —
`discover.go:168` 의 `// See exclusions/README.md.`, `patch.go:40` 의 경로 예시.
경로 상수가 아니므로 개명은 주석과 문서를 고치는 일이고, 빌드에 영향이 없다.

---

## 2. `ext/` — 출처가 아니라 관계로 이름 짓는다

### `third_party/` 는 틀린 이름이다

Google · Bazel · Chromium 관례라 축약어 문제는 풀리지만, **들어갈 것들이 남의 것이 아니다.**

- `cubrid-sqlancer` 는 cubrid-systems 자신의 저장소다. sibling 구현이지 third party 가 아니다.
- 앞으로는 진짜 외부 프로젝트가 들어올 수도 있다.
- 둘을 가르는 기준(누가 만들었나)은 testkit 입장에서 의미가 없다.

이름이 주장하는 바가 사실과 다르면 안 된다. **기각.**

### `ext` 는 이 저장소 안에서 이미 다른 뜻이다

풀어 쓰는 것만으로도 부족하다. 이 프로젝트의 어휘에서 `ext` 는 **CTP 의 `common/ext/run_*.sh`**
를 가리키며 ADR-001 · ADR-002 · ADR-003 에 그렇게 쓰여 있다. 한 단어가 두 가지를 가리킨다.

### 결정: `extensions/`

기준은 출처가 아니라 **testkit 과의 관계**다. 저기 들어가는 것은 "별도 저장소이고, testkit 이
언젠가 front-end 로서 실행을 제어할 대상"이다. 그 개념에는 이미 이 프로젝트의 이름이 있다 —
**확장(extension)**, `docs/category/extensions/` 의 E1–E10.

| | |
|---|---|
| 축약 해소 | `internal` `patches` 와 같은 결 |
| 의미 충돌 해소 | CTP 의 `common/ext` 와 더 이상 겹치지 않는다 |
| 출처를 주장하지 않는다 | sibling 이든 외부든 똑같이 확장이다 |
| 이미 쓰는 어휘 | `E3-sqlancer/design.md` 가 `ext/cubrid-sqlancer/` 를 그 확장의 구현으로 가리킨다 |

구현과 문서가 같은 단어를 쓰게 된다: `extensions/cubrid-sqlancer/` ↔
`docs/category/extensions/E3-sqlancer/`.

**비용:** `.gitmodules`, `.github/workflows/bump-sqlancer-submodule.yml:34`, `README.md:302`,
`E3-sqlancer/design.md` 2 곳, `ROADMAP.md:38`. Go 코드 0 곳.

**미결:** front-end 제어가 실제로 붙을 때 `extensions/` 아래가 저장소 하나당 서브모듈 하나로
계속 갈지, 실행 어댑터가 `internal/` 어딘가에 따로 생길지는 아직 정할 수 없다. 이 문서는 이름만
정하고, 그 구조는 E-축이 실제로 하나 붙을 때 결정한다.

---

## 3. `exclusions/` — `ext` 와 정확히 같은 결함

"코드가 참조하니 둔다" 는 근거를 걷어내고 이름만 보니, 이쪽이 `ext` 보다 나쁘다.

이 저장소에서 **`exclusion` 은 세 가지를 가리킨다.**

| 어디 | 무엇 | 누구 것 |
|---|---|---|
| `concept/external-surface-freeze.md` §3-5 | `testcase_exclude_from_file` 의 파일 형식, **F1 동결 표면** | CTP 로부터 물려받은 것 |
| `concept/migration-exclusions.md` | 마이그레이션에서 **뺀 것** (축 O) | 이 프로젝트의 범위 결정 |
| `exclusions/` ← 이 디렉터리 | **이 기계가** 못 도는 케이스 | 이 기계의 것 |

게다가 이 디렉터리의 README 는 **첫 문단부터** 그 구분이 핵심이라고 말한다:

> A case can be skipped for two very different reasons, and this directory exists
> so the two are never in the same file. […] The distinction is not bookkeeping.

업스트림의 제외는 *케이스*에 대한 주장이라 어느 기계에서나 살아남고, 여기 것은 *기계*에 대한
주장이라 기계가 바뀌는 순간 사라져야 한다 — README 가 그렇게 적어 놓았다. **그런데 디렉터리
이름은 그 둘 중 어느 쪽인지 말하지 않는다.** 최상단에서 `exclusions/` 만 본 사람은 그것이
코퍼스의 제외 목록이라고 읽는 것이 자연스럽다.

존재 이유가 "누구 것인지 구분하는 것" 인 디렉터리가, 이름에서 누구 것인지를 빼고 있다.

### 결정: `machine-exclusions/`

README 자신의 표현이 "this machine's" 다. 그대로 쓴다.

| | |
|---|---|
| 세 뜻의 충돌 해소 | 다른 둘은 `machine-` 이 아니다 |
| 존재 이유가 이름에 | 기계가 바뀌면 사라져야 한다는 것이 이름에서 읽힌다 |
| conf 키와 무관 | `testcase_exclude_from_file` 은 F1 이라 건드리지 않는다. 디렉터리 이름은 동결 표면이 아니다 |

**비용:** `discover.go:168` · `shell.go:853` 주석 2 곳, `README.md:299`,
`category/shell/02-writing-a-case.md:177` 의 링크. Go 로직 0 곳.

대안으로 `local-exclusions/` 도 같은 일을 하지만, 이 프로젝트에서 "local" 은 ADR-014 의
local/remote 실행 위치를 가리키므로 **또 하나의 충돌을 만든다.** `machine-` 을 쓴다.

---

## 4. `patches/` — 이름은 그대로, 자리는 `overrides/` 아래로

같은 기준을 적용한 결과다. 판정을 뒤집는 것이 아니라, 근거를 바꾼 뒤에도 같은 답이 나온다.

- 이 저장소에서 `patch` 는 **한 가지만** 가리킨다. `ext` `exclusion` 과 달리 충돌이 없다.
- 축약어가 아니다.
- conf 키 `case_patch_dir` 이 더 정확하지만, 디렉터리 이름이 그것과 어긋나지 않는다.

`corpus-patches/` 로 더 좁힐 수는 있으나 구분해야 할 다른 종류의 patch 가 없다. **이름은 유지한다.**

### `overrides/` 아래로 묶는다

처음에는 묶지 않는 쪽으로 판단했다. 근거는 "축이 다르다" 였다.

| | 무엇에 대한 주장인가 | 언제 사라지나 |
|---|---|---|
| `patches/` | **코퍼스**에 대한 주장 | 업스트림이 받아가면 |
| `machine-exclusions/` | **이 기계**에 대한 주장 | 기계가 바뀌면 |

**그 판단은 틀렸다.** 축이 다른 것은 맞지만, 그것은 *부모 이름이 축 하나를 주장할 때만*
문제가 된다. 구분을 들고 있는 것은 하위 디렉터리 이름이고, 그 둘은 `overrides/` 아래에서도
그대로 구분된다.

그리고 공통점이 있다. 코퍼스는 **무엇을 어떻게 실행할지의 목록**을 정하고, 이 둘은 각각
그 목록을 덮어쓴다 — patch 는 케이스의 내용을, exclusion 은 실행 대상 집합을. 제외가
"빼는 것이라 덮어쓰기가 아니다" 라고 본 것이 처음 판단의 오류였다.

```
overrides/
  patches/              코퍼스에 대한 주장 → 업스트림이 받아가면 사라진다
  machine-exclusions/   이 기계에 대한 주장 → 기계가 바뀌면 사라진다
```

부모는 축을 주장하지 않고 관계만 말한다: **이 run 이 코퍼스를 있는 그대로 실행하지 않는
지점들.** 후보 중 `deviations/` 는 `evidence/compare/deviations.txt`(판정 파일의 유일한
면제)와 ADR-015 가 이미 쓰고 있어 기각했고, `corpus-overrides/` 는 부모가 코퍼스를
주장해 `machine-exclusions` 와 어긋나므로 기각했다.

---

## 5. `tools/` → `scripts/`

`golang-standards/project-layout` 의 구분은 이렇다.

| | |
|---|---|
| `/tools` | 이 프로젝트를 위한 도구. **`/internal` 의 코드를 import 할 수 있는 것** |
| `/scripts` | build · install · analysis 등을 수행하는 스크립트 |

`sizing.sh` 는 Go 코드를 import 하지 않는 셸 스크립트이므로 `/scripts` 가 맞다. 충돌하는
뜻이 없어 §3 만큼 급하지는 않지만, 관례가 가리키는 쪽이 분명하고 비용이 문서 9 곳과 주석
3 곳뿐이라 같이 옮긴다.

---

## 6. `.gitignore` 와 빌드 산출물

지금 `.gitignore` 는 73 줄이고 대부분이 **Java 프로젝트 보일러플레이트**다 — `*.jar` `*.war`
`target/` `pom.xml.*` `.mvn/` `.gradle/` `dist/` `.classpath` `.settings/`. Go 저장소에
남아 있을 이유가 없고, "정리가 미흡하다" 는 인상의 가장 큰 근거다.

부작용도 있다. **`bin/` 이 무시되는 것은 의도가 아니라 Eclipse 절에 우연히 걸린 결과다.**
빌드 산출물 규칙이 두 군데로 갈라져 있다:

```
bin/          # Eclipse 절 안에 있다
/testkit      # "build output" 절에 따로
```

README 가 안내하는 빌드는 `go build -o bin/testkit ./cmd/testkit` 이므로 `bin/` 은 의도적으로
무시되어야 한다. 우연이 아니라 그렇게 적는다.

**결정:** Go 기준으로 재작성하고, 산출물 규칙을 한 곳에 모으고, `.omc/` 는 유지한다.
73 줄 → 25 줄.

---

## 7. 정리하면

| 한 것 | 비용 |
|---|---|
| `lob/` 삭제 | 0 — git 에 없었다 |
| `.gitignore` 재작성, 산출물 규칙 통합 | 0 — 코드 무관 |
| `ext/` → `extensions/` | `.gitmodules`(path 와 섹션 이름) + CI 1 + 문서 4 곳. Go 0 |
| `exclusions/` → `machine-exclusions/` | 주석 2 + 문서 3 곳. Go 로직 0 |
| `patches/` + `machine-exclusions/` → `overrides/` 아래로 | 주석 4 + 문서 5 곳. Go 로직 0 |
| `tools/` → `scripts/` | 주석 3 + 문서 9 곳. Go 로직 0 |

| 하지 않은 것 | 이유 |
|---|---|
| `src/` 도입 | Go 에서 후퇴. 137 파일 · 152 import 를 바꿔 덜 관용적이 된다 (§0) |
| `internal/` 개명 | 컴파일러가 강제하는 이름 (§0) |
| `patches/` 개명 | 이 저장소에서 `patch` 는 한 가지만 가리킨다 (§4) |
| `docs/` `cmd/` `.github/` 이동 | 이름이 정확하고 자리가 고정이다 (§1) |

바꾼 뒤의 최상단:

```
cmd/                   진입점
internal/              구현
extensions/            별도 저장소로 가는 확장 — 언젠가 testkit 이 실행을 제어할 것
overrides/             이 run 이 코퍼스를 있는 그대로 실행하지 않는 지점들
  patches/               코퍼스에 대한 주장 — 업스트림이 받아가면 사라진다
  machine-exclusions/    이 기계에 대한 주장 — 기계가 바뀌면 사라진다
scripts/               이 기계를 재는 스크립트
docs/                  문서
```

검증: `go build` · `gofmt -l` · `go vet` · `go test ./... -count=1` 모두 통과 (17 패키지, 실패 0,
go1.27.1). Go 변경은 전부 주석과 테스트 픽스처 문자열이고, 출력 문자열을 바꾼 두 곳
(`patch.go:271` 의 `patched.txt` 헤더, `shell.go:483` 의 `[WARN]` 줄) 도 기존 단언을 깨지
않는다 — `patch_test.go:194` 는 `"verdicts are about the patched case"` 를 부분 문자열로 찾고,
`shell_test.go:415` 의 `sizing.sh` 는 단언이 아니라 테스트 이름이다.

마크다운 링크는 저장소의 `.md` 140 개를 전수 검사해 깨진 곳이 없고, `git submodule status` 도
깨끗하다.

ADR-019 로 올리고 `adr/README.md` 의 번호 배정을 거친다.
