# The `ha_repl` category

`ha_repl` is a task of its own. It takes the `sql` corpus, runs it on a master, and checks that the
slave ends up holding the same thing — so it is the `sql` suite's question asked across a pair
instead of against an answer file.

The other thing that wears the word HA is [`ha-shell`](../ha-shell/README.md). They share their
oracle and almost nothing else:

| | **[`ha-shell`](../ha-shell/README.md)** | **`ha_repl`** |
|---|---|---|
| what it asks | does replication survive *this* — a stopped node, a restarted service, a rebuilt slave | does replication copy *this SQL* correctly |
| who moves the topology | the case, 263 of 373 do | **nobody.** Steady-state by construction |
| who builds the pair | the case, by calling `make_ha.sh` | **the runner** |
| corpus shape | shell scripts | the `sql` corpus, converted |
| frozen | yes | **no** — the conversion is not |

| | |
|---|---|
| **[2. Writing a case](02-writing-a-case.md)** | what the conversion changes, and the one rule that carries over |

**This task still runs as CTP's, as a subprocess, unchanged** — see
[What runs where](../../../README.md#what-runs-where). There is no native runner and no gate, so the
numbered documents about stages, slots and configuration that the native categories have would be
describing CTP rather than this repository. They are not written for that reason.

## What this suite is for

**It establishes that replication is correct for a statement, with the topology held still.** Not
that the topology survives a disturbance — that is [`ha-shell`](../ha-shell/README.md)'s job, and
`ha_repl` does not do it at all: two or three of its twenty-odd Java files mention `hb stop`, and
those are deploy and cleanup.

That makes it the narrower and the sharper of the two. A failure here is a replication defect for a
specific SQL construct, with nothing else moving that could explain it.

## The oracle, arrived at twice

A case runs on the master; both nodes are then dumped and the dumps compared.

This is the same oracle as [`ha-shell`](../ha-shell/README.md#the-oracle-and-why-it-is-the-good-part),
and the two suites reached it **independently**. That is the strongest evidence available that it is
the right one for HA: there is no expected output to go stale, nothing to re-record when the
engine's formatting changes, and the comparison is between two nodes running the same build.

It is also what the conversion is *for*. The `sql` corpus's cases each carry an `.answer` file
recording what running them produced; converting a case to `ha_repl` means **the answer file stops
being the oracle** and the slave takes its place.

## Its wait, which is the same wait

`Test.java` writes a flag on the master, polls the slave for `GOOD-<id>`, backs off between
attempts, and gives up at `ha_sync_detect_timeout_in_ms`.

CTP's `wait_for_slave` on the shell side does the same thing by a different route, and
`internal/sandbox.WaitForReplication` is a third implementation written to match both rather than
to improve on them. Three arrivals at one shape is why
[`module-ha.md`](../../project/design/module-ha.md) §4 P1 states it as a property rather than a
preference: **synchronisation is a poll on state, never a sleep.**
