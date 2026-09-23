# E1 — I/O Contract (STUB)

*English · [한국어](io-contract.ko.md)*

**Status:** STUB — the contract is frozen after formal entry into incubating through ADR-EXT-001.
**Source:** `requirements.md` §2

---

## 1. CLI (proposed)

```
ctp.sh sqllogictest [-c <sqllogictest.conf>]
   or
testkit run sqllogictest [-c <conf>] [--variant sqlite|duckdb|cockroach] [--client jdbc|cci|cli]
```

Options, defaults and exit codes: frozen after ADR-EXT-001.

## 2. conf schema (TBD)

| Key (agenda) | Value | Source |
|---|---|---|
| `corpus_root` | `<dir>` | where the external corpus lives (test-corpus.md) |
| `client` | `jdbc \| cci \| cli` | the channel that drives the SUT |
| `variant` | `sqlite \| duckdb \| cockroach` | the spec baseline |
| `compare_mode` | `hash \| values` | the result comparison mode |

Until ADR-EXT-001 is agreed this is *at the level of a proposal*. Key names, meanings and defaults
are none of them frozen.

## 3. Input format

`.slt` — the sqllogictest spec record format:
```
statement (ok|error)
<SQL>
----
<expected error msg, optional>

query <type> [<sort>] [label]
<SQL>
----
<hash | rows>
```

## 4. Output format and exit codes (TBD)

After ADR-EXT-001 — whether it is compatible with `<resultDir>/main.info` is decided then.

## 5. The NG2 check (the external surface is frozen)

None — a new entry point. This entry is a new addition *outside the external surface freeze*.
