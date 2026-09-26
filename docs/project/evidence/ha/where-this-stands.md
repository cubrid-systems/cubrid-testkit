# HA: where this stands, and the four ways on

- **Date:** 2026-09-22, updated 2026-09-23 (A settled, B run, C started) and 2026-09-25
  (the 42 classified, three runner bounds fixed, axis O settled — see §7)
- **What this is:** the handover. Everything below is what someone starting cold needs to pick up
  any of the four candidates without rediscovering it — including the things that cost a day and
  leave no trace in the code.
- **Read the findings for *what was learned*.** This document is *what to do next and how to run
  it*.

---

## 1. What exists

**A native `ha_repl` runner** (`internal/runner/hareplsuite`), behind `TESTKIT_NATIVE=ha_repl`.
It drives both nodes of a cluster-sandbox pair through `csb node exec` — not SSH, which is why it
is not CTP's runner and cannot be (ADR-022; a sandbox node has no sshd).

Its oracle, in the order the parts matter:

| | |
|---|---|
| the pair disagreeing with itself | a case's own SELECTs, run on both nodes and compared |
| synchronisation | a marker written on the master and polled on the slave — **never a sleep** |
| liveness | for a case that writes and never reads, one marker round trip: was replication alive while it ran (taken from CTP's `Test.java:937`) |

Outcomes, and what each means:

| | |
|---|---|
| `same` | every comparable read agreed |
| `differ` | a read disagreed after the wait — the thing the suite exists to find |
| `replicating` | the case wrote, made no comparable read, and its marker arrived |
| `unreplicatable` | every read touches something CUBRID HA does not replicate |
| `no_data` | the case neither writes nor reads |
| `skipped` | the splitter could not finish a block body |
| `session_differs` | the two nodes could not be put in the same session, so the reads are not the same question |
| `wait_timeout` / `case_failed` | the run, not the case |

**Four things are known not to replicate**, and a read touching one is skipped rather than
reported: a table with no primary key, a view onto one, a synonym for one, and an object-domain
column ([`object-domain-not-replicated.md`](object-domain-not-replicated.md)). The first is the
rule the other three are consequences of.

**A fifth is known and is not skipped, because it is a route rather than a shape.** A method call
on a catalog class -- `call change_owner (…) on class db_root` and its relatives -- reaches the
slave only if that class has a primary key, and `_db_trigger`, `_db_class` and `_db_user` have
none ([`method-calls-on-the-catalog-do-not-replicate.md`](method-calls-on-the-catalog-do-not-replicate.md)).
It stays reported: the DDL form of each of those changes replicates perfectly well, so skipping
the catalog they touch would hide coverage that works.

**The oracle has one precondition and it is now enforced** (2026-09-23). The comparison means what
it says only while the same session can be established on both nodes: a case that does
`call login ('u')` after creating `u` by the method route logs in on the master and fails to on the
slave, and the two reads then run as different users. `SELECT CURRENT_USER` is the last statement
of both batches now, and a segment whose two answers disagree about it is skipped and counted --
`session_differs` when nothing else in the case was comparable.

## 2. How to run it

```bash
# once, if the pair is not up
cd extensions/cluster-sandbox
CSB_CLUSTER=pmha ./bin/csb cluster up          # or: cluster create --name pmha --build <install.out>

# the run
export TESTKIT_CSB=$PWD/extensions/cluster-sandbox/bin/csb
TESTKIT_NATIVE=ha_repl ./bin/testkit ha_repl -c <conf>
```

A conf, with every key that matters:

```
scenario=/data/workspace/repos/cubrid-testcases/sql/_33_elderberry
sandbox_cluster=pmha                  # or $TESTKIT_CSB_CLUSTER
ha_sync_detect_timeout_in_ms=60000    # CTP's own name for the wait bound
add_primary_key=yes                   # off by default; the unconverted run is the baseline
reset=case                            # per case, which is what makes a run reproducible
```

`add_primary_key` gives a `CREATE TABLE` with no key a column of its own,
`tk_repl_key INT AUTO_INCREMENT PRIMARY KEY`, and rewrites the case's positional INSERTs around
it. Measured to add **zero** refusals the corpus did not already have.

Differences are written to `ha_repl_differences/` beside the conf — both answers and a normalised
diff, so a `differ` can be read instead of believed.

## 3. The two environments

**The sandbox pair `pmha`** — rootless podman, on this host, provisioned by cluster-sandbox. This
is what the runner uses. `csb cluster status` says whether it is serving.

**A second sandbox pair `gbha`**, created 2026-09-22 for group B. It exists because that work makes
two masters out of a cluster and partitions it, which cannot be done to a pair a run is using.
Anything that moves the topology goes here; `pmha` stays for the corpus runs.

**A two-machine pair** — `hgryoo-desktop` (master) and `hgryoo-notebook` (slave), over tailscale,
which is the only thing CTP can run on. As of 2026-09-22 it is **up, HA-configured by CTP, with a
database `xdb`**, and the three conf files on both machines were rewritten by CTP's deploy.
Backups of the originals are in this session's scratchpad; if they are gone, the machines are
still working pairs and the conf is CTP's rather than the operator's.

`./preflight.sh <master> <slave> <user> [<password>]` checks in seconds whether a pair can host
CTP's corpus at all. It reported READY on this one.

## 4. The four ways on — A settled and B run (2026-09-23), C started, D open

### A. The one difference — *settled 2026-09-22, and not by work*

`change_trigger_owner` does not replicate and three neighbouring owner changes do
([`trigger-owner-change-not-replicated.md`](trigger-owner-change-not-replicated.md)). Measured,
discriminated and explained: `_db_trigger` has no primary key and `_db_serial` has one, so the
serial's instance update replicates as data and the trigger's cannot, while both DDL forms arrive
on the statement channel.

**Judged: passed by, not filed.** CBRD-27302 (PR #7980, open, draft) gives `_db_trigger` a
`unique_name` primary key — for name lookup rather than for replication, but the method's instance
update gets a channel out of it either way. The difference stays *reported*: it is deliberately not
made the fifth skip, because `alter trigger ... owner to` replicates through `_db_trigger` today
and a skip there would hide working coverage.

**What is left is one re-run**, on a tree that has #7980 — the four statements in the finding, read
back as `unique_name` **and** `owner.name`. The owner column is an object domain, this suite's other
finding says those arrive as NULL, and `_db_serial.owner` nonetheless arrived intact; that tension
is unexplained, so the key alone may not settle it.

### A2. One CBRD issue is owed, and only the drafting is blocked

The scale run's finding is judged and the judgement is **file it, as one issue covering all
three** — see *The judgement* in
[`method-calls-on-the-catalog-do-not-replicate.md`](method-calls-on-the-catalog-do-not-replicate.md),
which carries every field the form asks for. The draft was not written because the
`cubrid-org-jira-create` skill's rule book (`issue/methodology/jira-writing.md` and its siblings)
is not on this machine and the skill halts rather than draft without it. Bring that vault, or run
the skill where it lives, and this is twenty minutes.

The one field still genuinely unknown is **affects versions**: only develop at `5f3a30d` is
measured, so a branch sweep is owed before the issue is filed.

### B. Scale — `_01_object`, 3,327 cases — *run 2026-09-23*

What it answers: whether the four known non-replicating shapes are the whole list, or whether a
fifth is waiting in a corpus twenty-five times larger than anything run so far.

**The reset had to be fixed before it could mean anything.** Starting this run printed the same
warning on every case: the database still holds `own_t`. The reset dropped by bare name, and a
bare name resolves in the caller's schema, so the drop of a table a case had given to another
owner named a table that does not exist — and failed exactly as a success looks. Serials, triggers
and users it never dropped at all, which `_04_trigger` and `_05_serial` would have met in the
first hour. Both are fixed, and the same 131 cases of `_33_elderberry` now give **117 same and
3 differ** where the recorded run gave 119 and 1
([`ha-repl-wide-sample.md`](ha-repl-wide-sample.md)).

**What to expect of the tally.** 72% of the corpus contains a `SELECT`, but it is not spread
evenly: `_01_type` is 11% and `_09_partition`, which is 1,500 of the 3,327, is 92%. So the early
hours come back `replicating` — the case wrote, made no comparable read, replication was alive —
and the comparisons arrive late. A run stopped halfway establishes much less than half.

**It ran.** 3,327 cases in 64 minutes: **2,308 same, 7 differ, 943 replicating, 67
unreplicatable**, one `wait_timeout` and one `case_failed`. 2,256 reads compared, of which **356
agreed about nothing** — the master returned no rows either. The tail of it was measured over a
pair a user trigger had left dirty, so `_10_system_table` was run again clean: **11 differences in
244 cases**, and one sentence explains all of them —
[`method-calls-on-the-catalog-do-not-replicate.md`](method-calls-on-the-catalog-do-not-replicate.md).

**What it answered.** The four known non-replicating shapes were not the whole list. The fifth is
not a shape at all but a *route*: a method call on a catalog class, which reaches the slave only
when that class has a primary key. `_db_trigger`, `_db_class` and `_db_user` have none, and the
last of those means a user — and its grants — can exist on the master alone.

**What it cost to get there:** four defects in this runner, each now fixed and tested, and a fifth
left open and named (the oracle assumes the same session can be established on both nodes). The
run is resumable now, which it had to be: something stopped every CUBRID process on the host four
times while it was going.

### C. Group B — cases that move the topology — *most likely to find something*

Every fault verb was executed and reversed on a rootless pair this week
(cluster-sandbox PR #6): `partition` both mechanisms, `ping-unavailable` both, `lag` suspend and
netem, `contend` cpu and io, `failcount`, `splitbrain`. The split brain produced the engine's own
`ha_ping_hosts` sentence verbatim.

`design/module-ha.md` §4 specifies group B — P3 partition, P4 split brain as a verdict, P5
divergence the gauges cannot see, P6 the switchover parameters as inputs — and deferred it for
want of a provisioner that could cut a network. **That provisioner now exists and is measured.**

This is the question the suite has never been able to ask. Everything so far establishes that
replication is correct while the topology holds still, and `module-ha.md` §3-1 is blunt about how
little of the corpus even does that.

**Started 2026-09-22, and it found something on the first day.** The testkit side of the fault
verbs is in `internal/sandbox` — `Partition`, `SplitBrain`, `Faults`, `ClearFaults`, and
`HAStatus` beside them as the gauge the oracle is not. The measurement is
`TestLiveSplitBrainLeavesWhatTheGaugesDoNotReport`, which needs a cluster of its own because it
makes two masters out of it:

```bash
extensions/cluster-sandbox/bin/csb cluster create --name gbha --build /path/to/install
TESTKIT_CSB=extensions/cluster-sandbox/bin/csb TESTKIT_CSB_CLUSTER=gbha \
  go test ./internal/sandbox/ -run TestLiveSplitBrain -v -count=3 -timeout 40m
```

What it found is in
[`split-brain-divergence-converges.md`](split-brain-divergence-converges.md): the divergence a
healed split brain leaves is **not permanent**, which corrects the sibling project's finding
rather than confirming it. Three of three runs differ at thirty seconds and agree at ninety with
nothing written. Everything else there stands, including that every gauge reads healthy the whole
time.

**What is left of group B:** P3 on its own (a partition without a split brain, and the `drop`
mechanism as well as `blackhole`), P4 as a *verdict* rather than a state a measurement asks for,
P6 at all, and the case format — this is a Go test, not something a case can express. The design
says HA needs "honest waits and a fault verb, and both are functions", and the functions now
exist.

### D. Tidy up

The two-machine pair can be torn down and its conf restored, or left as the only CTP-capable
environment there is. It is the left-hand side the owed baseline needs, so leaving it is
defensible.

## 5. Traps, each of which cost real time

- **CTP's cleanup used to `kill -9` every process the invoking user owns**, on the master, which
  on a workstation is the operator's session. Removed from `util_common.sh` on both machines of
  the pair, along with the two helpers that existed only to serve it. **Any third machine's CTP
  tree still has it.** `upgrade.sh` was verified not to restore it.
- **This repository carries the same trap, in its own copy, and containment is off by default.**
  `internal/runner/sqlsuite/stages.sh` `do_clean()` runs `pkill cub` (:73) and then
  `remove_shared_memory()`, which is `ipcs -a | grep $USER` into `ipcrm -m` (:84-86) — every
  shared-memory segment the account owns, not the run's. `internal/runner/shellsuite/deploy.go`
  (:219-221) sweeps ipc the same way on each host it deploys to. `do_clean` runs at the start of
  every `sql` run, and `TESTKIT_CONTAIN` is off unless set to `1` (`internal/contain/contain.go:27`).
  So **an uncontained `testkit sql` takes this project's own sandbox nodes with it**, along with
  anything else CUBRID the account is running.

  Measured from the other side on 2026-09-22: a peer session ran the sql suite about ten times
  between 22:44 and 22:52 **with** `TESTKIT_CONTAIN=1`, each run doing both the pkill and the
  ipcrm, and the clusters here lived through all of it. Containment holds; its default does not.
  Guarded the same day: `do_clean`'s two account-wide steps now run only inside containment and
  say why when they do not.
- **The third tree was on this machine.** Four mass deaths on 2026-09-22 — 21:22:12, 22:34:52,
  23:02:52, 23:03:39 — every CUBRID process the account owned, both sandbox clusters and the
  host's own install, in the same millisecond each time. A one-second watcher caught the last two:
  at both moments a process from **`/data/cub_sys/projects/regr-sql/CTP`** was running
  (`shell/init_path/cubrid createdb ... hnswload`), several levels below a peer Claude session.
  That tree still carries what was taken out of the pair's:

  | | |
  |---|---|
  | `shell/init_path/init.sh:807`, `:1733` | `pkill cub` |
  | `shell/init_path/init.sh:681` | `ipcrm -m` |
  | `shell/init_path/rqg_init.sh:440-443` | `ps -u $USER ... \| xargs kill -9` — server, broker, cas, master |
  | `shell/src/com/navercorp/cubridqa/shell/common/Constants.java:178`, `:198` | the same two, from the Java side |

  44 matches in that tree. Which line fired is not established; that the mechanism is there and
  that it was running at both captured moments is. **A sandbox pair on a shared account is not
  isolated from any CTP tree on the same machine**, and the note above about a third machine's
  tree was too narrow: a third *directory* is enough.

  The session driving it confirmed: its shell cases sourced that `init.sh` directly and
  uncontained, all evening, and it has since replaced it with a five-function stub of its own. It
  also reports a **second failure mode of the same tree, which is not the sweep**: a correlated
  query in its case answered `Cannot coerce value of domain (null) to integer` every time under
  that `init.sh` and correctly against the same database outside it. Not root-caused, and **not
  verified here** — it is recorded because the consequence is worth knowing before it is met: that
  tree can change the verdict of the case running in it, so a NOK read out of it is not yet a
  statement about the engine. `init.sh` prepends its own `bin` and `commonforc/lib` to `PATH` and
  `LD_LIBRARY_PATH`, which is where that session would look.
- **`cubrid_download_url` must be absent, not a placeholder.** `Main.java:72` treats any value as
  a request to install: `file:///dev/null` ran the installer, which refused it, and left
  `buildId` null for an NPE two steps later.
- **CTP needs the HA parameters given explicitly** — nothing else supplies them and heartbeat
  will not start:
  `env.instance1.cubrid.ha_mode=on`,
  `env.instance1.ha.ha_node_list=cubrid@<master>:<slave>`,
  `env.instance1.ha.ha_db_list=<db>`.
- **CTP authenticates by password only.** SSH keys pass the preflight and fail every case.
- **`xdbms32.sql` stalls any runner.** It is a real case that inserts 63 KB of HTML documentation,
  2,810 tags. CTP retries its comparison per line; this suite's splitter reads it as 170
  statements and no reads, because the HTML carries both semicolons and unbalanced apostrophes.
- **A rootless sandbox pair is not isolated from a host-level CUBRID stop.** On 2026-09-22 all
  four nodes of **two** clusters — one of them idle — stopped at the same millisecond, 1,228 cases
  into a 3,327-case run. No crash, no OOM, disk not full, and the containers still up: each node's
  master log ends with a clean *"CUBRID heartbeat feature stopped."* The containers share no `/tmp`
  with the host, so it was not a socket; a rootless container's processes are the same user's host
  processes, so anything that stops CUBRID by walking the process table — `cubrid service stop`,
  a `pkill cub_`, CTP's cleanup — reaches inside every container at once. The run reported
  2,100 `wait_timeout`, which is at least honest, and cost 70 minutes. `csb cluster up` brings a
  pair back with its database intact.
- **A DROP by bare name is a no-op once a case has changed an owner**, and it reports nothing.
  `DROP TABLE [t]` as dba means `dba.t`; the table a case gave to `u7` is `u7.t`. Ask the catalog
  for `[owner].[name]`. The same shape will bite anything else that drops, renames or grants by
  name.
- **Never validate a query with `2>/dev/null`.** A reset query here was checked that way, came
  back empty, and was empty because it was a syntax error: `db_serial.owner` is an object on
  `_db_serial` and a varchar on the `db_serial` view. It read as "no serials" for an hour.
- **This machine's shell wrapper truncates long lines through pipes.** `git show` came back 31
  lines of 603, `wc -l` under-counted, and a config rewritten with `grep -v … > file` had its
  longest line cut, which cost a run. Edit files with python, count with python.

## 6. Where the work is

| | |
|---|---|
| cubrid-testkit | PR **#9 merged** 2026-09-23. PR **#11 open**, branch `measure/where-the-time-goes` |
| cubrid-cluster-sandbox | PRs **#6 and #7 merged**; `main` is `4ba7e8c` and the submodule is pinned there |

Open questions recorded in the sandbox repo: **OQ13** whether podman is supported or was made to
work once, **OQ14** what actually holds a node in `to_be_active` — its escape is unit-tested and
has never been run end to end, because the state cannot be stood up on demand.

---

## 7. 2026-09-25 — what changed, and what to pick up next

### 7-1. Settled, do not re-open without new evidence

**The 42 the scale run called failures are fully accounted for**
([the-forty-two-and-what-they-were.md](the-forty-two-and-what-they-were.md)). Thirty-one were the
runner: 26 eight-way contention, 2 a conf key this runner never read, 3 a bound that did not exist.
The other eleven are the engine and all three of their causes were already documented.
**No new engine finding in 3,327 cases.**

**Axis O — the fleet — has its answer, and it is not about speed**
([where-the-time-goes.md](where-the-time-goes.md)). Scaling *one* category leaves this machine half
idle at twelve pairs, so speed is no argument. Running *two* categories at once lands on the sum of
their times rather than the maximum, with the whole cost on one side (4.14x) at 4% CPU — because the
contention is in what a host has exactly one of.

So **machine separation is an isolation guarantee, not an optimisation**, and buying it is the
caller's decision. Three framings were tried and discarded before that one; the note records why,
so they are not tried again. In particular: **do not give the layer knowledge of which category to
run with which.** A category is how a test is written, not what it costs, and the corpus changes.

### 7-2. Open, in the order the evidence suggests

| | |
|---|---|
| **`sql` does not run on this machine** | after the conf fix it fails compiling CQT's `TestkitExecutor` (`JunitXmlWriter` symbol). A blocker for any `sql` work here and untouched |
| **`MembersOf` counts a set by name, not liveness** | destroy leaves the describe artifact, so a set whose pairs were removed by hand still reports N members and `reuse` then runs against nothing. It fails loudly, but the message hides the cause |
| ADR-013 gate | `shell` over the whole corpus. Phase 3's exit condition, still open |
| ADR-019 | isolation controller — 27 cases whose answers record `qactl`'s ordering. Does not pass |
| two-machine baseline | ADR-022 Consequence 3 still calls it the next measurement, and `hareplsuite` is sandbox-only (`harepl.go` refuses an empty `sandbox_cluster`) |
| csb placement + netns backend | only if machine separation is chosen. The design is in [pairs-across-machines.md](pairs-across-machines.md) and §8 of the design note |
| the slot-provider seam | `internal/contain` is the only implementation and there is no seam. Everything else in the design note waits on it |

### 7-3. Traps added this round

**A reused pair is not a control, and I fell into it after writing that down.** The reproduction of
the 42 first ran on a pair worn to 35,417 pages; the same case took 639 ms on a fresh pair and
121,369 ms on that one, and the *verdict changed* with it. Stand a fresh pair up for anything you
intend to compare.

**Free disk is a performance variable.** The same eight pairs run the same work in 42 s at 90% full
and 32 s at 87%. Pairs only grow, so a machine running them walks itself into the expensive region.
`csb cluster ls`'s `DISK` column is how you see it; destroying a set is a performance action.

**Run the feature to find its bugs.** Six defects this round came out of *using* what had just been
built, not from reading it: a timeout blaming the node, `destroy --label` reporting success while
destroying nothing, a rebuild tearing down by the wrong label **and carrying on anyway**, the status
page listing nineteen destroyed clusters beside eight real ones, a probe bounded like a case, and
the `sql` guide's first example failing verbatim. None was visible in review.
