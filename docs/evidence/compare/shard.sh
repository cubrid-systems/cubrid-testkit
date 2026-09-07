#!/bin/bash
# Run one shard on both runners, keep both result trees, and compare them.
#
#   shard.sh <scenario-path> <output-root>
#
# A shard whose report already ends in COMPLETE is skipped. That is the only
# resume there is, which is why shard size is a decision: a killed run loses
# whatever the shard had done.
#
# Everything about *where* is passed in the environment, because none of it
# belongs to this repository:
#
#   COMPARE_ENV   file to source before either runner. Sets CTP_HOME, CUBRID,
#                 CUBRID_DATABASES, JAVA_HOME, PATH, LD_LIBRARY_PATH. The order
#                 of LD_LIBRARY_PATH matters -- see regression-shell.md 3-5.
#   TESTKIT       the testkit binary to compare against CTP.
#   CONF          a shell.conf to use as the template. Its scenario= line is
#                 replaced per shard; everything else is passed through.
#   WRAP          optional command to run each runner under -- in practice the
#                 namespace wrapper. Both runners must get the same one, or the
#                 comparison is not one.
set -u

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

scenario=${1:-}
outroot=${2:-}
[ -n "$scenario" ] && [ -n "$outroot" ] || { echo "usage: shard.sh <scenario-path> <output-root>" >&2; exit 2; }
[ -d "$scenario" ] || { echo "shard.sh: no such scenario: $scenario" >&2; exit 2; }
for v in COMPARE_ENV TESTKIT CONF; do
  eval "val=\${$v:-}"
  [ -n "$val" ] || { echo "shard.sh: $v is not set" >&2; exit 2; }
done
[ -r "$COMPARE_ENV" ] || { echo "shard.sh: cannot read COMPARE_ENV=$COMPARE_ENV" >&2; exit 2; }
[ -x "$TESTKIT" ]     || { echo "shard.sh: TESTKIT is not executable: $TESTKIT" >&2; exit 2; }
[ -r "$CONF" ]        || { echo "shard.sh: cannot read CONF=$CONF" >&2; exit 2; }
WRAP=${WRAP:-}

slug=$(echo "${scenario#/}" | tr '/' '_')
out=$outroot/$slug
report=$out/report.txt

if [ -f "$report" ] && grep -q '^COMPLETE ' "$report"; then
  echo "skip  $scenario  ($(grep -m1 '^COMPLETE ' "$report" | awk '{print $2}'))"
  exit 0
fi

mkdir -p "$out" || exit 2
# shellcheck disable=SC1090
. "$COMPARE_ENV"
[ -n "${CTP_HOME:-}" ] || { echo "shard.sh: COMPARE_ENV did not set CTP_HOME" >&2; exit 2; }

conf=$out/shell.conf
grep -v '^[[:space:]]*scenario[[:space:]]*=' "$CONF" > "$conf"
echo "scenario=$scenario" >> "$conf"

results=$CTP_HOME/result/shell/current_runtime_logs

# run <label> <destination> <command...>
run() {
  local label=$1 dest=$2; shift 2
  rm -rf "$CTP_HOME/result"
  echo "  $label ..."
  local start=$SECONDS
  # Lowering the hard limit stops a cub_server core from filling the disk. The
  # case scripts' own `ulimit -c unlimited` then fails, identically on both
  # sides, which is what keeps the comparison valid.
  ( ulimit -H -c 0; cd "$CTP_HOME" && env $RUNENV $WRAP "$@" ) > "$out/$label.out" 2>&1
  local code=$?
  echo "$code" > "$out/$label.exit"
  mkdir -p "$dest"
  if [ -d "$results" ]; then cp -a "$results"/. "$dest"/ 2>/dev/null; fi
  echo "  $label exit=$code  $((SECONDS - start))s"
}

echo "=== $scenario ==="
rm -rf "$out/ctp" "$out/testkit"
RUNENV=
run ctp "$out/ctp" bash "$CTP_HOME/bin/ctp.sh" shell -c "$conf"
# The gate is still opt-in; this line goes when it comes off.
RUNENV="TESTKIT_NATIVE_SHELL=1"
run testkit "$out/testkit" "$TESTKIT" shell -c "$conf"

ctp_code=$(cat "$out/ctp.exit")
tk_code=$(cat "$out/testkit.exit")

(
  bash "$here/compare.sh" "$out/ctp" "$out/testkit" "$scenario"
  rc=$?
  echo
  echo "EXIT CODES  CTP=$ctp_code  testkit=$tk_code"
  if [ "$ctp_code" != "$tk_code" ]; then
    echo "  they differ, which is a finding on its own"
    rc=1
  fi
  exit $rc
) > "$report" 2>&1
rc=$?

# A report with no COMPLETE line would be skipped by a resumed run, so say it.
grep -q '^COMPLETE ' "$report" || echo "COMPLETE broken $scenario" >> "$report"

tail -1 "$report"
exit $rc
