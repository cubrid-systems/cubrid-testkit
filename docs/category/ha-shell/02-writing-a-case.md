# 2. Writing an `ha-shell` case

[← back to the ha-shell category](README.md)

An `ha-shell` case is a [`shell` case](../shell/02-writing-a-case.md) that has a second node. Read
that document first: the layout, the four framing lines, the verdict helpers and the traps are all
the same here, and this one only covers what the pair adds.

- [The two paths](#the-two-paths)
- [The layout](#the-layout)
- [The verbs a case gets on the two-machine path](#the-verbs-a-case-gets-on-the-two-machine-path)
- [Three rules, and the corpus breaks all three](#three-rules-and-the-corpus-breaks-all-three)
- [A case on the two-machine path](#a-case-on-the-two-machine-path)
- [Writing for the sandbox path](#writing-for-the-sandbox-path)
- [Traps](#traps)

## The two paths

**The sandbox path is the default for anything written from now on**
([ADR-022](../../project/adr/ADR-022-topology-provider.md)). A pair is asked for rather than built,
the runner drives both nodes from outside, and a case never reaches across to a slave by itself.

**The frozen cases stay on two machines**, and not by preference. They reach their slave by
shelling out to CTP's Java SSH helper, so a node has to be a QA machine: sshd with password
authentication, a JVM, `expect`, and CTP's tree. A sandbox node is none of those
([`ha-topology.md`](../../project/evidence/ha-topology.md) §3), and the corpus is frozen, so the
case cannot be taught a different way to reach its slave.

**What is built on the sandbox path, and what is not.** Be clear about this before choosing it for
work that has to run this week:

| | |
|---|---|
| built, and proven against a live pair | `internal/sandbox` — bind a cluster, read its topology as a CTP fragment, run a command on any node, wait for replication, compare a query across both nodes. Its `TestLive*` tests run against a real cluster |
| **not built** | **anything that routes a suite to a cluster.** No configuration key names one, and no task imports the package. A case cannot yet be *run* this way |
| also missing | the two-machine baseline the frozen cases would be compared against ([`evidence/ha/`](../../project/evidence/ha/README.md)), which is owed before either path can claim a verdict is unchanged |

So a case written for the sandbox path today is written against a shape that is specified and a
seam that is measured, and it waits on the routing. That is a real wait, and it is the reason this
category has no *3. Running it*.

## The layout

Identical to `shell`, because the task is `shell`:

```
<scenario>/
  └── my_ha_case/
        └── cases/
              ├── my_ha_case.sh       ← THE CASE. Named after the directory two levels up
              └── my_ha_case.result   ← written by the run. Do not commit it
```

The discovery rule, the repeated name and the helper-versus-case distinction are all
[the shell rules](../shell/02-writing-a-case.md#the-layout-and-why-the-name-is-repeated). What
changes is `ha_shell.conf` in place of `shell.conf`, and a corpus that points at the `HA/shell`
tree.

## The verbs a case gets on the two-machine path

`make_ha.sh` reads `$init_path/HA.properties`, sources `make_ha_upper.sh`, and leaves these in
scope. The counts are over the 373 frozen cases, and they are worth reading as a map of what this
suite actually does:

| verb | cases | what it is |
|---|---:|---|
| `setup_ha_environment` | 370 | create the database on both nodes, rewrite four conf files, upload them, start the heartbeat, poll `changemode` until active |
| `revert_ha_environment` | 368 | the inverse |
| `run_on_slave` | 248 | a command on the other node |
| `wait_for_slave` | 131 | **until the slave's state is what was asked for.** See rule 1 |
| `run_upload_on_slave` · `run_download_on_slave` | 100 · 48 | move a file |
| `wait_for_active` · `wait_for_slave_active` | 81 · 13 | until `changemode` says active |
| `start_slave_hb` · `stop_slave_hb` | 74 · 52 | the heartbeat, on the other node |
| `stop_slave_service` | 43 | the service, on the other node |
| `slave_cmd` | 25 | as `run_on_slave`, with the engine's environment |
| `format_hb_status` | 12 | make `hb status` comparable |
| `add_ha_db` | 9 | another database into `ha_db_list` |

Three of those are real synchronisation: they poll a state until it is true, with a bound —
`wait_for_active` runs `cubrid changemode` every second, 120 times.

**On the number 373.** Every count on this page is over the 373 cases
[`module-ha.md`](../../project/design/module-ha.md) measured.
[ADR-022](../../project/adr/ADR-022-topology-provider.md) and
[`ha-topology.md`](../../project/evidence/ha-topology.md) say 367 for what reads like the same
tree. The two have not been reconciled, and this page does not pick one — the proportions are what
the rules below rest on, and those hold either way.

## Three rules, and the corpus breaks all three

These are not style. Each one is a measured defect in the tree you would otherwise copy from.

### 1. Never `sleep` to synchronise

| | |
|---|---:|
| cases with a bare `sleep N` | **244 of 373** |
| bare `sleep` statements | **726** |
| seconds slept, summed over the corpus | **20,370 — 5 h 39 m** |
| cases using a real wait | 136 |
| cases using **both** | 91 |

A sleep here is not a delay bolted onto a correct test. **It is the synchronisation** — the case
writes on the master, sleeps five seconds, reads the slave, and never checks that replication
caught up. That is wrong in both directions: on a loaded machine it fails for a reason that is not
the engine's, and on a fast one it passes without having waited for anything.

**Use `wait_for_slave`.** It is not a timer: it creates a table on the master, inserts the row
`'replication finished'`, and polls the slave with `-tillcontains` until it arrives
(`make_ha_upper.sh:74-87`). It has been sitting beside every case for years. Measured on four
cases, the wait costs about **1.3 seconds where the sleep it replaced cost 5 to 30**, and no
verdict changed ([`evidence/ha/p1-sleep-to-wait.md`](../../project/evidence/ha/p1-sleep-to-wait.md)).

And do not do both. 91 cases call `wait_for_slave` and then sleep five seconds anyway.

### 2. The case must be able to fail

Run it before you commit:

```bash
testkit check-cases <scenario> [<init_path>]
```

It reads no engine and runs no case. It finds three things: a case with no route to NOK at all, a
call one typo away from a verdict helper and defined nowhere, and a comparison of something against
itself. On its first run over the HA corpus it found one — `wirte_nok` for `write_nok`, in the
`else` branch of an `if`, which is to say **in the only line that would have reported the failure**.

An answer-file corpus would have surfaced that. This one cannot, because the oracle is a comparison
the case performs itself — which is exactly the trade the [oracle](README.md#the-oracle-and-why-it-is-the-good-part)
makes, and the reason this check exists.

### 3. The oracle is the pair, not a file

Write on the master, read the same rows from **both** nodes, compare the two.
`compare_result_between_files` is how 169 of 373 cases reach a verdict. Do not introduce an
expected-output file: the moment you do, the case has something to re-record when the engine's
formatting changes, and the property that makes this suite cheap is gone.

## A case on the two-machine path

```bash
#!/bin/bash
. $init_path/init.sh
init test
set -x

. $init_path/make_ha.sh
setup_ha_environment

csql -u dba -c "CREATE TABLE t(i INT PRIMARY KEY); INSERT INTO t VALUES (1),(2),(3);" $ha_db

# Rule 1. Not `sleep 5`.
wait_for_slave

csql -u dba -t -N -c "SELECT count(*) FROM t;" $ha_db          > master.log 2>&1
run_on_slave "csql -u dba -t -N -c \"SELECT count(*) FROM t;\" $ha_db" > slave.log 2>&1

# Rule 3. The two nodes are the oracle.
compare_result_between_files master.log slave.log

revert_ha_environment
finish
```

`compare_result_between_files` records the verdict itself, which is why there is no `write_ok`
here — and why `testkit check-cases` had to learn to follow verdicts *transitively* to see that
this case can fail.

## Writing for the sandbox path

The shape a case takes changes, because the case stops being the thing that reaches the slave.

**Stand a pair up**, once, outside the run:

```bash
make -C extensions/cluster-sandbox dist
export CSB_HOME=/somewhere
extensions/cluster-sandbox/bin/csb cluster create --name tkha --build /path/to/install.out
```

**What the runner then has.** `csb cluster describe --format ctp --instance instance1` renders the
topology as an `ha_repl.conf` fragment — `env.instance1.master.ssh.host`, `.slave.ssh.host`,
`ha_db_list` — which is CTP's frozen key set, so no new parser and no second model of what a node
is. `ssh.host` keeps meaning *where this instance is* and stops meaning *an address sshd answers
on*: the nodes run no sshd, and the channel is `csb node exec`.

`internal/sandbox` turns that into a `Pair`, and a case's three needs map onto it directly:

| what a case wants | on the two-machine path | on the sandbox path |
|---|---|---|
| run something on the master | `csql ...` | `Pair.MasterChannel().Run(ctx, ...)` |
| run something on the slave | `run_on_slave "..."` | the slave's `Node`, a fourth `exec.Channel` |
| wait for replication | `wait_for_slave` | `Pair.WaitForReplication(ctx, timeout)` |
| compare the two nodes | `compare_result_between_files` | `Pair.SameOnBothNodes(ctx, query)` |

`WaitForReplication` is deliberately the same shape as the two waits that already exist rather than
a third idea: a marker table created per call, written on the master, polled on the slave, bounded.
CTP's `wait_for_slave` and `ha_repl`'s `Test.java` arrived at it independently, which is the
argument that it is right.

**`Put` and `Get` do not exist on this channel, and are refused rather than approximated.** csb has
no file-transfer verb. A case that needs `run_upload_on_slave` has no sandbox equivalent today —
that is a real gap, not an oversight, and base64 through `node exec` was rejected because it works
until the argument list stops fitting.

**What you cannot do yet:** run the case. Nothing routes a suite to a cluster. Until it does, the
useful output of writing here is the case's shape and the gap it exposes.

## Traps

- **A setup transition that has not finished makes the comparison meaningless, and green.** 263
  cases move the topology and then compare data. If the move is still in flight, the comparison is
  measuring a node that is not yet what the case thinks it is. Rule 1 applies to the setup
  transition, not only to the write.
- **`kill -9` is not a partition.** Every fault the corpus can produce is process-level; `iptables`,
  `ip route` and `tc qdisc` appear in **zero** of 373 cases. A node that is gone and a node that is
  running and believes its peer is gone are different code paths. If you need the second, you need
  group B and a fault verb — [`module-ha.md`](../../project/design/module-ha.md) §4.
- **`to_be_active` is invisible to this corpus.** Zero cases wait on it, assert on it or time it,
  and the field's own tracker records a failover stopping there for hours. If your case's premise
  is a transition that half-finished, nothing in the existing tree shows you how, because nothing
  in the existing tree does it.
- **Do not vary the heartbeat parameters and expect the documented arithmetic.**
  `ha_calc_score_interval_in_msecs`, `ha_max_heartbeat_gap` and `ha_heartbeat_interval_in_msecs`
  appear in zero cases, and cluster-sandbox measured over nineteen runs that raising either
  heartbeat parameter fourfold leaves the result inside its own baseline band. A single run of a
  case that varies them is not evidence; a distribution is.
