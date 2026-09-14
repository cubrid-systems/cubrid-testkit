#!/bin/bash
# Print the corpus as a list of shards, one scenario path per line.
#
#   shards.sh <corpus-root> [max-cases-per-shard]
#
# A shard is the unit of work and of resume, so its size answers one question:
# how much is it acceptable to lose when a run dies? At about 19 seconds a case
# on each runner, the default limit is around two and a half hours for the pair.
#
# Families are the natural shard but are not evenly sized -- one holds half the
# corpus -- so a family over the limit is split into its own subdirectories. That
# works because the discovery rule only requires a case to be
# <name>/cases/<name>.sh somewhere below the scenario root.
#
# Largest first: a shard that is going to fail is better found early.
set -u

corpus=${1:-}
limit=${2:-250}
[ -n "$corpus" ] && [ -d "$corpus" ] || { echo "usage: shards.sh <corpus-root> [max-cases-per-shard]" >&2; exit 2; }

# ADR-013's rule, which is CTP's: a case is <name>/cases/<name>.sh, matched as
# "the directory two levels up, plus .sh".
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

  # Split only when the children account for every case. A directory with its
  # own cases/ is a case: it cannot be a shard root and cannot be descended into,
  # so a family holding one directly is not the union of its subdirectories and
  # splitting would drop cases silently. Such a family goes out whole and says
  # so -- an oversized shard costs time, a missing one costs the evidence.
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
