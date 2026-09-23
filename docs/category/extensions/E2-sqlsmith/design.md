# E2 — Design (STUB)

*English · [한국어](design.ko.md)*

**Status:** STUB — the real design comes after formal entry into incubating through ADR-EXT-002.
**Source:** ROADMAP §6a-E2, `requirements.md`, `project/survey/dbms-testing-ecosystem.md` §4
**Companion:** `requirements.md` (FULL), `io-contract.md` (STUB), `test-corpus.md` (STUB)

---

## 1. Where the module sits (agenda)

```
internal/runner/sqlsmith/
   ├── introspect/        # CUBRID system catalog → schema model (db_class / db_attribute / db_serial / db_method)
   ├── ast/               # type-correct random AST generator (option to add the CUBRID dialect)
   ├── runner/            # drives the SUT (JDBC | CCI) + crash detection (signal / core / server log)
   ├── triage/            # stack hash dedup
   └── corpus/            # keeps the crash query (reproducing seed + normalised query text)
```

A *shared dialect layer* with E3 (SQLancer) is possible — subordinate to the decision in Open
Question 3 (where the dialect adapter sits).

## 2. Data flow (agenda)

```
seed → AST.generate(depth)
       └─ runner.execute(query)
            ├─ ok / empty / SQL error  → drop
            └─ CRASH (signal | core | conn drop)
                 └─ triage.dedupe(stack hash)
                      └─ corpus.save({seed, query, stack})
```

## 3. Which channel decides a crash (agenda)

| Channel | What it can detect | Note |
|---|---|---|
| process signal | SIGSEGV / SIGABRT / SIGFPE | suits an in-process driver |
| core file | server-side crash | shares the core policy of the sql and isolation modules (analysis/sql §4) |
| connection drop | server hang / restart | needs to be combined with a timeout |
| server log assert | logical assertion failure | log scraping |

ADR-EXT-002 decides on a single channel, or an order of precedence.

## 4. External dependencies

- the CUBRID system catalog (db_class / db_attribute / db_serial / db_method)
- the language ADR-001 decides, plus a JDBC/CCI client
- SQLsmith itself (if reused), or a random AST library written here
- core dump infrastructure (shared with the ADR in analysis/sql §4)

## 5. Held over for decision → ADR-EXT-002

- reuse (the SQLsmith C++ subprocess) vs reimplementation
- how far the CUBRID dialect is extended (path expression / serial / connect-by / method)
- which channel decides a crash (signal / core / server log / all of them)
- where the corpus lives (the NG1 check)

## 6. The trigger for writing the design

Once ADR-EXT-002 is agreed, this document is filled in to FULL.
