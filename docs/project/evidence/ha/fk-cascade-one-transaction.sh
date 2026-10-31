#!/bin/sh
# Two dependent operations inside ONE transaction, from one session, nothing
# concurrent. If the slave diverges here, the cause is how a transaction's own
# items are applied, not the order between transactions.
#
#   sh /tools/fk-cascade-one-transaction.sh <db> <master> <slave> <rounds> <variant>
#
#   repoint:  UPDATE child r from parent X to Y; DELETE parent X (cascades)
#   reuse:    DELETE parent X (cascades child r); INSERT child r under Y
#   delins:   DELETE child r; DELETE parent X; INSERT parent X; INSERT child r under X
set -u
DB=$1 M=$2 S=$3 ROUNDS=$4 V=$5

q() { csql -u dba -t -c "$2" "$DB@$1" 2>/dev/null | awk '/^ *-?[0-9]+ *$/ { print $1 }'; }
w() {
  out=$(csql -u dba --no-auto-commit -c "$1 COMMIT;" "$DB@$M" 2>&1)
  case "$out" in *ERROR*) echo "master refused: $1" >&2; echo "$out" | grep ERROR >&2; exit 1 ;; esac
}
dump() { csql -u dba -c "$2" "$DB@$1" 2>/dev/null | grep -v -e 'sec)' -e 'Committed' -e NOTIFICATION -e "Program 'csql'"; }
barrier() {
  mk=intxn_mk_$(date +%s%N)
  w "CREATE TABLE $mk (i INT PRIMARY KEY); INSERT INTO $mk VALUES (1);"
  until [ "$(q "$S" "SELECT i FROM $mk")" = "1" ]; do sleep 0.2; done
  w "DROP TABLE $mk;"
}

w "DROP TABLE IF EXISTS i_child; DROP TABLE IF EXISTS i_parent;
   CREATE TABLE i_parent (p INT PRIMARY KEY);
   CREATE TABLE i_child (c INT PRIMARY KEY, p INT,
     CONSTRAINT fk_i FOREIGN KEY (p) REFERENCES i_parent (p) ON DELETE CASCADE);"
barrier
fail0=$(q "$S" "SELECT fail_counter FROM db_ha_apply_info")
r=1
while [ "$r" -le "$ROUNDS" ]; do
  w "INSERT INTO i_parent VALUES ($r); INSERT INTO i_parent VALUES ($r + 1000); INSERT INTO i_child VALUES ($r, $r);"
  case $V in
    repoint) w "UPDATE i_child SET p = $r + 1000 WHERE c = $r; DELETE FROM i_parent WHERE p = $r;" ;;
    reuse)   w "DELETE FROM i_parent WHERE p = $r; INSERT INTO i_child VALUES ($r, $r + 1000);" ;;
    delins)  w "DELETE FROM i_child WHERE c = $r; DELETE FROM i_parent WHERE p = $r;
                INSERT INTO i_parent VALUES ($r); INSERT INTO i_child VALUES ($r, $r);" ;;
  esac
  r=$((r + 1))
done
barrier
fail1=$(q "$S" "SELECT fail_counter FROM db_ha_apply_info")
res=""
for t in i_parent i_child; do
  if [ "$(dump "$M" "SELECT * FROM $t ORDER BY 1")" = "$(dump "$S" "SELECT * FROM $t ORDER BY 1")" ]; then
    res="$res $t=same"
  else
    res="$res $t=DIFFER(master $(q "$M" "SELECT COUNT(*) FROM $t") rows, slave $(q "$S" "SELECT COUNT(*) FROM $t") rows)"
  fi
done
printf 'variant=%s rounds=%s fail_counter_delta=%s |%s\n' "$V" "$ROUNDS" "$((fail1 - fail0))" "$res"
