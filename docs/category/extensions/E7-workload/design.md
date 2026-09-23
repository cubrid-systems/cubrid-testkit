# E7 — Design (STUB)

*English · [한국어](design.ko.md)*

**Status:** STUB — the real design comes after formal entry into incubating through ADR-EXT-007.
The C-004 responsibility boundary has to be defined first.
**Source:** ROADMAP §6a-E7, `requirements.md`, `project/survey/dbms-testing-ecosystem.md` §9
**Companion:** `requirements.md` (FULL), `io-contract.md` (STUB), `test-corpus.md` (STUB)

---

## 1. The responsibility boundary (C-004 comes first)

```
testkit (this module)                engine-suite (HammerDB / benchbase)
─────────────                       ──────────────────────────────────
verifying correctness invariants ↔  measuring throughput
randomized fault injection          the workload generator
deterministic seed replay           performance regression
```

*It cannot start* without the C-004 cross-cutting conclusion.

## 2. Where the module sits (agenda)

```
internal/runner/workload/
   ├── topology/         # cluster deploy (shared with E4)
   ├── workload/         # KV-style or SQL-style stateful tx generator
   ├── fault/            # can be shared with E4 — an owner ADR is needed
   ├── invariant/        # the invariant catalogue + checker
   ├── history/          # recording the tx / fault timeline
   └── corpus/           # keeping violations
```

## 3. Data flow (agenda)

```
seed → topology.deploy()
       └─ workload.start(generators)  ┐
                                       ├─ history.record()
       └─ fault.inject(timeline)       ┘
                                          └─ periodically: invariant.check(state)
                                               └─ violation? corpus.save({seed, fault, witness})
```

## 4. The invariant catalogue (agenda)

| invariant | The verifying SQL/method | Note |
|---|---|---|
| row_count_consistency | SELECT COUNT(*) — agreement between replicas | verifies replication |
| referential_integrity | 0 FK violations | concurrent DDL/DML |
| sum_conservation | the balance sum is conserved in a transfer scenario | a linearizability proxy |
| monotonicity | seq increases monotonically | verifies snapshot isolation |
| no_phantom_after_failover | no phantom immediately after failover | verifies HA |

ADR-EXT-007 picks the first invariants.

## 5. First candidates for the scenario (agenda)

| scenario | Origin | Cost of adoption | Immediate ROI |
|---|---|---|---|
| roachtest-style randomized | CockroachDB | medium | ★★★ |
| FoundationDB-style deterministic simulation | FoundationDB | very high | ★★ (long term) |
| invariants laid on top of benchbase / HammerDB | engine-suite | low | ★★ (more dependency) |

ADR-EXT-007 picks the first scenario.

## 6. External dependencies

- The C-004 cross-cutting conclusion (comes first)
- engine-suite assets (HammerDB / benchbase) — the scope of reuse to be decided
- E4's fault injection (shared)
- CUBRID HA / streaming-replication

## 7. Decisions held over → ADR-EXT-007

- the first scenario (roachtest / simulation / combined with benchbase)
- the first items of the invariant catalogue
- the scope of engine-suite asset reuse
- who owns the fault injector shared with E4
- where the violation corpus lives (NG1 check)

## 8. The trigger for writing the design

After the C-004 conclusion and ADR-EXT-007. A stub for now.
