# E2 — Test Corpus (STUB)

*English · [한국어](test-corpus.ko.md)*

**Status:** STUB — the corpus policy comes after formal entry into incubating through ADR-EXT-002.
**Source:** `requirements.md` §5, ROADMAP §8 risk (the E2/E3 corpus and NG1)

---

## 1. Kinds of corpus

This entry *has no input corpus* — generation is random, so a corpus means *an accumulating store of
the crashes produced*.

| Corpus | In/out | Use |
|---|---|---|
| seed corpus | input | regression seed — replaying past crash queries against a new build |
| crash corpus | output | the crashes of a fuzz run, accumulated — deduplicated on the stack hash |

## 2. Keeping policy (the NG1 check)

- ❌ not put into a testcases repository (cubrid-testcases / -private / -private-ex) — that
  violates NG1
- ✅ a separate tree inside testkit (`corpus/sqlsmith/`, say), or external storage
- ✅ one entry kept per stack hash directory (dedup)

ADR-EXT-002 states where the corpus lives, how long it is kept, and the GC policy.

## 3. The shape of a crash entry (agenda)

```
crash/<stack-hash>/
   ├── seed              # the random seed for reproduction
   ├── query.sql         # the normalised SQL
   ├── stack.txt         # stack / signal / registers
   ├── schema.sql        # the schema dump needed to reproduce it
   └── reproducer.sh     # a one-shot reproduction script
```

## 4. The dedup policy (agenda)

- first: the hash of the top N stack frames (excluding the signal frame)
- second: the hash of the query AST shape (normalised with the literals removed)

ADR-EXT-002 freezes N and the normalisation rules.

## 5. Licence and external dependencies

- SQLsmith itself (if reused): a custom licence — the vendoring policy needs checking (survey §12.8)
- there is no input corpus, so an external licence obligation arises *only when reuse is decided*

## 6. The trigger for writing the rest

Once ADR-EXT-002 decides where the corpus lives and the dedup rules, this document is filled in to
FULL.
