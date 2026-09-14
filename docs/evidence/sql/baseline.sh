#!/bin/bash
# The baseline, as `sql-baseline.md` reports it: CTP over the smoke cases, medium
# with both confs, and medium_dev and sql twice each -- with every sql run's
# `.result` files hashed, so two runs can be compared on bytes as well as on
# verdicts (`selfcheck.py`, and ADR-017).
#
#   SANDBOX=... baseline.sh
#
# It starts only if `sandbox.sh check` says the sandbox is at upstream develop,
# and checks again at the end: the three repositories move daily, and a run that
# took an hour should say whether they moved under it. The runs themselves are
# pinned to what PROVENANCE recorded.

set -uo pipefail

SANDBOX=${SANDBOX:-/data/cub_sys/projects/regr-sql}
here=$(cd "$(dirname "$0")" && pwd)

"$here/sandbox.sh" check || exit 1

run() {
    local out=$1 task=$2 conf=$3
    echo "== $out ($task, $conf) $(date -Is)"
    ( cd "$SANDBOX" && /usr/bin/time -f "WALL %e s" timeout 7200 ./in-ns.sh \
        bash -c ". $SANDBOX/env.sh; cd \$CTP_HOME; bash bin/ctp.sh $task -c $SANDBOX/$conf" ) \
        > "$SANDBOX/$out.out" 2>&1
    echo "exit=$?" >> "$SANDBOX/$out.out"
    grep -E "^(Fail|Success|Total):|WALL|exit=" "$SANDBOX/$out.out"
}

hashes() {   # <suite> <run>: every .result the run left, for spike.sh and selfcheck
    ( cd "$SANDBOX/cubrid-testcases" &&
      find "$1" -name '*.result' -type f -print0 | sort -z | xargs -0 -r sha1sum ) > "$SANDBOX/result-hashes-$2.txt"
}

run smoke sql smoke-sql.conf
run medium medium medium.conf
run medium_dev-1 medium medium_dev.conf
run medium_dev-2 medium medium_dev.conf
hashes medium medium_dev-2
run sql-1 sql sql.conf
hashes sql sql-1
run sql-2 sql sql.conf
hashes sql sql-2
echo "== .result files that differ between sql-1 and sql-2:"
diff "$SANDBOX/result-hashes-sql-1.txt" "$SANDBOX/result-hashes-sql-2.txt" | awk '/^>/ {print "  " $3}'

echo "== upstream after the runs $(date -Is)"
"$here/sandbox.sh" check || echo "upstream moved during the runs; the results are pinned to PROVENANCE"
