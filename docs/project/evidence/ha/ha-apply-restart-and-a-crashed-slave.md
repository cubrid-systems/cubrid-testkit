# `testkit ha-apply restart --kill node`: a slave node crashed under load applies commits twice on `19e83a5d9`, not on `b984637bb`

- **Date:** 2026-11-02
- **What this is:** the server crash that [ha-apply-restart-and-the-applied-tx-marker](ha-apply-restart-and-the-applied-tx-marker.md)
  left out. There the applier alone was killed and heartbeat started it again. Here the whole slave
  node is crashed and started again, so what the restarted applier finds is what the server's
  restart recovery kept: the stored apply info and, from `72650ce0d` on, the `ha_applied_tx_marker`
  rows, each written by a committed slave transaction. The question is the same: does the applier
  apply a committed transaction a second time? The completion condition is the same too: missed on
  `19e83a5d9`, the parent of `72650ce0d`, and held on `b984637bb`, #8052's head on 2026-11-01.
- **Command:** a shell prototype first, then `testkit ha-apply restart --kill node`, built from this
  change's tree but for the VCS stamp and the wording of the error a slave that does not start again
  gives, edited after the runs. `ha_apply_worker_count=4`.
- **Pairs:** RelWithDebInfo, each with a client node, on one host.

| pair | #8052 | how it was stood up |
|---|---|---|
| `hap1102bc41` | `19e83a5d9` (`11.5.0.2932-19e83a5`) | by `restart --build --keep` (§3) |
| `hap11010a6b` | `b984637bb` (`11.5.0.2989-b984637`) | kept from [ha-apply-restart-and-the-applied-tx-marker](ha-apply-restart-and-the-applied-tx-marker.md); its nodes had `ignore_trailing_space=yes`, which no table here reads |

---

## 1. Crashing the slave node

`csb node kill` on the slave runs `pkill -9` on `cub_admin`, `cub_server` and `cub_master` inside
the node's container, matched by process name; the server does not flush. Each csb node has a PID
namespace of its own (`docker inspect` gives an empty `PidMode`, and `/proc/1/ns/pid` reads
4026533368, 4026533511 and 4026533746 in `hap11010a6b`'s n1, n2 and c1), so the crash touches that
node only. `csb node start` stops the service, starts heartbeat, waits up to 45 seconds for the
node to join, and recopies a replication log the crash left short.

In the prototype runs below, a node was down 14 to 17 seconds a crash, from the kill to an applier
process running again (`pgrep`). In the command's runs the applier log said `log applier started`
once for every crash.

A crashed slave was slow to catch up, though these runs do not show why:

- `b984637bb`, 8 crashes under 8 clients × 6,000 keys: 6.5 to 7 minutes after the clients ended,
  where the prototype's barrier of 300 seconds gave up first. The applier-kill runs, which caught up
  within 5 minutes, had 3,000 keys a client, and no 6,000-key run without crashes was made.
- `19e83a5d9`, the same run: a table made on the master as a marker, about 5 to 6 minutes after
  the clients ended, reached the slave 836 seconds later by the wall clock of the loop that waited
  (the loop is not kept), about 19 to 19.5 minutes in all. That wait overlapped the earlier build's `b984637bb` command run
  (§3) on the same host, and this slave, which fails as it applies, was slow under applier kills too.

A slave that comes back with its buffers empty, behind by every second it was down, would look like
this; that is a guess. The command waits for its final marker up to 20 minutes under `--kill node`,
not 5. Both command runs, at 3,000 keys, caught up within it. At 6,000 keys `19e83a5d9`'s slave
needed about 19 to 19.5 of the 20 minutes; there its failures decide the verdict either way.

## 2. The prototype

`parallel-apply-restart-crash.sh` (kept here) crashes the slave node and starts it again, waits 10
seconds once it is back, and does it again, while `parallel-apply-restart-workload.sh` runs: 8
clients, each inserting 6,000 keys of its own and moving each one in the next transaction. Before
each crash it reads, on the slave, the markers above the stored `committed_lsa` ("-" where there is
no marker table).

| #8052 | crashes (with markers above the stored position) | when the workload's barrier gave up, 300 s after the clients | once caught up |
|---|---|---|---|
| `19e83a5d9` | 8 (no table) | slave 18,204 of 48,000 rows; `fail_counter` +39, -670 +39 | slave 48,039 rows to the master's 48,000, 39 keys not moved; `fail_counter` 186, 186 lines with -670 |
| `b984637bb` | 8 (8: 704, 2,045, 2,107, 2,036, 2,075, 2,065, 2,085, 2,079) | slave 43,525 of 48,000 rows, 2,030 keys not yet moved; no failure | both 48,000 rows, the same key sum, no key unmoved; `fail_counter` 0, no -670 |

On `19e83a5d9` the 39 keys back on the slave are inserts applied a second time, whose moves then
met the moved keys (-670). The pair had a `fail_counter` of 147 and 147 lines with -670 from §3's
applier-kill run before; the 39 are this run's. On `b984637bb` the slave was 4,475 rows behind when
the workload's barrier gave up; read by hand as it caught up, it held what the master held.

Files, beside the ledgers in the working directory: `crash-19e8-p1.log`, `crash-b984-p1.log` (the
crash lines), `crash-19e8-p1.out`, `crash-b984-p1.out` (the workload lines),
`crash-19e8-after-reads.txt` (the reads once the marker arrived, written by the script that waited)
and `crash-b984-after-reads.txt` (the hand reads, written into the file afterwards from the
terminal).

## 3. The command

`testkit ha-apply restart --kill node` is the same run as `restart` with the kill replaced: before
each kill the marker read, then `csb node kill` and `csb node start` on the slave, then
`--kill-every` until the next. A crash counts when `csb node kill` succeeds. The start follows even
a crash csb said failed, which may have reached the node, and neither takes the run's cancel; csb
runs in a process group of its own, so a terminal's ^C does not cut the start off. A node that does
not start again ends the kills, and the run fails, once the clients end, with an error that names
`csb node start` for it. The verdict is
ADR-EXT-012 3-3's, unchanged. Defaults: 8 clients × 3,000 keys, `--kill-every 5s`.

```
# cluster=hap11010a6b build=11.5.0.2989-b984637 ha_apply_worker_count=4 ha_writeset_history_size=unset clients=8 iters=3000 kill=node kill_every=5s timeout=30m0s
master_errors	clients	kills	kills_backlog	markers_peak	restarts	master_rows	slave_rows	key_sum	unmoved_master	unmoved_slave	fail_counter_delta	apply_670	slave	verdict
0	done	5	5	2086	5	24000	24000	same	0	0	0	0	caught	held
```

```
# cluster=hap1102bc41 build=11.5.0.2932-19e83a5 ha_apply_worker_count=4 ha_writeset_history_size=unset clients=8 iters=3000 kill=node kill_every=5s timeout=30m0s
master_errors	clients	kills	kills_backlog	markers_peak	restarts	master_rows	slave_rows	key_sum	unmoved_master	unmoved_slave	fail_counter_delta	apply_670	slave	verdict
0	done	5	-	0	5	24000	24025	differ	0	25	25	25	caught	missed
```

The two ran one after the other, `b984637bb` first (its ledger written at 04:15, `19e83a5d9`'s at
04:25). On `b984637bb` all 5 crashes found markers above the stored position, up to 2,086, and the
slave, caught up within the 20 minutes, held what the master held. On `19e83a5d9` the slave caught
up too, with 25 rows the master does not have: 25 keys back, and 25 failures with 25 -670s, their
moves.

An earlier build of the command, before the review that made a crash and its start run to their end
whatever stops the run, ran the same way once (`haapply-cr1-*.tsv`): held on `b984637bb` (6 crashes,
all with markers, up to 2,076) and missed on `19e83a5d9` (5 crashes; caught up with 16 rows over, 16
failures and 16 -670s). The `19e83a5d9` prototype wait of §1 overlapped that `b984637bb` run.

`hap1102bc41` was stood up by `restart --build --keep` with the applier kill, built from the commit
before this change (`d2ef449`). That run was missed as before: 33 kills, 147 failures and 147 -670s,
stalled. Once a table made on the master as a marker reached the slave, 227 seconds later, the slave
held 24,147 rows to the master's 24,000, 147 keys not moved (`haapply-rs3-19e8.tsv`,
`rs3-19e8-after-reads.txt`).

## 4. Not here

- **What `b984637bb` fixed**: a statistics statement chosen as a deadlock victim, which aborted the
  apply info store while the markers were deleted. Nothing here makes one, as in the applier-kill
  document; nor were `502685e64` and `9343b73f7` run on their own.
- **A crash during the server's recovery**, or one that loses the copied log. csb recopies a log
  left short; a node that has not joined within its 45 seconds is crashed again `--kill-every`
  later, and these runs did not check whether any was.
- **The master**: a crash of the master is a failover, which is `ha_repl`'s and csb's `ha` verbs'.
- **Statements**, as in the applier-kill document.

## 5. Reproducing it

```bash
export CSB_HOME=/data/...  TESTKIT_CSB=.../extensions/cluster-sandbox/bin/csb
testkit ha-apply restart --build <an install of #8052 19e83a5d9> --kill node    # missed
testkit ha-apply restart --build <an install of #8052 b984637bb> --kill node    # held
```

What ran was `--cluster` on the two kept pairs.
