# E1 — sqllogictest 적용 (Requirements)

*[English](requirements.md) · 한국어*

**Source:** ROADMAP §6a-E1 + survey/dbms-testing-ecosystem.md §3
**Status:** incubating (정식 진입 전 — ADR-EXT-001 대기)
**축 매핑:** 축 1 (sqllogictest 계열, 정답 회귀)
**Companion docs (후속):** `design.md`, `io-contract.md`, `test-corpus.md`, `implementation-notes.md`

---

## 1. 이 확장이 해결하는 문제

CUBRID 의 SQL 의미를 **외부 표준 포맷 (sqllogictest)** 의 결정적 input → 사전 기록된 expected output diff 로 회귀 검증한다.

기존 `sql` 모듈의 `.sql` ↔ `.answer` 패턴과 *판정 모델은 동일* 하지만 다음이 추가됨:
- **외부 코퍼스 자산 활용** — SQLite·DuckDB·CockroachDB·RisingWave 가 채택한 포맷이라 *코퍼스 자체가 자산*
- **hash 기반 결과 표현** — 큰 결과셋도 단일 hash 라인으로 표현 (정답 파일 크기 ↓)
- **cross-DBMS 회귀 가능 표준** — N13 pg-wire-compat 등 dialect 호환 작업의 *판정 채널*

**잡는 버그 종류:** 정답 회귀 (deterministic input → wrong output). parser crash·logic-bug 는 *축 외*.

---

## 2. 외부 호출 형태 (제안 — incubating)

```
ctp.sh sqllogictest [-c <sqllogictest.conf>]
   또는
testkit run sqllogictest [-c <conf>]
```

내부 진입 (의제):
```
SqllogictestRunner.exec(config)
  └─ for each <case>.slt:
       parse record (statement / query)
       execute via {JDBC | CCI | cubrid-cli}
       compare hash or values vs expected
```

**외부 표면 동결 영향:** 없음. sqllogictest 는 *신규 진입점* 이므로 ROADMAP NG2 (외부 표면 동결) 와 직교.

---

## 3. 사용자 요구사항 (incubating 추정)

1. **sqllogictest spec 호환 record 파싱** — `statement (ok|error)` / `query <type> [sort] [label]` 두 record 타입
2. **hash 비교 모드** — 표준 sqllogictest hash (rows MD5)
3. **values 비교 모드** — 작은 결과셋의 raw value diff
4. **결정성 옵션** — `sort rowsort|valuesort|nosort` 처리
5. **비결정성 case 격리** — float 정밀도, ORDER BY 없는 SELECT 등 *통과 case 표시*
6. **외부 코퍼스 ingestion** — SQLite 발 sqllogictest 트리 또는 DuckDB suite 의 *부분집합* 을 testkit 코퍼스에 흡수
7. **회귀 동등성 보고** — pass / fail / hash mismatch / dialect-skip 분류

---

## 4. 비기능 요구

| 항목 | 의제 | 새 시스템에서의 의미 |
|------|------|---------------------|
| 도입 비용 | 낮음 (record parser + hash + diff) | ROADMAP §6a-E1 *최저 의존 후보* |
| 즉시 ROI | ★★★★ | 외부 코퍼스가 즉시 사용 가능 |
| ADR-001 (구현 언어) 종속 | sqllogictest-rs 채택 시 Rust 강제 | ADR-001 결정 후 *implementation choice* |
| 비결정 결과 표준화 | float·ORDER BY 없는 SELECT | 케이스 작성자 가이드 + skip 정책 |
| 라이선스 | sqllogictest 코퍼스 라이선스 점검 필요 | full mirror 대신 *부분집합 vendor in* (ROADMAP §8 risk 6) |

---

## 5. 의존하는 외부 자원

- **외부 코퍼스** — SQLite sqllogictest 트리 / DuckDB test suite / CockroachDB logictest 중 primary target (미정)
- **CUBRID 클라이언트** — JDBC / CCI / cubrid-cli 중 SUT 구동 채널 (미정)
- **ADR-001 (구현 언어)** — sqllogictest-rs 채택 가능성과 결합
- **case-format ingestion 인터페이스** (design/contracts.md, Phase 2) — 본 항목이 *처음으로 요구하는* 공통 인터페이스

---

## 6. incubating 진입 조건 (ROADMAP §6a-E1 Open Questions)

다음이 결정되어야 ADR-EXT-001 작성 가능 (owner: hgryoo):

1. **Pain point** — 왜 지금 sqllogictest? (외부 표준 진입 / 다른 DBMS 와의 회귀 비교 / 테스트 코퍼스 확장 / 특정 RND·CBRD 티켓?)
2. **Spec target** — SQLite 원형 / DuckDB 확장 / CockroachDB 변형 중 baseline
3. **코퍼스 정책** — 외부 트리 import 또는 mirror, 라이선스 검증
4. **Acceptance** — 통과 case 수 / hash 일치율 / coverage 등 측정 기준
5. **결과 비교 모드** — sqllogictest 표준 hash vs CUBRID expected 파일 추가
6. **SUT 구동 클라이언트** — JDBC / CCI / cubrid-cli
7. **Phase 정합 재확인** — Phase 4·5 병행이 1인 가용성 초과 여부 (분기 게이트 §7)

**ADR placeholder:**
- ADR-EXT-001 — sqllogictest spec variant 선정 + 입력 코퍼스 import 정책 + 결과 비교 모드 + SUT 구동 클라이언트

---

## 7. 위험 / 정합성 메모

- **NG1 (testcases 레포 동결) 충돌 없음** — 외부 코퍼스는 testcases 외부 자산
- **NG2 (외부 표면 동결) 충돌 없음** — 신규 진입점
- **NG4 (비-CUBRID DBMS 호환 금지) 충돌 없음** — CUBRID 가 SUT 측이지, testkit 이 다른 DBMS 호환을 추가하는 것이 아님
- **분기 게이트 §7 충돌 가능** — strangler-fig Phase 3·4 와 자원 충돌 시 strangler-fig 우선 (ROADMAP §8 risk 7)
- **§6a-E5 (parser fuzzing) 와 보완** — sqllogictest 는 *문법 통과 SQL* 의 wrong-result 검증; parser crash 는 별 축
