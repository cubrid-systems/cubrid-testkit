# E7 — Stateful / Randomized Workload Testing (Requirements)

*[English](requirements.md) · 한국어*

**Source:** survey/dbms-testing-ecosystem.md §9 + §11
**Status:** incubating (조건부 — engine-suite 책임 경계 (C-004) 선결)
**축 매핑:** 축 7 (Stateful / workload testing)
**Companion docs (후속):** `design.md`, `io-contract.md`, `test-corpus.md`, `implementation-notes.md`

---

## 1. 이 확장이 해결하는 문제

CUBRID 의 **long-running invariant** 를 randomized schema mutation · node restart · partition · failover 가 동시 진행되는 시나리오에서 검증한다.

기존 testkit 의 모든 모듈은 *비교적 짧은 결정적 시나리오* 만 본다. 본 항목이 다루는 영역:
- random schema mutation (DDL during DML)
- node restart / kill 와 결합된 recovery invariant
- range split / merge / rebalance 류 운영 동작
- failover / partition 직후의 invariant
- crash/recovery 축 + 분산 axis 결합

대표 사례:
- **CockroachDB roachtest** — randomized testing, long-running invariant
- **FoundationDB simulation testing** — deterministic simulation (fake network/disk/clock)

**잡는 버그 종류:** invariant violation, race, partial failure, clock-related, ordering. *production reproduce 불가능* 한 결함을 *재현 가능* 하게.

---

## 2. 외부 호출 형태 (제안 — incubating)

```
ctp.sh workload [-c <workload.conf>] [--scenario roach|sim] [--time <sec>]
   또는
testkit run workload --scenario <name> [--seed <N>] [--invariants <list>]
```

내부 진입 (의제):
```
WorkloadDriver.exec(config)
  ├─ deploy cluster (HA / streaming-replication)
  ├─ start workload generators (KV-style or SQL-style stateful)
  ├─ start fault injector (random kill / partition / clock skew / DDL during DML)
  ├─ run for budget (time / iteration)
  ├─ check invariants periodically:
  │    no row count anomaly · no monotonicity violation · referential integrity
  └─ on violation: corpus.save({seed, fault sequence, witness})
```

**SUT 구동 채널:** 다중 클라이언트 + 다중 cubrid 노드.
**외부 표면 동결 영향:** 없음 (신규 진입점).

---

## 3. 사용자 요구사항 (incubating 추정)

1. **stateful workload generator** — KV-style 또는 SQL-style 의 randomized 트랜잭션
2. **invariant 카탈로그** — row count / monotonicity / referential integrity / sum-conservation 등
3. **fault injector** — process kill / network partition / clock skew / DDL-during-DML
4. **deterministic seed** — 같은 seed → 같은 fault sequence → 재현 가능
5. **long-running budget** — CI 안에서 수 분 ~ nightly 의 long run 까지
6. **violation 보고** — seed / fault sequence / witness query / 재현 스크립트
7. **engine-suite 와의 분담** — testkit = correctness invariant / engine-suite = throughput

---

## 4. 비기능 요구

| 항목 | 의제 | 새 시스템에서의 의미 |
|------|------|---------------------|
| 도입 비용 | 높음 (인프라 + invariant 카탈로그) | survey §9.3 — *후순위* |
| 즉시 ROI | ★★ | engine-suite 와의 책임 경계 정의가 선결 |
| C-004 (cross-cutting) 종속 | testkit × engine-suite 경계 | survey §9.3 — 본 repo 의 *연속선* |
| FoundationDB simulation 컨셉 | 직접 흡수 비현실적 | *deterministic harness* 컨셉만 차용 (장기) |
| §6a-E4 (distributed isolation) 와의 중첩 | fault injection 인프라 공유 | 어느 항목이 host 인지 ADR 필요 |
| HammerDB / benchbase 와의 분담 | engine-suite 측 자산 | testkit 책임 = correctness, engine-suite = throughput |

---

## 5. 의존하는 외부 자원

- **CUBRID HA / streaming-replication 인프라** — 다중 노드
- **engine-suite (HammerDB / benchbase)** — long-running workload 발생기 자산. *책임 경계가 선결*
- **fault injection 레이어** — iptables / cgroup / process signal (E4 와 공유)
- **invariant 카탈로그** — testkit 자체 자산
- **deterministic harness (장기)** — FoundationDB simulation 컨셉 차용 시 필요

---

## 6. incubating 진입 조건 (조건부)

다음이 충족된 후 정식 incubating 진입 (owner: hgryoo):

1. **C-004 책임 경계 정의 선결** — testkit (correctness) ↔ engine-suite (throughput) 경계가 ADR 화
2. **scenario 1차 선정** — roachtest 류 randomized vs FoundationDB 류 deterministic simulation. 도입 비용 차이 큼
3. **invariant 카탈로그 시드** — row count / monotonicity / referential integrity 등 1차 invariant 목록
4. **fault injector 채널** — E4 와 공유할지 / 별 구현 — 단일 채널 권장
5. **engine-suite 자산 재사용 범위** — HammerDB / benchbase 의 어느 layer 까지 testkit 가 호출할지
6. **분기 게이트 §7 충돌 점검** — strangler-fig 우선원칙. 1인 가용성 초과 시 *후순위*
7. **violation corpus 위치** — NG1 점검

**ADR placeholder:**
- ADR-EXT-007 — scenario 1차 선정 + invariant 카탈로그 + engine-suite 책임 경계 (C-004 와 결합) + corpus 위치

---

## 7. 위험 / 정합성 메모

- **engine-suite 책임 경계 미정 위험** — survey §9.3: testkit 단독 결정 불가. C-004 cross-cutting 정합 선결
- **§6a-E4 (distributed isolation) 와 중첩** — fault injection 인프라 공유. 어느 항목이 owner 인가 ADR 필요
- **NG1 충돌 가능** — violation corpus 가 testcases 에 들어가면 동결 위반. 외부 storage 권장
- **NG2 / NG4 충돌 없음**
- **분기 게이트 §7** — *후순위 권고* (survey §11). 1인 가용성 + strangler-fig 우선원칙 (ROADMAP §8 risk 7)
- **FoundationDB simulation 컨셉** — 장기 비전. 본 항목 1차 진입 시점에는 roachtest 류 randomized 가 현실적
