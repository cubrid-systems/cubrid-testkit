# E6 — I/O Contract (STUB)

*English · [한국어](io-contract.ko.md)*

**Status:** STUB — the contract is frozen after formal entry into incubating through ADR-EXT-006.
**Source:** `requirements.md` §2

---

## 1. CLI (proposed)

```
ctp.sh diff-pg [-c <diff.conf>] [--peer pg|mysql|sqlite] [--mode canonical|rewrite]
   or
testkit run differential --peer <name> [--cases <corpus>] [--seed <N>] [--mode <name>]
```

## 2. conf schema (TBD)

| Key (agenda) | Value | Where it comes from |
|---|---|---|
| `peer` | `pg \| mysql \| sqlite` | choice of peer DBMS |
| `mode` | `canonical \| rewrite` | how dialect is handled |
| `cases_root` | `<dir>` | the input corpus (E1 .slt / E2 generated and so on) |
| `peer_jdbc_url` | string | the peer's connection URL |
| `cubrid_client` | `jdbc \| cci \| pg_wire` | the client on the CUBRID side |
| `tolerance_float` | float | the tolerance for floating-point comparison |
| `corpus_root` | `<dir>` | where the mismatch corpus lives |
| `dialect_categories` | csv `date,null,float,collation,json,overflow` | which rewrites are active |

Keys and meanings are frozen after ADR-EXT-006.

## 3. Output format (TBD)

```
<resultDir>/
   ├── main.info
   ├── mismatch/
   │     └── <classification>/<witness-hash>/
   │           ├── case.sql
   │           ├── case_rewritten.sql      # in rewrite mode
   │           ├── result_cubrid.tsv
   │           ├── result_peer.tsv
   │           ├── classification.txt      # real / dialect / float / collation
   │           └── reproducer.sh
   └── stats.json                          # cases / real wrong-results / dialect skips
```

## 4. Exit codes (TBD)

| Value | Meaning |
|---|---|
| 0 | no *real wrong-result* within the time budget (dialect mismatch excluded) |
| 1 | a real wrong-result found |
| ≥2 | an infrastructure error (failure to reach the peer and so on) |

Where there is only a dialect mismatch, exit code 0 plus a warning is recommended (ADR-EXT-006).

## 5. NG2 / NG4 check

A new entry point — no conflict. **An explicit NG4 (no compatibility with non-CUBRID DBMSs)
check**: this entry is *verification by comparison with a peer DBMS* — it is not *CUBRID adding
compatibility with another DBMS*.
