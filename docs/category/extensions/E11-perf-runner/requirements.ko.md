# E11 — 주간 성능 회귀 러너 (Requirements)

*[English](requirements.md) · 한국어*

**Source:** cubrid_cv `plan/perf_regression/` — PROPOSAL, Spec v0.1.3, Design v0.1.3 (인터페이스와
알고리즘은 거기에 있다. 이 항목은 그중 무엇이 testkit 에 들어오는지, 왜 여기인지를 적는다)
**Status:** incubating — **진행 중** (ADR-EXT-011 초안 2026-10-02. `perf validate`·`perf list` 가 트리에
있고(M1), `perf run` 은 #16 으로 머지됐다(M2, 2026-10-03). `session` 은 M3(2026-10-07))
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
  sandbox 가 이것을 위해 얻은 플래그 다섯 — `ha_mode=off` 인 `single`, `cubrid.conf` 로 가는 `--set`,
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
밖의 카운터(statdump 이름은 엔진 자신의 표 — `statdump_names.go`, develop `5f3a30d` 기준 234개 — 로 확인한다), `repeats < 3`, `warmup < 1`, `tolerance ≤ 0`, 다른 드라이버 모양의 client,
드라이버에 맞지 않는 `clients`(utility 는 0, 나머지는 1 이상), `warm_s = 0` 인 restore_snapshot, `2G` 같은
볼륨 크기가 아닌 픽스처 `size`, `[a-z][a-z0-9_]*` 밖의 픽스처 이름·케이스 디렉터리·`client.bin`(픽스처 이름은
데이터베이스 `perf_<name>` 이 되고 createdb 는 대시를 거부한다), Java 클래스 이름이 아닌 `client.main`,
suite 에 없거나 버전이 다른 픽스처. `branches.conf` 는 모르는 키,
`owner=` 없는 줄, `owner/repo` 가 아닌 `repo=`, 날짜가 아닌 날짜, 맞을 수 없는 glob, 두 번 등록된
브랜치를 거부한다. `perf.conf` 도 닫힌 키 집합이다.

`testkit perf run <case-id> --suite <dir> --build <target> --build <reference> [--repeats N] [--out <dir>]
[--cpuset <list>] [--client-cpuset <list>] [--client-image <image>] [--keep]` 은 케이스 하나에 대한 세션의
경로다: `internal/sandbox` 를 통한 `single` 클러스터 둘(`CreateWith`: 클라이언트 이미지, 핀 고정, 케이스의
`cubrid.conf` 키, broker 의 CAS 수를 가장 큰 `clients` 로 고정), 각각에 픽스처를 만들고 일반 복사(`--reflink=never`)로 스냅샷, AB 워밍업과 ABBA 측정 패스,
패스마다 L0·statdump 스냅샷과 클라이언트 자체 보고, 판정, 그리고 Spec §7.6 의 파일들을 `--out` 아래에
(`regression-case.json`, `cases.csv`, `counters.json`, describe 아티팩트, `session.json`). 표준 출력 마지막
줄이 비율이다. publish 는 하지 않는다. cpuset 둘은 CPU 목록 자체에 쉼표가 있어 플래그 둘로 받는다.

`testkit perf session -c <perf.conf> [--dry-run] [--only <case-glob>] [--pair <name>] [--out <dir>]
[--deadline <RFC3339>] [--id <run-id>] [--keep]` 은 주간 세션이다(Design §5.1, M3). conf·suite·등록을 한꺼번에
읽어 문제가 있으면 종료 코드 2 로 거부하고, 빌드는 `builds.manifest` 에서 가져온다(`builds.go` — 파일에 없거나
설치 트리가 없는 쌍은 `skipped: build missing`, 24시간보다 오래된 파일은 모든 쌍을 `builds stale` 로). 지문을
읽어 `$BENCH_RUNS` 아래 직전 `*_perf-weekly` run 과 비교하고(FR-2), Debug 빌드는 거부한다. 호스트를 확인하고
(`hub.go`: csb, 클라이언트 이미지, 디스크, `MemAvailable`, 페이지 캐시 sudo 줄, bench-mode·부스트 상태 기록,
컨테이너 밖의 `cub_server`·`postgres`·`mysqld` 는 끝내되 끝나지 않으면 종료 코드 3 — 외부 부하 가드 1단),
남은 `pf-` 클러스터를 지운 뒤 쌍마다: 기준 빌드로 세운 두 클러스터 사이의 카나리 A/A, 하나라도
`canary_tolerance` 밖이면 그 쌍은 무효이고 케이스는 `skipped`, 그 다음 세션마다 섞은 순서의 케이스를
`cubrid.conf` 오버라이드별 묶음으로(묶음마다 클러스터 하나, `schedule.go`) 돌린다. 패스 동안 상대 클러스터의
컨테이너는 pause 되고 각 쪽에는 현재 픽스처의 서버만 떠 있으며(`cluster.go`), 측정 패스마다 서버 코어의 외부
CPU 와 `pswpin` 을 읽어(가드 3단, 코어 하나를 넘거나 swap-in 이 있으면 `null(contaminated)`), 추정치가
허용폭 밖인데 합의가 없으면 ABBA 5쌍을 더 돌리고(FR-20.1), 케이스 경계마다 임대(`$BENCH_RUNS/.lease.json`)를
확인하며(L7), 예산과 `$PERF_DEADLINE` 중 먼저 오는 쪽에서 쌍·케이스 경계에 멈춘다(`skipped: budget`, 종료
코드 0). 파일은 쌍마다 sidecar(그리고 `canary/`·`overlap/`), 케이스마다 `counters.json`, `summary.md`,
`ledger_rows.md`, `session.json` 이다(`summary.go`). `--dry-run` 은 계획과 `session.json` 만 쓰고 아무것도
세우지 않는다(Spec §13 A1). 결과 디렉터리는 bench-client 가 준 `$REPORTS_DIR` 이고 run id 는 그 이름이다.

## 5. 다음

- 허브: `perf-weekly` 래퍼와 unit, 빌드 단계의 `builds.json`, `hub.json` 과 대시보드 페이지(`bench-hub`),
  T3 드릴(Design §6.5.1 L11).
- conbench: `regression-case.json` ingest 분기. engine-suite: `build_fingerprint.sh`.
- 이후: 카나리 재설계(Design §13 13), 빌드 격리(§13 12), 오염 가드 임계와 최소 절대 변화(T1).
