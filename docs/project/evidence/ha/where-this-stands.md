# HA: where this stands, and the four ways on

- **Date:** 2026-09-22
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
| `wait_timeout` / `case_failed` | the run, not the case |

**Four things are known not to replicate**, and a read touching one is skipped rather than
reported: a table with no primary key, a view onto one, a synonym for one, and an object-domain
column ([`object-domain-not-replicated.md`](object-domain-not-replicated.md)). The first is the
rule the other three are consequences of.

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

**A two-machine pair** — `hgryoo-desktop` (master) and `hgryoo-notebook` (slave), over tailscale,
which is the only thing CTP can run on. As of 2026-09-22 it is **up, HA-configured by CTP, with a
database `xdb`**, and the three conf files on both machines were rewritten by CTP's deploy.
Backups of the originals are in this session's scratchpad; if they are gone, the machines are
still working pairs and the conf is CTP's rather than the operator's.

`./preflight.sh <master> <slave> <user> [<password>]` checks in seconds whether a pair can host
CTP's corpus at all. It reported READY on this one.

## 4. The four ways on — A settled 2026-09-22, three open

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

### B. Scale — `_01_object`, 3,327 cases

Now worth running: the suite is reproducible, nothing comes back `no_data`, and the conversion
adds no refusals. Three to four hours at the current rate; run it in the background.

What it would answer: whether the four known non-replicating shapes are the whole list, or
whether a fifth is waiting in a corpus twenty-five times larger than anything run so far.

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

### D. Tidy up

The two-machine pair can be torn down and its conf restored, or left as the only CTP-capable
environment there is. It is the left-hand side the owed baseline needs, so leaving it is
defensible.

## 5. Traps, each of which cost real time

- **CTP's cleanup used to `kill -9` every process the invoking user owns**, on the master, which
  on a workstation is the operator's session. Removed from `util_common.sh` on both machines of
  the pair, along with the two helpers that existed only to serve it. **Any third machine's CTP
  tree still has it.** `upgrade.sh` was verified not to restore it.
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
- **This machine's shell wrapper truncates long lines through pipes.** `git show` came back 31
  lines of 603, `wc -l` under-counted, and a config rewritten with `grep -v … > file` had its
  longest line cut, which cost a run. Edit files with python, count with python.

## 6. Where the work is

| | |
|---|---|
| cubrid-testkit | PR **#9**, branch `feat/ha-repl-sandbox` |
| cubrid-cluster-sandbox | PR **#6**, branch `feat/podman-backend` — **merge first**, the submodule pointer is on it |

Open questions recorded in the sandbox repo: **OQ13** whether podman is supported or was made to
work once, **OQ14** what actually holds a node in `to_be_active` — its escape is unit-tested and
has never been run end to end, because the state cannot be stood up on demand.
