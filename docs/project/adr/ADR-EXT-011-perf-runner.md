# ADR-EXT-011: 주간 성능 회귀 러너는 testkit 의 새 진입점이다 (E11)

- **Status:** **Proposed** (2026-10-02) — 사용자 검토
- **구현 상태:** M2(`run`)는 머지됐다 (#16, 2026-10-03). `session` 은 M3
- **Date:** 2026-10-02
- **Trigger:** cubrid_cv `plan/perf_regression/` Spec v0.1.2 §7.1 — "testkit 의 task 이름 목록은
  동결되어 있으므로 이 축은 새 하위 명령으로 들어간다"
- **Depends on:** ADR-003 (외부 표면 동결 — 새 하위 명령은 NF), ADR-022 (토폴로지는
  cluster-sandbox 가 세운다), ADR-001 Consequence 4 (subprocess + 아티팩트)
- **구현체:** `internal/perf/` (이 저장소), 케이스·픽스처·스크립트는 `cubrid-engine-suite`
  `benchmarks/regression/`

---

## 1. Context

허브(cubrid-desktop)는 표준 벤치마크를 돌리지만 기본 연산 하나가 몇 % 느려진 것은 그 안에서
보이지 않는다. 그것을 매주 같은 장비에서 재고 이력으로 남기는 러너가 필요하다. 인터페이스(Spec
§7)와 절차(Design §5)는 cubrid_cv 에 있다. 이 ADR 은 그것이 **testkit 의 어디에, 어떤 등급으로**
들어오는지와 **engine-suite 와의 경계(C-004)** 를 정한다.

## 2. 결정 전에 확인한 사실

| # | 확인 사항 | 결과 |
|---|---|---|
| F1 | task 이름을 더할 수 있는가 | **없다.** `external-surface-freeze` §6-1 이 task 목록을 F1 로 동결했다. 새 하위 명령은 NF 다 |
| F2 | 격리(`contain.Enter`) 앞에서 라우팅되는 하위 명령이 이미 있는가 | **있다.** `isolation-ctl`, `sizing`, `check-cases` — 코퍼스를 돌리지 않는 명령은 거기 선다 |
| F3 | sandbox 를 소비하는 길이 있는가 | **있다.** ADR-022, `internal/sandbox` 가 csb 를 subprocess + `--json` 으로 부른다 |
| F4 | sandbox 가 성능 모드에 필요한 것을 갖췄는가 | **머지됐다** (#8~#12, 62843f8; testkit 핀 #14). cluster-sandbox #8 (`single` = `ha_mode=off`), #9 (`--set` → `cubrid.conf`), #10 (`--client-image`), #11 (`--broker-set`), #12 (`--cpuset`), 모두 실제 엔진으로 e2e 통과 |
| F5 | conf 파서를 새로 써야 하는가 | **아니다.** `perf.conf` 는 다른 conf 와 같은 flat properties 이고 `conf.Home.Load` 가 그대로 읽는다 |

## 3. Decision

### 3-1. 진입점 → `testkit perf <verb>`, NF, 격리 없음

`perf` 는 `cmd/testkit/main.go` 에서 `sizing`·`check-cases` 와 같은 자리, 격리 **앞**에서 라우팅된다.
`TESTKIT_CONTAIN` 을 무시한다 — 세션은 csb 로 클러스터를 세우지 여기서 코퍼스를 돌리지 않고,
`validate`·`list` 는 파일만 읽는다. CLI·conf·출력 파일은 NF 등급이며 Spec §7 이 그 명세다.

### 3-2. 파서는 testkit 에, 데이터는 engine-suite 에 (C-004 를 이 축에 대해 닫는다)

`case.json`·`fixture.json`·`branches.conf`·`perf.conf` 의 **유일한 파서**는 `internal/perf` 다.
`validate` 는 세션이 쓰는 그 파서를 노출하므로 "validate 가 통과했다" 가 "세션이 이 파일에서
멈추지 않는다" 를 뜻한다. 케이스·픽스처·노드 안 스크립트·클라이언트 이미지·대장은 engine-suite
`benchmarks/regression/` 에 있다. 러너는 케이스를 **내용으로 알지 못한다** — 명세가 말하는 것만
한다.

### 3-3. 명세는 닫힌 스키마다

모르는 키는 이름을 말하며 거부된다. sandbox 시나리오의 `measure` 가 모르는 이름으로 null 열을
만든 경험에서 온 규칙이고, 카운터 목록(수집 층의 닫힌 목록)도 같다. 한 파일의 문제는 **한 번에
전부** 보고된다.

### 3-4. 결과는 engine-suite 형식, conbench 로

sidecar(`regression-case.json`)와 `cases.csv` 는 cbingest 가 읽는 형식이다(Spec §7.6). 러너는
publish 하지 않는다 — `bench-client` 가 rc=0 일 때만 한다.

### 3-5. Go 만

허브의 Python 은 cbingest 의 `uv` 환경이다. 러너는 그것에 기대지 않는다.

## 4. Consequences

- 동결된 표면은 건드리지 않는다. 기존 task 의 CLI·conf·종료 코드는 그대로다.
- `internal/sandbox` 는 M2 에서 생성 옵션(`cpuset`, `client_cpuset`, `client_image`, `set[]`,
  `broker_set[]`, `db`)과 호출별 타임아웃을 얻는다. `harepl` 경로는 바뀌지 않는다.
- 카탈로그에 E11 이 생긴다. 축 번호는 없다 — 오라클이 아니라 측정이다.

## 5. 기각한 대안

- **engine-suite 가 러너를 직접 가진다(bash + Python).** 러너의 일 — 클러스터, 예산, 중단·복구,
  sidecar — 은 testkit 이 이미 하는 일이고, 두 벌이 된다. engine-suite 의 로드맵도 기반 러너를
  testkit 으로 간다.
- **기존 task(`shell`) 에 케이스로 넣는다.** 측정은 판정이 diff 가 아니라 비율과 허용폭이고,
  두 빌드를 한 세션에서 교차한다. 동결된 task 가 표현할 수 없다.
- **bare metal, 포트 분리 (Spec §7.1.1 B).** T0 가 실패하면 돌아갈 대비로 남긴다.
