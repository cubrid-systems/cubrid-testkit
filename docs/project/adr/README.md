# ADR Index — 번호 배정의 단일 출처

이 파일이 **ADR 번호의 유일한 권위**다. 새 ADR 을 만들기 전에 여기에 번호를 먼저 등록한다.

## 확정 (Accepted)

| ADR | 제목 | 결정 요약 | Date |
|---|---|---|---|
| [000](ADR-000-repo-name.md) | Repository name | `cubrid-testkit` | 2026-04-28 |
| [001](ADR-001-implementation-language.md) | Implementation language | **Go** | 2026-09-02 |
| [002](ADR-002-build-tool.md) | Build tool | `go build` + `go.mod` + Justfile 메타 + `ctltool/Makefile` 유지 | 2026-09-02 |
| [003](ADR-003-external-surface-freeze.md) | External surface freeze | CLI/conf/출력/종료코드/원격컨트랙트만 동결. jar·Java API 제외. F1/F2/F3/NF 4등급 | 2026-09-02 |
| [004](ADR-004-first-replacement-candidate.md) | First replacement candidate | **Option C' — shell 단독** (rqg / unittest). ~~jdbc~~ 는 2026-09-02 결정으로 제외 | 2026-09-02 |
| [013](ADR-013-regression-equivalence.md) | How regression equivalence is proven | 정규화 후 diff 0. 코퍼스 = shell 전체 3,452, 스모크 = `_01_utility` 217. `_25_unstable` 별도 집계, HA·manually·Windows 제외. **개정 2026-09-19**: 게이트는 **패치된 코퍼스**에 대해 판정한다 — 패치는 러너의 conf 키가 아니라 **트리**에 들어가고(`shard.sh`), 양쪽 러너가 같은 소스를 읽으며, 패치된 케이스는 리포트의 `PATCHED` 블록에 이름으로 남는다. 상류 전송은 별도 트랙 | 2026-09-02 |
| [014](ADR-014-one-machine.md) | The runner's scope is one machine | 로컬이 기본, 원격도 '한 대'. 인스턴스 인벤토리·N대 deploy·기계 간 분배는 **축 O** 로 이관 (ADR-012 이 상속) | 2026-09-03 |
| [015](ADR-015-beyond-axis.md) | Axis B — beyond | 축 T·O 는 *호환성을 지키는 재작성*이라 개선이 들어갈 자리가 없다. 세 번째 축을 두고 **입회 조건 4개**(무엇을 이기는지 명시 / parity 먼저 / 증거를 미리 선언 / 끌 수 있을 것). 등록부는 `concept/beyond-axis.md` | 2026-09-03 |
| [016](ADR-016-sql-executor.md) | The sql executor | 케이스 실행(문장 전송 + 결과 텍스트)은 **인터페이스 뒤, 구현 둘**. `jdbc` = CQT 의 `SQLParser`·`ConsoleDAO` 를 그대로 쓰는 Java 프로세스(JDBC 모드 parity, 구성상 동일). `native` = Go CAS 클라이언트 + 모드별 포매터(CCI 모드 먼저, `ccqt` 대체). 문장 단위 텍스트를 주고받아 `jdbc` 가 `native` 의 오라클이 된다 | 2026-09-11 |

## 예약 (미결 — 트리거 대기)

| ADR | 제목 | 트리거 | 출처 |
|---|---|---|---|
| 005 | orphan ComponentEnum 폐기 정책 | Phase 1 마무리 | `phase0-retrospective.md` §2-5 |
| 006 | DB setup recipe 정형화 (`make_sql_db_data` / `make_db_data`) | Phase 2 | 동상 |
| [007](ADR-007-isolation-executor.md) | isolation 실행부 — ctltool 처리. **확정 (Accepted, 2026-09-15)**: 케이스는 `runone.sh`·ctltool 을 **그대로** subprocess 로, 러너는 그 둘레(탐색·큐·슬롯·판정·기록). CUBRID 만. 슬롯 1개여도 컨테이닝 — 스크립트가 사용자의 프로세스를 `pkill -9` 한다. 슬롯 1개 native run 이 표본에서 CTP 와 동일(결과 58/58) | 충족 | `design/module-isolation.md`, `evidence/isolation-baseline.md` §2. ⚠️ ADR-002 §4-2 의 Option A/B/C 문자와 **별개 공간** |
| 008 | `.ctl` DSL grammar 정형화 | **충족 (2026-09-16)** — ADR-019 가 트리거. 문법과 전수 census 는 `analysis/isolation/ctl-grammar.md` §4·§5·§8a 에 있고, 별도 ADR 파일을 두지 않는다 | 동상 |
| 009 | result normalization 정형화 | Phase 2 | 동상 |
| 010 | 신/구 공존 기간의 유지보수 정책 | Phase 3 | ROADMAP §4 |
| 011 | 인벤토리 모듈의 재작성 / 어댑터 분류 | Phase 4 | ROADMAP §5 |
| 012 | **QA 운영 층**의 경계와 재구축 설계 (scheduler / mail / issue / queue / **플릿** — ADR-014 로 추가) | Phase 5 완료 후 또는 운영 필요 발생 시 | `migration-exclusions.md` §3 |
| [017](ADR-017-sql-equivalence.md) | sql 동등성 증명 방법 — ADR-013 의 sql 판. **초안 (Proposed, 2026-09-11)**: 두 코퍼스 전체, 케이스별 판정 + **`.result` 바이트 동일** + `main.info`(시각 제외) + 집계, 자기 대조 noise floor — develop 에서 0/17,459 (판정·`.result` 모두), 앞선 버전 조합에서는 동률 정렬 1건 | 사용자 검토 | ADR-016, `evidence/sql-baseline.md` |
| [018](ADR-018-isolation-equivalence.md) | isolation 동등성 증명 방법 — ADR-013 의 isolation 판. **확정 (Accepted, 2026-09-15 — 병렬 슬롯 기본값 결정과 함께)**: CTP 는 같은 순서의 전체 run 두 번에서 판정 7개를 옮기므로 판정 diff 0 대신 — 러너 파일(check·디스패치 집합·스냅숏 키·쓴 파일 집합)은 엄격, **CTP 가 재현하는 판정은 같아야 하고**, 어긋나면 케이스 단독 3회씩 재실행해 두 러너가 갈릴 때만 러너 차이. 불안정 케이스는 제외하지 않고 보고. 현재 데이터: 슬롯 1개·4개 모두 러너 차이 0, 네 run 에 걸쳐 불안정 15, 늘 실패 7. **개정 2026-09-19 (rule 3a·결과 6·7)**: 실행부가 양쪽 같을 때만 분리=러너 차이였다. 컨트롤러를 바꾼 run 에서는 `.ctl` 이 판정한다 — **케이스가 출력 순서를 정해 두었을 때만** 러너에 귀속하고, 정해 두지 않았으면 코퍼스의 것이며 `cubrid-testkit-patches/isolation` 이 패치로 나른다. 상류 전송은 별도 트랙 | 충족 | `evidence/isolation-baseline.md` §4 |
| [019](ADR-019-isolation-controller.md) | isolation 컨트롤러 — `qactl` 처리. **초안 (Proposed, 2026-09-16)**: 컨트롤러는 testkit 이 Go 로 다시 쓰고, 클라이언트 `qacsql` 와 `runone.sh`·`prepare.sh`·`clean.sh` 는 그대로. C 로 남는 것은 `tran_is_blocked` 와 `lock_dump` 을 묻는 98줄(`internal/ctl/native/qablocked.c`)뿐. 코퍼스가 안 쓰는 13개 명령과 `qamccom.c` 전체를 버린다. ADR-007 의 "나중에" 항목을 여는 결정 | 사용자 검토 | `evidence/isolation-controller.md`, `analysis/isolation/ctl-grammar.md` |
| [020](ADR-020-sizing.md) | 병렬 슬롯 수를 누가 정하나. **초안 (Proposed, 2026-09-16)**: 상수 대신 `internal/sizing` 한 곳에서, 그 기계의 run 기록으로 — isolation·sql·medium·shell 모두. 메모리 예산은 같거나 더 큰 코퍼스를 돈 run 의 슬롯당 최대(가용 메모리가 떨어진 폭, +15%). 첫 run 은 검증된 수(isolation·sql 4, shell·medium 1)에서 시작해 run 마다 2배까지, 같은 코퍼스·같은 레인에서 더 많은 슬롯이 빠르지 않았던 지점(knee)에서 멈춤. 코퍼스 한계는 첫 시도에 통과한 가장 긴 단위로. 기록은 체크아웃 밖 `$XDG_STATE_HOME/testkit/sizing/<suite>.json`, 기계별 최근 10회. `parallel` = conservative·measured·aggressive | 사용자 검토 | `evidence/isolation-controller.md` §8 |
| [021](ADR-021-crash-reports.md) | 죽은 서버를 누가 보나. **초안 (Proposed, 2026-09-17)**: CTP 의 코어 판정(`core.*` + `FATAL ERROR`)은 core_pattern 이 apport 등으로 가는 환경에서 아무것도 못 본다. 케이스마다 러너가 직접 — 엔진이 쓰는 `$CUBRID/log/coredump/*.coredump`, 코어 파일, `FATAL ERROR` 증가분 — 을 보고 **새로 생긴 것이 있으면 그 케이스를 NOK**. 리포트는 run 디렉터리에, 코어는 gdb 스택만 보관. isolation 은 `runone.sh -n` 을 기본으로 넘겨 **`~/error_backup`(설치본 통째 복사)을 더 이상 쓰지 않는다** — CTP 와 다른 유일한 기본값, `backup_core_file_yn=yes` 로 되돌릴 수 있다. CTP 와 판정이 달라지는 것은 의도적(ADR-018 규칙 2): `reorganization_select_01` 은 여섯 번의 전체 run 에서 매번 서버를 죽이면서 평범한 diff 실패로 보고되어 왔다 (CBRD-27407) | 사용자 검토 | `evidence/isolation-always-failing.md` §1, `evidence/ctp-improvements.md` J |
| [022](ADR-022-topology-provider.md) | 토폴로지는 누가 세우나 — `cubrid-cluster-sandbox` 를 소비한다. **확정 (Accepted, 2026-09-20)**: ADR-014 가 *"the system under test has a topology; the runner does not have a fleet"* 라 했고, 그 토폴로지를 세우는 일은 `cluster-sandbox` 에 있다. subprocess + `--json` 으로 소비(ADR-001 C4), `describe --format ctp` 가 CTP 동결 키로 렌더되므로 파서도 모델도 새로 만들지 않고 바뀌는 것은 transport 뿐 — `internal/sandbox` 의 네 번째 Channel. submodule `extensions/cluster-sandbox`, E-번호는 주지 않는다(환경 제공자). **동결 HA shell 367 케이스는 두 대 경로(`relatedhosts`)로 남는다** — 케이스가 CTP 의 Java SSH 헬퍼로 슬레이브에 붙으므로 노드가 sshd·JVM·expect·CTP 트리를 갖춰야 하고, 그건 csb 에 낼 요청이다. sandbox 는 `ha_repl` 이후를 나른다 | 충족 | `internal/sandbox`, `evidence/ha-topology.md`, cluster-sandbox ADR-002 |

> 013·014·015·016 은 아래 예약 번호보다 먼저 확정되었다. 예약은 *트리거 대기*일 뿐 순서가 아니다.

### ⚠️ 번호 충돌 해소 기록 (2026-09-02)

ROADMAP 초안(§4·§5)은 `ADR-005 = 공존 유지보수 정책` / `ADR-006 = 인벤토리 분류` 로 적었고,
`concept/phase0-retrospective.md`(구 `concept/phase0-retrospective.md`(구 `PHASE0_EXIT.md`)) §2-5 — Phase 4 정밀 분석 산출는 `ADR-005 = orphan enum 폐기` / `ADR-006 = DB setup recipe` 로 적어 **005·006 이 이중 배정**되어 있었다.

**해소:** 005~009 는 Phase 4 정밀 분석의 5개 블록(coherent set)을 유지하고, ROADMAP 의 두 항목을 **010 / 011 로 재배정**했다. ROADMAP §4·§5 에 재배정 주석을 남겼다.

## §6a 확장 영역 (ADR-EXT-NNN)

번호 공간이 분리되어 있다. 인덱스는 [`../extensions/README.md`](../../category/extensions/README.md) 참조. **ADR-EXT-003 (SQLancer/E3)은 사용자 결정으로 동시 트랙 승격 — 작성 중.** 나머지는 incubating 트리거 대기.

**전제:** ADR-001 Consequence 4 — 모든 §6a 확장은 *외부 도구 subprocess 구동 + 결과 아티팩트 ingest* 형태로 통합하고, dialect 지식은 코드가 아니라 **데이터(카탈로그 파일)** 로 공유한다.
