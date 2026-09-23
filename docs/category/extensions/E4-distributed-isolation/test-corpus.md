# E4 — Test Corpus (STUB)

*English · [한국어](test-corpus.ko.md)*

**Status:** STUB — the corpus policy comes after formal entry into incubating through ADR-EXT-004.
**Source:** `requirements.md` §5

---

## 1. Kinds of corpus

| Corpus | In/out | Use |
|---|---|---|
| anomaly catalogue | input | the Adya/Bailis taxonomy plus Hermitage — a semantic catalogue (a knowledge asset) |
| seed corpus | input | regression seed — replaying the fault sequence of a past violation |
| violation corpus | output | the anomalies the analyzer detected, accumulated |

## 2. The anomaly catalogue (borrowed from Hermitage)

- the Adya classification: dirty read / lost update / read skew / write skew / phantom and the rest
- the Bailis classification: causal / monotonic / read-your-writes and the rest
- kept as testkit's own asset — no external licence obligation (it is knowledge)

## 3. Keeping policy (the NG1 check)

- ❌ not put into a testcases repository
- ✅ inside testkit, or external storage
- ✅ a history can grow huge (consider AWDIT's *huge history scalability*) — a GC policy has to be
  stated

## 4. The shape of a violation entry (agenda)

```
violations/<witness-hash>/
   ├── seed
   ├── topology.json       # node count / version / config
   ├── fault_seq.json      # the fault timeline for reproduction
   ├── history_excerpt.log # the cycle, or just before and just after the violation
   ├── analyzer_report.txt # AWDIT's or Jepsen's diagnosis
   └── reproducer.sh
```

## 5. Licence

- AWDIT: the exact repository / artifact needs confirming (filled in on entry to incubating)
- Jepsen: Eclipse Public License 1.0 (Clojure)
- Hermitage / Elle: knowledge assets (not the adoption of the tools themselves)

## 6. The trigger for writing the rest

After ADR-EXT-004 and N24/N11 graduation, this document is filled in to FULL.
