# E2 — I/O Contract (STUB)

*English · [한국어](io-contract.ko.md)*

**Status:** STUB — the contract is frozen after formal entry into incubating through ADR-EXT-002.
**Source:** `requirements.md` §2

---

## 1. CLI (proposed)

```
ctp.sh sqlsmith [-c <sqlsmith.conf>]
   or
testkit run sqlsmith [-c <conf>] [--seed <N>] [--time <sec>] [--max-depth <D>] [--client jdbc|cci]
```

The `--continue` mode: replays the crash queries of a past corpus against a new build.

## 2. conf schema (TBD)

| Key (agenda) | Value | Source |
|---|---|---|
| `seed` | `<int>` | for reproduction |
| `time_budget_sec` | `<int>` | controlling CI time |
| `max_depth` | `<int>` | the maximum depth of a nested AST |
| `client` | `jdbc \| cci` | the channel that drives the SUT |
| `dialect_extensions` | csv `path,serial,connect_by,method` | toggles for the CUBRID extensions |
| `corpus_root` | `<dir>` | where the crash corpus lives |
| `crash_channel` | `signal \| core \| server_log \| all` | the channel that decides a crash |

Keys and meanings are frozen after ADR-EXT-002.

## 3. Output format (TBD)

```
<resultDir>/
   ├── main.info              # compatible (borrowing the sql module's grep patterns, under review)
   ├── crash/
   │     └── <stack-hash>/
   │           ├── seed
   │           ├── query.sql
   │           ├── stack.txt
   │           └── reproducer.sh
   └── stats.json             # runs / unique crashes / coverage
```

## 4. Exit codes (TBD)

| Value | Meaning |
|---|---|
| 0 | no crash within the time |
| 1 | a crash was found (kept in corpus/) |
| ≥2 | an infrastructure error (the DB is not up, the conf is wrong, and so on) |

Frozen after ADR-EXT-002.

## 5. The NG2 / NG4 check

A new entry point — no conflict.
