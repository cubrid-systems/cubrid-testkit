# E6 — Differential Testing (PostgreSQL Pair) (Requirements)

**Source:** survey/dbms-testing-ecosystem.md §8 + §11
**Status:** incubating (조건부 — N13 pg-wire-compat selected 이상 권장)
**축 매핑:** 축 6 (Differential testing)
**Companion docs (후속):** `design.md`, `io-contract.md`, `test-corpus.md`, `implementation-notes.md`

---

## 1. 이 확장이 해결하는 문제

CUBRID 의 SQL 결과를 **외부 DBMS (1차 PostgreSQL) 와 짝지어** 같은 입력에서의 *결과 차이* 로 검증한다.

판정 기준이 *외부 DBMS 자체* — testkit 안에 oracle 을 만들 필요가 없다. 사례:
- RAGS (Microsoft) — random SQL → 다중 DBMS 결과 비교
- SQLancer differential mode
- CockroachDB ↔ PostgreSQL differential CI

**잡는 버그 종류:** wrong-result. *CUBRID 만 다르게 동작* 하는 경우. 단, dialect mismatch 가 노이즈를 낳음.

---

## 2. 외부 호출 형태 (제안 — incubating)

```
ctp.sh diff-pg [-c <diff.conf>] [--peer pg|other] [--mode canonical|rewrite]
   또는
testkit run differential --peer <name> [--cases <corpus>] [--seed <N>]
```

내부 진입 (의제):
```
DifferentialDriver.exec(config)
  ├─ load case corpus (sqllogictest .slt or sqlsmith generated)
  ├─ for each query Q:
  │    R_cubrid = run(Q on CUBRID via JDBC/CCI/pg-wire)
  │    R_peer   = run(Q on PostgreSQL via JDBC)
  │    case mode of
  │      canonical → expect identical result on canonical SQL subset
  │      rewrite   → apply dialect rewrite layer, then compare
  └─ on diff: corpus.save({Q, R_cubrid, R_peer, schema, seed})
```

**SUT 구동 채널:** N13 pg-wire-compat selected 이상이면 *PostgreSQL driver 직결* 가능. 미진척 시 JDBC + 별도 PostgreSQL 클라이언트 분기.
**외부 표면 동결 영향:** 없음 (신규 진입점).

---

## 3. 사용자 요구사항 (incubating 추정)

1. **peer DBMS 1차 = PostgreSQL** — N13 pg-wire-compat 와 자연스럽게 결합
2. **canonical subset 모드** — 모든 DBMS 가 동일하게 정의하는 SQL-92 core 등 *canonical 부분집합* 만 비교
3. **rewrite layer 모드** — DATE 함수 / NULL 정렬 / 부동소수점 / 정수 overflow / 문자열 collation / JSON 함수 등 dialect 차이를 *rewrite* 로 흡수
4. **dialect mismatch 분류** — *진짜 wrong-result* 와 *알려진 dialect 차이* 를 자동 분리
5. **케이스 코퍼스 입력** — sqllogictest (E1) 또는 sqlsmith generated (E2) 를 입력으로
6. **diff 보고** — query / 결과 양쪽 / dialect classification / regression seed
7. **regression seed 누적** — 과거 발견된 진짜 wrong-result 를 새 빌드에서 재실행

---

## 4. 비기능 요구

| 항목 | 의제 | 새 시스템에서의 의미 |
|------|------|---------------------|
| 도입 비용 | 중 (rewrite layer 가 핵심 비용) | survey §8.2 — *dialect mismatch 지옥* |
| 즉시 ROI | ★★★ (N13 후) | N13 미진척 시 ROI 작음 |
| N13 pg-wire-compat 종속 | selected 이상이면 비용 급감 | roadmap repo cross-cutting C-014 후보 |
| canonical subset 만 | rewrite layer 없이도 가능 | 단, 다룰 수 있는 SQL 범위가 좁음 |
| 라이선스 | PostgreSQL JDBC BSD 류 / 자유 | 외부 DBMS 자체 라이선스 점검 필요 |
| §6a-E1 (sqllogictest) 와의 결합 | sqllogictest cross-DB 강점 활용 | E1 + E6 자연스러운 조합 |

---

## 5. 의존하는 외부 자원

- **PostgreSQL 인스턴스** — 동일 또는 인접 호스트
- **PostgreSQL JDBC driver** — peer 측 query 실행
- **N13 pg-wire-compat 진척** — roadmap repo. selected 이상이면 CUBRID 측도 PostgreSQL driver 로 접속 가능
- **dialect rewrite layer** — DATE / NULL ordering / collation / JSON / 정수 overflow 등의 dialect 차이 흡수
- **케이스 코퍼스** — E1 (sqllogictest) 또는 E2 (sqlsmith) 의 출력

---

## 6. incubating 진입 조건 (조건부)

다음이 충족된 후 정식 incubating 진입 (owner: hgryoo):

1. **N13 pg-wire-compat selected 이상** — survey §8.3 결론: 우선순위 N13 selected 이후
2. **mode 1차 선정** — canonical subset (저비용) vs rewrite layer (고비용·고가치)
3. **dialect rewrite catalog** — DATE / NULL / float / collation / JSON / overflow 카테고리별 rewrite 정책
4. **peer DBMS 범위** — 1차 PostgreSQL 만 / + MySQL / + SQLite
5. **케이스 코퍼스 input source** — E1 / E2 / 직접 작성
6. **C-014 (roadmap cross-cutting)** — survey §13: testkit §6a-E6 × N13 pg-wire-compat
7. **regression seed 보관 정책** — NG1 점검

**ADR placeholder:**
- ADR-EXT-006 — peer DBMS 1차 선정 + mode (canonical vs rewrite) + dialect rewrite catalog 정책 + corpus 위치

---

## 7. 위험 / 정합성 메모

- **선결 의존 위반 위험** — N13 pg-wire-compat 미진척 시 ROI 작음 → empty value 위험. 분기 게이트 §7 후순위
- **dialect mismatch 노이즈** — survey §8.2 — false positive 가 많으면 신뢰 ↓. rewrite layer 또는 canonical subset 으로 통제
- **§6a-E1 (sqllogictest) 와의 결합** — sqllogictest 는 cross-DB 회귀에 적합, E6 는 *판정 기준이 외부 DBMS* — 결합 자연스러움
- **NG1 / NG2 충돌 없음**
- **NG4 (비-CUBRID DBMS 호환 금지) 점검** — *CUBRID 가 다른 DBMS 호환을 추가* 하는 것이 아니라, *CUBRID 를 외부 DBMS 와 비교하여 검증* 하는 것이므로 NG4 와 충돌하지 않음
