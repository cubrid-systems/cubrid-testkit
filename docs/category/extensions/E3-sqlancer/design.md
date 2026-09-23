# E3 — Design (STUB)

*English · [한국어](design.ko.md)*

**Status:** STUB — the real design comes after formal entry into incubating through ADR-EXT-003.
**Source:** ROADMAP §6a-E3, `requirements.md`, `project/survey/dbms-testing-ecosystem.md` §5
**Companion:** `requirements.md` (FULL), `io-contract.md` (STUB), `test-corpus.md` (STUB)

---

## 1. Where the module sits (agenda)

```
extensions/cubrid-sqlancer/   # settled: a separate repository (ADR-EXT-003)
   ├── oracle/
   │     ├── norec/      # WHERE p ↔ COUNT(*) WHERE (p IS TRUE), comparing rowcount
   │     ├── tlp/        # WHERE p ↔ the union of p IS TRUE / IS FALSE / IS NULL
   │     └── pqs/        # pivot preservation (a second, later step, cost ↑)
   ├── ast/              # can be shared with E2 — on the agenda (Open Question 3)
   ├── runner/           # drives the SUT (JDBC | CCI)
   └── corpus/           # keeps mismatches
```

## 2. Where the dialect adapter sits (agenda)

```
internal/catalog/                # candidate 1 — a common layer inside testkit (shared by E2/E3)
   ├── catalog.{rs,go,java} # an abstraction over the CUBRID system catalog
   ├── grammar.*            # the CUBRID dialect added on (path / serial / connect_by / method)
   └── adapter.*            # SQL emitter

# vs

internal/runner/sqlsmith/dialect/       # candidate 2 — spread per tool
extensions/cubrid-sqlancer/   # settled: a separate repository (ADR-EXT-003)dialect/
```

Decided in ADR-EXT-003 (Open Question 3).

## 3. Data flow (agenda — NoREC)

```
seed → ast.generate(predicate p, table t)
       ├─ Q1 = "SELECT * FROM t WHERE p"
       └─ Q2 = "SELECT COUNT(*) FROM (SELECT (p) IS TRUE p FROM t) WHERE p"
            └─ runner.execute(Q1).rowcount == runner.execute(Q2).scalar
                 └─ if mismatch: corpus.save({Q1, Q2, schema, seed})
```

## 4. Data flow (agenda — TLP)

```
seed → ast.generate(predicate p, base Q)
       └─ Q_true   = base WHERE p IS TRUE
          Q_false  = base WHERE p IS FALSE
          Q_null   = base WHERE p IS NULL
          Q_total  = base
            └─ row_set(Q_total) == row_set(Q_true) ⊎ row_set(Q_false) ⊎ row_set(Q_null)
```

## 5. External dependencies

- the same schema introspection as E2 (sharing the dialect adapter where possible)
- the language ADR-001 decides, plus JDBC/CCI
- SQLancer itself (if reused, MIT)

## 6. Held over for decision → ADR-EXT-003

- which oracle first (NoREC + TLP recommended)
- where the dialect adapter sits (shared with E2 vs per tool)
- reuse (SQLancer Java) vs reimplementation
- where the mismatch corpus lives (the NG1 check)

## 7. The trigger for writing the design

Once ADR-EXT-003 is agreed, this document is filled in to FULL.
