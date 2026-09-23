# The cases `ha_repl` cannot judge, and why CTP could not judge them either

- **Date:** 2026-09-22
- **Question:** the suite reports 24 cases as establishing nothing — `no_data` and
  `unreplicatable`. Would CTP's `ha_repl` establish something on them?
- **Answered by reading, then corrected by running.** The reading said CTP would not judge them
  either. **It was wrong**, and running CTP is what showed it — see
  [§ The correction](#the-correction). Running it at all first required removing the `kill -9`
  in CTP's own cleanup; see [§ Why it could not be run before](#why-it-could-not-be-run-before).

---

## The correction

**CTP judges all of them, and this document first said it would not.**

The reasoning was that CTP's oracle is the case's own reads compared across nodes, so a case
with no read offers nothing to compare. That half is true. What it missed is that **CTP has a
second oracle**, and it does not depend on the case at all:

```java
// Test.java:937
private boolean waitDataReplicated (SSHConnect ssh, long expectedFlagId) {
  spt += "csql -u dba " + hostManager.getTestDb()
       + " -c \"select 'GO'||'OD-'||v FROM QA_SYSTEM_TB_FLAG \" | grep GOOD";
  ...
  if (result.indexOf ("GOOD-" + expectedFlagId) != -1) { ... }
```

`QA_SYSTEM_TB_FLAG` is CTP's own table. It writes a flag on the master and polls the slave for
it, **once per case, whatever the case contains**. So a case that reads nothing still gets a
verdict: did replication carry CTP's marker across while this case ran.

That is a liveness check on the pair rather than a check of the case, and it is a thing this
suite does not do — although it already has the parts. `Pair.WaitForReplication` writes and
polls exactly such a marker; the suite runs it and then, for a case with no reads, reports
`no_data` and throws the result away. **A `no_data` case whose wait succeeded has established
something small and real.**

## The run

Three attempts, the last on a pair whose replication was verified working first. **10 cases
executed, all ten NOK.** The run was stopped after that; it was not going to say anything new.

| | |
|---|---:|
| `Wait data replicated ... GOT` | **271** |
| `Wait data replicated ... FAIL` | **0** |
| `Fail. Retry to compare.` | **239** |
| database rebuilds | 1 |
| cases executed / passed | 10 / **0** |

**The liveness oracle passes every time.** 271 checks, no failures — the pair carries CTP's flag
across, every case. (In the first attempt it failed on all ten, with the slave holding an older
flag than the one being waited for; that was the pair still catching up after repeated rebuilds,
and it resolved: master and slave now read the same value, and an update on one appears on the
other.)

**What fails is the per-statement comparison**, and the statements it fails on are the ones the
engine refuses. The first one in the log:

```sql
create class test (
  testno    char(10) PRIMARY KEY not null,
  ...
  primary key(testdate)
)
```

Two primary keys — a deliberate negative test. CUBRID rejects it, so nothing reached the slave
and nothing was going to. CTP compares, finds a difference, and **retries, because it reads a
difference as the slave not having caught up yet**. It cannot tell "the slave is behind" from
"there was nothing to be behind on", so it retries until it gives up and the case is NOK.

That is the distinction this suite draws by comparing only reads and skipping statements that
produce no result. It is why the same cases come back `no_data` here and NOK there.

**Neither verdict is a finding about replication.** CTP's is noise on a case whose point is that
a statement is refused; this suite's is silence on the same case. The difference is which one a
reader has to investigate.

### And one case no runner should be given

`xdbms32.sql` stalls it. The case is real — it creates a table with a primary key — but the data
it inserts is **63 KB of HTML documentation, 2,810 tags**, and CTP retries the comparison on
every line of it. Two attempts stopped there before it was removed from the scenario. It also
defeats this suite's statement splitter, which reads it as 170 statements and no reads, because
the HTML carries both `;` and unbalanced apostrophes.

## The 24 still execute no read, and that part stands

How it was established, three ways.

Counting reads with the suite's own splitter is one reading of the corpus, so it was checked
against the text and then against the engine.

| | |
|---|---:|
| cases whose text does not contain the word `select` | 18 |
| cases that contain it inside `create view ... as select`, `insert ... select`, `merge ... using (select)`, a prepared-statement string, or HTML documentation text | 5 |
| cases that contain a standalone `select` | **1** |

The one is `_06_manipulation/_04_insert/cases/1009.sql`, and it is the interesting one. Its first
line is `--[er]test insert with invalid use of single quote`, and line 6 is:

```sql
insert into tb values(''a');
```

The quote is deliberately unbalanced, so everything after it is inside a string literal that never
closes — including the `select`. **csql agrees**, and says so in its own error:

```
ERROR: In line 7, column 25 before '');
select * from tb;
drop class tb; '
Syntax error: unexpected 'a', expecting ',' or ')'
```

The `select` is quoted *inside* the message as part of the broken statement. It does not run, for
csql and therefore for anything that drives csql.

## What the 23 are

Fourteen of them write and never read: create, insert, drop. Nine write nothing at all. Both are
cases whose subject is the statement being accepted or refused, not the data that results — which
is what [`module-ha.md`](../../design/module-ha.md) predicted of the conversion: *"a case whose
point is the rendering has nothing to say here … converting one is not wrong, it is empty."*

## Why it could not be run before

CTP's `ha_repl` deploy calls `clean_processes` on **every node, master included**, and
`CTP/common/script/util_common.sh:38` is:

```bash
function kill_process {
   all_pids=$(calc_pids "$$")
   kill -9 `ps -u $USER -o pid | grep -v "PID" | grep -E -v "$all_pids" | grep -vw $$`
}
```

`kill -9` on every process the invoking user owns, excepting only itself and its own ancestors.
On a two-machine pair whose master is a workstation, that is the operator's ssh session, their
editors, and anything else they were running. It was run twice here before the cause was found,
and both times it took the session down with it. It also stopped the podman sandbox on the same
host, because conmon belongs to the same user.

**Removed, 2026-09-22.** `kill_process` and the two helpers that existed only to serve it are
gone from `util_common.sh` on both machines of this pair, and `clean_processes` now stops at
`cleanCUBRID`, `releaseSharedmemory` and the monitor. Verified that `upgrade.sh` does not
restore them (`[INFO] SKIP TO UPDATE`, md5 unchanged), and then verified the way that matters:
the run completed the deploy, stood HA up on both nodes and executed ten cases, and the
operator's session was still there afterwards.

Two more things a reader will need. `cubrid_download_url` must be **absent**, not a placeholder:
`Main.java:72` treats any value as a request to install and calls
`setReInstallTestBuildYn(true)`, so `file:///dev/null` got as far as running the installer, which
refused it — and left `buildId` null, which is an NPE two steps later. And the HA parameters have
to be given, because nothing else supplies them:

```
env.instance1.cubrid.ha_mode=on
env.instance1.ha.ha_node_list=cubrid@hgryoo-desktop:hgryoo-notebook
env.instance1.ha.ha_db_list=xdb
```

## What is not claimed

That CTP would agree case for case — nothing was run. The claim is narrower and rests on the
oracle being the same shape: a case that executes no read offers neither runner anything to
compare, and 23 of the 24 execute no read.
