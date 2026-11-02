# `testkit ha-apply restart`: an applier killed under load applies commits twice on `19e83a5d9`, not on `b984637bb`

- **Date:** 2026-11-01
- **What this is:** M3 of ADR-EXT-012. In #8052 a worker commits a transaction on its own connection
  before the coordinator stores the apply info, at most every 500 ms, so an applier that dies in
  between meets, when heartbeat starts it again, commits the slave already has. `72650ce0d`
  ("slave: skip the transactions a restart finds already applied") added `ha_applied_tx_marker`, one
  row a transaction committed in the same slave transaction as its data, and a restart skips what
  has one; its message names the failure without it: an INSERT of key 1 and an UPDATE of key 1 to 2,
  applied again, bring key 1 back. `502685e64`, `9343b73f7` and `b984637bb` changed how the markers
  are deleted and read. Nothing in `testkit ha-apply` restarted an applier. This is the run that does,
  and its completion condition: missed on `19e83a5d9`, the parent of `72650ce0d`, and held on
  `b984637bb`, #8052's head on 2026-11-01.
- **Command:** two shell prototypes first, then `testkit ha-apply restart`, built from this change's
  tree but for the VCS stamp. `ha_apply_worker_count=4`.
- **Pairs:** RelWithDebInfo, each with a client node, on one host.

| pair | #8052 | how it was stood up |
|---|---|---|
| `hap11013755` | `19e83a5d9` (`11.5.0.2932-19e83a5`) | by `stress --build --keep` |
| `hap11010a6b` | `b984637bb` (`11.5.0.2989-b984637`) | kept from [ha-apply-and-the-binary-nul](ha-apply-and-the-binary-nul.md); its nodes had `ignore_trailing_space=yes`, which no table here reads |

---

## 1. Killing the applier

On the slave, `kill -9` of `cub_admin applylogdb` and nothing else: heartbeat started a new one
within 5 seconds, and the applier log said `log applier started` each time. During a run the slave
can be read. By the comment on `LA_APPLIED_TX_MARKER_NAME` in `log_applier.c`, the markers at or
below the stored `committed_lsa` are deleted when it is stored, so markers just before a kill say
the applier that comes up has commits to skip. The prototype counted them all; the command counts
only those above the `committed_lsa` in `db_ha_apply_info`, since a delete that failed would leave
some below it. An idle `b984637bb` slave had none (read once, not saved). `19e83a5d9` has no such
table.

## 2. The prototypes

[`parallel-apply-restart-kill.sh`](parallel-apply-restart-kill.sh) kills the applier every 5 seconds
while the clients run and reads the markers first. It ran beside `stress --ops 0,1,2,3,4,9 --seed 1`
and beside [`parallel-apply-restart-workload.sh`](parallel-apply-restart-workload.sh): 8 clients,
each inserting a key of its own (client × 10^7 + i) and committing, then moving it by 5 × 10^6 and
committing, `ITERS` times. Each client's keys are its own, so the workers apply the clients in
parallel.

| pair | workload | kills (with markers) | result |
|---|---|---|---|
| `19e83a5d9` | `stress`, no kill | 0 | same |
| `19e83a5d9` | `stress` | 9 (no table) | same, `fail_counter` unmoved |
| `b984637bb` | `stress` | 10 (10, 258 markers in all) | same, `fail_counter` unmoved |
| `19e83a5d9` | 8 × 200, no kill | 0 | same |
| `19e83a5d9` | 8 × 3,000 | 31 (no table) | stalled at 300 s with 14,532 rows; read later: **slave 111 rows over the master**, 111 keys not moved, `fail_counter` +111 |
| `b984637bb` | 8 × 3,000 | 39 (39, 74,703 in all) | same: 24,000 rows on both, no key unmoved, `fail_counter` unmoved, no -670 |

`stress` with the kills was the same even on `19e83a5d9`. A reading, not a measurement: its 20 keys
take an INSERT and a key change in random order, and a pair of them in the window before the apply
info is stored, with nothing after it that hides the difference, is rare. The workload of the second
prototype makes that pair every two transactions.

On `19e83a5d9` the slave had not shown the end marker 300 seconds after the clients ended, with
14,532 rows; the script's line says so. Read on the slave later, by hand and not saved: it held
24,111 rows to the master's 24,000, 111 of them keys still where they were inserted, its
`fail_counter` was 111, and its applier log had -670 on 111 lines, as
`failed to apply update replication log ... key: "30000247", server error: -670`: the move of a key
that the re-applied INSERT had brought back met the moved key. That is the failure the message of
`72650ce0d` describes.

The kill runs beside `stress` and this 8 × 3,000 run counted -670 only as `CODE = -670`, which an
applier's update failure does not carry, so their lines say `dup670` 0; the copies here count it
anywhere. Before the `b984637bb` 8 × 3,000 run, the whole of `b984637bb`'s applier log had no -670
anywhere (the `dup670_before=0` in the head of that run's kill log, which counts it anywhere), and
the `stress` runs' `fail_counter` did not move.

The first try of the second prototype, with no kill, read 332 keys unmoved on `19e83a5d9`'s slave:
its end marker was an INSERT into the workload's table, which shares no key with the clients' rows
and was applied ahead of them. With a table of its own as the marker, as the command's barrier
makes it, the same run read the same on both.

## 3. The command

`testkit ha-apply restart` is the second prototype and the kill loop in Go, with the verdict of
ADR-EXT-012 3-3: missed for a failing applier (`fail_counter` moved, or the slave's applier log
gained a -670 during the run) or a caught-up slave whose table differs (rows, the sum of the keys,
the keys not moved); inconclusive for a slave behind without a failure, no kill, or no kill that
found markers above the stored position; held otherwise. A kill counts only when `pkill` found an
applier; a kill that could not be made is said and the loop goes on.

```
# cluster=hap11010a6b build=11.5.0.2989-b984637 ha_apply_worker_count=4 ha_writeset_history_size=unset clients=8 iters=3000 kill_every=5s timeout=30m0s
master_errors	clients	kills	kills_backlog	markers_peak	restarts	master_rows	slave_rows	key_sum	unmoved_master	unmoved_slave	fail_counter_delta	apply_670	slave	verdict
0	done	43	43	2060	43	24000	24000	same	0	0	0	0	caught	held
```

```
# cluster=hap11013755 build=11.5.0.2932-19e83a5 ha_apply_worker_count=4 ha_writeset_history_size=unset clients=8 iters=3000 kill_every=5s timeout=30m0s
master_errors	clients	kills	kills_backlog	markers_peak	restarts	master_rows	slave_rows	key_sum	unmoved_master	unmoved_slave	fail_counter_delta	apply_670	slave	verdict
0	done	43	-	0	43	24000	10960	differ	0	2097	80	80	stalled	missed
```

These kills ran on a ticker, every 5 seconds start to start. From
[ha-apply-restart-and-a-crashed-slave](ha-apply-restart-and-a-crashed-slave.md) on, the command waits
`--kill-every` from the end of one kill to the next, so the same run makes somewhat fewer kills.

The two ran at the same time. On `b984637bb` all 43 kills found markers above the stored position,
up to 2,060, the applier started 43 times, and the slave held what the master held. On `19e83a5d9`
the applier failed 80 times with 80 -670s, and the slave had not caught up when the barrier gave
up; the verdict comes from the failures, since a slave behind has keys not yet moved for that alone.

An earlier build of the command, before the review that brought the count above the stored
position, the bounded marker read and a kill loop that goes on past a failed kill, ran the same
way once: held on `b984637bb` (44 kills, all with markers, up to 2,061) and missed on `19e83a5d9`
(97 failures, 97 -670s, stalled). After it, once a table made on the master as a marker had reached
`19e83a5d9`'s slave, the slave held 24,097 rows, 97 keys not moved, a `fail_counter` of 208 (the
prototype's 111 and those 97) and 208 lines with -670; that read is kept with the ledgers as
`restart-after-reads.txt`.

## 4. Not here

- **`b984637bb` itself**: a statistics statement chosen as a deadlock victim, which aborted the
  apply info store while the markers were deleted. Nothing here makes one.
- **`502685e64` and `9343b73f7` on their own**: no head between `72650ce0d` and them was run.
- **A crash of the slave's server** (`csb node kill`), which also restarts the applier but after a
  server recovery: [ha-apply-restart-and-a-crashed-slave](ha-apply-restart-and-a-crashed-slave.md)
  (`--kill node`, 2026-11-02).
- **Statements**: the marker covers a DDL or a DML replicated as a statement too; the workload has
  only rows.

## 5. Reproducing it

```bash
export CSB_HOME=/data/...  TESTKIT_CSB=.../extensions/cluster-sandbox/bin/csb
testkit ha-apply restart --build <an install of #8052 19e83a5d9>    # missed
testkit ha-apply restart --build <an install of #8052 b984637bb>    # held
```

What ran was `--cluster` on the two kept pairs.
