# E1 — Test Corpus (STUB)

*English · [한국어](test-corpus.ko.md)*

**Status:** STUB — the corpus policy comes after formal entry into incubating through ADR-EXT-001.
**Source:** `requirements.md` §5, ROADMAP §8 risk 6

---

## 1. Where the input corpus comes from (agenda)

| Candidate | Size | Licence | Note |
|---|---|---|---|
| The SQLite sqllogictest original | 5.8M rows | Public Domain | the original — first in line for the baseline |
| DuckDB test suite (the `.slt` part) | many | MIT | many cases tied to DuckDB's internal estimates |
| CockroachDB logictest | many | Apache 2.0 | many cases depending on distribution or SERIAL |
| RisingWave logictest | many | Apache 2.0 | streaming SQL — little applicability to CUBRID |

Subordinate to the *spec target* decision in ADR-EXT-001.

## 2. Licence check (required)

- SQLite Public Domain — vendoring is free
- DuckDB MIT — vendoring is free (include the NOTICE)
- CockroachDB Apache 2.0 — vendoring is free (include the NOTICE)

Even where only a *subset* of a corpus is vendored in, stating the source and the licence is
obligatory.

## 3. Keeping policy (the NG1 check)

The testcases repositories (cubrid-testcases / -private / -private-ex) are *what the freeze covers*.
This corpus:

- ❌ is not put inside a testcases repository (that violates NG1)
- ✅ a separate tree inside testkit (`corpus/sqllogictest/`, say), or external storage
- ✅ *vendor in a meaningful subset* rather than a full mirror (ROADMAP §8 risk 6)
- ✅ where the external tree is synchronised automatically, it is run by a sync script

## 4. The dialect-skip policy (agenda)

Cases using SQL features CUBRID does not support (a dependency on a PostgreSQL extension, say, or
something SQLite-specific) are classified as *skip*. The frozen form: after ADR-EXT-001.

## 5. The trigger for writing the rest

Once ADR-EXT-001 decides the *corpus policy*, this document is filled in to FULL.
