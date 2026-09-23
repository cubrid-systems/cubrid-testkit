# E7 — Test Corpus (STUB)

*English · [한국어](test-corpus.ko.md)*

**Status:** STUB — the corpus policy comes after formal entry into incubating through ADR-EXT-007.
**Source:** `requirements.md` §5

---

## 1. Kinds of corpus

| Corpus | Input/output | Purpose |
|---|---|---|
| invariant catalogue | input | the definition of the invariants verified (a testkit asset) |
| seed corpus | input | regression seeds — the fault sequences of past violations |
| violation corpus | output | accumulated invariant violations |
| (engine-suite handoff) | input | the workload definitions of benchbase / HammerDB — *reused* |

This entry has *no input SQL corpus* — the workload is randomly generated.

## 2. The invariant catalogue (a testkit asset)

```
catalog/
   ├── row_count_consistency.yaml
   ├── referential_integrity.yaml
   ├── sum_conservation.yaml
   ├── monotonicity.yaml
   └── no_phantom_after_failover.yaml
```

Each yaml: the invariant's name / the SQL query / the threshold / the scope (per-table / per-tx) —
the schema is frozen after ADR-EXT-007.

## 3. Storage policy (NG1 check)

- ❌ not kept in the testcases repository
- ✅ inside testkit (the invariant catalogue) + external storage (the violation corpus)
- ✅ *sharing the corpus location policy with E4 is recommended* (the history may grow very large —
  a GC policy has to be stated)

## 4. The structure of a violation entry (agenda)

```
violations/<invariant>/<witness-hash>/
   ├── seed
   ├── topology.json          # cluster config
   ├── workload_mix.json
   ├── fault_seq.json
   ├── state_dump.txt         # part of the DB state at the moment of the violation
   ├── invariant_report.txt   # the checker's output
   └── reproducer.sh
```

## 5. Reusing engine-suite assets (combined with C-004)

| engine-suite asset | Can it be reused | Note |
|---|---|---|
| benchbase TPCC schema | yes | consistent with the sum_conservation invariant |
| HammerDB schema/workload | yes | combines with randomized DDL/DML |
| a KV workload generated here | separate | suits debugging an invariant |

ADR-EXT-007 decides the scope of reuse.

## 6. Licence

- benchbase: Apache 2.0 (a decision on the engine-suite side)
- HammerDB: GPLv3 (on the engine-suite side — a *process boundary* is recommended so that testkit
  does not take on the same licence obligation)
- invariant catalogue: a testkit asset

## 7. The trigger for the follow-up

After the C-004 conclusion and ADR-EXT-007, this document is brought up to FULL.
