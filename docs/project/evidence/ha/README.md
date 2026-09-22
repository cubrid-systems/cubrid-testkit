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

## Why this is the first measurement

Three separate pieces of work are waiting on one number nobody has.

1. **P1, the sleep patches.** 244 of 373 cases synchronise with `sleep`, 20,370 seconds of it. The
   patches that replace those with `wait_for_slave` are writable today — but *"no verdict changed"*
   is the whole claim, and there is nothing to compare against.
2. **ADR-022's sandbox path.** Whether a pair provisioned by `cluster-sandbox` produces the same
   verdicts as two machines is a comparison, and it needs the left-hand side.
3. **Group B at all.** A fault-injection case is a claim about behaviour under a disturbance, and a
   corpus whose ordinary behaviour is unrecorded cannot support one.

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
