# E1 — Adopt sqllogictest (Requirements)

*English · [한국어](requirements.ko.md)*

**Source:** ROADMAP §6a-E1 + survey/dbms-testing-ecosystem.md §3
**Status:** incubating (before formal entry — waiting on ADR-EXT-001)
**Axis mapping:** axis 1 (the sqllogictest family, answer regression)
**Companion docs (to follow):** `design.md`, `io-contract.md`, `test-corpus.md`, `implementation-notes.md`

---

## 1. The problem this extension solves

It verifies CUBRID's SQL semantics by regression — a deterministic input in an **external standard
format (sqllogictest)**, diffed against a pre-recorded expected output.

The *judgement model is the same* as the existing `sql` module's `.sql` ↔ `.answer` pattern, but
these are added:
- **use of external corpus assets** — it is the format SQLite, DuckDB, CockroachDB and RisingWave
  have adopted, so *the corpus is itself an asset*
- **hash-based result representation** — even a large result set is expressed as a single hash line
  (answer files get smaller)
- **a standard that makes cross-DBMS regression possible** — the *judgement channel* for dialect
  compatibility work such as N13 pg-wire-compat

**The kind of bug it catches:** answer regression (deterministic input → wrong output). Parser
crashes and logic bugs are *outside the axis*.

---

## 2. How it is called from outside (proposed — incubating)

```
ctp.sh sqllogictest [-c <sqllogictest.conf>]
   or
testkit run sqllogictest [-c <conf>]
```

The internal entry point (agenda):
```
SqllogictestRunner.exec(config)
  └─ for each <case>.slt:
       parse record (statement / query)
       execute via {JDBC | CCI | cubrid-cli}
       compare hash or values vs expected
```

**Effect on the external surface freeze:** none. sqllogictest is a *new entry point*, so it is
orthogonal to ROADMAP NG2 (the external surface is frozen).

---

## 3. What users need (an incubating estimate)

1. **Record parsing compatible with the sqllogictest spec** — the two record types
   `statement (ok|error)` and `query <type> [sort] [label]`
2. **A hash comparison mode** — the standard sqllogictest hash (MD5 of the rows)
3. **A values comparison mode** — a raw value diff for small result sets
4. **Determinism options** — handling `sort rowsort|valuesort|nosort`
5. **Isolating non-deterministic cases** — float precision, a SELECT without ORDER BY and so on:
   *marking the cases that pass*
6. **External corpus ingestion** — absorbing a *subset* of the SQLite-origin sqllogictest tree, or
   of the DuckDB suite, into the testkit corpus
7. **Regression equivalence reporting** — classified as pass / fail / hash mismatch / dialect-skip

---

## 4. Non-functional requirements

| Item | Agenda | What it means in the new system |
|------|------|---------------------|
| Cost of adoption | low (record parser + hash + diff) | ROADMAP §6a-E1, *the candidate with the fewest dependencies* |
| Immediate ROI | ★★★★ | the external corpus can be used at once |
| Dependence on ADR-001 (implementation language) | adopting sqllogictest-rs forces Rust | an *implementation choice*, after ADR-001 is decided |
| Standardising non-deterministic results | floats, a SELECT without ORDER BY | a guide for case authors plus a skip policy |
| Licence | the sqllogictest corpus licence needs checking | *vendor in a subset* rather than a full mirror (ROADMAP §8 risk 6) |

---

## 5. External resources it depends on

- **An external corpus** — the primary target among the SQLite sqllogictest tree, the DuckDB test
  suite and CockroachDB logictest (undecided)
- **A CUBRID client** — the channel that drives the SUT, among JDBC, CCI and cubrid-cli (undecided)
- **ADR-001 (implementation language)** — bound up with whether sqllogictest-rs is adopted
- **The case-format ingestion interface** (design/contracts.md, phase 2) — the shared interface this
  entry is *the first to require*

---

## 6. The conditions for entering incubating (ROADMAP §6a-E1 Open Questions)

ADR-EXT-001 can be written once these are decided (owner: hgryoo):

1. **Pain point** — why sqllogictest, and why now? (entering an external standard / regression
   comparison against another DBMS / enlarging the test corpus / a particular RND or CBRD ticket?)
2. **Spec target** — which is the baseline: the SQLite original, the DuckDB extension, or the
   CockroachDB variant
3. **Corpus policy** — importing or mirroring the external tree, and licence verification
4. **Acceptance** — the measure: number of cases passing, hash match rate, coverage and the like
5. **Result comparison mode** — the standard sqllogictest hash, or adding a CUBRID expected file
6. **The client that drives the SUT** — JDBC, CCI or cubrid-cli
7. **Re-checking phase alignment** — whether running alongside phases 4 and 5 exceeds what one
   person has available (the branch gate, §7)

**ADR placeholder:**
- ADR-EXT-001 — which sqllogictest spec variant, the input corpus import policy, the result
  comparison mode, the client that drives the SUT

---

## 7. Notes on risk and consistency

- **No conflict with NG1 (the testcases repository is frozen)** — an external corpus is an asset
  outside testcases
- **No conflict with NG2 (the external surface is frozen)** — a new entry point
- **No conflict with NG4 (no compatibility with non-CUBRID DBMSs)** — CUBRID is on the SUT side;
  testkit is not adding compatibility with another DBMS
- **Possible conflict with the branch gate, §7** — where it competes for resources with
  strangler-fig phases 3 and 4, the strangler fig comes first (ROADMAP §8 risk 7)
- **Complementary to §6a-E5 (parser fuzzing)** — sqllogictest verifies wrong results for *SQL that
  passes the grammar*; parser crashes are a separate axis
