#!/bin/bash
# shard-clean.sh <scenario> <outroot>
#
# shard.sh, with the corpus put back between the two runners.
#
# shard.sh runs CTP and then testkit over the same tree, in place. A case that
# leaves a database behind therefore hands it to the second runner, and the
# second runner is always testkit -- so contamination and "a difference between
# the runners" look identical. Restoring the tree in between tells them apart.
#
# Same environment contract as shard.sh: COMPARE_ENV, TESTKIT, CONF, WRAP.
set -u
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
scenario=$1; outroot=$2
for v in COMPARE_ENV TESTKIT CONF WRAP; do eval "[ -n \"\${$v:-}\" ]" || { echo "$v unset" >&2; exit 2; }; done
. "$COMPARE_ENV"
out=$outroot/$(echo "${scenario#/}" | tr '/' '_'); mkdir -p "$out"
conf=$out/shell.conf
grep -v '^[[:space:]]*scenario[[:space:]]*=' "$CONF" > "$conf"; echo "scenario=$scenario" >> "$conf"
results=$CTP_HOME/result/shell/current_runtime_logs

pristine=$out/pristine.tar
[ -f "$pristine" ] || tar -cf "$pristine" -C "$(dirname "$scenario")" "$(basename "$scenario")"
restore() { rm -rf "$scenario"; tar -xf "$pristine" -C "$(dirname "$scenario")"; }

run() { # run <label> <dest> <env> <cmd...>
  local label=$1 dest=$2 renv=$3; shift 3
  rm -rf "$CTP_HOME/result"
  restore
  head -1 "$CUBRID_DATABASES/databases.txt" > "$CUBRID_DATABASES/.t" && mv "$CUBRID_DATABASES/.t" "$CUBRID_DATABASES/databases.txt"
  local s=$SECONDS
  ( ulimit -H -c 0; cd "$CTP_HOME" && env $renv $WRAP "$@" ) > "$out/$label.out" 2>&1
  echo "$?" > "$out/$label.exit"
  mkdir -p "$dest"; [ -d "$results" ] && cp -a "$results"/. "$dest"/ 2>/dev/null
  echo "  $label exit=$(cat "$out/$label.exit")  $((SECONDS-s))s"
}

echo "=== $scenario (corpus restored between runners) ==="
rm -rf "$out/ctp" "$out/testkit"
run ctp     "$out/ctp"     ""                      bash "$CTP_HOME/bin/ctp.sh" shell -c "$conf"
run testkit "$out/testkit" "TESTKIT_NATIVE_SHELL=1" "$TESTKIT" shell -c "$conf"
restore
bash "$here/compare.sh" "$out/ctp" "$out/testkit" "$scenario" > "$out/report.txt" 2>&1
tail -1 "$out/report.txt"
