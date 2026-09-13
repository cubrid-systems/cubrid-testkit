# 3. Configuration

[← back to the sql category](README.md)

- [The file has sections](#the-file-has-sections)
- [CTP's keys](#ctps-keys)
- [This runner's keys](#this-runners-keys)
- [Environment](#environment)
- [Recommended settings](#recommended-settings)

## The file has sections

`sql.conf` carries two different things: what the test framework reads, and what it **writes into the
engine's own configuration files** before the run. So it is an ini file rather than a flat one, and
`ha_mode` appears in two sections meaning two different things.

| section | what it is |
|---|---|
| top level | CTP's own keys, unsectioned |
| `[sql]` | this runner's keys |
| `[sql/cubrid.conf]` | written into `$CUBRID/conf/cubrid.conf` by `do_configure` |
| `[sql/cubrid_ha.conf]` | written into `cubrid_ha.conf` |
| `[sql/cubrid_broker.conf/%BROKER1]` | written into the broker's section of that name |

The shell family's conf is flat, and that is the one real difference between the two files. Each
runner reads the path in its own format.

## CTP's keys

These behave as they always did.

| key | default | |
|---|---|---|
| `scenario` | — | the corpus root, `.../sql` or `.../medium`. Required |
| `test_category` | the task | names the result tree and CQT's alias |
| `testcase_exclude_from_file` | — | one file of path fragments to skip. **A file that is not there is no filter and no message** — every shipped conf names `${CTP_HOME}/conf/exclusions.txt`, which CTP does not ship |
| `jdbc_config_file` | `test_default.xml` | CQT's own XML under `sql/configuration/test_config/`: the connection, the run mode, and whether the summary carries answers |
| `db_charset` | `en_US.iso88591` | forced to `en_US.utf8` for the sql task on 11.5 and later, as `run.sh` does |
| `cubrid_createdb_opts` | — | passed to `createdb` |
| `need_make_locale` | `yes` | build the locale library before the database |
| `data_file` | — | medium's `mdb.tar.gz` |
| `enable_memory_leak` | `no` | `yes` hands the whole task back to CTP: it is a valgrind run and not a corpus run |

## This runner's keys

All under `[sql]`.

| key | default | impact | |
|---|---|---|---|
| `parallel_slots` | `1` | **large** | how many cases run at once. A directory is claimed whole, so this is slots, not case-level fan-out |
| `case_patch_dir` | off | none directly | corpus changes this run carries, applied before the first case and reverted at the end. See [when a case fails](05-when-a-case-fails.md) |
| `status_http` | off | none | the progress page. `on` is `127.0.0.1:51523`; a bare port takes every interface |

There is no `testcase_retry_num` and no `testcase_timeout_in_secs` here: CQT has neither, and a
runner that added them would be answering a different question from the one CTP's numbers answer.

## Environment

| | |
|---|---|
| `TESTKIT_NATIVE=sql` | run `sql` and `medium` here rather than handing them to CTP. The opt-in gate; it names families, comma-separated (`shell,sql`), and `all` is every one. `TESTKIT_NATIVE_SQL=1` is the older spelling and still works |
| `TESTKIT_CONTAIN=1` | put the run in namespaces of its own. **Required** by slots |
| `TESTKIT_SLOT_ROOT` | where each slot's writes land. `/var/tmp/testkit-slots` when unset — and **which disk that is matters**: eight slots took 1,131 s on one machine's root SSD and 722 s on its data disk |
| `TESTKIT_SLOT_VOLATILE=1` | the syncs on a slot's layer return having done nothing. Off by default, because they are part of the conditions CTP runs under. Worth 722 s → 338 s at eight slots; also what lets the slots start together |
| `TESTKIT_CONTAIN_SH` | which shell to bind over `/bin/sh`. `bash` if it can be found |

And CTP's own: `CUBRID`, `CUBRID_DATABASES`, `CTP_HOME`, `JAVA_HOME`.

## Recommended settings

Run `tools/sizing.sh sql <your conf>` — it measures the machine and prints these with its numbers.
For a 30 GB, 16-core machine whose fast disk is `/data`:

```
[sql]
parallel_slots=6
case_patch_dir=/path/to/cubrid-testkit/patches/sql
status_http=on
```

```bash
TESTKIT_NATIVE=sql TESTKIT_CONTAIN=1 \
TESTKIT_SLOT_ROOT=/data/slots TESTKIT_SLOT_VOLATILE=1 \
testkit sql -c sql.conf
```

and for medium, the same environment with `parallel_slots` left at 1.

Leave `[sql/cubrid.conf]` as the corpus expects. The buffers are 512 MB of data and 256 of log in
every shipped conf, and a slot's server is most of the 2.6 GB a slot costs — but they are also what
the cases were recorded against, and trace output carries page counts that move with them.
