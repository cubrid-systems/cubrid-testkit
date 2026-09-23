# E7 — Design (STUB)

*[English](design.md) · 한국어*

**Status:** STUB — 정식 design 은 ADR-EXT-007 incubating 정식 진입 후. C-004 책임 경계 정의 선결.
**Source:** ROADMAP §6a-E7, `requirements.md`, `project/survey/dbms-testing-ecosystem.md` §9
**Companion:** `requirements.md` (FULL), `io-contract.md` (STUB), `test-corpus.md` (STUB)

---

## 1. 책임 경계 (C-004 선결)

```
testkit (이 모듈)                    engine-suite (HammerDB / benchbase)
─────────────                       ──────────────────────────────────
correctness invariant 검증     ↔    throughput 측정
randomized fault injection           workload 발생기
deterministic seed replay            performance regression
```

C-004 cross-cutting 결론 *없이는 시작 불가*.

## 2. 모듈 위치 (의제)

```
internal/runner/workload/
   ├── topology/         # cluster deploy (E4 와 공유)
   ├── workload/         # KV-style 또는 SQL-style stateful tx generator
   ├── fault/            # E4 와 공유 가능 — owner ADR 필요
   ├── invariant/        # invariant 카탈로그 + checker
   ├── history/          # tx / fault timeline 기록
   └── corpus/           # violation 보관
```

## 3. 데이터 흐름 (의제)

```
seed → topology.deploy()
       └─ workload.start(generators)  ┐
                                       ├─ history.record()
       └─ fault.inject(timeline)       ┘
                                          └─ periodically: invariant.check(state)
                                               └─ violation? corpus.save({seed, fault, witness})
```

## 4. invariant 카탈로그 (의제)

| invariant | 검증 SQL/method | 비고 |
|---|---|---|
| row_count_consistency | SELECT COUNT(*) — replica 간 일치 | replication 검증 |
| referential_integrity | FK violation 0 | concurrent DDL/DML |
| sum_conservation | 송금 시나리오 잔액 sum 보존 | linearizability proxy |
| monotonicity | seq 단조 증가 | snapshot isolation 검증 |
| no_phantom_after_failover | failover 직후 phantom 부재 | HA 검증 |

ADR-EXT-007 에서 1차 invariant 선정.

## 5. scenario 1차 후보 (의제)

| scenario | 출처 | 도입 비용 | 즉시 ROI |
|---|---|---|---|
| roachtest 류 randomized | CockroachDB | 중 | ★★★ |
| FoundationDB 류 deterministic simulation | FoundationDB | 매우 높음 | ★★ (장기) |
| benchbase / HammerDB 위에 invariant 얹기 | engine-suite | 낮음 | ★★ (의존성 ↑) |

ADR-EXT-007 에서 scenario 1차 선정.

## 6. 외부 의존

- C-004 cross-cutting 결론 (선결)
- engine-suite 자산 (HammerDB / benchbase) — 재사용 범위 결정
- E4 의 fault injection (공유)
- CUBRID HA / streaming-replication

## 7. 결정 보류 항목 → ADR-EXT-007

- scenario 1차 (roachtest / simulation / benchbase 결합)
- invariant 카탈로그 1차 항목
- engine-suite 자산 재사용 범위
- E4 와 fault injector 공유 owner
- violation corpus 위치 (NG1 점검)

## 8. design 작성 트리거

C-004 결론 + ADR-EXT-007 후. 현 시점 stub.
