# Pairs across machines: what already works, what is missing, and why it is worth doing

- **Date:** 2026-09-23
- **Status:** a context note, not a finding. Nothing here was measured; it is what the code says
  and what the next person needs so they do not rediscover it.
- **The want:** run several HA pairs with the masters on one machine and the slaves on another —
  eight pairs over two machines rather than sixteen containers on one.

---

## Why this came up

An `ha_repl` run now drives several pairs at once (`sandbox_cluster=a,b,c`, one process, a lane and
a pair panel each). On this host that means sixteen CUBRID servers on sixteen cores, and the first
question about any speed-up measured there is whether it is bounded by the machine rather than by
the suite. Splitting the pairs over two machines answers it, and it is also the shape a real HA
test wants: replication between two hosts is the thing, and a pair on one host is the convenience.

## What already works

**The runner does not know where a node is, and that is deliberate.** It names a cluster and
reaches nodes with `csb node exec <node>`; ADR-022 is where that line was drawn, and the phrase
there is that `ssh.host` stops meaning "an address sshd answers on" and starts meaning "which node
this is". So a cluster whose nodes sit on two machines needs **no change in testkit at all** — the
sharding committed today would drive it as it stands.

**cluster-sandbox has the network for it.** `--network tailnet` exists and says why, in
`internal/backend/tailnet.go`:

> *"What it buys is the one structural limit this tool has: `ha_node_list` works today because both
> containers sit on one host's bridge. On a tailnet the nodes are members, so a topology can span
> machines."*

**And the faults keep their meaning.** The same file is explicit that the cut is unchanged on a
tailnet: a tailnet address is reached through an interface like any other, so
`ip route add blackhole 100.x` and `iptables -A OUTPUT -d 100.x -j DROP` still mean what they mean
on a bridge, and still mean two different things from each other. Group B does not have to be
re-argued for a two-machine pair.

## What is missing

**Placement.** `csb cluster create` has `--network bridge|tailnet`, `--nodes`, `--ping-host`,
`--ts-authkey` — and nothing that says *which machine a node runs on*. A tailnet makes the nodes
members of one network; it does not start a container anywhere but the host the command was run on.

So the gap is one thing: **`create` has to be able to start some nodes elsewhere.** Two shapes, and
this note does not pick between them:

| | |
|---|---|
| **remote engine** | csb talks to the other machine's container engine (`DOCKER_HOST`/`CONTAINER_HOST` over ssh; `--backend` is docker here) and starts the node there. One artifact, one `csb`, the topology stays one object. Nothing in csb reads either variable today, so this is the larger of the two. |
| **two csb, one artifact** | each machine builds its own half and the two are joined into one described cluster. Cheaper to start, and it makes `describe` a merge — which is the thing ADR-022 says the artifact is for. |

The second is probably closer to how this tool already thinks: `describe` is *"the reproducible
artifact"*, and a cluster that was assembled from two halves is still one artifact if the merge is
the thing that writes it.

## What to check before building either

1. **`ha_node_list` and the addresses.** The engine's HA group is configured with host names; on a
   tailnet those are tailnet names. The conf the nodes get is written by csb, so this is csb's to
   get right, but it is the first thing that will be wrong.
2. **The ping witness — csb has already decided this, and the default is wrong for two machines.**
   `--ping-host` is the witness that lets a partitioned node tell "the peer is gone" from "I am
   gone", and the split-brain flavour this suite measured (`ping-survives`) depends on it surviving
   the cut ([`split-brain-divergence-converges.md`](split-brain-divergence-converges.md)). On a
   tailnet csb defaults it to *this host* — a third member, which a cut between two nodes does not
   touch. With the slaves on another machine that host is no longer a disinterested third party,
   and `internal/cli/cluster.go` says so outright: *"for a cluster spanning machines it should be a
   third machine rather than either of the two"*. So a two-machine run must pass `--ping-host`
   explicitly, and the note to write down is which third machine.
3. **The build.** Both machines must run the same engine. Today the install tree is bind-mounted
   read-only from the host (`engine.path`, `/home/hgryoo/CUBRID` → `/opt/cubrid-ro:ro`), so a second
   machine needs the same tree at a path of its own. The artifact's `engine.build` is what proves
   they match — `11.5.0.2513-5f3a30d` on these pairs today — and a two-machine `describe` has to
   carry one per half, or it is claiming something it did not check.
4. **Who may kill what.** A sandbox node's processes are the invoking user's processes on the
   machine they run on. The four mass kills of 2026-09-22 came from a CTP tree on this host
   (`where-this-stands.md` §5); a second machine adds a second place that can do it, and the
   watcher this session wrote only watches this one.

## What it would settle

The measurement that prompted it: a 64-case slice, one pair against eight, on this host. Whatever
that says, the same slice with the slaves on `hgryoo-notebook` says whether the answer was about
the suite or about one machine's sixteen cores. The two-machine pair in
[`where-this-stands.md`](where-this-stands.md) §3 is already up and tailscale-connected, which is
the environment half of it.

It would also close the gap ADR-022 left open on purpose: the sandbox path was justified as
"a pair, cheaply, on one machine", and the thing it could not do was the thing HA is actually
about.
