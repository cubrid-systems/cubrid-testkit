# A DDL whose text names a csql variable cannot be replayed, and this one the engine does report

- **Date:** 2026-09-23
- **Status:** measured, discriminated, and **explained by the applier in its own words**. The
  slave's log names the statement and the reason.
- **What this is:** the second cause behind the scale run's seven differences, and the one that is
  not [the method-call family](method-calls-on-the-catalog-do-not-replicate.md). It is a limit of
  statement replication itself rather than of any catalog class.
- **Trees:** engine `11.5.0.2513-5f3a30d` from `cubrid/cubrid` develop · pair `gbha` from
  `cubrid-cluster-sandbox`, rootless podman, one host.
- **Found by:** `_01_object/_10_system_table/_004_db_attribute/1003` and `1007`, which came back
  `differ` with rows on the master and none at all on the slave.

---

## The reproduction, with its control

```sql
set system parameters 'create_table_reuseoid=no';
create class oc(c1 int);
insert into oc values(1) into :vobj;      -- a variable holding an object
select 7 into :vnum from db_root;         -- a variable holding a number

create class d_obj  (c1 int, c2 oc  SHARED :vobj);
create class d_num  (c1 int, c2 int SHARED :vnum);
create class d_plain(c1 int, c2 int SHARED 7);
```

Every one of them is accepted on the master. After a marker crosses:

| class | its default | master | slave |
|---|---|---|---|
| `d_obj` | a variable holding an object | yes | **absent** |
| `d_num` | a variable holding the number 7 | yes | **absent** |
| `d_plain` | the literal 7 | yes | yes |

**So it is the variable and not what the variable holds.** `d_num` is the discriminating case: its
default is a plain integer, and it is missing on the slave for the same reason `d_obj` is. `d_plain`
is the control that says the `SHARED` default itself is fine.

That also settles what `1003` and `1007` are. They were read as one more consequence of a user the
slave never got; they are not. The corpus writes

```sql
insert into test_class values(999) into :arg1;
create class test_class2(col1 integer, col2 test_class SHARED :arg1);
```

and the class is absent on the slave with no user, no owner change and no method call anywhere in
the case.

## Why, and the engine says it

Statement replication replays a DDL statement's **text** (`la_apply_statement_log` →
`la_update_query_execute`, `log_applier.c:5657`). A csql variable is state of the csql process that
ran the statement. The text crosses; the variable does not. From the slave's
`applylogdb` error log, verbatim:

```
log applier: failed to apply the following statement. class: "[dba.d_obj]",
  statement: "create class d_obj(c1 int, c2 oc SHARED :vobj)",
  server error: -494, Semantic: ... Unknown variable vobj.
log applier: failed to apply the following statement. class: "[dba.d_num]",
  statement: "create class d_num(c1 int, c2 int SHARED :vnum)",
  server error: -494, Semantic: ... Unknown variable vnum.
```

Nothing is guessed here: the applier prints the statement it tried, prints that the variable is
unknown, and moves on.

## This is the first one in this directory that is reported

Every other finding here ends with the same sentence — the applier logs nothing, `fail_counter`
does not move, the pair calls itself healthy. **Not this one.** The applier logs the statement and
the reason, and the counter moves: `fail_counter` was 48 on the slave after these runs.

Two things follow from that, and they pull in opposite directions:

- **An operator can find this.** It is in the log with the SQL that failed, which is more than an
  object-domain NULL or a method-call owner change ever gives them.
- **`fail_counter` is now ambiguous for this suite.** It counts these, and it counts the failed
  `CREATE` this runner's own slave repair issues on purpose
  ([`class-owner-change-not-replicated.md`](class-owner-change-not-replicated.md)). A non-zero
  counter after a run of this suite has to be read against `slave-stranded.tsv` and the applier log
  before it means anything about the engine.

## What the suite does with it

Nothing yet, and deliberately. It is not a shape a read touches, so there is nothing to skip: it is
a statement a case runs whose effect never arrives. Reporting it as `differ` is correct, and the
case's later reads differ for a real reason — the class is not there.

What would help a reader is the applier's own line beside the verdict, which this runner does not
collect. That is worth doing and is not done.

## What is not claimed

One build, one pair, one host. Only `SHARED` defaults were measured, and only in `CREATE CLASS`.
Whether other DDL can carry a csql variable into its stored text — `DEFAULT`, a partition bound, a
check constraint — is unmeasured, and so is whether anything outside csql's `INTO :var` puts
session state into a statement the applier will try to replay.
