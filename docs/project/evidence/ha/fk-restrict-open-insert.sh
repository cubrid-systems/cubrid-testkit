#!/bin/sh
# A parent DELETE under a RESTRICT foreign key while a child insert is open:
# the DELETE has to wait for the child's transaction, then fail if it committed
# and go through if it rolled back.
#
#   sh /tools/fk-restrict-open-insert.sh <db> <master> <slave> commit|rollback
set -u
DB=$1 M=$2 S=$3 END=$4
R=/results/fkrestrict; mkdir -p $R
q() { csql -u dba -t -c "$2" "$DB@$1" 2>/dev/null | awk '/^ *-?[0-9]+ *$/ { print $1 }'; }
rows() { csql -u dba -c "SELECT cid, pid FROM r_child ORDER BY cid" "$DB@$1" 2>/dev/null | awk '/^ +[0-9]+ +[0-9]+ *$/ { printf "(%s,%s) ", $1, $2 }'; }

csql -u dba -c "DROP TABLE IF EXISTS r_child; DROP TABLE IF EXISTS r_parent;
  CREATE TABLE r_parent (id INT PRIMARY KEY);
  CREATE TABLE r_child (cid INT PRIMARY KEY, pid INT, CONSTRAINT fk_r FOREIGN KEY (pid) REFERENCES r_parent (id));
  INSERT INTO r_parent VALUES (1); INSERT INTO r_parent VALUES (2); INSERT INTO r_child VALUES (10, 2);" "$DB@$M" >/dev/null 2>&1

printf 'INSERT INTO r_child VALUES (21, 1);\nSELECT SLEEP(4);\n%s;\n' "$(echo $END | tr a-z A-Z)" > $R/s1.sql
csql -u dba --no-auto-commit -i $R/s1.sql "$DB@$M" > $R/s1.out 2>&1 &
sleep 1
printf 'DELETE FROM r_parent WHERE id = 1;\nCOMMIT;\n' > $R/s2.sql
t0=$(date +%s%N)
csql -u dba --no-auto-commit -i $R/s2.sql "$DB@$M" > $R/s2.out 2>&1
waited=$(( ($(date +%s%N) - t0) / 1000000 ))
wait

mk=fkr_mk_$(date +%s%N)
csql -u dba -c "CREATE TABLE $mk (i INT PRIMARY KEY); INSERT INTO $mk VALUES (1);" "$DB@$M" >/dev/null 2>&1
n=0; until [ "$(q "$S" "SELECT i FROM $mk")" = "1" ] || [ $n -ge 300 ]; do sleep 0.2; n=$((n+1)); done
csql -u dba -c "DROP TABLE $mk;" "$DB@$M" >/dev/null 2>&1

err=$(grep -oE 'ERROR CODE = -[0-9]+' $R/s2.out | head -1 | awk '{print $4}')
echo "end=$END delete_waited_ms=$waited delete_error=${err:-none}"
echo "  master: parents=$(q "$M" "SELECT COUNT(*) FROM r_parent") children=$(rows "$M")"
echo "  slave:  parents=$(q "$S" "SELECT COUNT(*) FROM r_parent") children=$(rows "$S")"
