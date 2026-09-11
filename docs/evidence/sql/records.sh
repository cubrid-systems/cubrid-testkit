#!/bin/bash
# Are sqlsuite's records CQT's, byte for byte?
#
#   SANDBOX=... records.sh <result-dir-name> <stdout-capture> [...]
#
# For each real CQT run named -- a directory under CTP/sql/result/y*/m*/ and the
# standard output that was captured while it ran -- rebuild its records from its
# own verdicts and compare every file and every line (the Go test
# TestTheRecordsAreCQTs in internal/runner/sqlsuite).
#
# The records have to be written at the path CQT wrote them, because the orders
# in them hash that path. So each check runs in a mount namespace of its own:
# an empty directory over CTP/sql/result, the real tree copied aside as the
# reference, and an overlay over the corpus to take the .result files a failed
# case writes beside itself. Nothing under the sandbox changes.

set -euo pipefail
SANDBOX=${SANDBOX:-/data/cub_sys/projects/regr-sql}
repo=$(cd "$(dirname "$0")/../../.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
export PATH=${GO_BIN:-$HOME/.local/go/bin}:$PATH

( cd "$repo" && go test -c -o "$work/sqlsuite.test" ./internal/runner/sqlsuite )

while [ $# -ge 2 ]; do
    name=$1 capture=$2
    shift 2
    real=$(ls -d "$SANDBOX"/CTP/sql/result/y*/m*/"$name")
    mkdir -p "$work/$name"/{ref,root,upper,wk}
    cp -a "$real" "$work/$name/ref/"
    echo "== $name ($capture)"
    unshare --map-root-user --mount bash -c '
        set -e
        mount --bind "$1/root" "$2/CTP/sql/result"
        mount -t overlay overlay -o lowerdir="$2/cubrid-testcases",upperdir="$1/upper",workdir="$1/wk" "$2/cubrid-testcases"
        cd "$3/internal/runner/sqlsuite"
        TESTKIT_CQT_REF="$1/ref/$4" TESTKIT_CQT_OUT="$5" \
            "$6" -test.run TestTheRecordsAreCQTs -test.v -test.count=1
    ' -- "$work/$name" "$SANDBOX" "$repo" "$name" "$capture" "$work/sqlsuite.test" \
        | grep -E -- '^(--- |\s+cqtrun_test.go|ok|FAIL|PASS)' || true
done
