#!/bin/sh
# Prototype for testkit ha-apply restart (evidence/ha/ha-apply-restart-and-the-applied-tx-marker.md),
# run from the host with docker exec against a csb pair; the command replaces it.
# In that document, the kill runs beside stress and the 8 x 3,000 run on 19e83a5d9 counted -670
# as "CODE = -670", which an applier's update failure does not carry; this copy counts it anywhere.
# parallel-apply-restart-workload.sh CLUSTER CLIENTS ITERS OUT
# The shape 72650ce0d names: a transaction inserts a fresh key and commits,
# the next one moves that key and commits. Applied a second time after a
# restart, the insert brings the old key back. Each client has its own keys,
# so the workers apply the clients in parallel. Run parallel-apply-restart-kill.sh beside it.
# At the end: master and slave counts, keys left unmoved on each (0 on the
# master), fail_counter, and the slave applier's -670 count.
set -u
C=$1 N=$2 M=$3 OUT=$4
M1=$C-n1 S=$C-n2 CL=$C-c1
env_of() { echo "export CUBRID=/work/$1/cubrid CUBRID_DATABASES=/db LD_LIBRARY_PATH=/work/$1/cubrid/lib PATH=/work/$1/cubrid/bin:\$PATH"; }
q() { # node host sql
  docker exec "$1" sh -c "$(env_of "$1"); csql -u dba -t -c \"$3\" $C@$2 2>/dev/null" | awk '/^ *-?[0-9]+ *$/ {print $1; exit}'
}
LOG=/work/$S/cubrid/log/$C@localhost_applylogdb_${C}_$M1.err
dup0=$(docker exec $S sh -c "grep -c -- '-670' $LOG 2>/dev/null; true")
fc0=$(q $S localhost "SELECT fail_counter FROM db_ha_apply_info")

docker exec $CL sh -c "$(env_of $CL); csql -u dba -c 'DROP TABLE IF EXISTS r_kv; CREATE TABLE r_kv (id INT PRIMARY KEY, c INT, i INT);' $C@$M1 >/dev/null 2>&1"
D=/tmp/insmove; docker exec $CL sh -c "rm -rf $D; mkdir -p $D"
for c in $(seq 1 $N); do
  awk -v c=$c -v m=$M 'BEGIN { for (i = 1; i <= m; i++) { k = c * 10000000 + i;
    printf "INSERT INTO r_kv VALUES (%d, %d, %d);\nCOMMIT;\nUPDATE r_kv SET id = %d WHERE id = %d;\nCOMMIT;\n", k, c, i, k + 5000000, k } }' |
    docker exec -i $CL sh -c "cat > $D/c$c.sql"
done
t0=$(date +%s)
docker exec $CL sh -c "$(env_of $CL); cd $D; for c in \$(seq 1 $N); do csql -u dba --no-auto-commit -i c\$c.sql $C@$M1 > c\$c.log 2>&1 & done; wait"
t1=$(date +%s)
# the end marker is a table of its own: a replicated statement is applied in
# commit order, so everything committed before it is on the slave when it is
E=r_end_$(date +%s)
docker exec $CL sh -c "$(env_of $CL); csql -u dba -c 'CREATE TABLE $E (i INT PRIMARY KEY); INSERT INTO $E VALUES (1);' $C@$M1 >/dev/null 2>&1"
i=0; while [ "$(q $S localhost "SELECT COUNT(*) FROM $E")" != 1 ] && [ $i -lt 300 ]; do sleep 1; i=$((i+1)); done
docker exec $CL sh -c "$(env_of $CL); csql -u dba -c 'DROP TABLE $E;' $C@$M1 >/dev/null 2>&1"
caught=$([ $i -lt 300 ] && echo caught || echo stalled)
mc=$(q $CL $M1 "SELECT COUNT(*) FROM r_kv"); sc=$(q $S localhost "SELECT COUNT(*) FROM r_kv")
mu=$(q $CL $M1 "SELECT COUNT(*) FROM r_kv WHERE id > 0 AND MOD(id, 10000000) < 5000000")
su=$(q $S localhost "SELECT COUNT(*) FROM r_kv WHERE id > 0 AND MOD(id, 10000000) < 5000000")
fc1=$(q $S localhost "SELECT fail_counter FROM db_ha_apply_info")
dup1=$(docker exec $S sh -c "grep -c -- '-670' $LOG 2>/dev/null; true")
v=same; { [ "$mc" != "$sc" ] || [ "$su" != 0 ] || [ "$fc1" != "$fc0" ] || [ "$dup1" != "$dup0" ]; } && v=diverged
[ "$caught" = stalled ] && v=stalled
echo "cluster=$C clients=$N iters=$M clients_s=$((t1 - t0)) slave=$caught master_rows=$mc slave_rows=$sc unmoved_master=$mu unmoved_slave=$su fail_counter_delta=$((fc1 - fc0)) dup670_delta=$((dup1 - dup0)) verdict=$v" | tee -a "$OUT"
