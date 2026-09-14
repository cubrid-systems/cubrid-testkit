#!/bin/bash
# Does TestkitExecutor write the .result CTP writes, byte for byte?
#
#   SANDBOX=... spike.sh
#
# The reference is what `baseline.sh` hashed after its last medium and sql runs,
# so the tree's current .result files -- a previous spike's -- do not matter. CTP's own
# setup runs -- run.sh's stages, unchanged -- and only the one line that starts
# CQT is replaced by the driver and the executor. The copy of run.sh has to sit in
# CTP's sql/bin: run.sh derives CTP_HOME from its own path.
#
# medium runs whole; sql runs one case directory in ten, whole directories in
# CQT's order, so that a case that leans on its directory's earlier cases is not
# mistaken for an executor that renders differently.

set -euo pipefail
SANDBOX=${SANDBOX:-/data/cub_sys/projects/regr-sql}
here=$(cd "$(dirname "$0")" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work" "$SANDBOX/CTP/sql/bin/run-spike.sh"' EXIT

# The reference is pinned to the baseline, so upstream moving since is reported, not fatal.
"$here/../sandbox.sh" check || echo "upstream has moved since the baseline; comparing against the baseline PROVENANCE pins"

"$JAVA_HOME/bin/javac" -d "$work" -cp "$SANDBOX/CTP/sql/lib/*" "$here/TestkitExecutor.java"

python3 - "$SANDBOX/CTP/sql/bin/run.sh" "$SANDBOX/CTP/sql/bin/run-spike.sh" "$here" "$work" <<'EOF'
import re, sys
src, dst, here, work = sys.argv[1:5]
text = open(src).read()
cqt = re.search(r'^\s*"\$JAVA_HOME/bin/java" -Xms1024m .*ConsoleAgent runCQT .*$', text, re.M).group(0)
assert text.count(cqt) == 1
ours = ('          python3 ' + here + '/driver.py "$SPIKE_ORDER" -- "$JAVA_HOME/bin/java" -Xms1024m '
        '-XX:+UseParallelGC -classpath "${CLASSPATH}${separator}${CPCLASSES}${separator}' + work + '" '
        'TestkitExecutor ${scenario_category} ${scenario_alias} $jdbc_config_file_ext $javaArgs '
        '2>&1 | tee -a $log_filename')
open(dst, "w").write(text.replace(cqt, ours))
EOF

# The case orders, from the baseline's own progress lines.
python3 - "$SANDBOX/sql-1.out" "$SANDBOX/medium_dev-1.out" "$work" <<'EOF'
import os, re, sys
rx = re.compile(r"Testing (\S+\.sql) \(\d+/\d+ [\d.]+%\)")
cases = lambda p: rx.findall(open(p, errors="replace").read())
sql, medium, out = cases(sys.argv[1]), cases(sys.argv[2]), sys.argv[3]
dirs = []
for c in sql:
    if not dirs or dirs[-1] != os.path.dirname(c):
        dirs.append(os.path.dirname(c))
chosen = set(dirs[::10])
sample = [c for c in sql if os.path.dirname(c) in chosen]
for name, cs in (("sql", sample), ("medium", medium)):
    open(os.path.join(out, "order-" + name), "w").write("".join(f"Testing {c} (0/0 0%)\n" for c in cs))
print(f"sql: {len(sample)} cases in {len(chosen)} of {len(dirs)} directories; medium: {len(medium)} cases")
EOF

hashes() {   # <suite> <out>
    ( cd "$SANDBOX/cubrid-testcases" &&
      find "$1" -name '*.result' -type f -print0 | sort -z | xargs -0 -r sha1sum ) > "$2"
}

spike() {   # <suite> <conf> <baseline run whose .result files are the reference>
    local suite=$1 conf=$2 ref=$SANDBOX/result-hashes-$3.txt
    [ -s "$ref" ] || { echo "no $ref -- run baseline.sh first" >&2; exit 1; }
    cp "$ref" "$work/ctp-$suite"
    # Gone before the executor runs: a case it failed to write must not pass on CTP's file.
    find "$SANDBOX/cubrid-testcases/$suite" -name '*.result' -type f -delete
    # run.sh's last stage exits 1 here, and that is expected: it looks for the
    # result tree CQT would have written ("No Results!!"), and the executor writes
    # .result files and nothing else. The comparison below is the verdict.
    ( cd "$SANDBOX" && timeout 3600 ./in-ns.sh bash -c \
        ". $SANDBOX/env.sh; export SPIKE_ORDER=$work/order-$suite; cd \$CTP_HOME; sh sql/bin/run-spike.sh -s $suite -f $SANDBOX/$conf" ) \
        > "$SANDBOX/spike-$suite.out" 2>&1 || true
    grep -E "^executor:|^done:" "$SANDBOX/spike-$suite.out" || echo "$suite: the executor never reported"
    hashes "$suite" "$work/spike-$suite"
    python3 - "$work/ctp-$suite" "$work/spike-$suite" "$suite" <<'EOF'
import sys
load = lambda p: {l.split(None, 1)[1].strip(): l.split(None, 1)[0] for l in open(p) if l.strip()}
ctp, got = load(sys.argv[1]), load(sys.argv[2])
same = sum(1 for p in got if ctp.get(p) == got[p])
diff = [p for p in got if p in ctp and ctp[p] != got[p]]
print(f"{sys.argv[3]}: wrote {len(got)}, identical to CTP {same}, different {len(diff)}, "
      f"without a CTP counterpart {sum(1 for p in got if p not in ctp)}")
for p in diff[:10]:
    print("  differs:", p)
EOF
}

spike medium medium_dev.conf medium_dev-2
spike sql sql.conf sql-2
