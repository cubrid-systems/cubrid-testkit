# 1. How a run works

[← back to the sql category](README.md)

- [The stages](#the-stages)
- [The executor](#the-executor)
- [What a slot is here](#what-a-slot-is-here)
- [What the run writes](#what-the-run-writes)

## The stages

CTP's `sql/bin/run.sh` is six stages in a row, and this runner is the same six. The first three and
the last are shell — the script is CTP's, transcribed close enough to diff — and the fourth is Go.

| stage | what it does |
|---|---|
| `do_init` | the database's name (`basic`, or `mdb` for medium) and the category |
| `do_clean` | stops and deletes the database, kills this user's `cub` processes, removes its shared memory |
| `do_configure` | writes the conf's `[sql/cubrid.conf]` and broker sections into the engine's own files, and forces `en_US.utf8` for sql on 11.5 and later |
| `do_create_db` | `createdb`, the locale, and the data: stored procedures compiled and loaded for sql, `mdb.tar.gz` unpacked and loaded for medium |
| `do_test` | the cases — [the executor](#the-executor), and Go for everything around it |
| `do_summary_and_clean` | the summary files, the search for core files, and the clean-up |

The first three run once, in the runner's own namespaces, before any slot exists. The database they
make is at `$CUBRID/databases/<db>`, which is inside `$CUBRID` — so every slot's overlay has it as
its lower layer. Prepared once, copied never, and at the same path in every slot, so nothing in
`databases.txt` needs rewriting.

`do_clean` deletes. The paths it deletes under — `$CUBRID`, `$CTP_HOME`, the scenario — are checked
before it runs: an unset path in a shell script is not an error, it is the root of the filesystem,
and a run on the shell side once deleted 69 directories across a machine because one variable was
empty.

## The executor

A case is executed by CQT — the Java that CTP has always used — and nothing else. What was rewritten
is the loop around it: discovery, the exclusion file, which answer a case is judged against, the
comparison, and every file the run writes.

That split is [ADR-016](../../project/adr/ADR-016-sql-executor.md). It exists because a `.sql` case is not a
script that produces text; it is text produced by a parser, a connection, a renderer and a set of
rules about errors and hints that only CQT's own code gets exactly right. A rewrite of those would be
a second implementation to keep byte-identical forever.

So each slot starts one JVM for the whole run — `TestkitExecutor`, CQT without its loop — and the
runner hands it a case at a time:

```
   runner ──"X <case>"──►  JVM          on fd 3, not stdout: a thread dump
          ◄─"R <bytes> <ms>"─           or a JVM log line on stdout would
                                        shift every later verdict by one
```

The JVM is compiled against the CTP it runs with, every class it reaches by reflection is looked up
before it says it is ready, and anything that escapes is an error and an exit rather than a JVM that
neither answers nor ends.

One thing CQT carries from case to case of its own accord: the server-message flag, which a
`--+ server-message` hint turns on and which decides whether an error prints its message under its
code. In a slotted run the cases before a given case are not CTP's, so each executor is told, before
every case, where CTP's single run would have left that flag, and it puts it there through CQT's own
code.

## What a slot is here

A slot is what the shell suite's slots are — a namespace for processes, shared memory, the network
and mounts — with two additions that belong to this family:

- **`$CUBRID` is behind an overlay.** The lower layer is the install with the prepared database in
  it; everything the slot writes lands in its own upper layer, which is thrown away at the end. So
  every slot runs the same database without copying 1 GB per slot up front.
- **A directory is claimed whole.** `dispatch.Queue.Affinity` gives a directory to the slot that
  takes its first case, and the rest of the directory follows. Cases in a directory depend on what
  their neighbours leave: reversing the order inside each directory fails 71 of medium's 975 cases,
  62 of them in one chain.

A run with one slot and no containment runs on the machine itself, as CTP does.

## What the run writes

Everything a reader or a script outside this program can see is CQT's, byte for byte — the progress
lines, the `.result` beside every case, the failure copies, and the result tree:

```
$CTP_HOME/sql/result/y2026/m9/schedule_linux_sql_64bit_<ddHHmmss><rnd>_<build>/
├── main.info              the counts, the build, the machine
├── summary.info           XStream's tree of the same
├── summary.xml            every case, its answer, its time, its verdict
├── linux_sql_64bit.xml    the JUnit report
└── sql/…/summary_info     per directory: totals, and a line per case
```

Two orders in those files are Java's collections rather than anyone's choice — the children of a
`summary_info` and the cases in the JUnit report follow JDK 8 `Hashtable` iteration over paths that
carry the run's timestamp — and they are reproduced, because a comparison with CTP is a comparison of
bytes. [`sql/records.sh`](../../project/evidence/sql/records.sh) checks that against real CTP runs.

The runner's own output goes to standard error, never to standard output: what CTP prints there is
frozen and compared.
