#!/bin/sh
# A dependent write that follows its predecessor's commit as closely as it can.
#
#   sh /tools/parallel-apply-chaser.sh <db> <master> <slave> <rounds> fk|uk <noise-clients>
#
# The master makes a commit visible to other transactions (logtb_complete_mvcc)
# before it logs the commit and before it publishes the commit's keys to the
# writeset history. A transaction that sees the commit and decides its own
# label in that gap would find nothing to depend on. This tries to land there.
#
#   A  repeats: ballast rows, then the predecessor write, then COMMIT
#        fk: INSERT a parent i          uk: DELETE the row holding unique i
#   B  repeats one statement thousands of times, each its own commit, that
#      writes the dependent row for every predecessor it can see:
#        fk: a child for each visible parent that has none
#        uk: a new row with unique i for each i no row holds any more
#   N  noise clients commit one-row inserts as fast as they can, so commits
#      queue on the log and the gap is wider.
#
# A dependent row applied on the slave ahead of its predecessor fails there --
# a missing parent, a duplicate unique key -- and moves fail_counter. The
# tables are compared across the pair afterwards.
set -u
DB=$1 M=$2 S=$3 ROUNDS=$4 MODE=$5 NOISE=$6
DIR=/results/chaser-$MODE
mkdir -p "$DIR"

q() { csql -u dba -t -c "$2" "$DB@$1" 2>/dev/null | awk '/^ *-?[0-9]+ *$/ { print $1 }'; }
w() {
  out=$(csql -u dba -c "$1" "$DB@$M" 2>&1)
  case "$out" in *ERROR*) echo "master refused: $1" >&2; echo "$out" | grep ERROR >&2; exit 1 ;; esac
}
dump() { csql -u dba -c "$2" "$DB@$1" 2>/dev/null | grep -v -e 'sec)' -e 'Committed' -e NOTIFICATION -e "Program 'csql'"; }
barrier() {
  mk=chaser_mk_$(date +%s%N)
  w "CREATE TABLE $mk (i INT PRIMARY KEY); INSERT INTO $mk VALUES (1);"
  until [ "$(q "$S" "SELECT i FROM $mk")" = "1" ]; do sleep 0.2; done
  w "DROP TABLE $mk;"
}

w "DROP TABLE IF EXISTS ch_c; DROP TABLE IF EXISTS ch_p; DROP TABLE IF EXISTS ch_u;
   DROP TABLE IF EXISTS ch_seq; DROP TABLE IF EXISTS ch_b; DROP TABLE IF EXISTS ch_n;
   CREATE TABLE ch_p (p INT PRIMARY KEY);
   CREATE TABLE ch_c (c INT PRIMARY KEY, p INT, CONSTRAINT fk_ch FOREIGN KEY (p) REFERENCES ch_p (p));
   CREATE TABLE ch_u (id INT PRIMARY KEY, uk INT UNIQUE);
   CREATE TABLE ch_seq (i INT PRIMARY KEY);
   CREATE TABLE ch_b (i INT PRIMARY KEY, r INT, pad VARCHAR(1000));
   CREATE TABLE ch_n (i INT PRIMARY KEY);
   INSERT INTO ch_seq SELECT ROWNUM FROM db_root CONNECT BY LEVEL <= $ROUNDS;
   INSERT INTO ch_u SELECT i, i FROM ch_seq;"
barrier

i=1
: > "$DIR/a.sql"
while [ "$i" -le "$ROUNDS" ]; do
  echo "INSERT INTO ch_b SELECT $i * 10000 + ROWNUM, $i, REPEAT('b', 900) FROM db_root CONNECT BY LEVEL <= 2000;" >> "$DIR/a.sql"
  if [ "$MODE" = fk ]; then echo "INSERT INTO ch_p VALUES ($i);" >> "$DIR/a.sql"
  else echo "DELETE FROM ch_u WHERE uk = $i;" >> "$DIR/a.sql"; fi
  echo "COMMIT;" >> "$DIR/a.sql"
  i=$((i + 1))
done
if [ "$MODE" = fk ]; then
  chase="INSERT INTO ch_c SELECT p, p FROM ch_p WHERE p NOT IN (SELECT c FROM ch_c); COMMIT;"
else
  chase="INSERT INTO ch_u SELECT i + 100000000, i FROM ch_seq WHERE i NOT IN (SELECT uk FROM ch_u); COMMIT;"
fi
awk -v s="$chase" 'BEGIN { for (k = 0; k < 40000; k++) print s }' > "$DIR/b.sql"
n=1
while [ "$n" -le "$NOISE" ]; do
  awk -v n="$n" 'BEGIN { for (k = 1; k <= 20000; k++) printf "INSERT INTO ch_n VALUES (%d); COMMIT;\n", n * 1000000 + k }' > "$DIR/n$n.sql"
  n=$((n + 1))
done

if [ "$MODE" = fk ]; then
  chased_q="SELECT COUNT(*) FROM ch_c"
else
  chased_q="SELECT COUNT(*) FROM ch_u WHERE id > 100000000"
fi
fail0=$(q "$S" "SELECT fail_counter FROM db_ha_apply_info")
pids=""
n=1
while [ "$n" -le "$NOISE" ]; do
  csql -u dba --no-auto-commit -i "$DIR/n$n.sql" "$DB@$M" > "$DIR/n$n.log" 2>&1 &
  pids="$pids $!"
  n=$((n + 1))
done
csql -u dba --no-auto-commit -i "$DIR/b.sql" "$DB@$M" > "$DIR/b.log" 2>&1 &
pids="$pids $!"
csql -u dba --no-auto-commit -i "$DIR/a.sql" "$DB@$M" > "$DIR/a.log" 2>&1
# B has the last few rounds still to chase; then it and the noise have done
# their work, and what is left of their scripts is only time.
t0=$(date +%s)
until [ "$(q "$M" "$chased_q")" = "$ROUNDS" ] || [ $(( $(date +%s) - t0 )) -ge 60 ]; do sleep 0.5; done
kill $pids 2>/dev/null
wait
chased=$(q "$M" "$chased_q")
barrier
result=""
for t in ch_p ch_c ch_u; do
  if [ "$(dump "$M" "SELECT * FROM $t ORDER BY 1")" = "$(dump "$S" "SELECT * FROM $t ORDER BY 1")" ]; then
    result="$result $t=same"
  else
    result="$result $t=DIFFER"
  fi
done
fail1=$(q "$S" "SELECT fail_counter FROM db_ha_apply_info")
printf 'mode=%s rounds=%s noise=%s chased=%s a_errors=%s b_errors=%s fail_counter_delta=%s |%s\n' \
  "$MODE" "$ROUNDS" "$NOISE" "$chased" "$(grep -c 'ERROR:' "$DIR/a.log")" "$(grep -c 'ERROR:' "$DIR/b.log")" \
  "$((fail1 - fail0))" "$result"
