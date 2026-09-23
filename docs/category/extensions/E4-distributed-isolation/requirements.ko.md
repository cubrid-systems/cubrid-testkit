# E4 — Distributed Isolation Testing (AWDIT / Jepsen) (Requirements)

*[English](requirements.md) · 한국어*

**Source:** survey/dbms-testing-ecosystem.md §6 + §11
**Status:** incubating (조건부 — N24 streaming-replication / N11 graduation 대기)
**축 매핑:** 축 4 (Isolation / transaction testing — 분산 부분)
**Companion docs (후속):** `design.md`, `io-contract.md`, `test-corpus.md`, `implementation-notes.md`

---

## 1. 이 확장이 해결하는 문제

CUBRID 의 **분산 / HA / streaming-replication 환경에서의 isolation anomaly** 를 history graph 또는 *외부 spec 위반 탐지* 로 검증한다.

기존 testkit `isolation` 모듈 (analysis/isolation) 은 *단일 노드의 다중 클라이언트* 만 다룸. 분산 axis 가 들어오면 다음이 사각지대:
- network partition / clock skew / process kill 하 anomaly
- weak isolation 의 long-running history 에서의 cycle
- replicated read 에서의 causal / linearizability 위반
- HA failover 직후의 visibility 위반

축 4 의 *단일 노드* 부분 (PostgreSQL `.spec` 포맷 + Hermitage 카탈로그) 은 strangler-fig isolation 모듈에 흡수 (ROADMAP Phase 3 ADR-004 검토 시). 본 항목은 *분산 부분만* 다룬다.

**잡는 버그 종류:** anomaly·linearizability·causal·snapshot isolation 위반. 단일 노드 격리 동작 자체는 *축 외* (그건 기존 isolation 모듈).

---

## 2. 외부 호출 형태 (제안 — incubating)

```
ctp.sh isolation-dist [-c <isolation-dist.conf>] [--mode awdit|jepsen]
   또는
testkit run isolation-dist --mode <name> [--topology <ha|streaming>] [--time <sec>]
```

내부 진입 (의제):
```
DistIsolationDriver.exec(config)
  ├─ topology setup (HA master-slave / streaming-replication N node)
  ├─ workload generator (random tx + commit/abort)
  ├─ fault injector (network partition / kill / clock skew)
  ├─ history collector (per-client tx log)
  └─ analyzer:
       AWDIT  → anomaly-aware diagnostic (history graph cycle)
       Jepsen → external spec checker (linearizability/causal/SI)
```

**SUT 구동 채널:** 다중 클라이언트 (JDBC) + 다중 cubrid 노드 (HA / streaming-replication).
**외부 표면 동결 영향:** 없음 (신규 진입점).

---

## 3. 사용자 요구사항 (incubating 추정)

1. **분산 토폴로지 설정** — HA master-slave / streaming-replication N 노드 자동 deploy (shell 모듈의 DeployHA 와 결합 가능)
2. **fault injector** — network partition (iptables) / process kill / clock skew
3. **history collection** — per-client tx start/commit/abort 로그
4. **anomaly catalog** — Adya/Bailis taxonomy (Hermitage 차용)
5. **deterministic seed** — 같은 seed → 같은 fault sequence → 재현 가능
6. **long-running scalability** — AWDIT 의 강점 — 거대한 history graph 를 시간/메모리 안에서 처리
7. **외부 spec checker** — Jepsen 의 linearizability / causal / snapshot isolation 검증기 위탁

---

## 4. 비기능 요구

| 항목 | 의제 | 새 시스템에서의 의미 |
|------|------|---------------------|
| 도입 비용 | 높음 (분산 인프라 + history analyzer) | survey §6.5 — *조건부* |
| 즉시 ROI | ★★★ (조건부) | N24 / N11 진척에 비례 |
| ADR-001 종속 | Jepsen Clojure | 비-Clojure 시 subprocess 또는 부분 재구현 |
| AWDIT 연구 단계 | 정확한 venue 확인 필요 | survey §13 — incubating 진입 시 보강 |
| HA 인프라 의존 | shell 모듈 DeployHA + ha_repl 자산 | 본 항목이 *strangler-fig HA 진척* 에 종속 |
| §6a-E7 (workload) 와의 중첩 | fault injection 부분 공유 | engine-suite 책임 경계 (C-004) 와 함께 정리 |

---

## 5. 의존하는 외부 자원

- **CUBRID HA / streaming-replication 인프라** — 다중 노드 deploy, failover, replication slot
- **shell 모듈 DeployHA** — 토폴로지 자동화 (재사용)
- **AWDIT 본체** — 연구 도구. 정확한 repo / artifact 확인 필요 (incubating 진입 시 보강)
- **Jepsen** — jepsen.io / Clojure
- **fault injection 레이어** — iptables / cgroup / process signal
- **N24 streaming-replication 진척** — roadmap repo
- **N11 logical-replication-extension graduation** — roadmap repo

---

## 6. incubating 진입 조건 (조건부)

다음이 *모두 충족된 후* 정식 incubating 진입 (owner: hgryoo):

1. **선결 의존 충족** — N24 streaming-replication 또는 N11 logical-replication-extension graduation
2. **AWDIT vs Jepsen 1차 선택** — survey §6.5: AWDIT 는 weak isolation anomaly 에 강하고, Jepsen 은 인프라 비용이 큼
3. **fault injection 채널** — iptables / cgroup / 자체 — 단일 채널 확정
4. **history corpus 보관 정책** — replay 가능성, NG1 점검
5. **engine-suite 책임 경계 (C-004)** — fault injection 인프라가 testkit 책임인지 engine-suite 책임인지
6. **단일노드 축 4 분리 확정** — PostgreSQL `.spec` + Hermitage 흡수는 *기존 isolation 모듈* (Phase 3 ADR-004) 에서 별도 결정. 본 항목은 *분산만*

**ADR placeholder:**
- ADR-EXT-004 — AWDIT / Jepsen 1차 선택 + 토폴로지 자동화 정책 + fault injection 채널 + corpus 보관

---

## 7. 위험 / 정합성 메모

- **선결 의존 위반 위험** — N24 / N11 가 미진척이면 본 항목은 *empty value*. 분기 게이트 §7 에서 우선순위 후순위
- **§6a-E7 (workload) 와 중첩** — fault injection 인프라는 공통. 어느 항목이 책임 host 인가 ADR 필요
- **roadmap repo cross-cutting** — survey §13: 본 항목은 N24 / N11 와 직결. roadmap repo planning 과 동기화 필요
- **NG1 / NG2 / NG4 충돌 없음**
- **단일노드 축 4 (Hermitage / `.spec`)** — 본 항목 *외부*. 기존 isolation 모듈의 strangler-fig 1차 대체 (ADR-004) 검토 시 함께 결정
