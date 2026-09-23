# 3. Running it

*English · [한국어](03-running-it.ko.md)*

[← back to the shell category](README.md)

Both ways run the same binary with the same conf. Only what the environment brings differs.

- [On a host machine](#on-a-host-machine)
- [In Docker](#in-docker)
- [Watching a run](#watching-a-run)
- [Reading the result](#reading-the-result)

## On a host machine

**No root, no container runtime, no change to the machine.** The namespaces and the overlay are both
unprivileged: this is a normal user running a normal binary.

### From an empty directory to a verdict

```bash
# 1. build
git clone <this repo> && cd cubrid-testkit
go build -o bin/testkit ./cmd/testkit

# 2. the environment CTP has always needed
export CUBRID=/path/to/CUBRID
export CUBRID_DATABASES=$CUBRID/databases        # must be inside $CUBRID
export LD_LIBRARY_PATH=$CUBRID/lib
export PATH=$CUBRID/bin:$PATH
export CTP_HOME=/path/to/CTP
export JAVA_HOME=/path/to/jdk                    # for the tasks still dispatched to CTP

# 3. a conf
cat > shell.conf <<'EOF'
scenario=/path/to/testcases/shell
test_category=shell
feedback_type=file
testcase_retry_num=0
testcase_timeout_in_secs=720
parallel_slots=4
status_http=on
EOF

# 4. size the machine before asking for more slots than it has
CUBRID=$CUBRID scripts/sizing.sh

# 5. run
TESTKIT_CONTAIN=1 TESTKIT_NATIVE=shell bin/testkit shell -c shell.conf
```

`TESTKIT_CONTAIN=1` is what gives each slot its namespaces; without it slots refuse to start rather
than colliding silently. `TESTKIT_NATIVE=shell` is the opt-in gate that says *run `shell` here*
rather than handing it to CTP.

### One case, in a loop

After the suite has told you a case is unreliable:

```bash
bin/testkit run-shell --loop --maxloop 200 _01_utility/_38_csql/csql1
```

`touch STOP` in the case directory ends the loop after the attempt in flight.

## In Docker

```bash
docker run --rm \
  --security-opt seccomp=unconfined \      # to make namespaces
  --security-opt systempaths=unconfined \  # to remount /proc
  -p 51523:51523 \                         # to watch the page
  -v /host/work:/home \
  -e CUBRID=/home/CUBRID \
  -e CUBRID_DATABASES=/home/CUBRID/databases \
  -e CTP_HOME=/home/CTP \
  -e TESTKIT_CONTAIN=1 \
  -e TESTKIT_NATIVE=shell \
  -e TESTKIT_SLOT_ROOT=/home/slots/run \
  <image> testkit shell -c /home/shell.conf
```

`--privileged` is **not** needed. The two `--security-opt` flags are, and each has a symptom that
names nothing on its own:

| symptom | flag |
|---|---|
| the namespace cannot be created | `seccomp=unconfined` |
| `mount /proc: operation not permitted` | `systempaths=unconfined` |

### Three things that bite in a container and not on a host

**1. Mount the parent, not the tree.** `docker run -v` makes a mount point of any path it is given,
and an overlay's lower layer cannot be a mount point — overlayfs refuses it with the one message it
has for every refusal.

```
   WRONG                                    RIGHT
   -v /host/testcases:/home/testcases       -v /host/work:/home
      → /home/testcases is a mount point       → /home/work/testcases is an
        and cannot be a lower layer               ordinary directory inside a mount
```

The same applies to `$CUBRID`, `$CUBRID_DATABASES`, and the slot root's parent: overlay upper and
work directories cannot sit on the container's own overlayfs either.

**2. One owner for the whole tree, and run as that uid.** A bind mount carries the host's uids in,
and the namespace maps exactly one. A tree owned by uid 1000 is `nobody` inside a namespace that
maps only uid 0, and the failure surfaces as `permission denied` from whatever writes first:

```bash
# on the host
sudo chown -R 1000:1000 /host/work
# and run the container as that uid
docker run --user 1000 ...
```

Whichever uid you choose, the mounted tree and the container must agree.

**3. Clear the result tree between runs.** `feedback.log` and the rest live under
`$CTP_HOME/result/<category>/current_runtime_logs`. If `$CTP_HOME` is a bind mount, a second run
appends to the first run's files and reading them afterwards mixes two runs:

```bash
rm -rf /host/work/CTP/result
```

or give each run its own `CTP_HOME`.

## Watching a run

```
status_http=on        # 127.0.0.1:51523
status_http=8123      # every interface, port 8123
```

The page shows the slots, what each is running and for how long, the recent verdicts, the failures,
and how much of the ceiling is in use.

Clicking a case under **slots** shows what it has written *so far* — a running case has nothing in
`feedback.log` yet, because that block is written when it ends, but it appends to its own result
file at every check. This is where a case that has held a slot for 325 seconds against a plan of 6
tells you which check it is stuck on. Clicking a case under **finished** shows the recorded block.

## Reading the result

The result files are frozen: they are the same whichever runner produced them.

| | |
|---|---|
| `test_status.data`, `check_local.log` | the verdicts |
| `dispatch_tc_ALL.txt`, `dispatch_tc_FIN_local.txt` | the cases found, and the cases finished |
| `feedback.log` | one block per case, with its checks and its diff |
| `test-shell.xml` | JUnit, for CI |
| `main.info`, `summary_info` | the counters |
| `patched.txt` | **this runner's own**, and absent unless a patch was applied |

**Two runs at once are refused**, for one reason: they would share
`<CTP_HOME>/result/<category>`, which holds one `feedback.log` and one `test_status.data` between
them, so the result would describe neither. Everything else a run makes is already its own. Give the
second run a different `CTP_HOME` and the two coexist.
