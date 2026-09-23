# E1 — Design (STUB)

*English · [한국어](design.ko.md)*

**Status:** STUB — the real design comes after formal entry into incubating through ADR-EXT-001.
**Source:** ROADMAP §6a-E1, `requirements.md`, `project/survey/dbms-testing-ecosystem.md` §3
**Companion:** `requirements.md` (FULL), `io-contract.md` (STUB), `test-corpus.md` (STUB)

---

## 1. Where the module sits (agenda)

```
internal/runner/sqllogictest/        # new module — joined to the testkit skeleton through the case-format ingestion interface
   ├── parser/            # .slt record parser (statement / query)
   ├── runner/            # drives the SUT (JDBC | CCI | cubrid-cli)
   ├── compare/           # hash comparison + values comparison + sort option handling
   └── report/            # classifies pass / fail / hash mismatch / dialect-skip
```

Whether sqllogictest-rs is depended on directly (`Rust`) or the thing is implemented here is decided
by ADR-001 (implementation language).

## 2. Data flow (agenda)

```
.slt file
  └─ parser
       └─ record stream { Statement(ok|error) | Query(type, sort, [hash|values]) }
            └─ runner.execute(record)
                 └─ compare.diff(actual, expected)
                      └─ report.classify
```

## 3. External dependencies

- the language ADR-001 decides, plus a JDBC/CCI client
- an external sqllogictest corpus (see test-corpus.md)
- the case-format ingestion interface in phase 2's `project/design/contracts.md`

## 4. Held over for decision → ADR-EXT-001

- the spec variant (SQLite / DuckDB / CockroachDB)
- the result comparison mode (hash vs values vs adding a CUBRID expected file)
- the client that drives the SUT (JDBC / CCI / cubrid-cli)
- reusing sqllogictest-rs vs implementing it here

## 5. The trigger for writing the design

Once ADR-EXT-001 is agreed, this document is filled in to *FULL*. For now it is a holding place for
the agenda.
