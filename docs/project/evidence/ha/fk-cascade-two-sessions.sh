#!/bin/sh
# Can a child be pointed at a parent another transaction is deleting?
#
#   sh /tools/fk-cascade-two-sessions.sh <db> <master> update|insert
#
# Session 1 deletes parent 1 (ON DELETE CASCADE) and holds its transaction open
# for five seconds. One second in, session 2 points child 2 at parent 1 --
# by UPDATE, or by INSERTing a new child -- and commits. If session 2 waited
# for session 1, it either failed (the parent was gone) or saw nothing to
# cascade; if it did not wait, the master is left with a child whose parent
# does not exist.
set -u
DB=$1 M=$2 HOW=$3
s() { csql -u dba --no-auto-commit -c "$1" "$DB@$M" 2>&1 | grep -E 'ERROR:|rows? affected' | head -2; }

s "DROP TABLE IF EXISTS r_child; DROP TABLE IF EXISTS r_parent;
   CREATE TABLE r_parent (p INT PRIMARY KEY);
   CREATE TABLE r_child (c INT PRIMARY KEY, p INT,
     CONSTRAINT fk_r FOREIGN KEY (p) REFERENCES r_parent (p) ON DELETE CASCADE);
   INSERT INTO r_parent VALUES (1); INSERT INTO r_parent VALUES (2);
   INSERT INTO r_child VALUES (1, 1); INSERT INTO r_child VALUES (2, 2); COMMIT;" >/dev/null

t0=$(date +%s%N)
( s "DELETE FROM r_parent WHERE p = 1; SELECT SLEEP(5); COMMIT;" > /tmp/fkrace.s1 ) &
sleep 1
if [ "$HOW" = update ]; then
  s2=$(s "UPDATE r_child SET p = 1 WHERE c = 2; COMMIT;")
else
  s2=$(s "INSERT INTO r_child VALUES (3, 1); COMMIT;")
fi
t2=$(( ($(date +%s%N) - t0) / 1000000 ))
wait
q() { csql -u dba -t -c "$1" "$DB@$M" 2>/dev/null | awk '/^ *-?[0-9]+ *$/ { print $1 }'; }
echo "how=$HOW session2_done_at_ms=$t2 session2=[$(echo $s2)] session1=[$(cat /tmp/fkrace.s1 | tr '\n' ' ')]"
echo "parents=$(q "SELECT COUNT(*) FROM r_parent") children: $(csql -u dba -c "SELECT c, p FROM r_child ORDER BY c" "$DB@$M" 2>/dev/null | awk '/^ +[0-9]+ +[0-9]+ *$/ { printf "(%s,%s) ", $1, $2 }') orphans=$(q "SELECT COUNT(*) FROM r_child c WHERE NOT EXISTS (SELECT 1 FROM r_parent p WHERE p.p = c.p)")"
