# CUBRID Test Kit

CUBRID 의 기능 검증을 위한 테스트 툴킷.

## 개요

cubrid-testkit 는 CUBRID 의 기능 테스트 케이스를 실행하기 위한 도구를 제공한다. 테스트 케이스 레포(`cubrid-testcases`, `cubrid-testcases-private`, `cubrid-testcases-private-ex`)에 정의된 케이스들을 다양한 방식(SQL / Shell / JDBC / Isolation / Replication 등)으로 수행하고 결과를 보고한다.

## 문서

- [Roadmap](ROADMAP.md) — 단계별 분석/설계/구현 계획
- [ADRs](adr/) — 주요 의사결정 기록
- [Analysis notes](analysis/) — 모듈별 요구사항·설계·구현 노트
- [Concept](concept/) — 시스템 컨셉 및 외부 인터페이스 정의
- [Design](design/) — 아키텍처 및 모듈 설계

## 디렉터리 구조

```
cubrid-testkit/
├── docs/        프로젝트 문서 (roadmap, ADR, 분석/설계 노트)
└── impl/        구현 코드
```
