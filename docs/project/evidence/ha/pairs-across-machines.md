# Pairs across machines: csb is the abstraction, and where a node runs is behind it

- **Date:** 2026-09-23
- **Status:** a design note with two measured parts. The code claims are read from the source. The
  notebook's side was probed read-only over ssh on 2026-09-23. The contention number is from the
  eight-shard run of the same day.
- **The want:** run HA pairs with the masters on one machine and the slaves on another.
- **The frame, and it decides most of the rest:** whether a node is local or remote, **you use
  `csb`**. There is no second tool above it and no mode the caller selects. Placement is a fact
  about a node, resolved inside csb, and the surface does not know about it.

---

## Why this is worth doing now, with a number

The eight-shard measurement is in: the same 290 cases over 13 directories take **142s on one pair
and 31s on eight — 4.58x**, on a sixteen-core host running sixteen CUBRID servers.

The gap to the ideal is not a straggler and not the dealing. Read off the ledgers, per-case cost
rose **uniformly by 1.52x** under eight-way concurrency — 1.38x to 1.65x across the eight, with no
outlier:

| | control (sh11, alone) | eight pairs | |
|---|---:|---:|---|
| wall clock | 142s | **31s** | 4.58x |
| case time, summed | 111.9s | 170.5s | 1.52x |
| median case | 376ms | 578ms | 1.54x |

**That 1.52x is what machine separation would buy back**, and it is the whole of the shortfall:
without it the same deal would have finished in about 23s, or 6.2x. So the question "is this bounded
by the suite or by the machine?" now has an answer worth acting on — the machine — and a number to
beat.

*(An earlier run of this measurement read 0.86x. It was invalid: one of the eight pairs had
accumulated 127,114 log pages against the others' 500–1,800, and ran 9.83x slow. That is its own
finding and belongs in its own note; here it is only the reason these eight are freshly built.)*

## What one machine was giving for free

The current design works because a single host silently supplies six things. That list is the work.

| given for free | where it is assumed |
|---|---|
| one filesystem | `assembly.Seed` builds the slave with **`copyFile`** from the master's db directory to the slave's — a host file copy, not a node operation |
| one network | the bridge; it is why `ha_node_list` resolves |
| one witness | `NetworkGateway` — the bridge's gateway is automatically a third party |
| one author | a single csb writes **both** nodes' `cubrid.conf`, so nothing has to be agreed |
| one lifecycle | `create`/`destroy` are effectively atomic; there is no half |
| **a control channel outside the network under test** | `docker exec` does not traverse the container network |

`--network tailnet` already answers the second and third, and says so: *"On a tailnet the nodes are
members, so a topology can span machines."* It also keeps the faults intact — a tailnet address is
reached through an interface like any other, so `ip route add blackhole 100.x` and
`iptables -A OUTPUT -d 100.x -j DROP` still mean what they mean on a bridge, and still mean two
different things from each other. Group B does not have to be re-argued.

The other four are what is left.

## What follows from the frame

**The surface does not grow.** Placement appears exactly once, at `create`, next to `--backend`:
that flag already says *what* starts a node, and this says *where*. `node exec`, `fault`,
`ha status` and `describe` do not change by a character. **A node's name is its address** — which is
the decision ADR-022 already made one level up, where `ssh.host` stopped meaning "an address sshd
answers on" and became "which node this is". csb now holds that same promise inside itself.

**Local is the degenerate case of remote**, one machine in the placement. If local keeps a special
path, the abstraction is a veneer and will drift apart. csb has shown this discipline before: the
tailnet is *"an option and not a backend"*, changing four of the eleven backend operations and
leaving seven alone.

**The delegating backend is the mechanism, and it is not a new concept.** "Do this to a node" is
already a contract — csb's ADR-002, eleven operations — and reaching a node on another machine is
another implementation of it, carried over the `csb/v1` envelope that is already a stated contract.
csb consumes csb, the way testkit consumes csb: drive the tool, ingest the artifact, link nothing.
csb's own rule is that a second implementation is when the Go interface gets declared.

**One `describe`, or it is two clusters.** The artifact is what evidence names, so the merge of two
halves is internal and the document stays single. A half-up cluster is a note on one object, not a
new kind of object; `cluster ls` already has the vocabulary for it in `stale_state`.

## What goes inside `create`, and must not become a verb

Three things genuinely have to be built. None of them is allowed to surface.

1. **Agreement before start.** Today there is one author, so `ha_node_list`, the db name, addresses
   and engine parameters need no negotiation. With two machines they must be fixed *before either
   half starts*. `create` becomes plan → each machine applies its own part → the halves are merged
   into one artifact.
2. **A direct transfer between the machines.** Seeding is a file copy across a boundary that is no
   longer one filesystem, and ADR-022 Consequence 6 already refused to move files through the exec
   channel on purpose: *"Base64 through `node exec` would work until the argument list stopped
   fitting, which is worse than not having it."* So the two csb move the bytes to each other. The
   moment they travel through the invoking terminal, the abstraction has leaked.
3. **Placement in the topology**, so a node carries which machine it is on and every later operation
   resolves it without being told.

## The invariant: the control channel must not ride the plane being cut

This is the one that breaks if it is treated as advice.

`fault partition` is a cut expressed against tailnet addresses. If csb reaches the far half **over
the tailnet**, then severing the pair severs csb's own ability to command that half — the second
half of a split-brain cannot be issued, and `fault clear` cannot undo it. On one host this never
arises, because `docker exec` is out-of-band from the bridge by construction.

Under this frame that is **not an operator's responsibility**. A tool whose fault verb can cut its
own hands off is defective, not misconfigured. So: **control out-of-band (ssh over the ordinary
network), cuts expressed on tailnet addresses only.** csb must guarantee the separation it used to
get for nothing.

The witness is the same species of problem, and csb's source already has the answer written down —
*"for a cluster spanning machines it should be a third machine rather than either of the two"* — so
a two-machine cluster has to be given `--ping-host` rather than defaulting to this host.

## The cost that does not disappear

The surface can be kept identical. **The error model cannot.** A local `node exec` effectively never
produces "it ran but the answer was lost"; a remote one does. For a read that is a retry. For a
mutating verb — `fault partition`, `ha resync` — it is a real ambiguity.

Exposing it would mean the caller distinguishing local from remote, which is exactly the simplicity
this frame is protecting. So the price is paid the other way: **the verbs become idempotent and
checkable** rather than the difference becoming visible. This is not "it can be hidden"; it is "it
can be hidden, and this is what that costs".

## The backend is a separate axis, and it is what the notebook needs

Probed over ssh on 2026-09-23, read-only:

| | |
|---|---|
| ssh, key-based, `BatchMode` | **yes** — `hgryoo-notebook`, sshd active |
| cores / memory | 16 / 28 GB, 15 GB available — the same core count as this host |
| `/home/hgryoo/CUBRID` | **present, and the same build**: `11.5.0.2513-5f3a30d`, `Sep 21 2026 14:59:03`, identical to this host's, checked with `cubrid_rel` on each |
| tailnet | member, `100.97.197.13`, direct rather than relayed |
| docker / podman | **neither is installed** |
| glibc | 2.43 |

So the engine question is already settled, and the missing container engine is **not** a reason to
reach for a different tool. It is a missing *backend* — which is the axis orthogonal to placement.
ADR-022 Consequence 5 already describes the one that fits: a netns backend gives *"HA testing on a
machine with no Docker"*, two network namespaces in one unprivileged user namespace reaching each
other over a veth, cut by a blackhole route, with a witness outside the pair surviving the cut —
csb's ADR-002 operations 2, 3, 4 and 8, with no root and no daemon.

`--backend netns` plus placement puts a node on the notebook with the surface unchanged. That is the
frame doing its job: a machine's limitation is answered inside csb, not around it.

## The other path, recorded as what it is

ADR-022 kept a second path open, and it needs none of the above: **testkit driving a hand-built pair
over ssh**, through `relatedhosts`, as CTP has always run the frozen corpus. Its parts exist —
`exec.SSH` is one of the channels beside `Local`, the slot's and `csb node exec`; `internal/topology`
parses `env.instance1.master.ssh.host` and was widened in Consequence 4 to carry roles its fixed list
did not name; the notebook has sshd and the matching engine tree. The single gap is in this
repository: `hareplsuite` is sandbox-only by construction — *"runs ha_repl against a topology
cluster-sandbox provides"* — and `harepl.go:94` refuses a run whose `sandbox_cluster` is empty.

It is the smaller change, and ADR-022 Consequence 3 says it is owed anyway: *"The two-machine
baseline is now the next measurement … it is the only path that runs the frozen corpus without
waiting on anything, and every later comparison against a sandbox pair needs it first."*

**But it is a second surface.** Under the frame at the top of this note that is its weakness, not
its strength: a run reaching nodes through csb and a run reaching them through ssh are two ways to
say the same thing, and the evidence they produce is described by two different artifacts. Worth
doing for the baseline it owes; not worth mistaking for the answer to machine separation.

## What it would settle

Whether the 1.52x is contention for this host's cores. Masters here, slaves on the notebook, the
same 290-case slice: if the per-case cost falls back toward the control's, the bound was the
machine, and the eight-shard number has room in it. If it does not, the suite is the bound and the
csb placement work buys less than it costs.
