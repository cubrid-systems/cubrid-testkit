# CUBRID Test Kit

CUBRID 의 기능 검증을 위한 테스트 툴킷.

## 개요

cubrid-testkit 는 CUBRID 의 기능 테스트 케이스를 실행하기 위한 도구를 제공한다. 테스트 케이스 레포(`cubrid-testcases`, `cubrid-testcases-private`, `cubrid-testcases-private-ex`)에 정의된 케이스들을 다양한 방식(SQL / Shell / JDBC / Isolation / Replication 등)으로 수행하고 결과를 보고한다.

## 현재 위치

**Phase 2 완료** (2026-09-02). 구현 언어 **Go**, 1차 대체 대상 **shell 모듈**(+ rqg / unittest / jdbc). 다음은 Phase 3 착수.

**프로젝트 방향성:** 테스트 실행 축(T)과 QA 운영 축(O)을 분리한다. 이번 마이그레이션은 축 T 만 옮기고, 축 O 는 제외 기록 후 나중에 새 층으로 세운다 → [migration-exclusions.md](concept/migration-exclusions.md)

## 문서

- [Roadmap](ROADMAP.md) — 단계별 분석/설계/구현 계획
- [ADR 인덱스](adr/README.md) — 번호 배정의 단일 출처. 확정 5건 / 예약 7건
- **Concept (Phase 1 산출물)**
  - [North Star](concept/north-star.md) — 새 시스템의 정체성
  - [External Surface Freeze](concept/external-surface-freeze.md) — 동결 명세 + 신↔구 1:1 매핑 표
  - [Non-Goals](concept/non-goals.md) — 의도적으로 하지 않는 것 (NG1~NG11)
  - [Migration Exclusions](concept/migration-exclusions.md) — **축 T/O 분리 원칙** + 마이그레이션 제외 목록
  - [Phase 0 회고](concept/phase0-retrospective.md) — 게이트 통과 기록
- [Analysis notes](analysis/) — Phase 0 산출물. 모듈별 요구사항·설계·io-contract
- [Extensions](extensions/) — §6a 확장 영역 E1~E10 (incubating)
- [Survey](survey/dbms-testing-ecosystem.md) — DBMS 테스팅 생태계 8축 분류
- **Design (Phase 2 산출물)**
  - [Architecture](design/architecture.md) — 패키지 구조 · 실행 모델 · 결과 파이프라인
  - [Contracts](design/contracts.md) — 계약 5개와 그 경계
  - [module-shell](design/module-shell.md) — 1차 대체 대상, 35 클래스 매핑
  - [module-sql](design/module-sql.md) · [module-isolation](design/module-isolation.md) · [module-medium](design/module-medium.md)

- **Category (as-built 운영 가이드)** — 돌리는 사람을 위한 문서. 설계가 아니라 지금 코드가 하는 일
  - [category/shell](category/shell/README.md) — 단계·슬롯·메모리 ceiling·모든 키
  - [category/sql](category/sql/README.md) — sql·medium: 단계와 실행부, 키와 스위치, 슬롯이 사 주는 것과 그 값, 실패 읽는 법
- **Evidence (측정과 비교)** — 주장마다 근거. 숫자는 전부 실제 run 에서 나온다
  - [regression-shell](evidence/regression-shell.md) · [parallel-shell](evidence/parallel-shell.md) — shell
  - [sql-baseline](evidence/sql-baseline.md) · [sql-native](evidence/sql-native.md) · [regression-sql](evidence/regression-sql.md) — sql·medium (ADR-017 게이트 포함)
  - [compare/](evidence/compare/README.md) — 정규화 후 비교하는 방법 (ADR-013)

## 디렉터리 구조

```
cubrid-testkit/
├── docs/        프로젝트 문서 (roadmap, ADR, 분석/설계 노트, 운영 가이드, 증거)
├── cmd/testkit/ 진입점
├── internal/    구현
├── patches/     코퍼스가 고쳐지기 전까지 run 이 들고 다니는 diff
└── tools/       sizing.sh — 이 기계가 한 번에 얼마나 돌릴 수 있는지 재서 답한다
```
