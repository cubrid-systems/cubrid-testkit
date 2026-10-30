#!/bin/sh
# fksetnull.sh with the child's change an UPDATE or a DELETE instead of an
# INSERT: S1 changes committed edge 1 and holds it, S2 changes node 1's key so
# its ON UPDATE SET NULL pass meets edge 1, and S1 then deletes a node.
#
#   sh /tools/fk-setnull-open-change.sh <db> <master> <slave> update|delete
#
#   update: S1 moves edge 1's dst from 3 to 2, then deletes node 2, which its
#           own uncommitted edge now refers to through the RESTRICT key.
#   delete: S1 deletes edge 1, then deletes node 3, which only that edge
#           referred to.
set -u
DB=$1 M=$2 S=$3 HOW=$4
R=/results/fkownwait-$HOW; mkdir -p $R
q() { csql -u dba -t -c "$2" "$DB@$1" 2>/dev/null | awk '/^ *-?[0-9]+ *$/ { print $1 }'; }
says() { grep -E 'ERROR CODE|row affected|rows affected' $R/$1.out | sed -E 's/.*ERROR CODE = (-[0-9]+).*/err\1/; s/ *([0-9]+) rows? affected.*/ok\1/' | tr '\n' ' '; }
state() { echo "nodes=$(csql -u dba -c "SELECT id FROM node ORDER BY id" "$DB@$1" 2>/dev/null | awk '/^ +[0-9]+ *$/ { printf "%s ", $1 }')edges=$(csql -u dba -c "SELECT id, NVL(src, -1), dst FROM edge ORDER BY id" "$DB@$1" 2>/dev/null | awk '/^ +[0-9]+ +-?[0-9]+ +[0-9]+ *$/ { printf "(%s,%s,%s) ", $1, $2, $3 }')"; }

csql -u dba -c "DROP TABLE IF EXISTS edge; DROP TABLE IF EXISTS node;
  CREATE TABLE node (id INT PRIMARY KEY);
  CREATE TABLE edge (id INT PRIMARY KEY, src INT, dst INT,
    CONSTRAINT fk_src FOREIGN KEY (src) REFERENCES node (id) ON UPDATE SET NULL,
    CONSTRAINT fk_dst FOREIGN KEY (dst) REFERENCES node (id));
  INSERT INTO node VALUES (1); INSERT INTO node VALUES (2); INSERT INTO node VALUES (3);
  INSERT INTO edge VALUES (1, 1, 3);" "$DB@$M" >/dev/null 2>&1

if [ "$HOW" = update ]; then
  printf 'UPDATE edge SET dst = 2 WHERE id = 1;\nSELECT SLEEP(3);\nDELETE FROM node WHERE id = 2;\nCOMMIT;\n' > $R/s1.sql
else
  printf 'DELETE FROM edge WHERE id = 1;\nSELECT SLEEP(3);\nDELETE FROM node WHERE id = 3;\nCOMMIT;\n' > $R/s1.sql
fi
printf 'UPDATE node SET id = 10 WHERE id = 1;\nCOMMIT;\n' > $R/s2.sql
t0=$(date +%s)
csql -u dba --no-auto-commit -i $R/s1.sql "$DB@$M" > $R/s1.out 2>&1 &
sleep 1
csql -u dba --no-auto-commit -i $R/s2.sql "$DB@$M" > $R/s2.out 2>&1 &
wait
el=$(( $(date +%s) - t0 ))

mk=fko_mk_$(date +%s%N)
csql -u dba -c "CREATE TABLE $mk (i INT PRIMARY KEY); INSERT INTO $mk VALUES (1);" "$DB@$M" >/dev/null 2>&1
n=0; until [ "$(q "$S" "SELECT i FROM $mk")" = "1" ] || [ $n -ge 300 ]; do sleep 0.2; n=$((n+1)); done
csql -u dba -c "DROP TABLE $mk;" "$DB@$M" >/dev/null 2>&1

echo "how=$HOW elapsed_s=$el"
echo "  S1: $(says s1)   [$HOW edge 1, then delete a node]"
echo "  S2: $(says s2)   [node 1 -> 10]"
echo "  master: $(state "$M")"
echo "  slave:  $(state "$S")"
