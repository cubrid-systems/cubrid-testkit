# E6 — Design (STUB)

*English · [한국어](design.ko.md)*

**Status:** STUB — the real design comes after formal entry into incubating through ADR-EXT-006.
N13 pg-wire-compat at *selected* or beyond is recommended.
**Source:** ROADMAP §6a-E6, `requirements.md`, `project/survey/dbms-testing-ecosystem.md` §8
**Companion:** `requirements.md` (FULL), `io-contract.md` (STUB), `test-corpus.md` (STUB)

---

## 1. Where the module sits (agenda)

```
internal/runner/differential/
   ├── input/            # case corpus loader (E1 / E2 / written by hand)
   ├── runner/
   │     ├── cubrid/     # JDBC | CCI | pg-wire (after N13)
   │     └── peer/       # PostgreSQL JDBC
   ├── rewrite/          # dialect rewrite layer (DATE / NULL / float / collation / JSON / overflow)
   ├── compare/          # row-by-row comparison + sort handling
   └── classify/         # mismatch classification: real wrong-result / known dialect / float / collation
```

## 2. The mode branch (agenda)

```
canonical mode:
  case → run on both → compare assert identical
  scope: the SQL-92 core subset

rewrite mode:
  case → rewrite_for_peer(case) → run peer
       → run cubrid raw → compare
  scope: whatever the dialect rewrite catalogue covers
```

## 3. The dialect rewrite catalogue (agenda)

| Category | Example of the difference | Rewrite pattern |
|---|---|---|
| DATE functions | `DATE_ADD` ↔ `+ INTERVAL` | function mapping |
| NULL ordering | different NULLS FIRST default | state it explicitly |
| float precision | precision / rounding differences | tolerance compare |
| collation | different default collation | state COLLATE explicitly |
| JSON functions | different function names | function mapping |
| integer overflow | wrap vs error | restrict the range |

ADR-EXT-006 picks the first categories.

## 4. External dependencies

- N13 pg-wire-compat (at *selected* or beyond recommended)
- A PostgreSQL instance + the JDBC driver
- A case corpus (E1 / E2 / by hand)
- The dialect rewrite catalogue (testkit's own asset)

## 5. Decisions held over → ADR-EXT-006

- the first mode (canonical vs rewrite)
- the first categories of the dialect rewrite catalogue
- the first peer DBMS (PostgreSQL only / + MySQL / + SQLite)
- the input source for the case corpus
- where the mismatch corpus lives (NG1 check)

## 6. The trigger for writing the design

After N13 enters selected and ADR-EXT-006. A stub for now.
