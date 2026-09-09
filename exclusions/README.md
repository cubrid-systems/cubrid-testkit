# Known exclusions

A case can be skipped for two very different reasons, and this directory exists
so the two are never in the same file.

| whose | where | what it means |
|---|---|---|
| upstream's | the corpus's own `daily_regression/*_exclude` list | someone decided the *case* should not be judged, and named the issue. Not ours to edit |
| this machine's | the files here | the case is fine and the *environment* cannot run it. Ours, and each one has to say why and how it ends |

The distinction is not bookkeeping. An upstream exclusion is a claim about the
case that survives any machine; one of ours is a claim about a machine that
should disappear the moment the machine changes. Kept in one file they cannot be
told apart, and the machine's list can never be safely deleted.

## Using them

`testcase_exclude_from_file` takes a comma-separated list, so both apply and
neither is edited into the other:

```
testcase_exclude_from_file=/path/to/daily_regression_exclude,/path/to/cubrid-testkit/exclusions/no-cubrid-manager.txt
```

The run prints how many each file contributed, then the total in CTP's frozen
`# OF EXCLUDED = N` line. One path is still accepted, which is what CTP took.

## What is here

| file | why, and what ends it |
|---|---|
| `no-cubrid-manager.txt` (22 cases) | CUBRID Manager Server does not link on a modern glibc: the prebuilt `libevent-2.1.4-alpha` archive committed under `cubridmanager/server/external/` references `sysctl()`, which glibc removed, and there is no `libevent-dev` here to link against instead. The engine is therefore built `-DWITH_CMSERVER=OFF` and `$CUBRID` has no `cub_manager`, no `conf/cm.conf`, no `log/manager`. **Ends when** a build with the manager exists here — upstream CI has one, on an older glibc, so none of these are upstream's to exclude |

| `socket-dir-is-not-the-default.txt` (2 cases) | both assert the Unix socket directory the engine picks on its own, `$CUBRID/var/CUBRID_SOCK`. A slotted run must set `CUBRID_TMP` — every slot keeps the shipped port, so without a per-slot directory they would all want `/tmp/CUBRID1523` — and it cannot be `$CUBRID/var/CUBRID_SOCK`, because the per-case reset runs `rm -rf ${CUBRID}/var/*` and the directory is gone after the first case. Measured: a two-case run went from 26 s to 426 s with both failing. **Ends when** the reset stops emptying `$CUBRID/var`, which is CTP's script rather than this runner's |

## What is deliberately not here

`_01_utility/_27_emergency_patch_logdb/bug_xdbms278` counts temporary volumes
with `ls -al | grep testdb_t*`, where the glob is expanded by the shell before
grep sees it. With exactly one temp volume the name becomes grep's pattern and
the count is 1, which is what the check wants. With two it becomes
`grep <name> <binary file>` and the count is 0; with none the glob stays literal
and matches five lines of `ls`. Measured both ways: this run's lowered
`db_volume_size` gives two, and restoring the shipped 512M gives none. The case
needs exactly one, which is a number no setting here produces, so there is
nothing to patch to -- it is the case that is fragile, and worth reporting as
such rather than hiding.

`_38_fig/cbrd_24911` drives the manager (`conf/cm.conf`, an HTTPS call to the CMS
port) and would qualify, but its failing checks are `answer file has a problem!`
against `./result/cbrd_24911_schema*` and `./result/cbrd_24911_unloaddb.log`,
which are unloaddb outputs that were never produced. That may be a consequence of
the manager step failing first, or it may be a second fault. Excluding it would
hide whichever it is, so it stays in and needs a run with the manager to settle.
