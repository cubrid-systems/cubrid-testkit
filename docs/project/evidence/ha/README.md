# The HA baseline — what CTP's HA shell does on this engine

- **Status:** **not measured.** This directory is the harness and the reason; the numbers are not
  here yet because the pair is not configured.
- **What it is for:** the 373 frozen HA cases, run by CTP, on two machines, so that everything after
  it has something to be compared against. ADR-015's second criterion — parity before beyond — and
  `design/module-ha.md` §5 both land on this being owed first.

---

**Start here to pick the work up:** [where-this-stands](where-this-stands.md) — what the native
`ha_repl` runner is, how to run it, the state of both environments, the four candidates for what
is next, and the traps that cost a day each.

## Findings from this directory

- [ctp-ha-repl-deletes-the-call](ctp-ha-repl-deletes-the-call.md) — **2026-09-23.** Why the
  findings above are new, and it is not that nobody looked. CTP's ha_repl conversion **deletes
  every statement beginning with `CALL`** — a 2012 rule, from when `CALL` meant a method on a class
  — and almost every `SELECT` besides. So 1,707 catalog-method calls never reach a CTP run, which
  is three of this directory's findings; and 10,257 procedure calls in 1,037 cases go with them,
  because PL/CSQL made `CALL` a way to run arbitrary DML and the rule was never revisited. In **26
  cases the deleted `CALL` is the only thing that would have written a row**: the converted case
  creates a table and a procedure, drops both, and asks the slave nothing. The change that taught
  the conversion about PL/CSQL (CUBRIDQA-1204, 2023) added 461 lines to preserve the procedure
  *body* and left the line that deletes the call to it.

- [ddl-with-a-session-variable-does-not-replay](ddl-with-a-session-variable-does-not-replay.md)
  — **2026-09-23.** The scale run's other cause, and not the one above: a `CREATE CLASS` whose text
  names a csql variable — `create class t(c2 x SHARED :arg1)` — is replayed verbatim on the slave,
  which has no such variable, so the class is simply absent there. Discriminated with a control: a
  variable holding a **number** fails the same way, a literal default does not, so it is the
  variable and not what it holds. **The first finding in this directory the engine reports**: the
  applier names the statement and says `Unknown variable`, and `fail_counter` moves. That corrects
  the attribution of `_004_db_attribute/1003` and `1007`, which have no user, no owner change and
  no method call in them.

- [method-calls-on-the-catalog-do-not-replicate](method-calls-on-the-catalog-do-not-replicate.md)
  — **2026-09-23.** The rule the two findings below are instances of, and the scale run is what
  showed it is a family: a method call on a catalog class reaches the slave only if that class has
  a **primary** key, and three of them have none. The newest instance is the sharpest —
  `create user u` replicates and `call add_user ('u') on class db_root` does not, so **a user can
  exist on the master alone**, with every grant that rests on it. Measured over `_10_system_table`
  on a clean pair: **11 differences in 244 cases**, all of them this rule or a consequence. Two of
  the eleven are this runner's own: the oracle compares two nodes only while the same session can
  be established on both, and a `call login` of a user the slave never got makes the two reads run
  as different users.

- [class-owner-change-not-replicated](class-owner-change-not-replicated.md) — **2026-09-22.**
  `call change_owner (...) on class db_root` does not reach the slave, for the same reason the
  trigger's does not: `_db_class` has an index and not a primary key. **Unlike the trigger, no key
  is coming for it**, so this one still wants a judgement. It also explains the run's other new
  difference without any engine defect in it: a reset runs on the master and reaches a slave as
  replication, so once the owners disagree the DROP names nothing on the slave, the object stays,
  and every later case starts from a database the run did not choose. The suite now repairs that
  and — more importantly — records that it had to. Two runner defects fell out on the way: the
  key conversion was breaking any table that already auto-increments, and a `SELECT` of a serial's
  next value was being compared across the pair.

- [where-the-time-goes](where-the-time-goes.md) — **2026-09-24.** The measurement that was supposed
  to decide whether to open axis O, and the answer is **no case**. Pairs from 1 to 12: it is not the
  cores (4.2 of 16 working at twelve pairs), not the bandwidth (25 MB/s at most), and the machine is
  never saturated — 5.5 cores idle at n=12 while wall clock is still falling. iowait is the only term
  that grows and it grows beside an idle disk, which makes it commit **latency** rather than
  throughput. Also finds what nobody was looking for: **free disk is a performance variable** — the
  same eight pairs run the same arm in 42 s at 90% full and 32 s at 87%, so cleaning up is a
  performance setting and not hygiene. **Extended 2026-09-25** with the arms that scaling one
  category cannot show: two categories at once land on the *sum* of their times, not the maximum —
  `shell` 141 s and `ha_repl` 44 s become 182 s together, and the whole cost falls on `ha_repl`
  (4.14x) while the CPU sits at 4%. There is nothing to schedule around, because the contention is in
  what a host has one of. So machine separation is an isolation guarantee rather than a speed
  optimisation, and buying it is the caller's choice.

- [the-forty-two-and-what-they-were](the-forty-two-and-what-they-were.md) — **2026-09-23.** The
  whole-corpus run over `_01_object` reported 42 failures. **Thirty-one were the runner.** Twenty-six
  were eight-way contention pushing `_09_partition` past a sixty-second bound — the clearest of them
  takes 88.7 s and fails in a sharded run and 1.4 s and passes alone. Two were a conf key this runner
  never read: CTP's is `ha_sync_detect_timeout_in_secs`, in seconds, defaulting to 600; this read the
  name of the Java *field* and defaulted to 60. Three were a bound that did not exist, for a call
  that runs a case rather than a read — the corpus's largest case needs 250 s on a cold database and
  was killed at 120 s, then reported as "the master could not be reached". **The other eleven are the
  engine, and all three of their causes are already in this directory.** No new engine finding in
  3,327 cases.

- [a-sandbox-pair-wears-out](a-sandbox-pair-wears-out.md) — **2026-09-23.** A pair's cost per case
  rises with how much it has been used and never falls: the per-case reset drops the schema, but the
  database keeps the volumes it grew and the copy log keeps its 2.2 GB. Found by an eight-shard
  timing that read **0.86x** — eight pairs slower than one — which was one pair at 127,114 log pages
  running 9.83x slow, not anything about concurrency. Rebuilt fresh, the same slice gives **4.58x**.
  Consequences: a sharded run needs pairs of equal wear, and the ledger's duration column is not
  comparable across a long run.

- [pairs-across-machines](pairs-across-machines.md) — **2026-09-23.** A design note for putting the
  masters here and the slaves elsewhere, under one frame: **local or remote, you use `csb`** — no
  second tool, no mode the caller picks. The runner already needs nothing (ADR-022 made a node's
  name its address) and the tailnet already carries the network and the faults. What is missing is
  placement, an agreement step inside `create`, and a direct transfer between the machines for the
  seed. Records the invariant that breaks if treated as advice — a fault verb must not be able to
  cut the tool's own control channel — and the probe of the notebook: same engine build, sshd, on
  the tailnet, **no container engine**, which is a missing backend rather than a reason for a
  different tool.

- [split-brain-divergence-converges](split-brain-divergence-converges.md) — **2026-09-22.** Group
  B's first measurement, and the first time this suite has asked its question of a topology that
  moved. A split brain is reached on purpose, a row is written on each side, the network is healed,
  and the pair is read. **It corrects the sibling project's published finding rather than
  confirming it:** the divergence is real, and the direction reproduces, but it is not permanent —
  three of three runs differ at thirty seconds and three of three agree at ninety, with nothing
  written in between. Every gauge reads healthy throughout, which is the half that stands. The arm
  that writes a marker was a coin flip because the marker itself takes 55 s to cross a pair that
  has just healed.

- [unjudged-cases](unjudged-cases.md) — **2026-09-22.** The 24 cases the suite reports as
  establishing nothing. 23 of them execute no read at all — and **CTP judges them anyway**, by a
  second oracle this document first missed: a flag table of its own, written on the master and
  polled on the slave once per case whatever the case contains. This suite has the parts for the
  same check and throws the result away. **Run: 10 executed, 10 NOK** — the liveness oracle passed
  271 times out of 271, and what failed was the per-statement comparison, on statements the engine
  refuses, which CTP retries as though the slave were behind. Also records the `kill -9` in CTP's
  cleanup that had to be removed before it could be run, and the two configuration traps after it.

- [trigger-owner-change-not-replicated](trigger-owner-change-not-replicated.md) — **2026-09-22.**
  `call change_trigger_owner (...) on class db_root` does not reach the slave. Three neighbouring
  changes do, including the DDL form of the same change and the same method form on a serial, so
  it is neither "triggers" nor "methods". **Explained:** `_db_serial` has a primary key and
  `_db_trigger` has no index at all, so the serial's instance update replicates as data and the
  trigger's cannot — while both DDL forms arrive on the statement channel. The primary-key rule
  again, in the catalog. Nothing reports it. **Judged 2026-09-22: passed by, not filed** —
  CBRD-27302 (PR #7980) gives `_db_trigger` the key it lacks. The suite keeps reporting the
  difference rather than skipping it, and what is left is one re-run after that merge.

- [ha-repl-wide-sample](ha-repl-wide-sample.md) — **2026-09-22.** 131 cases, two runs, identical
  verdicts: 119 same, 1 differ, 765 reads compared. **110 of the agreeing reads returned no rows
  on the master either**, so the suite establishes less than the tally suggests. The one
  difference is a trigger's owner, changed by a method call rather than by DDL.

- [object-domain-not-replicated](object-domain-not-replicated.md) — **2026-09-21.** A column whose
  type is another class holds a value on the master and a stored NULL on the slave. **A known
  constraint, undocumented**, which is why it is written here: the suite met it six times as a
  difference before it could be called one. Isolated to four statements, and the source explains
  it — replication carries the primary key, and the slave rebuilds the row from the master's heap
  image, in which an object reference is an OID. Nothing reports it: the applier logs nothing and
  `fail_counter` does not move.

## Why this is the first measurement, and what it is no longer a gate on

Three separate pieces of work are waiting on one number nobody has.

1. **P1, the sleep patches.** 244 of 373 cases synchronise with `sleep`, 20,370 seconds of it. The
   patches that replace those with `wait_for_slave` are writable today — but *"no verdict changed"*
   is the whole claim, and there is nothing to compare against.
2. **ADR-022's sandbox path.** Whether a pair provisioned by `cluster-sandbox` produces the same
   verdicts as two machines is a comparison, and it needs the left-hand side.
3. **Group B at all.** A fault-injection case is a claim about behaviour under a disturbance, and a
   corpus whose ordinary behaviour is unrecorded cannot support one.

**The findings in this directory are not among them** — decided 2026-09-23, and recorded as an
amendment to ADR-015's second admission criterion. That criterion exists to stop an improvement
destroying the evidence that a rewrite was faithful; where the baseline cannot express the question
there is no such evidence to destroy. CTP's `ha_repl` converts the sql corpus before running it and
the conversion deletes every `CALL` and almost every `SELECT`
([`ctp-ha-repl-deletes-the-call.md`](ctp-ha-repl-deletes-the-call.md)), so three of the findings
here are about statements a CTP run never executes. There is no verdict of CTP's to diff them
against, and they stand on their own reproductions.

What still needs the baseline is what CTP can actually run: the three items above, unchanged.

## What has to be true of the pair

Measured, not assumed — `../ha-topology.md` §3 derived these by reading `make_ha.sh`,
`make_ha_upper.sh` and `ha_common.sh`, and then checking a sandbox pair against them.

| | why |
|---|---|
| **sshd on both, with password authentication** | `run_on_slave` is CTP's `run_remote_script`, a Java class that takes `-password`. A key-only slave fails every case |
| **a JVM on both** | same reason: the helper the case shells out to is Java |
| **CTP's tree on both** | `$init_path` is `CTP_HOME/shell/init_path`, and `HA.properties` is written into it |
| **`expect`, `scp`, `ssh`** | `make_ha.sh`'s header requires `hostname.exp`, `rm_db_info.exp`, `scp.exp`, `start_cubrid_ha.exp` |
| **a CUBRID build on both, `$CUBRID/conf` writable** | `modify_cubrid_conf` and its five siblings rewrite those files |
| **the master can reach the slave** | replication runs between the nodes, not through the controller |

`preflight.sh` checks every one of them in seconds:

```sh
./preflight.sh <master> <slave> <user> [<password>] [<ssh-port>]
```

It exists because the alternative is finding out inside case 200, in a message about something else.
It needs a key to look, which CTP does not — so a pair can pass every check here and still be
misconfigured for CTP in exactly one way, which is why the password check is separate and asks for
the password explicitly.

## Running it

```sh
cp $CTP_HOME/conf/ha_shell.conf ha_shell.conf
cat >> ha_shell.conf <<'EOF'
scenario=${HOME}/cubrid-testcases-private/HA/shell
env.instance1.ssh.host=<master>
env.instance1.ssh.relatedhosts=<slave>
env.instance1.ssh.user=<user>
env.instance1.ssh.pwd=<password>
EOF

bash $CTP_HOME/bin/ctp.sh shell -c ha_shell.conf
```

`relatedhosts` is the whole switch. `Deploy.java:65-74` reads it, deploys to each host it names, and
then calls `DeployHA` for the first — which writes `$init_path/HA.properties` and does nothing else.
Everything after that is the cases calling `make_ha.sh`.

## What to record

The comparison this baseline serves is per case, so the artifact is the verdict map and the timings,
not a summary:

- **every case's verdict**, as `dispatch_tc_FIN_*` and `test_status.data` carry it
- **how long the run took, and how much of it was `sleep`** — the second number is already known
  statically (20,370 s) and the first is what says whether that is most of the run or a tenth of it
- **which cases are unstable**, from a second run in the same order. ADR-018 measured CTP moving
  seven verdicts between two identical isolation runs; nobody has asked the same of HA, and P1's
  claim cannot be read without it
- **the eleven staged cases** (`cubrid-testkit-patches`, "Cases that cannot fail"), run with the
  patch and without. One of them is in this corpus

Two runs, then, not one. The second is not a luxury: a single run cannot tell a verdict that moved
from a verdict that changed.
