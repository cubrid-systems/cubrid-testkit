#!/bin/bash
# Run one runner twice over a shard and report which cases it disagrees with
# itself about.
#
#   selfcheck.sh <scenario-path> <output-root> [ctp|testkit]
#
# This has to happen before a comparison can judge. A case whose verdict a
# single runner cannot reproduce cannot be evidence that two runners differ --
# the same argument that stopped dispatch_tc_ALL.txt from being gradeable, moved
# from a file to a verdict. Without it, every unstable case in the corpus reads
# as a runner difference.
#
# What it produces is a list, and the list is the input to a decision that is not
# this script's to make: an unstable case is counted separately, the way
# _25_unstable already is, and that belongs in ADR-013 rather than here.
#
# Environment is shard.sh's, minus the runner choice: COMPARE_ENV, TESTKIT,
# CONF, and WRAP.
set -u

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

scenario=${1:-}
outroot=${2:-}
which=${3:-ctp}
[ -n "$scenario" ] && [ -n "$outroot" ] || { echo "usage: selfcheck.sh <scenario-path> <output-root> [ctp|testkit]" >&2; exit 2; }
[ -d "$scenario" ] || { echo "selfcheck.sh: no such scenario: $scenario" >&2; exit 2; }
for v in COMPARE_ENV CONF; do
  eval "val=\${$v:-}"
  [ -n "$val" ] || { echo "selfcheck.sh: $v is not set" >&2; exit 2; }
done
WRAP=${WRAP:-}

slug=$(echo "${scenario#/}" | tr '/' '_')
out=$outroot/selfcheck-$which-$slug
report=$out/report.txt
if [ -f "$report" ] && grep -q '^COMPLETE ' "$report"; then
  echo "skip  $scenario ($which)"
  exit 0
fi
mkdir -p "$out" || exit 2

# shellcheck disable=SC1090
. "$COMPARE_ENV"
[ -n "${CTP_HOME:-}" ] || { echo "selfcheck.sh: COMPARE_ENV did not set CTP_HOME" >&2; exit 2; }

conf=$out/shell.conf
grep -v '^[[:space:]]*scenario[[:space:]]*=' "$CONF" > "$conf"
echo "scenario=$scenario" >> "$conf"

results=$CTP_HOME/result/shell/current_runtime_logs

run() {
  local label=$1
  rm -rf "$CTP_HOME/result"
  local t0=$SECONDS
  if [ "$which" = testkit ]; then
    [ -x "${TESTKIT:-}" ] || { echo "selfcheck.sh: TESTKIT is not executable" >&2; exit 2; }
    ( ulimit -H -c 0; cd "$CTP_HOME" && env TESTKIT_NATIVE_SHELL=1 $WRAP "$TESTKIT" shell -c "$conf" ) > "$out/$label.out" 2>&1
  else
    ( ulimit -H -c 0; cd "$CTP_HOME" && $WRAP bash "$CTP_HOME/bin/ctp.sh" shell -c "$conf" ) > "$out/$label.out" 2>&1
  fi
  echo "  $label exit=$? $((SECONDS - t0))s"
  mkdir -p "$out/$label"
  [ -d "$results" ] && cp -a "$results"/. "$out/$label"/ 2>/dev/null
}

echo "=== $which twice over $scenario ==="
rm -rf "$out/a" "$out/b"
run a
run b

# compare.sh labels its two sides CTP and testkit; here they are both the same
# runner, and the label says so.
bash "$here/compare.sh" "$out/a" "$out/b" "$which against itself: $scenario" > "$report" 2>&1
rc=$?
grep -q '^COMPLETE ' "$report" || echo "COMPLETE broken $scenario" >> "$report"
sed -n '/^VERDICTS/,/^$/p' "$report"
tail -1 "$report"
exit $rc
