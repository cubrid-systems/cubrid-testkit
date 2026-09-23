# E6 — Differential Testing (PostgreSQL Pair) (Requirements)

*English · [한국어](requirements.ko.md)*

**Source:** survey/dbms-testing-ecosystem.md §8 + §11
**Status:** incubating (conditional — N13 pg-wire-compat at *selected* or beyond recommended)
**Axis mapping:** axis 6 (Differential testing)
**Companion docs (to follow):** `design.md`, `io-contract.md`, `test-corpus.md`, `implementation-notes.md`

---

## 1. The problem this extension solves

CUBRID's SQL results are checked by **pairing them with an external DBMS (PostgreSQL first)** and
looking at the *difference in the results* for the same input.

The criterion is *the external DBMS itself* — there is no need to build an oracle inside testkit.
Precedents:
- RAGS (Microsoft) — random SQL → comparing the results of several DBMSs
- SQLancer differential mode
- CockroachDB ↔ PostgreSQL differential CI

**The kind of bug it catches:** wrong-result. The cases where *only CUBRID behaves differently*.
But dialect mismatch produces noise.

---

## 2. The form of the external call (proposed — incubating)

```
ctp.sh diff-pg [-c <diff.conf>] [--peer pg|other] [--mode canonical|rewrite]
   or
testkit run differential --peer <name> [--cases <corpus>] [--seed <N>]
```

The internal entry (agenda):
```
DifferentialDriver.exec(config)
  ├─ load case corpus (sqllogictest .slt or sqlsmith generated)
  ├─ for each query Q:
  │    R_cubrid = run(Q on CUBRID via JDBC/CCI/pg-wire)
  │    R_peer   = run(Q on PostgreSQL via JDBC)
  │    case mode of
  │      canonical → expect identical result on canonical SQL subset
  │      rewrite   → apply dialect rewrite layer, then compare
  └─ on diff: corpus.save({Q, R_cubrid, R_peer, schema, seed})
```

**The channel that drives the SUT:** with N13 pg-wire-compat at *selected* or beyond, connecting
*directly with the PostgreSQL driver* is possible. Without that progress, JDBC plus a separate
PostgreSQL client branch.
**Effect on the frozen external surface:** none (a new entry point).

---

## 3. User requirements (incubating estimate)

1. **peer DBMS, PostgreSQL first** — it combines naturally with N13 pg-wire-compat
2. **canonical subset mode** — compare only the *canonical subset* that every DBMS defines
   identically, such as SQL-92 core
3. **rewrite layer mode** — absorb dialect differences by *rewriting* them: DATE functions / NULL
   ordering / floating point / integer overflow / string collation / JSON functions
4. **dialect mismatch classification** — separate *a real wrong-result* from *a known dialect
   difference* automatically
5. **case corpus as input** — take sqllogictest (E1) or sqlsmith generated (E2) as the input
6. **diff reporting** — query / both results / dialect classification / regression seed
7. **accumulating regression seeds** — re-run the real wrong-results found in the past against a
   new build

---

## 4. Non-functional requirements

| Item | Agenda | What it means in the new system |
|------|------|---------------------|
| Cost of adoption | medium (the rewrite layer is the bulk of it) | survey §8.2 — *dialect mismatch hell* |
| Immediate ROI | ★★★ (after N13) | small ROI if N13 has not progressed |
| Dependency on N13 pg-wire-compat | at *selected* or beyond the cost drops sharply | a cross-cutting candidate in the roadmap repository (no number assigned) |
| canonical subset only | possible even without a rewrite layer | but the range of SQL it can handle is narrow |
| Licence | PostgreSQL JDBC, BSD-like / free | the external DBMS's own licence needs checking |
| Combination with §6a-E1 (sqllogictest) | makes use of sqllogictest's cross-DB strength | E1 + E6 is a natural combination |

---

## 5. External resources it depends on

- **A PostgreSQL instance** — on the same or a neighbouring host
- **The PostgreSQL JDBC driver** — running the query on the peer side
- **Progress on N13 pg-wire-compat** — roadmap repository. At *selected* or beyond the CUBRID side
  can be reached with the PostgreSQL driver too
- **A dialect rewrite layer** — absorbing dialect differences such as DATE / NULL ordering /
  collation / JSON / integer overflow
- **A case corpus** — the output of E1 (sqllogictest) or E2 (sqlsmith)

---

## 6. Conditions for entering incubating (conditional)

Formal entry into incubating once the following are met (owner: hgryoo):

1. **N13 pg-wire-compat at *selected* or beyond** — survey §8.3 conclusion: after N13 reaches
   selected in the priority order
2. **the first mode chosen** — canonical subset (cheap) vs rewrite layer (expensive, high value)
3. **a dialect rewrite catalogue** — a rewrite policy per category: DATE / NULL / float /
   collation / JSON / overflow
4. **the scope of peer DBMSs** — PostgreSQL only at first / + MySQL / + SQLite
5. **the input source for the case corpus** — E1 / E2 / written by hand
6. **roadmap cross-cutting (no number assigned)** — survey §13: testkit §6a-E6 × N13
   pg-wire-compat. ~~C-014~~ was registered on 2026-05-13 as lock-manager × maintenance-mode and
   cannot be used — a new number is applied for when N13 enters selected
7. **a retention policy for regression seeds** — NG1 check

**ADR placeholder:**
- ADR-EXT-006 — the first peer DBMS + the mode (canonical vs rewrite) + the dialect rewrite
  catalogue policy + where the corpus lives

---

## 7. Risk and consistency notes

- **The risk of a violated prerequisite** — if N13 pg-wire-compat has not progressed the ROI is
  small → the risk of empty value. Lower priority under the branch gate, §7
- **dialect mismatch noise** — survey §8.2 — many false positives and trust drops. Controlled by
  the rewrite layer or by the canonical subset
- **Combination with §6a-E1 (sqllogictest)** — sqllogictest suits cross-DB regression, and E6's
  *criterion is an external DBMS* — the combination is natural
- **No conflict with NG1 / NG2**
- **NG4 (no compatibility with non-CUBRID DBMSs) check** — this is not *CUBRID adding
  compatibility with another DBMS*, it is *verifying CUBRID by comparing it against an external
  DBMS*, so there is no conflict with NG4
