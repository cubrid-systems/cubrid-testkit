# 2. Writing a case

[← back to the isolation category](README.md)

- [Where a case lives](#where-a-case-lives)
- [The language](#the-language)
- [Waiting](#waiting)
- [Answers](#answers)
- [What a case should not rely on](#what-a-case-should-not-rely-on)

## Where a case lives

```
_01_ReadCommitted/serial/
  serial_12.ctl
  answer/serial_12.answer
```

The top level names the isolation levels the case's clients use (`_01_ReadCommitted`, `_02_RepeatableRead`,
`_04_RepeatableRead_ReadCommitted`, `_05_ReadCommitted_RepeatableRead`, `_07_serializable`) or a feature
(`_06_features`). A case is any `*.ctl` file; its answers are beside it in `answer/`. A run adds `result/`.

Two optional files beside the case change what `runone.sh` does: `<name>.sql` is run through csql before the case, and
`<name>.sh` replaces the answer comparison with a script of its own. No case in the corpus uses either.

## The language

A `.ctl` file is a sequence of statements, each ended by a `;` outside quotes and comments. `/* */` and `--` are
comments. A statement is for the controller (`MC:`) or for a client (`C1:`, `C2:`, …):

```
MC: setup NUM_CLIENTS = 2;
C1: set transaction lock timeout INFINITE;
C1: set transaction isolation level read committed;
C2: set transaction lock timeout INFINITE;
C2: set transaction isolation level read committed;

C1: CREATE SERIAL s1 START WITH 101 INCREMENT BY 1 MAXVALUE 20000;
C1: commit work;
MC: wait until C1 ready;

C1: SELECT SERIAL_NEXT_VALUE(s1,10);
MC: wait until C1 ready;
C2: DROP SERIAL s1;
MC: wait until C2 blocked;
C1: commit;
MC: wait until C2 ready;
C2: commit;
C2: quit;
C1: quit;
```

A client statement is SQL, sent by that client's `qacsql` on its own connection. The controller's statements, as
`qactl` reads them (case does not matter):

| statement | what it does |
|---|---|
| `setup NUM_CLIENTS = <n>;` | starts `n` clients; the first statement of every case |
| `wait until C<n> ready;` | until client `n` has finished what it was given |
| `wait until C<n> blocked;` | until client `n` is waiting on a lock — read from the engine's lock table, not guessed from time |
| `wait until C<n> unblocked;` | until it no longer is |
| `wait until C<n> finished;` | until it has run all it will |
| `pause for deadlock resolution;` | three seconds, for the engine to pick a victim |
| `sleep <n>;` | `n` **seconds** |
| `wait for <n>;` | `n` seconds, still serving the clients meanwhile |
| `reconnect;` | the controller's own connection, again |

A wait gets 100 seconds. If by then it cannot come true any more — the client is ready when it was meant to block,
every client is blocked — it fails there; otherwise the controller prints a warning and waits up to 300 seconds more.
A wait that fails prints the lock table and ends the case, which then fails against its answer.

## Waiting

**Every point where the order matters has to be a `wait until`.** Two clients given statements in a row run them
concurrently; the only thing that makes their outputs come out in the answer's order is the controller waiting between
them. `blocked` is the reason the language exists: it lets a case say "C2 is now waiting on C1's lock" and continue
exactly then.

A `sleep` is not a synchronization point. It is time, and time on a loaded machine is not the time the answer was
written on.

## Answers

What a client prints — row counts, results, errors — goes into `result/<name>.result`, each line prefixed with `| ` by
the controller. `result/<name>.log` is the same after fifteen `sed` steps, and that is what is compared. They delete
commit and rollback confirmations, `set transaction` lines, the controller's and the engine's status lines
(`QACTL…`, `INFO…`, `Transaction index`, `shutting down`), statement echoes — any line ending in `;` — `Ope_no` blocks
and blank lines; and they mask what varies between runs: the victim's name in a deadlock abort, killed pids, OIDs and
`key: n` values, and the host name and digits in lock-wait errors.

```
| 3 rows affected
| =================   Q U E R Y   R E S U L T S   =================
| 
| 
|    110  
| 1 row selected
```

A case may have more than one answer — `.answer`, `.answer1`, `.answer2`, `.answer_1` — for outcomes that are all
correct; the first that matches wins, in `ls` order. 59 cases have more than one. Misspelled names (`.asnwer`) are
never read.

## What a case should not rely on

Measured over three whole-corpus runs and reruns alone ([`isolation-baseline.md`](../../project/evidence/isolation-baseline.md)
§4):

- **The order of rows a query does not order.** Six cases fail inside a run and pass alone, their diffs the same
  catalog rows in a different order — three of them only with four slots, where each slot's `ctldb` has seen a
  different sequence of cases. `ORDER BY` makes each of them one case instead of two.
- **What earlier cases left.** Between cases `runone.sh` drops users, triggers, serials, stored procedures, views and
  tables, and nothing else — `ctldb` itself lives for the whole run and is recreated only after a crash.
- **Timing that nothing waits for.** Six cases flip from run to run — which client's `rows affected` lands first, which
  statement meets a unique-constraint violation — under CTP as much as under this runner.
