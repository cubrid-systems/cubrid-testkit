# E9 — Test Corpus (STUB)

*English · [한국어](test-corpus.ko.md)*

**Status:** STUB — the corpus policy comes after formal entry into incubating through ADR-EXT-009.
**Source:** `requirements.md` §8

---

## 1. Kinds of corpus

| Corpus | Input/output | Purpose |
|---|---|---|
| seed corpus | input | meaningful operation sequences (bootstrapping coverage) |
| crash corpus | output | the op sequences that crashed — stack hash dedup |
| invariant corpus | output | the op sequences that broke an invariant with no crash |
| coverage corpus | output | libFuzzer manages it automatically |

The difference from E5, whose corpus is bytes, is that **the input here is a structure**. Adopting
libprotobuf-mutator makes it possible to store corpus entries in protobuf TextFormat, which gives
seeds *a person can read and edit by hand* — one of the practical reasons this entry prefers LPM.

## 2. Where the seed corpus comes from (agenda)

| Candidate | Cost | Note |
|---|---|---|
| boundary scenarios written by hand | low | the overflow promotion boundary / slot reuse / re-insertion after a unique violation |
| extracting the traces of operations from the existing shell and sql cases | medium | a SQL → internal op mapping is needed. Not easy to automate |
| the reproduction sequences of past CBRD storage defect tickets | medium | the greatest value as regression seeds |
| a random bootstrap (starting with no seed) | 0 | coverage rises slowly. Only as a control |

ADR-EXT-009 decides the seed policy.

## 3. Storage policy (NG1 check)

- ❌ the crash and seed corpus are not kept in the testcases repository
- ✅ a separate tree inside testkit or external storage — **sharing the same location as E5**
- ✅ seeds are text (TextFormat), so they can be diffed and reviewed → a lighter version-control
  burden than a byte corpus

## 4. The structure of a crash entry (agenda)

```
crash/<stack-hash>/
   ├── input.bin              # libFuzzer's original (the canonical form for reproduction)
   ├── sequence.txt           # the op sequence a person reads
   ├── stack.txt
   ├── sanitizer.txt
   ├── reset_strategy         # which reset strategy it came out under (essential for judging reproducibility)
   └── reproducer.sh
```

Why `reset_strategy` is recorded: reproduction depends on the reset strategy, so when the strategy
changes, whether a past crash can still be reproduced changes with it (`requirements.md` §5).

## 5. Licence

- libFuzzer: Apache 2.0
- protobuf: BSD-3-Clause
- libprotobuf-mutator: Apache 2.0

All of them are linked only into the fuzz build, so the licence composition of the shipped
artefacts does not change.

## 6. The trigger for the follow-up

After the E5 infrastructure, the state reset spike and ADR-EXT-009, this document is brought up to
FULL.
