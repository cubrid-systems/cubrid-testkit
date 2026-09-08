# Axis B — the register

- **Started:** 2026-09-03
- **What this is:** every place this system should be better than CTP rather than equal to it, what
  it improves on, and what would show it worked.
- **Rules:** ADR-015. Four admission criteria, all four required. An entry not written here is not
  on the axis.

Axis T and axis O are **rewrites that preserve compatibility** — done when the frozen surface holds
and the diff is zero. Axis B has nothing to be compatible with, so every entry carries its own
evidence.

**Nothing here ships before the T or O item it improves on has passed its own gate.** That is not
ceremony: equivalence cannot be proven against a system that has already been improved.

---

## Status key

| | |
|---|---|
| **shipped** | already done, and why it could not wait |
| **ready** | the parity it improves on has passed its gate; may be built |
| **blocked** | waiting on a named T or O item |
| **idea** | admitted, not yet sized |

---

## B-T — beyond, on the test-execution axis

### B-T1. Reproducible dispatch — **shipped**

| | |
|---|---|
| Improves on | T: `Dispatch.findAllTestCase` |
| Today was | `dispatch_tc_ALL.txt` records the case list in `find` order, which is `readdir` order. Three consecutive runs over the same unchanged tree gave three different lists |
| Now | discovery sorts |
| Evidence | two runs of the same suite produce the same file; `TestDiscoverIsIndependentOfFindOrder` |
| Why it did not wait | **there was no parity to ship first.** CTP cannot reproduce this file against itself, so F1 was not a grade anything could earn. Recorded as a deviation in `evidence/spec-corrections.md` and masked in ADR-013 |

### B-T2. A runner you can hand a container — **blocked**

| | |
|---|---|
| Blocked on | T: the shell task passing the full-corpus gate (ADR-013) |
| Improves on | T: `Constants.createLinKillScripts`, and the posture behind it |
| Today | the reset before every case matches substrings across everything the user owns. On the machine used for `evidence/regression-shell.md` that was 63 processes and 13 shared-memory segments belonging to other work. **A CTP run and anything else the same user is doing cannot share a machine** |
| Beyond | the runner runs inside a PID and IPC namespace it creates for itself, so the reset reaches its own work and nothing else |
| Evidence | the full suite runs on a developer's machine while that developer keeps working; the reset script is unchanged, and `ps -u $USER` inside the namespace shows only the run |
| Note | the wrapper already exists — `regression-shell.md` §1 had to build it before either runner could be measured. It is a script beside the evidence, not part of the runner. **This entry is about making it the runner's own behaviour**, which ADR-014 made possible by scoping the runner to one machine |
| Also in scope | **fixing the JVM sweep** (freeze §11-23). `[ $isExistPid -eq 0]` has no space before the bracket, so the test is a shell syntax error, the branch is never taken, and CTP has never killed a stray JVM through it. Fixing it alone would widen what the runner kills, which is the direction ADR-014 moved away from; inside a namespace "every JVM the user owns" *is* "every JVM this run started", and the sweep does what it was written to do |
| Why the coupling gets tighter | the sweep spares CTP's own JVM by matching `com.navercorp.cubridqa\|service.Server`. **When the migration finishes there is no CTP JVM to spare**, so that filter matches nothing and the fixed sweep becomes an unconditional "kill every JVM". The containment is not a nicety that could be added later -- it is what the fix depends on, and it depends on it more as the port progresses |
| Also in scope | **the network.** The wrapper isolates mounts, pids and IPC and not ports, so a run's `cub_master` and brokers compete with everything else on the box for them -- 47 listening sockets on the machine this was measured on, against 0 inside an added `unshare --net` with `lo` up. That is a live source of verdict differences: broker cases fail when a port is held and pass when it is not. Unprivileged and cheap, and nothing in the corpus reads what it would change (`TEST_SSH_HOST` is read by no case, and by CTP only to name a host in a core-backup message). **Deliberately not done yet**: ADR-014 scopes the runner to one machine, and which resources a run may share with that machine is this entry's question rather than the runner's |
| Also in scope | **reaping.** A container puts the runner at PID 1 with no shell above it; see the note below |

**The design, and the constraint that shaped it (2026-09-07).** Prototyped in Go:
the runner re-executes itself with `CLONE_NEWUSER|CLONE_NEWNS|CLONE_NEWPID|
CLONE_NEWIPC`, remounts `/proc`, and reaps as PID 1. All three work
unprivileged.

The constraint is that **the uid mapping decides whether mounting is possible**,
and there is only one mapping available to an unprivileged process:

| mapping | `/proc` remount | `ps -u $USER` |
|---|---|---|
| uid → uid (stay yourself) | **fails** — `execve` drops capabilities when the euid inside is not 0 | works |
| uid → 0 | works | **selects nothing** |

Neither column can be had with the other, so the sweep has to stop selecting by
user. It does not need to: **inside a PID namespace `ps -e` is already exactly
this run's work**, and inside an IPC namespace so is `ipcs`. The prototype's
`ps -e` returns three processes, all of them the run's own, on a machine with
three hundred.

So the reset changes shape here, and that *is* the entry rather than a detail of
it: `ps -u $USER` becomes `ps -e`, `ipcs | grep $USER` becomes `ipcs`, and
containment stops being a filter that can be wrong and becomes a property of
where the process is. It is also what makes the JVM sweep fix safe -- inside,
"every JVM the user owns" and "every JVM this run started" are the same set.

| Evidence | the full suite runs while the developer keeps working, and `ps -e` inside the namespace shows only the run. Before and after verdicts identical case by case, measured with `selfcheck.sh`, because the reset going from a no-op to actually killing things is a behaviour change and has to be treated as one |
| Off by default | it must be, and not only because ADR-015 says so: today's sweep does nothing, and turning it on is the first time in this project's history that the reset will kill anything |

**Status correction (2026-09-03).** This entry was first written as *ready*, and that was wrong
under criterion 2 of ADR-015: the T item it improves on is the shell task, and the shell task has
not passed its gate. The rule is not ceremony -- improving the reset before the reset is proven
equivalent would leave any later difference with two possible causes. Corrected the day it was
written, which is the cheapest a correction ever gets.

**A runner at PID 1 has to reap, and this one does not (2026-09-07).** The comparison wrapper found
this before the feature did. `in-ns.sh` ended in `exec "$@"`, which makes whichever runner it starts
PID 1 of the new PID namespace -- and PID 1 must collect orphans. A shell does, so CTP, whose PID 1
was `bash ctp.sh`, collected what its cases left behind. A Go binary does not, so testkit did not:
`cub_master` never learned that `cub_server` had exited, and `cubrid server stop` polled through
`cub_commdb -S` indefinitely. On `_01_sqlx/bug_cubridsus2018` that is 21 s under CTP against over
twelve minutes under testkit, and 15 s under testkit once a shell is left at PID 1.

In the sandbox it is the wrapper's bug and it is fixed there. **For this entry it is the feature's
bug**, because a container is exactly the case where the runner *is* PID 1 with no shell above it
and no system init to fall back on. Handing testkit a container therefore means one of two things,
and the entry has to choose: the runner reaps orphans itself, or the image ships an init and the
runner documents that it requires one. Neither is written yet.


### B-T3. Cases that run at the same time — **blocked**

| | |
|---|---|
| Blocked on | T: the shell task passing the full-corpus gate (ADR-013) |
| Improves on | T: `Test.runAll` — one case at a time, per machine |
| Today | one case at a time. `_01_utility` is 217 cases at a measured median of 12 s: **66 minutes, serially**, and the full corpus is 3,452 -- around sixteen hours a runner. Two cases cannot share a machine today because they would collide on the master port, the broker ports, the shared-memory ids, `cubrid.conf` and `databases.txt` -- and because the reset would kill each other's servers. `RestoreScript` is not the reason: it measures 0.00 s |
| Beyond | each case gets its own instance — own port, own shared-memory id, own data directory — so N run at once on one machine. The per-instance parameters this needs are the ones `ConfigureScript` already writes; what is missing is allocating them per case rather than per machine |
| Evidence | wall-clock for `_01_utility` at N=1 against N=4 and N=8, with verdicts identical to the serial run — **identical, not merely similar**: a case that passes only when it has the machine to itself is a finding, not an acceptable cost |
| Risk | this is where the parallel-run bugs live. It must not ship before the serial version is proven, or a difference has two possible causes |

**What a slot is.** Everything below has to differ between two cases running at
once, and each line is somewhere the suite already writes:

| | where | |
|---|---|---|
| `cubrid_port_id` | `cubrid.conf` | each slot gets its own master |
| `BROKER_PORT` | `cubrid_broker.conf`, `%query_editor` and `%BROKER1` | |
| `MASTER_SHM_ID` | `cubrid_broker.conf`, `[broker]` | **ids that collide do not fail, they interfere** -- the same failure mode `spec-corrections.md` already records for the broker-only configuration |
| `APPL_SERVER_SHM_ID` | per broker section | |
| `$CUBRID` | the install | cases edit `cubrid.conf`, so the file cannot be shared |
| `$CUBRID_DATABASES` | `databases.txt` | one registry, and every case adds and removes an entry |

**The reset is what actually blocks it, not the ports.** `KillScript` matches
`cub` as a substring across everything the user owns, so slot A's reset kills
slot B's server. Partitioning ports without containing the reset produces a
suite that fails at random. **Each slot therefore needs its own PID and IPC
namespace** -- the same mechanism B-T2 is about, which makes B-T2 a dependency
rather than a neighbour. It is available unprivileged: measured.

**Configuration: a count and a range, and nothing else.**

```
parallel_slots=4                  # 1 is today's behaviour and the default
parallel_port_range=15230-15299   # slots take consecutive blocks from here
```

Slot *i* takes a block -- one master port, the broker ports, and headroom -- and
shared-memory ids come from a second range the same way. **A range too small for
the requested slots is a startup failure**, not a wrap-around: two slots quietly
sharing a port is the bug this whole entry exists to avoid, and it must not be
reachable by arithmetic.

**Most of it is already written.** `deploy.go`'s `iniTargets` maps a role to the
`ini.sh` section that holds its parameters, and `ConfigureScript` writes them.
What is missing is the allocation unit: they are computed per machine and need to
be computed per slot. That is the sentence this entry has carried from the start,
now with the mapping named.

**Two questions this design does not answer.**

*Does each slot get its own `$CUBRID`?* Copying the install per slot is expensive
and copy-on-write through overlayfs is O(1), which is the third option under
**B-T8** -- worth little serially, and the reason it exists is here.

*Disk.* Four slots each creating a 707 MB database, alongside B-T8's template
store. The sparse storage that made templates 356 MB stops being a nicety at
four slots.

**Built so far: the allocation, and nothing that runs.** `internal/slot` turns a
count and a range into slots, or refuses. It was written first because it is the
part where being wrong is silent: two slots sharing a port produce a suite that
fails at random, and the test that matters checks *every* number in an allocation
for a repeat rather than spot-checking one. A range too small refuses with the
arithmetic in the message rather than wrapping.

Slot *N* is always the same slot, so a failure found under four can be reproduced
by running that one alone. `Parameters` is keyed by the file and section
`deploy.go` already names, so the allocator decides the values and nothing
between it and the file gets to invent one.

**A network namespace made most of that table unnecessary.** Every line above
except the last two is a number two slots would collide on, and a network
namespace hands each slot the whole port space instead: every slot runs on the
shipped 1523, and the configuration is never rewritten. That is not only less
code than allocating blocks -- it is what keeps a slotted run's conf files and
log lines byte-identical to a serial run's, which is the evidence this entry has
to produce. `internal/slot`'s port and shared-memory arithmetic is no longer on
the path; the IPC namespace covers the segments the same way.

What it costs is the outside, and the cost is nameable: six of the 3,452 cases
mention wget or curl and only one, `_06_issues/_25_2h/cbrd_26350`, fetches a URL
that is genuinely external. The 201 cases that say `localhost`, the 34 that say
`127.0.0.1` and the 126 that read the hostname are all unaffected.

**Five things had to be isolated, and the list was not obvious.** Each was found
by running four slots and reading what broke, and each is written down because
the next person to try this will meet them in the same order:

| | why the obvious namespace was not enough |
|---|---|
| the reset | slot 0 was left outside the namespaces, to keep one slot on the old path. A worker outside sees into all of them, so its `ps -e` sweep killed three keepers, two other slots' cases and another slot's `cub_server` |
| the worker log | a case's trace went out a line at a time, so two cases interleaved -- a `[TESTCASE]` header followed by another case's trace |
| `/dev/shm` | an IPC namespace separates System V segments. `cub_broker` and `cub_cas` call `shm_open` too, and POSIX shared memory is a file on a tmpfs the mount namespace inherits |
| the master's socket | `cub_master` listens on `$CUBRID_TMP/CUBRID<port>`, `/tmp` when unset -- and the network namespace that lets every slot keep 1523 is exactly what made them all want one path. Four masters over one socket is four that do not start |
| that socket's length | `sockaddr_un.sun_path` is 108 bytes, and a `CUBRID_TMP` under the slot root was 127. The engine says "The $CUBRID_TMP is too long" on every command, and the case then fails on a comparison |

The fourth is the one worth pausing on: it is the *cost* of the network
namespace's simplification. Removing the need to allocate ports is what made
every slot want the same socket path.

**What is left is the plan the queue is ordered by.** N workers on the shared
queue is done; ordering that queue longest-first is not, and it is worth 18% at
ten slots and 31% at sixteen -- measured over the smoke corpus's own durations,
which `test-shell.xml` already records.

### A mount namespace per slot

Each slot runs in a mount namespace where **`$CUBRID` is the same path it always
was**, with the few directories a run writes to bound over from that slot's own
copy. Nothing else moves.

*Why the path must not change.* If slot 2 had `$CUBRID=/…/slot2/CUBRID`, every
absolute path a run records would carry the slot number: `databases.txt`, the
`_vinf` and `_lginf` files, every log line, every traced command. The comparison
would need a slot mask, and a parallel run would stop being byte-comparable with
a serial one -- which is precisely the evidence B-T3 has to produce ("verdicts
identical, not merely similar"). Keeping the path makes a slotted run's output
the same output. It also lets B-T8's template cache be shared across slots
unchanged, because the key stops depending on which slot built it.

*What has to be bound, measured rather than guessed.* `$CUBRID` is 321 MB, and
`lib/` is 241 MB of it. What a run writes is much smaller:

| | size | why it is written |
|---|---:|---|
| `conf/` | **60 KB** | cases edit `cubrid.conf`; the slot's ports and shm ids live here |
| `log/` | **12 KB** | server and utility logs, and `qa_fatal_error_count.log` |
| `var/` | **4 KB** | cleared by the reset |
| `lib/` | 241 MB | **206 cases of 3,452 call `make_locale`**, which writes `libcubrid_*.so` here |
| `$CUBRID_DATABASES` | -- | outside `$CUBRID` already, so it is simply a per-slot directory |

Three of those are 76 KB together: a per-slot copy is free. `lib/` is the awkward
one, and it is awkward at 6% of the corpus rather than at none, so it cannot be
waved away. Two ways, and the choice is not made here: bind a per-slot copy
(241 MB a slot, real but not prohibitive beside the 707 MB a case's database
already costs), or bind only the generated `libcubrid_*.so` names over a shared
read-only `lib/` and accept that a case which writes something else there is
outside the design.

**That paragraph used to say this could not be settled here, and it was a
misreading.** No case calls `make_locale`: 196 of them call `do_make_locale`,
which is a function in `init.sh` that calls the `make_locale.sh` this machine
has. The count that said otherwise came from a pattern that matched the tail of
the longer name. What is true is that `make_locale.sh` needs
`cubrid_locales.txt` to name a locale, and 172 of the 206 cases copy
`cubrid_locales.all.txt` over it first -- so the thing runs, and its cost is
measurable here.

**And the writable set was wrong as well.** `make_locale.sh` builds into
`$CUBRID/locales/loclib/` and *then* moves the result to `$CUBRID/lib/`. This
entry named only the second. Both are per-slot, and the first is 12 KB.

*What this turned out to be: overlayfs after all.* This entry ruled it out on
the grounds that there was nothing to stack when the writable set was four
directories. The set was five, one of them 241 MB, and the list had already been
wrong once -- so the argument was resting on a list that could not be trusted to
stay right. Unprivileged overlayfs works inside the runner's user namespace,
measured, and it needs no list: the install is a shared lower layer and whatever
a slot writes anywhere under `$CUBRID` lands in its own upper.

Reading through it costs nothing that can be measured -- executing a binary,
reading a large file and listing a directory all come out the same on the
overlay as off it, because an unmodified file is opened straight through to the
lower layer. The one cost is the first write to a file, which copies it up:
6.8 ms for `cubrid.conf`, 0.1 ms for every write after. And N slots sharing one
lower layer share its page cache, which N copies would not.

*Where it lands.* `contain` currently re-executes the runner once, for the whole
process. A slot needs the same thing per worker, so `contain` grows the ability
to run a subprocess in a namespace rather than only to put itself in one. That is
the interface change B-T3 asks of B-T2, and naming it is the point of writing
this down before building it.

### B-T7. Compare a query plan as data, not as prose — **blocked**

| | |
|---|---|
| Blocked on | T: the sql module (Phase 4) |
| Improves on | T: `StringUtil.replaceQureyPlan` |
| Kind | **capability** — it makes the suite able to see something it currently cannot |

**Today the masking blinds the test to what it was written to catch.** A plan is
compared as rendered text, and normalisation is one blind regex over the whole thing:

```java
queryPlan = queryPlan.replaceAll("[0-9]+", "?");
```

Cost and cardinality *should* be masked — they move with the statistics. But the regex cannot tell
a cost from an identifier, and the committed answer files show what that costs:

```
class: t?              (30 occurrences)   ← t1, t2 and t3 are the same string now
index: i_t?_j?_j?
index: i_t?_i?_i?_i?
```

So a plan that scans `t1` and a plan that scans `t2` compare equal. A plan that switched from
`i_t1_i1_i2` to `i_t1_i1_i3` compares equal. **These are plan tests, and a changed index is exactly
the regression they exist to detect.** The masking does not merely blur the answer; it removes the
signal and leaves the test passing.

**The engine already emits the plan as JSON.** `qo_plan_print_json` in
`src/optimizer/query_planner.c` builds it, and the identifiers are their own fields:

```c
json_object_set_new (scan, "table", json_string (class_name));
json_object_set_new (scan, "index", json_string (...->constraints->name));
```

and the switch is a system parameter that exists — `query_trace_format`, whose keywords are
`text` and `json` (`system_parameter.c:5694`).

| Beyond | ask for `query_trace_format=json`, compare field by field: mask `cost` and `cardinality`, keep `table`, `index` and the plan shape |
| Evidence | a case whose plan changes index from `i_a` to `i_b` fails, where today it passes. That is one case to write, and it is the whole argument |
| Constraint | the text form stays. The `.answer` files are frozen (NG1) and hold rendered plans; JSON is a **second** comparison an operator turns on, not a replacement |
| Note | this is the clearest instance of criterion 1 in ADR-015: it names what it beats and what today is measured, and the measurement is a count from the corpus rather than an opinion |

### B-T8. Stop paying 1.1 GB for every case — **blocked**

| | |
|---|---|
| Blocked on | T: the shell task passing the full-corpus gate (ADR-013) |
| Improves on | T: nothing in the runner. **The cost is inside the cases, and the cases may not be touched (NG1)** |
| Kind | **speed** — the same verdicts, sooner |

**Where the time actually goes, measured.** The runner is not the problem, and the number says how
much not: over 217 cases the runner spends **52 s against the cases' 3,922** -- 1%, or 0.2 s a case.
`RestoreScript` between cases measures 0.00 s. The rest is inside the cases, and over all 217 of
`_01_utility` the two runners spend it identically:

| | CTP | testkit |
|---|---:|---:|
| total | 7,708 s | 7,670 s |
| **median** | **11 s** | **12 s** |
| mean | 17.8 s | 17.8 s |
| max | 219 s | 219 s |

**The median is the number that matters.** Half the corpus is cases that do very little, and they
still cost twelve seconds each, because that is what it costs to arrive at the point where a case
can do anything at all. Timed one at a time, that point costs:

| | measured | what it needs | |
|---|---:|---:|---|
| `cubrid createdb` | 4.6–7.7 s | — | wall 7.72 s against **0.53 s of CPU**: waiting, not computing |
| `cubrid server start` | 3.02 s | **0.17 s** | the server answers a query at 0.17 s; the utility says so at 3.02 |
| `cubrid server stop` | 2.02 s | ~0 s | the process is already gone when it returns |
| `cubrid deletedb` | 0.21 s | 0.21 s | |

`util_service.c` polls with `sleep (1)` -- `is_server_running`, and the `sleep (1); /* wait to
start */` and `/* wait to stop */` loops around the Java stored-procedure server. Turning that
server off with `java_stored_procedure=no` takes start to 2.02 s and stop to 1.01 s, so a second of
each is javasp and the rest is the same one-second granularity. **94% of the corpus (3,260 of 3,452
cases) creates a database, so every case pays this.**

What a case spends it on is visible in its own trace:

```
+ cubrid createdb -r csqldb --db-volume-size=20M en_US
  Creating database with 64.0M size using locale en_US.
  The total amount of disk space needed is 1.1G.
+ cubrid server start csqldb
  ... the actual test ...
+ cubrid server stop csqldb
+ cubrid deletedb csqldb
```

**The case asked for 20M and the engine laid out 1.1 GB**, of which 707 MB is really on disk. Across
3,452 cases that is over two terabytes written and deleted to run a suite whose queries are small.

**It is not the disk, and that was measured rather than assumed.** Putting `databases/` on tmpfs
takes `createdb` from 4.29 s to 4.57 s -- no faster. The first version of this entry named tmpfs as
the cheapest of three options; it is not an option at all, and the CPU numbers above say why.

**Copying a prepared database is 27 times cheaper than creating one**, and the prototype in
`scratchpad` proves the whole path rather than the copy alone: create in one directory, save a
sparse template (356 MB, half the on-disk size), restore into a *different* directory, start the
server, answer a query.

| | |
|---|---:|
| `cubrid createdb` | 4.60 s |
| save the template | 0.16 s |
| restore it elsewhere | **0.17 s** |

Restoring elsewhere needs two things a plain copy does not do: `<db>_vinf` and `<db>_lginf` are
ASCII and hold absolute paths, and `databases.txt` needs the entry. Both are text edits.

**The key has to carry the name.** Keyed on options alone the copy has to be renamed, and
`cubrid renamedb` costs 3.13 s -- it eats half the saving. Keyed on `(name, options, charset,
build)` there is nothing to rename. `_01_utility` uses 64 such keys against 16 option-only ones, so
the cache is bounded by disk rather than by correctness: at 356 MB a template, the six most-used
keys cover 56% of the 223 calls for about 2 GB.

**The interception point already exists, and it is not the cases.** `init.sh` puts `${init_path}`
at the head of `PATH` and makes `${init_path}/cubrid` executable, so **every `cubrid` a case runs is
already a wrapper script**, which today intercepts `deletedb`, `server` and `checkdb` to save a
snapshot when recovery fails. `cubrid_createdb` — what cases actually call — is a shell function in
`init.sh:1614`. Neither is in the testcases repository, so neither is frozen by NG1.

**And CTP already did this once.** `create_ccidb` in the same file checks for
`$CUBRID/databases/ccidbbak` and, if it is there and big enough, does `cp -r` instead of
`createdb`. The pattern is not a new idea here; it is one function away from being general.

| Beyond | **memoise `cubrid_createdb`.** Normalise its arguments into a key; copy the prepared database when one exists for that key, create it and keep it when one does not. `create_ccidb` in the same file already does this for one fixed database, so the shape is CTP's own |
| Evidence | `selfcheck.sh` over `_01_utility` with the cache off and then on: **verdicts identical case by case**, not merely the same counts, and wall clock roughly halved. The instrument for this already exists and its floor is measured — two runs of one runner differ by 24 lines in `feedback.log` and none at all in the four files that carry verdicts, so a verdict that moves is a finding rather than noise. `_25_unstable` gets the same treatment: it is the family whose own readme says it depends on elapsed time, and a case that only passes when creation is slow is a finding, not an acceptable cost |
| Where it goes | `init.sh`'s `cubrid_createdb`, which **both runners call**. That is what keeps criterion 2 satisfiable: the change does not distinguish CTP from testkit, so a comparison across it stays a comparison. What it does change is old behaviour against new, and `selfcheck.sh` is exactly the instrument for that |
| Risk | a wrong key copies the wrong database, and that failure is silent. The mitigation is that the key *is* the parameters: same key, same database by construction. An argument the key does not understand falls through to a real `createdb` rather than being guessed at |
| Not in the key | the parameters cases actually change. `supplemental_log`, `unicode_input_normalization`, `dont_reuse_heap_file`, `isolation_level` and `lock_timeout_in_secs` were each set before a `createdb` and the resulting files compared: **identical every time**, against `db_page_size` and `db_volume_size` which differ as they should. These are read by the server at startup, which happens after the copy, so a case's edit still takes effect. Hashing the whole `cubrid.conf` into the key -- the first thing this entry proposed -- would have missed the cache for the 46 cases that set `supplemental_log` and gained nothing |
| Start and stop | start and stop are 5 s of the 11.6 and **about 4.8 of that is `sleep (1)`** in `util_service.c`. This entry concluded that removing it was an engine change in a third repository. **That was wrong for start**: the wrapper can return when the server answers without touching the engine, which is B-T9. It remains true for stop |
| Why not share one database | because the corpus does not. 88% of cases call `deletedb`, 75% `server start`, 64% `server stop`: they own the lifecycle explicitly. 13% call `loaddb`, 6% `unloaddb`, 3% `backupdb`, 2% `restoredb` and 2% `checkdb` -- they test the database files themselves. A long-lived shared database would change what a quarter of the corpus is testing |

### B-T9. Let a case go on when the server answers — **blocked**

| | |
|---|---|
| Blocked on | T: the shell task passing the full-corpus gate (ADR-013) |
| Improves on | T: nothing in the runner. Like B-T8 the cost is inside the cases, and the cases may not be touched (NG1) |
| Kind | **speed** — the same verdicts, sooner |

**B-T8 said this one was somebody else's repository, and that was wrong.** Its last row concluded
that the `sleep (1)` in `util_service.c` "is an engine change, not a harness one". The measurement
that settles it is when the server becomes usable against when the utility says so:

| | |
|---|---:|
| `cub_commdb -P` lists the database | 1.16 s |
| `csql` connects and answers a query | **1.16 s** |
| `cubrid server start` returns | 3.02 s |

The two readiness numbers are the same instant, which is the whole argument: `cub_commdb -P` is not
a registration that precedes usability, it *is* usability, so a wrapper that waits for it and then
returns has not shortened anything a case depends on. **2,590 of the 3,452 case scripts start a server**,
so the 1.9 s is hours.

**What it costs is that the output has to be written rather than read.** The utility buffers
everything and flushes at exit -- at 1.16 s, when the server is usable, nothing has been printed
yet. So an early return has to produce the text itself, and the text is compared: 14 cases hold
these lines in an answer file. Measured rather than guessed, the grammar is:

```
[jsp=n]        java_stored_procedure system parameter is not enabled
[master down]  @ cubrid master start
[master down]  ++ cubrid master start: success
               @ cubrid server start: <db>
               ++ cubrid server start: success
[jsp=y]        Calling java stored procedure is allowed
[jsp=n]        Calling java stored procedure is not allowed
```

all on stdout, stderr empty, exit 0. `java_stored_procedure` is read the way the engine reads it --
`[@db]` overriding `[common]`, which was confirmed by setting them against each other.

| Beyond | **return from `server start` when the server answers.** The wrapper backgrounds the real utility, polls until the database is connectable, writes the output above and returns; the utility finishes on its own |
| Where it goes | `init.sh`'s sibling, the `${init_path}/cubrid` wrapper that already intercepts `server` and `checkdb`. **Both runners put `${init_path}` at the head of `PATH`**, so like B-T8 this does not distinguish CTP from testkit and a comparison across it stays a comparison |
| Risk | the written output is wrong somewhere the grammar does not cover, and that failure is silent in exactly the 14 cases that would catch it. The mitigation is that **everything outside the grammar declines**: a flag, a database already up, a `java_stored_procedure` it cannot parse, a start that fails -- each falls back to the wait and to the utility's own words, so the written text is only ever produced for the one shape it was measured against |
| Evidence | `CTP_SERVER_START_NOWAIT=verify` takes the fast path and then waits anyway, comparing what was written against what the utility really printed and recording **every** fast path, not only the ones that differ -- a log that only recorded differences would be equally empty when nothing was taken |
| Why a third setting | because `1` has to be earned. `verify` is no faster than before and exists only to produce the evidence for flipping the default |

**What has been shown so far.** Against the utility itself, over the conditions the grammar
distinguishes -- `java_stored_procedure` y and n, master up and down, and a `[@db]` section set
against `[common]` -- stdout, stderr and exit code are **byte-identical**, at 1.16-1.32 s against
3.02. Against the corpus, the 14 cases that hold this output in an answer file were run three ways:

| | wall | OK | NOK | fast paths | differing |
|---|---:|---:|---:|---:|---:|
| wait | 345 s | 6 | 8 | — | — |
| verify | 350 s | 6 | 8 | 12 | **0** |
| nowait | 336 s | 6 | 8 | — | — |

Verdicts identical case by case, and the eight failures are the machine's, not the change's: they
fail in the `wait` arm too. The wall clock barely moves because these fourteen are heavy cases that
spend their time on their own work -- this run was for the output, not for the speed.

**Over the corpus, and what it found.** `verify` across `_01_utility`'s 217 cases took 173 fast
paths and **three of them wrote something the engine did not say**: `Calling java stored procedure
is allowed`, on a start where the engine had printed no such line.

That line is not util_service's. **`cub_javasp` prints it about itself**, and
`process_javasp_start` does not spawn it when a javasp server for that database is already up -- so
the line is simply absent. All three were the *later* starts inside a case that starts a server more
than once: the first start left a javasp running, and the grammar, which read only the
`java_stored_procedure` parameter, could not see it.

The fix is not a fourth condition in the grammar but a fourth reason to decline. `cubrid javasp
status` answers in 0.01 s and only "is not running" earns the fast path. Re-run:

| | before | after |
|---|---:|---:|
| fast paths, master down | 95 | 95 |
| fast paths, master up | 81 | **78** |
| **differing** | **3** | **0** |

Exactly the three, and nothing else. Verdicts identical to the wait baseline case by case, 174 OK
and 43 NOK, and the wall clock 3,851 s waiting against 3,525 s not -- **8.5%** off a corpus whose
time is mostly the cases' own.

**What that does not yet earn.** Every one of the 173 was `java_stored_procedure=y`. The `n` branch
of the grammar has been checked against the utility but never against a case, and 217 is 6% of the
corpus. `verify` costs 2% over `wait` (3,946 s against 3,851), so the full-corpus run that ADR-013
requires can carry the audit at no meaningful cost -- which is where a default should be earned,
rather than from a sixteenth of the evidence.

**The lingering utility does not interfere.** The fast path returns with the real `cubrid server
start` still running for another 1.9 s, so what a case does next was tried against it: an immediate
`server stop` succeeds and the server stays down after the utility finishes, an immediate query
connects, and an immediate `deletedb` refuses exactly as it does today. It is started with stdin on
`/dev/null` so it cannot hold a pipe the case owns, and its output goes to files that are unlinked
before the wrapper returns.

### B-T10. The other second, on the way down — **idea**

| | |
|---|---|
| Improves on | T: nothing in the runner; the same place as B-T8 and B-T9 |
| Kind | **speed** — the same verdicts, sooner |

**The mechanism is proven for the engine that is actually running.** The installed build is
11.3.5.1275-0e31336 and that commit is in the source tree, so this is read from it rather than from
a neighbouring version. `cubrid server stop` runs `cub_commdb --server-stop` and waits for it;
`cub_commdb` asks the master for the server's pid, sends the kill, prints the reply, and then waits
like this:

```c
while (1) {
    if (kill (pid, 0) < 0) break;
    sleep (1);
}
```

**It checks before it sleeps.** So the 1.01 s a stop costs with the Java stored-procedure server off
is one whole tick: the server was still alive at the first check and gone by the second. The real
shutdown therefore takes somewhere in `(0, 1 s)`, and what could be saved is one second minus that
-- **a quantity this has not measured.** The register's own rule is why this is an idea and not a
plan.

**Where it would probably disappoint.** Shutdown has to flush, so a case that wrote a lot should
take longer to come down than an empty database -- which means the saving is largest exactly where
the case was cheapest, and a single measurement on an empty database would overstate it. The
measurement has to span a database with data in it, and the Java stored-procedure server on as well
as off: `process_javasp(STOP)` runs first and is the second that separates 2.02 s from 1.01 s.

**The risk is the opposite shape to B-T9's, and then it is not.** Returning early from a *start*
hands the case a server; if that were wrong the case fails visibly. Returning early from a *stop*
hands the case a database it believes is down, and if that were wrong the next `deletedb` or
`createdb` would act on live files. But unlike start, the predicate here can be the state itself
rather than a promise about it: **the process is gone (`kill -0` fails) and the master no longer
lists it**. Both are cheap, and if both hold the case's belief is true. So the risk collapses back
to the output text, which is where B-T9's already is.

**The output is harder here than it was for start.** `cub_commdb` prints its own two lines as a
child process, and the order they land in depends on buffering -- into a file the child's text
arrives before the parent's, because the parent's stdout is block-buffered and the child flushes its
own:

```
Server <db> notified of shutdown.
This may take several minutes. Please wait.
@ cubrid server stop: <db>
++ cubrid server stop: success
```

Five cases hold this in an answer file, four of which are already in B-T9's fourteen, so the
instrument for checking it exists. **A third of the calls would decline anyway**: in a sample of
case logs, `++ cubrid server '<db>' is not running.` appears 98 times against 172 successes, and
that path does no shutdown work and has nothing to save.

| Worth | 2,239 of the 3,452 case scripts stop a server, so a bound of one second each is under half of what B-T9 is worth, and the true figure is smaller by however long a shutdown really takes |
| First | measure `t_gone` against `t_return`, on an empty database and on one with data, with the stored-procedure server on and off. If `t_gone` is not well under a second there is nothing here and this entry closes |

### B-T11. One place to watch every task run — **idea**

| | |
|---|---|
| Improves on | T: nothing. A run's progress has never been visible anywhere but its log |
| Kind | **operability** — the same verdicts, and you can see them arriving |

**Built so far: a page for the shell task.** `status_http` serves it; it says what
each slot is on and for how long, the counts, and the rate. It is not on standard
output and could not be: what the runner prints there is frozen (ADR-003) and the
comparison reads it, so a screen drawn over it would be drawn over the evidence.

**Two things it does not do yet, both asked for while it was being built.**

*The machine, not just the run.* Everything this project has measured about
parallelism is a resource question -- 332 MB/s of disk, 498 MB a server, 16
cores against ten slots -- and none of it is visible while a run is going. A slot
holding a case for four minutes reads the same whether it is waiting on a lock or
on a disk that has nothing left to give. Load, free memory, disk throughput and
free space alongside the slots would answer that from the same screen, and the
numbers are cheap: `/proc/loadavg`, `/proc/meminfo`, `/proc/diskstats`, `statfs`.

*Every task, not just shell.* The register's own plan is `sqlsuite`, `isolation`
and `replication` after shell (ROADMAP Phase 4), and a QA run is all of them.
Four pages on four ports is four things to keep open; the useful shape is one
board that several runners report to. That is a decision about where the board
lives -- in the process, or behind something a run registers with -- and it is
easier to make now, while there is one runner, than after there are four.

| First | decide whether the board is a library each runner serves or a place they report to. The rest follows from that, and it does not become cheaper to decide later |
| Not yet | a history across runs. This is a window on a run in flight; what a run *was* is the result tree, and duplicating it here would make two answers to the same question |

### B-T12. A corpus that is clean because it cannot be otherwise — **ready**

| | |
|---|---|
| Improves on | T: nothing. Nobody put the 20 GB there on purpose |
| Kind | **correctness first, then speed** |

**The measurement that started this.** The `_01_utility` tree is 105 MB in git and
**20 GB on this machine**, with 125 files over 10 MB in it -- `xdb_lgat`,
`testdb_lgar_t`, database volumes from runs that ended weeks ago. Cases create
their databases in their own directory and do not all delete them, and nothing
puts the tree back. Every run since has been reading and writing over that, and
**upstream's CI has the same tree and the same problem**.

That is a correctness question before it is a speed one. A case that finds a
database it did not create behaves differently from one that does not, and which
of the two a run gets depends on what ran on that machine last.

**An overlay makes clean structural rather than a step.** Lower layer is the tree
as the repository has it, read-only and untouched; upper layer is where a run's
writes go and is discarded when the run ends. There is nothing to remember to do
and nothing to copy -- 105 MB is cheap to copy, but a copy is a step that can be
skipped and an overlay is a property of how the run is mounted.

**And the upper layer can be RAM, which is the same mechanism answering the
speed question.** Measured: an overlay with a disk lower and a tmpfs upper reads
the clean tree through, puts every write in memory, leaves the lower unchanged,
and stops with `No space left on device` at the size it was given.

That is the hybrid rather than a choice between disk and memory:

| | where | why |
|---|---|---|
| the corpus | disk, read-only | 105 MB, page-cached, shared by every slot |
| a run's writes | tmpfs upper | 8 slots at `log_volume_size=20M` is 1.7 GB; at the 512M default, 5.5 GB. 17 GB free |
| the template store | disk | 10 GB, and it has to survive the run. Restoring then reads disk at **1.4 GB/s** -- four times the write speed -- into memory, which is the fastest arrangement available |

**The size bound is the safety property.** The reason not to put everything in
memory is a case that writes without cleaning up: on disk that accumulated 20 GB
over weeks, in memory it would end the run. Capping the upper layer turns that
from a run that dies into a case that fails with a full disk -- which is also a
change in behaviour and has to be checked, because a case that tests running out
of space now finds a different amount of it.

**Where it goes and in what order.** In the runner's own namespace, before the
slots are opened, so that they inherit one shared overlay: each case has its own
directory, so there is nothing to isolate per slot, and mounting it per slot
would be N tmpfs where one will do.

| Evidence | wall clock and verdicts at one and eight slots, against the same corpus on disk. Two levers -- the volume size and the ramdisk -- measured separately, because a combined number cannot say which one paid |
| Risk | `ENOSPC` where there was 141 GB free. Named rather than mitigated: the cap is a number to pick with the corpus in front of you, and the verdicts say whether it was picked right |

### B-T4. A verdict that says why — **idea**

| | |
|---|---|
| Improves on | T: `Test.collectGeneralResult` |
| Today | a case fails if any line of its `.result` contains the substring `NOK`. A case that prints the word while explaining itself fails, and always has (`TestVerdict`) |
| Beyond | a structured result — which check, expected against actual — alongside the text, so a failure can be read without opening the case |
| Evidence | a failing case's report identifies the failed check without a human reading `.result`; the substring rule still decides the verdict, so nothing about pass or fail changes |
| Constraint | the `.result` file is a frozen surface written by the cases themselves, and the cases repository cannot be modified (NG1). So this **adds** a channel; it does not replace one |

### B-T5. Flakiness as a first-class outcome — **idea**

| | |
|---|---|
| Improves on | T: `dispatch.Queue`, `feedback.CaseStopRetry` |
| Today | a retried case reports only its final attempt. `feedback.log` carries the intermediate ones, and nothing adds them up |
| Beyond | every attempt is recorded and the case is classified: deterministic failure, passed on retry, or alternating. A build with 3 hard failures and 40 flakes is a different situation from one with 43 failures, and today they print the same |
| Evidence | a run over `_25_unstable` — 195 cases whose own readme says they depend on machine load — separates into classes that match what that readme predicts |

### B-T6. Ask a case why it is slow — **idea**

| | |
|---|---|
| Improves on | T: `TestMonitor` |
| Today | the monitor kills a case that outruns its timeout and records `NOK timeout`. What the case was doing is gone |
| Beyond | before the kill, capture what the monitor can see — the process tree, what the server is waiting on — into the worker log |
| Evidence | a timed-out case's log says what it was blocked on; no change to the verdict or to the timeout |

---

## B-O — beyond, on the QA-operations axis

Every entry here is **blocked on the operations layer existing at all** (ADR-012). They are recorded
now because they were found now, and because they say what that layer is for.

### B-O1. More machines by more runners — **blocked**

| | |
|---|---|
| Blocked on | ADR-012 |
| Improves on | O: the fleet excluded by ADR-014 — instance inventory, deploy across N, dispatch between them |
| Today | CTP taught one runner to manage a fleet, and that is what made it need to own its machines |
| Beyond | the operations layer shards a corpus across N runners, each on its own machine, and merges the results. The runner stays scoped to one machine, which is what makes it something you can hand a container |
| Evidence | 3,452 cases across 4 runners complete in near a quarter of the single-runner time, and the merged verdicts match a single-runner run case for case |

### B-O2. Results that outlive the next run — **blocked**

| | |
|---|---|
| Blocked on | ADR-012 |
| Improves on | O: `FeedbackDB` (excluded), `current_runtime_logs` |
| Today | the run directory has a fixed name and the next run overwrites it. The tarball CTP packs at the end is the only history, and it is a file on the machine that produced it |
| Beyond | a result store with history, so **"when did this case start failing?"** is answerable without unpacking archives |
| Evidence | the question above, answered for a case, across runs, from a query rather than from a shell pipeline |
| Note | this is where `FeedbackDB` should have been. It was excluded as axis O, and it is the right thing built in the right place rather than the wrong thing kept |

### B-O3. Failures grouped by cause — **idea**

| | |
|---|---|
| Blocked on | ADR-012, B-O2 |
| Improves on | O: the JIRA filing excluded in `migration-exclusions.md` §1-3 |
| Today | CTP files an issue per failure from a template. A single broken commit produces as many issues as it broke cases |
| Beyond | failures are grouped by signature — the same assertion, the same stack — so two hundred failures with one cause become one item with two hundred instances |
| Evidence | a run with a known single-cause breakage produces one group |

---

## What is not here

Anything that adds a **new kind of testing** belongs in §6a, not here (ADR-015). SQLancer, fuzzing
and workload generation change what can be found; this axis changes how well the existing work is
done. If an entry would need a new oracle, it is an extension.
