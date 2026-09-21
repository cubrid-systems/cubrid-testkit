# The HA baseline — what CTP's HA shell does on this engine

- **Status:** **not measured.** This directory is the harness and the reason; the numbers are not
  here yet because the pair is not configured.
- **What it is for:** the 373 frozen HA cases, run by CTP, on two machines, so that everything after
  it has something to be compared against. ADR-015's second criterion — parity before beyond — and
  `design/module-ha.md` §5 both land on this being owed first.

---

## Findings from this directory

- [object-domain-not-replicated](object-domain-not-replicated.md) — **2026-09-21.** A column whose
  type is another class holds a value on the master and a stored NULL on the slave. Both rows
  arrive, both tables have primary keys, `fail_counter` does not move and the applier logs nothing.
  Isolated to four statements; the source explains it (replication carries the primary key and the
  slave rebuilds the row from the master's heap image, in which an object reference is an OID).

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
