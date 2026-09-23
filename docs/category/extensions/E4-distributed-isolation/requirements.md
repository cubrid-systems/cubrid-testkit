# E4 — Distributed Isolation Testing (AWDIT / Jepsen) (Requirements)

*English · [한국어](requirements.ko.md)*

**Source:** survey/dbms-testing-ecosystem.md §6 + §11
**Status:** incubating (conditional — waiting on N24 streaming-replication / N11 graduation)
**Axis mapping:** axis 4 (Isolation / transaction testing — the distributed part)
**Companion docs (to follow):** `design.md`, `io-contract.md`, `test-corpus.md`, `implementation-notes.md`

---

## 1. The problem this extension solves

It verifies **isolation anomalies in CUBRID's distributed / HA / streaming-replication
environments** with a history graph, or by *detecting a violation of an external spec*.

The existing testkit `isolation` module (analysis/isolation) deals only with *several clients on a
single node*. Once the distributed axis comes in, these are blind:
- anomalies under network partition / clock skew / process kill
- cycles in the long-running history of weak isolation
- causal / linearizability violations on a replicated read
- visibility violations right after an HA failover

The *single-node* part of axis 4 (the PostgreSQL `.spec` format plus the Hermitage catalogue) is
absorbed into the strangler-fig isolation module (when ROADMAP phase 3 ADR-004 is reviewed). This
entry deals with *the distributed part only*.

**The kind of bug it catches:** anomalies, and violations of linearizability, causality and snapshot
isolation. Single-node isolation behaviour itself is *outside the axis* (that is the existing
isolation module).

---

## 2. How it is called from outside (proposed — incubating)

```
ctp.sh isolation-dist [-c <isolation-dist.conf>] [--mode awdit|jepsen]
   or
testkit run isolation-dist --mode <name> [--topology <ha|streaming>] [--time <sec>]
```

The internal entry point (agenda):
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

**The channel that drives the SUT:** several clients (JDBC) plus several cubrid nodes (HA /
streaming-replication).
**Effect on the external surface freeze:** none (a new entry point).

---

## 3. What users need (an incubating estimate)

1. **Setting up the distributed topology** — automatic deploy of HA master-slave, or of N
   streaming-replication nodes (can be combined with the shell module's DeployHA)
2. **A fault injector** — network partition (iptables) / process kill / clock skew
3. **History collection** — per-client tx start/commit/abort logs
4. **An anomaly catalogue** — the Adya/Bailis taxonomy (borrowed from Hermitage)
5. **A deterministic seed** — the same seed → the same fault sequence → reproducible
6. **Long-running scalability** — AWDIT's strength — handling a huge history graph within time and
   memory
7. **An external spec checker** — delegating to Jepsen's linearizability / causal / snapshot
   isolation verifiers

---

## 4. Non-functional requirements

| Item | Agenda | What it means in the new system |
|------|------|---------------------|
| Cost of adoption | high (distributed infrastructure + a history analyzer) | survey §6.5 — *conditional* |
| Immediate ROI | ★★★ (conditional) | in proportion to how far N24 / N11 have got |
| Dependence on ADR-001 | Jepsen is Clojure | a subprocess, or a partial reimplementation, if not Clojure |
| AWDIT is at the research stage | the exact venue needs confirming | survey §13 — filled in on entry to incubating |
| Dependence on HA infrastructure | the shell module's DeployHA plus the ha_repl assets | this entry is subordinate to *how far the strangler fig's HA work has got* |
| Overlap with §6a-E7 (workload) | the fault injection part is shared | sorted out together with the engine-suite responsibility boundary (C-004) |

---

## 5. External resources it depends on

- **CUBRID HA / streaming-replication infrastructure** — multi-node deploy, failover, replication
  slot
- **The shell module's DeployHA** — topology automation (reused)
- **AWDIT itself** — a research tool. The exact repository / artifact needs confirming (filled in on
  entry to incubating)
- **Jepsen** — jepsen.io / Clojure
- **A fault injection layer** — iptables / cgroup / process signal
- **Progress on N24 streaming-replication** — the roadmap repository
- **N11 logical-replication-extension graduation** — the roadmap repository

---

## 6. The conditions for entering incubating (conditional)

Formal entry into incubating comes once *all* of these are met (owner: hgryoo):

1. **The prerequisite is met** — N24 streaming-replication, or N11 logical-replication-extension
   graduation
2. **AWDIT or Jepsen first** — survey §6.5: AWDIT is strong on weak isolation anomalies, and Jepsen
   costs a lot in infrastructure
3. **The fault injection channel** — iptables / cgroup / our own — settle on a single channel
4. **The keeping policy for the history corpus** — whether it can be replayed, and the NG1 check
5. **The engine-suite responsibility boundary (C-004)** — whether the fault injection infrastructure
   is testkit's responsibility or engine-suite's
6. **Settling the separation of single-node axis 4** — absorbing the PostgreSQL `.spec` format and
   Hermitage is decided separately, in the *existing isolation module* (phase 3 ADR-004). This entry
   is *distributed only*

**ADR placeholder:**
- ADR-EXT-004 — AWDIT or Jepsen first, the topology automation policy, the fault injection channel,
  and how the corpus is kept

---

## 7. Notes on risk and consistency

- **The risk of breaking the prerequisite** — if N24 / N11 have not moved, this entry is *empty
  value*. Lower priority at the branch gate, §7
- **Overlap with §6a-E7 (workload)** — the fault injection infrastructure is common to both. An ADR
  is needed on which entry hosts the responsibility
- **A roadmap repository cross-cutting** — survey §13: this entry is directly tied to N24 / N11. It
  needs to stay in step with roadmap repository planning
- **No conflict with NG1 / NG2 / NG4**
- **Single-node axis 4 (Hermitage / `.spec`)** — *outside* this entry. Decided alongside the review
  of the strangler fig's first replacement of the existing isolation module (ADR-004)
