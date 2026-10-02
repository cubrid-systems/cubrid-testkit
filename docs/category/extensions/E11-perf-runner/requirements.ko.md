# E11 — 주간 성능 회귀 러너 (Requirements)

*[English](requirements.md) · 한국어*

**Source:** cubrid_cv `plan/perf_regression/` — PROPOSAL, Spec v0.1.2, Design v0.1.2 (인터페이스와
알고리즘은 거기에 있다. 이 항목은 그중 무엇이 testkit 에 들어오는지, 왜 여기인지를 적는다)
**Status:** incubating — **진행 중** (ADR-EXT-011 초안 2026-10-02. `perf validate`·`perf list` 가
트리에 있고, `session`·`run` 은 M2)
**축 매핑:** 여덟 축 어디도 아니다 — *측정* 역량이지 오라클이 아니다. 가장 가까운 이웃은
E7(workload)이고, 이것은 C-004 가 testkit 쪽에 남긴 몫이다
**Companion docs:** cubrid_cv 의 Spec 과 Design. `io-contract` 는 Spec §7

---

## 1. 이 항목이 해결하는 문제

`develop` 이 지난 릴리스 이후 단일 행 commit, cold heap scan, `backupdb`, CDC 추출에서 느려졌는지
지금은 아무도 말할 수 없다 — 같은 장비에서 매주 그것을 재고 숫자를 남기는 것이 없기 때문이다.
허브가 돌리는 표준 벤치마크(DOTS, PostgreSQL·MySQL 과의 TPC-C)는 다른 질문에 답한다: 혼합 부하
아래의 처리량. 기본 연산 하나의 몇 % 는 그 안에서 사라진다.

러너는 고정된 기본 연산 카탈로그를 대상 빌드와 기준 빌드로 조용한 한 대에서 교차해 재고, 비율을
빌드 지문과 함께 남긴다. 팀 브랜치는 파일에 등록하면 merge-base 와 같은 방식으로 비교된다.

## 2. 왜 testkit 인가

- 러너다: 클러스터를 세우고, 일정에 따라 그 위에서 프로그램을 돌리고, 프로그램과 `/proc` 이 말하는
  것을 모아 파일로 쓴다. 그것이 testkit 이 하는 일이다.
- testkit 이 이미 하는 방식으로 `cubrid-cluster-sandbox` 를 소비한다(ADR-022, `internal/sandbox`).
  sandbox 가 이것을 위해 얻은 플래그 — `ha_mode=off` 인 `single`, `cubrid.conf` 로 가는 `--set`,
  `--client-image`, `--broker-set`, `--cpuset` — 를 쓴다.
- **새 진입점** `testkit perf` 로, 동결된 task 이름 옆에 선다(`external-surface-freeze` §6-1: task
  목록은 F1, 새 하위 명령은 NF). `testkit shell`·`testkit sql` 이 하는 일은 아무것도 바뀌지 않고,
  코퍼스 실행이 아니므로 격리 앞에서 라우팅된다.

## 3. 경계 (C-004, 이 축에 대해 닫음)

| 어디 | 무엇 |
|---|---|
| **cubrid-engine-suite** `benchmarks/regression/` | 케이스(`case.json` + 클라이언트 소스), 픽스처, 노드 안에서 도는 스크립트, 클라이언트 이미지, `branches.conf`, `perf.conf`, 대장 |
| **cubrid-testkit** `internal/perf/` | 그 파일들의 유일한 파서, 세션, 측정, 판정, conbench 가 ingest 하는 sidecar |
| **cubrid-cluster-sandbox** | 클러스터 |
| **cubrid-conbench** | 비율이 가는 곳, 이력 |
| **cubrid-desktop** | 허브: `bench-client` 임대, 주간 timer, 빌드 단계 |

러너는 Go 만 쓴다. 허브에 Python 이 있지만 그 `uv` 환경은 cbingest 의 것이고, 러너는 거기에
기대지 않는다.

## 4. 지금 트리에 있는 것

`testkit perf validate <case-dir|fixture-dir|suite-dir|branches.conf|perf.conf>` 는 세션의 파서로
파일을 읽고 문제를 전부 이름으로 말한다. 하나라도 있으면 종료 코드 2. conf 는 가리키는 것까지
검증한다: suite, 등록, 그리고 카나리마다 suite 에 있는 케이스인지.
`testkit perf list -c <perf.conf> | --suite <dir>` 는 케이스를 패스 예산과, 세션이 주말과 견주는
상한과 함께 표로 낸다.

거부 목록은 Spec §7.2 의 것이다: 모르는 키, 빠진 키, 틀린 타입, 디렉터리와 다른 id, 수집 층 목록
밖의 카운터, `repeats < 3`, `warmup < 1`, `tolerance ≤ 0`, 다른 드라이버 모양의 client,
`warm_s = 0` 인 restore_snapshot, suite 에 없거나 버전이 다른 픽스처. `branches.conf` 는 모르는 키,
`owner=` 없는 줄, `owner/repo` 가 아닌 `repo=`, 날짜가 아닌 날짜, 맞을 수 없는 glob, 두 번 등록된
브랜치를 거부한다. `perf.conf` 도 닫힌 키 집합이다.

## 5. 다음

- **M2** — `session`·`run`: `internal/sandbox` 에 생성 옵션 추가, ABBA 패스, L0·statdump 수집,
  판정, sidecar (Design §6.1).
- **M0** — Design 이 기대는 허브 확인: rootless podman 아래의 csb, cpuset 위임, bind mount 된
  디렉터리의 내용 교체, reflink 시간 (Design §12).
- perf-client 이미지와 첫 카나리 — engine-suite 쪽.
