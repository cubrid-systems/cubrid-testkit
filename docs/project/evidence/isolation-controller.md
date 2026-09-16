# The isolation controller, measured before it is replaced

- **Date:** 2026-09-16
- **What this is:** what `qactl` is made of, how much of it runs, and whether the two pieces a Go controller
  would need can be built at all. It is the evidence ADR-019 rests on. Everything here is measured on this
  machine or read from source.
- **Trees:** engine `cubrid/cubrid` `f1ae86ff7` · cases `cubrid/cubrid-testcases` `6ab786aa9` · CTP
  `cubrid/cubrid-testtools` `a1bec87`, in the `regr-iso` sandbox (`isolation-baseline.md` §1).
- **Status:** §1–§8 done. The gate (§7) has been run and does not pass.

---

## 1. How much of `qactl` runs

`tk-cov/CTP` is a copy of the sandbox's CTP tree whose `ctltool/Makefile` adds `--coverage` to `CFLAGS` and to
the link. `run-cov.sh` runs `runone.sh` over the sample directly — CTP's Java is not involved, because
`qactl`'s coverage does not depend on who calls `runone.sh` — and `gcov-10` reads the `.gcda` it leaves.

The loop that drives it **runs as PID 1 of its namespace**, and that is not decoration: `clean.sh` ends every
case with `cubrid tranlist | xargs kill -9` and `runone.sh` with `pkill -9 sleep`, so a driver that is an
ordinary child gets killed with everything else. The first attempt died after one case. A SIGKILL sent from
inside a PID namespace to its own PID 1 is not delivered, so the loop survives.

| 60 cases, 58 OK | executable lines | executed | |
|---|---:|---:|---|
| `qactl.c` | 1,468 | **38.35%** | about 563 lines, of 4,627 physical |
| `qacsql.c` | 213 | 30.99% | |
| `parse.c` | 120 | 85.83% | |
| `qamccom.c` | 250 | **8.80%** | the super-controller socket layer, which nothing turns on |
| `common.c` | 112 | 59.82% | |
| `cubrid_drv.c` | 144 | 61.81% | |

The sample is six topics of the corpus plus a deadlock case; the whole corpus would add the `unblocked` and
`login as` paths and little else, because the language it uses is eight commands (§2).

## 2. What the corpus asks for

Over all **6,790 `.ctl` files** in `cubrid-testcases/isolation` — the count of files, where 6,772 is the count
of cases a run judges after the exclusion list:

| command | files | occurrences |
|---|---:|---:|
| `MC: setup NUM_CLIENTS = n;` | 6,790 | 6,790 |
| `MC: wait until Cn ready;` | 6,786 | 36,754 |
| `MC: wait until Cn blocked;` | 2,480 | 3,236 |
| `MC: wait until Cn unblocked;` | 150 | 153 |
| `MC: sleep n;` (seconds) | 804 | 921 |
| `MC: pause for deadlock resolution;` | 23 | 24 |
| a statement for a client (`Cn:` or, having lost its prefix, client 1) | 6,790 | 155,968 |

Counted by `TestCorpusVocabulary`, which classifies what the reader returns — so a command inside a comment
is not counted, and `grep` over the same corpus reports a little more.

**Used by no case at all:** `wait until Cn finished`, `wait for n`, `reconnect`, `rendezvous with super`,
`execute` in all three forms (with `exec_stressgen` and `exec_stressexec`), `allocate client`, `no-op`,
`client_names =`, and the client's own `save state by`, `verify state unchanged|changed`, `simulate`, `schema`
and `print`.

The full grammar, with the two places where what the corpus writes and what the parser does disagree, is
`analysis/isolation/ctl-grammar.md`.

## 3. The two pieces a Go controller needs

### The reader is a port, and it is the same port

`internal/ctl` is `parse.c`'s statement splitter in Go: the same twelve states, ten character classes and two
tables — printed out of the C source by a program that includes `parse.c`, rather than copied by eye — and the
same 1,024-byte line buffer, so that `parse_line_num` counts what it counted.

`TestAgainstParseC` builds ctltool's parser, runs both over every `.ctl` file of a corpus and requires the same
statements from both:

```
CTP_HOME=… TESTKIT_CTL_CORPUS=… go test ./internal/ctl/
    corpus_test.go:83: 6790 files, every statement identical
```

It found one difference in the port on the way: a line holding two statements lost its second one, because the
port had thrown away the rest of the line when it returned the first. 2,256 files write a statement that way.

### The lock question can be asked from another process

`MC: wait until C2 blocked;` is answered by `tran_is_blocked (tran_index)`, which `libcubridcs` exports and no
header declares. Reading the engine settles what it is: a client stub that sends `NET_SERVER_TM_ISBLOCKED`
(`network_interface_cl.c:3112`), answered on the server by `lock_is_waiting_transaction (tran_index)`
(`transaction_sr.c:576` → `lock_manager.c:7819`), which walks the threads waiting on locks for that
transaction. **It is a question about a transaction, not about the caller**, so any connected client may ask it
about any other.

`internal/ctl/native/qablocked.c` is that and nothing else: connect, then a transaction index per line in and a
`1` or a `0` per line out — and, added later, a `dump` that answers with the lock table (§4). 98 lines, built
against the engine under test.

`probe-blocked.sh` drives two `qacsql` clients by hand through
`_05_ReadCommitted_RepeatableRead/index_column/composite_index/basic_sql/insert_insert_01.ctl`'s own
statements — a case whose `MC: wait until C2 blocked;` passes under CTP, so C2 does block there — and asks the
probe at each step:

| | C1 | C2 | |
|---|---|---|---|
| after the setup | 0 | 0 | |
| C2 inserts C1's uncommitted key | 0 | **1** | `cubrid tranlist` in the same second: `3(ACTIVE) … qacsql … Wait for lock holder 1` |
| after C1 rolls back | 0 | 0 | |

Twice in a row, with the transaction indexes falling differently each time.

**Two false negatives came from the harness, not the probe**, and are recorded because both are easy to write
again: a reader that took the last line of the probe's output returned the previous answer until a new one
happened to differ, and a reader that counted its own questions counted them inside `$( )`, which is a
subshell, so the count never reached the caller.

### What the sandbox taught about namespaces

A killed namespace leaves `/tmp/CUBRID<port>` behind. The next run's `cubrid server start` then finds a socket
it cannot connect to, does not start a master, and fails with *Cannot make connection to master server …
Connection refused* — while the database itself is fine. One run of this left `ctldb` unmountable and it was
recreated. Anything that starts a server in a fresh namespace should remove that file first.

### What the controller is, and what it is not

`internal/ctl` is the controller: the reader above, a classifier for the eight commands the corpus uses, a
client that is three pipes around an unchanged `qacsql`, and the statement loop. It is deliberately shaped like
`qactl.c` — the same 8,192-byte reads, the same `"| "` in front of every line of a chunk, the same order of
writing a statement and then printing it — because the answers were written against that shape. What it does
not have is `qamccom.c`, the stress subsystem, the MySQL and Oracle paths, and the thirteen commands of §2.

It reaches `runone.sh` without `runone.sh` knowing: the slot's `ctltool` is an overlay, and `Install` rewrites
the `Makefile`'s `qactl` rule there so that `make clean qactl qacsql` — which is what `prepare.sh` runs —
produces a shim to testkit and builds the probe, and still builds `qacsql` from ctltool's own source. The probe
is built when the slot opens as well, because `prepare.sh` runs only when it finds no `qactl` or no server.
`TESTKIT_ISOLATION_CTL=1` asks for all of this; without it the run is ADR-007's, unchanged.

## 4. Against ctltool's controller, case by case

`compare-ctl.sh` runs each case through `runone.sh` twice over one database — once with ctltool's `qactl`, once
with testkit's controller, with the same `qacsql` on both sides — and compares the normalized
`result/<name>.log` that the second half of `runone.sh` produces. It is the same question ADR-018 asks, at the
size of one case, and it is what a whole-corpus comparison would be made of.

**The sample, 60 cases** (`runs/ctl-sample-3`, 2026-09-16). Every verdict agrees — the same 58 pass and the
same two fail under both controllers — and **58 of the 60 normalized results are byte-identical**.

| a case, end to end through `runone.sh` | ctltool's `qactl` | testkit's controller | |
|---|---:|---:|---|
| all 55 that were timed | 274.0 s | 237.6 s | −13.3% |
| the 53 that do not wait out a timeout | 60.2 s | 36.7 s | **−38.9%** |
| the median one | 617 ms | **249 ms** | |
| `createindex_02`, the slowest | 10,295 ms | 9,845 ms | the case's own work, not the controller's |

These are whole cases: `runone.sh`'s own steps — the core check, `clean.sh`'s seventeen `csql` round trips
(115 ms), the fifteen `sed`s — are on both sides of every row and do not move. What moves is the controller,
and for a short case the controller was most of it: 100 ms before connecting, 100 ms for each client at the
end, 21 ms between client starts, and a 10 ms poll rounding up every wait.

**The two that differ are the two that fail**, and they fail the same way under both: a
`MC: wait until Cn blocked;` on a client the case itself expects not to block
(`select_delete_incrdecr_02` says so in a comment), which times out. **What differed was what qactl prints when
a wait fails**: `lock_dump (stdout)`, the server's lock table, straight into the result (`qactl.c:2271`). The
controller did not print it — 109 lines in one case and 59 in the other.

That is what a comparison is for. The probe answers a `dump` as well now, because `lock_dump` is the same kind
of symbol as `tran_is_blocked` — `libcubridcs` exports it and no header declares it — and it reports its
connection to the server as `qactl`, so that the dump names the controller's transaction the way it always did.

**And the dump found a bug in ctltool.** The first attempt printed 43 lines where qactl printed 126. The
engine's function is `lock_dump (FILE *outfp, int is_contention)`; ctltool declares it with one argument
(`cubrid_drv.c:49`) and calls it with one (`qactl.c:2271`), so the second comes from whatever the register
held, and it decides whether the whole lock table is printed or only what something is waiting on. Declared
properly and called with 0, the dump is the one qactl usually prints
(`spec-corrections.md` §9).

**What a failing case can be held to, once it prints a lock table.** The dump carries session ids, MVCCIDs and
OID slot numbers, so its bytes are not reproducible by anything. Measured on `select_delete_incrdecr_02`:

| | lines differing | of them, not a session id |
|---|---:|---:|
| CTP against CTP, the same case run twice | 36 | 28 |
| testkit against CTP | **36** | **28** |

The same lines, of the same kinds. The controller's dump now matches CTP's as closely as CTP's matches itself,
which for these two cases is the only standard there is — and ADR-018 judges them by their verdict, which
agrees.

**What the raw result does differ in, on every case.** `result/<name>.result`, before the normalization, has the
same blocks in a different order: a controller that does not sleep sends the next statement sooner. On
`invisible_index_01` that is 34 lines of the raw file, and none of the normalized one. The normalization
deletes every line the order can be read from -- the `C%d output` headers, the `is ready` lines, the `MC to C%d`
echoes, the statement echoes and the blank lines between them -- and what is left is the query results, in the
order the case's own `wait until` commands put them in.

**The line numbers match.** `QACTL %d line: %d statement: (%s)` is CTP's, including the number, on every case
checked -- so the port's `fgets` counting reproduces `parse_line_num` through the file's second pass.

## 5. The one thing that looked like a bug to fix, and was not

`parse.c` splits on the naked semicolon without looking at the `Cn:` prefix, so the second statement of
`C2: insert a; insert b;` arrives without one and qactl sends it to **client 1** — a client the line did not
name. It reads like a bug, and the first version of this controller fixed it: a statement with no prefix goes
to the client that opened its line.

**Who is affected.** Counted with a parser that reports the line each statement came from (`dumpline`), over
6,790 files:

| | files |
|---|---:|
| a prefix-less statement sharing a line with an earlier one | 2,256 |
| …where the line belongs to a client other than C1 | **5** (32 statements) |

`_04_…/dml_ddl/createtable_03.ctl` · `_04_…/function_index/basic_sql/insert_insert_03.ctl` ·
`_04_…/multi_index/basic_sql/insert_delete_02.ctl` · `_05_ReadCommitted_RepeatableRead/dml_ddl/createtable_03.ctl` ·
`_06_features/cbrd_22705_online_index_parallel/…/insert_delete_02.ctl`.

A prefix-less statement on a line of its own is a different thing: the author using the default. 2,256 files do
that, and they mean client 1 — `bug_bts_14165.ctl:32-35` writes a four-line preparation block with no prefix and
the line after it is `MC: wait until C1 ready;`. Counting without the same-line condition first reported 8
files; three of them were this.

**What the fix did.** All five, through `runone.sh`, against ctltool's own controller on the same database:

| file | CTP | with the line's client |
|---|---|---|
| `_04_…/dml_ddl/createtable_03` | OK | **OK** — no difference at all |
| `_04_…/function_index/basic_sql/insert_insert_03` | OK, 666 ms | **OK**, 256 ms — no difference |
| `_05_ReadCommitted_RepeatableRead/dml_ddl/createtable_03` | OK | **NOK**, one line: `on statement number: 13` became `14` |
| `_04_…/multi_index/basic_sql/insert_delete_02` | OK, 551 ms | **NOK**, 300,328 ms |
| `_06_features/…/insert_delete_02` | OK | **NOK**, the same |

Two are untouched. One fails on a single line of its answer — C2 runs one statement more before the error, so
the error's ordinal moves — and an answer file could be corrected for it. The other two cannot be corrected
that way. The 300 s is `longDuration`: the controller waiting for a client to be ready so that it can send it
the next statement. The case is

```
C2: insert into t values(2,1);insert into t values(2,2);insert into t values(2,5);
MC: wait until C2 blocked;
```

and the first insert is the one that blocks — that is what the case is *for*. Sent all three, C2 blocks on the
first and never takes the second, and the run ends with the client not responding. **The case works because of
the misrouting.**

**So the routing is kept.** The rule this project applies to CTP's behaviour is `spec-corrections.md`'s:
*change it when leaving it alone would make someone trust a wrong result.* These cases end by checking the rows
in the table, and the rows are there either way; what the misrouting changes is which client inserted them,
which no case asserts. It is a trap for whoever writes the next case, and that belongs in the corpus — the five
files should name their client on every statement, and the two that block should be restructured so that the
blocking statement is last.

## 6. How often to ask the server whether a client is blocked

`MC: wait until Cn blocked;` is the one wait whose answer is not on a pipe, so it is a poll, and the interval is
a number someone has to choose. qactl polls every 10 ms and rounds every such wait up by that much. The first
version of this controller asked every millisecond for the first hundred and every ten after that, on the
argument that a block forms early and a wait that will time out should not cost a hundred thousand questions.
That argument was not measured, so it was.

Two builds, differing only in the interval, over the sample's **26 cases that use `wait until … blocked`**, each
run against ctltool's controller on the same database:

| | 1 ms, then 10 ms | 10 ms flat |
|---|---:|---:|
| the 24 that do not wait out a timeout | 27.23 s | 27.73 s (+1.84%) |
| the median one | 253 ms | 266 ms |
| all 26 | 228.15 s | 228.65 s (+0.22%) |
| verdicts against CTP | 24 same, 2 differ | 24 same, 2 differ |
| results identical between the two builds | — | **25 of 26** |

The one result that is not identical between the builds is `select_delete_incrdecr_02`, whose failure prints the
lock table — session ids and MVCCIDs, which differ between any two runs of anything (§4).

**The interval does not matter.** Individual cases move 500 ms in both directions between the two builds —
`createindex_02` is 369 ms *faster* at 10 ms — which is more than the interval can explain and is what run to
run costs. So the controller polls at qactl's 10 ms: the same number, one fewer invented, and a 100-second wait
is ten thousand questions rather than a hundred thousand.

## 7. The gate, run

ADR-018's rules over the whole corpus, four slots, against the two CTP runs already recorded
(`isolation-baseline.md` §4). **It does not pass.**

| | tk-full-p4 (ctltool's controller) | ctl-full-p4 (testkit's) |
|---|---:|---:|
| wall | 2,977 s | **2,509 s (−15.7%)** |
| case seconds | 11,894 | 9,823 (−17.4%) |
| `qactl` seconds | 10,635 | 8,614 (−19.0%) |
| an attempt with 2 clients, median | 454 ms | **116 ms** |
| with 3 | 569 ms | 117 ms |
| with 4 | 742 ms | 104 ms |
| NOK | 14 | **40** |

**Rule 1** holds. **Rule 4**: 6,538 of the normalized results are byte-identical to `full-2`'s, 48 differ, 186
are not in both. **Rule 2** finds 40 failures: six that fail under CTP too, six already known to flip, and 28
that had never failed in any of the four earlier whole-corpus runs. **Rule 3**, each of the 28 alone and three
times under each controller on a fresh database: ctltool's controller passes all three every time; testkit's
fails all three in **25** of them, passes all three in two, and is unstable in one. Twenty-five runner
differences.

**What they are.** Not the database, and not a defect in the controller's own work: each of the 25 is a case
that leaves two clients' statements unordered and whose answer records the order qactl's pauses produced. The
worked examples, the full list and the fixes are `evidence/isolation-corpus-races.md`. Under the rule as
written they are runner differences, and they are: this runner is faster, and the cases can tell.

**Where the wall time went, and did not.** wall is case seconds ÷ 4 on both runs — the slots are saturated, so
wall follows case time exactly. Case time fell 17% and not 60% because the corpus's seconds are not in the
median attempt:

| an attempt taking | tk-full-p4 | ctl-full-p4 |
|---|---|---|
| under 100 ms | none | 3,393 attempts, 124 s |
| 0.1 – 2 s | 5,960 attempts, 3,488 s | 2,646 attempts, 775 s |
| 2 – 10 s | 653, 2,699 s | 665, 2,626 s |
| over 10 s | 231, **4,449 s (41.8%)** | 245, **5,089 s (59.1%)** |

The fast band collapsed by 74%, and that is the whole saving. The 910 attempts over two seconds are the case's
own `MC: sleep`, its lock waits and its timeouts — nothing a controller can shorten — and they now carry 89.6%
of the time. The tail also grew: the 40 failures spend 1,539 s of it against the 84 s the 14 earlier failures
spent, because a failing case uses all five attempts and some of them wait a hundred seconds first.

## 8. More slots, and a volatile layer

The same controller over the whole corpus at four, eight and fourteen slots, and at eight with the slots'
overlays mounted volatile (`TESTKIT_SLOT_VOLATILE=1`). The disk is the slow one: 5–8 ms for a synchronous 4 KB
write, and `/var/tmp` on this machine is no faster, so a slot root elsewhere was not tried.

| | 4 slots | 8 slots | 14 slots | 8 slots, volatile |
|---|---:|---:|---:|---:|
| wall | 2,509 s | 1,239 s | **797 s** | 1,632 s |
| case seconds | 9,823 | 9,591 | 10,473 | 11,899 |
| case seconds, without the hangs below | 9,093 | 9,101 | 9,899 | 9,025 |
| NOK | 40 | 32 | 30 | 36 |
| cases that needed a second attempt | 56 | 44 | 47 | 65 |
| peak, the run's processes' resident size | — | 11,381 MB | 17,738 MB | 11,739 MB |
| peak, the fall in available memory | — | — | 18,631 MB, **1,330 a slot** | 12,106 MB, **1,513 a slot** |
| least available | — | — | 2,033 MB | 12,430 MB |

"Without the hangs" leaves out, in all four runs, the twelve cases that took at least a minute and at least five
times their fastest time in the four.

**Volatile buys isolation nothing.** Without the hangs, eight volatile slots spent 9,025 case seconds against
9,101 — under 1%. A case's time is its sleeps, its lock waits and its round trips to the clients, not the
database's writes; the sql family's factor of two under volatile does not carry over.

**The volatile run was slower because of its hangs.** Five cases took 2,508 s, 21% of its case seconds.
`_01_ReadCommitted/catalog/db_index_04.ctl` alone held a slot for **1,501 s** of the run's 1,632: five attempts,
each ended by `timeout3.sh` at 300 s. In each, C3's `alter table tb1 drop constraint pk_tb1_id_col` fails on the
foreign key that C2's `alter table tb2 drop constraint fk_tb2_id_col`, sent just before it, was to drop — the
wait between them names C1, which has nothing outstanding — and then `MC: wait until C2 ready;` gives up after a
hundred seconds and the attempt runs out its time. It is kind 2 of `isolation-corpus-races.md`, and the cases
that hang change from run to run:

| run | cases that hung | seconds |
|---|---|---:|
| 4 slots | 4 — `insert_insert_01`, `delete_insert_12`, `delete_select_02`, `create_multiple_index_02` | 684 |
| 8 slots | 1 — `update_update_17` | 301 |
| 14 slots | 4 — `delete_insert_02`, `insert_insert_03`, `delete_select_02`, `delete_select_02_5` | 569 |
| 8 slots, volatile | 5 — `db_index_04`, `db_trig_04`, two `insert_insert_01`, `delete_insert_02` | 2,508 |

Ten of the fourteen pass on a later attempt, so they are not in the NOK column. One of the twelve cases,
`_02_RepeatableRead/index_column/common_index/aggregate/delete_select_02.ctl`, is among the 25 of §7. So is the
longest case of three of the four runs, `_01_ReadCommitted/catalog/db_index_key_04.ctl` — 502 s, and 302 s at
fourteen slots — whose failing attempts each wait a hundred seconds for `C3 blocked`.

**What the hangs were.** Every attempt of the fourteen, read from the worker logs (`test_local.log` keeps each
attempt's `elapse` and what the controller said), falls into four groups:

| what gave up | cases (run) | the attempts |
|---|---|---|
| `wait until Cn blocked` on the statement that **closes a lock cycle** | `_01…/primary_key_column/basic_sql/delete_insert_12` (4) `:49-50`, `_01…/primary_key_column/basic_sql/delete_insert_02` (14) `:46-47`, `_01…/index_column/common_index/basic_sql/delete_insert_02` (8 volatile) `:47-48`, `_05…/primary_key_column/basic_sql/insert_insert_03` (14) `:47-48` | 100 s ending in `ERROR! Client n is ready.`, then a pass in 0–3 s |
| `wait until Cn blocked` with **nothing ordering the lock** it expects | `_01…/catalog/db_trig_04` (8 volatile) `:34-41`, `_06…/normal_index/create_multiple_index_02` (4) | 100 s ending in `ERROR! Client n is ready.` — three attempts of `db_trig_04`, one of the other — then a pass |
| `wait until Cn ready` on the client that **lost a race** and stays blocked | `_01…/composite_index/basic_sql/update_update_17` (8) `:49-50`, `_01…/catalog/db_index_04` (8 volatile) `:36-39`, `_02…` and `_05…/partition_table/range/with_index/unique_with_key/insert_insert_01` (4; 8 volatile, both) `:35-40` | `WARNING! Client 2 was not ready after waiting 100 seconds.`, then `timeout3.sh` at 300 s |
| nothing — a **failing case** whose attempts are 35–37 s each | `_02…/aggregate/delete_select_02` (4, 14), `_02…/aggregate/delete_select_02_5` (14) | five NOKs, no warning |

- **A lock cycle.** One client's statement waits on the other's, then the other's waits on the first, and the
  case waits for the second one blocked — in three of the four before `MC: pause for deadlock resolution;`,
  in `delete_insert_12` with no pause. The server looks for cycles every second (`Run Deadlock interval = 1.00`
  in the lock dump). The traces fit a detector that resolved the cycle before the controller saw the client
  blocked: the client is ready, and in two of them it is ready with `Operation would have caused one or more
  unique constraint violations` on the key the other client's rolled-back delete had removed.
- **Nothing ordering the lock.** `db_trig_04`: C2's `DROP TRIGGER tt1_delete` is blocked on C1's; C1 commits,
  and `MC: wait until C1 ready;` is followed at once by C3's insert into `tt1` and `MC: wait until C3 blocked;` —
  nothing waits for C2 to have taken the lock C3 is to wait on. That is kind 2; the fix is a
  `MC: wait until C2 ready;` before line 40. `create_multiple_index_02`: every lock in the dump at the failure is held by
  one transaction (`Tran_index = 2`, among them a `SCH_M_LOCK` with count 9); which of the file's two
  `wait until C2 blocked` gave up, and why, is **not determined** from the log.
- **A lost race.** `update_update_17` sends C2's and C3's updates with no wait between them (kind 1). The trace
  fits C3 having queued first: C1's commit releases it, C2 stays blocked behind it, and `wait until C2 ready`
  never comes true.
  `db_index_04` is the kind 2 case above. The two `insert_insert_01` are different: C2 and C3 are ordered by
  their waits, both blocked on C1's key, and C1's `rollback` releases both — the case needs C2 to get the key.
  **They hang under ctltool's controller too**: `full-2` spent 300 s on two attempts of each, `tk-full-1` on one
  and three, `tk-full-p4` on one of each. No wait in the language can decide which of two released clients
  goes first — kind 4, and the same shape as the four cases of `isolation-always-failing.md` §2, where it costs
  a verdict rather than the wall clock.
- **Not a hang.** The two `delete_select_02` cases take 35–37 s an attempt when they pass as well — their
  `MC: sleep` and `sleep()` calls — and failed all five; one is among the 25 of §7 and the other is in the same
  document's later section.

**So the corpus's longest case is not 502 s.** The longest case that passed on its first attempt is 64–67 s in
every run — `_01_ReadCommitted/index_column/common_index/aggregate/max/delete_select_01_2.ctl`. What a hang
costs is set by `runone.sh`'s attempts and `timeout3.sh`'s limit — five at 300 s here, 1,500 s for one slot —
and no slot count changes it.

**Fourteen slots cost each case 9%.** Without the hangs, case seconds rose from 9,101 at eight slots to 9,899 at
fourteen, most of it outside the controller — the rest of `runone.sh`, its `sed`, `find`, `diff` and
`clean.sh`, went from 1,239 s to 1,889 s — on sixteen processors. Wall still fell to 797 s: 36% below eight slots for 75% more slots.

**What a slot costs in memory.** The fall in `MemAvailable` is 3–5% above the sum of the run's resident sizes,
which does not see what the kernel holds for those processes, and it is the quantity a slot count is divided
out of — so it is what `internal/sizing` records (ADR-020). Per slot the cost falls as slots are added, resident
size 1,422 MB a slot at eight and 1,267 at fourteen, so a budget taken from an eight-slot run is careful at
fourteen. At fourteen slots the machine kept 2,033 MB, just under the 2,048 MB `internal/sizing` reserves.

## 9. What is left

- The 25 are upstream's (`isolation-corpus-races.md`). Until they are fixed a whole-corpus run under this
  controller reports them, and `TESTKIT_ISOLATION_CTL` stays off by default.
- The cases of §8 that hang and then pass on a later attempt are not in that report — rule 3 was run on failures
  only — and cost wall time rather than verdicts. Two of the four groups are its kinds 1 and 2 and have the same
  fixes; the lock-cycle group is a race with the deadlock detector, which a controller that looks sooner loses
  less, not more; and the two `insert_insert_01` hang under ctltool's controller as well.
- ADR-018's rule 3 was written when both runners used the same executor, so any separation meant the runner.
  It now separates a runner that is faster from a corpus that depends on the old speed. Whether the rule should
  say so is ADR-018's to decide, not this document's.
