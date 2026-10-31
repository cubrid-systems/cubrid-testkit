#!/bin/sh
# Two child inserts that fail together leave each session an X lock on the
# heap slot its rolled-back row vacated. Each then inserts again into the slot
# the other vacated, and both wait at the foreign-key index's X lock on their
# own new row (locator_add_or_remove_index_internal). The deadlock victim is A,
# which has written less log; its second insert refers to parent 17, which B
# inserted and has not committed.
#
#   sh /tools/fk-victim-reused-slot.sh <db> <master> <slave> [lock_timeout_s]
#
# A victim that is told so fails its second insert with -72 and B goes on.
# A victim that is not told so carries on to its foreign-key check, waits there
# for B, and the two wait for each other until a lock timeout (default 20 s;
# 0 waits forever and leaves the server for a restart).
set -u
DB=$1 M=$2 S=$3 TMO=${4:-20}
R=/results/fkvictim; mkdir -p $R
tmo=""; [ "$TMO" != 0 ] && tmo="SET TRANSACTION LOCK TIMEOUT $TMO;"
# One statement a line: csql drops the rest of a line after an error on it.
run() { echo "$2" | sed 's/; */;\n/g' | sed '/^ *$/d' > $R/$1.sql; csql -u dba --no-auto-commit -i $R/$1.sql "$DB@$M" > $R/$1.out 2>&1; }
q() { csql -u dba -t -c "$2" "$DB@$1" 2>/dev/null | awk '/^ *-?[0-9]+ *$/ { print $1 }'; }
rows() { csql -u dba -c "SELECT cid, pid FROM v_child ORDER BY cid" "$DB@$1" 2>/dev/null | awk '/^ +[0-9]+ +[0-9]+ *$/ { printf "(%s,%s) ", $1, $2 }'; }
says() { grep -E 'ERROR CODE|row affected|rows affected' $R/$1.out | sed -E 's/.*ERROR CODE = (-[0-9]+).*/err\1/; s/ *([0-9]+) rows? affected.*/ok\1/' | tr '\n' ' '; }

csql -u dba -c "DROP TABLE IF EXISTS v_child; DROP TABLE IF EXISTS v_parent; DROP TABLE IF EXISTS v_ballast;
  CREATE TABLE v_parent (id INT PRIMARY KEY);
  CREATE TABLE v_child (cid INT PRIMARY KEY, pid INT,
    CONSTRAINT fk_v FOREIGN KEY (pid) REFERENCES v_parent (id) ON DELETE CASCADE);
  CREATE TABLE v_ballast (i INT PRIMARY KEY, pad VARCHAR(1000));
  INSERT INTO v_parent VALUES (1); INSERT INTO v_parent VALUES (2);" "$DB@$M" >/dev/null 2>&1

t0=$(date +%s)
# C holds keys 100 and 101 in progress, so A and B both wait on it and fail together.
run c "INSERT INTO v_child VALUES (100, 1); INSERT INTO v_child VALUES (101, 1); SELECT SLEEP(4); COMMIT;" &
sleep 1
run a "$tmo INSERT INTO v_child VALUES (100, 2); SELECT SLEEP(4); INSERT INTO v_child VALUES (201, 17); COMMIT;" &
sleep 1
run b "$tmo INSERT INTO v_parent VALUES (17);
  INSERT INTO v_ballast SELECT ROWNUM, REPEAT('b', 900) FROM db_root CONNECT BY LEVEL <= 2000;
  INSERT INTO v_child VALUES (101, 2); SELECT SLEEP(2); INSERT INTO v_child VALUES (200, 1); COMMIT;" &
wait
el=$(( $(date +%s) - t0 ))

mk=fkv_mk_$(date +%s%N)
csql -u dba -c "CREATE TABLE $mk (i INT PRIMARY KEY); INSERT INTO $mk VALUES (1);" "$DB@$M" >/dev/null 2>&1
n=0; until [ "$(q "$S" "SELECT i FROM $mk")" = "1" ] || [ $n -ge 300 ]; do sleep 0.2; n=$((n+1)); done
csql -u dba -c "DROP TABLE $mk;" "$DB@$M" >/dev/null 2>&1

echo "timeout=$TMO elapsed_s=$el"
echo "  C: $(says c)"
echo "  A: $(says a)   [insert 100 -> -670 expected; insert 201 under B's parent 17]"
echo "  B: $(says b)   [parent 17, ballast, insert 101 -> -670 expected, insert 200]"
echo "  master: parent17=$(q "$M" "SELECT COUNT(*) FROM v_parent WHERE id = 17") children=$(rows "$M")"
echo "  slave:  parent17=$(q "$S" "SELECT COUNT(*) FROM v_parent WHERE id = 17") children=$(rows "$S")"
