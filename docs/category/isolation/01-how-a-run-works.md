# 1. How a run works

*English · [한국어](01-how-a-run-works.ko.md)*

[← back to the isolation category](README.md)

- [The stages](#the-stages)
- [A case](#a-case)
- [The verdict](#the-verdict)
- [What a slot is here](#what-a-slot-is-here)

## The stages

The order is CTP's `TestFactory`, and so is every line it prints.

| stage | what it does |
|---|---|
| start | reads the build from `cubrid_rel`, resolves `scenario` — relative to `$HOME` when it is under it, which is how case paths in the records become relative too |
| `BEGIN TO CHECK` | three variables, five commands, the scenario and ctltool's directory, into `check_local.log` and on the console. A machine that fails it stops with exit 255 |
| `UPDATE TEST CASES` | nothing: pulling cases and upgrading CTP are not this runner's job. CTP also made ctltool's scripts executable here; the runner does that in each slot, below |
| `FETCH TEST CASES` | every `*.ctl` under the scenario, sorted, less the exclusion file's entries — each entry takes out the first case whose path contains it, and only that one. `dispatch_tc_ALL.txt` |
| slots | one or `parallel_slots`, each with its overlays ([below](#what-a-slot-is-here)) |
| `DEPLOY` | in every slot: `chmod u+x` on ctltool's scripts, CTP's process sweep, `inquire_on_exit=3` appended to `cubrid.conf`, and the `default.*` engine parameters written into the install's conf files |
| `TEST` | each slot takes cases until the queue is empty ([a case](#a-case)); at the end, `cubrid service stop` |
| end | the summary, `TEST COMPLETE`, and `isolation_result_<build>_<bits>_0_<stamp>.tar.gz` beside the run directory |

A resumed run (`test_continue_yn=yes`) fetches first — `dispatch_tc_ALL.txt` less every `dispatch_tc_FIN_*.txt` — and
checks the machine after, as CTP's did; it appends to the logs instead of starting them.

## A case

The runner sends one script per case, as CTP did — CTP's own prologue, `export ctlpath=${CTP_HOME}/isolation/ctltool`,
and then:

```
ulimit -c unlimited
export TEST_ID=0
cd $ctlpath
sh runone.sh  -r 5 /path/to/case.ctl 300 qacsql 2>&1
```

`-r` is `testcase_retry_num` plus one; the last argument is the client program, and the database is always `ctldb`.
`runone.sh` then does, per attempt:

1. **clears** core files under `$ctlpath`, `$CUBRID` and the case's directory, and empties every file under
   `~/CUBRID/log`;
2. **sets up** when `qactl` is missing or `ctldb` is not running: `cubrid service stop`, `pkill -9 cub`, deletes and
   creates `ctldb` in `$CUBRID/databases`, starts it, and builds `qactl` and `qacsql` with `make`;
3. **runs** `<name>.sql` through csql if the case has one;
4. **executes** `timeout3.sh -t <timeout> qactl ctldb <case> qacsql` into `result/<name>.result`;
5. **normalizes** a copy into `result/<name>.log` — fifteen `sed` steps ([writing a case](02-writing-a-case.md#answers));
6. **judges**: `<name>.sh` if the case has one, otherwise `answer/<name>.answer*` in `ls` order, the first identical
   one winning — `flag: OK` or `flag: NOK`;
7. **looks for cores** and `FATAL ERROR` in `$CUBRID/log`, and for the crash report a dying server writes — the runner's
   own check, which fails the case and keeps the report and a core's stack with the run
   ([ADR-021](../../project/adr/ADR-021-crash-reports.md)). `backup_core_file_yn=yes` hands the check back to
   `runone.sh`, which backs the cores up with the whole install into `~/error_backup` and recreates `ctldb`;
8. **cleans**: kills leftover `sleep`, `qactl` and `qacsql` and the transactions still open, and drops the users, the
   tables with foreign keys, triggers, serials, functions (all but the `sleep` ones), procedures, views and tables in
   `ctldb`, through seventeen `csql` calls.

It stops at the first attempt that passes.

## The verdict

The runner reads the verdict from the script's output as CTP's `Test.java` did: **the last `flag: NOK` after the last
`flag: OK` fails; any `found core file` or `found fatal error` fails; no `flag: OK` at all fails** (`Not found OK
word.`). For a failure it then diffs the base answer against the normalized result, 185 columns wide, into
`feedback.log`.

What a script printed is its standard output between two markers, trimmed — the rule CTP's SSH layer applied even to
the local machine. Standard error does not reach the verdict; `runone.sh`'s own trace does, because the command ends
`2>&1`.

## What a slot is here

A slot is shell's slot — PID, IPC, network and mount namespaces, every slot on the shipped ports — with overlays over:

| | why |
|---|---|
| `$CUBRID` | `ctldb`, the logs and the conf line Deploy appends are the slot's |
| `$CTP_HOME/isolation/ctltool` | `make clean qactl qacsql`, and the logs `runone.sh` keeps in its working directory |
| the cases tree, with `scenario_disk` | `result/` and `<name>.result`, which otherwise land in the tree as under CTP |
| `~/error_backup` | a bind to the slot's own directory, copied into the real one when the slot closes |
| `~/CUBRID/log` | an empty directory, when that is not the install under test's |

Slots share one queue and one EnvId, so a parallel run writes the same files a serial one writes. Each slot builds
ctltool and creates `ctldb` at its first case.
