# E4 — I/O Contract (STUB)

*English · [한국어](io-contract.ko.md)*

**Status:** STUB — the contract is frozen after formal entry into incubating through ADR-EXT-004.
**Source:** `requirements.md` §2

---

## 1. CLI (proposed)

```
ctp.sh isolation-dist [-c <isolation-dist.conf>] [--mode awdit|jepsen]
   or
testkit run isolation-dist --mode <name> [--topology ha|streaming] [--time <sec>] [--seed <N>]
```

## 2. conf schema (TBD)

| Key (agenda) | Value | Source |
|---|---|---|
| `seed` | `<int>` | for reproduction |
| `time_budget_sec` | `<int>` | controlling CI time |
| `mode` | `awdit \| jepsen` | choice of analyzer |
| `topology` | `ha \| streaming` | the cluster's shape |
| `nodes` | `<int>` | the number of nodes |
| `fault_channels` | csv `partition,kill,clock,cgroup` | which fault injection is enabled |
| `corpus_root` | `<dir>` | where the history / violation corpus lives |
| `client_count` | `<int>` | the number of concurrent clients |

Keys and meanings are frozen after ADR-EXT-004.

## 3. Output format (TBD)

```
<resultDir>/
   ├── main.info
   ├── history/
   │     └── <client-id>.tx.log    # tx start / commit / abort logs
   ├── faults/
   │     └── timeline.json         # the fault sequence (the seed that reproduces it)
   ├── violations/
   │     └── <witness-hash>/
   │           ├── seed
   │           ├── fault_seq.json
   │           ├── history_excerpt.log
   │           └── reproducer.sh
   └── stats.json                  # iterations / violation count / per-anomaly hit
```

## 4. Exit codes (TBD)

| Value | Meaning |
|---|---|
| 0 | no violation within the time |
| 1 | a violation was found |
| ≥2 | an infrastructure error (deploy failed, a node is not up, and so on) |

## 5. The NG2 / NG4 check

A new entry point — no conflict.
