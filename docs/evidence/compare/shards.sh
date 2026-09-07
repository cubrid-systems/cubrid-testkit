#!/bin/bash
# Print the corpus as a list of shards, one scenario path per line.
#
#   shards.sh <corpus-root> [max-cases-per-shard]
#
# A shard is the unit of work and the unit of resume, so its size is the answer
# to one question: how much is it acceptable to lose when a run dies? At the
# rate the four-case comparison measured -- about 18 seconds a case on each
# runner, so roughly 36 a case for the pair -- 250 cases is around two and a
# half hours. That is the default.
#
# Families are the natural shard, except that they are not evenly sized: on the
# corpus as measured, _06_issues holds 1,721 of 3,452 cases, half of everything,
# and the next largest holds 253. A family over the limit is split into its own
# subdirectories, which is enough because the discovery rule only cares that a
# case is <name>/cases/<name>.sh somewhere below the scenario root.
#
# Shards are printed largest first, because a shard that is going to fail is
# better found early, and the big ones fail in more ways.
set -u

corpus=${1:-}
limit=${2:-250}
[ -n "$corpus" ] && [ -d "$corpus" ] || { echo "usage: shards.sh <corpus-root> [max-cases-per-shard]" >&2; exit 2; }

# count is ADR-013's rule, and it is the one CTP applies: a case is
# <name>/cases/<name>.sh, matched as "the directory two levels up, plus .sh".
count() {
  find "$1" -name '*.sh' -type f -print 2>/dev/null |
    awk -F/ '{ if ($(NF-2)".sh" == $NF) print }' | wc -l
}

emit() {
  local dir=$1 n
  n=$(count "$dir")
  [ "$n" -gt 0 ] || return 0
  if [ "$n" -le "$limit" ]; then
    printf '%6d\t%s\n' "$n" "$dir"
    return 0
  fi

  # Splitting is only allowed when the children account for every case. A case
  # is a directory with its own cases/, so it cannot be a shard root and cannot
  # be descended into; if a family holds such directories directly, its cases
  # are not the union of its subdirectories and splitting would silently drop
  # them. Then the family goes out whole, over the limit, and says so -- a shard
  # that is too big costs time, and a shard that is missing costs the evidence.
  local child c sub_total=0
  local -a subs=()
  for child in "$dir"/*/; do
    [ -d "$child" ] || continue
    [ -d "${child}cases" ] && continue
    c=$(count "${child%/}")
    [ "$c" -gt 0 ] || continue
    subs+=("${child%/}")
    sub_total=$(( sub_total + c ))
  done

  if [ "${#subs[@]}" -gt 0 ] && [ "$sub_total" -eq "$n" ]; then
    for child in "${subs[@]}"; do emit "$child"; done
  else
    printf '%6d\t%s\n' "$n" "$dir"
  fi
}

for d in "$corpus"/*/; do
  [ -d "$d" ] || continue
  emit "${d%/}"
done | LC_ALL=C sort -rn
