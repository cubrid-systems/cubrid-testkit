# ADR-002: Build Tool

- **Status:** Proposed (Phase 0 M0 출력. ADR-001 결정에 종속하여 결정)
- **Date:** 2026-04-29
- **Trigger:** Phase 0 M0 종료 (ADR-001과 함께)
- **Depends on:** ADR-001 (Implementation Language)

---

## 1. Context

기존 CTP 는 **Apache Ant** (`build.xml`)로 빌드한다. 산출물 (deps-of-common.md / cli-tree.md 의 build.xml 분석):

```
sql/lib/cubridqa-cqt.jar       ← cqt/** + plaintext/**
common/lib/cubridqa-common.jar ← common/** + ctp/** + nhncorp.grepo/*
common/sched/lib/cubridqa-scheduler.jar
shell/lib/cubridqa-shell.jar
shell/init_path/commonforjdbc.jar
isolation/lib/cubridqa-isolation.jar
ha_repl/lib/cubridqa-ha_repl.jar
cdc_repl/lib/cubridqa-cdc_repl.jar
```

추가로:
- `isolation/ctltool/Makefile` — native C 빌드 (별도)
- `sql_by_cci/` — ccqt C 바이너리 빌드 (별도)
- 각종 셸 자산 (`bin/ctp.sh`, `common/script/*`, `common/ext/*`) — 빌드 없음, 그대로 배포

새 시스템 `cubrid-testkit` 의 빌드 도구를 결정해야 한다. **결정은 ADR-001 (언어)에 직접 종속**한다.

## 2. Decision Drivers

1. **ADR-001 의 언어 선택** — 언어가 빌드 도구를 거의 결정한다 (Go = `go build`, Rust = `cargo`, Java/Kotlin = Maven/Gradle/Bazel/...).
2. **1인 운영의 단순성** — 빌드 conf 자체가 큰 학습 비용이 되면 안 됨. *zero-config* 또는 *one-file-config* 가 우호적.
3. **다중 언어/자산 통합** — Go/Rust/Java 어느 것을 골라도 *native C 의 ctltool 빌드*와 *셸 자산 패키징*은 따로 해야 한다. 이를 묶는 메타 빌드 도구 필요 (Just / Make / Mage).
4. **공존 기간의 기존 jar 빌드 유지** — 새 시스템이 기존 CTP/build.xml 을 *대체* 할지 *공존* 할지 결정 필요. 공존이면 양쪽 빌드를 다 다룰 수 있어야 함 — 이 경우 메타 빌드 도구가 필요.
5. **CI 단순성** — 1인 사이드는 CI 가 단순한 게 좋다 (GitHub Actions matrix 등).
6. **재현성** — Lockfile / version pin 이 표준.
7. **빠른 iteration** — 1인 6-12개월에서 빌드 시간이 길면 누적 손실.

## 3. Options Considered (언어별)

### 3-A. ADR-001 = Java 17/21 인 경우

| 옵션 | 장 | 단 |
|------|----|----|
| **Maven** | 표준, 의존성 풍부, 공식 지원, IDE 친화 | XML pom.xml 장황. 단순 작업도 수십 라인. |
| **Gradle (Kotlin DSL)** | 강력, 멀티 모듈에 좋음, version catalog | 학습 곡선, daemon 시작 시간, 복잡한 상태 관리 |
| **Bazel** | 멀티 언어/네이티브 통합, 캐싱 강력 | 학습 곡선 매우 큼. 1인 프로젝트 과잉 |
| **Apache Ant 유지** | 기존 그대로 | 모던화 거부 = 새 시스템 정신과 충돌 |
| **Buildr / Mill / sbt** | 간결한 DSL | 생태계 작음, 사용자 친숙도 낮음 |

**1인 우호 추천**: Gradle (Kotlin DSL) — pom.xml 보다 짧고, 멀티-모듈에 강함. 단, IDE 외에서 daemon 시작 비용 있음.
**최소 학습 추천**: Maven — 가장 표준적, 검색하면 답이 있음.

### 3-B. ADR-001 = Kotlin 인 경우

- **Gradle (Kotlin DSL)** 사실상 표준. 위와 동일한 트레이드오프.
- 다른 옵션은 비추천.

### 3-C. ADR-001 = Go 인 경우

| 옵션 | 장 | 단 |
|------|----|----|
| **`go build` + `go.mod`** | zero-config, single-binary, 빠름 | 다중 언어 통합은 별도 |
| **Mage** (`magefile.go`) | Go 자체로 빌드 스크립트 작성 — Just/Make 대체 | Go 학습이 빌드 도구로도 이어짐 (좋은 것) |
| **+ Just / Makefile** (메타) | ctltool C 빌드 + Go 빌드 + 셸 자산 패키징을 한 줄 명령으로 | 작은 추가 도구 |

**추천**: `go build` + `Justfile` (또는 `Makefile`) 메타. Go가 단일 언어 빌드는 이미 zero-config라 별도 도구 불필요.

### 3-D. ADR-001 = Rust 인 경우

| 옵션 | 장 | 단 |
|------|----|----|
| **`cargo` + `Cargo.toml`** | 표준, 의존 관리 우수, 빌드 캐시, lock 파일 | 큰 의존 시 컴파일 시간 길음 |
| **+ Just / Makefile** | ctltool/셸 자산 통합 | 동일 |

**추천**: `cargo` + `Justfile` — Rust는 cargo 외 대안이 사실상 없음.

### 3-E. ADR-001 = Python 인 경우

| 옵션 | 장 | 단 |
|------|----|----|
| **`uv`** (Astral) | 현재 Python 빌드/의존 도구 중 가장 빠름, lockfile 표준 | 새로움. 일부 PaaS 미지원 |
| **`poetry`** | 안정적, lock + venv 통합 | 느림 |
| **`pip-tools`** + setuptools | 표준 | 분산된 conf |
| **PyInstaller / Nuitka** (single-binary) | 배포 단순화 | reflection-heavy 라이브러리 깨짐 가능 |

**추천**: `uv` — 1인 작업에서 빠른 의존 동기화/락 매우 가치 있음.

### 3-F. ADR-001 = Polyglot 인 경우

- **`Just` (justfile)** 또는 `Make` (Makefile) — 메타 명령 디스패처
- 각 언어 부분은 그 언어의 표준 도구 사용
- **추천**: Just (Make 보다 간결, 변수/함수 표현 좋음)

## 4. Cross-cutting (언어와 무관한 공통 결정)

### 4-1. 메타 빌드 도구 — 어느 언어를 골라도 필요

새 시스템은 **다중 자산** (각 언어 + native C ctltool + 셸 자산 + 문서)을 동시에 빌드/패키징해야 한다. 이를 묶는 단일 진입점:

| 도구 | 추천도 |
|------|--------|
| **`Justfile`** | ⭐⭐⭐⭐⭐ — 간결, 모던, cross-platform, 변수/conditional 풍부 |
| `Makefile` | ⭐⭐⭐⭐ — 보편적, 학습 곡선 0, 단 일부 표현이 어색 |
| `Mage` (Go) | ⭐⭐⭐ — Go가 ADR-001일 때만 |
| `Cargo workspace` | ⭐⭐⭐ — Rust 한정 |

**권고: Justfile 을 메타 빌드로**. 모든 언어 옵션과 양립 가능.

### 4-2. native C ctltool 처리

- **Option A**: ctltool 디렉터리 그대로 유지, `make` 호출을 Justfile/Makefile 한 줄로 흡수.
- **Option B**: 새 시스템의 빌드 그래프 안에 통합 (Bazel / cargo build.rs / cgo).
- **Option C**: 빌드된 binary를 별도 release artifact 으로 두고 새 시스템은 *download/install* 만.

**권고: Option A** — 변경 최소, 기존 빌드 그대로 호환.

### 4-3. 셸 자산 패키징

- `bin/ctp.sh` (호환 layer), `init_path/init.sh`, `script/*` 등은 *코드*라기보다 *자산*. 빌드 시 dist/ 디렉터리에 그대로 복사.
- Justfile 의 `dist:` 타겟에서 cp/rsync 한 줄.

### 4-4. CI

- 1인 사이드는 GitHub Actions 가 표준 (무료 분당 시간이 충분).
- ADR-001 의 언어에 따라 액션 selector 가 달라짐 (`actions/setup-go`, `actions/setup-java`, `dtolnay/rust-toolchain`, `astral-sh/setup-uv`, ...).

### 4-5. Lockfile

- Go: `go.sum`
- Rust: `Cargo.lock`
- Java/Maven: pom.xml + dependency:resolve plugin (실질 lockfile 부재. Maven 4 dependency-management 필요)
- Java/Gradle: `gradle.lockfile`
- Python/uv: `uv.lock`

**권고: 어느 언어로 가도 lockfile 유지**. 1인 환경에서 의존 변동은 큰 위험.

## 5. Recommendation Tree (Conditional)

```
ADR-001 = Java
   └─► Maven (단순) 또는 Gradle Kotlin DSL (표현력)  +  Justfile 메타
ADR-001 = Kotlin
   └─► Gradle Kotlin DSL  +  Justfile 메타
ADR-001 = Go        ◀── 1순위 (ADR-001 권고)
   └─► go build + go.mod  +  Justfile 메타  +  ctltool Makefile 그대로
ADR-001 = Rust
   └─► cargo  +  Justfile 메타
ADR-001 = Python
   └─► uv (최우선)  +  Justfile 메타
ADR-001 = Polyglot
   └─► Justfile 단독  +  각 언어 표준 도구
```

## 6. Open Questions

- 새 시스템이 *기존 build.xml 을 점진적으로 대체*할지, *완전히 새 빌드*로 갈지? (strangler-fig 와 일관: 공존 기간엔 기존 build.xml 유지가 맞음)
- 빌드 산출물의 *호환성 의무* (cubridqa-common.jar 등 jar 이름이 외부 자산에 의해 참조될 가능성)? — case-formats.md 의 외부 표면 동결 의무에 jar 이름이 포함되는지 확인 필요.
- single-binary vs jar 배포 정책?
- CI 의 *케이스 실행 검증* 단계가 어떻게 구성될지 (CI 환경에 CUBRID 인스턴스가 있어야 함 — 별도 ADR 후보).

## 7. Decision

(미정. ADR-001 결정 후 본 ADR 결정. 결정 후 Status → Accepted, Decision/Why/Consequences 섹션 채움.)

## 8. Consequences (결정 후 채움)

- CI 파이프라인 설계 입력
- IDE 통합 설정 결정
- 신/구 빌드 공존 기간의 산출물 동결 정책 결정
