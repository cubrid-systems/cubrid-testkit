# Coverage gaps on #8052 `787defb50`: small history, worker counts, memory limit, serial fragments

- **Date:** 2026-11-02
- **What this is:** a review of #8052 against what this tool had checked (its commits, the
  writeset's own counters, and CBRD-27459's comments) left four gaps that are cheap to close:
  - the writeset history overflowing, and with it the per-transaction key limit, which is the same
    number (`wset_push_hash` falls back to commit order when a transaction holds
    `ha_writeset_history_size` keys);
  - worker counts other than 4;
  - the applier's memory limit;
  - the serial fragments: a system operation that replicates rows (the `_db_serial` write-back of
    an AUTO_INCREMENT or a SERIAL, `INCR`), which the slave applies in commit order apart from its
    transaction.
- **Command:**
  - The verbs as they are, built from `main` at `3f124e4` (the tree differed from it only in the
    `cubrid-sqlancer` gitlink), each on a pair it stood up and removed.
  - For the serial fragments, a shell prototype, `parallel-apply-serial.sh`, kept here in its last
    form: the first run below had no large rows, and only the two runs alone had the no-kill mode
    and the clients' own time.
  - `ha_apply_worker_count=4` unless named.
- **Pairs:** #8052 `787defb50` (`11.5.0.2991-787defb`), and `3fb54e549` (`11.5.0.2967-3fb54e5`) as a
  control; RelWithDebInfo, each with a client node, on one host; all removed afterwards.

---

## 1. A small history

| run | pair | result | the writeset |
|---|---|---|---|
| `stress --history 1000 --ops 0,1,2,3,4,9 --seed 1` | `hap11022ce4` | **same** | 468 fallbacks, 468 history clears, 112 write conflicts |
| `pairs --history 50000` | `hap11021edd` | 32 patterns **held**; the control first in 11 of 20 rounds | 9 clears in each of 12 patterns |

With 1,000 keys, a transaction that writes more falls back to commit order, as `stress`'s
3,000-row ballast does. The ledger gives the fallbacks as one count and does not say which were
for the key limit.

With 50,000 keys the history cleared in 12 patterns, 9 rounds of 20 in each: the control, nine of
the `fk_*` patterns (not `fk_delete`, `fk_cascade`, `fk_repoint` or `fk_cascade_reuse`), `sp_keep`
and `sp_redo`. `dbl_nan` and `ddl_dml` cleared in all 20 rounds too, as they do at the default size
(`haapply-g-pairs-w16.tsv`); the other 19 patterns did not clear at all. A clear puts what follows
it behind it, and the control went first in 11 rounds and cleared in 9, 20 together; the ledger
gives totals, not rounds. So in a round with a clear the dependency holds by construction, and
these runs show the clears do not break it, not that the hash holds it across one. Every earlier
run used the default, 1,000,000 keys: their `stress` ledgers give no fallback and at most one
clear.

## 2. Other worker counts

| run | pair | result |
|---|---|---|
| `stress --workers 1 --ops 0,1,2,3,4,9 --seed 1` | `hap110271ab` | **same** |
| `stress --workers 16 --ops 0,1,2,3,4,9 --seed 1` | `hap1102efd5` | **same** |
| `pairs --workers 16` | `hap1102b943` | 32 patterns **held**; the control first in 19 of 20 rounds |
| `restart --workers 16` | `hap1102fa6c` | **held**: 41 kills, all with markers above the stored position, up to 8,219; 40 starts |

With 16 workers the markers a kill found went up to 8,219, against about 2,000 with 4.

## 3. The memory limit

`restart --set ha_apply_max_mem_size=1` (`hap11021107`) was **held**: 47 kills, all with markers, 46
starts. The applier never stopped on its limit: it started no more often than it was killed. Its
limit is the larger of `ha_apply_max_mem_size` and twice its virtual size once the workers have
started (`la_check_mem_size`; sixty times on AIX). But that check returns at once for the first
10 seconds of an applier's life (`HB_START_WAITING_TIME_IN_SECS`), and the kills came every 5
seconds, so it may rarely or never have run; nothing read the applier's size. The limit's path is
still not run.

## 4. Serial fragments

`parallel-apply-serial.sh`: each client, again and again, commits four things:
- an INSERT whose id is an AUTO_INCREMENT;
- one whose id is a NOCACHE serial's `NEXT_VALUE`;
- an `INCR` of its own counter;
- an `INCR` of one of its 50 rows of about 3 KB.

The large rows are for `501e5f315`, which fixed increments lost when an `INCR`'s heap update needed
a new page. The slave's applier is killed every 5 seconds while the clients run. Once the slave has
caught up, both nodes are read: each table's rows and key sum, the counters' sums, and both serials'
current values in `_db_serial` (the AUTO_INCREMENT's is not in the `db_serial` view).

| #8052 | pair | clients × iterations | kills, starts | once caught up |
|---|---|---|---|---|
| `787defb50` | `sf787d` | 8 × 2,000, without the large rows | 102, 101 | the same on both nodes: 16,000 of each, both serials at 16,000; no failure, no -670 |
| `787defb50` | `sf787d` | 8 × 1,000 | 136, 136 | the same: 8,000 of each, both serials at 8,000; no failure, no -670 |
| `3fb54e549` (before `501e5f315`) | `sf3fb5` | 8 × 1,000 | 24, 24 | the same |

- **The second `787defb50` run** did not show its final marker within the prototype's 30 minutes.
  The slave was applying, about 460 to 500 iterations a minute, and showed 8,000 rows 145 seconds
  into the reads that followed (`sf-787d-p2-after.txt` and its `.progress`).
- **The `3fb54e549` run** did not lose an increment. So nothing here shows the check can miss: the
  workload does not reproduce `501e5f315`'s case, whose page allocation it may not reach.
- **Speed.** Under kills the two heads took very different times, from each log's start to its
  clients' end (about 10 seconds of setup included): `787defb50` 11 minutes 52, `3fb54e549` 2
  minutes 16. The `787defb50` clients shared the host with `pairs --history 50000`, and its catch-up
  with that run's end and then `pairs --workers 16`; the `3fb54e549` clients only with the reads of
  `sf787d`'s catch-up. Run alone, one after the other and with no kill, the same workload took 123
  seconds of clients and 323 of catch-up on `787defb50`, and 127 and 343 on `3fb54e549`. So the
  heads do not differ when run alone; the slow run is consistent with what it ran beside, but
  `787defb50` under kills on an idle host was not run.

## 5. Where this leaves the gaps

| gap | now |
|---|---|
| the history overflowing and clearing | run: `stress` same; `pairs` held, its clears in 12 patterns ordering by construction; no head that should fail |
| the per-transaction key limit | run inside the same `stress`, not counted apart |
| worker counts | 1 and 16 run, besides 4 |
| the memory limit | not run: the check skips an applier's first 10 seconds |
| serial fragments | run under kills on two heads, the same; the check is not shown to miss |

## 6. Not here

- A head that should fail the history runs: no #8052 fix is known to be about a clear losing a
  dependency.
- A run that reaches the applier's memory limit: kills further apart than 10 seconds, or none.
- Reads on the standby while it applies; DDL in `stress`; LOBs and records past a page.
- `INCR` on a heap that needs a new page for sure, as the HA test in `501e5f315`'s message had.

Files, in the working directory:
- the ledgers `haapply-g-*.tsv` with `g-*.out` and `g-*.err`;
- `sf-787d-p1.log`, `sf-787d-p2.log` with `sf-787d-p2-after.txt`, `sf-3fb5-p1.log` with
  `sf-3fb5-p1-after.txt`, `sf-3fb5-speed.log`, `sf-787d-speed.log` with
  `sf-787d-speed-after.txt`;
- the clients' files in `sf-787d.clients/`;
- `tools/serial-after-reads.sh`.
