# E7 — I/O Contract (STUB)

*[English](io-contract.md) · 한국어*

**Status:** STUB — contract 동결은 ADR-EXT-007 incubating 정식 진입 후.
**Source:** `requirements.md` §2

---

## 1. CLI (제안)

```
ctp.sh workload [-c <workload.conf>] [--scenario roach|sim|benchbase] [--time <sec>]
   또는
testkit run workload --scenario <name> [--seed <N>] [--invariants <list>]
```

## 2. conf 스키마 (TBD)

| 키 (의제) | 값 | 출처 |
|---|---|---|
| `seed` | `<int>` | 재현용 |
| `time_budget_sec` | `<int>` | CI / nightly 시간 통제 |
| `scenario` | `roach \| sim \| benchbase` | 1차 scenario |
| `topology` | `ha \| streaming` | 클러스터 |
| `nodes` | `<int>` | 노드 수 |
| `invariants` | csv `row_count,fk,sum,monotone,phantom` | 활성 invariant |
| `fault_channels` | csv (E4 와 동일) | fault injection 활성 |
| `workload_mix` | csv `ddl=10,dml=80,select=10` | tx mix |
| `corpus_root` | `<dir>` | violation corpus 위치 |
| `engine_suite_handoff` | bool | benchbase / HammerDB 위탁 모드 |

ADR-EXT-007 후 키/의미 동결.

## 3. 출력 포맷 (TBD)

```
<resultDir>/
   ├── main.info
   ├── history/
   │     ├── tx.log               # tx 시작/종료 timeline
   │     └── fault.json           # fault sequence
   ├── violations/
   │     └── <invariant>/<witness-hash>/
   │           ├── seed
   │           ├── fault_seq.json
   │           ├── state_dump.txt
   │           ├── invariant_report.txt
   │           └── reproducer.sh
   └── stats.json                 # iterations / violation count / per-invariant hit
```

## 4. 종료 코드 (TBD)

| 값 | 의미 |
|---|---|
| 0 | 시간 내 invariant violation 없음 |
| 1 | violation 발견 |
| ≥2 | infra 오류 (deploy / engine-suite handoff 실패 등) |

## 5. engine-suite 위탁 contract (C-004)

| 항목 | testkit 측 | engine-suite 측 |
|---|---|---|
| workload 발생 | invariant checker | benchbase / HammerDB workload |
| 결과 | violation corpus | throughput report |
| 책임 | correctness | performance |

C-004 ADR 에서 위탁 인터페이스 / 결과 회수 형식 동결.

## 6. NG2 / NG4 점검

신규 진입점 — 충돌 없음.
