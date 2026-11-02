#!/bin/sh
# Prototype for evidence/ha/ha-apply-and-a-crashed-master.md, run from the host against a csb pair.
# CSB_HOME and the csb binary are this host's; set them for another.
# A crash of the master under load, and the slave that takes over.
# parallel-apply-failover.sh CLUSTER CLIENTS ITERS DELAY_S OUT
# Clients on CLUSTER-c1 run the restart workload against the master CLUSTER-n1
# (each inserts a key of its own, commits, moves it, commits). DELAY_S seconds
# in, the master node is crashed (csb node kill: no flush). The run waits for
# CLUSTER-n2 to say it is active, then reads its table client by client and
# sets it against what each client saw: A acknowledged commits ("Execute OK.")
# leave floor(A/2) keys moved and A mod 2 not; one more, the commit in flight
# when the master went, may be there or not. A client whose keys show fewer
# commits than it saw acknowledged lost a commit in the failover; one with more
# than one extra, or keys out of that shape, is a second apply or a hole.
set -u
C=$1 N=$2 M=$3 D=$4 OUT=$5
M1=$C-n1 S=$C-n2 CL=$C-c1
export CSB_HOME=/data/cub_sys/projects/workspace/testkit-wsprobe/csb-home
B=/data/cub_sys/projects/cubrid-testkit/extensions/cluster-sandbox/bin/csb
env_of() { echo "export CUBRID=/work/$1/cubrid CUBRID_DATABASES=/db LD_LIBRARY_PATH=/work/$1/cubrid/lib PATH=/work/$1/cubrid/bin:\$PATH"; }
q() { # node host sql
  docker exec "$1" sh -c "$(env_of "$1"); csql -u dba -t -c \"$3\" $C@$2 2>/dev/null" | awk '/^ *-?[0-9]+ *$/ {print $1; exit}'
}
mode() { docker exec "$1" sh -c "$(env_of "$1"); cubrid changemode $C@localhost 2>&1" | sed -n "s/.*current HA running mode is \([a-z-]*\).*/\1/p" | head -1; }
LOG=/work/$S/cubrid/log/$C@localhost_applylogdb_${C}_$M1.err
dup0=$(docker exec $S sh -c "grep -c -- '-670' $LOG 2>/dev/null; true")
fc0=$(q $S localhost "SELECT fail_counter FROM db_ha_apply_info")
sync=$(docker exec $S sh -c "grep -E '^ha_copy_sync_mode' /work/$S/cubrid/conf/cubrid_ha.conf 2>/dev/null | tail -1")
echo "# $(date -Is) cluster=$C clients=$N iters=$M delay=${D}s ${sync:-ha_copy_sync_mode=unset} n1=$(mode $M1) n2=$(mode $S)" >> "$OUT"

docker exec $CL sh -c "$(env_of $CL); csql -u dba -c 'DROP TABLE IF EXISTS r_kv; CREATE TABLE r_kv (id INT PRIMARY KEY, c INT, i INT);' $C@$M1 >/dev/null 2>&1"
E=r_ready_$(date +%s)
docker exec $CL sh -c "$(env_of $CL); csql -u dba -c 'CREATE TABLE $E (i INT PRIMARY KEY);' $C@$M1 >/dev/null 2>&1"
i=0; while [ "$(q $S localhost "SELECT COUNT(*) FROM db_class WHERE class_name = '$E'")" != 1 ] && [ $i -lt 120 ]; do sleep 1; i=$((i+1)); done
[ $i -lt 120 ] || { echo "# the slave did not show $E within 120 s; stopping" >> "$OUT"; exit 1; }
docker exec $CL sh -c "$(env_of $CL); csql -u dba -c 'DROP TABLE $E;' $C@$M1 >/dev/null 2>&1"
W=/tmp/failover; docker exec $CL sh -c "rm -rf $W; mkdir -p $W"
for c in $(seq 1 $N); do
  awk -v c=$c -v m=$M 'BEGIN { for (i = 1; i <= m; i++) { k = c * 10000000 + i;
    printf "INSERT INTO r_kv VALUES (%d, %d, %d);\nCOMMIT;\nUPDATE r_kv SET id = %d WHERE id = %d;\nCOMMIT;\n", k, c, i, k + 5000000, k } }' |
    docker exec -i $CL sh -c "cat > $W/c$c.sql"
done
docker exec -d $CL sh -c "$(env_of $CL); cd $W; for c in \$(seq 1 $N); do csql -u dba --no-auto-commit -i c\$c.sql $C@$M1 > c\$c.log 2>&1 & done; wait; touch done"
sleep "$D"
t0=$(date +%s)
$B node kill $M1 --cluster $C -q >/dev/null 2>&1; krc=$?
i=0; while ! docker exec $CL test -f $W/done && [ $i -lt 600 ]; do sleep 1; i=$((i+1)); done
i=0; m=""; while [ $i -lt 600 ]; do m=$(mode $S); [ "$m" = active ] && break; sleep 2; i=$((i+2)); done
t1=$(date +%s)
echo "crash at=$(date -d @$t0 +%T) kill_rc=$krc n2=$m after=$((t1 - t0))s" >> "$OUT"

bad=0 lost=0 extra=0
for c in $(seq 1 $N); do
  a=$(docker exec $CL sh -c "grep -c 'Execute OK' $W/c$c.log; true")
  lo=$((c * 10000000)); hi=$((lo + 10000000)); mv=$((lo + 5000000))
  moved=$(q $S localhost "SELECT COUNT(*) FROM r_kv WHERE id >= $mv AND id < $hi")
  un=$(q $S localhost "SELECT COUNT(*) FROM r_kv WHERE id > $lo AND id < $mv")
  ui=$(q $S localhost "SELECT NVL(MAX(id) - $lo, 0) FROM r_kv WHERE id > $lo AND id < $mv")
  mx=$(q $S localhost "SELECT NVL(MAX(id) - $mv, 0) FROM r_kv WHERE id >= $mv AND id < $hi")
  j=$((moved * 2 + un))         # the commits the keys show
  ok=yes
  [ "$mx" = "$moved" ] || ok=no  # the moved keys are 1..moved, no hole
  [ "$un" -le 1 ] || ok=no
  [ "$un" = 0 ] || [ "$ui" = $((moved + 1)) ] || ok=no
  if [ "$j" -lt "$a" ]; then lost=$((lost + a - j)); ok=no; fi
  if [ "$j" -gt $((a + 1)) ]; then extra=$((extra + j - a - 1)); ok=no; fi
  [ $ok = yes ] || bad=$((bad + 1))
  echo "client=$c acked=$a shown=$j moved=$moved unmoved=$un ok=$ok" >> "$OUT"
done
fc1=$(q $S localhost "SELECT fail_counter FROM db_ha_apply_info")
dup1=$(docker exec $S sh -c "grep -c -- '-670' $LOG 2>/dev/null; true")
echo "# $(date -Is) clients_bad=$bad acked_lost=$lost extra=$extra fail_counter_delta=$((fc1 - fc0)) dup670_delta=$((dup1 - dup0))" >> "$OUT"
