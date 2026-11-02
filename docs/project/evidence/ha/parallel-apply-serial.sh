#!/bin/sh
# Prototype for evidence/ha/ha-apply-coverage-on-787defb50.md, run from the host against a csb pair.
# The slave's serial fragments under applier kills.
# parallel-apply-serial.sh CLUSTER CLIENTS ITERS KILL_EVERY_S OUT
# A system operation that replicates rows -- the _db_serial write-back of an AUTO_INCREMENT or a
# SERIAL's NEXT_VALUE, and INCR -- is applied on the slave as a serial fragment, in commit order,
# apart from its transaction. Each client, ITERS times: an INSERT whose id is an AUTO_INCREMENT, one
# whose id is a NOCACHE serial's NEXT_VALUE, an INCR of its own counter, and an INCR of one of its
# 50 rows of about 3 KB in r_big, whose heap update can need a new page (#8052 501e5f315 lost such
# increments), each committed. Meanwhile the slave's applier is killed every KILL_EVERY_S seconds
# (pkill inside the slave's container; 0: no kill). Once the slave shows a final marker, both nodes
# are read: each table's rows and key sum, the counters, and both serials' current values; and the
# slave's failures.
set -u
C=$1 N=$2 M=$3 K=$4 OUT=$5
M1=$C-n1 S=$C-n2 CL=$C-c1
env_of() { echo "export CUBRID=/work/$1/cubrid CUBRID_DATABASES=/db LD_LIBRARY_PATH=/work/$1/cubrid/lib PATH=/work/$1/cubrid/bin:\$PATH"; }
q() { # node host sql: the first number csql prints
  docker exec "$1" sh -c "$(env_of "$1"); csql -u dba -t -c \"$3\" $C@$2 2>/dev/null" | awk '/^ *-?[0-9]+ *$/ {print $1; exit}'
}
LOG=/work/$S/cubrid/log/$C@localhost_applylogdb_${C}_$M1.err
dups() { docker exec $S sh -c "grep -c -- '-670' $LOG 2>/dev/null; true"; }
starts() { docker exec $S sh -c "cat /work/$S/cubrid/log/*_applylogdb_*.err* 2>/dev/null | grep -c 'log applier started'; true"; }
clients() { docker exec $CL sh -c 'pgrep -f "csql -u dba --no-auto-commit -i c[0-9]*.sql" >/dev/null' && echo yes || echo no; }
dup0=$(dups); st0=$(starts); fc0=$(q $S localhost "SELECT fail_counter FROM db_ha_apply_info")
echo "# $(date -Is) cluster=$C clients=$N iters=$M kill_every=${K}s" >> "$OUT"

setup="DROP TABLE IF EXISTS r_ai; DROP TABLE IF EXISTS r_sv; DROP TABLE IF EXISTS r_cnt; DROP TABLE IF EXISTS r_big; DROP SERIAL IF EXISTS r_seq;
CREATE TABLE r_big (id INT PRIMARY KEY, n INT, pad VARCHAR(3000));
CREATE TABLE r_ai (id INT AUTO_INCREMENT PRIMARY KEY, c INT, i INT);
CREATE SERIAL r_seq START WITH 1 NOCACHE;
CREATE TABLE r_sv (id INT PRIMARY KEY, c INT, i INT);
CREATE TABLE r_cnt (c INT PRIMARY KEY, n INT);"
docker exec $CL sh -c "$(env_of $CL); csql -u dba -c \"$setup\" $C@$M1 >/dev/null 2>&1"
for c in $(seq 1 $N); do
  docker exec $CL sh -c "$(env_of $CL); csql -u dba -c 'INSERT INTO r_cnt VALUES ($c, 0);' $C@$M1 >/dev/null 2>&1"
  awk -v c=$c 'BEGIN { for (j = 0; j < 50; j++) printf "INSERT INTO r_big VALUES (%d, 0, REPEAT(\x27x\x27, 3000));\n", c * 1000 + j; print "COMMIT;" }' |
    docker exec -i $CL sh -c "$(env_of $CL); cat > /tmp/big$c.sql; csql -u dba --no-auto-commit -i /tmp/big$c.sql $C@$M1 >/dev/null 2>&1"
done
E=r_ready_$(date +%s)
docker exec $CL sh -c "$(env_of $CL); csql -u dba -c 'CREATE TABLE $E (i INT PRIMARY KEY);' $C@$M1 >/dev/null 2>&1"
i=0; while [ "$(q $S localhost "SELECT COUNT(*) FROM db_class WHERE class_name = '$E'")" != 1 ] && [ $i -lt 120 ]; do sleep 1; i=$((i+1)); done
[ $i -lt 120 ] || { echo "# the slave did not show $E within 120 s; stopping" >> "$OUT"; exit 1; }
docker exec $CL sh -c "$(env_of $CL); csql -u dba -c 'DROP TABLE $E;' $C@$M1 >/dev/null 2>&1"

W=/tmp/serialfrag; docker exec $CL sh -c "rm -rf $W; mkdir -p $W"
for c in $(seq 1 $N); do
  awk -v c=$c -v m=$M 'BEGIN { for (i = 1; i <= m; i++) {
    printf "INSERT INTO r_ai (c, i) VALUES (%d, %d);\nCOMMIT;\n", c, i;
    printf "INSERT INTO r_sv VALUES (r_seq.NEXT_VALUE, %d, %d);\nCOMMIT;\n", c, i;
    printf "SELECT INCR(n) FROM r_cnt WHERE c = %d;\nCOMMIT;\n", c;
    printf "SELECT INCR(n) FROM r_big WHERE id = %d;\nCOMMIT;\n", c * 1000 + i % 50 } }' |
    docker exec -i $CL sh -c "cat > $W/c$c.sql"
done
docker exec -d $CL sh -c "$(env_of $CL); cd $W; for c in \$(seq 1 $N); do csql -u dba --no-auto-commit -i c\$c.sql $C@$M1 > c\$c.log 2>&1 & done; wait; touch done"
i=0; while [ "$(clients)" = no ] && [ $i -lt 60 ]; do sleep 1; i=$((i+1)); done
k=0; t_c=$(date +%s)
while [ "$(clients)" = yes ]; do
  if [ "$K" = 0 ]; then sleep 1; continue; fi   # KILL_EVERY_S 0: no kill
  sleep "$K"
  [ "$(clients)" = yes ] || break
  docker exec $S sh -c 'pkill -9 -f "cub_admin [a]pplylogdb"' && k=$((k+1))
done
echo "kills=$k clients_done_at=$(date +%T) clients_s=$(( $(date +%s) - t_c ))" >> "$OUT"

E=r_end_$(date +%s)
docker exec $CL sh -c "$(env_of $CL); csql -u dba -c 'CREATE TABLE $E (i INT PRIMARY KEY);' $C@$M1 >/dev/null 2>&1"
t0=$(date +%s); i=0
while [ "$(q $S localhost "SELECT COUNT(*) FROM db_class WHERE class_name = '$E'")" != 1 ] && [ $i -lt 1800 ]; do sleep 5; i=$((i+5)); done
caught=$([ $i -lt 1800 ] && echo caught || echo stalled)
echo "slave=$caught after=$(( $(date +%s) - t0 ))s" >> "$OUT"
read_node() { # node host
  echo "ai_rows=$(q $1 $2 "SELECT COUNT(*) FROM r_ai") ai_max=$(q $1 $2 "SELECT NVL(MAX(id), 0) FROM r_ai") ai_sum=$(q $1 $2 "SELECT NVL(SUM(CAST(id AS BIGINT)), 0) FROM r_ai") sv_rows=$(q $1 $2 "SELECT COUNT(*) FROM r_sv") sv_sum=$(q $1 $2 "SELECT NVL(SUM(CAST(id AS BIGINT)), 0) FROM r_sv") cnt_sum=$(q $1 $2 "SELECT NVL(SUM(n), 0) FROM r_cnt") big_sum=$(q $1 $2 "SELECT NVL(SUM(n), 0) FROM r_big") seq=$(q $1 $2 "SELECT current_val FROM _db_serial WHERE name = 'r_seq'") ai_serial=$(q $1 $2 "SELECT current_val FROM _db_serial WHERE name = 'r_ai_ai_id'")"
}
mr=$(read_node $CL $M1); sr=$(read_node $S localhost)
echo "master $mr" >> "$OUT"
echo "slave  $sr" >> "$OUT"
fc1=$(q $S localhost "SELECT fail_counter FROM db_ha_apply_info")
v=same; [ "$mr" = "$sr" ] || v=differ; [ "$caught" = caught ] || v=stalled
echo "# $(date -Is) verdict=$v fail_counter_delta=$((fc1 - fc0)) dup670_delta=$(( $(dups) - dup0 )) starts=$(( $(starts) - st0 ))" >> "$OUT"
