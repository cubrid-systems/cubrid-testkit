# 2. Writing an `ha_repl` case

[← back to the ha_repl category](README.md)

An `ha_repl` case is a [`sql` case](../sql/02-writing-a-case.md) whose oracle is a second node.
Read that document first — the statements, the `holdcas` framing, the naming and the traps are all
the same — and read this one for what the pair changes.

- [What the conversion changes](#what-the-conversion-changes)
- [What this document does not tell you](#what-this-document-does-not-tell-you)
- [Writing the case](#writing-the-case)
- [The pair](#the-pair)
- [Traps](#traps)

## What the conversion changes

A `sql` case is a file of statements beside a file of expected text:

```
sql/_13_issues/_26_1h/
├── cases/cbrd_26541.sql       the statements
└── answers/cbrd_26541.answer  what running them produced
```

**`ha_repl` takes the statements and drops the answer.** The case runs on the master, the runner
waits for replication, both nodes are dumped, and the dumps are compared. What the case asserts is
no longer *"this is what the engine prints"* but *"the slave holds what the master holds"*.

Three consequences, and they are the whole of what makes this different from writing a `sql` case:

1. **A case whose point is the rendering has nothing to say here.** `--[er]` cases — 2,911 of the
   `sql` corpus exist to pin an error's text — convert to a case where both nodes agree that
   nothing happened. Converting one is not wrong, it is empty.
2. **A case whose point is a side effect on data is exactly right.** Anything that writes, and
   especially anything whose write path the replication log has to represent — types, defaults,
   auto-increment, triggers, partitions, `ON UPDATE` — is where a replication defect can hide and
   an answer file cannot see it.
3. **Non-determinism stops being a problem and becomes the point.** A `sql` case that produces a
   different answer on each run is unusable there. Here both nodes see the same one, so the
   comparison holds — as long as the value is *replicated* rather than *recomputed* on each node.
   That last clause is [the first trap](#traps).

## What this document does not tell you

**The converted corpus's on-disk layout.** It lives in CTP's tree
(`cubrid-testtools`), not in this repository, and it was not read to write this document. What is
recorded here is what
[`module-ha.md`](../../project/design/module-ha.md) §1 measured about the suite — its corpus is the
`sql` corpus converted, its oracle is master-against-slave dump by dump, and the runner builds the
pair — plus what `internal/sandbox` and
[ADR-022](../../project/adr/ADR-022-topology-provider.md) establish about the topology.

Before adding a case, read a neighbouring one in the tree for the file layout. If what you find
contradicts anything here, the tree is right and this document is the thing to fix.

## Writing the case

The statements are a `sql` case's statements. What changes is what you choose to write, and it
follows from [the conversion](#what-the-conversion-changes):

```sql
--+ holdcas on;
--auto-increment continues across the replica

create table t_seq (i INT AUTO_INCREMENT PRIMARY KEY, v VARCHAR(10));
insert into t_seq (v) values ('a'), ('b'), ('c');
delete from t_seq where v = 'b';
insert into t_seq (v) values ('d');

--+ holdcas off;
```

There is no `select` at the end and no answer file. The case's claim is that after these
statements, `t_seq` is the same object on both nodes — including the column the engine assigned,
which is the part an answer file would have pinned on one node and never checked on the other.

**Nothing in the case builds or moves the topology.** The runner has the pair before the first
statement runs, and `ha_repl` performs no `hb stop`, no `service stop` and no `kill`. A case that
wants a node to go away is an [`ha-shell`](../ha-shell/README.md) case, and a case that wants a node
to become *unreachable* is neither — it is group B in
[`module-ha.md`](../../project/design/module-ha.md) §4 and needs a fault verb.

## The pair

`ha_repl.conf` carries it, and the key names are CTP's frozen surface
([ADR-003](../../project/adr/ADR-003-external-surface-freeze.md)):

```
env.instance1.master.ssh.host=...
env.instance1.slave.ssh.host=...
env.instance1.slave2.ssh.host=...      # a master may have several
ha_db_list=...
```

**On the sandbox path**, which is the default for anything new
([ADR-022](../../project/adr/ADR-022-topology-provider.md)), you do not write that file. A pair is
asked for:

```bash
make -C extensions/cluster-sandbox dist
export CSB_HOME=/somewhere
extensions/cluster-sandbox/bin/csb cluster create --name tkha --build /path/to/install.out
```

and `csb cluster describe --format ctp --instance instance1` renders exactly the fragment above.
The key names keep their meaning with one substitution: the nodes run no sshd, so `ssh.host` names
*where this instance is* and the transport is `csb node exec` rather than a port. `internal/sandbox`
reads that fragment into the same `topology.Instance` the conf file produces, so nothing downstream
learns a second model of what a node is.

**Multiple slaves work and used not to.** `conf/ha_repl.conf` has always documented `slave1`,
`slave2`, and `internal/topology` read only the roles a fixed list named — so `env.instance1.slave2.*`
was dropped whole. A role the keys mention is now carried whether or not the list names it
([ADR-022](../../project/adr/ADR-022-topology-provider.md) Consequence 4). If you are writing the
first case that depends on a second slave, that fix is why it works.

## Traps

- **Recomputed is not replicated.** `SYSDATETIME`, `RANDOM`, `SYS_GUID` and anything else evaluated
  per node will differ between master and slave without any replication defect being present. A
  case that includes one is testing the clock. If the *point* is that the value replicates rather
  than re-evaluates, say so in the first-line comment, because the next reader will otherwise
  delete it as flaky.
- **The comparison is the dumps, so anything not in a dump is not checked.** A defect that lands in
  a catalog table, a serial's internal position or a log the dump does not carry is invisible here
  even though the suite looks like it compares everything.
- **The conversion is not frozen.** The freeze covers the HA *shell* corpus, not this one. A
  converted case can be edited, which means it can also drift from the `sql` case it came from
  without anything noticing.
- **A sleep is not a wait, here either.** The runner's own synchronisation is a flag polled to
  `ha_sync_detect_timeout_in_ms`. Do not add a delay inside a case to "let replication catch up" —
  if the runner's wait is not enough, the bound is the thing to change, and a case that hides a
  timing problem behind a delay removes the only signal that it exists.
