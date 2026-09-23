# E7 — Stateful / Randomized Workload Testing (Requirements)

*English · [한국어](requirements.ko.md)*

**Source:** survey/dbms-testing-ecosystem.md §9 + §11
**Status:** incubating (conditional — the engine-suite responsibility boundary (C-004) comes first)
**Axis mapping:** axis 7 (Stateful / workload testing)
**Companion docs (to follow):** `design.md`, `io-contract.md`, `test-corpus.md`, `implementation-notes.md`

---

## 1. The problem this extension solves

CUBRID's **long-running invariants** are verified in scenarios where randomized schema mutation,
node restart, partition and failover are all going on at once.

Every module in testkit today looks only at *comparatively short, deterministic scenarios*. What
this entry covers:
- random schema mutation (DDL during DML)
- recovery invariants combined with node restart / kill
- operational behaviour of the range split / merge / rebalance sort
- invariants immediately after failover / partition
- the crash/recovery axis combined with the distributed axis

The representative cases:
- **CockroachDB roachtest** — randomized testing, long-running invariants
- **FoundationDB simulation testing** — deterministic simulation (fake network/disk/clock)

**The kinds of bug it catches:** invariant violation, race, partial failure, clock-related,
ordering. It makes defects that *cannot be reproduced in production* *reproducible*.

---

## 2. The form of the external call (proposed — incubating)

```
ctp.sh workload [-c <workload.conf>] [--scenario roach|sim] [--time <sec>]
   or
testkit run workload --scenario <name> [--seed <N>] [--invariants <list>]
```

The internal entry (agenda):
```
WorkloadDriver.exec(config)
  ├─ deploy cluster (HA / streaming-replication)
  ├─ start workload generators (KV-style or SQL-style stateful)
  ├─ start fault injector (random kill / partition / clock skew / DDL during DML)
  ├─ run for budget (time / iteration)
  ├─ check invariants periodically:
  │    no row count anomaly · no monotonicity violation · referential integrity
  └─ on violation: corpus.save({seed, fault sequence, witness})
```

**The channel that drives the SUT:** several clients against several cubrid nodes.
**Effect on the frozen external surface:** none (a new entry point).

---

## 3. User requirements (incubating estimate)

1. **a stateful workload generator** — randomized transactions, KV-style or SQL-style
2. **an invariant catalogue** — row count / monotonicity / referential integrity /
   sum-conservation and so on
3. **a fault injector** — process kill / network partition / clock skew / DDL-during-DML
4. **a deterministic seed** — the same seed → the same fault sequence → reproducible
5. **a long-running budget** — from a few minutes inside CI up to a nightly long run
6. **violation reporting** — seed / fault sequence / witness query / a reproduction script
7. **the division with engine-suite** — testkit = correctness invariants / engine-suite =
   throughput

---

## 4. Non-functional requirements

| Item | Agenda | What it means in the new system |
|------|------|---------------------|
| Cost of adoption | high (infrastructure + the invariant catalogue) | survey §9.3 — *lower priority* |
| Immediate ROI | ★★ | defining the responsibility boundary with engine-suite comes first |
| Dependency on C-004 (cross-cutting) | the testkit × engine-suite boundary | survey §9.3 — the *continuation line* of this repository |
| The FoundationDB simulation concept | absorbing it directly is unrealistic | only the *deterministic harness* concept is borrowed (long term) |
| Overlap with §6a-E4 (distributed isolation) | the fault injection infrastructure is shared | an ADR is needed on which entry is the host |
| The division with HammerDB / benchbase | assets on the engine-suite side | testkit's responsibility = correctness, engine-suite = throughput |

---

## 5. External resources it depends on

- **CUBRID HA / streaming-replication infrastructure** — several nodes
- **engine-suite (HammerDB / benchbase)** — the long-running workload generator assets. *The
  responsibility boundary comes first*
- **A fault injection layer** — iptables / cgroup / process signal (shared with E4)
- **An invariant catalogue** — testkit's own asset
- **A deterministic harness (long term)** — needed if the FoundationDB simulation concept is
  borrowed

---

## 6. Conditions for entering incubating (conditional)

Formal entry into incubating once the following are met (owner: hgryoo):

1. **the C-004 responsibility boundary defined first** — the testkit (correctness) ↔ engine-suite
   (throughput) boundary written into an ADR
2. **the first scenario chosen** — roachtest-style randomized vs FoundationDB-style deterministic
   simulation. The difference in the cost of adoption is large
3. **a seed for the invariant catalogue** — a first list of invariants: row count / monotonicity /
   referential integrity and so on
4. **the fault injector channel** — shared with E4 or implemented separately — a single channel is
   recommended
5. **the scope of engine-suite asset reuse** — how far into HammerDB / benchbase testkit calls
6. **a branch gate §7 conflict check** — the strangler fig comes first. Where it exceeds the
   availability of one person, *lower priority*
7. **where the violation corpus lives** — NG1 check

**ADR placeholder:**
- ADR-EXT-007 — the first scenario + the invariant catalogue + the engine-suite responsibility
  boundary (combined with C-004) + where the corpus lives

---

## 7. Risk and consistency notes

- **The risk of an undecided engine-suite responsibility boundary** — survey §9.3: testkit cannot
  decide alone. C-004 cross-cutting consistency comes first
- **Overlap with §6a-E4 (distributed isolation)** — the fault injection infrastructure is shared.
  An ADR is needed on which entry owns it
- **Possible NG1 conflict** — a violation corpus that goes into testcases violates the freeze.
  External storage is recommended
- **No conflict with NG2 / NG4**
- **The branch gate, §7** — *lower priority is the recommendation* (survey §11). The availability
  of one person plus the strangler fig first (ROADMAP §8 risk 7)
- **The FoundationDB simulation concept** — a long-term vision. At the point this entry first
  enters, roachtest-style randomized is the realistic choice
