# E4 — I/O Contract (STUB)

**Status:** STUB — contract 동결은 ADR-EXT-004 incubating 정식 진입 후.
**Source:** `requirements.md` §2

---

## 1. CLI (제안)

```
ctp.sh isolation-dist [-c <isolation-dist.conf>] [--mode awdit|jepsen]
   또는
testkit run isolation-dist --mode <name> [--topology ha|streaming] [--time <sec>] [--seed <N>]
```

## 2. conf 스키마 (TBD)

| 키 (의제) | 값 | 출처 |
|---|---|---|
| `seed` | `<int>` | 재현용 |
| `time_budget_sec` | `<int>` | CI 시간 통제 |
| `mode` | `awdit \| jepsen` | analyzer 선택 |
| `topology` | `ha \| streaming` | 클러스터 구성 |
| `nodes` | `<int>` | 노드 수 |
| `fault_channels` | csv `partition,kill,clock,cgroup` | fault injection 활성화 |
| `corpus_root` | `<dir>` | history / violation corpus 위치 |
| `client_count` | `<int>` | 동시 클라이언트 수 |

ADR-EXT-004 후 키/의미 동결.

## 3. 출력 포맷 (TBD)

```
<resultDir>/
   ├── main.info
   ├── history/
   │     └── <client-id>.tx.log    # tx start / commit / abort 로그
   ├── faults/
   │     └── timeline.json         # fault sequence (재현 seed)
   ├── violations/
   │     └── <witness-hash>/
   │           ├── seed
   │           ├── fault_seq.json
   │           ├── history_excerpt.log
   │           └── reproducer.sh
   └── stats.json                  # iterations / violation count / per-anomaly hit
```

## 4. 종료 코드 (TBD)

| 값 | 의미 |
|---|---|
| 0 | 시간 내 violation 없음 |
| 1 | violation 발견 |
| ≥2 | infra 오류 (deploy 실패 / 노드 미기동 등) |

## 5. NG2 / NG4 점검

신규 진입점 — 충돌 없음.
