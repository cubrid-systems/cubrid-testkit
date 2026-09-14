# E2 — Random SQL Fuzzing (SQLsmith 포팅) (Requirements)

**Source:** survey/dbms-testing-ecosystem.md §4 + §11 (ROADMAP §6a 카탈로그 신규 후보)
**Status:** incubating (정식 진입 전 — ADR-EXT-002 자리)
**축 매핑:** 축 2 (Random SQL generation)
**Companion docs (후속):** `design.md`, `io-contract.md`, `test-corpus.md`, `implementation-notes.md`

---

## 1. 이 확장이 해결하는 문제

CUBRID 의 **parser / planner / executor 강건성** 을 *valid 하지만 광범위한 random SQL* 로 검증한다.

기존 testkit (sql / medium / shell / isolation) 에는 *random generation 축이 부재*. 케이스 작성자가 손으로 만든 케이스만 회귀하므로 다음 영역이 사각지대:
- parser crash / UB
- planner assert / stack overflow
- executor segfault / OOM
- 깊게 nested 된 expression / window / lateral / subquery 조합에서의 internal state corruption

SQLsmith 는 PostgreSQL ecosystem 에서 *100+ 버그* 의 출처. 동작:
1. 대상 DB schema introspect
2. type-correct random AST 생성 (parser 통과 = valid SQL)
3. 임의 깊이의 nested expression / window / lateral / subquery
4. crash / signal / process exit 만으로 판정 (oracle 없음)

**잡는 버그 종류:** crash·assert·UB. *wrong-result 는 못 잡음* (그 영역은 §6a-E3).

---

## 2. 외부 호출 형태 (제안 — incubating)

```
ctp.sh sqlsmith [-c <sqlsmith.conf>]
   또는
testkit run sqlsmith [-c <conf>] [--seed <N>] [--time <sec>] [--max-depth <D>]
```

내부 진입 (의제):
```
SqlsmithDriver.exec(config)
  ├─ schema introspect (CUBRID dialect — information_schema 또는 db_class/db_attribute)
  ├─ AST generator loop:
  │    while time_left:
  │      query = generate(seed, depth)
  │      run(query) → {ok | empty | error | CRASH}
  │      if CRASH: corpus.save(query, seed, stack)
  └─ report: { runs, crashes, top stack hashes }
```

**SUT 구동 채널:** JDBC 또는 CCI (case-format ingestion 인터페이스 공유).
**외부 표면 동결 영향:** 없음 (신규 진입점).

---

## 3. 사용자 요구사항 (incubating 추정)

1. **CUBRID schema introspect** — db_class / db_attribute / db_index 등 CUBRID system catalog 로 SQLsmith 의 PostgreSQL information_schema 의존을 대체
2. **CUBRID 확장 SQL 생성 옵션** — object-oriented constructs (path expression, class hierarchy), serial, hierarchical query (CONNECT BY), method 등 *CUBRID dialect 가산*
3. **crash detection** — process signal (SIGSEGV/SIGABRT) + cubrid server connection 끊김 + core file 발견
4. **corpus 누적** — crash 유발 query 를 *재현 가능 seed + 정규화된 query text* 로 보관
5. **stack hash dedup** — 같은 stack 의 여러 crash 를 1 항목으로 묶음
6. **time / iteration / depth budget** — CI 안에서 정해진 시간 내 fuzzing
7. **continue 모드** — corpus 의 과거 crash query 를 새 빌드에서 재실행 (regression seed 기능)

---

## 4. 비기능 요구

| 항목 | 의제 | 새 시스템에서의 의미 |
|------|------|---------------------|
| 도입 비용 | 낮음 (schema introspect 경로만 CUBRID 화) | survey §4 결론 — 즉시 후보 |
| 즉시 ROI | ★★★★ | testkit 이 비어 있는 영역 직접 채움 |
| §6a-E3 (SQLancer) 와의 관계 | 상보적 (SQLancer 는 wrong-result, SQLsmith 는 crash) | hybrid CI (E8) 시 함께 도입 |
| AST nesting 깊이 | SQLsmith 강점 (deep) | SQLancer 보다 *더 깊은 AST* 가 핵심 자산 |
| 라이선스 | SQLsmith custom — vendoring 정책 점검 필요 | ROADMAP §8 risk 6 동일 패턴 |
| testcases 레포 동결 (NG1) | 충돌 없음 (corpus 외부 보관) | 단, corpus 위치 ADR 필요 |

---

## 5. 의존하는 외부 자원

- **CUBRID 클라이언트** — JDBC / CCI 중 SUT 구동 채널
- **CUBRID system catalog** — db_class / db_attribute / db_serial / db_method 등
- **SQLsmith 본체** — github.com/anse1/sqlsmith (C++) 또는 *재구현* (ADR-001 구현 언어 결정에 종속)
- **core dump 인프라** — sql / isolation 모듈과 *core 정책 공유* 필요 (analysis/sql/requirements.md §4 의 core file 처리 ADR 와 결합)
- **fuzz corpus storage** — testkit 내부 또는 별도 storage (ADR-EXT-002 입력)

---

## 6. incubating 진입 조건

다음이 결정되어야 ADR-EXT-002 작성 가능 (owner: hgryoo):

1. **재사용 vs 재구현** — SQLsmith C++ 본체를 *subprocess* 로 사용할지, ADR-001 결정 언어로 *재구현* 할지
2. **dialect 가산 범위** — CUBRID 확장 SQL (path expression / serial / connect-by / method) 을 어디까지 random AST 에 포함
3. **fuzz corpus 위치** — testkit 내부 / cubrid 본 repo / 별도 storage. testcases 레포 동결 (NG1) 위반 여부 점검
4. **crash 판정 채널** — signal vs core file vs cubrid server log — 단일 채널 확정
5. **CI 통합 모드** — 매 PR 마다 short fuzz vs nightly long fuzz vs 별도 lane
6. **§6a-E3 (SQLancer) 와 dialect adapter 공유** — survey Open Question 3 (dialect adapter 위치) 와 결합
7. **strangler-fig 분기 게이트 §7 충돌 점검** — Phase 3·4 1차 대체와 자원 충돌 시 strangler-fig 우선

**ADR placeholder:**
- ADR-EXT-002 — SQLsmith 재사용/재구현 선택 + dialect 가산 범위 + corpus 위치 + crash 판정 채널

---

## 7. 위험 / 정합성 메모

- **NG1 충돌 가능** — fuzz corpus 가 testcases 레포에 들어가면 동결 위반. *외부 corpus storage* 가 안전
- **NG2 충돌 없음** — 신규 진입점
- **NG4 충돌 없음** — CUBRID 가 SUT
- **§6a-E3 (SQLancer) + E2 (SQLsmith) 결합** — survey §4.4 / §11: PostgreSQL ecosystem 의 *de facto* 모범. 함께 도입 권장
- **§6a-E5 (parser fuzzing) 와 보완** — SQLsmith 는 *문법 내 random*; libFuzzer 는 *문법 외 byte-level*. 둘 다 parser crash 를 잡지만 영역이 다름
