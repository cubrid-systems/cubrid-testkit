# 2. Running it

[← back to the sql category](README.md)

- [What it needs](#what-it-needs)
- [A run](#a-run)
- [Watching it](#watching-it)
- [Running medium](#running-medium)
- [What it leaves behind](#what-it-leaves-behind)

## What it needs

| | |
|---|---|
| an install | `$CUBRID` at a CUBRID install this run may stop, start and reconfigure. It is **not** a shared one: `do_clean` kills this user's `cub` processes and deletes the database |
| CTP | `$CTP_HOME` at a CTP tree. The executor is compiled against the CQT jars in it, so it has to be the CTP the corpus expects |
| a corpus | `cubrid-testcases`, at the `sql` or `medium` directory |
| a JDK 8 | `$JAVA_HOME`. CQT is JDK 8 code and the executor is compiled with `javac` at run time |
| the registry inside the install | `$CUBRID_DATABASES` under `$CUBRID`, as CUBRID's own default has it. A registry outside it is not covered by the slot's overlay |

Both families run under `TESTKIT_NATIVE=sql` — the gate that says this program runs the task rather
than handing it to CTP — and want `TESTKIT_CONTAIN=1`, which puts the run in namespaces of its own so
that it cannot meet another CUBRID on the machine. Slots need it.

## A run

```bash
export CUBRID=/path/to/CUBRID
export CUBRID_DATABASES=$CUBRID/databases
export CTP_HOME=/path/to/CTP
export JAVA_HOME=/usr/lib/jvm/java-8-openjdk-amd64
export PATH=$CTP_HOME/bin:$CUBRID/bin:$JAVA_HOME/bin:$PATH

TESTKIT_NATIVE=sql TESTKIT_CONTAIN=1 testkit sql -c sql.conf
```

The conf is CTP's own, with one section this runner reads. A minimal one:

```
scenario=/path/to/cubrid-testcases/sql
test_category=sql
jdbc_config_file=test_default.xml
db_charset=en_US
need_make_locale=yes

[sql]
parallel_slots=6
status_http=on
case_patch_dir=/path/to/cubrid-testkit/patches/sql

[sql/cubrid.conf]
data_buffer_size=512M
log_buffer_size=256M
```

`tools/sizing.sh sql sql.conf` measures this machine — its cores, what they deliver at once, its
memory, and what its disk does with a synchronous write — and prints the settings it would use.
[Slots and speed](04-slots-and-speed.md) is the reasoning behind them; [configuration](03-configuration.md)
is every key.

The run exits 0 whether or not cases failed, as CTP's does. A configuration it cannot run with, or a
missing install, exits 1.

## Watching it

`status_http=on` serves a page on `127.0.0.1:51523` — a second run moves along to the next free port
and says where it went. It shows what each slot is running, the rate, the failures as they land, and
a setup panel with the switches that change what the run *means*: whether it is contained, where the
slots write, whether their syncs are real, and whether a patch directory is in use.

Clicking a case gives its verdict and, for a failure, the first line where the result leaves the
answer with the lines around it. A case that is still running shows its statements — a sql case
writes nothing until it ends, because the executor renders the whole case at once.

## Running medium

The same runner, a different task and conf:

```bash
TESTKIT_NATIVE=sql TESTKIT_CONTAIN=1 testkit medium -c medium_dev.conf
```

Use `medium_dev.conf`, not `medium.conf`: it carries `create_table_reuseoid=no`, without which the
data load breaks on 11.x and 579 of the 975 cases fail on the consequences.

**Run medium serial.** Its cases are 12.6 s in total and one directory is half of them, while
starting a slot costs about 26 s — measured, 77 s serial against 77–91 s with four slots.

## What it leaves behind

- a result tree under `$CTP_HOME/sql/result/y<year>/m<month>/`, named for the run
- a `.result` beside every case in the corpus, which the next run overwrites
- `$CTP_HOME/sql/log/<category>_<version>_<epoch>.log`, the stages' own output
- `patched.txt` in the result tree, when a patch directory was in use
- nothing else: the slots and everything they wrote go when the run ends

A core file found under `$CUBRID` or `$CTP_HOME` is announced as `CORE_FILE:<path>` — cases in the
frozen corpora grep for that line — and one found inside a slot is copied into the result tree before
the slot closes.
