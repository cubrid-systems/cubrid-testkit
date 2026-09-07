#!/bin/bash
# Compare one shard's two result trees and say whether anything new happened.
#
#   compare.sh <ctp-result-dir> <testkit-result-dir> [label]
#
# Prints a report on stdout. Exits 0 when the run is clean, 1 when it is not,
# and 2 when it could not be run at all -- so it can drive a loop over shards.
#
# "Clean" is two separate things, and the difference is the whole design:
#
#   The files that carry verdicts are held to zero differences with the
#   baseline switched off. dispatch_tc_ALL.txt, dispatch_tc_FIN_<env>.txt,
#   test_status.data, check_<env>.log, current_task_id and monitor_<env>.log
#   were byte-identical on the first comparison and there is no reason for
#   them ever not to be. A rule that let one of them differ quietly would be
#   hiding the only thing the exercise is for.
#
#   The other four -- main_snapshot.properties, feedback.log, test-<cat>.xml
#   and test_<env>.log -- carry prose, environment and traced output, and they
#   differ for reasons that have been named. Those go through baseline.txt, and
#   what does not match a rule is printed in full.
#
# ADR-013 defines the comparison: normalise, then require the rest to be
# identical. This is that, applied to a whole shard instead of by hand.
#
# The operator is comm, not diff. Both inputs are sorted -- normalize.sh ends
# with `LC_ALL=C sort` -- and on sorted input comm is the exact multiset
# difference, with no alignment heuristics to produce pairs that are not
# really differences. diff on the same files agreed line for line and offered
# no way to tell those pairs apart from real ones.
set -u

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
normalize=$here/../normalize.sh
rules=$here/baseline.txt

if [ $# -lt 2 ]; then
  echo "usage: compare.sh <ctp-result-dir> <testkit-result-dir> [label]" >&2
  exit 2
fi
old=$1
new=$2
label=${3:-$(basename "$old")}

for d in "$old" "$new"; do
  [ -d "$d" ] || { echo "compare.sh: not a directory: $d" >&2; exit 2; }
done
[ -x "$normalize" ] || [ -r "$normalize" ] || { echo "compare.sh: no normalize.sh beside $here" >&2; exit 2; }

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# strict says whether a file's verdicts are what it is for. The names carry the
# environment id, so they are matched as patterns rather than listed.
strict() {
  case $1 in
    dispatch_tc_ALL.txt|dispatch_tc_FIN_*.txt) return 0 ;;
    test_status.data|current_task_id)          return 0 ;;
    check_*.log|monitor_*.log)                 return 0 ;;
    *)                                         return 1 ;;
  esac
}

echo "=== $label ==="
echo "  CTP      $old"
echo "  testkit  $new"
echo

# ---------------------------------------------------------------------------
# Which files each side left behind. A file only one runner wrote is a finding
# on its own, and a louder one than any line inside a file they both wrote.
# ---------------------------------------------------------------------------
(cd "$old" && ls -1) | LC_ALL=C sort > "$work/files.old"
(cd "$new" && ls -1) | LC_ALL=C sort > "$work/files.new"
missing=$(LC_ALL=C comm -23 "$work/files.old" "$work/files.new")
extra=$(LC_ALL=C comm -13 "$work/files.old" "$work/files.new")
status=0
if [ -n "$missing" ] || [ -n "$extra" ]; then
  echo "FILES THE TWO RUNS DO NOT AGREE ON"
  [ -n "$missing" ] && echo "$missing" | sed 's/^/  only CTP:     /'
  [ -n "$extra" ]   && echo "$extra"   | sed 's/^/  only testkit: /'
  echo
  status=1
fi

# ---------------------------------------------------------------------------
# Timing, from what the runners already record. Not a comparison -- the numbers
# are what a plan to make the suite faster has to start from, and the first
# shard is where they stop being a guess.
# ---------------------------------------------------------------------------
times() {
  # init.sh closes every case with "<time>----<dir>--- time=<seconds>".
  local f
  f=$(ls "$1"/test_*.log 2>/dev/null | head -1)
  [ -n "$f" ] || return 0
  grep -oE -- '--- time=[0-9]+' "$f" | grep -oE '[0-9]+'
}
report_times() {
  local name=$1 dir=$2 t
  t=$(times "$dir")
  [ -n "$t" ] || { printf '  %-8s no per-case times\n' "$name"; return; }
  echo "$t" | LC_ALL=C sort -n | awk -v n="$name" '
    { v[NR] = $1; sum += $1 }
    END {
      if (NR == 0) exit
      printf "  %-8s cases %-5d total %5ds   min %3ds  median %3ds  max %4ds  mean %5.1fs\n",
             n, NR, sum, v[1], v[int((NR+1)/2)], v[NR], sum/NR
    }'
}
echo "PER-CASE ELAPSED, as the cases themselves reported it"
report_times CTP "$old"
report_times testkit "$new"
echo

# ---------------------------------------------------------------------------
# The comparison itself.
# ---------------------------------------------------------------------------
echo "RESULT FILES"
both=$(LC_ALL=C comm -12 "$work/files.old" "$work/files.new")
for f in $both; do
  [ -f "$old/$f" ] && [ -f "$new/$f" ] || continue
  bash "$normalize" < "$old/$f" > "$work/a" 2>/dev/null
  bash "$normalize" < "$new/$f" > "$work/b" 2>/dev/null
  {
    LC_ALL=C comm -23 "$work/a" "$work/b" | sed 's/^/< /'
    LC_ALL=C comm -13 "$work/a" "$work/b" | sed 's/^/> /'
  } > "$work/d"
  n=$(wc -l < "$work/d")
  lines=$(wc -l < "$work/a")

  if strict "$f"; then
    if [ "$n" -eq 0 ]; then
      printf '  %-28s %6s lines   identical\n' "$f" "$lines"
    else
      printf '  %-28s %6s lines   VERDICT FILE DIFFERS -- %s lines, baseline not applied\n' "$f" "$lines" "$n"
      sed 's/^/    /' "$work/d"
      status=1
    fi
    continue
  fi

  if [ "$n" -eq 0 ]; then
    printf '  %-28s %6s lines   identical\n' "$f" "$lines"
    continue
  fi
  printf '  %-28s %6s lines\n' "$f" "$lines"
  if awk -v rules="$rules" -f "$here/classify.awk" "$work/d"; then :; else status=1; fi
done

echo
if [ "$status" -eq 0 ]; then
  echo "COMPLETE clean $label"
else
  echo "COMPLETE dirty $label"
fi
exit $status
