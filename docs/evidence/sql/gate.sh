#!/bin/bash
# ADR-017's gate: sqlsuite, one case at a time, against CTP's own runs at the
# same pins -- every verdict, every `.result`, and the records.
#
#   SANDBOX=... gate.sh <testkit binary>
#
# CTP's side is `baseline.sh`'s last medium_dev and sql runs, and the hashes it
# took of what they wrote. This runs the same two tasks through sqlsuite, hashes
# what those wrote, and compares: the `.result` files on their bytes, and the
# result trees through `gate.py`.
#
# One slot, no `TESTKIT_SLOT_VOLATILE`: the gate is about what a run produces
# under CTP's own conditions, and the settings that make a run faster are
# measured elsewhere (`evidence/sql-native.md`). The slot's layer goes beside
# the sandbox, on whatever disk the sandbox is on.

set -uo pipefail

SANDBOX=${SANDBOX:-/data/cub_sys/projects/regr-sql}
TESTKIT=${1:-}
here=$(cd "$(dirname "$0")" && pwd)

[ -x "$TESTKIT" ] || { echo "gate: $0 <testkit binary>" >&2; exit 2; }
"$here/sandbox.sh" check || exit 1

export TESTKIT_SLOT_ROOT=${TESTKIT_SLOT_ROOT:-$SANDBOX/slots}
mkdir -p "$TESTKIT_SLOT_ROOT"

run() {   # <out> <task> <conf>
    local out=$1 task=$2 conf=$3
    echo "== $out ($task, $conf, sqlsuite) $(date -Is)"
    ( cd "$SANDBOX" && /usr/bin/time -f "WALL %e s" timeout 7200 \
        env TESTKIT_NATIVE_SQL=1 TESTKIT_CONTAIN=1 \
        bash -c ". $SANDBOX/env.sh; $TESTKIT $task -c $SANDBOX/$conf" ) \
        > "$SANDBOX/$out.out" 2>&1
    echo "exit=$?" >> "$SANDBOX/$out.out"
    grep -E "^(Fail|Success|Total):|WALL|exit=" "$SANDBOX/$out.out"
}

hashes() {   # <suite> <run>: every .result the run left
    ( cd "$SANDBOX/cubrid-testcases" &&
      find "$1" -name '*.result' -type f -print0 | sort -z | xargs -0 -r sha1sum ) > "$SANDBOX/result-hashes-$2.txt"
}

newest() {   # <suite>: the result tree the last run wrote
    ls -td "$SANDBOX"/CTP/sql/result/*/*/schedule_linux_"$1"_64bit_* 2>/dev/null | head -1
}

compare() {   # <suite> <ctp run> <native run> <ctp result tree>
    local suite=$1 ctp=$2 native=$3 tree=$4
    echo "== $suite: sqlsuite against CTP"
    if diff -q "$SANDBOX/result-hashes-$ctp.txt" "$SANDBOX/result-hashes-$native.txt" >/dev/null; then
        echo "ok   every .result is byte for byte CTP's ($(wc -l < "$SANDBOX/result-hashes-$ctp.txt") files)"
    else
        echo "DIFF .result files:"
        diff "$SANDBOX/result-hashes-$ctp.txt" "$SANDBOX/result-hashes-$native.txt" |
            awk '/^>/ {print "       " $3}' | head -20
    fi
    python3 "$here/gate.py" "$tree" "$(newest "$suite")"
}

ctp_medium=$(newest medium)
ctp_sql=$(newest sql)
echo "CTP's runs: $(basename "$ctp_medium"), $(basename "$ctp_sql")"

run native-medium medium medium_dev.conf
hashes medium native-medium
compare medium medium_dev-2 native-medium "$ctp_medium"

run native-sql sql sql.conf
hashes sql native-sql
compare sql sql-2 native-sql "$ctp_sql"

echo "== upstream after the runs $(date -Is)"
"$here/sandbox.sh" check || echo "upstream moved during the runs; the results are pinned to PROVENANCE"
