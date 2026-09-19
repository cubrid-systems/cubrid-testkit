# What a sandbox pair gives, and what the HA assets still want

- **Date:** 2026-09-20
- **What this is:** a two-node HA cluster stood up by `cubrid-cluster-sandbox`, driven through
  `internal/sandbox`, and then measured against what CTP's frozen HA assets require. The first half
  works. The second half is a list of four things, and each one is a sentence rather than a guess.
- **Trees:** engine `cubrid-develop/install.out` — CUBRID 11.5.0 (11.5.0.2513-5f3a30d) ·
  `cubrid-cluster-sandbox` `3081496` · CTP `cubrid-testtools` as checked out here.
- **Backend:** docker. The netns backend does not exist yet (§4).

---

## 1. The pair, and the runner driving it

```
cluster tkha: 2 node(s) on tkha-net, state serving
  tkha-n1   master   registered_and_active
  tkha-n2   slave    registered_and_standby
```

About fifty seconds from nothing, most of it the engine. `internal/sandbox` then reads
`csb cluster describe --format ctp --instance instance1` and runs cases on the nodes it names.

Seven checks, all passing (`internal/sandbox/integration_test.go`, skipped without
`TESTKIT_CSB_CLUSTER`):

| | |
|---|---|
| the fragment reads as a pair | master `tkha-n1`, slave `tkha-n2`, db `tkha` |
| a command runs on the master | `cubrid_rel` answers with the build above |
| the engine environment is there | `CUBRID=/work/tkha-n1/cubrid`, `CUBRID_DATABASES=/db` |
| a multi-line script runs whole | `cd /tmp; pwd; echo two` → `/tmp two` |
| a failing command is data | exit 7 in the `Result`, both streams kept |
| the slave is a different node | `hostname`: `tkha-n1` against `tkha-n2` |
| **the slave has what the master wrote** | a `CREATE` and an `INSERT` through the channel, read back through the slave's |

The last one is the claim the integration exists to make: **a topology provisioned elsewhere that
this runner can drive.** The read polls rather than sleeping — csb has `repl check` for that
question and it is the right verb to reach for later, but a fixed sleep would have made it pass for
the wrong reason.

**What this does not yet show** is an HA *case* passing, which is what ADR-022 set as the exit for
this step. Section 3 is why.

## 2. What the pair already satisfies

Measured on the node, through the same channel:

| | |
|---|---|
| the peers resolve each other | `getent hosts tkha-n2` → `172.21.0.3` |
| the peers reach each other on the engine port | `tkha-n2:31523` open from `tkha-n1` |
| the engine's configuration is writable | `$CUBRID/conf/` takes a file — `modify_cubrid_conf` and its five siblings rewrite exactly this |
| `csql`, `cubrid`, `awk`, `sed` | present |

The port is **31523**, not the shipped 1523: csb moves it, and `DeployHA` reads
`cubrid_port_id` out of `$CUBRID/conf/cubrid.conf` rather than assuming, so nothing in the HA
assets conflicts with that. The same is true of `ha_port_id`, which is 59901 here and is the
default `DeployHA` would have written anyway.

## 3. What the HA assets still want — four things

CTP's HA shell corpus is 367 cases (ADR-013), and none of them sets up HA itself. They call
`$init_path/make_ha.sh`, which reads `$init_path/HA.properties` and then hands the work to
`make_ha_upper.sh`. Reading those three files says exactly what a node has to be:

**(a) An sshd on the slave, with password authentication.** `make_ha.sh` aliases `run_on_slave`,
`run_upload_on_slave` and `run_download_on_slave` to `common/script/run_remote_script`, and that
script is a wrapper around a **Java class** — `com.navercorp.cubridqa.common.RunRemoteScript` out of
`cubridqa-common.jar` — which opens an SSH session with `-host`, `-port`, `-user`, `-password`.
Measured: `tkha-n2:22` is closed, which is what csb says of itself ("these nodes run no sshd and
publish no port"). **Not negotiable from this side**: the corpus is frozen (NG1), so the case cannot
be taught a different way to reach its slave.

**(b) A JVM on the master.** Same reason: the helper the case shells out to is Java. Measured: no
`java`, `JAVA_HOME` unset.

**(c) CTP's tree on both nodes.** `$init_path` *is* `CTP_HOME/shell/init_path`, and the properties
file `DeployHA` writes lives in it. Measured: `CTP_HOME` unset, no tree. This one is the cheapest of
the four — csb already bind-mounts the engine read-only, and a second read-only mount is the same
mechanism.

**(d) `expect`.** `make_ha.sh`'s own header requires `hostname.exp`, `rm_db_info.exp`, `scp.exp` and
`start_cubrid_ha.exp` beside it. Measured: no `expect`, no `ssh`, no `scp`.

### What that adds up to, and the decision it forces

Three of the four turn a csb node from *a runtime for the engine* into *a QA machine*. That is a
real change to what the tool is for, and it belongs to cluster-sandbox rather than here — its
ADR-002 §5 says there is a base image and never an engine image, and its tailnet option is the
precedent for how a second shape gets added: **a second recipe, not a flag on the first**, because
the image tag is the hash of the recipe and the two must not collide.

So the request to that project is one thing with four parts: **a node flavour that can host the
frozen HA shell assets** — sshd with password auth, a JVM, `expect`, and a second read-only mount
for a CTP tree. A cluster that does not ask for it builds none of it.

**And it is worth saying what does not need any of this.** `ha_repl` is not the shell corpus. Its
oracle is master-against-slave rather than an answer file, and the runner drives both nodes from
outside — which is exactly what `internal/sandbox` already does, and what §1 measured working. The
two halves of "HA testing" have different requirements, and only one of them is blocked here.

## 4. Two environments, and why the list is the same for both

The machines are available for HA, so the pair above is not the only way to run these. What §3 asks
for does not change between them:

| | how the pair is made | what §3 costs there |
|---|---|---|
| this machine, docker | `csb cluster create` | the four things above |
| this machine, namespaces | a netns backend — **not built** | the same four |
| two machines | `env.instanceN.ssh.relatedhosts`, as CTP has always done | **nothing** — a QA machine already has all four |

That last column is the useful one. **The two-machine path is the one that can run the frozen corpus
today**, and it is where the baseline should be measured: what CTP's HA shell actually does on this
engine, which nobody here has yet. Bringing it onto a sandbox pair afterwards is a question of
whether the verdicts match, and that question needs the baseline first.

The netns backend is unaffected by any of this. Its premise was measured separately and holds: two
network namespaces inside one unprivileged user namespace reach each other over a veth pair, a
blackhole route cuts them and a witness outside the pair survives the cut — ADR-002's operations 2,
3, 4 and 8, with no root and no daemon. What it buys is HA testing on a machine with no Docker, and
it does not make the four things above any cheaper.

## 5. What this changes

1. **ADR-022's exit condition was "an HA case passes on a csb pair", and it cannot be met yet.** Not
   because the seam is wrong — §1 shows it carrying real work — but because the cases need a machine
   the nodes are not. The exit moves: the seam is proven by §1, and the case is proven when a node
   flavour exists.
2. **The two-machine baseline comes first.** It is the only path that runs the frozen corpus without
   waiting on anything, and every later comparison needs it.
3. **`ha_repl` is not blocked.** It can be built on `internal/sandbox` as it stands.
