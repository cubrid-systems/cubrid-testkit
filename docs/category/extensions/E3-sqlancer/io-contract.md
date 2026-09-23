# E3 — I/O Contract (STUB)

*English · [한국어](io-contract.ko.md)*

**Status:** STUB — the contract is frozen after formal entry into incubating through ADR-EXT-003.
**Source:** `requirements.md` §2

---

## 1. CLI (proposed)

```
ctp.sh sqlancer [-c <sqlancer.conf>] [--oracle norec|tlp|pqs]
   or
testkit run sqlancer [-c <conf>] [--oracle <name>] [--seed <N>] [--time <sec>] [--client jdbc|cci]
```

Naming several oracles at once is on the agenda: `--oracle norec,tlp` (a mismatch is tagged with its
oracle when it is classified).

## 2. conf schema (TBD)

| Key (agenda) | Value | Source |
|---|---|---|
| `seed` | `<int>` | for reproduction |
| `time_budget_sec` | `<int>` | controlling CI time |
| `oracle` | csv `norec,tlp,pqs` | choice of oracle |
| `client` | `jdbc \| cci` | the channel that drives the SUT |
| `corpus_root` | `<dir>` | where the mismatch corpus lives |
| `dialect_extensions` | (shared with E2) | the CUBRID extensions added on |

Keys and meanings are frozen after ADR-EXT-003.

## 3. Output format (TBD)

```
<resultDir>/
   ├── main.info
   ├── mismatch/
   │     └── <oracle>/<seed>/
   │           ├── q1.sql
   │           ├── q2.sql           # NoREC: the rewrite / TLP: the partition triple
   │           ├── schema.sql
   │           ├── result_q1.tsv
   │           └── result_q2.tsv
   └── stats.json                   # iterations / unique mismatches / per-oracle hit
```

## 4. Exit codes (TBD)

| Value | Meaning |
|---|---|
| 0 | no mismatch within the time |
| 1 | a mismatch was found |
| ≥2 | an infrastructure error |

## 5. The NG2 / NG4 check

A new entry point — no conflict. Orthogonal to NG4 as well (CUBRID is the SUT).
