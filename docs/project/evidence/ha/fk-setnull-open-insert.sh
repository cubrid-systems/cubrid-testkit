#!/bin/sh
# A parent-key UPDATE whose ON UPDATE SET NULL pass meets a child insert still
# open, while the inserter then deletes the other parent its row refers to
# through a RESTRICT key.
#
#   sh /tools/fk-setnull-open-insert.sh <db> <master> <slave>
#
# S1 inserts edge (1, src 1, dst 2) and holds it; S2 changes node 1's key; S1
# then deletes node 2, which its own uncommitted edge still refers to.
set -u
DB=$1 M=$2 S=$3
R=/results/fksetnull; mkdir -p $R
q() { csql -u dba -t -c "$2" "$DB@$1" 2>/dev/null | awk '/^ *-?[0-9]+ *$/ { print $1 }'; }
says() { grep -E 'ERROR CODE|row affected|rows affected' $R/$1.out | sed -E 's/.*ERROR CODE = (-[0-9]+).*/err\1/; s/ *([0-9]+) rows? affected.*/ok\1/' | tr '\n' ' '; }
state() { echo "nodes=$(csql -u dba -c "SELECT id FROM node ORDER BY id" "$DB@$1" 2>/dev/null | awk '/^ +[0-9]+ *$/ { printf "%s ", $1 }')edges=$(csql -u dba -c "SELECT id, NVL(src, -1), dst FROM edge ORDER BY id" "$DB@$1" 2>/dev/null | awk '/^ +[0-9]+ +-?[0-9]+ +[0-9]+ *$/ { printf "(%s,%s,%s) ", $1, $2, $3 }')"; }

csql -u dba -c "DROP TABLE IF EXISTS edge; DROP TABLE IF EXISTS node;
  CREATE TABLE node (id INT PRIMARY KEY);
  CREATE TABLE edge (id INT PRIMARY KEY, src INT, dst INT,
    CONSTRAINT fk_src FOREIGN KEY (src) REFERENCES node (id) ON UPDATE SET NULL,
    CONSTRAINT fk_dst FOREIGN KEY (dst) REFERENCES node (id));
  INSERT INTO node VALUES (1); INSERT INTO node VALUES (2);" "$DB@$M" >/dev/null 2>&1

printf 'INSERT INTO edge VALUES (1, 1, 2);\nSELECT SLEEP(3);\nDELETE FROM node WHERE id = 2;\nCOMMIT;\n' > $R/s1.sql
printf 'UPDATE node SET id = 10 WHERE id = 1;\nCOMMIT;\n' > $R/s2.sql
t0=$(date +%s)
csql -u dba --no-auto-commit -i $R/s1.sql "$DB@$M" > $R/s1.out 2>&1 &
sleep 1
csql -u dba --no-auto-commit -i $R/s2.sql "$DB@$M" > $R/s2.out 2>&1 &
wait
el=$(( $(date +%s) - t0 ))

mk=fks_mk_$(date +%s%N)
csql -u dba -c "CREATE TABLE $mk (i INT PRIMARY KEY); INSERT INTO $mk VALUES (1);" "$DB@$M" >/dev/null 2>&1
n=0; until [ "$(q "$S" "SELECT i FROM $mk")" = "1" ] || [ $n -ge 300 ]; do sleep 0.2; n=$((n+1)); done
csql -u dba -c "DROP TABLE $mk;" "$DB@$M" >/dev/null 2>&1

echo "elapsed_s=$el"
echo "  S1: $(says s1)   [insert edge, delete node 2]"
echo "  S2: $(says s2)   [node 1 -> 10]"
echo "  master: $(state "$M")"
echo "  slave:  $(state "$S")"
