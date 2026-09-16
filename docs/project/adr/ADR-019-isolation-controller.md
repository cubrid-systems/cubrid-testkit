# ADR-019: The controller is rewritten; the client is not

- **Date:** 2026-09-16
- **Status:** Proposed — the controller is written, and the gate has been run and **does not pass**. Over the
  whole corpus it is 15.7% faster in wall time and 40 cases fail where ctltool's controller fails 14; of the 28
  that had never failed before, ADR-018 rule 3 makes **25 runner differences**. Each of the 25 is a case that
  leaves two clients' statements unordered and whose answer records the order qactl's pauses produced
  (`evidence/isolation-corpus-races.md`). `TESTKIT_ISOLATION_CTL` stays off until they are fixed upstream
  (`evidence/isolation-controller.md` §7)
- **Related:** ADR-007 (which this amends — its "kept for later" alternative is what this decides), ADR-001
  Consequence 4, ADR-002 (`ctltool/Makefile` kept), ADR-008 and ADR-009 (reserved for the grammar and the
  normalization — the grammar half is now needed, and `analysis/isolation/ctl-grammar.md` §4·§5·§8a is it), ADR-018 (the gate this must pass),
  `design/module-isolation.md` §3, `evidence/isolation-baseline.md` §4, `evidence/isolation-controller.md`

## Context

ADR-007 decided that a case is executed by `runone.sh` and ctltool **unchanged**, and listed the alternative it
was rejecting: *"Call `qactl` directly and move the rest of `runone.sh` into Go. Rejected for the gate, kept for
later. It is the natural next step — it is where the time between cases can be won."* The gate has since been
met (ADR-018), and the measurement that followed says the time is not between cases. It is inside `qactl`.

### `qactl` is the run

Of an attempt's median 525 ms, `qactl` is 456 ms — 86.8% (`isolation-baseline.md` §4). Of that, a
one-client case's 282 ms breaks down as: 110 ms before the first client is even started, 71 ms of the client
doing its work, and 101 ms after the client has exited. The two ends are two `sleepms(100)` calls in `qactl.c`
— one before it connects (line 3034), one for each client as it quits (line 1131) — and over the corpus they
are **2,417 s, 21.8% of the case time**. The `wait until` loops add a 10 ms poll each time round: 60–121 ms a
case in the same traces. Clients are started one at a time, 21 ms apart.

A prototype that removed only the second sleep took the corpus from 2,977 s to **2,520 s (−15.4%)** with no
runner difference under ADR-018's rules. The rest — the first sleep, the polls, the serial start — is the same
size again, and none of it is the database's.

### What `qactl` actually does with the database: nothing but ask

`qactl` links `libcubridcs` and calls six of `db_drv.h`'s functions. Five are connect and disconnect. The sixth
is `local_tm_isblocked` → `tran_is_blocked`, called seven times, which is how `MC: wait until C2 blocked;` is
answered. It never executes a statement: `execute_sql_statement`, `print_class_info`, `print_ope` and
`get_tran_id` are called **zero** times in `qactl.c` — every one of them is `qacsql`'s.

`tran_is_blocked` is a client-side stub that sends `NET_SERVER_TM_ISBLOCKED` to the server, which answers with
`lock_is_waiting_transaction (tran_index)` (`transaction_sr.c:576`, `lock_manager.c:7819`). **Any connected
client may ask about any transaction.** It is exported from `libcubridcs` and declared in no header, which is
why `cubrid_drv.c:50` declares it itself.

### How the controller drives a client: three pipes and two markers

`start_process` (`qactl.c:1868`) gives each client a pipe for each standard descriptor and `execvp`s
`qacsql <db> -cl <n>`. The controller writes a statement to the client's standard input and reads its standard
output in 8,192-byte chunks. It scrapes two strings out of what comes back — `"Transaction index = "` for the
index it will ask `tran_is_blocked` about, and `") is ready."` to count statements finished — and prints the
rest. There is no socket and no message format. (`analysis/isolation/ctl-grammar.md` said Unix sockets; that
is `qamccom.c`, which belongs to a mode nothing turns on — see *What is dropped*.)

### Most of the program is not reached

Built with `--coverage` and run over the 60-case sample, `qactl.c` executes **38.35% of its 1,468 executable
lines** — about 563. `qamccom.c`, the 934-line message layer, executes 8.8%. Over the whole corpus of 6,790
`.ctl` files, the language in use is eight commands; thirteen more are implemented and used by no case
(§3).

## Decision

**The controller becomes testkit's, written in Go. The client stays CTP's, unchanged. The one thing Go cannot
ask is asked by a C program of under a hundred lines, built at run time as ctltool is.**

| testkit's | CTP's, unchanged |
|---|---|
| the `.ctl` reader (`internal/ctl`, a port of `parse.c`) | `qacsql` — it makes the bytes an answer file is made of |
| the statement loop, the client processes and their pipes, the `wait until` commands | `runone.sh`, `prepare.sh`, `clean.sh`, `timeout3.sh` |
| `internal/ctl/native/qablocked.c` — connect, then answer `tran_is_blocked` for a transaction index | the `Makefile` that builds `qacsql` (ADR-002) |
| discovery, the exclusion file, the queue, slots, the verdict, the records (already, ADR-007) | |

**The client is not touched, and that is the point.** The corpus's 6,865 answer files are what `qacsql` printed,
wrapped by the controller. Keeping the client keeps the content of every answer by construction, and leaves the
comparison to argue about the wrapping only.

### What is dropped

Each of these is implemented in `qactl.c` and used by **zero** of the 6,790 cases
(`analysis/isolation/ctl-grammar.md` §8a):

`MC: wait until Cn finished` · `MC: wait for n` · `MC: reconnect` · `MC: rendezvous with super` ·
`MC: execute …` in all three forms, with `exec_stressgen` and `exec_stressexec` behind them ·
`MC: allocate client` · `MC: no-op` · the `client_names =` setup line · and, on the client side,
`save state by` · `verify state unchanged|changed` · `simulate` · `schema` · `print`.

With them goes the whole of `qamccom.c`: the super-controller socket protocol, its FIFO queue and its token
passing. It is reached only under `-slave`, which `runone.sh` and `runall.sh` never pass, and the program that
would be on the other end of the socket does not exist in the tree. So do the MySQL and Oracle builds, which
ADR-007 already excluded.

**Dropped means refused, not ignored.** A `.ctl` file that uses one of them is an error naming the command, not
a case that quietly does something else.

### What is kept exactly, including where it is wrong

- **The output bytes.** `MC to C%d: %s\n`, `C%d output (Transaction index = %d):\n`, the `"| "` in front of
  every line of client output — including the chunked form, one header and one `"| "` run per `read()` — and
  `QACTL %d line: %d statement: (%s)\n` with `parse_line_num` counting `fgets` calls, so that a line longer
  than 1,023 bytes counts as two.
- **The default client.** A statement with no `Cn:` prefix goes to client 1. 2,256 files write one, almost
  always as a preparation block on lines of its own, and they mean client 1 — the line after such a block is
  usually `MC: wait until C1 ready;`. That is kept.
- **`set transaction isolation level` commits.** `qacsql.c:682` appends a `COMMIT` of its own. It is the
  client's, and the client is unchanged.
- **The warnings.** `WARNING! Client %d was not blocked after waiting %d seconds.` and the errors around it
  reach the result file and some answers hold them.

### What was fixed and then put back

A statement with no `Cn:` prefix goes to client 1. For the second statement of `C2: insert a; insert b;` that
is not the client the line named, and this controller fixed it — a prefix-less statement went to the client
that opened its line — before the corpus said no.

Five files write 32 such statements, and all five were run both ways. Two are untouched by the fix. One fails
on a single line of its answer, where an error's ordinal moves because its client runs one statement more.
**Two depend on the old routing and cannot be repaired by editing an answer**: the first statement of the line
is the one that blocks — which is what the case is testing — and a blocked client cannot be given the next
statement. Measured on `insert_delete_02`: 551 ms and OK as qactl routes it, **300,328 ms and NOK** with the
line's client, the 300 s being the controller waiting for a client that will not come back.

So the routing is qactl's, and `spec-corrections.md`'s rule is why: *change it when leaving it alone would make
someone trust a wrong result.* These cases end by checking the rows in the table and the rows are there either
way; no case asserts which client inserted them. The trap is real for whoever writes the next case, and it
belongs in the corpus: the five files should name their client on every statement, and the two that block
should put the blocking statement last (`evidence/isolation-controller.md` §5).

### What changes, deliberately

The two fixed sleeps go. Clients start together rather than 21 ms apart. `wait until Cn ready` stops polling and
waits on the client's own pipe, which is the only thing that can end it. `wait until Cn blocked` still polls, because the
answer is the server's, and at qactl's own 10 ms. Asking every millisecond was tried and measured over the 26
sample cases that use the command: 27.23 s against 27.73 s, cases swinging 500 ms either way, and 25 of 26
results byte-identical between the two builds (`evidence/isolation-controller.md` §6). It buys nothing, so the
number stays the one being replaced.

Nothing else about *when* a statement is sent changes, and two places that look like candidates are left alone:
`MC: sleep n` still does not read from the clients while it sleeps, so what they say meanwhile is read after it
rather than during; and each statement still waits for its client to be ready before it is written.

## Consequences

1. **Parity stops being by construction.** ADR-007's first consequence — same script, same binaries, so the
   bytes are the same — half survives: the client is still the same binary. The wrapping is now testkit's, and
   the argument for it is the comparison, under ADR-018's rules, over the whole corpus against CTP.
2. **The raw result interleaves differently, and the compared one does not.** A controller that does not
   sleep sends the next statement sooner, so `result/<name>.result` -- the unnormalized dump -- has the same
   blocks in a different order. The normalization deletes every line that order can be read from: the
   `C%d output (Transaction index = %d):` headers and the client's `is ready` lines (`/Transaction index/d`),
   the `MC to C%d:` echoes and the client's statement echo (`/;$/d`, `/^| Ope_no =/,+1d`) and the blank lines
   between them. What is left is the query results, in the order the case's own `wait until` commands put them
   in. Measured on `_01_ReadCommitted/.../invisible_index_01`: 34 lines differ in the raw file and the
   normalized `result/<name>.log` is identical. It is the same kind of difference the baseline already accepts
   in `test_local.log`, where bash's trace of a pipeline arrives in whichever order it arrives
   (`isolation-baseline.md` §2).
3. **A faster controller moves timing, and some cases are races.** Fifteen cases already flip between runs
   under CTP itself. Detecting a block sooner, or reaping a client sooner, can change which of them lose. That
   is what ADR-018's rule is for — a disagreement is a runner difference only if the case, rerun alone three
   times under each runner, separates the two.
4. **The runner builds one more C file**, against the engine under test, for the same reason ctltool is built
   there: the symbol belongs to that engine's `libcubridcs`. `gcc` was already required (ADR-007 C3).
5. **`runone.sh` must find the new controller at `$ctlpath/qactl`**, which `prepare.sh`'s
   `make clean qactl qacsql` would otherwise rebuild from `qactl.c`. The slot already sees ctltool through an
   overlay of its own; the `Makefile` in it gets a `qactl` target that writes a shim to testkit instead of
   compiling one, with the probe hanging off it, and the build of `qacsql` is untouched. The probe is also built
   when the slot is set up, because `prepare.sh` runs only when it finds no `qactl` or no server, and a case
   must not be the one to discover it missing.
6. **The upstream patch is still worth sending.** The `kill_aclient` fix is four lines and worth 15.4% to
   everyone who runs CTP, whether or not testkit has its own controller
   (`evidence/ctp-improvements.md`).
7. **ADR-008 is no longer reserved.** The grammar had to be written down to be reimplemented, and it is
   (`analysis/isolation/ctl-grammar.md`). ADR-009 stays reserved: the fifteen `sed` steps are still
   `runone.sh`'s, and the controller does not touch them.

## Alternatives considered

**Send the sleep patch upstream and keep ctltool as it is.** This is ADR-007 standing, with a four-line patch.
It wins 15.4% and leaves the other half — the connect sleep, the polls, the serial start — on the floor, and
it leaves the executor a 4,627-line C program of which 563 lines run. Worth doing anyway (C5), not instead.

**Strip `qactl.c` in C, inside testkit.** The same speed by the same means, and the output byte-identical by
construction rather than by comparison. Measured against what was built instead:

| | the reduced C | the Go controller |
|---|---|---|
| what it carries | `qactl.c` minus the 14 definitions of its 44 that nothing enters — the stress subsystem, slave mode, the generic filter, the unreached error paths — which is **964 lines out of 4,627, leaving 3,354**; `qamccom.c` (934) goes whole | **1,387 lines of Go**, 345 of them comment or blank, and **98 lines of C** |
| the output bytes | the same `printf` calls, so the same bytes | re-derived, and the comparison is what says so: two defects found on the sample, the missing lock table and then its `is_contention` argument, both fixed |
| what can be tested without an engine | nothing — every test needs a database | the reader against `parse.c` over 6,790 files, the vocabulary over the same, the install against a stub `make`: 555 lines of test that run in CI |
| what it needs at run time | `gcc` and the engine's headers, as ctltool does | the same, for 98 lines |
| whose code it is | a fork of `cubrid-testtools`' file, and `design/module-isolation.md` §3 says a change to ctltool's C belongs upstream | testkit's, in testkit's language |
| what a disagreement looks like | a diff against `qactl.c` | a comparison report |

The reduction is the safer half-step and the Go controller is the one that can be tested and maintained here.
The decision goes to the second, and the first stays available: nothing in it is precluded, and the
measurements above are what a later reversal would argue with.

**cgo, in testkit's own binary.** Rejected. It would make the runner's build require a CUBRID install and end
the single static binary, to call one function. A separate program, built where the engine is, costs a pipe
round trip of about 50 µs per poll.

**A Go driver instead of the C probe.** There is none that reaches the lock table. `tran_is_blocked` is not in
CCI, not in JDBC and not in any header; it is a client-library entry point answered by the server's lock
manager. This is the same reason ADR-007 gave for not absorbing ctltool, and it still holds — for one function.
