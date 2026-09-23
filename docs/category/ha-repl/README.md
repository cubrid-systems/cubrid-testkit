# The `ha_repl` category

*English · [한국어](README.ko.md)*

`ha_repl` is a task of its own. It takes the `sql` corpus, runs it on a master, and checks that the
slave ends up holding the same thing — so it is the `sql` suite's question asked across a pair
instead of against an answer file.

The other thing that wears the word HA is [`ha-shell`](../ha-shell/README.md). They share their
oracle and almost nothing else:

| | **[`ha-shell`](../ha-shell/README.md)** | **`ha_repl`** |
|---|---|---|
| what it asks | does replication survive *this* — a stopped node, a restarted service, a rebuilt slave | does replication copy *this SQL* correctly |
| who moves the topology | the case, 263 of 373 do | **nobody.** Steady-state by construction |
| who builds the pair | the case, by calling `make_ha.sh` | **the runner** |
| corpus shape | shell scripts | the `sql` corpus, converted |
| frozen | yes | **no** — the conversion is not |

| | |
|---|---|
| **[2. Writing a case](02-writing-a-case.md)** | what the conversion changes, and the one rule that carries over |

**There is now a native runner**, behind `TESTKIT_NATIVE=ha_repl`, and the rest of this document is
its as-built guide. Without that switch the task still goes to CTP as a subprocess, unchanged — see
[What runs where](../../../README.md#what-runs-where).

The gate is the unusual part, and it is worth knowing before you read further: this runner is
**not** waiting on a parity comparison with CTP, and
[ADR-015](../../project/adr/ADR-015-beyond-axis.md) was amended to say why. CTP reaches the same
corpus through a conversion that deletes every statement beginning with `CALL` and every `SELECT`
that does not contain `INCR` or `DECR`. Parity against that is a statement about a different corpus
— and the cases' own reads, which are this runner's oracle, are among the statements it removes.

---

## What this suite is for

**It establishes that replication is correct for a statement, with the topology held still.** Not
that the topology survives a disturbance — that is [`ha-shell`](../ha-shell/README.md)'s job, and
`ha_repl` does not do it at all: two or three of its twenty-odd Java files mention `hb stop`, and
those are deploy and cleanup.

That makes it the narrower and the sharper of the two. A failure here is a replication defect for a
specific SQL construct, with nothing else moving that could explain it.

## The oracle, arrived at twice

A case runs on the master, and then **the two nodes are asked the same question and their answers
compared**. Nothing is checked against a recorded answer.

The two implementations ask it differently, and the difference matters:

| | asks the pair by |
|---|---|
| CTP's runner | dumping both nodes and diffing the dumps |
| this runner | re-running **the case's own reads** on both nodes and diffing the answers |

Either way it is the same oracle as [`ha-shell`](../ha-shell/README.md#the-oracle-and-why-it-is-the-good-part),
and the two suites reached it **independently**. That is the strongest evidence available that it is
the right one for HA: there is no expected output to go stale, nothing to re-record when the
engine's formatting changes, and the comparison is between two nodes running the same build.

It is also what the conversion is *for*. The `sql` corpus's cases each carry an `.answer` file
recording what running them produced; converting a case to `ha_repl` means **the answer file stops
being the oracle** and the slave takes its place.

Keeping the case's own reads has one further consequence worth stating plainly: there is no list of
statements to leave out, because nothing is rewritten. CTP's conversion has such a list, and what is
on it is [a finding of its own](../../project/evidence/ha/ctp-ha-repl-deletes-the-call.md).

## Its wait, which is the same wait

`Test.java` writes a flag on the master, polls the slave for `GOOD-<id>`, backs off between
attempts, and gives up at `ha_sync_detect_timeout_in_ms`.

CTP's `wait_for_slave` on the shell side does the same thing by a different route, and
`internal/sandbox.WaitForReplication` is a third implementation written to match both rather than
to improve on them. Three arrivals at one shape is why
[`module-ha.md`](../../project/design/module-ha.md) §4 P1 states it as a property rather than a
preference: **synchronisation is a poll on state, never a sleep.**

## The native runner in one paragraph

You point it at a corpus and at a master/slave pair. For each case it clears the database, runs the
case's statements on the master, waits for a marker of its own to appear on the slave, and then
**re-runs the case's own reads on both nodes and compares the answers**. No answer file, no dump
diff: the oracle is the two nodes disagreeing.

That last part is the difference from CTP's runner, which dumps both nodes and compares the dumps.
Keeping the case's reads is what makes a deleted statement impossible — there is nothing to delete,
because nothing is rewritten.

## The shortest possible run

A pair comes from [`cluster-sandbox`](../extensions/README.md) (`csb`), which is pinned at
`extensions/cluster-sandbox`. The runner never builds a topology; it asks for one
([ADR-022](../../project/adr/ADR-022-topology-provider.md)).

```bash
csb cluster create --cluster hadb --build /path/to/CUBRID --preset ha

cat > /tmp/ha.conf <<'EOF'
scenario=/path/to/cubrid-testcases/sql/_01_object
sandbox_cluster=hadb
EOF

TESTKIT_NATIVE=ha_repl testkit ha_repl -c /tmp/ha.conf
```

It prints one line per case as it is judged, then a summary and a per-pair tally.

## The verdicts, and why there are nine of them

A case does not simply pass or fail here. Replication can be correct, incorrect, *not applicable*,
or not asked — and collapsing those would report CUBRID's design as a defect.

| verdict | what it means |
|---|---|
| `same` | the case wrote, the wait returned, and every user table reads identically on both nodes. This is the one the suite exists to produce |
| `differ` | **the finding.** Contents disagree *after* replication was waited for — which is the difference between "in flight" and "not going to arrive" |
| `replicating` | the case wrote, made no comparable read, and its write was followed across. Less than `same`, more than nothing: the pair was replicating while it ran |
| `no_data` | the case is read-only. Much of the `sql` corpus is, and the honest claim is that nothing happened on either node |
| `unreplicatable` | every read touches a table with **no primary key**. CUBRID's data replication is keyed on the primary key, so those rows were never going to cross. Not a failure — a question this corpus cannot ask |
| `session_differs` | the two nodes ran the reads as different users, so the answers are not comparable. Usually a symptom of a real replication problem, but not itself a verdict about one |
| `wait_timeout` | replication did not arrive inside the bound. Reported apart from `differ` because the remedy is not the same one |
| `case_failed` | the master refused the case's own SQL. Not a replication finding — the case never got far enough to make one |
| `skipped` | the case's semicolons are not all statement terminators, so splitting it would run fragments. Named and counted rather than attempted or hidden |

## Configuration

| key | default | what it does |
|---|---|---|
| `scenario` | — | the corpus root. Every `*.sql` under a `cases/` directory is a case |
| `sandbox_cluster` | — | the pair. **A comma-separated list runs the corpus over several pairs at once** — see below. `TESTKIT_CSB_CLUSTER` overrides it |
| `ha_sync_detect_timeout_in_ms` | `60000` | the bound on the marker crossing. The wait itself is a poll on state, never a sleep |
| `add_primary_key` | `no` | give a keyless `CREATE TABLE` a generated primary key, so its rows can replicate at all. Off by default because the unconverted run is the baseline the conversion has to be measured against, and a switch that is on by default hides it |
| `reset` | `case` | `case` clears the database before every case; `dir` clears it at a directory boundary, which is what the `sql` corpus's own contract assumes |
| `resume` | `no` | skip the cases a previous run already judged (see the ledger) |
| `status_http` | off | `on` serves a live status page on `:51523`; a value of `host:port` or `:port` chooses another |
| `difference_dir` | beside the conf | where differences, the ledger and the reports are written |

## Running several pairs from one run

`sandbox_cluster=a,b,c,d` drives four pairs from **one process**.

This is deliberately not four processes. testkit's front end is one test at a time by design — a
result tree takes one run's lock (`internal/result/lock.go`) — so eight invocations would be eight
runs arguing over one record. Inside one process there is one record, one page and one ledger, and
the pairs are the only thing that is several.

**How the corpus is split depends on `reset`**, and this matters more than it looks:

- Under `reset=dir` a directory is dealt whole, because that is the unit whose cases may rely on
  each other. One large directory then sets the floor on the run: `_01_object`'s biggest holds 867
  of its 3,327 cases, so four pairs and sixteen pairs finish at the same time.
- Under `reset=case` — the default — cases are dealt one at a time. The database is cleared between
  every case anyway, so the dependence a whole directory was protecting has already been broken by
  the runner itself, and there is nothing left to keep together.

Measured on this corpus: 290 cases over 13 directories take 142s on one pair and 31s on eight.
Per-case cost rises about 1.5x under eight-way concurrency on a sixteen-core host, which is the
gap between that and a clean 8x.

**A pair wears out**, and it is worth knowing before you compare timings. The per-case reset drops
the schema; it does not give back the volumes the database grew or the replication copy log. A pair
that has run tens of thousands of statements costs several times more per case than a fresh one, and
in a sharded run the worn pair sets the wall clock
([`evidence/ha/a-sandbox-pair-wears-out.md`](../../project/evidence/ha/a-sandbox-pair-wears-out.md)).

**It also costs disk that nothing gives back, so ask what it is costing:**

```
$ csb cluster ls
NAME                 STATE    CONTAINERS  DISK      HOST               LABELS
sh7                  yes      2           7.5G      hgryoo-desktop     -
sh11                 yes      2           9.5G      hgryoo-desktop     -
```

Eleven pairs here reached 53 GB and took the filesystem to 98%. The filesystem's own total could not
say which pair to remove, which is why `cluster ls` reports it per cluster. **This runner does not
destroy a pair**: it asks for a topology and never builds one (ADR-022), so a pair it did not create
is not its to remove. `csb cluster destroy` is that verb, and rebuilding takes about a minute.

## What a run leaves behind

Everything lands under `difference_dir`.

**`verdicts.tsv` — the ledger.** One line per case, appended and flushed as the verdict is reached:
case, outcome, detail, milliseconds. It exists because a run over 3,327 cases is better than an hour
and an hour is long enough to be interrupted by something unrelated — twice on 2026-09-22 every
CUBRID process on the host stopped in the same millisecond. `resume=yes` reads it back and skips
what was judged, **except** `wait_timeout` and `case_failed`, which are verdicts about the run rather
than about the case: resuming them would carry the interruption's damage into the next run's tally
and call it a measurement.

**A directory per difference**, holding the statement, both nodes' raw output, and a normalised diff
— so a `differ` can be read without rerunning anything.

**`slave-stranded.tsv`**, when a case leaves an object behind on a slave that the reset could not
remove. The suite repairs those where it safely can and **records that it had to**, because a
reader otherwise cannot tell a corpus that meant to diverge from one that diverged by accident.

## The status page, and watching a run that is already going

`status_http=on` serves a page on `:51523` for the life of the run: a lane per pair, the case each
is on, and a panel per pair carrying both nodes' role, HA state, applied and copied page IDs, lag,
`fail_counter`, the faults in force, and the artifact the cluster was built from. Those are on the
page because their absence cost something — a pair that stopped serving once turned an hour of a run
into `wait_timeout` per case, honestly reported and unnoticed.

If a run is already going and you did not turn the page on, or you closed it:

```bash
testkit watch
```

It finds the `ha_repl` runs on this machine by reading `/proc` for their own command lines, and
serves one page covering all of them. Only a process whose argv actually says `ha_repl` and names a
readable conf is picked up — a watcher that invented a run would draw a lane that never fills.

## Faults

`internal/sandbox` exposes csb's fault verbs — `partition`, `splitbrain`, `clear` — so a scenario
can cut the pair and ask the same question of a topology that moved. What has been measured with
them is in
[`evidence/ha/split-brain-divergence-converges.md`](../../project/evidence/ha/split-brain-divergence-converges.md).

## Findings so far

[`project/evidence/ha/`](../../project/evidence/ha/README.md) is the index. They are findings about
CUBRID, not about this runner, and each carries its own reproduction.
