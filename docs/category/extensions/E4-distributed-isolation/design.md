# E4 — Design (STUB)

*English · [한국어](design.ko.md)*

**Status:** STUB — the real design comes after formal entry into incubating through ADR-EXT-004, and
N24 / N11 graduation is a prerequisite besides.
**Source:** ROADMAP §6a-E4, `requirements.md`, `project/survey/dbms-testing-ecosystem.md` §6
**Companion:** `requirements.md` (FULL), `io-contract.md` (STUB), `test-corpus.md` (STUB)

---

## 1. Where the module sits (agenda)

```
internal/runner/isolationdist/
   ├── topology/         # automatic deploy of HA / streaming-replication (reusing shell.DeployHA)
   ├── workload/         # randomized tx generator (KV-style or SQL-style)
   ├── fault/            # network partition (iptables) / process kill / clock skew
   ├── history/          # per-client tx start/commit/abort logs
   └── analyzer/
         ├── awdit/      # weak isolation anomaly + history graph cycle
         └── jepsen/     # external spec checker (linearizability/causal/SI) — called out to Clojure
```

*Single-node axis 4* (the PostgreSQL `.spec` format plus Hermitage) is outside this module — decided
when the strangler fig's first replacement of the existing `project/analysis/isolation` (ADR-004) is
reviewed.

## 2. Data flow (agenda)

```
seed → topology.deploy(HA|streaming, N nodes)
       └─ workload.start(N clients) ┐
                                    ├─ history.collect()
       └─ fault.inject(seed)        ┘
                                       └─ analyzer.run(history)
                                            └─ violation? corpus.save({seed, fault seq, witness})
```

## 3. The fault injection channel (agenda)

| Channel | What it can detect | Note |
|---|---|---|
| iptables network partition | replication split-brain | needs sudo |
| process kill (SIGKILL) | recovery invariant | reuses the shell module |
| clock skew (faketime) | causal / linearizability | depends on a library |
| cgroup CPU/IO throttle | timing race | an operational dependency |

*Can be shared* with E7 (workload) — an ADR is needed on which entry owns it.

## 4. External dependencies

- N24 streaming-replication / N11 logical-replication-extension graduation (prerequisites)
- the shell module's DeployHA plus the ha_repl assets
- AWDIT itself (the exact repository is to be confirmed and filled in)
- Jepsen (jepsen.io / Clojure)
- the engine-suite C-004 responsibility boundary (bound up with E7)

## 5. Held over for decision → ADR-EXT-004

- AWDIT or Jepsen first
- settling on a single fault injection channel
- who owns the fault injector shared with E7
- where the history corpus lives (the NG1 check)

## 6. The trigger for writing the design

After ADR-EXT-004 is agreed, and after N24/N11 graduation. A stub for now.
