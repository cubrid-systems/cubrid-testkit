# E6 — Test Corpus (STUB)

*English · [한국어](test-corpus.ko.md)*

**Status:** STUB — the corpus policy comes after formal entry into incubating through ADR-EXT-006.
**Source:** `requirements.md` §5

---

## 1. Kinds of corpus

| Corpus | Input/output | Purpose |
|---|---|---|
| input case corpus | input | the SQL being compared — E1 (.slt) / E2 (generated) / written by hand |
| dialect rewrite catalogue | input | mapping a dialect difference → a rewrite pattern |
| seed corpus | input | regression seeds — the queries of past *real wrong-results* |
| mismatch corpus | output | accumulated per mismatch classification |

## 2. Where the input cases come from (agenda)

- **Borrowing E1 (.slt)** — sqllogictest assumes cross-DB application, so it can be used directly
- **E2 (sqlsmith generated)** — random valid SQL — a higher proportion of dialect skips
- **A canonical subset written by hand** — SQL-92 core SQL that is *identical to PostgreSQL's* —
  few in number

ADR-EXT-006 decides the first input source.

## 3. Keeping the dialect rewrite catalogue (agenda)

```
catalog/
   ├── date.yaml          # DATE function mapping
   ├── null.yaml          # NULL ordering / IS NULL handling
   ├── float.yaml         # floating-point tolerance / rounding
   ├── collation.yaml     # different default collation
   ├── json.yaml          # JSON function mapping
   └── overflow.yaml      # integer overflow, wrap vs error
```

testkit's own asset. No external licence obligation.

## 4. Storage policy (NG1 check)

- ❌ not kept in the testcases repository
- ✅ inside testkit (the catalogue) + external storage (the mismatch corpus)
- ✅ consistent with the corpus location policy of E1 / E2

## 5. Mismatch classification (agenda)

| classification | Meaning | What is done |
|---|---|---|
| `real_wrong_result` | a difference that is not dialect | exit code 1 — a real bug |
| `dialect_known` | matched in the catalogue | warning |
| `float_tolerance` | outside the tolerance | classified, then a person decides |
| `collation_known` | matched in the catalogue | warning |
| `unknown` | cannot be classified | needs inspection |

ADR-EXT-006 freezes the classification policy.

## 6. Licence

- PostgreSQL JDBC: BSD-2-Clause
- Where the input corpus is external (E1) — it inherits E1's licence policy
- dialect catalogue: a testkit asset

## 7. The trigger for the follow-up

After N13 enters selected and ADR-EXT-006, this document is brought up to FULL.
