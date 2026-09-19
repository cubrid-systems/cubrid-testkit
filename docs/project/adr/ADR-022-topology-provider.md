# ADR-022: Who stands the topology up

- **Date:** 2026-09-20
- **Status:** **Accepted** (2026-09-20)
- **Trigger:** HA needs a master and a slave, and nothing here could make one
- **Related:** ADR-014 (one machine) · ADR-013 (the HA tree is excluded from the evidence) ·
  ADR-001 Consequence 4 (how an extension integrates) · ADR-003 (the frozen surface) ·
  `cubrid-cluster-sandbox` ADR-002 (what a backend has to provide) ·
  `evidence/ha-topology.md` (what was measured)

---

## Context

Two tasks have never run here. `ha_repl` is still CTP's, and the HA shell tree — 367 cases — is
excluded from ADR-013's evidence with the reason written down: *"Needs a master/slave topology that
does not exist yet."* `DeployHA` was listed in `design/module-shell.md` as ported and unverified,
and was not ported at all; `topology.SSH.Related` parsed `relatedhosts` and no code read it.

So the question was never *whether* to support HA. It was who makes the pair.

ADR-014 already answered it, and in a sentence written before there was anything to apply it to:

> **HA is not an exception.** `ha_repl` needs a master and a slave, and that looks like a fleet. It
> is not. **The system under test has a topology; the runner does not have a fleet.**

That forbids the obvious move — teaching this runner to provision nodes — without saying what to do
instead. What changed is that something else now does it. `cubrid-cluster-sandbox` stands a
multi-node CUBRID topology up from one command and a build, and it had already written down the
relationship from its side: *"cubrid-testkit consumes this tool: it owns suites, dispatch and
reporting; this project owns the environment they run on."*

## Options

1. **Provision here.** A `contain` slot already has its own network namespace, so a veth between two
   slots would be a pair. Measured and it works (`evidence/ha-topology.md` §4). Rejected anyway: it
   is the fleet ADR-014 excluded, arriving through the back door, and it would put node lifecycle,
   addressing and fault injection in a runner whose job is to find cases and judge them.
2. **Require two machines, as CTP does.** Correct, and it still is for the frozen corpus (§Decision
   below). Rejected as the *only* answer: it puts HA testing behind hardware, which is why none of
   it runs in CI today.
3. **Consume a provisioner.** The topology is asked for. Chosen.

## Decision

**A topology is asked for, never built here.** `cubrid-cluster-sandbox` is pinned at
`extensions/cluster-sandbox` and consumed as a subprocess.

**The form is ADR-001 Consequence 4** — drive the tool, ingest the artifact, link nothing. Two
reasons and either alone decides it: the submodule has to be able to move without this repository
recompiling, and csb's `--json` envelope is a stated contract where its Go packages are not.

**The artifact is the CTP fragment, not the JSON describe.** `csb cluster describe --format ctp`
renders the same topology as `env.instance1.master.ssh.host`, `.slave.ssh.host`, `ha_db_list` and
the engine parameters — CTP's frozen key names (ADR-003), which this runner already parses and
already turns into `topology.Instance`. No parser and no second model of what a node is. What
changes is the transport, and csb says which in the note it attaches to that output: the nodes run
no sshd, so the frozen key names are filled for an exec channel. `ssh.host` keeps its meaning of
*where this instance is* and stops meaning *an address sshd answers on*.

**The transport is a fourth `exec.Channel`.** `internal/sandbox.Node` runs `csb node exec`, beside
`Local`, `SSH` and a slot's. The suites do not learn a fourth way to run something (contract C3).
It does not prepend `exec.Profile` the way `SSH` does, and that is measured rather than assumed: the
engine's environment is already set on the node, so sourcing a host's `~/.bash_profile` would import
the wrong machine's setup rather than the missing one.

**`csb` gets no E number.** E1–E10 are testing capabilities; this is an environment provider.
Numbering it would put infrastructure into the ladder that orders them and hide that it is a shared
dependency of at least three of them — ROADMAP records the overlap with E4, E7 and ladder rank 6 —
as well as of two strangler-fig tasks. The registry entry is in `category/extensions/README.md`
under a heading that says it is not one of them.

**Two paths stay open, and the frozen corpus uses the older one.** The HA shell cases shell out to
CTP's Java SSH helper to reach their slave, so a node has to be a QA machine: sshd with password
authentication, a JVM, `expect`, and CTP's tree. A sandbox node is none of those
(`evidence/ha-topology.md` §3). **So the 367 frozen cases run on two machines, through
`relatedhosts`, as CTP has always run them** — and the sandbox carries `ha_repl` and everything
built after it, where the runner drives both nodes from outside and none of that is needed.

## Consequences

1. **The exit condition moved, and the reason is recorded.** This ADR was going to be proven by *an
   HA case passing on a sandbox pair*. It cannot be, and not because the seam is wrong: the seam
   carries a `CREATE`, an `INSERT`, and the row read back off the slave through its own channel
   (`evidence/ha-topology.md` §1). It is the cases that need a machine the nodes are not. The seam
   is proven by that measurement; the case waits on a node flavour.
2. **One request goes to `cubrid-cluster-sandbox`, with four parts**: sshd, a JVM, `expect`, and a
   second read-only mount for a CTP tree. Its own ADR-002 shows the shape — a second recipe rather
   than a flag on the first, because the image tag is the recipe's hash and two shapes must not
   collide.
3. **The two-machine baseline is now the next measurement**, and it was always going to be: it is
   the only path that runs the frozen corpus without waiting on anything, and every later comparison
   against a sandbox pair needs it first.
4. **`internal/topology` grew a defect fix on the way in.** It read only the nine roles a fixed list
   named, so `env.instance1.slave2.*` was dropped whole — and CTP's own `conf/ha_repl.conf`
   documents that shape: *"A master can have multiple slaves, for instance, slave1, slave2"*. A role
   the keys mention is now carried whether or not the list names it.
5. **A second backend for csb is unblocked but not owed here.** A netns backend would give HA
   testing on a machine with no Docker, and its premise holds — two network namespaces in one
   unprivileged user namespace reach each other over a veth, a blackhole route cuts them, a witness
   outside the pair survives the cut. That is csb's ADR-002 operations 2, 3, 4 and 8 with no root and
   no daemon, and csb's own rule says a second implementation is when the Go interface gets declared.
6. **`Put` and `Get` are refused on this channel rather than approximated.** csb has no
   file-transfer verb; it has a host-side path (its ADR-002 operation 11). Base64 through `node exec`
   would work until the argument list stopped fitting, which is worse than not having it. Nothing
   here calls them.

## Alternatives considered

**Read csb's JSON describe instead of the CTP fragment.** Richer, and it is the artifact csb
considers primary. Rejected: it would mean a second reader and a second model of a node, to arrive
at the same two names this runner already knows how to hold. The fragment is a rendering of the same
artifact — csb writes it from the same describe, so a cluster cannot describe itself one way to a
reader and another way to a harness.

**Vendor nothing and require `csb` on PATH.** It is what the integration actually needs: the
submodule is not compiled against. Rejected because the pin is the point — evidence produced here
names which revision of the provisioner produced the topology, and a tool found on PATH names
nothing.

**Put the HA assets into the node image so the frozen corpus runs on a sandbox pair.** This is
consequence 2, deferred rather than rejected. It is the right eventual answer; it is not the right
first one, because the baseline it would be compared against does not exist yet.
