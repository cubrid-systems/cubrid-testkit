# sqlsuite, measured

- **Date:** 2026-09-11
- **What this is:** the P1 runner (`internal/runner/sqlsuite`, `design/module-sql.md`) measured against
  CTP. It is the evidence the code was written against and the reason several of its decisions were
  taken; it is **not** ADR-017's gate, which needs all three repositories at upstream develop's head
  (§5).
- **Pins:** the sandbox of `sql-baseline.md` — engine `a7a1db84b` (11.5.0.2560), cases `b94995abf`,
  CTP `a1bec87`. Every comparison below is against CTP's own runs at the same pins
  (`sql-baseline.md` §3, §4), so the pins moving upstream since does not affect them.

---

## 1. The records are CQT's, byte for byte

What CQT prints and writes around a case — the result directory's name, the progress lines, the
`.result` beside each case, the failure copies, `summary.xml`, a `summary_info` and a `summary.info`
for every directory, the JUnit report, `main.info` — is `internal/result/sql.go`, written from a
byte-level spec of CQT's source. `sql/records.sh` checks it against real runs: it rebuilds a run's
records from that run's own verdicts and times, and compares every file and every line of output.

| CTP run | cases | files compared | stdout | files |
|---|---:|---:|---|---|
| medium `1114081778` (`medium_dev`) | 975 | 23 | identical | identical |
| medium `1114064658` (`medium_dev`) | 975 | 23 | identical | identical |
| medium `1114053070` (`medium.conf`, 579 NOK) | 975 | 1,760 | identical | identical |
| sql smoke `1114043485` | 3 | 13 | identical | identical |
| sql `1114094064` | 17,459 | 2,763 | identical¹ | identical |
| sql `1114391568` | 17,459 | 2,763 | identical¹ | identical |

¹ Once one `*** XASL generation failed ***`, which CTP's server wrote into its standard output
between the two halves of a progress line (`sql-baseline.md` §6), is taken out of the capture.

Two orders in those files are Java's collections, not anyone's choice — the children in a
`summary_info` and the JUnit report's cases follow JDK 8 `Hashtable` iteration over result-directory
paths, so they change with the run's timestamp — and they are emulated. **Breaking that emulation
makes the check fail** on the JUnit report, `summary.info` and `summary_info`; the check is not
vacuous.

## 2. A serial run is CTP's

`TESTKIT_NATIVE_SQL=1 TESTKIT_CONTAIN=1 testkit medium -c medium_dev.conf`, one slot:

| | CTP | sqlsuite |
|---|---:|---:|
| OK / NOK | 975 / 0 | 975 / 0 |
| `.result` identical to CTP's | — | **975 of 975** |
| wall | 87–90 s | 110–125 s |

Run twice, before and after the review fixes (§6), with the same result. The extra wall time is the
executor's own start (a JVM and CQT's discovery) and the server stage waiting out the daemons that hold
its output pipe (`exec.Local`'s five seconds).

## 3. Parallel: what it costs, on this machine

A slot is a namespace with `$CUBRID` behind an overlay whose lower layer is the one database the setup
prepared. Two costs follow, and both are the disk's.

**Copy-up.** The first write a slot's server makes to a volume copies the whole file into the slot's
upper layer: 1.1 GB a slot for either database (512 MB data, 512 MB log), eight slots at once to one
disk before the first case. medium, four slots: 152 s from start to the first case, against 77 s
serial.

**Every commit's log flush, from every slot, to one disk.** The slots' upper layers are on
`/var/tmp`, a consumer SATA SSD here; eight servers flushing their logs to it serialise on the file
system's journal.

| medium, 4 slots | upper on disk | upper in memory | serial | CTP |
|---|---:|---:|---:|---:|
| wall | 184 s | 69 s | 110 s | 87 s |
| start to first case | 152 s | 58 s | 77 s | 56 s |
| cases, summed | 81 s | 14 s | 28 s | 22 s |

**Memory is not an answer on this machine, and it was dropped.** A slot's copy of the sql database
grows on its own — extra data volumes, archive logs, temporary volumes — to as much as 3.3 GB: eight
slots filled a 14 GB tmpfs in the middle of the run, the servers that could not extend a volume
stopped, and their executors waited on them. Sizing it safely needs a measured growth per slot and
the memory to match; the decision (2026-09-11) is to run the slots on disk and take the speed-up that
parallelism alone gives.

**The executors.** At CTP's sizing (`-Xms1024m`, no cap, a parallel collector with a thread per
core) eight executors reached 2 GB each before their first case, 16 GB between them, and the first
eight-slot sql run spent the rest of the machine's memory in swap. A slotted executor now starts at
256 MB and stops at 1 GB, with two collector threads — the largest answer in the corpus is 8.5 MB —
and eight of them take 4 GB. A serial run keeps CTP's options.

| sql, 8 slots on disk | wall | start to first case | cases, summed | OK / NOK |
|---|---:|---:|---:|---:|
| executors at CTP's sizing, before §4's fix | 1,329 s | — | 8,121 s | 16,962 / 497 |
| executors capped, §4's fix | 1,422 s | 9.5 min | 6,488 s | 17,456 / 3 |
| servers one at a time, the scan fixed | **1,131 s** | 75 s | 6,846 s | 17,457 / 2 |
| CTP, serial | 1,771 s | 78 s | 1,693 s | 17,459 / 0 |

The last run wrote 17,457 `.result` files identical to CTP's; the two it did not are §4's.

**Bringing the slots up.** The 9.5 minutes before the second run's first case had two causes. Every
executor reads every case at its start to find the server-message hints (§4), and JDK 8's
`String.toLowerCase` is quadratic on text full of characters that lower to two: one Turkish case of
5.4 MB took thirty seconds of every executor's start. The executor now matches the bytes. And eight
servers started at once, each copying its 1.1 GB up on its first write, to one disk. They now start
one at a time, and a slot takes cases as soon as its own server is up, so the first case waits for one
server, not eight.

medium does not gain from it. Its later slots came up after the queue had drained, and a slot whose
turn comes after that no longer starts; four slots went from 243 s to **157 s**, and still lose to a
serial run (110–125 s). Its cases are 22 s of CTP's run, a server's start is 32–48 s, one directory
holds 444 of the 975 cases and stays on one slot, and a start already under way when the queue drains
is waited out. medium is a serial run.

**Where the time goes, sql.** Two things, both measured on the 1,131 s run.

- *Every case is slower.* 4.04 times CTP's serial run in total: a statement takes 10.5 ms against
  2.6 ms. Per case the spread is wide — median 2.5×, p10 1.2×, p90 9.4×, over the cases of 20 ms
  or more. So eight slots do about twice a serial run's work per second.
- *The slots join late.* Servers one at a time, 37–61 s each; the eighth slot's first case came at
  436 s of 1,131. Once up they were busy: 6,846 s of cases in about 7,090 slot-seconds, 96%.

The slowdown is the disk's, as far as it has been checked. The upper layers are on `/var/tmp`, sdc
here, the SATA SSD that also holds the swap file; CTP's database is in the sandbox on `/data`, sdb. A
4 KB synchronous write (`dd oflag=dsync`, 300 of them, three times) takes **5.8 ms on sdc and 1.3 ms
on sdb**. CUBRID flushes its log on every commit and CQT runs in autocommit, so nearly every statement
that writes waits on one, and the sql corpus is mostly tables created, filled and dropped. medium
with its upper layers in memory (the table above) is the same processes on the same sixteen cores
with the writes going elsewhere, and its cases went from 81 s to 14 s. How much of the four times is
sdc being slower and how much is eight servers sharing it has not been separated.

**The disk, taken apart.** Two changes, one at a time, on the same pins and configuration. The
upper layers moved to `/data` with `TESTKIT_SLOT_ROOT`; then the overlay was mounted `volatile`, which
makes every sync on it a no-op — a layer the run throws away at the end has nothing to keep. That
second one was a build made for the measurement, and is not in the code.

| sql, 8 slots | wall | slots' start, summed | cases, summed | a statement | flushes on the slots' disk | OK / NOK |
|---|---:|---:|---:|---:|---:|---:|
| upper on `/var/tmp` (sdc) | 1,131 s | 409 s | 6,846 s | 10.5 ms | — | 17,457 / 2 |
| upper on `/data` (sdb) | **722 s** | 266 s | 4,022 s | 6.2 ms | 199,133 | 17,457 / 2 |
| on `/data`, `volatile` (run 1) | **394 s** | 233 s | 894 s | 1.4 ms | 768 | 17,454 / 5 |
| on `/data`, `volatile` (run 2) | **338 s** | 219 s | 840 s | 1.3 ms | 739 | 17,456 / 3 |
| CTP, serial | 1,771 s | — | 1,693 s | 2.6 ms | — | 17,459 / 0 |

It was the syncs. With them gone the cases on eight slots take half of what they take in CTP's own
serial run, which waits on one at every commit too, and the run is 4.5 to 5.2 times CTP's. What is
left is the slots' start, one after another: 219–233 s of a 338–394 s run, where the cases of all
eight slots fit in about 105 s.

Every NOK in these runs is §4's kind but one. `1003` fails in each; a table another case left
(`-494`) meets a different case each time — `union.sql`, `trac_343_01`, `example.sql` — and
`create_view_data_type` is the other way round, its answer recording the `-494` of an object that
the slot never had; `cbrd_26104`, in both `volatile` runs, prints trace statistics where CTP's answer
has the plan of the query that reads the current user's groups. The one that is not: in the first
`volatile` run `_08_javasp/case_join_01` got `-111` (a transaction the server aborted, "server failure
or mode change") and took 8.6 s where every other run took 1.1–1.2 s. No kernel OOM, no core, and it
did not recur; that run's slot logs were gone with its slots, and the next run's, copied out while it
ran, show no mode change or restart.

| medium | wall | cases, summed | OK / NOK | `.result` identical to CTP's |
|---|---:|---:|---:|---:|
| sqlsuite, serial, upper on `/var/tmp` | 110–125 s | 28–33 s | 975 / 0 | 975 |
| sqlsuite, serial, on `/data`, `volatile` | **77 s** | 16 s | 975 / 0 | — |
| sqlsuite, 4 slots, on `/data`, `volatile` | 91 s | 16 s | 975 / 0 | 975 |
| CTP, serial | 87–90 s | 22 s | 975 / 0 | — |

A serial medium run is now faster than CTP's; four slots still lose to one, two of them never
starting because the queue had drained.

**Memory is the next limit.** A run of eight slots on `/var/tmp` with `volatile` was stopped at 7%
of its cases, just after its last slot came up, because the machine was running low on memory; it
is not measured. A slot in medium, sampled every five seconds: its server up to 0.7–0.85 GB, the
executor 0.55–0.7 GB, five CAS processes 15 MB each, the PL server 56 MB. Eight sql slots had this
30 GB machine at 23 GB used, next to a desktop and other sessions that were not the run's.

## 4. What slots found in the corpus

A serial run's cases run one after another on one database and one connection, so a case can depend
on what any case before it left — and CTP's answers were recorded that way. Slots change which cases
come before which. The comparison of one slot with many is how those dependencies show.

**CQT's own state: the server-message flag.** `resetConnection` puts autocommit, holdcas and the
reset script back before every case, and leaves `test.serverMessage` where the last
`--+ server-message` hint put it. When it is on, CQT prints each error's message after its code. In a
slot that ran a different set of cases before one, the flag differs from CTP's — **495 of the 497
verdicts the first eight-slot sql run moved were this and nothing else**, the result differing from
the answer only in the message lines under an `Error:` code.

It is deterministic, so it is reproduced rather than tolerated: in a slotted run each executor works
out, with CQT's own parser and hint checks and in CQT's order, where the flag stands at the start of
every case, and puts it there through CQT's own code before running the case. **In the run that tested
it, none of 11,137 cases failed that way** (its later failures were §3's full tmpfs). A serial run
does not do this; its flag already is CTP's.

**The corpus's own order.** What is left is what cases leave in the database or the session for
cases in other directories:

| case | depends on | how |
|---|---|---|
| medium `_05_err_x/6360` | `_04_full/sesnsch.sql` | it runs `call login('PUBLIC','')` and never logs back, so in CTP every medium case after it runs as PUBLIC, and 6360's answer says so: its class is `public.b`, where a slot that did not run `_04_full` first makes `dba.b`. `_05_err_x/3318` and `_06_fulltests/7015` failed in the same run in the same way as far as their differences show — a class that exists or does not, a `change_owner` that is refused — and are taken to be the same until shown otherwise |
| sql `_01_object/_10_system_table/_001_db_class/1003` | every case before it | it lists the catalog, and the slot's catalog held a `test_vclass` that some other case there left |
| sql `_16_index_enhancement/_05_index_covering_plan_dump/_01_index_covering` | a case that left a table | its `create table d1` succeeds in CTP's run and fails with -494 in the slot, where an earlier case left a `d1` |
| sql `_19_apricot/_04_multi-table_update/attributes_01` | a case that left a table | the same with `t1` |
| sql `_13_issues/_26_2h/cbrd_27200` | not yet found | its `show trace` printed nothing in the slot where CTP's answer has the statistics; it failed in one eight-slot run and passed in the next |

A directory stays with the slot that claimed its first case, in order (`dispatch.Queue.Affinity`), so
nothing inside a directory is affected. Which of these a run meets depends on which slot takes what,
and is at most a handful of cases.

## 5. Not yet measured

- **ADR-017's gate**: CTP and sqlsuite, serial, over both corpora with all three repositories at
  upstream develop's head. The engine has been rebuilt there (11.5.0.2562-1642235); the sandbox has to
  be laid down again (`sandbox.sh refresh`) and CTP's baseline taken again at the same pins.
- **Slots against one slot, whole corpora**, after the gate, with the order dependencies of §4
  accounted for.
- **`volatile` on `/var/tmp`** (§3): stopped for memory; it decides whether the default slot root
  has to move once `volatile` is in the code.
- **A sql slot's memory**, sampled as medium's was, and what eight of them leave the machine.
- **The slots started together** again, now that a start no longer waits on the disk's syncs.

## 6. What a review changed

An independent review of the runner (22 findings, three of them high) led to: the executor's protocol
moving off standard output to fd 3, so a thread dump or a JVM log line cannot shift every later reply
by one case; every reflective lookup before the executor says it is ready, and anything that escapes it
an error and an exit rather than a JVM that neither answers nor ends; cores kept from a slot stored
where the next core search and the next clean do not find them; `main.info`'s user line naming the
account rather than the namespace's root; a medium data load that fails stopping the run; and the
output lock split so an executor's own output cannot wait on the records while they wait on it.
