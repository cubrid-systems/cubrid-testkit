#!/bin/bash
# Compare one shard's two result trees and say whether anything new happened.
#
#   compare.sh <ctp-result-dir> <testkit-result-dir> [label]
#
# Prints a report on stdout. Exit 0 clean, 1 dirty, 2 could not run -- so it can
# drive a loop over shards.
#
# "Clean" means two different things, and the split is the design. The six files
# that carry verdicts must be identical, with the baseline not applied: a rule
# that let one of them differ quietly would hide the only thing this is for.
# Their one exemption is deviations.txt, which matches whole lines literally and
# names every match in the report. The other four carry prose, environment and
# traced output, and go through baseline.txt; whatever matches no rule is
# printed in full.
#
# The operator is comm rather than diff because normalize.sh leaves both inputs
# sorted, and on sorted input comm is the exact multiset difference. diff also
# pairs lines inside a hunk, and its output gives no way to tell such a pair from
# a real difference.
set -u

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
normalize=$here/../normalize.sh
rules=$here/baseline.txt
deviations=$here/deviations.txt

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

# named_deviations reports which recorded exemptions a strict file used, so an
# exemption is never silent. It re-reads the raw difference kept in $work/raw.
named_deviations() {
  [ -r "$deviations" ] || return 0
  local out="" id line
  while IFS=$'\t' read -r id line; do
    case $id in \#*|"") continue ;; esac
    [ -n "$line" ] || continue
    if LC_ALL=C grep -F -x -q -- "$line" "$work/raw" 2>/dev/null; then out="$out $id"; fi
  done < "$deviations"
  [ -n "$out" ] && echo " (allowed:$out)"
}

echo "=== $label ==="
echo "  CTP      $old"
echo "  testkit  $new"
echo

# ---------------------------------------------------------------------------
# A file only one runner wrote is a finding on its own, and a louder one than
# any line inside a file they both wrote.
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
# Timing, from what the runners already record. Not part of the comparison.
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
  cp "$work/d" "$work/raw"
  n=$(wc -l < "$work/d")
  lines=$(wc -l < "$work/a")

  if strict "$f"; then
    if [ -s "$work/d" ] && [ -r "$deviations" ]; then
      cut -f2- "$deviations" | grep -v '^[[:space:]]*#' | grep -v '^[[:space:]]*$' > "$work/dev"
      LC_ALL=C grep -F -x -v -f "$work/dev" "$work/d" > "$work/d2" || true
      mv "$work/d2" "$work/d"
      n=$(wc -l < "$work/d")
    fi
    if [ "$n" -eq 0 ]; then
      printf '  %-28s %6s lines   identical%s\n' "$f" "$lines" "$(named_deviations "$f")"
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
