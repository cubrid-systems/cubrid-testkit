# 3,327 cases: seven differences, three causes, and one of them was the suite's own mess

- **Date:** 2026-09-23
- **What this is:** the scale run. `sql/_01_object`, twenty-five times larger than anything this
  suite had run before, against a sandbox pair. It was run to answer one question — whether the
  known non-replicating shapes are the whole list — and the answer is no: it found two rules
  nobody here had written down, and confirmed a third across a second route.
- **Trees:** engine `11.5.0.2513-5f3a30d` from `cubrid/cubrid` develop · pairs `pmha` (the run) and
  `gbha` (every re-measurement), cluster-sandbox, rootless podman, one host.
- **Runner:** testkit's `ha_repl`, `add_primary_key=yes`, `reset=case`, `resume=yes`.

---

## The run

| | |
|---|---:|
| cases | **3,327** |
| same | 2,308 |
| **differ** | **7** |
| replicating | 943 |
| unreplicatable | 67 |
| wait_timeout · case_failed | 1 · 1 |
| statements | 14,155 |
| **reads compared across the pair** | **2,256** |
| reads skipped for want of a primary key | 8 |
| reads skipped for an object-domain column | 1 |
| statements rewritten to carry a generated key | 903 |
| writes the engine refused | 998 (626 on a NOT NULL constraint) |
| **agreeing reads where the master returned no rows either** | **356** (38 in a case that was never going to show one) |
| time spent waiting for replication | 7 m 53 s — and never slept |

**Read the 356 before the 2,256.** One agreeing read in six agreed about nothing, which is the
same discount this suite reported on 131 cases and it has not improved with scale.

Two things about the run itself, said here rather than left for someone to find:

- **It ran in two segments**, 23:15-00:55 and 00:55-01:59, resuming 2,435 cases from the ledger.
  The statement and read counters above are the second segment's own cases only; the verdict tally
  is all 3,327. **The second segment was not started by the session that did this work** — the
  ledger is what made an unattended continuation safe, and this note is what makes it honest.
- **The tail was running over a poisoned pair.** A case left `test_trigger` and its owner
  `TEST_USER` on the slave, the repair refuses a trigger because it will not invent a definition,
  and so every reset after it reported the same pair of objects — sixty-odd times. Anything judged
  after that is suspect, which is why **every one of the seven differences was re-run on a clean
  pair before being believed.**

## Seven differences, three causes, one artefact

| case | what differs | cause |
|---|---|---|
| `_02_class/_003_auto_increment/cubridsus-965` | `db_class.owner_name`: `PUBLIC` / `DBA` | **1** |
| `_10_system_table/_021_db_authorizations/1006` | `db_class.owner_name`: `TEST_USER2` / `DBA` | **1** |
| `_10_system_table/_012_db_auth/1011` | `db_auth`: one grant / none | **1**, one step removed |
| `_10_system_table/_021_db_authorizations/1001` | `_db_user.password`: `_db_password` / `NULL` | **2** |
| `_10_system_table/_004_db_attribute/1003` | `db_attribute` for `test_class2`: rows / none | **3** |
| `_10_system_table/_004_db_attribute/1007` | `db_attribute` for `test_class`: rows / none | **3** |
| `_10_system_table/_012_db_auth/1015` | `select * from dba.test_class`: none / two rows | **artefact** |

### Cause 1 — a catalog method call does not replicate, and the DDL form of the same change does

Measured on a clean pair, three statements:

```sql
call add_user('mu1') on class _db_user;   -- master: MU1        slave: absent
create user mu2;                          -- master: MU2        slave: MU2
```

That is the rule this suite first met as `change_trigger_owner`
([`trigger-owner-change-not-replicated.md`](trigger-owner-change-not-replicated.md)) and then as
`change_owner` on a class ([`class-owner-change-not-replicated.md`](class-owner-change-not-replicated.md)).
`add_user` is the third route and it is not about owners at all, so the finding is not "owner
changes by method" but **`call … on class` into the catalog**: no statement to replay, and an
instance update on a catalog class that has no primary key, so no data channel either.

**And it does not stop at the object it made.** `1011` is the same cause one step removed: the case
creates its user with `add_user`, so the user is missing on the slave, so the `GRANT` to that user —
a statement that replays perfectly well on its own — **fails on the slave for want of a grantee**:

```sql
call add_user('mu1') on class _db_user;
create table mt(c1 int primary key);
grant select on mt to mu1;
-- db_auth on the master: MU1 mt SELECT        on the slave: nothing
```

A plain `grant select on gt to gu1` where the user was made with `create user` replicates. So a
difference in `db_auth` is not evidence about GRANT; it is evidence about what made the grantee.

### Cause 2 — an object reference still does not cross, and `_db_user` is where it shows

`1001` reads `_db_user`: the master's `TEST_USER` has a `_db_password` and the slave's has `NULL`.
That is [`object-domain-not-replicated.md`](object-domain-not-replicated.md) exactly — a column
whose type is another class replicates its row and not its reference — in a system catalog rather
than a user table. The password object is the reference; the user row arrives without it.

### Cause 3 — a DDL whose text names a csql session variable cannot be replayed

New, and the one that surprised. `1003` and `1007` build a class whose column default is an object
held in a **csql session variable**:

```sql
insert into test_class values(999) into :arg1;
create class test_class2(col1 integer, col2 test_class SHARED :arg1);
```

Statement replication replays the statement's *text*. `:arg1` is session state of the csql that ran
it, the applier has no such variable, and so the `CREATE` fails on the slave and the class is simply
not there. Measured, five statements, with a control in the same batch:

```sql
set system parameters 'create_table_reuseoid=no';
create class oc(c1 int);
insert into oc values(1) into :v1;
create class oc2(c1 int, c2 oc SHARED :v1);   -- master: yes   slave: NO
create class oc3(c1 int, c2 oc);              -- master: yes   slave: yes
```

`oc3` is the control: an object-domain column with no variable in its text replicates fine, so this
is about the variable and not about the domain.

### The artefact — and why the re-run exists

`1015` differed with **rows on the slave and none on the master**, which is the signature of
something the reset could not remove from the slave rather than of anything the case did. On a
clean pair it comes back `same`. One difference in seven was the suite's own residue, and the only
reason that is known is that all seven were re-run somewhere clean.

## What this changes

**The list of shapes this suite skips does not grow.** None of the three causes is a "shape a read
touches"; they are things a *case does* whose effect never reaches the slave. Skipping reads would
hide them, which is the opposite of what is wanted.

**`call … on class` deserves its own name in the corpus.** Three routes measured, three failures,
and the corpus uses at least four of these methods (`add_user`, `drop_user`, `change_owner`,
`change_trigger_owner`). A case that uses one is a case whose later statements are running against
two different databases, and the divergence outlives the case: `_db_user` and `_db_class` have no
primary key, so nothing the reset does on the master can take the object off the slave
([`class-owner-change-not-replicated.md`](class-owner-change-not-replicated.md) for what the suite
does about that).

**Cause 3 is a limit of statement replication itself**, not of a catalog class. Anything whose SQL
text depends on the session that issued it — a variable, and possibly more — cannot be replayed
elsewhere. This suite met it through a corpus that uses `INTO :var`; whether anything else in CUBRID
puts session state into replayable DDL is unmeasured.

## What is not claimed

One build, one host, two pairs. The 943 `replicating` cases wrote and made no comparable read, so
they establish that replication was alive and not that their data matches; the 356 empty agreements
establish nothing at all. Whether the three causes hold across a failover, or for the methods this
corpus does not call, is unmeasured. The poisoned tail is a reason to re-run `_10_system_table`
alone on a clean pair before quoting anything from it beyond the seven differences, all of which
were checked.
