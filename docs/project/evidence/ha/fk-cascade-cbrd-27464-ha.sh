#!/bin/sh
# CBRD-27464's order, across a pair: the child write is open first, then the
# parent DELETE (ON DELETE CASCADE) waits on it, then the child write commits.
#
#   sh /tools/fk-cascade-cbrd-27464-ha.sh <db> <master> <slave> insert|update
#
#   insert: case A -- session 1 inserts child 21 under parent 1
#   update: case B -- session 1 moves child 10 from parent 1 to parent 2
#
# Prints what the master and the slave each hold afterwards.
set -u
DB=$1 M=$2 S=$3 HOW=$4
s() { csql -u dba --no-auto-commit -c "$1" "$DB@$M" 2>&1 | grep -E 'ERROR:' | head -2; }
q() { csql -u dba -t -c "$2" "$DB@$1" 2>/dev/null | awk '/^ *-?[0-9]+ *$/ { print $1 }'; }
rows() { csql -u dba -c "SELECT cid, pid FROM f_child ORDER BY cid" "$DB@$1" 2>/dev/null | awk '/^ +[0-9]+ +[0-9]+ *$/ { printf "(%s,%s) ", $1, $2 }'; }

s "DROP TABLE IF EXISTS f_child; DROP TABLE IF EXISTS f_parent;
   CREATE TABLE f_parent (id INT PRIMARY KEY);
   CREATE TABLE f_child (cid INT PRIMARY KEY, pid INT,
     CONSTRAINT fk_f FOREIGN KEY (pid) REFERENCES f_parent (id) ON DELETE CASCADE);
   INSERT INTO f_parent VALUES (1); INSERT INTO f_parent VALUES (2);
   INSERT INTO f_child VALUES (10, 1); COMMIT;" >/dev/null

if [ "$HOW" = insert ]; then w1="INSERT INTO f_child VALUES (21, 1);"; else w1="UPDATE f_child SET pid = 2 WHERE cid = 10;"; fi
( s "$w1 SELECT SLEEP(4); COMMIT;" > /tmp/fkrace2.s1 ) &
sleep 1
t0=$(date +%s%N)
s2=$(s "DELETE FROM f_parent WHERE id = 1; COMMIT;")
t2=$(( ($(date +%s%N) - t0) / 1000000 ))
wait

mk=fk2_mk_$(date +%s%N)
csql -u dba -c "CREATE TABLE $mk (i INT PRIMARY KEY); INSERT INTO $mk VALUES (1);" "$DB@$M" >/dev/null 2>&1
until [ "$(q "$S" "SELECT i FROM $mk")" = "1" ]; do sleep 0.2; done
csql -u dba -c "DROP TABLE $mk;" "$DB@$M" >/dev/null 2>&1

echo "how=$HOW delete_waited_ms=$t2 delete=[$(echo $s2)] session1=[$(cat /tmp/fkrace2.s1 | tr '\n' ' ')]"
echo "  master: parents=$(q "$M" "SELECT COUNT(*) FROM f_parent") children=$(rows "$M")"
echo "  slave:  parents=$(q "$S" "SELECT COUNT(*) FROM f_parent") children=$(rows "$S")"
