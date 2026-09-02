# ADR-014: One machine is the runner's scope

- **Date:** 2026-09-03
- **Status:** Accepted
- **Supersedes in part:** `design/module-shell.md` §5-6, which treated multi-machine dispatch as
  axis T
- **Related:** ADR-004 (first replacement candidate), `concept/migration-exclusions.md` (the axis
  split), `evidence/regression-shell.md` (how this was noticed)

## Context

The project separates two axes. **Axis T** is test execution: find the cases, run them, judge them,
record what happened. **Axis O** is QA operations: when to run, who to tell, what to file. Axis O is
excluded from the migration and rebuilt later as a separate layer.

Scheduling, mail and issue filing were excluded on that basis. Reaching machines over SSH was not —
it was carried over as axis T because CTP's shell module does it and because a case has to run
somewhere.

That was the wrong call, and it was wrong for the same reason the others were right: CTP put
`default.ssh.pwd` in the same properties file as `testcase_timeout_in_secs`, and sharing a file was
mistaken for sharing a purpose.

### What the evidence says

**Nothing CTP ships configures a fleet.** Of the fourteen configuration files in `conf/`, the SSH
keys that appear are placeholders — `env.instance1.master.ssh.host=<master ip>` — or commented out.
`shell.conf` has three live keys and none of them names a machine. The `default.*` keys that are
real are engine parameters: ports and shared-memory ids. **CTP ships as a local runner. The fleet is
what a QA team layers on top**, which is the definition of axis O.

**The first side-by-side run could not be done at all** (`evidence/regression-shell.md`). Not
because the test logic was wrong, but because the runner assumes it owns the machine: it kills every
process the user has, deletes `$CUBRID`, and requires `~/.bash_profile` to be arranged a particular
way. On the machine at hand that meant 63 processes and 13 shared-memory segments belonging to other
work. The run needed a PID and IPC namespace before it could start.

That is the tell. **Handing the runner an isolated machine is the operations layer's job.** Doing it
by force, to a shared machine, is what a fleet manager does — and building a namespace to contain
the damage was standing in for an operations layer that does not exist yet.

## Decision

**The runner runs on one machine.**

| | |
|---|---|
| **Axis T** | "Run this command where the database is." One `Channel` (contract C3). The machine may be this one or a remote one; either way it is *one* |
| **Axis O** | "Here are eight machines, their hostnames and passwords. Spread 3,452 cases across them, install the build on each, take one out when it stops answering." That is a fleet |

Concretely:

- **Local is the default.** A configuration that names no machine runs here, which is already what
  CTP does (`Context` adds an environment called `local`).
- **`exec.SSH` stays, demoted.** It is one implementation of `Channel`, for when the one machine is
  a remote one. It is no longer the runner's reason for existing, and it is not on the default path.
- **A configuration naming several instances uses the first, and says so.** Ignoring the rest costs
  throughput, not correctness, so under the config-key policy it warns rather than fails
  (`migration-exclusions.md` §2a).
- **The fleet is recorded as excluded and deferred:** the instance inventory, deployment across
  several machines, dispatching cases between them, and adding or removing a machine mid-run.

### What stays

`Channel` earns its place even in a runner that only ever runs locally: it is what lets the same
case logic drive a local unittest and a case shipped elsewhere, and it is the seam the tests hold.
Discovery, the dispatch queue, the worker loop, the verdict, the result files and the feedback
backends are all axis T and unaffected.

Applying engine parameters (`ini.sh` into `cubrid.conf`) stays too, but its reason changes: it is
**configuring the engine under test**, not provisioning a fleet. It applies to one machine.

### HA is not an exception

`ha_repl` needs a master and a slave, and that looks like a fleet. It is not. **The system under
test has a topology; the runner does not have a fleet.** The runner still runs in one place and
drives a database that happens to span nodes — the same way a case that talks to a broker on another
host is still one case running in one place.

## Consequences

1. **The runner gets smaller and the operations layer gets a clearer brief.** ADR-012 (the QA
   operations layer) inherits the fleet along with the scheduler, the mailer and the issue filer.
2. **`evidence/regression-shell.md`'s namespace becomes a statement about the old design, not a
   workaround for the new one.** A runner scoped to one machine is a runner you can give a container
   to.
3. **The 3,452-case exit corpus takes longer on one machine.** That is the cost, and it is an
   operations problem with an operations answer: give the runner more machines by running more
   runners, rather than teaching one runner to manage machines.
4. **`exec.SSH` is written and tested but off the default path.** That is deliberate. Deleting
   verified work to make a diagram tidy would be worse than leaving it where the one remote machine
   can use it.
5. **`docs/design/module-shell.md` §5-6 is superseded** where it treats per-environment workers as
   axis T.

## Alternatives considered

**Keep SSH and the fleet as axis T.** It is what CTP does in the shell module, and it is what the
analysis assumed. Rejected: the shipped configuration does not support the claim, and carrying it
would keep the fused design the project exists to take apart.

**Exclude SSH entirely; local only.** Cleaner, and it would have made this ADR shorter. Rejected
because "one machine" and "this machine" are different claims, and the second one is a restriction
nothing in the freeze requires. A remote single machine is still one machine.

**Defer the decision to an ADR and keep building.** Rejected: every further slice would have been
written against the wrong boundary, and the boundary is the point of the project.
