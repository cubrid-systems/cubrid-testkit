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
#   PATCHES       optional directory of case patches, overrides/patches/shell.
#                 Applied to the corpus here, before either runner, and taken
#                 out after both.
#
# The patches are this script's and not the runner's, although the runner has
# its own way of applying them. A patch is a diff against the corpus; which
# runner executes the patched case is not part of it. Left to testkit's
# case_patch_dir, one side would run patched source and the other would not, and
# every patched case would come back as a difference between the runners -- a
# difference this script would then be reporting about itself. So the conf's
# case_patch_dir is dropped on the way through, and both runners read one tree.
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
PATCHES=${PATCHES:-}
[ -z "$PATCHES" ] || [ -d "$PATCHES" ] || { echo "shard.sh: no such PATCHES directory: $PATCHES" >&2; exit 2; }

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

# The corpus root, which patch names are relative to: the scenario the template
# names, before this shard replaces it.
corpus=$(sed -n 's/^[[:space:]]*scenario[[:space:]]*=[[:space:]]*//p' "$CONF" | tail -1)

conf=$out/shell.conf
grep -vE '^[[:space:]]*(scenario|case_patch_dir)[[:space:]]*=' "$CONF" > "$conf"
echo "scenario=$scenario" >> "$conf"

results=$CTP_HOME/result/shell/current_runtime_logs

# case_dir_of <patch-name> -- the directory a patch applies in, or nothing.
#
# A patch is named for its case's path under the corpus with two segments taken
# out: the "cases" every case sits in, and a file name that repeats its
# directory (overrides/patches/README.md). Putting the separators back and
# adding cases/ recovers the directory the runner applies it in. Where one
# directory holds two cases the name keeps the case's own last segment, so that
# is the second candidate. A name that matches neither is a patch for some other
# corpus, which is reported rather than guessed at.
case_dir_of() {
  local p=$corpus/${1//\~/\/}
  [ -d "$p/cases" ] && { echo "$p/cases"; return 0; }
  p=${p%/*}
  [ -d "$p/cases" ] && { echo "$p/cases"; return 0; }
  return 1
}

# Names of the patches this shard is carrying, in the order they were applied.
applied=()
applied_from=()

# put_patches_back runs however this script ends. A corpus left patched is a
# corpus the next shard compares without knowing it, and the resume makes that
# silent: the shard after this one starts by trusting the tree.
put_patches_back() {
  local i
  for (( i=${#applied[@]}-1; i>=0; i-- )); do
    patch -p0 --batch --reverse --forward --dry-run -d "${applied_from[$i]}" -i "$PATCHES/${applied[$i]}.patch" >/dev/null 2>&1 &&
      patch -p0 --batch --reverse --forward -d "${applied_from[$i]}" -i "$PATCHES/${applied[$i]}.patch" >/dev/null 2>&1 ||
      echo "shard.sh: ${applied[$i]} is still applied in ${applied_from[$i]}" >&2
  done
  applied=()
  applied_from=()
}
trap put_patches_back EXIT

# A patch that will not apply stops the shard rather than the case. It means the
# case has moved, and a shard that silently compared the unpatched case would
# put a difference in the report that is about the patch set, not the runners --
# and no report is written here, so a resumed run comes back to it.
apply_patches() {
  [ -n "$PATCHES" ] || return 0
  local f name dir
  for f in "$PATCHES"/*.patch; do
    [ -e "$f" ] || continue
    name=$(basename "$f" .patch)
    if ! dir=$(case_dir_of "$name"); then
      echo "  patch with no case in this corpus: $name"
      continue
    fi
    # Only what this shard runs. A patch applied outside it changes a tree
    # nothing here reads and would have to be put back on trust.
    case "$dir/" in "$scenario"/*) ;; *) continue ;; esac
    if ! patch -p0 --batch --forward --dry-run -d "$dir" -i "$f" >/dev/null 2>&1; then
      echo "shard.sh: $name does not apply to $dir -- check whether upstream fixed the case" >&2
      exit 2
    fi
    patch -p0 --batch --forward -d "$dir" -i "$f" >/dev/null || exit 2
    applied+=("$name")
    applied_from+=("$dir")
  done
  [ ${#applied[@]} -gt 0 ] && echo "  ${#applied[@]} patch(es) applied to the corpus"
  return 0
}

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
apply_patches
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
  # In the report and not only on the console: a reader of this file is being
  # told what two runners did to a corpus, and which cases were not the corpus
  # as it stands is part of that sentence.
  if [ ${#applied[@]} -gt 0 ]; then
    echo "PATCHED  ${#applied[@]} case(s) ran against a patch, on both sides"
    printf '  %s\n' "${applied[@]}"
    echo
  fi
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
