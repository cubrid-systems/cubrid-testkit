# cubrid-testkit

The successor to CTP (CUBRID Test Program) -- a strangler-fig rewrite in progress.

**Status:** Phase 0 -- Analysis

---

## 프로젝트 개요

`cubrid-testkit`은 기존 CTP(CUBRID Test Program)를 **strangler-fig** 방식으로 점진 대체하기 위한 후속 프로젝트입니다.
기존 시스템을 일시에 교체(big-bang)하지 않고, 모듈 단위로 신시스템으로 이전하는 전략을 택합니다.

## cubrid-testtools와의 관계

- **원본 레포**: [cubrid-testtools](https://github.com/CUBRID/cubrid-testtools)
- `cubrid-testtools`는 마이그레이션 기간(Phase 0~4) 동안 **계속 운영**됩니다.
- 이 레포(`cubrid-testkit`)는 분석/설계/구현 산출물이 모이는 장소이며, 신시스템의 코드베이스가 됩니다.
- Phase 5에서 기존 CTP의 호출되지 않는 코드를 격리/폐기 결정합니다.

## 인터페이스 동결 의무

`bin/ctp.sh` CLI, `conf/*.conf` 스키마, 출력 포맷은 1차 대체(Phase 3) 시점까지 동결.

기존 `cubrid-testcases`, `cubrid-testcases-private(-ex)` 레포는 **수정 불가 자산**으로 간주합니다.
새 시스템은 기존 케이스 포맷을 그대로 읽어야 합니다.

## 문서 안내

- **[ROADMAP.md](ROADMAP.md)** — 6-Phase 상세 로드맵 (Phase 0 즉시 착수 가능한 수준으로 전개)
- **[adr/](adr/)** — Architecture Decision Records (ADR-000 ~ ADR-006 예정)

## 현재 상태

**Phase 0 -- 분석** 단계.

이 레포는 현재 계획/분석 산출물만 포함합니다. `analysis/` 디렉터리 아래에 5개 심도 모듈(medium, sql, shell, isolation, common) 분석 산출물과 5개 인벤토리 모듈의 표층 스냅샷이 작성될 예정입니다.

Phase 0 진행 상황은 `ROADMAP.md`의 체크리스트 섹션에서 추적합니다.
