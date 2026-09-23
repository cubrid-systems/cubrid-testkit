# E7 — I/O Contract (STUB)

*English · [한국어](io-contract.ko.md)*

**Status:** STUB — the contract is frozen after formal entry into incubating through ADR-EXT-007.
**Source:** `requirements.md` §2

---

## 1. CLI (proposed)

```
ctp.sh workload [-c <workload.conf>] [--scenario roach|sim|benchbase] [--time <sec>]
   or
testkit run workload --scenario <name> [--seed <N>] [--invariants <list>]
```

## 2. conf schema (TBD)

| Key (agenda) | Value | Where it comes from |
|---|---|---|
| `seed` | `<int>` | for reproduction |
| `time_budget_sec` | `<int>` | controlling CI / nightly time |
| `scenario` | `roach \| sim \| benchbase` | the first scenario |
| `topology` | `ha \| streaming` | the cluster |
| `nodes` | `<int>` | the number of nodes |
| `invariants` | csv `row_count,fk,sum,monotone,phantom` | which invariants are active |
| `fault_channels` | csv (the same as E4's) | which fault injection is active |
| `workload_mix` | csv `ddl=10,dml=80,select=10` | the tx mix |
| `corpus_root` | `<dir>` | where the violation corpus lives |
| `engine_suite_handoff` | bool | the mode that hands off to benchbase / HammerDB |

Keys and meanings are frozen after ADR-EXT-007.

## 3. Output format (TBD)

```
<resultDir>/
   ├── main.info
   ├── history/
   │     ├── tx.log               # the timeline of tx starts and ends
   │     └── fault.json           # the fault sequence
   ├── violations/
   │     └── <invariant>/<witness-hash>/
   │           ├── seed
   │           ├── fault_seq.json
   │           ├── state_dump.txt
   │           ├── invariant_report.txt
   │           └── reproducer.sh
   └── stats.json                 # iterations / violation count / per-invariant hit
```

## 4. Exit codes (TBD)

| Value | Meaning |
|---|---|
| 0 | no invariant violation within the time budget |
| 1 | a violation found |
| ≥2 | an infrastructure error (deploy / engine-suite handoff failure and so on) |

## 5. The engine-suite handoff contract (C-004)

| Item | On the testkit side | On the engine-suite side |
|---|---|---|
| generating the workload | the invariant checker | the benchbase / HammerDB workload |
| the result | the violation corpus | the throughput report |
| the responsibility | correctness | performance |

The C-004 ADR freezes the handoff interface and the form in which results are collected back.

## 6. NG2 / NG4 check

A new entry point — no conflict.
