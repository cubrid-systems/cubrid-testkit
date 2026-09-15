# 4. Configuration

[← back to the isolation category](README.md)

- [The conf](#the-conf)
- [Engine parameters](#engine-parameters)
- [Refused or ignored](#refused-or-ignored)
- [Environment](#environment)

The conf is CTP's flat `isolation.conf`: `key=value`, `#` comments, `${VAR}` expanded. `-c` names it; without `-c`,
`$CTP_HOME/conf/isolation.conf`.

## The conf

| key | default | what it does |
|---|---|---|
| `scenario` | — (required) | the cases tree. Under `$HOME` it is taken relative to `$HOME`, as CTP did, and the paths in the dispatch files are relative too |
| `testcase_timeout_in_secs` | 2147483647 | `timeout3.sh`'s bound on one attempt's `qactl`. CTP's shipped conf says 300 |
| `testcase_retry_num` | 0 | attempts `runone.sh` makes after the first; it stops at the first pass. CTP's shipped conf says 4 |
| `testcase_exclude_from_file` | none | one file of path fragments, `#` and `--` for comments. **Each entry excludes the first case whose path contains it, and only that one.** A file that is not there stops the run — CTP read it as an empty list and ran everything |
| `backup_core_file_yn` | yes | `no` passes `-n`: no core check and no backup in `~/error_backup` |
| `cubrid_testdb_name` | cubrid | the client program, despite the name: `cubrid` is `qacsql`, `mysql` is `qamysql`, anything else makes `runone.sh` refuse the case |
| `test_category` | isolation | what feedback prints. The run directory is `result/isolation` whatever it says |
| `test_continue_yn` | no | resume: `dispatch_tc_ALL.txt` less every `dispatch_tc_FIN_*.txt`, appending to the logs |
| `feedback_type` | file | `file` writes `feedback.log` and `test_status.data`; `database` is not a backend here and writes the file with a warning; anything else keeps no feedback |
| `parallel_slots` | 1 | cases at once, each slot with its own `ctldb` ([running it](03-running-it.md#slots)) |
| `scenario_disk` | no | puts the cases tree behind an overlay per slot, so `result/` and `<name>.result` are not written into it |
| `status_http` | off | the status page: `on`, a port, or `host:port` |

## Engine parameters

Written into each slot's install at `DEPLOY`, with CTP's `ini.sh`, before any case runs:

| prefix | file, section |
|---|---|
| `default.cubrid.<param>` | `cubrid.conf`, `[common]` |
| `default.ha.<param>` | `cubrid_ha.conf`, `[common]` |
| `default.cm.<param>` | `cm.conf`, `[cm]` |
| `default.broker1.<param>` | `cubrid_broker.conf`, `[%query_editor]` |
| `default.broker2.<param>` | `cubrid_broker.conf`, `[%BROKER1]` |
| `default.brokercommon.<param>` | `cubrid_broker.conf`, `[broker]` |

`ctldb` itself is created by `prepare.sh` with its own volume sizes (50M each), so `db_volume_size` does not reach it.
Deploy also appends `inquire_on_exit=3` to `cubrid.conf` on every run, as CTP does — in a slot the line goes with the
slot's overlay.

## Refused or ignored

| key | what happens |
|---|---|
| `env.<instance>.*` | refused: this runner runs isolation on the machine it is started on (ADR-014). Run it there, or through CTP |
| `testcase_update_yn=yes` | refused: updating the corpus is not the runner's job |
| `cubrid_download_url` | a warning; the installed build is tested |
| `enable_check_disk_space_yn` | not checked |

## Environment

| variable | what it does |
|---|---|
| `TESTKIT_NATIVE=isolation` | runs the task here rather than in CTP. Without it `testkit isolation` is CTP's |
| `TESTKIT_CONTAIN=1` | **required**: without namespaces of its own the runner refuses to start, because `runone.sh` kills by user |
| `TESTKIT_SLOT_ROOT` | where slots' writes land; `/var/tmp/testkit-slots` otherwise. Put it on the fastest disk |
| `TESTKIT_SLOT_VOLATILE=1` | mounts slot overlays volatile: syncs on them return at once. The layers are thrown away at the end anyway |
| `CUBRID`, `CUBRID_DATABASES`, `CTP_HOME`, `JAVA_HOME` | as for CTP; keep `CUBRID_DATABASES` under `$CUBRID`, where a slot's install overlay covers it |
