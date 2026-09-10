# The `shell` category

`shell` is CTP's largest test category and the first one testkit rewrote. These documents are the
as-built guide.

| | |
|---|---|
| **[1. How a run works](01-how-a-run-works.md)** | the stages, the module structure, and what a slot is |
| **[2. Writing a case](02-writing-a-case.md)** | the file layout, the helpers, and a case you can copy and run |
| **[3. Running it](03-running-it.md)** | on a host and in Docker, from an empty directory to a verdict |
| **[4. Configuration](04-configuration.md)** | every key, what it costs, and what to set |
| **[5. The memory ceiling](05-the-ceiling.md)** | the one setting that fails a run rather than slowing it |

The pre-implementation design — the old Java class mapping and the ADRs behind the rewrite — is
[`../../design/module-shell.md`](../../design/module-shell.md).

## In one picture

```
     conf ──────────────┐
                        v
                  ┌───────────┐
   scenario ─────►│  testkit  │──────► result files      dispatch_tc_ALL.txt
   (read-only)    │   shell   │        (frozen: same     test_status.data
                  └─────┬─────┘         bytes CTP wrote) feedback.log · …
                        │
        ┌───────────────┼───────────────┐
        v               v               v
    ┌────────┐     ┌────────┐      ┌────────┐            each slot has its own
    │ slot 0 │     │ slot 1 │  …   │ slot N │            PID · IPC · mount · net
    └────┬───┘     └────┬───┘      └────┬───┘            namespace, so every one
         │              │               │                keeps the shipped port
         └──────────────┴───────────────┘                1523 and nothing is
                        │                                reconfigured
                        v
              overlay upper layer
              (tmpfs, if scenario_ram_mb)
```

The corpus is never written to. Every case's writes land in an overlay whose lower layer is the
scenario as the repository has it, and whose upper layer is memory when `scenario_ram_mb` is set.
When a directory's last case finishes, its writes are dropped.

## The shortest possible run

```bash
mkdir -p /tmp/demo/scenario/my_first_case/cases

cat > /tmp/demo/scenario/my_first_case/cases/my_first_case.sh <<'EOF'
#!/bin/bash
. $init_path/init.sh
init test
set -x

cubrid_createdb --db-volume-size=20M --log-volume-size=20M demodb
cubrid server start demodb
csql -u dba demodb -c "CREATE TABLE t(i INT); INSERT INTO t VALUES (1),(2);"
csql -u dba demodb -c "SELECT count(*) FROM t;" > actual.log 2>&1

if grep -qE "^ +2$" actual.log; then
    write_ok
else
    write_nok "expected count(*) = 2"
fi

cubrid server stop demodb
cubrid deletedb demodb
finish
EOF

cat > /tmp/demo/shell.conf <<'EOF'
scenario=/tmp/demo/scenario
test_category=shell
feedback_type=file
testcase_retry_num=0
parallel_slots=1
EOF

TESTKIT_CONTAIN=1 TESTKIT_NATIVE_SHELL=1 testkit shell -c /tmp/demo/shell.conf
```

```
Total Case:1
Total Success Case:1
Total Fail Case:0
```

[Writing a case](02-writing-a-case.md) explains every line of that script; [Running
it](03-running-it.md) explains the environment it needs.
