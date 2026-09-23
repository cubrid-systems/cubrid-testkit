# module-ha — what HA testing has to establish

- **Date:** 2026-09-21
- **What this is:** the functional specification for HA testing in this system. Written **before**
  any port, because the thing being ported is weak enough that copying it would lock the weakness
  in. What CTP does is §2, measured; what it does not examine is §3, measured; §4 is the
  specification, in two groups — **A, the topology holding still**, which is what these two suites
  are actually for and where the near work is; and **B, the topology as the subject**, which is
  admitted, specified and deferred.
- **Trees:** cases `CUBRID/cubrid-testcases-private` `HA/shell` (373 cases by ADR-013's rule) · CTP
  `cubrid-testtools` as checked out here · `cubrid-cluster-sandbox` `3081496`.
- **Related:** ADR-022 (who stands the topology up) · ADR-015 (axis B, and its four admission
  criteria) · `evidence/ha-topology.md` (what a sandbox pair gives) ·
  `concept/beyond-axis.md` B-T16.

---

## 1. Two things wear the word

ADR-022 separated them and this document keeps them apart, because their requirements have almost
nothing in common.

| | **HA shell** | **`ha_repl`** |
|---|---|---|
| what it is | the `shell` task against a pair — `ha_shell.conf` | a task of its own |
| corpus | `HA/shell`, 373 cases, shell-format | the `sql` corpus, converted |
| who builds the pair | the **case**, by calling `make_ha.sh` | the runner |
| oracle | master-against-slave, file by file | master-against-slave, dump by dump |
| frozen | yes (NG1) | the conversion is not |

They agree on the one thing that matters most: **the oracle is the pair disagreeing with itself.**
Neither needs an answer file to know it failed, which is the single best property either of them
has, and §4 keeps it.

## 2. What CTP does — measured

### 2-1. The surface a case is given

`make_ha.sh` reads `$init_path/HA.properties`, sources `make_ha_upper.sh`, and leaves a case with
these verbs. Counted over the 373 cases:

| verb | cases | what it is |
|---|---|---|
| `setup_ha_environment` | 370 | create the database on both nodes, rewrite four conf files, upload them, start the heartbeat, poll `changemode` until active |
| `revert_ha_environment` | 368 | the inverse |
| `run_on_slave` | 248 | a command on the other node |
| `run_upload_on_slave` · `run_download_on_slave` | 100 · 48 | move a file |
| `wait_for_slave` | 131 | until the slave's state is what was asked for |
| `wait_for_active` · `wait_for_slave_active` | 81 · 13 | until `changemode` says active |
| `stop_slave_hb` · `start_slave_hb` | 52 · 74 | the heartbeat, on the other node |
| `stop_slave_service` | 43 | the service, on the other node |
| `slave_cmd` | 25 | as `run_on_slave`, with the engine's environment |
| `add_ha_db` | 9 | another database into `ha_db_list` |
| `format_hb_status` | 12 | make `hb status` comparable |

Three of those are real synchronisation — they poll a state until it is true, with a bound
(`wait_for_active`: `cubrid changemode` every second, 120 times). That is the right shape, and §2-3
is about how rarely it is used.

### 2-2. The oracle is the pair, and it is good

A representative case (`_22_ha/bug_xdbms3769`): write on the master, read the same rows from both
nodes into two files, `compare_result_between_files`. 169 of 373 cases call
`compare_result_between_files`; 313 read through `csql`. There is no answer file to re-record when
the engine's formatting changes, and no expected output to go stale — the comparison is between two
nodes running the same build.

**Keep this.** It is the property that makes HA testing cheap to maintain, and it is why `ha_repl`
was built the same way independently.

### 2-3. Synchronisation is time, not state

| | |
|---|---:|
| cases with a bare `sleep N` | **244 of 373** |
| bare `sleep` statements | **726** |
| **seconds slept, summed over the corpus** | **20,370 — 5 hours 39 minutes** |
| cases using a real wait (`wait_for_slave`/`wait_for_active`) | 136 |
| cases using **both** | 91 |

The corpus spends the better part of six hours asleep, and those sleeps are not a delay bolted onto
a correct test — **they are the synchronisation**. The representative case above writes on the
master, sleeps five seconds, and reads the slave. Whether replication had caught up is not checked;
it is assumed from the clock.

This is the same defect isolation was measured to have, with the same two consequences. It is slow,
and it is **wrong in both directions**: on a loaded machine the case fails for a reason that is not
the engine's, and on a fast one it passes without ever having waited for anything.

**And the fix is already in the corpus's own helper library.** `wait_for_slave` is not a timer — it
creates a table on the master, inserts the row `'replication finished'`, and polls the slave with
`-tillcontains "replication finished"` until it arrives (`make_ha_upper.sh:74-87`). That is exactly
the right mechanism, written years ago, sitting beside every case. `ha_repl` arrived at the same
shape independently, with a backoff and a configurable bound (`Test.java`,
`ha_sync_detect_timeout_in_ms`).

So this is not a missing capability. **136 cases use the real wait and 244 sleep instead**, and 91
do both — a case that calls `wait_for_slave` and then sleeps five seconds anyway. Nothing had to be
invented; the corpus simply did not converge on what it already had.

### 2-4. One corpus defect worth naming

`_22_ha/bug_xdbms3769` calls `wirte_nok` where it means `write_nok`. The branch that reports a
failure is a typo, so that comparison can only ever pass. One case of 373, found by counting every
spelling of the two verdict helpers — but it is the kind of defect an answer-file corpus would have
surfaced and this one cannot, because nothing checks that a case is capable of failing.

## 3. What it does not examine — measured

**No fault is ever injected into the network.** Over all 373 cases:

| | cases | tree-wide |
|---|---:|---:|
| `iptables` | **0** | **0** |
| `ip route` | **0** | **0** |
| `tc qdisc` | **0** | **0** |

Every fault the corpus can produce is process-level: `kill -9` (82 files), `cubrid hb stop` (129),
`cubrid service stop` (110). A node is stopped; a node is never *unreachable*. The distinction is
not academic — it is the difference between a node that is gone and a node that is running and
believes its peer is gone, which is where split brain lives.

**The states a half-finished transition passes through are invisible:**

| state | cases |
|---|---:|
| `registered_and_active` | 17 |
| `registered_and_standby` | 15 |
| **`to_be_active`** | **0** |
| **`to_be_standby`** | **0** |
| `fail_counter` | **0** |

The one appearance of `to_be_active` anywhere in the tree is inside a recorded `hb status` dump —
`_12_bts_issue/bug_bts_8896/cases/status1.answer` holds the line
`Server hatestdb (pid , state registered_and_to_be_active)`. The state is in the corpus by accident,
as a byte in a snapshot. Nothing waits for it, asserts on it, or times it.

`to_be_active` is not a corner case. The field's own tracker records a failover stopping there **for
hours** (`cluster-sandbox` `requirements/02-ha-role-transition-field-evidence.md`). The corpus has
no case that can see it.

**The parameters that decide when a cluster switches over are never varied:**

| parameter | cases |
|---|---:|
| `ha_calc_score_interval_in_msecs` | **0** |
| `ha_max_heartbeat_gap` | **0** |
| `ha_heartbeat_interval_in_msecs` | **0** |

These are the three that govern the decision, and `cluster-sandbox` measured over nineteen runs that
**the behaviour is not the documented arithmetic** — raising either heartbeat parameter fourfold
leaves the measurement inside its own baseline band, while `ha_calc_score_interval_in_msecs` moves
it by about 2× on means. A corpus that never varies them cannot have noticed.

**And the field's own words appear nowhere:** `unnecessary failover` — the tracker's first-named
failback problem — 0 files. `slave rebuild` — a script with a long history of trouble — 0 files.
`ha_ping_hosts` appears in 9, and every one of them writes the setting and greps a message; none
makes the ping host unreachable.

### 3-1. The sharper statement: the move is setup, never the subject

"No fault is injected" is true and it is the wrong headline, because it suggests the corpus leaves
the topology alone. It does not. Counted:

| | cases |
|---|---:|
| cases that move the topology (`hb stop`, `service stop`, `kill -9`) | **263 of 373** |
| of those, asserting **data equality** afterwards | **115** |
| of those, asserting on `hb status` or `changemode` | 22 |
| of those, **timing** the transition | **4** |
| using `wait_for_slave_failover` | 8 |

So 263 cases stop a node, and what they then do is ask whether replication still copied the rows.
**The transition is a precondition for a steady-state comparison, and almost never the thing being
examined** — 22 look at the states it passed through and 4 time it.

`ha_repl` does not move the topology at all: two or three of its twenty-odd Java files mention `hb
stop`, and those are deploy and cleanup. It is steady-state by construction.

**What that adds up to.** CTP's HA testing establishes that *replication copies rows across a
topology that is holding still*, including after it has been disturbed. That is a real and useful
thing, and it is most of what these two suites are for. What is missing is not only the fault
vocabulary — it is that **nothing examines the disturbance itself**, and the absent network faults
are one class of disturbance among several the corpus already performs and does not look at.

## 4. The specification

Seven properties. Each says what must be established, how it is observed, and what result would show
it worked — ADR-015's third criterion is that the evidence is declared before the work, so each row
is written to be falsifiable now rather than described later.

**They are in two groups, and the order is a decision** (2026-09-21). §3-1 is why: these suites test
a topology that is holding still, and that is a legitimate job rather than a shortfall. So the
properties that make *that* job correct come first; the ones that make the disturbance itself the
subject are a second axis and wait.

| | | |
|---|---|---|
| **A. The held-still topology** | P1 · P7 · P2's setup half | needs the corpus and the runner. No new environment capability, no second machine, no fault verb |
| **B. The moving topology** | P3 · P4 · P5 · P6 · P2's subject half | needs a provisioner that can cut a network — ADR-022's sandbox path |

Group A is the near work. Group B is admitted, specified, and deferred.

### Group A — the topology is holding still

**P1 — Replication is complete, and the wait is a wait.**
Every write accepted by the master appears on every slave. Observed by the existing oracle: the same
query on both nodes, compared. **Synchronisation is a poll on state, never a sleep** — a marker
written on the master and waited for on the slave, with a bound.

This is the one property where the work is not invention but **convergence**: `wait_for_slave`
already does exactly this, and 244 of 373 cases sleep instead (§2-3). The target is that the word
`sleep` is not a synchronisation point in any case this system runs, and that the corpus's 20,370
seconds fall to the time the engine actually takes. Since the frozen cases cannot be edited (NG1),
the runner has to be what closes the gap — see §5.

**P2 — A role transition completes, and its intermediate states are observable.**
A failover reaches `registered_and_active` on exactly one node. **`to_be_active` and `to_be_standby`
are states a test can wait for, assert on, and time**, because a transition that stops in one is the
field's reported failure and is currently invisible. A transition that does not complete within a
declared bound is a failure with the state it stopped in named in the verdict.

**This property is in both groups, and its halves separate cleanly.** *Waiting for a transition to
complete* belongs to group A and belongs there urgently: 263 cases perform one as setup and wait for
it with a sleep (§3-1), so when the setup transition has not finished, the steady-state comparison
that follows is measuring nothing and says so with a green verdict. *Timing the transition, and
asserting on the states it passed through*, is group B.

**P7 — A case cannot silently pass. — built, 2026-09-21**
Every case must be capable of failing. Checked mechanically, not by review: a case whose failure
path is unreachable — a misspelt `write_nok`, a comparison whose inputs are always identical — is a
defect in the case. §2-4 was one instance found by hand; nothing looked for more.

`testkit check-cases <scenario> [<init_path>]` now does. It reads the helper library to find which
helpers can put NOK into a result — transitively, because 169 HA cases reach their verdict only
through `compare_result_between_files` — and then reads each case for three things: no route to NOK
at all, a call one typo away from a verdict helper and defined nowhere, and a comparison of
something against itself. It runs no case and needs no engine.

**What it found on its first run:**

| | HA corpus (373) | shell corpus (3,475) |
|---|---:|---:|
| cannot fail at all | 0 | 0 |
| misspelt verdict call | **1** | **7** |
| self-comparison | 0 | **3** |

Every one was read back against the source. The typos are all the same shape — `wirte_nok`,
`write_no`, `test_exec_commanr`, `compare_result_between_file` — and all but one sit in the `else`
branch of an `if`, which is to say **in the only line that would have reported the failure**. The
three self-comparisons are `plan.result` against `plan.result`, `group_concat_max_len.result`
against itself where its four sibling lines each compare a result against an answer, and
`test.answer` against `test.answer` in a case whose own comment says *"this result is not same as
the previous one. We expect they are same."*

The check earned its keep during development too: its first version matched identifiers anywhere and
reported `db_stats=` (an assignment) and `exec_csql.exp` (a filename). Both are regression tests now.
Reporting a typo is an accusation, so that rule reads command position only and under-approximates;
*can this case fail* over-approximates, because the safe direction is opposite for each.

### Group B — the topology is the subject

Admitted and specified so the evidence is declared in advance, deferred so that group A is not held
behind a provisioner.

**P3 — A partition is a fault the tests can produce.**
Unreachability between two nodes, with the mechanism named — a dropped route and a dropped packet
are different engine code paths and both must be expressible. This is the whole class CTP has zero
coverage of. `cluster-sandbox` already provides it (`fault partition`, and ADR-002 operation 8 makes
it portable across backends), so this is a capability to *use*, not to build.

**P4 — Split brain is reachable on purpose, and is a verdict.**
Two nodes both believing they are master is a state the tests can create and must detect. It is not
hypothetical: a correctly configured cluster reaches it in 9 s when the ping host survives the
partition (`cluster-sandbox` `findings/split-brain.md`). The specification is that a test can ask for
it, and that any test which reaches it **unintentionally fails**.

**P5 — Divergence is detected even when every gauge says healthy.**
After a healed partition there is a window in which both nodes accept writes, and the rows that
crossed in one direction leave a permanent difference that the engine's own status reports as
healthy (`findings/active-active-window.md`). So the oracle cannot be the engine's opinion of
itself: it has to be the data. The pair-comparison of §2-2 is already this, and P5 is the
requirement that it runs **after** a fault and not only after a clean write.

**P6 — The switchover decision is measurable, and the parameters are inputs.**
`ha_calc_score_interval_in_msecs`, `ha_max_heartbeat_gap` and `ha_heartbeat_interval_in_msecs` are
test inputs, not fixed configuration. What is recorded is a distribution over repeated runs, not one
number, because the measurement's own spread is wider than some of the effects — which is what made
four years of work on this stall on reproducibility rather than on knowledge
(`requirements/02-ha-role-transition-field-evidence.md`).

### What is deliberately not here

**A new case language.** `.ctl` earned its place in isolation because the thing being expressed —
interleaving — has no natural shell form. HA's verbs are `stop this`, `cut that`, `wait for this
state`, and shell expresses those. What HA needs is not a new grammar but **honest waits and a fault
verb**, and both are functions.

**Anything that requires the frozen corpus to change.** NG1 holds. §5 is how that is squared.

## 5. Parity first — what this does not skip

ADR-015's second criterion. The 373 cases stay, run as they are, on the two-machine path, and their
verdicts are the baseline everything here is measured against. Nothing in §4 ships before that
baseline exists — **and it does not exist yet**, which makes it the next piece of work regardless of
this document.

> **Amended 2026-09-23 — and a Group A and Group B suite has shipped without it.** What changed is
> not the appetite for the baseline; it is what the baseline turned out to be. CTP's `ha_repl`
> reaches the sql corpus through a conversion that deletes every statement beginning with `CALL` and
> every `SELECT` that does not say `INCR` or `DECR`, so a parity comparison against it is a
> comparison against a corpus the findings' own statements are not in
> ([`evidence/ha/ctp-ha-repl-deletes-the-call.md`](../evidence/ha/ctp-ha-repl-deletes-the-call.md)).
> [ADR-015](../adr/ADR-015-beyond-axis.md) was amended on the same day to say the criterion does not
> apply where the baseline cannot express the question, and what ships instead is each finding's own
> reproduction. **The two-machine baseline is still owed** — it is what the 373 frozen shell cases
> need, and [ADR-022](../adr/ADR-022-topology-provider.md) Consequence 3 says the same. It stopped
> being a gate on `ha_repl`; it did not stop being work.

The two groups then arrive by different routes.

**Group A is a patch set, and the mechanism is already built.** A `sleep` cannot be deleted from a
frozen case — but a run has been able to carry corpus changes it does not own since the shell suite
needed them, and isolation's set is the same defect with the same fix:

```
delete_select_06.ctl :32    MC: sleep 1;   ->   MC: wait until C2 ready;
```

HA is that shape again: `sleep 5` becomes `wait_for_slave`, and **`wait_for_slave` is already in the
corpus's own helper library** — 136 cases call it while 244 sleep (§2-3). Everything the mechanism
needs exists: `case_patch_dir`, the apply-and-revert around a run, the guard that fails when a patch
stops applying, and the `PATCHED` report that names every case whose verdict came from patched
source. The set itself lives outside this repository — `cubrid-testkit-patches`, named by
`TESTKIT_PATCHES` — which is where an HA set would go too, and for the same reason: an HA patch
carries context lines from a private corpus.

A patch that replaces a sleep with the wait sitting beside it is also the kind that goes upstream
unchanged, and a patch that stops applying is how this side finds out it landed.

P7 needs even less: it reads the corpus and reports cases whose failure path cannot be reached. No
machine at all.

**Group B is new cases**, outside the frozen corpus, and they need a provisioner that can cut a
network — ADR-022's sandbox path, and the node flavour `evidence/ha-topology.md` §3 asks for.

So the near work needs **no new environment capability**: not the four node additions, not a fault
verb, not a second machine. It needs the corpus, the patch layer and the runner. The one thing it
does need a pair for is the *verification* — whether removing a sleep leaves every verdict where it
was — and that is the two-machine baseline above, which is owed anyway.
