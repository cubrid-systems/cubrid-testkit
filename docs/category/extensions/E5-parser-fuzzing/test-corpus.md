# E5 — Test Corpus (STUB)

*English · [한국어](test-corpus.ko.md)*

**Status:** STUB — the corpus policy comes after formal entry into incubating through ADR-EXT-005.
**Source:** `requirements.md` §5

---

## 1. Kinds of corpus

| Corpus | In/out | Use |
|---|---|---|
| seed corpus | input | an initial seed of meaningful SQL and protocol messages (bootstrapping coverage) |
| crash corpus | output | the crashes of a fuzz run, accumulated — deduplicated on the stack hash |
| coverage corpus | output | the input that updates edge coverage (libFuzzer manages it automatically) |

## 2. Where the seed corpus comes from (agenda)

| Candidate | Cost | Note |
|---|---|---|
| converting the existing sql module's cases into byte inputs | low | a *subset* of the 17,411 .sql files as fuzz seeds |
| the sqllogictest (E1) corpus | low | borrowing an external corpus |
| a seed synthesised from CUBRID's own grammar | middling | hand-crafted edge cases |
| CCI / JDBC capture replay | middling | capturing client traffic (check privacy) |

ADR-EXT-005 decides the seed policy.

## 3. Keeping policy (the NG1 check)

- ❌ the crash corpus is not put into a testcases repository
- ✅ a separate tree inside testkit, or external storage
- ✅ the seed corpus can be *generated* (by converting the sql module's cases) — treated as a cache
  where there is no need to keep it

## 4. The shape of a crash entry (agenda)

```
crash/<stack-hash>/
   ├── input.bin               # raw bytes (libFuzzer's format)
   ├── input.repr              # a human-readable representation (where one is possible)
   ├── stack.txt
   ├── sanitizer.txt
   ├── target_layer            # parser / cci / jdbc
   └── reproducer.sh
```

## 5. Licence

- libFuzzer: Apache 2.0
- AFL: Apache 2.0
- honggfuzz: Apache 2.0
- where an external seed corpus is used, check the licence of its source

## 6. The trigger for writing the rest

After the PR to the cubrid repository and ADR-EXT-005, this document is filled in to FULL.
