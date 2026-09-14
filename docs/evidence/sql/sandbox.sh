#!/bin/bash
# The sql/medium baseline sandbox, laid down from upstream develop and checked
# against it before every run. `sql-baseline.md` §1 says why every tree in it is
# a copy; this script is how the copies are made and how their age is checked.
#
#   sandbox.sh check              exit 1 unless the engine, the cases and CTP are
#                                 all at upstream develop's current head
#   sandbox.sh refresh <install>  lay the sandbox down again from upstream
#                                 develop, with <install> copied in as $CUBRID
#
# All three repositories move every day, so a baseline is a statement about three
# commits and means nothing without them. `check` is what a run does first.
#
# The repositories are found by URL, not by remote name: on this machine the
# engine's upstream is `upstream`, the cases' is `origin`. Nothing here checks
# anything out; it fetches and reads with `git archive`, so whatever branch a
# checkout is on is left alone.

set -euo pipefail

SANDBOX=${SANDBOX:-/data/cub_sys/projects/regr-sql}
CUBRID_REPO=${CUBRID_REPO:-/data/cub_sys/cubrid}
CASES_REPO=${CASES_REPO:-/data/workspace/cubrid-testcases}
CTP_REPO=${CTP_REPO:-/data/cub_sys/cubrid-testtools}
JAVA_HOME=${JAVA_HOME:-/usr/lib/jvm/java-8-openjdk-amd64}

die() { echo "sandbox: $*" >&2; exit 1; }

# upstream <repo> <name>: fetch develop from the remote that is
# github.com/cubrid/<name>, and print the commit it is at.
upstream() {
    local repo=$1 name=$2 remote
    remote=$(git -C "$repo" remote -v | awk -v n="$name" \
        'tolower($2) ~ "github.com/cubrid/" n "(\\.git)?$" && $3 == "(fetch)" {print $1; exit}')
    [ -n "$remote" ] || die "$repo has no remote for github.com/cubrid/$name"
    git -C "$repo" fetch -q "$remote" develop
    git -C "$repo" rev-parse "$remote/develop"
}

# The engine names its own commit: `cubrid_rel` prints (11.5.0.NNNN-<7 hex>).
engine_commit() {
    CUBRID=$1 LD_LIBRARY_PATH=$1/lib "$1/bin/cubrid_rel" 2>/dev/null |
        sed -n 's/.*([0-9.]*-\([0-9a-f]*\)).*/\1/p' | head -1
}

# A sandbox with no PROVENANCE has no recorded commits, which `check` reports
# as stale rather than dying on the missing file.
recorded() { [ ! -f "$SANDBOX/PROVENANCE" ] || sed -n "s/^$1=//p" "$SANDBOX/PROVENANCE"; }

check() {
    local engine cases ctp e c t stale=0
    engine=$(upstream "$CUBRID_REPO" cubrid)
    cases=$(upstream "$CASES_REPO" cubrid-testcases)
    ctp=$(upstream "$CTP_REPO" cubrid-testtools)
    # No install yet is a sandbox that is stale, not a reason to die silently.
    e=$(engine_commit "$SANDBOX/CUBRID" 2>/dev/null || true)
    c=$(recorded cases)
    t=$(recorded ctp)
    printf '%-10s %-12s %-12s %s\n' "" sandbox upstream ""
    # The sandbox side may be a 7-hex prefix (the engine names itself that way),
    # so current means upstream's commit starts with it.
    for row in "engine ${e:-none} $engine" "cases ${c:-none} $cases" "ctp ${t:-none} $ctp"; do
        set -- $row
        local verdict=current
        [ "$2" != none ] && [ "${3#"$2"}" != "$3" ] || { verdict=STALE; stale=1; }
        printf '%-10s %-12s %-12s %s\n' "$1" "${2:0:12}" "${3:0:12}" "$verdict"
    done
    [ $stale -eq 0 ] || die "not at upstream develop -- run 'sandbox.sh refresh <install>' first"
}

refresh() {
    local install=${1:-} engine cases ctp e
    [ -x "$install/bin/cubrid_rel" ] || die "'$install' is not a CUBRID install"
    case "$SANDBOX" in /|""|"$HOME") die "refusing SANDBOX='$SANDBOX'" ;; esac
    engine=$(upstream "$CUBRID_REPO" cubrid)
    cases=$(upstream "$CASES_REPO" cubrid-testcases)
    ctp=$(upstream "$CTP_REPO" cubrid-testtools)
    e=$(engine_commit "$install")
    [ -n "$e" ] && [ "${engine#"$e"}" != "$engine" ] ||
        die "$install is $e, upstream develop is ${engine:0:12} -- build develop first"

    mkdir -p "$SANDBOX"
    # What the previous sandbox measured is kept, not wiped with it: the result
    # trees CTP wrote and the runs' own output.
    local was=$SANDBOX/archive/$(date +%Y%m%d-%H%M%S)-$(engine_commit "$SANDBOX/CUBRID" 2>/dev/null || true)
    if [ -d "$SANDBOX/CTP/sql/result" ] || compgen -G "$SANDBOX/*.out" >/dev/null; then
        mkdir -p "$was"
        [ ! -d "$SANDBOX/CTP/sql/result" ] || mv "$SANDBOX/CTP/sql/result" "$was/result"
        for f in "$SANDBOX"/*.out "$SANDBOX"/result-hashes-*.txt "$SANDBOX"/PROVENANCE*; do
            [ ! -e "$f" ] || mv "$f" "$was/"
        done
        echo "previous results: $was"
    fi
    rm -rf "$SANDBOX/CUBRID" "$SANDBOX/CTP" "$SANDBOX/cubrid-testcases" "$SANDBOX/databases"
    mkdir -p "$SANDBOX/cubrid-testcases" "$SANDBOX/databases" "$SANDBOX/bigspace"
    cp -a "$install" "$SANDBOX/CUBRID"
    git -C "$CTP_REPO" archive "$ctp" CTP | tar -x -C "$SANDBOX"
    git -C "$CASES_REPO" archive "$cases" sql medium | tar -x -C "$SANDBOX/cubrid-testcases"

    cat > "$SANDBOX/env.sh" <<EOF
# regr-sql: CTP sql/medium baseline sandbox. Every path is a copy. See PROVENANCE.
export CTP_HOME=$SANDBOX/CTP
export init_path=\$CTP_HOME/shell/init_path
export CUBRID=$SANDBOX/CUBRID
export CUBRID_DATABASES=$SANDBOX/databases
export JAVA_HOME=$JAVA_HOME
# JVM directory first: the JDK 8 launcher rewrites LD_LIBRARY_PATH otherwise
# (regression-shell.md 3-5).
export LD_LIBRARY_PATH=\$JAVA_HOME/jre/lib/amd64/server:\$CUBRID/lib:\$CUBRID/cci/lib\${LD_LIBRARY_PATH:+:\$LD_LIBRARY_PATH}
export PATH=\$CTP_HOME/bin:\$CTP_HOME/common/script:\$CUBRID/bin:\$JAVA_HOME/bin:\$PATH
export TEST_BIG_SPACE=$SANDBOX/bigspace
EOF
    cat > "$SANDBOX/in-ns.sh" <<'EOF'
#!/bin/bash
# Processes, shared memory, ports and mounts are the run's own; /bin/sh is bash.
# PID 1 stays a shell so orphans are reaped (regr/in-ns.sh explains why).
exec unshare --map-root-user --mount --pid --ipc --net --fork --mount-proc \
  bash -c 'mount --bind /bin/bash /bin/sh; ip link set lo up; "$@"; exit $?' -- "$@"
EOF
    chmod +x "$SANDBOX/in-ns.sh"
    # CTP's confs, pointed at the copies: their defaults name ${HOME}/cubrid-testcases.
    for suite in sql medium medium_dev; do
        local dir=${suite%_dev}
        sed -e "s#^scenario=.*#scenario=$SANDBOX/cubrid-testcases/$dir#" \
            -e "s#^data_file=.*#data_file=$SANDBOX/cubrid-testcases/medium/files/mdb.tar.gz#" \
            "$SANDBOX/CTP/conf/$suite.conf" > "$SANDBOX/$suite.conf"
    done

    # Three cases for a one-minute check that the sandbox runs at all (§2).
    local smoke=sql/_01_object/_04_trigger/_004_event_target
    rm -rf "$SANDBOX/smoke"
    mkdir -p "$SANDBOX/smoke/$(dirname "$smoke")"
    cp -a "$SANDBOX/cubrid-testcases/$smoke" "$SANDBOX/smoke/$smoke"
    sed -e "s#^scenario=.*#scenario=$SANDBOX/smoke/sql#" "$SANDBOX/sql.conf" > "$SANDBOX/smoke-sql.conf"

    cat > "$SANDBOX/PROVENANCE" <<EOF
created=$(date -Is)
engine=$e
engine_install=$install
cases=$cases
ctp=$ctp
EOF
    check
}

case "${1:-}" in
    check) check ;;
    refresh) shift; refresh "$@" ;;
    *) sed -n '2,17p' "$0"; exit 2 ;;
esac
