# What the legacy CTP could fix cheaply

Findings from running the shell corpus against CTP and against this runner on the
same engine, same cases, same machine. Everything here is about **CTP as CI runs
it today** — none of it needs the migration to finish, and most of it is a line
or a configuration value.

Each item says what was measured, what the fix is, and what it would cost.

---

## How to try any of these in real CI

The CI image takes the testtools branch from the environment:

```dockerfile
ENV BRANCH_TESTTOOLS=develop
ENV CTP_BRANCH_NAME=$BRANCH_TESTTOOLS
```

(`cubridci`, branch `test_rl8.10`, `docker/ci/Dockerfile`.)

So a change to CTP can be pushed to a branch of `cubrid-testtools` and a PR
pointed at it with `BRANCH_TESTTOOLS=<branch>`, and the real CI runs the real
corpus against it. Nothing below has to be argued from a developer's machine.

---

## A. CTP runs every case with `sh` while requiring bash

`common/src/com/navercorp/cubridqa/common/LocalInvoker.java:83`

```java
invokedCmd = "sh " + tmpFile.getAbsolutePath() + " 2>&1";
```

Every case begins with `. $init_path/init.sh`, and `init.sh` is bash. Where
`/bin/sh` is bash — Rocky, RHEL, CentOS, which is what CI runs — this works. Where
it is dash — Ubuntu, Debian, which is what a developer's machine usually is — it
does not:

```
init.sh: 51: Syntax error: "(" unexpected
```

Measured: CTP run on such a machine failed **all seventeen** cases it was given,
every one of them at `init.sh`'s first line, every one reported as `blank result`
with no verdict at all. The failure looks like seventeen broken cases and is one
missing word.

**The fix is the word, not the shell scripts.** `init.sh` line 51 is
`function get_os(){`, and removing the 75 `function` keywords does not make it
POSIX — the next error is at line 101, and `shell_utils.sh` fails at line 79 the
same way. Rewriting four thousand lines of shell to POSIX would be a large change
with nothing to gain: the suite has always required bash and always will.

Saying `bash` reproduces exactly what happens on the machines CI uses, and
additionally works on the machines it currently cannot. Four call sites:
`LocalInvoker.java:83`, `coreanalyzer/LocalInvoker.java:103`, `CTP.java:222`,
`ParseActionFiles.java:42`.

| Cost | one word in four places |
| Risk | none on a machine where `/bin/sh` is already bash: same interpreter |
| Gain | CTP becomes runnable on a developer's laptop |

*(This runner does the same thing and records why in `internal/exec/exec.go`. It
also binds bash over `/bin/sh` inside its own mount namespace, which is a second
belt for the same trousers.)*

---

## B. The reset kills processes across the whole machine

`shell/init_path/init.sh:1148` and `:1150`

```sh
pids=`ps -u $USER -o pid,command | grep "$strkill" | grep -v grep | awk '{print $1}'`
```

and the broker sweep at `:634`

```sh
broker_sid=`ipcs | grep $USER | awk '{print $2}'`
```

Both select by **user**, not by anything belonging to the run. Observed directly:
while CTP ran the seventeen cases on this machine it repeatedly `SIGKILL`ed this
session's own `sleep` commands, which had nothing to do with it.

CI already knows. The image works around it by giving the node a different
account from the controller's:

```dockerfile
# CTP kills every process owned by the node account, so it must differ from the controller's.
RUN useradd -M -d $WORKDIR $NODE_USER
```

That protects the controller. It does not protect anything else running as the
node account, and it does not help on a machine where a developer is working.

| Cost | scoping the kill — a process group, or a `$CUBRID`-rooted match |
| Risk | a leftover this no longer kills is a leftover; the current behaviour kills too much, not too little |
| Gain | CTP stops being unsafe to run on a shared machine |

---

## C. `db_volume_size=512M` makes one case write 9 GB

The shipped default is 512M, and `shell_ci.conf` does not change it. Measured on
the full corpus, mid-run:

```
9091 MB  _06_issues/_11_1h/bug_bts_4823/cases     ← 251 volume files
1624 MB  _06_issues/_11_1h/bug_bts_3680_2/cases
 987 MB  _06_issues/_11_2h/bug_bts_5639/cases
```

`bug_4823_x001` is exactly 536,870,912 bytes. The case creates 251 of them,
because every volume it extends into is created at `db_volume_size`. One case
writes nine gigabytes of a corpus whose entire git tree is 1.4 GB.

Lowering it makes every case that does not name its own size smaller by about
175 MB, and cases like this one smaller by a factor of twenty-five. On this
runner it took a family's wall clock from 407 s to 391 and its peak memory from
7,675 MB to 5,353.

**It is not free.** Three cases changed verdict, and two have a specific reason:
they assume a fresh database has exactly one data volume, which stops being true
when 20M is too small for the catalog and `createdb` adds `_x001` itself.

- `_07_addvoldb/itrack_10005` asserts the volume it added is `_x001`; it becomes
  `_x002`.
- `_09_renamedb/bug_xdbms265` writes a `renamedb` control file naming volume 0
  only, and `renamedb` then refuses.

Both are cases that need updating rather than engine defects, and both are
one-case fixes. The third, `_27_emergency_patch_logdb/bug_xdbms278`, did not
reproduce in isolation.

| Cost | one line in `shell_ci.conf`, after two cases are fixed |
| Risk | measurable — run it with `BRANCH_TESTTOOLS` and diff the verdicts |
| Gain | the suite's write volume falls by roughly an order of magnitude |

---

## D. A Linux run calls `taskkill`

`shell/_01_utility/_37_cubrid/_01_service/cases/_01_service.sh:9`

```sh
cub_commdb -A
taskkill /F /IM cub*
```

Unguarded, so it runs on Linux, where it is `command not found`. The case's
intent — a clean slate before `cubrid service start` — is therefore **not
achieved on Linux at all**, and the case goes on to assert that the service
started cleanly. Every other OS-specific branch in the same file is guarded with
`if [ "$OS" != "Linux" ]`.

| Cost | one `if` in the testcases repository |
| Risk | the case starts doing on Linux what it has always claimed to do |

---

## E. Three excluded cases now have causes

`shell/config/daily_regression_test_excluded_list_linux.conf` holds nine entries,
each with a ticket. Two of them say the case is the problem and that nobody has
looked yet. Both now have a specific answer.

**`_16_restoredb/itrack_10001`** — CBRD-21637, *"need invertigae the test points
and update the case"*:

```sh
csql -S qadb -c "create class x(name char(10000), age char(10000), w char(1000))"
ERROR: Maximum byte string length is 2048 bytes.
```

The class is never created, so the JDBC loader then fails with
`Unknown class "dba.x"` and all fifteen restore checks fail behind it. The case
predates the limit; `char(2000)` is within it.

**`_38_csql/csql2`** — CBRD-23602. It compares
`select ... from db_class ... where rownum < 10` against an answer file last
touched **2016-03-24**, and the query has no `ORDER BY`. The engine now returns
the `INFORMATION_SCHEMA` views where the answer expects the `DBA` ones. The
answer file is stale by ten years; the query is also malformed (two `where`
clauses).

**`_16_restoredb/cbrd_24892`** — CBRD-25050. Still open here: it takes 381 s and
fails, and no cause was established.

| Cost | one line each for the first two |
| Gain | two entries leave the exclusion list |

---

## F. A missing JVM costs two minutes before it is reported

`src/sp/pl_sr.cpp`, `server_monitor_task`:

```c
constexpr int MAX_FAIL_COUNT = 10;              // one-second polls
error = do_check_connection (MAX_FAIL_COUNT);
...
while (try_count++ < 10 && m_state != SERVER_MONITOR_STATE_RUNNING);
```

Eleven attempts over ten one-second polls each — about 110 seconds before a
server that cannot find its JVM stops looking. Two cases exercise this
deliberately (`_03_start_server/itrack_10003` and `itrack_10004`), and they take
115 s and 124 s, repeatable to within 1% across three independent runs. That is
**240 seconds of a 3,189 case-second family spent waiting**, and the source
already carries `// TODO: parameterize this` above the constant.

This is the engine rather than CTP, and it is a product question before it is a
test one: a server that cannot find its JVM will not find it by retrying eleven
times.

---

## G. The shell suite runs serially, in one container

`cubridci` `test_rl8.10` `docker/ci/docker-entrypoint.sh:820`:

```sh
( cd "$WORKDIR" && HOME="$WORKDIR" "$CTP_HOME/bin/ctp.sh" "$CTP_CMD" -c "$CTP_HOME/$CTP_CONF" )
```

One invocation, and `shell` uses `conf/shell_ci.conf` unmodified — in which every
`env.instance*` entry is commented out. So the whole corpus, 3,444 cases after
the two exclusions, runs on one node one case at a time. CTP's own `TestFactory`
already runs a `Test` per configured environment in a hundred-thread pool; it is
being given one.

Two ways forward, and they are not exclusive. CTP can be given several `env.*`
entries and the nodes to go with them — the image already has a `node` mode for
exactly that. Or the machine-level parallelism this project is building can take
it: 217 cases of `_01_utility` go from 2,932 s on one slot to 391 s on eight, on
one machine, with the verdicts identical case for case.

---

## What this runner does about each

| | CTP today | here |
|---|---|---|
| A | `sh <case>` | `bash`, and bash bound over `/bin/sh` in the run's own mount namespace |
| B | kills by `$USER` across the machine | a PID namespace per slot, so `ps -e` *is* the slot |
| C | 512M, unchanged | measured both ways; 512M kept until the two cases are fixed |
| D | — | nothing: it is the corpus's, and the corpus is not ours to change |
| E | — | causes recorded above |
| F | — | nothing: it is the engine's |
| G | one node, serial | slots on one machine, with a status page and a duration plan |
