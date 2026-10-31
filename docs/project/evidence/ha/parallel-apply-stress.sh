#!/bin/sh
# Several clients, one small key space, a parallel applier on the other end.
#
#   sh /tools/parallel-apply-stress.sh <db> <master> <slave> <clients> <txns> <keys> <seed> [ops] [ballast]
#
# ops is a comma-separated subset of the operation numbers below (default all
# ten); ballast is the rows a slow transaction inserts first (default 3000).
#
# Each client runs <txns> transactions from a script generated from <seed>.
# A transaction touches 1-3 keys out of <keys> through primary keys, a unique
# column, a key change, and a parent/child pair with ON DELETE CASCADE; about a
# third of them first insert 3,000 ballast rows, so the slave has slow and fast
# transactions to reorder. Many statements fail on the master -- duplicate keys,
# missing parents, deadlocks -- and that is fine: whatever the master ends up
# holding is the answer, and the question is only whether the slave holds it
# too. The interleaving is the run's own and is not reproduced by the seed.
#
# Prints one line: clients, txns, keys, seed, the comparison for each table,
# fail_counter's movement, and the master's writeset counters' movement.
set -u
DB=$1 M=$2 S=$3 CLIENTS=$4 TXNS=$5 KEYS=$6 SEED=$7 OPS=${8:-0,1,2,3,4,5,6,7,8,9} BALLAST=${9:-3000}
DIR=/results/stress-$SEED
mkdir -p "$DIR"

q() { csql -u dba -t -c "$2" "$DB@$1" 2>/dev/null | awk '/^ *-?[0-9]+ *$/ { print $1 }'; }
w() {
  out=$(csql -u dba -c "$1" "$DB@$M" 2>&1)
  case "$out" in *ERROR*) echo "master refused: $1" >&2; echo "$out" | grep ERROR >&2; exit 1 ;; esac
}
dump() { csql -u dba -c "$2" "$DB@$1" 2>/dev/null | grep -v -e 'sec)' -e 'Committed' -e NOTIFICATION -e "Program 'csql'"; }
barrier() {
  mk=stress_mk_$(date +%s%N)
  w "CREATE TABLE $mk (i INT PRIMARY KEY); INSERT INTO $mk VALUES (1);"
  until [ "$(q "$S" "SELECT i FROM $mk")" = "1" ]; do sleep 0.2; done
  w "DROP TABLE $mk;"
}
ws() {
  csql -u dba -l -c "SHOW WRITESET STATUS" "$DB@$M" 2>/dev/null | awk -F: '
    { gsub(/ /, "", $1); gsub(/ /, "", $2) }
    $1 ~ /Conflict_write_count$/ { w = $2 } $1 ~ /Conflict_ref_count$/ { r = $2 }
    $1 ~ /Commit_fallback_count$/ { f = $2 } $1 ~ /Commit_count$/ { c = $2 }
    END { print c + 0, w + 0, r + 0, f + 0 }'
}

w "DROP TABLE IF EXISTS s_child; DROP TABLE IF EXISTS s_parent; DROP TABLE IF EXISTS s_kv;
   DROP TABLE IF EXISTS s_ballast;
   CREATE TABLE s_kv (id INT PRIMARY KEY, uk INT UNIQUE, v INT);
   CREATE TABLE s_parent (p INT PRIMARY KEY, v INT);
   CREATE TABLE s_child (c INT PRIMARY KEY, p INT, v INT,
     CONSTRAINT fk_s FOREIGN KEY (p) REFERENCES s_parent (p) ON DELETE CASCADE);
   CREATE TABLE s_ballast (i INT PRIMARY KEY, c INT, pad VARCHAR(1000));"
barrier

c=1
while [ "$c" -le "$CLIENTS" ]; do
  awk -v sq="'" -v seed="$SEED" -v c="$c" -v txns="$TXNS" -v keys="$KEYS" -v opsl="$OPS" -v ballast="$BALLAST" '
  # Park-Miller, exact in double: mawk srand(n) does not repeat its sequence, so the seed did not name the SQL.
  function rnd() { st = (st * 16807) % 2147483647; return st / 2147483647 }
  BEGIN {
    st = (seed * 1000 + c) % 2147483646 + 1
    nops = split(opsl, ops, ",")
    for (t = 1; t <= txns; t++) {
      if (rnd() < 0.3)
        printf "INSERT INTO s_ballast SELECT %d + ROWNUM, %d, REPEAT(%sb%s, 900) FROM db_root CONNECT BY LEVEL <= %d;\n", c * 100000000 + t * 10000, c, sq, sq, ballast
      n = 1 + int(rnd() * 3)
      for (i = 0; i < n; i++) {
        k = 1 + int(rnd() * keys); u = 1 + int(rnd() * keys); k2 = 1 + int(rnd() * keys)
        op = ops[1 + int(rnd() * nops)] + 0
        if (op == 0) printf "UPDATE s_kv SET v = v + 1 WHERE id = %d;\n", k
        else if (op == 1) printf "UPDATE s_kv SET uk = %d WHERE id = %d;\n", u, k
        else if (op == 2) printf "DELETE FROM s_kv WHERE id = %d;\n", k
        else if (op == 3) printf "INSERT INTO s_kv VALUES (%d, %d, 0);\n", k, u
        else if (op == 4) printf "UPDATE s_kv SET id = %d WHERE id = %d;\n", k2, k
        else if (op == 5) printf "INSERT INTO s_parent VALUES (%d, 0);\n", k
        else if (op == 6) printf "DELETE FROM s_parent WHERE p = %d;\n", k
        else if (op == 7) printf "INSERT INTO s_child VALUES (%d, %d, 0);\n", k, u
        else if (op == 8) printf "UPDATE s_child SET p = %d WHERE c = %d;\n", u, k
        else printf "UPDATE s_parent SET v = v + 1 WHERE p = %d;\n", k
      }
      print (rnd() < 0.05) ? "ROLLBACK;" : "COMMIT;"
    }
  }' > "$DIR/c$c.sql"
  c=$((c + 1))
done

fail0=$(q "$S" "SELECT fail_counter FROM db_ha_apply_info")
set -- $(ws); c0=$1 w0=$2 r0=$3 f0=$4
c=1
while [ "$c" -le "$CLIENTS" ]; do
  csql -u dba --no-auto-commit -i "$DIR/c$c.sql" "$DB@$M" > "$DIR/c$c.log" 2>&1 &
  c=$((c + 1))
done
# While the clients run, count the master's orphans now and then: one that a
# later statement cleaned up is gone from the final state but not from history.
oq="SELECT COUNT(*) FROM s_child c WHERE NOT EXISTS (SELECT 1 FROM s_parent p WHERE p.p = c.p)"
( peak=0 seen=0
  while [ -n "$(jobs -p 2>/dev/null)" ] || pgrep -f "csql -u dba --no-auto-commit -i $DIR/" >/dev/null; do
    o=$(q "$M" "$oq"); [ -n "$o" ] && [ "$o" -gt 0 ] && seen=$((seen + 1)) && [ "$o" -gt "$peak" ] && peak=$o
    pgrep -f "csql -u dba --no-auto-commit -i $DIR/" >/dev/null || break
  done
  echo "$seen $peak" > "$DIR/orphans.during" ) &
watcher=$!
wait
set -- $(ws); c1=$1 w1=$2 r1=$3 f1=$4
errors=$(cat "$DIR"/c*.log | grep -c 'ERROR:')

barrier
result=""
for t in s_kv s_parent s_child; do
  if [ "$(dump "$M" "SELECT * FROM $t ORDER BY 1")" = "$(dump "$S" "SELECT * FROM $t ORDER BY 1")" ]; then
    result="$result $t=same"
  else
    result="$result $t=DIFFER"
    dump "$M" "SELECT * FROM $t ORDER BY 1" > "$DIR/$t.master"
    dump "$S" "SELECT * FROM $t ORDER BY 1" > "$DIR/$t.slave"
  fi
done
# The comparison above reads each table through its primary key, so a row the heap has and no index has (an
# insert that lost its index entries) passes it. Count the heap through an expression no index covers.
for t in s_kv:id s_parent:p s_child:c; do
  tb=${t%%:*} k=${t#*:}
  hm=$(q "$M" "SELECT COUNT(*) FROM $tb WHERE NVL(v, 0) = NVL(v, 0)")
  hs=$(q "$S" "SELECT COUNT(*) FROM $tb WHERE NVL(v, 0) = NVL(v, 0)")
  im=$(q "$M" "SELECT $k FROM $tb ORDER BY 1" | wc -l)
  [ "$hm" = "$hs" ] && [ "$hm" = "$im" ] || result="$result $tb.heap(master=$hm,slave=$hs,master_by_pk=$im)"
done
bq="SELECT c, COUNT(*) FROM s_ballast GROUP BY c ORDER BY c"
if [ "$(dump "$M" "$bq")" = "$(dump "$S" "$bq")" ]; then result="$result s_ballast=same"; else result="$result s_ballast=DIFFER"; fi
# A child whose parent is gone breaks the foreign key on that node itself, and
# says the divergence began on the master rather than in replication.
read seen peak < "$DIR/orphans.during"
result="$result orphans(master=$(q "$M" "$oq"),slave=$(q "$S" "$oq")) master_orphan_polls_during=$seen peak=$peak"
fail1=$(q "$S" "SELECT fail_counter FROM db_ha_apply_info")
printf 'ops=%s clients=%s txns=%s keys=%s seed=%s master_errors=%s commits=%s conflict_write=%s conflict_ref=%s fallback=%s fail_counter_delta=%s |%s\n' \
  "$OPS" "$CLIENTS" "$TXNS" "$KEYS" "$SEED" "$errors" "$((c1 - c0))" "$((w1 - w0))" "$((r1 - r0))" "$((f1 - f0))" \
  "$((fail1 - fail0))" "$result"
