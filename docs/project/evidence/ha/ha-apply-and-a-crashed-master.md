# A master crashed under load: on `787defb50` the slave that took over held every acknowledged commit

- **Date:** 2026-11-02
- **What this is:** the crash the restart evidence left to `ha_repl` and csb's `ha` verbs: the
  master's. With `ha_apply_worker_count` above one the slave applies on several workers, and when
  the master goes it becomes the master with whatever it has applied. The question: does it hold
  every commit the clients were told succeeded, each once and in each client's order? No #8052
  commit aims at this, so this is a check of the head, not a before and after.
- **Command:** a shell prototype, `parallel-apply-failover.sh` (kept here), not a testkit verb.
  8 clients on the client node run the restart workload against the master (each inserts a key of
  its own, commits, moves it, commits); DELAY seconds in, `csb node kill` crashes the master node
  (no flush); the script waits for the slave to say `active` (`cubrid changemode`) and reads its
  table client by client. `ha_apply_worker_count=4`; `ha_copy_sync_mode` not set, the engine's
  default.
- **Pairs:** #8052 `787defb50` (`11.5.0.2991-787defb`), RelWithDebInfo, each with a client node, on
  one host: `hap11022bde`, which had just run
  [ha-apply-restart-and-statements](ha-apply-restart-and-statements.md), and two stood up for this,
  `fo787d10` and `fo787d90`. All removed afterwards.

---

## 1. What a client's keys may show

A client that saw A commits acknowledged (`Execute OK.` lines in its csql log) leaves floor(A/2)
keys moved and A mod 2 not; the commit in flight when the master went may be there or not, so A + 1
is allowed too. Fewer than A is a commit lost in the failover; more than A + 1, a moved key missing
from the run 1..n, or more than one key not moved is a second apply or a hole.

## 2. The runs

| crash after | clients' acknowledged commits | slave active after | clients with A + 1 | lost | beyond A + 1, holes | `fail_counter` / -670 |
|---|---|---|---|---|---|---|
| 10 s (6,000 keys a client) | 338 or 339 each | 9 s | 0 of 8 | 0 | 0 | 0 / 0 |
| 30 s (3,000 keys a client, on `hap11022bde`) | 1,057 each | 10 s | 2 of 8 | 0 | 0 | 0 / 0 |
| 90 s (6,000 keys a client) | 3,097 each | 17 s | 2 of 8 | 0 | 0 | 0 / 0 |

In every run the slave, once active, held every commit each client saw acknowledged, and at most
the one in flight. The clients went at the same pace: their counts differ by at most one.

Each client's last commit was answered "Your transaction has been aborted by the system due to
server failure or mode change" (all 8 logs of every run). For 2 clients in the 30-second run and 2
in the 90-second one, that commit is on the new master all the same: a commit the server could not
answer, which the client cannot tell from one that did not happen.

Files, in the working directory: `fo-787d-p1.log`, `fo-787d-d10.log`, `fo-787d-d90.log`, and each
run's clients' scripts and logs in `fo-787d-*.clients/`.

The kept script has two fixes made after these runs: the hole check for a client with no moved
key, and a stop, and a drop, for the table the script waits on before the clients start. Neither
touches these runs: every client had moved keys, and the table reached the slave each time.

## 3. Not here

- **A crash during the slave's recovery**, or of the new master; the old master starting again and
  following the new one.
- **Another copy mode** than the default, under which an acknowledged commit may lawfully be lost.
- **A head that should fail**: no #8052 commit is known to have fixed this, so nothing shows the
  check can miss.
- **A testkit verb**: this is a prototype, run three times on one head.
