# 2. Writing a case

[← back to the shell category](README.md)

The corpus this runner is tested against — `cubrid-testcases-private-ex` — is private, so this
document is what you need to write cases without reading it.

- [The layout, and why the name is repeated](#the-layout-and-why-the-name-is-repeated)
- [A case you can copy and run](#a-case-you-can-copy-and-run)
- [The four lines every case starts and ends with](#the-four-lines-every-case-starts-and-ends-with)
- [Recording a verdict](#recording-a-verdict)
- [Comparing against an expected file](#comparing-against-an-expected-file)
- [What a case may assume](#what-a-case-may-assume)
- [Skipping a case on a platform](#skipping-a-case-on-a-platform)
- [Traps](#traps)

## The layout, and why the name is repeated

```
<scenario>/
  └── my_first_case/                    ← the case directory. Its name is the case name
        └── cases/
              ├── my_first_case.sh      ← THE CASE. Named after the directory two levels up
              ├── my_first_case.result  ← written by the run. Do not commit it
              ├── expected.answer       ← whatever the case wants to diff against
              └── helper.sh             ← NOT a case. Any other name is a helper
```

**The repeated name is a rule, not a convention.** CTP's `do_check_more_errors` derives the result
file from the *directory* name and the runner derives it from the *script* name:

```
result_file_full_name=${test_case_dir%/cases*}/cases/${case_name}.result
```

The two agree only when the script is named after its directory. A script that is not named that way
becomes a case whose verdict is read from a file nobody wrote — so the discovery rule is exactly:

```
<scenario>/**/<name>/cases/<name>.sh
```

and every other `*.sh` under `cases/` is a helper. This is what keeps `PrintInfo.sh`, `common.sh` and
`build.sh` from being run as tests.

**A directory the run cannot read is skipped, and named.** A run as root leaves databases and logs
under a case's `cases/` as `root:root 700`, and the run's uid cannot enter them. Discovery prints a
`[WARN]` with the path and goes on with every case it could list. A case inside such a directory is
not found, so the warning is worth reading. The workspace copy, when `testcase_workspace_dir` names a
directory other than the scenario, does the same.

Any directory depth above the case directory is yours: group cases however you like.

## A case you can copy and run

This is a complete, working case. It creates a database, asks it a question, records a verdict, and
cleans up.

```bash
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
```

Put it at `<scenario>/my_first_case/cases/my_first_case.sh` and run it — [Running
it](03-running-it.md) has the two commands. It leaves:

```
my_first_case-1 : OK
19:11:06----<scenario>/my_first_case/cases--- time=13
```

## The four lines every case starts and ends with

```bash
. $init_path/init.sh     # the helpers. $init_path is exported for you
init test                # sets cur_path, the result file, and case_no = 1
set -x                   # so a failure is readable. Every case in the corpus does this
...
finish                   # stops the service, releases broker shared memory,
                         # restores every conf the case changed
```

`$init_path` is set by the runner before your script runs, along with `CTP_HOME` and a `PATH` that
has CTP's `bin` and `common/script` on it. You do not set them and you should not.

`finish` is not optional. It stops the service, `pkill cub`s what is left, releases broker shared
memory and restores conf files — and a case that skips it leaves the slot dirty for the next case.

## Recording a verdict

One script can record **several** verdicts: `case_no` starts at 1 and each call increments it.

```bash
write_ok                            # my_first_case-1 : OK
write_ok "created 4 volumes"        # my_first_case-2 : OK created 4 volumes
write_nok                           # my_first_case-3 : NOK
write_nok "expected count(*) = 2"   # my_first_case-4 : NOK expected count(*) = 2
write_nok some_file.log             # a filename is inlined into the result
```

They append to `$case_name.result`, which is what the runner reads to decide the verdict. **The
whole case is NOK if any line is NOK.**

`write_nok` also greps `$CUBRID/log/server/*.err` for `Internal Error` and appends what it finds, so
a server-side fault lands next to the failure that revealed it.

## Comparing against an expected file

```bash
compare_result_between_files actual.log expected.answer
```

It diffs the two and calls `write_ok` or `write_nok` for you. Two optional arguments:

```bash
compare_result_between_files actual.log expected.answer error       # ignore line numbers
compare_result_between_files actual.log expected.answer error sort  # sort both first
```

**Argument order.** The verdict is the same either way — a diff is a diff — but the order decides
which side of the output is which, and the corpus is not consistent about it. Read the case rather
than assuming: `compare_result_between_files a.log a.answer` puts *actual* first, which is the
common form, and the opposite exists too.

**It needs `$CUBRID/qa.conf` to pick version-specific answers.** `get_best_compat_file` reads
`Server_Version` and `CCI_Version` from it, so a case may have `expected.answer`,
`expected.answer.11.2` and so on and the right one is chosen. Without `qa.conf` the plain file is
used.

## What a case may assume

| | |
|---|---|
| the working directory | its own `cases/` directory. Every case begins there |
| `$CUBRID` | an install with no databases of its own, restored before this case ran |
| `$CUBRID_DATABASES` | `$CUBRID/databases`, with a `databases.txt` and nothing in it |
| the port | 1523 is free, and belongs to this case alone |
| processes | no CUBRID process is running |
| `$init_path`, `$CTP_HOME`, `$PATH` | set, with CTP's scripts reachable |
| `/bin/sh` | bash-compatible, even on a distribution where it is dash |
| the commands | `java`, `javac`, `diff`, `wget`, `find`, `cat`, `kill`, `tar`, `expect` — the run checks them once and refuses a machine that is missing one |

**Each case gets the machine back as it was.** The slot is reset before every case, so a case may
change `cubrid.conf`, create databases and leave files behind — the next case does not see them.
That also means a case may not depend on anything an earlier case left.

## Skipping a case on a platform

Put the bare word in the script and set `testcase_exclude_by_macro` in the conf:

```bash
LINUX_NOT_SUPPORTED
```

They are no-op shell functions in `init.sh`, so the line is harmless when the macro is not being
filtered on. `WINDOWS_NOT_SUPPORTED` and `AIX_NOT_SUPPORTED` exist too.

To skip a case by path instead, put a fragment of its path in a file and point
`testcase_exclude_from_file` at it — see [`../../../overrides/machine-exclusions/README.md`](../../../overrides/machine-exclusions/README.md),
which also explains why this machine's list is kept apart from upstream's.

A file named there that does not exist stops the run. CTP read nothing from it and ran every case it
was meant to keep out.

## Traps

**Timing lines make a literal diff flaky.** `csql` prints `1 row selected. (0.001000 sec)`, and that
number is different every run. Either grep for what you mean, as the example does, or strip the
volatile lines before comparing.

**`cubrid createdb` is not `cubrid_createdb`.** The bare utility requires a locale argument;
`cubrid_createdb` is CTP's wrapper and supplies it from the conf's `cubrid_db_charset`. Use the
wrapper.

**A case that leaves a server running holds the slot.** `finish` stops it. If a case must exit
early, call `finish` before it does.

**A missing tool fails quietly.** 216 cases drive an interactive `csql` through an
`expect` script and then grep its log. Without `expect` the case still runs, the `.exp` script does
not, and the grep counts zero — a plain NOK, indistinguishable from a wrong answer. That is why the
run checks for it at the start rather than letting each case discover it.

**Do not commit `.result` files.** The run writes them into the overlay, not into the repository,
but a stray one committed from a manual run will confuse the next reader.

**Do not write outside the case directory** unless you mean it. The overlay drops writes under the
scenario when the directory finishes; writes elsewhere are the slot's and go away with it, which is
usually what you want but is worth knowing.
