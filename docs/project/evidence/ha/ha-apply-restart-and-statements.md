# `testkit ha-apply restart --statements`: missed before the statement marker, held on `787defb50`

- **Date:** 2026-11-02
- **What this is:** the restart evidence left statements out: "the marker covers a DDL or a DML
  replicated as a statement too; the workload has only rows"
  ([ha-apply-restart-and-the-applied-tx-marker](ha-apply-restart-and-the-applied-tx-marker.md) §4).
  The master replicates as a statement a DDL, `DROP VARIABLE`, and an `INSERT`, `UPDATE` or `DELETE`
  with the `USE_SBR` hint (`is_stmt_based_repl_type`, `execute_statement.c`); the slave runs each
  again as SQL. `--statements` gives the restart workload's INSERT and its move that hint. Statement
  transactions were protected before row transactions: `d398beba2` ("a statement transaction is
  never applied twice after a restart") records each one's commit in a one-row slave table,
  `ha_applied_stmt_marker`, in the same slave transaction, and `72650ce0d` replaced that table with
  `ha_applied_tx_marker`. So the heads are `b84567f36`, the parent of `d398beba2`, where a run
  should be missed; `19e83a5d9`, which has the statement marker and not `ha_applied_tx_marker`; and
  `787defb50`, #8052's head.
- **Command:** `testkit ha-apply restart --build <an install of the head> --statements --keep`,
  built from this change, except the `787defb50` run (before the wait became a function, the same
  30 minutes). The applier is killed, 8 clients × 3,000 keys, `--kill-every 5s`.
  `ha_apply_worker_count=4`.
- **Pairs:** RelWithDebInfo, each with a client node, on one host; all removed afterwards.

---

## 1. The workload did replicate as statements

`SHOW WRITESET STATUS` on the `19e83a5d9` and `787defb50` masters after their runs: `Commit_count
48007`, `Fallback_statement_count 48005` (`s-19e8-writeset.txt`, `s-787d-writeset.txt`). The
clients' commits alone, 24,000 inserts and 24,000 moves, are 48,000, so each was a statement the
writeset did not hash and that fell back to a serial apply on the slave.

## 2. The runs

| #8052 | pair | kills, starts | slave | key sums (master / slave) | `fail_counter` / -670 | verdict |
|---|---|---|---|---|---|---|
| `b84567f36` (`11.5.0.2778-b84567f`) | `hap11027fea` | 27, 104 | caught up: 24,481 rows to 24,000, 481 keys not moved | 1,200,036,012,000 / 1,221,536,459,940 | +803 / +1,606 | **missed** |
| `19e83a5d9` (`11.5.0.2932-19e83a5`) | `hap11021d43` | 37, 36 | caught up: 24,000 rows, no key unmoved | 1,200,036,012,000 / the same | 0 / 0 | inconclusive |
| `787defb50` (`11.5.0.2991-787defb`) | `hap11022bde` | 28, 28 (all 28 with markers, up to 27) | caught up: 24,000 rows, no key unmoved | 1,200,036,012,000 / the same | 0 / 0 | **held** |

- **`b84567f36`**: 481 inserts applied a second time brought their keys back, and their moves met
  the moved keys. That accounts for 481 of the 803 failures; the other 322 are likely inserts run
  again before their key was moved, which fail on it and change nothing, but the applier log was
  not kept to show it. The applier started 104 times in 27 kills: 77 starts came without a kill.
  No writeset read was taken on this head. That its DML went as statements is shown only
  indirectly: 1,606 lines with -670 to 803 failures, two to one, where every row run had them one
  to one (16 to 147 of each); a failed statement logs the -670 and then the applier's own error,
  which carries it again.
- **`19e83a5d9`**: inconclusive, since this tool counts the markers in `ha_applied_tx_marker`,
  which this head does not have. The slave, caught up within the 30 minutes, held what the master
  held, with no failure.
  - An earlier run on this head, built before the wait was 30 minutes (`haapply-s-19e8.tsv`), gave
    the same: it waited 5 minutes, the slave was 7,608 rows in, and read by hand as it caught up
    (`s-19e8-after-reads.txt`, by `tools/after-reads.sh`) it held what the master held about 20
    minutes after the clients ended.
  - 37 kills against 36 starts, and 34 against 32 in the earlier run, were not looked into.
- **`787defb50`**: every kill found markers above the stored position, at most 27, against about
  2,000 in the row workload.
  - The coordinator stores the apply position at most once per
    `ha_applylogdb_max_commit_interval_in_msecs` (`la_coordinator_store_apply_info_at_interval`).
    So the markers above it are the commits applied since the last store, fewer under a serial
    apply.

A run took about 20 minutes on `b84567f36` and `19e83a5d9` and about 11.5 on `787defb50`, from the
clients' first connection to the ledger.

## 3. What this says

- `--statements` can miss: on `b84567f36`, before any marker, it did.
- `19e83a5d9` and `787defb50` both skip a statement transaction already applied, through
  `ha_applied_stmt_marker` and `ha_applied_tx_marker` respectively, and neither run showed a second
  apply. `--statements` therefore does not separate those two heads, by the code; the two runs, one
  each, agree.
- What the checks can see: an INSERT applied a second time brings its key back, or fails on it.
  A move applied a second time (`UPDATE ... SET id = k + 5000000 WHERE id = k`) finds no row and
  succeeds without a word. So "held" here is "no second apply changed the table or failed".

Files, in the working directory:
- the ledgers `haapply-s3-b845.tsv`, `haapply-s3-19e8.tsv`, `haapply-s-787d.tsv` and
  `haapply-s-19e8.tsv`, each with its `.d` directory and its run's `.out` and `.err`;
- the two writeset reads, and `s-19e8-after-reads.txt` with its `.progress` file;
- the binaries `testkit-sbr1` (the earlier `19e83a5d9` run), `testkit-sbr2` (`787defb50`) and
  `testkit-sbr3` (the others).

## 4. Not here

- **A DDL workload.** A DDL is replicated as a statement too, and a second apply of a `CREATE TABLE`
  fails (`d398beba2` names -492, -245 and -272); nothing here runs DDL under kills.
- **A non-idempotent statement**, such as `SET n = n + 1`, whose second apply changes the table
  where a second move does not.
- **`--kill node` with `--statements`.**
- **More than one run** on each head.
