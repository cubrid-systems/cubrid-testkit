# The `isolation` category

*English · [한국어](README.ko.md)*

`isolation` is CTP's concurrency corpus — 6,790 cases, each a script that drives two or more clients against one
database so that a lock wait, a deadlock or a visibility rule happens in an order the script decides — and the third
family testkit rewrote. Every case is still executed by CTP's own `runone.sh` and ctltool; everything around a case is
the runner's. These documents are the as-built guide.

| | |
|---|---|
| **[1. How a run works](01-how-a-run-works.md)** | the stages, what `runone.sh` does with a case, the verdict, and what a slot is here |
| **[2. Writing a case](02-writing-a-case.md)** | the `.ctl` language, where answers live, and what a case should not rely on |
| **[3. Running it](03-running-it.md)** | from an install and a corpus to a verdict, and what a run leaves behind |
| **[4. Configuration](04-configuration.md)** | every key and switch the runner reads |
| **[5. When a case fails](05-when-a-case-fails.md)** | reading a verdict, the cases CTP cannot reproduce either, and the ones that pass only while the controller is slow |

The design, the decisions and what was measured on the way are
[`../../project/design/module-isolation.md`](../../project/design/module-isolation.md),
[ADR-007](../../project/adr/ADR-007-isolation-executor.md) (the executor),
[ADR-018](../../project/adr/ADR-018-isolation-equivalence.md) (the gate) and
[`../../project/evidence/isolation-baseline.md`](../../project/evidence/isolation-baseline.md).

## In a paragraph

A run checks the machine, lists the `.ctl` files under `scenario`, and opens one slot or several. A slot is a set of
namespaces with `$CUBRID`, ctltool's directory and — with `scenario_disk` — the cases tree behind overlays of its own,
and it creates its own `ctldb`. That matters more here than for any other family: `runone.sh` kills every `cub`,
`sleep`, `qactl` and `qacsql` the user owns, and in a slot the user owns only the slot. The slot hands each case to
`runone.sh`, and the verdict is read from what the script printed.

## The shortest possible run

With `$CUBRID` pointing at an install, `$CTP_HOME` at a CTP tree, and `cubrid-testcases` checked out:

```bash
cat > /tmp/demo/isolation.conf <<'EOF'
scenario=/path/to/cubrid-testcases/isolation
testcase_timeout_in_secs=300
testcase_retry_num=4
testcase_exclude_from_file=/path/to/cubrid-testcases/isolation/config/daily_regression_test_excluded_list_linux.conf
EOF

TESTKIT_NATIVE=isolation TESTKIT_CONTAIN=1 testkit isolation -c /tmp/demo/isolation.conf
```

```
Available Env: [local]
Build Id: 11.5.0.2574-f1ae86f
Build Bits: 64bits
BEGIN TO CHECK: 
…
The Number of Test Case : 6772
============= DEPLOY ==================
DONE
============= TEST ==================
STARTED
[ENV START] local
[ENV START] local
[ENV START] local
[ENV START] local
[TESTCASE] /path/to/cubrid-testcases/isolation/_01_ReadCommitted/…/x.ctl EnvId=local [OK]
…
============= PRINT SUMMARY ==================
Test Category:isolation
Total Case:6790
Total Execution Case:6772
Total Success Case:6758
Total Fail Case:14
Total Skip Case:18

TEST COMPLETE
```

That is CTP's output, line for line, but for one `[ENV START]` and `[ENV STOP]` a slot: the run took four slots, the
default, and about fifty minutes where one slot takes three and a half hours. [Running it](03-running-it.md#slots)
says how many slots a machine gets, and [when a case fails](05-when-a-case-fails.md) which of the fourteen are not
about the build under test.
