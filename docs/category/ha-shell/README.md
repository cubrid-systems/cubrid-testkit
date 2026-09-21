# The `ha-shell` category

`ha-shell` is the `shell` task pointed at a master/slave pair — CTP runs it as `shell` with
`ha_shell.conf`, over the `HA/shell` tree. It is not a task of its own, and that is worth knowing
before anything else here: **a case is a shell case**, discovered by the same rule and judged by the
same verdict helpers.

The other thing that wears the word HA is [`ha_repl`](../ha-repl/README.md), and the two share
almost nothing but their oracle. The one-line difference:

| | **`ha-shell`** | **[`ha_repl`](../ha-repl/README.md)** |
|---|---|---|
| what it asks | does replication survive *this* — a stopped node, a restarted service, a rebuilt slave | does replication copy *this SQL* correctly |
| who moves the topology | the case, 263 of 373 do | nobody. Steady-state by construction |
| who builds the pair | the case, by calling `make_ha.sh` | the runner |
| corpus shape | shell scripts | the `sql` corpus, converted |

| | |
|---|---|
| **[2. Writing a case](02-writing-a-case.md)** | the two paths, the layout, the verbs, and the three rules the frozen corpus breaks |

The other numbered documents the sibling categories have — how a run works, running it,
configuration, what a failure leaves behind — are not written, because the parts they would describe
are not built. [§ The two paths](02-writing-a-case.md#the-two-paths) says exactly which.

The pre-implementation design — every measurement quoted here, and the seven properties HA testing
has to establish — is [`../../project/design/module-ha.md`](../../project/design/module-ha.md).

## What this suite is for

**It establishes that replication copies rows across a topology that is holding still, including
after that topology has been disturbed.** That is a real and useful job, and saying it plainly
matters because the corpus looks like it is doing something else.

263 of the 373 cases stop a node — `cubrid hb stop`, `cubrid service stop`, `kill -9`. What they
then do is ask whether the rows still arrived: 115 assert data equality afterwards, 22 look at the
states the transition passed through, and **4 time it**. So the move is a precondition for a
steady-state comparison and almost never the subject.

Two consequences for anyone writing a case here:

- **If you want to examine the disturbance itself** — a partition, a split brain, a switchover
  measured against its parameters — that is the other axis, it needs a fault verb, and it is
  specified as group B in [`module-ha.md`](../../project/design/module-ha.md) §4. Do not try to
  approximate it with `kill -9`. A node that is *gone* and a node that is *running and believes its
  peer is gone* are different engine code paths, and only the second is where split brain lives.
- **If a setup transition has not finished when your comparison runs, the comparison measures
  nothing** and says so with a green verdict. That is what [rule 1](02-writing-a-case.md#1-never-sleep-to-synchronise)
  is about, and it is the single most common defect in the existing corpus.

## The oracle, and why it is the good part

A case writes on the master, reads the same rows from both nodes into two files, and compares them.
169 of 373 reach their verdict through `compare_result_between_files`; 313 read through `csql`.

**There is no answer file.** Nothing goes stale when the engine's formatting changes, and there is
nothing to re-record. The comparison is between two nodes running the same build, which is why HA
testing is cheap to maintain — and `ha_repl` arrived at the same oracle independently, which is the
best argument that it is the right one.

Keep it. A new case that introduces an expected-output file is giving up the property that makes
this suite worth having.
