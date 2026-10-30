#!/bin/sh
# Does a marker become visible on the slave before a transaction committed
# ahead of it on the master?
#
#   sh /tools/parallel-apply-marker-probe.sh <db> <master> <slave> <rounds> <rows> dml|ddl
#
# Each round commits T_big -- <rows> rows tagged with the round, in one
# transaction -- then a marker in a second session. On the slave one SELECT
# reads the marker and T_big's row count from the same snapshot, so "marker
# there, count 0" is the marker overtaking T_big and nothing else.
#
#   dml: INSERT INTO mk VALUES (round)          -- a table that already exists
#   ddl: CREATE TABLE mk_<round>; INSERT 1 row  -- testkit's and CTP's shape
#
# Runs on a csb client node, which has GNU date (`%N`) and the engine's csql.
set -u
DB=$1 M=$2 S=$3 ROUNDS=$4 ROWS=$5 KIND=$6
OUT=/results/probe-$KIND.tsv

q() { csql -u dba -t -c "$2" "$DB@$1" 2>/dev/null | awk '/^ *-?[0-9]+ *$/ { print $1 }'; }
# A write the master refuses stops the run: a round whose T_big or marker never
# happened would otherwise poll forever or be counted as an overtake.
w() {
  out=$(csql -u dba -c "$1" "$DB@$M" 2>&1)
  case "$out" in *ERROR*) echo "master refused: $1" >&2; echo "$out" | grep ERROR >&2; exit 1 ;; esac
}

# A marker an interrupted run left on the slave would be read before this
# run's own, so every one this run could name goes first. The setup is DDL,
# applied in commit order, so an empty mk on the slave means all of it arrived.
stale=""
r=1
while [ "$r" -le "$ROUNDS" ]; do stale="$stale DROP TABLE IF EXISTS mk_$r;"; r=$((r + 1)); done
w "$stale DROP TABLE IF EXISTS big; DROP TABLE IF EXISTS mk;
CREATE TABLE big (i INT PRIMARY KEY, r INT, pad VARCHAR(2000));
CREATE INDEX big_r ON big (r);
CREATE TABLE mk (a INT PRIMARY KEY);"
until [ "$(q "$S" "SELECT COUNT(*) FROM mk")" = "0" ]; do sleep 0.2; done

printf 'round\tkind\tpolls\tseen_with_count\tfirst_seen_ms\n' > "$OUT"
r=1
while [ "$r" -le "$ROUNDS" ]; do
  base=$(( (r - 1) * ROWS ))
  w "INSERT INTO big SELECT $base + ROWNUM, $r, REPEAT('x', 1500) FROM db_root CONNECT BY LEVEL <= $ROWS"
  if [ "$KIND" = dml ]; then
    w "INSERT INTO mk VALUES ($r)"
    look="SELECT (SELECT COUNT(*) FROM big WHERE r = $r) FROM mk WHERE a = $r"
  else
    w "DROP TABLE IF EXISTS mk_$r; CREATE TABLE mk_$r (a INT PRIMARY KEY); INSERT INTO mk_$r VALUES (1)"
    look="SELECT (SELECT COUNT(*) FROM big WHERE r = $r) FROM mk_$r"
  fi
  t0=$(date +%s%N)
  polls=0
  while :; do
    polls=$((polls + 1))
    got=$(q "$S" "$look")
    [ -n "$got" ] && break
  done
  ms=$(( ($(date +%s%N) - t0) / 1000000 ))
  printf '%s\t%s\t%s\t%s\t%s\n' "$r" "$KIND" "$polls" "$got" "$ms" >> "$OUT"
  # the next round starts from a slave that has all of this one
  until [ "$(q "$S" "SELECT COUNT(*) FROM big WHERE r = $r")" = "$ROWS" ]; do sleep 0.1; done
  [ "$KIND" = ddl ] && w "DROP TABLE mk_$r"
  r=$((r + 1))
done
cat "$OUT"
