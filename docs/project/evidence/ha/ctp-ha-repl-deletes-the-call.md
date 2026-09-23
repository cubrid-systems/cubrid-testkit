# CTP's ha_repl conversion deletes every CALL, so a procedure's writes are never replicated — or asked about

- **Date:** 2026-09-23
- **Status:** read from CTP's source and measured against the corpus. **Not run**: no CTP ha_repl
  run was made to confirm the converted corpus in use today is the one this migration produces.
- **What this is:** why the findings in this directory are new. They are not new because nobody
  looked; they are new because the only tool that ran this corpus across a pair **removes the
  statements that show them** before the run starts.
- **Raised by:** the observation that a `CALL` is not only a catalog method — it is also how a
  PL/CSQL procedure runs, and a procedure body holds INSERT, UPDATE and DELETE.

---

## The rule

`CTP/ha_repl/src/com/navercorp/cubridqa/ha_repl/migrate/SQLFileReader.java`, `shouldBeDeleted`:

```java
if (n1.startsWith("AUTOCOMMIT")) return true;
if (n1.startsWith("ROLLBACK"))   return true;
if (n1.startsWith("COMMIT"))     return true;
if (n1.startsWith("$"))          return true;
if (n1.startsWith("SHOW"))       return true;
if (n1.startsWith("CALL"))       return true;
if (n1.startsWith("SELECT")) {
    if (n1.indexOf("INCR") == -1 && n1.indexOf("DECR") == -1) return true;
}
```

A deleted statement is not written to the converted case at all, and for a statement spread over
several lines the decision is made on the **first** line and applied to all of them. What survives
gets CTP's own checkpoint appended (`--check:` / `@HC_CHECK_FOR_EACH_STATEMENT`), which is the
oracle: CTP compares the nodes at its checkpoint, not by the case's own reads — the case's own
reads are the `SELECT`s it just deleted.

## What that costs, counted on `cubrid-testcases/sql`

| | |
|---|---:|
| cases | 17,447 |
| cases holding at least one `CALL` | **1,410** |
| `call … on class …` — catalog methods | 1,707 statements |
| every other `CALL` — a procedure or a method | **10,257 statements in 1,037 cases** |
| of those cases, ones that also define a procedure or function | 1,004 |
| **cases whose only data change is inside a procedure body** | **26** |

The last row is the sharp one. In those 26 the conversion keeps the table, keeps the procedure, and
deletes the one statement that would have put a row anywhere.

`_05_plcsql/_01_testspec/_05_bug_fix/cases/30_plcsql_var_in_static_sql_insert_error.sql`, in full,
is the shape:

```sql
CREATE TABLE athlete (...);                               -- kept
CREATE OR REPLACE PROCEDURE insert_athlete(...) AS
BEGIN
    INSERT INTO athlete (...) VALUES (...);               -- kept, as text
    COMMIT;
EXCEPTION WHEN OTHERS THEN ROLLBACK;
END;
call insert_athlete('test', 'M', 'KOR', 'test');          -- DELETED
select * from athlete;                                    -- DELETED
drop procedure insert_athlete;                            -- kept
drop table athlete;                                       -- kept
```

Converted, the case creates a table and a procedure, drops both, and never writes a row. The slave
is asked nothing, and the case passes.

## The same door on the SELECT side, and the rule already knows about it

`CALL` is not the only way a deleted statement writes. A `SELECT` can call a user-defined function,
and a PL/CSQL function body holds DML like any other. Measured on the pair rather than argued:

```sql
create table fl(c1 int primary key, tag varchar(20));
CREATE OR REPLACE FUNCTION f_write(n INT) RETURN INT AS
BEGIN
    INSERT INTO fl VALUES (n, 'from the function');
    RETURN n;
END;
select f_write(1) from db_root;
select f_write(2) from db_root;
```

Both rows are on the master **and on the slave**. The engine does exactly the right thing: the
INSERT inside the function is ordinary data replication. What the conversion removes is the only
statement that makes it happen, so the pair is never asked about rows it would have carried
correctly.

**The rule already concedes the principle.** Its `SELECT` clause is not unconditional:

```java
if (n1.startsWith("SELECT")) {
    if (n1.indexOf("INCR") == -1 && n1.indexOf("DECR") == -1) return true;
}
```

`INCR` and `DECR` are kept because a `SELECT` containing them writes. So "a SELECT can change data"
was understood, and the exception was drawn around the two functions that did it in 2012. A call to
a user-defined function belongs in that exception and is not in it.

How much it costs, counted separately because the evidence differs:

| | |
|---:|---|
| **2 cases, 3 SELECTs** | a PL/CSQL function whose body writes, called from a `SELECT` — the body is in the `.sql`, so the DML is visible and this count is solid |
| **30 cases, 235 SELECTs** | a Java stored function called from a `SELECT` — the body is Java and not in the corpus file, so **whether these write is not established here** |

Small beside the `CALL` side, and the principle is the same one: the conversion decides what may be
deleted by the first word of a statement, and two of the words it deletes can run arbitrary DML.

## The rule is from 2012 and PL/CSQL arrived after it

`shouldBeDeleted` carries a 2012 comment (`added by cn15209 2012.08.07`), when `CALL` in this
corpus meant a method on a class — the legacy object feature this directory's other findings are
about. A procedure that runs arbitrary DML was not a thing `CALL` did.

**It was looked at since, and the rule was not.** The last change to that file is
`eabb85c 2023-12-20 [CUBRIDQA-1204] Improve CTP to support PL/CSQL in ha_repl (#667)`, which adds a
461-line `LineScanner` for exactly one purpose: to recognise PL/CSQL text so that a procedure's
**body** passes through the conversion verbatim instead of being mangled. It does not touch
`shouldBeDeleted`. So the change that made these cases survive the conversion left in place the
line that stops them doing anything.

## What follows, for this project

**The baseline is weaker than "parity before beyond" assumes** (ADR-015). Whatever CTP's ha_repl
reports about this corpus, it reports it about a corpus with every `CALL` and almost every `SELECT`
removed. A comparison against it is a comparison against that, and the number owed
([`README.md`](README.md)) should be read with this beside it.

**It explains the method-call family exactly.** `call … on class` does not replicate
([`method-calls-on-the-catalog-do-not-replicate.md`](method-calls-on-the-catalog-do-not-replicate.md)),
and 1,707 such statements are deleted before any CTP run sees them. Three findings in this
directory sit behind that one rule.

**It does not explain the other two.** The session-variable finding
([`ddl-with-a-session-variable-does-not-replay.md`](ddl-with-a-session-variable-does-not-replay.md))
is an `INSERT` and a `CREATE`, both of which the conversion keeps, so a CTP run could meet it. The
object-domain one is visible only through a `SELECT`, which the conversion deletes — but CTP's own
checkpoint might still catch the row.

**And it is an argument for this suite's oracle.** Keeping the case's own reads and comparing them
across the pair is what makes a deleted statement impossible: there is nothing to delete, because
nothing is rewritten.

## What is not claimed

That any CTP deployment today runs a corpus produced by this migration. The shipped
`conf/ha_repl.conf` points `scenario` at `${HOME}/cubrid-testcases/sql`, the raw corpus, and the
runtime helper that would add checkpoints to a raw file — `TestReader.getNextLine()` — is
`@Deprecated`. Which of the two a real run uses is a question for a CTP run, and that run is the
baseline this directory still owes.

Nor is it claimed that deleting `CALL` was careless. It was right for the corpus of 2012. What is
measurable is that it is wrong for the corpus of today, and that the change which noticed PL/CSQL
did not revisit it.
