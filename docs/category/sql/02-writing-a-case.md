# 2. Writing a case

[← back to the sql category](README.md)

- [The layout](#the-layout)
- [A case you can copy](#a-case-you-can-copy)
- [Where the answer comes from](#where-the-answer-comes-from)
- [What the rendering looks like](#what-the-rendering-looks-like)
- [The pragmas the corpus uses](#the-pragmas-the-corpus-uses)
- [Answers for other conditions](#answers-for-other-conditions)
- [Rules a case should follow](#rules-a-case-should-follow)

## The layout

A case is a file of SQL and a file of expected text, in sibling directories:

```
sql/_13_issues/_26_1h/
├── cases/cbrd_26541.sql       the statements
└── answers/cbrd_26541.answer  what running them produced
```

The names match, the extension is `.sql`, and nothing else in `cases/` is a case — a `.txt` helper is
ignored, and `answers/` and `common/` are not walked for cases at all. The directory is the unit a
slot claims, so a directory is also the unit whose cases may rely on each other.

## A case you can copy

```sql
--+ holdcas on;
--[er]length() of an attribute that does not exist

create table t_demo (b varchar(10));
insert into t_demo values ('22');

select length(b) from t_demo;
select length(no_such_attribute) from t_demo;

drop table t_demo;
--+ holdcas off;
```

Three things in it are conventions rather than syntax: `--+ holdcas on` … `off` around the body,
which 6,821 cases use; a first-line comment saying what the case is for, with `[er]` when the point
of the case is an error (2,911 cases do); and a name that is an issue id where there is one.

## Where the answer comes from

**Run the case and keep what it produced.** Do not write an answer by hand: the rendering has a shape
— separators, headers, the exact spacing of values, `Error:-<code>` for a statement that failed — and
a hand-written file will differ in a way that has nothing to do with the behaviour being tested.

```bash
mkdir -p /tmp/mycorpus/_99_mine/{cases,answers}
cp t_demo.sql /tmp/mycorpus/_99_mine/cases/
: > /tmp/mycorpus/_99_mine/answers/t_demo.answer     # empty: the case will fail, on purpose

sed 's#^scenario=.*#scenario=/tmp/mycorpus#' sql.conf > /tmp/mycase.conf
TESTKIT_NATIVE=sql TESTKIT_CONTAIN=1 testkit sql -c /tmp/mycase.conf

cp /tmp/mycorpus/_99_mine/cases/t_demo.result /tmp/mycorpus/_99_mine/answers/t_demo.answer
```

The second run passes, and the answer is a recording of this engine on this corpus. Read it before
keeping it: an answer is what the case will assert forever, and anything in it that the case did not
mean to assert — a row order, a plan, an error another case caused — is a failure waiting for a
machine that does things in a different order.

CQT does have a mode that writes answers for you, and CTP's entry point does not reach it: the agent
sets the result mode and nothing offers the other. Taking the `.result` is the supported way.

## What the rendering looks like

What the case above produces, run:

```
===================================================
0
===================================================
1
===================================================
char_length(b)    
2     

===================================================
Error:-494
===================================================
0
```

- a line of fifty-one `=` before each statement's output;
- `0` for a statement that returns no rows — the `create` and the `drop` — and the number of rows an
  insert touched;
- for a query, the column names and then the rows, every value followed by five spaces, and a blank
  line after the last row. The header is the expression as written, so `length(b)` comes back as
  `char_length(b)`: the answer records the engine's naming, not the case's;
- `Error:-<code>` when the statement failed. The message under it appears only when a
  `--+ server-message on` hint asked for it.

The comparison takes every `\r` and `\n` out of both sides and then wants equality. So where the
lines break is not asserted, and neither is a blank line — but everything else is: the trailing
spaces after a value, and the order the rows came back in.

## The pragmas the corpus uses

Counted in this corpus rather than listed from a manual:

| | uses | |
|---|---:|---|
| `--+ holdcas on` / `off` | 6,821 | hold the connection's transaction open across statements; the idiom is to wrap the case |
| `--+ server-message on` / `off` | 2,559 | print an error's message under its code. **It stays on** until something turns it off, which is why a slotted run puts it back where CTP's order would have it before every case |
| `evaluate '<text>'` | 2,924 | write a label into the output, so a long case says which step each block belongs to |
| `--@queryplan` | 586 | append the statement's plan to the output |
| `--@fullplan` · `--@joingraph` | 10 · 2 | more of the same, rarely |
| `$<type>, $<value>, …` on the line before a statement | 1,000+ | bind values for the `?` in it, type first: `$varchar, $1, $varchar, $json_array('aa')` |

`@conn:` appears in CTP's parser and in **no case in this corpus**, so nothing here depends on it.

## Answers for other conditions

A case can carry more than one answer, and the run picks by its configured mode and by the database's
charset and collation:

```
answers/1001.answer                         the default
answers/1001.answer_cci                     335 of these: the CCI driver renders differently
answers/1001.answer_D_utf8_C_utf8_bin       the database's charset and collation
```

2,423 such files exist. Which one a run uses comes from the `<run_mode>` element of the XML that
`jdbc_config_file` names — and when that element is absent, the run uses the plain `.answer`.

## Rules a case should follow

These are not style: each one is a measured cause of a case failing when the cases before it change.

- **Clean up what you create.** 1,922 cases in this corpus create a table and never drop it, and 245
  of those names are created by more than one directory — `t1` is left behind by 61 directories and
  created by 423. A case that drops what it made cannot be the reason another case fails.
- **Create what you need, and drop it first if the name is common.** `drop table if exists t1;` at
  the top costs one line of output and removes a whole class of failure.
- **Order what you read.** A `select` from a catalog with no `ORDER BY` returns rows in the order the
  catalog happens to hold them, which depends on everything created before. Sort by something that
  decides the order completely — for an index listing, that means the key's position too.
- **Put back what you change.** A `set system parameters` lives on the connection, and so do session
  variables — CUBRID holds **twenty** of them, so a case that leaves five spends the next case's
  budget. Drop your variables and restore the parameter at the end.
- **Do not read what is not yours.** A listing of `_db_user` includes whether some other case gave a
  user a password; a trace shows the server's own queries. Scope what the case selects to what the
  case made.

A case that follows all five runs the same whatever ran before it — which is what makes a corpus
parallelisable. Where the corpus does not follow them,
[patches](06-when-a-case-fails.md#carrying-a-corpus-fix-as-a-patch) carry the fix until upstream
takes it.
