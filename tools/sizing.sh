#!/bin/bash
# Say what this machine can run at once, and why.
#
#   tools/sizing.sh [--measure-disk] [shell|sql] [a run's conf file]
#
# shell is the default. `sql` sizes the sql family -- sql and medium -- whose
# slots cost something different and whose bound is a different one; give it the
# conf the run will use and it reads the [sql/cubrid.conf] section the runner
# writes into the engine's own file.
#
# Every number below came from measuring this suite rather than from a rule of
# thumb, and the script says which measurement each one rests on. Where a figure
# is a model fitted to a handful of points it says that too: the point is that an
# operator can disagree with a number and see what they are disagreeing with.
#
# The one thing worth understanding before reading the output: the ceiling on the
# memory the run is allowed is not a performance setting. A tmpfs occupies what
# is written to it and nothing more, so a generous ceiling costs nothing until it
# is used. Its only job is to turn a case that never cleans up from a run that
# dies into a case that fails for want of space -- so it wants to be comfortably
# above what a run really writes and comfortably below what this machine can
# lose without the kernel starting to kill things.
set -u

measure_disk=no
family=shell
conf_file=
for arg in "$@"; do
  case "$arg" in
    --measure-disk) measure_disk=yes ;;
    shell|sql|medium) family=$arg ;;
    *) conf_file=$arg ;;
  esac
done
[ "$family" = medium ] && family=sql

CONF=${CUBRID:-}/conf/cubrid.conf
say() { printf '%s\n' "$*"; }
kv()  { printf '  %-22s %s\n' "$1" "$2"; }

# ---- what the machine is ---------------------------------------------------
cores=$(nproc)
mem_total=$(awk '/^MemTotal/{printf "%d", $2/1024}' /proc/meminfo)
mem_avail=$(awk '/^MemAvailable/{printf "%d", $2/1024}' /proc/meminfo)

say "machine"
kv "cores" "$cores"
kv "memory" "${mem_total} MB total, ${mem_avail} MB available"

# ---- what the engine is configured to want --------------------------------
# Read from the file the run will actually use. A parameter absent from it takes
# the engine's shipped default, which is why those are written out here.
param() { # $1 name, $2 default
  local v
  v=$(awk -F= -v k="$1" '
    $0 ~ "^[ \t]*" k "[ \t]*=" { v=$2; gsub(/[ \t\r]/,"",v); print v }' "$CONF" 2>/dev/null | tail -1)
  [ -n "$v" ] || v=$2
  echo "$v"
}
# Sizes are written 512M, 20M, 1G. Everything here is megabytes.
mb() {
  local v=${1^^}
  case "$v" in
    *K) echo $(( ${v%K} / 1024 )) ;;
    *M) echo "${v%M}" ;;
    *G) echo $(( ${v%G} * 1024 )) ;;
    *)  echo $(( ${v:-0} / 1048576 )) ;;
  esac
}

if [ ! -f "$CONF" ]; then
  say ""
  say "cannot read $CONF -- set CUBRID first. The rest of this needs the"
  say "buffer and volume sizes the run will use."
  exit 2
fi

data_mb=$(mb "$(param data_buffer_size 512M)")
log_mb=$(mb "$(param log_buffer_size 256M)")
logvol_mb=$(mb "$(param log_volume_size 512M)")
dbvol_mb=$(mb "$(param db_volume_size 512M)")

# The sql family writes its own values into the engine's file before it runs
# (stages.sh's do_configure, from the run conf's [sql/cubrid.conf] section), so
# what is in $CONF now is what the last run left. Read the run's conf when it is
# given, and say which file each number came from.
from=$CONF
if [ "$family" = sql ] && [ -n "$conf_file" ] && [ -f "$conf_file" ]; then
  section() { # $1 name, $2 default: a key of [sql/cubrid.conf]
    local v
    v=$(awk -F= -v k="$1" '
      /^[ \t]*\[/ { insec = ($0 ~ /^[ \t]*\[sql\/cubrid\.conf\][ \t]*$/); next }
      insec && $0 ~ "^[ \t]*" k "[ \t]*=" { v=$2; gsub(/[ \t\r]/,"",v); print v }' "$conf_file" | tail -1)
    [ -n "$v" ] || v=$2
    echo "$v"
  }
  data_mb=$(mb "$(section data_buffer_size "${data_mb}M")")
  log_mb=$(mb "$(section log_buffer_size "${log_mb}M")")
  logvol_mb=$(mb "$(section log_volume_size "${logvol_mb}M")")
  dbvol_mb=$(mb "$(section db_volume_size "${dbvol_mb}M")")
  from="$conf_file [sql/cubrid.conf], over $CONF"
fi

say ""
say "engine, from $from"
kv "data_buffer_size" "${data_mb} MB"
kv "log_buffer_size" "${log_mb} MB"
kv "log_volume_size" "${logvol_mb} MB"
kv "db_volume_size" "${dbvol_mb} MB"

if [ "$family" = sql ]; then
  # ---- what one sql slot costs --------------------------------------------
  # Measured on four slots over the sql corpus, sampling every process of the
  # run every five seconds (evidence/sql-native.md §3). A slot peaked at 2.6 GB
  # with 512 MB of data buffer and 256 of log: its server 1.38 GB, the PL server
  # 0.58, the executor's JVM 0.52, five CAS processes 0.12 together.
  #
  # Only the server moves with the buffers, so the rest is a floor no parameter
  # here reaches: 1.22 GB of JVMs and CAS, and a server that is its buffers plus
  # about 0.61 GB of its own.
  slot_mb=$(( 1220 + 610 + data_mb + log_mb ))

  say ""
  say "one sql slot costs"
  kv "server" "~$(( 610 + data_mb + log_mb )) MB (its buffers, plus a floor of 610)"
  kv "PL server, executor, CAS" "~1220 MB, and no parameter here changes it"
  kv "a slot" "~${slot_mb} MB at its peak"

  # ---- what bounds the slot count ----------------------------------------
  by_mem=$(( (mem_avail - 2048) / slot_mb ))
  [ "$by_mem" -lt 1 ] && by_mem=1
  by_cpu=$cores
  # A directory is claimed whole -- its cases depend on each other's leavings,
  # so dispatch.Queue.Affinity keeps it on one slot -- and the run cannot finish
  # before its longest directory does. Measured on this corpus: 891 s of cases
  # over 1,168 directories, the longest 72 s, so about twelve slots is where the
  # wall stops improving however much memory there is.
  by_corpus=12

  say ""
  say "what bounds the slot count"
  kv "memory" "${by_mem} slots (${mem_avail} MB less 2 GB, at ${slot_mb} MB a slot)"
  kv "cpu" "${by_cpu} slots (one a core)"
  kv "the corpus" "${by_corpus} slots -- a directory runs on one slot, and the longest is 72 s of the 891"

  # ---- which disk, and whether to skip its syncs ---------------------------
  # What a slot writes goes to TESTKIT_SLOT_ROOT, and a server waits on a
  # synchronous write at every commit. Measured: 5.8 ms on this machine's root
  # SSD against 1.3 ms on its data disk, and eight slots on the slow one took
  # 1,131 s where the same eight on the fast one took 722.
  sync_ms() { # $1 directory -> ms per 4 KB synchronous write, or ""
    local f=$1/.sizing-sync.$$ out
    out=$(dd if=/dev/zero of="$f" bs=4k count=200 oflag=dsync 2>&1 | awk '/copied/{print $(NF-3)}')
    rm -f "$f" 2>/dev/null
    [ -n "$out" ] && awk -v s="$out" 'BEGIN{printf "%.1f", s * 1000 / 200}'
  }
  say ""
  say "the slot root -- measured now, and a machine under load measures worse"
  best_dir= ; best_ms=
  for d in "${TESTKIT_SLOT_ROOT:-}" /var/tmp "$([ -n "$conf_file" ] && dirname "$conf_file")"; do
    [ -n "$d" ] && [ -d "$d" ] && [ -w "$d" ] || continue
    ms=$(sync_ms "$d")
    [ -n "$ms" ] || continue
    kv "$d" "${ms} ms a synchronous write"
    if [ -z "$best_ms" ] || awk -v a="$ms" -v b="$best_ms" 'BEGIN{exit !(a<b)}'; then
      best_ms=$ms; best_dir=$d
    fi
  done

  slots=$by_mem
  [ "$slots" -gt "$by_cpu" ] && slots=$by_cpu
  [ "$slots" -gt "$by_corpus" ] && slots=$by_corpus

  say ""
  say "for the run's conf and environment"
  say ""
  say "  [sql]"
  say "  parallel_slots=${slots}"
  say "  case_patch_dir=<this repository>/patches/sql"
  say ""
  say "  TESTKIT_NATIVE_SQL=1 TESTKIT_CONTAIN=1"
  [ -n "$best_dir" ] && say "  TESTKIT_SLOT_ROOT=${best_dir}   # ${best_ms} ms a synchronous write, the fastest of those tried"
  say "  TESTKIT_SLOT_VOLATILE=1        # the slot's layer is thrown away; its syncs are the run's"
  say ""
  say "what volatile is worth"
  kv "eight slots, syncs kept" "722 s, 199,133 flushes"
  kv "eight slots, volatile" "338-394 s, 750 flushes"
  kv "and the cases" "half the time CTP's serial run spends on them"
  say ""
  say "medium"
  say "  Run it serial. Its cases are 12.6 s in total and one directory is half of"
  say "  them, while a slot costs 26 s to start: measured 77 s serial against 77-91"
  say "  with four slots."
  say ""
  say "not measured"
  say "  Whether the slots can start together without volatile, and what a machine"
  say "  with more memory does above ${by_corpus} slots."
  exit 0
fi

# ---- what one slot costs --------------------------------------------------
# server ~= 157 MB + 0.26 * (data_buffer + log_buffer).
#
# Re-fitted, because the earlier 85 + 0.54x was off by 43% at the settings this
# suite actually runs with -- it said 122 MB where the server measures 175, and
# a slot count built on that is a slot count the machine cannot hold. Measured
# again on one server under the same insert-and-sort workload, taking Pss rather
# than Rss so that shared pages are not counted once per slot:
#
#   buffers   Pss     old formula   this one
#     516 M   292 MB    364 MB       291 MB
#     132 M   192 MB    156 MB       191 MB
#      68 M   175 MB    122 MB       175 MB
#      20 M   163 MB     96 MB       162 MB
#
# The shape is what matters: a floor near 157 MB that no parameter reaches, and
# a quarter of the buffers on top. Cutting data_buffer_size from 64M to 16M buys
# 12 MB, and max_clients from 100 to 10 buys another 12 (124 threads to 44 --
# the stacks are lazily allocated, so they cost almost nothing resident).
# Neither is a way to fit more slots in.
#
# What a server reaches under a real case is a different number again: servers
# in a 24-slot run measured 456 MB Pss each. That is the case's working set --
# its data, its sorts, its temporary volumes -- and it is not a setting.
server_mb=$(( 157 + (26 * (data_mb + log_mb)) / 100 ))

# case database ~= 195 MB + log_volume_size, and that one is not a fit so much
# as an identity: 512M gave 707 MB on disk, 64M gave 259, 20M gave 215.
# A case asks for --db-volume-size=20M and gets this, which is where the
# suite's write volume comes from.
case_mb=$(( 195 + logvol_mb ))

# But the ceiling cannot be sized on the typical case, and the first version of
# this script did exactly that -- it would have recommended 3,440 MB for the run
# that peaked at 6,026. What a slot holds is whatever case it drew, and the cases
# do not resemble each other: at log_volume_size=20M the typical case database
# measured 215 MB and the largest directory measured 1,078
# (_18_unloaddb/itrack_10010, which creates two of them), a factor of five.
#
# So the ceiling rests on the worst case a slot can draw. Sized that way it comes
# out conservative -- 8 x 1,078 is 8.6 GB against the 6,026 that was measured --
# and conservative is the safe direction for a ceiling that costs nothing until
# it is used.
worst_ratio=5
worst_mb=$(( case_mb * worst_ratio ))

say ""
say "one slot costs"
kv "cub_server" "~${server_mb} MB resident"
kv "a typical case's database" "~${case_mb} MB written"
kv "the largest one's" "~${worst_mb} MB -- measured 5x the typical, and a slot draws what it draws"

# ---- what bounds the slot count ------------------------------------------
# Memory: a slot costs its server *and* its database, because with the writes
# in memory the database is memory too. Counting only the server is how the
# first version of this recommended eighteen slots and then a ceiling smaller
# than eighteen slots need.
per_slot_mb=$(( server_mb + worst_mb ))
by_mem=$(( (mem_avail - 2048) / per_slot_mb ))
[ "$by_mem" -lt 1 ] && by_mem=1

# CPU: a case waits far more than it computes -- createdb measured 7.72 s of
# wall against 0.53 s of CPU -- so slots could exceed cores. Whether they should
# is not measured, so the recommendation stops at the core count and says so.
by_cpu=$cores

# Disk, and this is the one that actually bound this machine. At eight slots
# writing to disk the wall clock got *worse* than at four: 217 cases at 708 MB
# is 150 GB, and the disk that writes 332 MB/s sequentially delivered an
# effective 88 MB/s under four concurrent writers. Concurrent writes are seeks,
# not bandwidth, so a figure derived from sequential throughput would be wrong
# in the optimistic direction. Four is what was measured, and it is used as
# measured.
by_disk=4

say ""
say "what bounds the slot count"
kv "memory" "${by_mem} slots (${mem_avail} MB less 2 GB, at ${per_slot_mb} MB a slot: server plus database)"
kv "cpu" "${by_cpu} slots (one a core; a case waits more than it computes, so more may work)"
kv "disk, writes on disk" "${by_disk} slots -- measured: eight was slower than four"
say "  That bound is the syncs, not the disk's bandwidth, and there is now a way"
say "  round it: scenario_disk=on gives each slot an overlay of the corpus, and"
say "  TESTKIT_SLOT_VOLATILE=1 makes the syncs on it return at once. Measured on"
say "  _01_utility at eight slots: 1,426 s becomes 540, and two thirds of the"
say "  188 GB never reaches the disk (evidence/parallel-shell.md §5). How many"
say "  slots that allows has not been measured."

if [ "$measure_disk" = yes ]; then
  d=${TMPDIR:-/tmp}/sizing.$$
  seq_mb=$(dd if=/dev/zero of="$d" bs=1M count=1024 conv=fdatasync 2>&1 |
           awk '/copied/{print int($(NF-1))}')
  rm -f "$d"
  kv "disk, sequential" "${seq_mb:-?} MB/s -- concurrent writers get far less"
fi

# ---- the recommendation ---------------------------------------------------
# With the writes in memory the disk bound is gone: reads come off disk at
# 1.4 GB/s -- four times its write speed -- and the corpus is shared page
# cache, so what is left is memory and cores.
ram_slots=$(( by_mem < by_cpu ? by_mem : by_cpu ))
disk_slots=$(( by_disk < ram_slots ? by_disk : ram_slots ))

# The ceiling: what the run really writes, with room, and never so much that
# filling it would take the machine down instead of the case.
need_mb=$(( ram_slots * worst_mb ))
room_mb=$(( mem_avail - ram_slots * server_mb - 2048 ))
# The need is already the worst case for every slot at once, so it is its own
# headroom -- doubling it again would ask for more than the machine has and
# report the run as impossible.
# Rounded up to a 512 boundary and then clamped, in that order. Rounding down
# last is how this recommended 11 slots and then declared 11 slots impossible:
# the ceiling landed 49 MB under what it had just said the slots need.
ceiling_mb=$(( (need_mb + 511) / 512 * 512 ))
[ "$ceiling_mb" -gt "$room_mb" ] && ceiling_mb=$room_mb
[ "$ceiling_mb" -lt 1024 ] && ceiling_mb=1024
# A ceiling below what a run needs would stop the run rather than a case, which
# is the opposite of the point.
if [ "$ceiling_mb" -lt "$need_mb" ]; then
  short=yes
else
  short=no
fi

say ""
say "for shell.conf"
say ""
say "  # writes in memory -- the corpus stays read-only and the run leaves nothing behind"
say "  parallel_slots=${ram_slots}"
say "  scenario_ram_mb=${ceiling_mb}"
say ""
say "  # writes on disk -- more slots than this made it slower, measured"
say "  # parallel_slots=${disk_slots}"
say ""
say "why ${ceiling_mb} MB"
kv "a run needs" "~${need_mb} MB (${ram_slots} slots x ${worst_mb} MB, the largest case)"
kv "the machine can spare" "~${room_mb} MB (available, less the servers and 2 GB)"
kv "the ceiling is" "the need, or what can be spared, whichever is less"
if [ "$short" = yes ]; then
  say ""
  say "this machine is too small for ${ram_slots} slots with the writes in memory:"
  say "  the ceiling it can spare (${ceiling_mb} MB) is under what the slots need (${need_mb} MB)."
  say "  Lower parallel_slots, lower log_volume_size, or leave the writes on disk."
fi
say ""
say "not measured"
say "  Above ${by_disk} slots this suite has only been run with the writes on disk,"
say "  where it got slower. With them in memory the disk bound is gone -- reads"
say "  come off it at 1.4 GB/s against 332 MB/s of writes -- but where the next"
say "  ceiling sits has not been measured. Treat ${ram_slots} as the number to"
say "  verify, not the number to trust."

if [ "$logvol_mb" -gt 64 ]; then
  say ""
  say "worth changing first"
  kv "log_volume_size=${logvol_mb}M" "makes every case database ${case_mb} MB"
  kv "log_volume_size=20M" "makes it 215 MB -- 3.3x less to write"
  say "  Only 40% of the 3,452 cases that create a database say --log-volume-size,"
  say "  so for the rest this file decides it. The template key includes it, so"
  say "  lowering it does not serve templates built at the old size."
fi

if [ "$dbvol_mb" -gt 64 ]; then
  say ""
  say "and one lever that has not been measured"
  kv "db_volume_size=${dbvol_mb}M" "the same default log_volume_size had, and untouched"
  say "  111 of this family's 217 cases do not override it, and the fitted model puts"
  say "  it at about 195 MB of a case's ${case_mb}. Lowering it is one arm, worth"
  say "  roughly 175 MB a case, and it needs its own verdict check -- a case that"
  say "  tests filling a volume would find a different volume."
fi
