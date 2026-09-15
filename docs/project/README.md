# CUBRID Test Kit — 프로젝트 문서

재작성이 왜 필요한지, 무엇을 얼려 두는지, 어떻게 설계했는지, 무엇으로 증명하는지. 테스트를 **돌리는** 방법은
여기가 아니라 [`../category/`](../category/)에 있다.

## 현재 위치

- **Phase 3 진행 중** — `unittest` 네이티브. `shell`·`rqg` 는 `TESTKIT_NATIVE=shell` 뒤에서 전체 코퍼스를
  돈다(develop head 에서 3,216건 판정, 실패 32건 전부 원인 귀속). CTP 와의 전체 코퍼스 비교는 아직 통과 전
  → [parallel-shell](evidence/parallel-shell.md) §6–7
- **Phase 4 착수** — `sql`·`medium` 네이티브, ADR-017 게이트 통과(세 저장소 모두 upstream develop head)
  → [regression-sql](evidence/regression-sql.md)

**프로젝트 방향성:** 테스트 실행 축(T)과 QA 운영 축(O)을 분리한다. 이번 마이그레이션은 축 T 만 옮기고, 축 O 는
제외 기록 후 나중에 새 층으로 세운다 → [migration-exclusions.md](concept/migration-exclusions.md)

## 문서

- [Roadmap](ROADMAP.md) — 단계별 분석/설계/구현 계획
- [ADR 인덱스](adr/README.md) — 번호 배정의 단일 출처
- **Concept (Phase 1 산출물)**
  - [North Star](concept/north-star.md) — 새 시스템의 정체성
  - [External Surface Freeze](concept/external-surface-freeze.md) — 동결 명세 + 신↔구 1:1 매핑 표
  - [Non-Goals](concept/non-goals.md) — 의도적으로 하지 않는 것 (NG1~NG11)
  - [Migration Exclusions](concept/migration-exclusions.md) — **축 T/O 분리 원칙** + 마이그레이션 제외 목록
  - [Phase 0 회고](concept/phase0-retrospective.md) — 게이트 통과 기록
- [Analysis notes](analysis/) — Phase 0 산출물. 모듈별 요구사항·설계·io-contract
- [Survey](survey/dbms-testing-ecosystem.md) — DBMS 테스팅 생태계 8축 분류
- **Design (Phase 2 산출물)**
  - [Architecture](design/architecture.md) — 패키지 구조 · 실행 모델 · 결과 파이프라인
  - [Contracts](design/contracts.md) — 계약 5개와 그 경계
  - [module-shell](design/module-shell.md) — 1차 대체 대상, 35 클래스 매핑
  - [module-sql](design/module-sql.md) · [module-isolation](design/module-isolation.md) · [module-medium](design/module-medium.md)
- **Evidence (측정과 비교)** — 주장마다 근거. 숫자는 전부 실제 run 에서 나온다
  - [regression-shell](evidence/regression-shell.md) · [parallel-shell](evidence/parallel-shell.md) — shell
  - [sql-baseline](evidence/sql-baseline.md) · [sql-native](evidence/sql-native.md) · [regression-sql](evidence/regression-sql.md) — sql·medium (ADR-017 게이트 포함)
  - [compare/](evidence/compare/README.md) — 정규화 후 비교하는 방법 (ADR-013)

돌리는 사람을 위한 문서는 한 단계 위에 있다.

- [category/shell](../category/shell/README.md) — 단계·슬롯·메모리 ceiling·모든 키
- [category/sql](../category/sql/README.md) — sql·medium: 단계와 실행부, 키와 스위치, 슬롯이 사 주는 것과 그 값, 실패 읽는 법
- [category/isolation](../category/isolation/README.md) — isolation: 단계와 `runone.sh`, `qactl` 이 읽는 `.ctl` 언어, 키, CTP 도 재현하지 못하는 케이스
- [category/extensions](../category/extensions/) — §6a 확장 영역 E1~E10 (incubating)

## 디렉터리 구조

```
docs/
├── assets/      README 의 그림
├── category/    돌리는 방법 — shell/ · sql/ · isolation/ · extensions/
└── project/     이 폴더 — 왜, 무엇을 얼리고, 어떻게 만들고, 무엇으로 증명하는가
    ├── ROADMAP.md
    ├── adr/  concept/  design/  analysis/  survey/
    └── evidence/    측정과 비교, 그리고 compare/ 하니스
```
