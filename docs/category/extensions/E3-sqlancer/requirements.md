# E3 — Logic Bug Detection (SQLancer NoREC + TLP) (Requirements)

*English · [한국어](requirements.ko.md)*

**Source:** survey/dbms-testing-ecosystem.md §5 + §11
**Status:** incubating (before formal entry — the ADR-EXT-003 slot)
**Axis mapping:** axis 3 (Logic-bug / semantic testing)
**Companion docs (to follow):** `design.md`, `io-contract.md`, `test-corpus.md`, `implementation-notes.md`

---

## 1. The problem this extension solves

It verifies **CUBRID's optimizer correctness, its executor's semantic correctness and its handling
of 3-valued logic** with *pairs of semantically equivalent queries*.

The existing testkit's sql module does only an *expected file diff* — it looks only at queries whose
answer is known in advance. Logic-bug tools of the SQLancer kind:
- *compare against each other* random query pairs *whose answer is unknown*
- verify that an optimizer rewrite preserves meaning
- verify that NULL / UNKNOWN handling is consistent
- verify, from a pivot, that no row is missing or added

**The kind of bug it catches:** wrong results. The kind where, on the same SQL surface, *a
difference in the internal plan* makes the result different. Parser crashes are *outside the axis*
(those are §6a-E2/E5).

---

## 2. How it is called from outside (proposed — incubating)

```
ctp.sh sqlancer [-c <sqlancer.conf>] [--oracle norec|tlp|pqs]
   or
testkit run sqlancer [-c <conf>] [--oracle <name>] [--seed <N>] [--time <sec>]
```

The internal entry point (agenda):
```
SqlancerDriver.exec(config)
  ├─ schema introspect (CUBRID dialect)
  ├─ for each iteration:
  │    case oracle of
  │      NoREC → generate Q1 (predicate); rewrite Q2 (boolean projection); compare rowcount
  │      TLP   → generate base Q; partition into IS TRUE / IS FALSE / IS NULL; compare union
  │      PQS   → pick pivot row; synthesize containing query; verify pivot present
  └─ on mismatch: corpus.save({Q1, Q2, schema, seed})
```

**The channel that drives the SUT:** JDBC or CCI.
**Effect on the external surface freeze:** none (a new entry point).

---

## 3. What users need (an incubating estimate)

1. **The NoREC oracle (recommended for the first entry)** — comparing the rowcount of `WHERE p`
   against `COUNT(*) FROM (SELECT (p) IS TRUE p FROM t) WHERE p`. *No need to know the internal
   plan*
2. **The TLP oracle (recommended for the first entry)** — verifying the union of a 3-valued logic
   partition
3. **The PQS oracle (a second, later step)** — verifying that the pivot is preserved — the highest
   cost of adoption
4. **A CUBRID dialect adapter** — schema introspection / SQL surface generator / result comparison —
   a dialect layer that *can be shared* with SQLsmith (E2)
5. **Mismatch reporting** — the two queries, the schema, the seed, the execution results, and a
   reduce after normalisation
6. **Accumulating regression seeds** — re-running bugs found in the past against a new build
7. **A time / iteration budget** — running within a fixed time inside CI

---

## 4. Non-functional requirements

| Item | Agenda | What it means in the new system |
|------|------|---------------------|
| Cost of adoption | low (NoREC first) to middling (PQS) | survey §5.4 — only NoREC + TLP recommended for the first adoption |
| Immediate ROI | ★★★★ | it fills directly the *wrong-result verification* area testkit is empty in |
| Relation to §6a-E2 (SQLsmith) | complementary — only an oracle is added on top of the random AST | the dialect adapter is shared (Open Question 3) |
| Cost of judgement | NoREC < TLP < PQS | NoREC first. A trade-off against the depth at which a nested AST can be generated efficiently |
| Licence | SQLancer is MIT — vendoring is free | survey §12.8 — better placed than SQLsmith on the licence side |
| Dependence on ADR-001 (implementation language) | SQLancer is Java | import it directly if the JVM is adopted; a subprocess or a reimplementation if not |

---

## 5. External resources it depends on

- **A CUBRID client** — JDBC / CCI
- **The CUBRID system catalog** — schema introspection (shared with E2)
- **SQLancer itself** — github.com/sqlancer/sqlancer (Java/MIT), or a reimplementation
- **A dialect adapter** — shared with E2. A *common dialect layer* inside testkit is the sensible
  form (Open Question 3)
- **Mismatch corpus storage** — the same policy as E2 (check against NG1, the testcases repository
  freeze)

---

## 6. The conditions for entering incubating

ADR-EXT-003 can be written once these are decided (owner: hgryoo):

1. **Which oracle first** — NoREC recommended (lowest cost of adoption, highest ROI). TLP alongside
   it or after it
2. **Where the dialect adapter sits** — survey Open Question 3: a *common dialect layer* inside
   testkit (shared with E2) vs one spread per tool
3. **Reuse vs reimplement** — using the SQLancer Java body directly vs reimplementing it in the
   language ADR-001 decides
4. **Where the mismatch corpus lives** — the same policy as E2 (the NG1 check)
5. **The CI integration mode** — a short oracle pass on every PR vs a nightly long oracle pass
6. **The follow-on roadmap for PQS / SQLaser / SQLancer++** — in what order they extend the first
   adoption
7. **Checking for a conflict with the strangler fig's branch gate, §7** — competing for resources
   with the first replacement of phases 3 and 4

**ADR placeholder:**
- ADR-EXT-003 — which oracle first (NoREC + TLP), where the dialect adapter sits, reuse or
  reimplement, and the corpus policy

---

## 7. Notes on risk and consistency

- **Possible conflict with NG1** — putting the mismatch corpus into testcases violates the freeze.
  External storage is recommended
- **No conflict with NG2** — a new entry point
- **No conflict with NG4** — CUBRID is the SUT
- **§6a-E2 (SQLsmith) together with E3 (SQLancer)** — the *de facto* practice of the PostgreSQL
  ecosystem. Adopting them together is the consistent course
- **A roadmap repository cross-cutting candidate (no number assigned)** — survey §13: testkit
  §6a-E3 × {N27 lock-manager, N28 mvcc, N29 page-buffer, N30 log-buffer} — a channel for
  *consistency verification*, not regression. ~~C-013~~ cannot be used: it was registered on
  2026-05-13 as lock-manager × wait-event-stats
- **SQLancer++ (adaptive grammar)** — meaningful *in the long run* for CUBRID, a niche DBMS. It is
  at the research stage today — a separate ADR once its stability is confirmed
