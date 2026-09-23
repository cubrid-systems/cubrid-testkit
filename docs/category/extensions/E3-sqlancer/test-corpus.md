# E3 — Test Corpus (STUB)

*English · [한국어](test-corpus.ko.md)*

**Status:** STUB — the corpus policy comes after formal entry into incubating through ADR-EXT-003.
**Source:** `requirements.md` §5, ROADMAP §8 risk (the E2/E3 corpus and NG1)

---

## 1. Kinds of corpus

The same as E2 — *no input corpus* (generation is random). The corpus is *an accumulating store of
mismatches*.

| Corpus | In/out | Use |
|---|---|---|
| seed corpus | input | regression seed — replaying past mismatching query pairs against a new build |
| mismatch corpus | output | oracle violations, accumulated |

## 2. Keeping policy (the NG1 check)

- ❌ not put into a testcases repository
- ✅ a separate tree inside testkit, or external storage
- ✅ *shares* the corpus location policy with E2 (bound up with Open Question 3)

## 3. The shape of a mismatch entry (agenda)

```
mismatch/<oracle>/<witness-hash>/
   ├── seed
   ├── q1.sql
   ├── q2.sql              # NoREC: the rewrite / TLP: the 3-way partition
   ├── schema.sql
   ├── result_q1.tsv
   ├── result_q2.tsv
   └── reproducer.sh
```

## 4. The dedup policy (agenda)

- witness-hash = the hash of (oracle, AST shape after literal normalisation, schema fingerprint)
- several mismatches with the same witness are one entry

ADR-EXT-003 freezes the normalisation rules.

## 5. Licence

- SQLancer itself (if reused): MIT — vendoring is free (survey §12.8)
- no obligation over an input corpus

## 6. The trigger for writing the rest

Once ADR-EXT-003 decides where the corpus lives and the dedup rules, this document is filled in to
FULL.
