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
| [013](ADR-013-regression-equivalence.md) | How regression equivalence is proven | 정규화 후 diff 0. 코퍼스 = shell 전체 3,722, 스모크 = `_01_utility` 234. `_25_unstable` 별도 집계, HA·manually·Windows 제외 | 2026-09-02 |

## 예약 (미결 — 트리거 대기)

| ADR | 제목 | 트리거 | 출처 |
|---|---|---|---|
| 005 | orphan ComponentEnum 폐기 정책 | Phase 1 마무리 | `phase0-retrospective.md` §2-5 |
| 006 | DB setup recipe 정형화 (`make_sql_db_data` / `make_db_data`) | Phase 2 | 동상 |
| 007 | ctltool 처리 (흡수 / subprocess / CUBRID-only) | Phase 2~4 | 동상. ADR-002 §7-1 은 `ctltool/Makefile` **유지**만 확정 — 런타임 통합 형태는 미결. ⚠️ ADR-002 §4-2 의 Option A/B/C 문자와 **별개 공간**이니 혼동 주의 |
| 008 | `.ctl` DSL grammar 정형화 | Phase 2 | 동상 |
| 009 | result normalization 정형화 | Phase 2 | 동상 |
| 010 | 신/구 공존 기간의 유지보수 정책 | Phase 3 | ROADMAP §4 |
| 011 | 인벤토리 모듈의 재작성 / 어댑터 분류 | Phase 4 | ROADMAP §5 |
| 012 | **QA 운영 층**의 경계와 재구축 설계 (scheduler / mail / issue / queue) | Phase 5 완료 후 또는 운영 필요 발생 시 | `migration-exclusions.md` §3 |

> 013 은 아래 예약 번호보다 먼저 확정되었다. 예약은 *트리거 대기*일 뿐 순서가 아니다.

### ⚠️ 번호 충돌 해소 기록 (2026-09-02)

ROADMAP 초안(§4·§5)은 `ADR-005 = 공존 유지보수 정책` / `ADR-006 = 인벤토리 분류` 로 적었고,
`concept/phase0-retrospective.md`(구 `concept/phase0-retrospective.md`(구 `PHASE0_EXIT.md`)) §2-5 — Phase 4 정밀 분석 산출는 `ADR-005 = orphan enum 폐기` / `ADR-006 = DB setup recipe` 로 적어 **005·006 이 이중 배정**되어 있었다.

**해소:** 005~009 는 Phase 4 정밀 분석의 5개 블록(coherent set)을 유지하고, ROADMAP 의 두 항목을 **010 / 011 로 재배정**했다. ROADMAP §4·§5 에 재배정 주석을 남겼다.

## §6a 확장 영역 (ADR-EXT-NNN)

번호 공간이 분리되어 있다. 인덱스는 [`../extensions/README.md`](../extensions/README.md) 참조. **ADR-EXT-003 (SQLancer/E3)은 사용자 결정으로 동시 트랙 승격 — 작성 중.** 나머지는 incubating 트리거 대기.

**전제:** ADR-001 Consequence 4 — 모든 §6a 확장은 *외부 도구 subprocess 구동 + 결과 아티팩트 ingest* 형태로 통합하고, dialect 지식은 코드가 아니라 **데이터(카탈로그 파일)** 로 공유한다.
