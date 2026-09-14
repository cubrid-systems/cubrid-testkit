# The `sql` category

`sql` and `medium` are CTP's two SQL corpora — 17,459 cases and 975 — and the second family testkit
rewrote. These documents are the as-built guide.

| | |
|---|---|
| **[1. How a run works](01-how-a-run-works.md)** | the stages, the executor, and what a slot is here |
| **[2. Writing a case](02-writing-a-case.md)** | the layout, where an answer comes from, and the rules a case should follow |
| **[3. Running it](03-running-it.md)** | from an install and a corpus to a verdict |
| **[4. Configuration](04-configuration.md)** | every key and every switch, what it costs, and what to set |
| **[5. Slots and speed](05-slots-and-speed.md)** | what parallel buys, what it costs, and why medium is serial |
| **[6. When a case fails](06-when-a-case-fails.md)** | reading a verdict, and the failures that are not the engine's |

The pre-implementation design — the CQT class mapping, the ADRs, and what was measured on the way —
is [`../../design/module-sql.md`](../../project/design/module-sql.md); the evidence is
[`../../evidence/sql-native.md`](../../project/evidence/sql-native.md) and
[`../../evidence/regression-sql.md`](../../project/evidence/regression-sql.md).

## In one picture

```
   conf (sections) ─────┐
                        v
                  ┌───────────┐
   corpus ───────►│  testkit  │──────► $CTP_HOME/sql/result/y2026/m9/schedule_…
   (cases and     │ sql·medium│        main.info · summary.xml · summary_info
    answers)      └─────┬─────┘        the JUnit report · a .result beside
                        │              every case — CQT's bytes, exactly
        ┌───────────────┼───────────────┐
        v               v               v
    ┌────────┐     ┌────────┐      ┌────────┐        one database, prepared once,
    │ slot 0 │     │ slot 1 │  …   │ slot N │        under an overlay per slot, and
    │  JVM   │     │  JVM   │      │  JVM   │        a JVM that lives for the run
    └────────┘     └────────┘      └────────┘        and runs CQT's own executor
```

A slot is a set of namespaces with its own server and broker, and `$CUBRID` behind an overlay whose
lower layer is the one database the setup prepared. Every slot keeps the shipped port, so nothing is
reconfigured. A directory of cases is claimed whole by one slot, because cases in a directory depend
on what their neighbours leave behind.

## The shortest possible run

With `$CUBRID` pointing at an install, `$CTP_HOME` at a CTP tree, and a corpus checked out:

```bash
cat > /tmp/demo/sql.conf <<'EOF'
scenario=/path/to/cubrid-testcases/sql
test_category=sql
jdbc_config_file=test_default.xml
db_charset=en_US

[sql]
parallel_slots=1
EOF

TESTKIT_NATIVE=sql TESTKIT_CONTAIN=1 testkit sql -c /tmp/demo/sql.conf
```

```
Result Root Dir:/path/to/CTP/sql/result/y2026/m9/schedule_linux_sql_64bit_1312570267_11.5.0.2568-c3967ec
[14:31:26] Testing /path/to/cubrid-testcases/sql/_01_object/_01_type/_001_nchar_nvarchar/cases/1001.sql (1/17459 0.01%) [OK]
…
Fail:0
Success:17459
Total:17459
```

That is CTP's output, line for line: [when a case fails](06-when-a-case-fails.md) explains what the
run writes beside it, and [slots and speed](05-slots-and-speed.md) how to make it take five minutes
instead of twenty-nine.
