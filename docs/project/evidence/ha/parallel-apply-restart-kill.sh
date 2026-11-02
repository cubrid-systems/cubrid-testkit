#!/bin/sh
# Prototype for testkit ha-apply restart (evidence/ha/ha-apply-restart-and-the-applied-tx-marker.md),
# run from the host with docker exec against a csb pair; the command replaces it.
# In that document, the kill runs beside stress and the 8 x 3,000 run on 19e83a5d9 counted -670
# as "CODE = -670", which an applier's update failure does not carry; this copy counts it anywhere.
# parallel-apply-restart-kill.sh CLUSTER INTERVAL_S OUT
# While the clients (stress or parallel-apply-restart-workload.sh) run on CLUSTER-c1, kill -9 the slave's applylogdb
# every INTERVAL_S seconds. Before each kill it reads, on the slave, the
# number of ha_applied_tx_marker rows (or "-" when the table does not exist)
# and the stored committed_lsa. After each kill it waits for heartbeat to
# start a new applylogdb and records the new pid. Lines go to OUT.
# The slave is CLUSTER-n2; this prototype does not follow a failover.
set -u
C=$1 D=$2 OUT=$3
S=$C-n2 CL=$C-c1
ENV="export CUBRID=/work/$S/cubrid CUBRID_DATABASES=/db LD_LIBRARY_PATH=/work/$S/cubrid/lib PATH=/work/$S/cubrid/bin:\$PATH"
LOG=/work/$S/cubrid/log/$C@localhost_applylogdb_${C}_$C-n1.err
dups() { docker exec $S sh -c "grep -c -- '-670' $LOG 2>/dev/null; true"; }
clients() { docker exec $CL sh -c 'pgrep -f "csql -u dba --no-auto-commit -i c[0-9]*.sql" >/dev/null' && echo yes || echo no; }
pid() { docker exec $S sh -c 'pgrep -f "cub_admin applylogdb" | head -1'; }

echo "# $(date -Is) cluster=$C interval=${D}s dup670_before=$(dups)" >> "$OUT"
# wait up to 10 minutes for the clients to start
i=0; while [ "$(clients)" = no ] && [ $i -lt 600 ]; do sleep 1; i=$((i+1)); done
k=0
while [ "$(clients)" = yes ]; do
  sleep "$D"
  [ "$(clients)" = yes ] || break
  before=$(docker exec $S sh -c "$ENV; csql -u dba -t -c \"SELECT COUNT(*) FROM ha_applied_tx_marker\" $C@localhost 2>/dev/null | awk '/^ *[0-9]+ *\$/ {print \$1; exit}'")
  lsa=$(docker exec $S sh -c "$ENV; csql -u dba -t -c \"SELECT committed_lsa_pageid || '|' || committed_lsa_offset FROM db_ha_apply_info\" $C@localhost 2>/dev/null | awk '/[0-9]+[|][0-9]+/ {print \$1; exit}'")
  old=$(pid)
  docker exec $S sh -c 'pkill -9 -f "cub_admin applylogdb"'
  t0=$(date +%s.%N)
  new=""; j=0
  while [ $j -lt 60 ]; do new=$(pid); [ -n "$new" ] && [ "$new" != "$old" ] && break; sleep 0.5; j=$((j+1)); done
  k=$((k+1))
  echo "kill=$k at=$(date +%T) markers=${before:--} committed_lsa=${lsa:--} old_pid=$old new_pid=${new:-none} restart_s=$(echo "$(date +%s.%N) - $t0" | bc | cut -c1-4)" >> "$OUT"
done
echo "# $(date -Is) kills=$k dup670_after=$(dups)" >> "$OUT"
