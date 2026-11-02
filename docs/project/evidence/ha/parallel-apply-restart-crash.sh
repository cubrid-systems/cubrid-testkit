#!/bin/sh
# Prototype for testkit ha-apply restart --kill node (evidence/ha/ha-apply-restart-and-a-crashed-slave.md),
# run from the host against a csb pair beside parallel-apply-restart-workload.sh; the command replaces it.
# CSB_HOME and the csb binary are this host's; set them for another.
# parallel-apply-restart-crash.sh CLUSTER INTERVAL_S OUT
# A server crash under load: while the
# clients of parallel-apply-restart-workload.sh run on CLUSTER-c1, crash the
# whole slave node (csb node kill: cub_admin, cub_server and cub_master get
# kill -9, the server does not flush) and start it again (csb node start),
# then wait INTERVAL_S seconds and do it again. Before each crash it reads, on
# the slave, the ha_applied_tx_marker rows above the stored committed_lsa
# ("-" when the read fails, as on a build with no marker table). Lines go to OUT.
# The slave is CLUSTER-n2; this prototype does not follow a failover.
set -u
C=$1 D=$2 OUT=$3
S=$C-n2 CL=$C-c1
export CSB_HOME=/data/cub_sys/projects/workspace/testkit-wsprobe/csb-home
B=/data/cub_sys/projects/cubrid-testkit/extensions/cluster-sandbox/bin/csb
ENV="export CUBRID=/work/$S/cubrid CUBRID_DATABASES=/db LD_LIBRARY_PATH=/work/$S/cubrid/lib PATH=/work/$S/cubrid/bin:\$PATH"
LOG=/work/$S/cubrid/log/$C@localhost_applylogdb_${C}_$C-n1.err
BQ="SELECT COUNT(*) FROM ha_applied_tx_marker m, db_ha_apply_info a WHERE m.copied_log_path = a.copied_log_path AND (m.commit_pageid > a.committed_lsa_pageid OR (m.commit_pageid = a.committed_lsa_pageid AND m.commit_offset > a.committed_lsa_offset))"
dups() { docker exec $S sh -c "grep -c -- '-670' $LOG 2>/dev/null; true"; }
clients() { docker exec $CL sh -c 'pgrep -f "csql -u dba --no-auto-commit -i c[0-9]*.sql" >/dev/null' && echo yes || echo no; }
applier() { docker exec $S sh -c 'pgrep -f "cub_admin [a]pplylogdb" | head -1'; }

echo "# $(date -Is) cluster=$C interval=${D}s dup670_before=$(dups)" >> "$OUT"
i=0; while [ "$(clients)" = no ] && [ $i -lt 600 ]; do sleep 1; i=$((i+1)); done
k=0
while [ "$(clients)" = yes ]; do
  sleep "$D"
  [ "$(clients)" = yes ] || break
  before=$(docker exec $S sh -c "$ENV; timeout 10 csql -u dba -t -c \"$BQ\" $C@localhost 2>/dev/null | awk '/^ *[0-9]+ *\$/ {print \$1; exit}'")
  t0=$(date +%s)
  $B node kill $S --cluster $C -q >/dev/null 2>&1; krc=$?
  $B node start $S --cluster $C -q > "$OUT.start" 2>&1; src=$?
  j=0; while [ -z "$(applier)" ] && [ $j -lt 120 ]; do sleep 1; j=$((j+1)); done
  k=$((k+1))
  echo "crash=$k at=$(date +%T) markers=${before:--} kill_rc=$krc start_rc=$src applier=$( [ -n "$(applier)" ] && echo up || echo down) down_s=$(( $(date +%s) - t0 ))" >> "$OUT"
  [ $src = 0 ] || sed 's/^/  start: /' "$OUT.start" >> "$OUT"
done
rm -f "$OUT.start"
echo "# $(date -Is) crashes=$k dup670_after=$(dups)" >> "$OUT"
