# E3 — Logic Bug Detection (SQLancer NoREC + TLP) (Requirements)

*[English](requirements.md) · 한국어*

**Source:** survey/dbms-testing-ecosystem.md §5 + §11
**Status:** incubating (정식 진입 전 — ADR-EXT-003 자리)
**축 매핑:** 축 3 (Logic-bug / semantic testing)
**Companion docs (후속):** `design.md`, `io-contract.md`, `test-corpus.md`, `implementation-notes.md`

---

## 1. 이 확장이 해결하는 문제

CUBRID 의 **optimizer 정확성 / executor semantic correctness / 3-valued logic 처리** 를 *의미 등가 query 쌍* 으로 검증한다.

기존 testkit 의 sql 모듈은 *expected file diff* 만 — 정답이 미리 알려진 쿼리만 본다. SQLancer 류 logic-bug 도구는:
- *정답을 모르는* random query 쌍을 *서로 비교*
- optimizer rewrite 가 의미를 보존하는지 검증
- NULL / UNKNOWN 처리가 일관된지 검증
- pivot 기반으로 행 누락 / 추가 검증

**잡는 버그 종류:** wrong-result. 같은 SQL surface 에서 *내부 plan 차이* 가 결과를 다르게 만드는 종류. parser crash 는 *축 외* (그건 §6a-E2/E5).

---

## 2. 외부 호출 형태 (제안 — incubating)

```
ctp.sh sqlancer [-c <sqlancer.conf>] [--oracle norec|tlp|pqs]
   또는
testkit run sqlancer [-c <conf>] [--oracle <name>] [--seed <N>] [--time <sec>]
```

내부 진입 (의제):
```
SqlancerDriver.exec(config)
  ├─ schema introspect (CUBRID dialect)
  ├─ for each iteration:
  │    case oracle of
  │      NoREC → generate Q1 (predicate); rewrite Q2 (boolean projection); compare rowcount
  │      TLP   → generate base Q; partition into IS TRUE / IS FALSE / IS NULL; compare union
  │      PQS   → pick pivot row; synthesize containing query; verify pivot present
  └─ on mismatch: corpus.save({Q1, Q2, schema, seed})
```

**SUT 구동 채널:** JDBC 또는 CCI.
**외부 표면 동결 영향:** 없음 (신규 진입점).

---

## 3. 사용자 요구사항 (incubating 추정)

1. **NoREC oracle (1차 진입 권장)** — `WHERE p` ↔ `COUNT(*) FROM (SELECT (p) IS TRUE p FROM t) WHERE p` 의 rowcount 비교. *내부 plan 을 알 필요 없음*
2. **TLP oracle (1차 진입 권장)** — 3-valued logic 분할 합집합 검증
3. **PQS oracle (2차 후속)** — pivot 보존 검증 — 도입 비용 가장 높음
4. **CUBRID dialect adapter** — schema introspect / SQL surface generator / 결과 비교 — SQLsmith (E2) 와 *공유 가능* 한 dialect 레이어
5. **mismatch 보고** — 두 query / schema / seed / 실행 결과 / 정규화 후 reduce
6. **regression seed 누적** — 과거 발견된 버그를 새 빌드에서 재실행
7. **time / iteration budget** — CI 안에서 정해진 시간 내 동작

---

## 4. 비기능 요구

| 항목 | 의제 | 새 시스템에서의 의미 |
|------|------|---------------------|
| 도입 비용 | 낮음 (NoREC 1차) ~ 중간 (PQS) | survey §5.4 — NoREC + TLP 만 1차 도입 권장 |
| 즉시 ROI | ★★★★ | testkit 이 비어 있는 *wrong-result 검증* 영역 직접 채움 |
| §6a-E2 (SQLsmith) 와의 관계 | 상보적 — random AST 위에 oracle 만 추가 | dialect adapter 공유 (Open Question 3) |
| 판정 비용 | NoREC < TLP < PQS | 1차는 NoREC. 효율적인 nested AST 생성 깊이 trade-off |
| 라이선스 | SQLancer MIT — vendoring 자유 | survey §12.8 — SQLsmith 보다 license 측면 유리 |
| ADR-001 (구현 언어) 종속 | SQLancer Java | JVM 채택 시 직접 import, 비-JVM 시 subprocess 또는 재구현 |

---

## 5. 의존하는 외부 자원

- **CUBRID 클라이언트** — JDBC / CCI
- **CUBRID system catalog** — schema introspect (E2 와 공유)
- **SQLancer 본체** — github.com/sqlancer/sqlancer (Java/MIT) 또는 재구현
- **dialect adapter** — E2 와 공유. testkit 내부 *공통 dialect 레이어* 가 합리적 (Open Question 3)
- **mismatch corpus storage** — E2 와 동일 정책 (testcases 레포 동결 NG1 점검)

---

## 6. incubating 진입 조건

다음이 결정되어야 ADR-EXT-003 작성 가능 (owner: hgryoo):

1. **1차 oracle 선정** — NoREC 권장 (도입 비용 최저, ROI 최고). TLP 는 동시 또는 후속
2. **dialect adapter 위치** — survey Open Question 3: testkit 내부 *공통 dialect 레이어* (E2 와 공유) vs 도구별 분산
3. **재사용 vs 재구현** — SQLancer Java 본체 직접 사용 vs ADR-001 결정 언어로 재구현
4. **mismatch corpus 위치** — E2 와 동일 정책 (NG1 점검)
5. **CI 통합 모드** — short oracle pass 매 PR vs nightly long oracle pass
6. **PQS / SQLaser / SQLancer++ 후속 로드맵** — 1차 도입 후 어느 순서로 확장
7. **strangler-fig 분기 게이트 §7 충돌 점검** — Phase 3·4 1차 대체와 자원 충돌

**ADR placeholder:**
- ADR-EXT-003 — 1차 oracle 선정 (NoREC + TLP) + dialect adapter 위치 + 재사용/재구현 + corpus 정책

---

## 7. 위험 / 정합성 메모

- **NG1 충돌 가능** — mismatch corpus 가 testcases 에 들어가면 동결 위반. 외부 storage 권장
- **NG2 충돌 없음** — 신규 진입점
- **NG4 충돌 없음** — CUBRID 가 SUT
- **§6a-E2 (SQLsmith) + E3 (SQLancer) 결합** — PostgreSQL ecosystem *de facto* 모범. 함께 도입이 정합적
- **roadmap repo cross-cutting 후보 (번호 미배정)** — survey §13: testkit §6a-E3 × {N27 lock-manager, N28 mvcc, N29 page-buffer, N30 log-buffer} — 회귀가 아니라 *정합성 검증* 채널. ~~C-013~~ 은 2026-05-13 에 lock-manager × wait-event-stats 로 등록되어 쓸 수 없다
- **SQLancer++ (adaptive grammar)** — niche DBMS 인 CUBRID 에 *장기적으로* 의미. 현재는 연구 단계 — 안정성 확인 후 별도 ADR
