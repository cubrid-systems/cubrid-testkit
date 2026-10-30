#!/bin/sh
# Conflicting transaction pairs across a parallel applier.
#
#   sh /tools/parallel-apply-pairs.sh <db> <master> <slave> <rounds> <rows> <pattern>...
#
# Each round commits T1 -- <rows> ballast rows, then the pattern's first
# operation, in one transaction -- and then T2, the pattern's second operation
# plus a witness row, in another. The slave reads the witness and T1's ballast
# count in one SELECT: witness present with count 0 is T2 applied ahead of T1.
# For `ctl` that is expected (no shared key); for every other pattern T2
# depends on T1 and that is the finding. After the rounds the small tables are
# compared across the pair, behind a CREATE TABLE marker, and fail_counter's
# movement is reported.
set -u
DB=$1 M=$2 S=$3 ROUNDS=$4 ROWS=$5
shift 5

q() { csql -u dba -t -c "$2" "$DB@$1" 2>/dev/null | awk '/^ *-?[0-9]+ *$/ { print $1 }'; }
# One transaction on the master. A refusal is printed, because a pattern whose
# T1 never ran proves nothing about T2.
x() {
  out=$(csql -u dba --no-auto-commit -c "$1 COMMIT;" "$DB@$M" 2>&1)
  case "$out" in *ERROR*) echo "master refused: $1 -> $(echo "$out" | grep ERROR | head -1)" >&2 ;; esac
}
dump() { csql -u dba -c "$2" "$DB@$1" 2>/dev/null | grep -v -e 'sec)' -e 'Committed' -e NOTIFICATION -e "Program 'csql'"; }
# The master's writeset counters: write and REF conflicts found, commit-order
# fallbacks, history clears. A pattern that held because of a key conflict moves
# the first two; one that held only because of a fallback moves the third.
ws() {
  csql -u dba -l -c "SHOW WRITESET STATUS" "$DB@$M" 2>/dev/null | awk -F: '
    { gsub(/ /, "", $1); gsub(/ /, "", $2) }
    $1 ~ /Conflict_write_count$/ { w = $2 } $1 ~ /Conflict_ref_count$/ { r = $2 }
    $1 ~ /Commit_fallback_count$/ { f = $2 } $1 ~ /History_clear_count$/ { c = $2 }
    END { print w + 0, r + 0, f + 0, c + 0 }'
}
barrier() {
  mk=pairs_mk_$(date +%s%N)
  x "CREATE TABLE $mk (i INT PRIMARY KEY); INSERT INTO $mk VALUES (1);"
  until [ "$(q "$S" "SELECT i FROM $mk")" = "1" ]; do sleep 0.1; done
  x "DROP TABLE $mk;"
}

setup() { # $1 pattern
  p=$1
  x "DROP TABLE IF EXISTS child_$p; DROP TABLE IF EXISTS parent_$p; DROP TABLE IF EXISTS sub_$p; DROP TABLE IF EXISTS t_$p;
     DROP TABLE IF EXISTS w_$p; DROP TABLE IF EXISTS ballast_$p;
     CREATE TABLE ballast_$p (i INT PRIMARY KEY, r INT, pad VARCHAR(2000));
     CREATE INDEX ballast_${p}_r ON ballast_$p (r);
     CREATE TABLE w_$p (r INT PRIMARY KEY);"
  case $p in
    ctl|pk_update|pk_move) x "CREATE TABLE t_$p (id INT PRIMARY KEY, v INT);" ;;
    uk_reinsert|uk_move)   x "CREATE TABLE t_$p (id INT PRIMARY KEY, uk INT UNIQUE);" ;;
    fk_insert|fk_delete)   x "CREATE TABLE parent_$p (p INT PRIMARY KEY);
                              CREATE TABLE child_$p (c INT PRIMARY KEY, p INT,
                                CONSTRAINT fk_$p FOREIGN KEY (p) REFERENCES parent_$p (p));" ;;
    rk_reinsert)           x "CREATE TABLE t_$p (id INT PRIMARY KEY, uk INT);
                              CREATE REVERSE UNIQUE INDEX rk_$p ON t_$p (uk);" ;;
    fk_composite)          x "CREATE TABLE parent_$p (a INT, b INT, PRIMARY KEY (a, b));
                              CREATE TABLE child_$p (c INT PRIMARY KEY, a INT, b INT,
                                CONSTRAINT fk_$p FOREIGN KEY (a, b) REFERENCES parent_$p (a, b));" ;;
    fk_cascade|fk_repoint|fk_cascade_reuse)
                           x "CREATE TABLE parent_$p (p INT PRIMARY KEY);
                              CREATE TABLE child_$p (c INT PRIMARY KEY, p INT,
                                CONSTRAINT fk_$p FOREIGN KEY (p) REFERENCES parent_$p (p) ON DELETE CASCADE);" ;;
    sp_keep|sp_redo|ddl_dml) x "CREATE TABLE t_$p (id INT PRIMARY KEY, v INT);" ;;
    part_move)             x "CREATE TABLE t_$p (id INT PRIMARY KEY, v INT)
                                PARTITION BY RANGE (id) (PARTITION p0 VALUES LESS THAN (100000000),
                                                         PARTITION p1 VALUES LESS THAN MAXVALUE);" ;;
    # Keys the index holds equal while their bytes differ.
    fp_negzero)            x "CREATE TABLE t_$p (id INT PRIMARY KEY, g INT, d DOUBLE, UNIQUE (g, d));" ;;
    tstz_zone)             x "CREATE TABLE t_$p (id INT PRIMARY KEY, ts TIMESTAMPTZ UNIQUE);" ;;
    dttz_zone)             x "CREATE TABLE t_$p (id INT PRIMARY KEY, ts DATETIMETZ UNIQUE);" ;;
    ci_case)               x "CREATE TABLE t_$p (id INT PRIMARY KEY, s VARCHAR(20) COLLATE utf8_en_ci UNIQUE);" ;;
    inherit_uk)            x "CREATE TABLE t_$p (id INT PRIMARY KEY, uk INT UNIQUE); CREATE TABLE sub_$p UNDER t_$p;" ;;
    fk_negzero)            x "CREATE TABLE parent_$p (g INT, d DOUBLE, PRIMARY KEY (g, d));
                              CREATE TABLE child_$p (c INT PRIMARY KEY, g INT, d DOUBLE,
                                CONSTRAINT fk_$p FOREIGN KEY (g, d) REFERENCES parent_$p (g, d));" ;;
    fk_tz)                 x "CREATE TABLE parent_$p (ts TIMESTAMPTZ PRIMARY KEY);
                              CREATE TABLE child_$p (c INT PRIMARY KEY, ts TIMESTAMPTZ,
                                CONSTRAINT fk_$p FOREIGN KEY (ts) REFERENCES parent_$p (ts));" ;;
    fk_ci)                 x "CREATE TABLE parent_$p (s VARCHAR(20) COLLATE utf8_en_ci PRIMARY KEY);
                              CREATE TABLE child_$p (c INT PRIMARY KEY, s VARCHAR(20) COLLATE utf8_en_ci,
                                CONSTRAINT fk_$p FOREIGN KEY (s) REFERENCES parent_$p (s));" ;;
  esac
}

# pre: committed and replicated before the round. t1: the operation after the
# ballast. t2: the dependent operation.
round_sql() { # $1 pattern $2 round
  p=$1 r=$2
  pre="" t1="" t2=""
  case $p in
    ctl)         t2="INSERT INTO t_$p VALUES ($r, $r);" ;;
    pk_update)   pre="INSERT INTO t_$p VALUES ($r, 0);"
                 t1="UPDATE t_$p SET v = 1 WHERE id = $r;"
                 t2="UPDATE t_$p SET v = 2 WHERE id = $r;" ;;
    pk_move)     pre="INSERT INTO t_$p VALUES ($r, 0);"
                 t1="UPDATE t_$p SET id = id + 100000000 WHERE id = $r;"
                 t2="INSERT INTO t_$p VALUES ($r, 1);" ;;
    uk_reinsert) pre="INSERT INTO t_$p VALUES ($r * 2, $r);"
                 t1="DELETE FROM t_$p WHERE uk = $r;"
                 t2="INSERT INTO t_$p VALUES ($r * 2 + 1, $r);" ;;
    uk_move)     pre="INSERT INTO t_$p VALUES ($r * 2, $r); INSERT INTO t_$p VALUES ($r * 2 + 1, -$r);"
                 t1="UPDATE t_$p SET uk = uk + 100000000 WHERE id = $r * 2;"
                 t2="UPDATE t_$p SET uk = $r WHERE id = $r * 2 + 1;" ;;
    fk_insert)   t1="INSERT INTO parent_$p VALUES ($r);"
                 t2="INSERT INTO child_$p VALUES ($r, $r);" ;;
    fk_delete)   pre="INSERT INTO parent_$p VALUES ($r); INSERT INTO child_$p VALUES ($r, $r);"
                 t1="DELETE FROM child_$p WHERE c = $r;"
                 t2="DELETE FROM parent_$p WHERE p = $r;" ;;
    rk_reinsert) pre="INSERT INTO t_$p VALUES ($r * 2, $r);"
                 t1="DELETE FROM t_$p WHERE uk = $r;"
                 t2="INSERT INTO t_$p VALUES ($r * 2 + 1, $r);" ;;
    fk_composite) t1="INSERT INTO parent_$p VALUES ($r, -$r);"
                 t2="INSERT INTO child_$p VALUES ($r, $r, -$r);" ;;
    # T2's own delete reaches the child only through the cascade, so the key
    # T1 wrote is one T2 never names.
    fk_cascade)  pre="INSERT INTO parent_$p VALUES ($r);"
                 t1="INSERT INTO child_$p VALUES ($r, $r);"
                 t2="DELETE FROM parent_$p WHERE p = $r;" ;;
    # The row is written before the savepoint and kept; only a later change to
    # it is rolled back.
    sp_keep)     t1="INSERT INTO t_$p VALUES ($r, 0); SAVEPOINT s1; UPDATE t_$p SET v = 9 WHERE id = $r;
                     ROLLBACK TO SAVEPOINT s1;"
                 t2="UPDATE t_$p SET v = 2 WHERE id = $r;" ;;
    # The row is written, rolled back, and written again: the key that
    # survives is the second write's.
    sp_redo)     t1="SAVEPOINT s1; INSERT INTO t_$p VALUES ($r, 0); ROLLBACK TO SAVEPOINT s1;
                     INSERT INTO t_$p VALUES ($r, 5);"
                 t2="UPDATE t_$p SET v = 2 WHERE id = $r;" ;;
    # The child leaves X for Y, then X goes. The child's UPDATE names X only
    # as the value it had.
    fk_repoint)  pre="INSERT INTO parent_$p VALUES ($r); INSERT INTO parent_$p VALUES ($r + 1000);
                     INSERT INTO child_$p VALUES ($r, $r);"
                 t1="UPDATE child_$p SET p = $r + 1000 WHERE c = $r;"
                 t2="DELETE FROM parent_$p WHERE p = $r;" ;;
    # X goes and takes child r with it by cascade; then a new child r is made
    # under Y. Only the cascade wrote child r in T1.
    fk_cascade_reuse) pre="INSERT INTO parent_$p VALUES ($r); INSERT INTO parent_$p VALUES ($r + 1000);
                     INSERT INTO child_$p VALUES ($r, $r);"
                 t1="DELETE FROM parent_$p WHERE p = $r;"
                 t2="INSERT INTO child_$p VALUES ($r, $r + 1000);" ;;
    ddl_dml)     t1="ALTER TABLE t_$p ADD COLUMN c$r INT;"
                 t2="INSERT INTO t_$p (id, c$r) VALUES ($r, $r);" ;;
    part_move)   pre="INSERT INTO t_$p VALUES ($r, 0);"
                 t1="UPDATE t_$p SET id = id + 100000000 WHERE id = $r;"
                 t2="INSERT INTO t_$p VALUES ($r, 1);" ;;
    # T1 frees a key and T2 takes one the index holds equal to it but written
    # differently: -0.0 and +0.0, one instant in two zones, two cases under a
    # case-insensitive collation, the subclass and the superclass of one unique
    # hierarchy. A writeset that hashes the bytes misses the dependency.
    fp_negzero)  pre="INSERT INTO t_$p VALUES ($r * 2, $r, -0e0);"
                 t1="DELETE FROM t_$p WHERE id = $r * 2;"
                 t2="INSERT INTO t_$p VALUES ($r * 2 + 1, $r, 0e0);" ;;
    tstz_zone|dttz_zone)
                 ty=TIMESTAMPTZ; [ "$p" = dttz_zone ] && ty=DATETIMETZ
                 pre="INSERT INTO t_$p VALUES ($r * 2, $ty$(tz0 $r));"
                 t1="DELETE FROM t_$p WHERE id = $r * 2;"
                 t2="INSERT INTO t_$p VALUES ($r * 2 + 1, $ty$(tz9 $r));" ;;
    ci_case)     pre="INSERT INTO t_$p VALUES ($r * 2, 'Key$r');"
                 t1="DELETE FROM t_$p WHERE id = $r * 2;"
                 t2="INSERT INTO t_$p VALUES ($r * 2 + 1, 'KEY$r');" ;;
    inherit_uk)  pre="INSERT INTO sub_$p VALUES ($r * 2, $r);"
                 t1="DELETE FROM sub_$p WHERE uk = $r;"
                 t2="INSERT INTO t_$p VALUES ($r * 2 + 1, $r);" ;;
    # The same equalities as a reference: T1 adds the parent one way and T2's
    # child names it the other way.
    fk_negzero)  t1="INSERT INTO parent_$p VALUES ($r, -0e0);"
                 t2="INSERT INTO child_$p VALUES ($r, $r, 0e0);" ;;
    fk_tz)       t1="INSERT INTO parent_$p VALUES (TIMESTAMPTZ$(tz0 $r));"
                 t2="INSERT INTO child_$p VALUES ($r, TIMESTAMPTZ$(tz9 $r));" ;;
    fk_ci)       t1="INSERT INTO parent_$p VALUES ('Key$r');"
                 t2="INSERT INTO child_$p VALUES ($r, 'KEY$r');" ;;
  esac
}
# One instant per round, written in UTC and in +09:00.
tz0() { printf "'2020-01-01 00:%02d:%02d +00:00'" $(($1 / 60)) $(($1 % 60)); }
tz9() { printf "'2020-01-01 09:%02d:%02d +09:00'" $(($1 / 60)) $(($1 % 60)); }

tables_of() {
  case $1 in fk_*) echo "parent_$1 child_$1" ;; inherit_uk) echo "t_$1 sub_$1" ;; *) echo "t_$1" ;; esac
}

printf 'pattern\trounds\tt2_ahead_of_t1\tt2_never_seen\tfinal\tfail_counter_delta\tconflict_write\tconflict_ref\tfallback\tclear\n'
for p in "$@"; do
  setup "$p"
  barrier
  fail0=$(q "$S" "SELECT fail_counter FROM db_ha_apply_info")
  ahead=0 never=0 r=1
  while [ "$r" -le "$ROUNDS" ]; do
    round_sql "$p" "$r"
    if [ -n "$pre" ]; then x "$pre"; barrier; fi
    base=$(( r * ROWS ))
    # Counted around T1 and T2 alone: the pre-writes and barriers above are DDL
    # and would add a fallback and a clear of their own.
    set -- $(ws); w0=$1 r0=$2 f0=$3 c0=$4
    x "INSERT INTO ballast_$p SELECT $base + ROWNUM, $r, REPEAT('x', 1500) FROM db_root CONNECT BY LEVEL <= $ROWS; $t1"
    x "$t2 INSERT INTO w_$p VALUES ($r);"
    set -- $(ws); w1=$1 r1=$2 f1=$3 c1=$4
    dw=$((${dw:-0} + w1 - w0)) dr=$((${dr:-0} + r1 - r0)) df=$((${df:-0} + f1 - f0)) dc=$((${dc:-0} + c1 - c0))
    t0=$(date +%s)
    while :; do
      got=$(q "$S" "SELECT (SELECT COUNT(*) FROM ballast_$p WHERE r = $r) FROM w_$p WHERE r = $r")
      [ -n "$got" ] && break
      if [ $(( $(date +%s) - t0 )) -ge 30 ]; then got=never; break; fi
    done
    case $got in never) never=$((never + 1)) ;; 0) ahead=$((ahead + 1)) ;; esac
    until [ "$(q "$S" "SELECT COUNT(*) FROM ballast_$p WHERE r = $r")" = "$ROWS" ]; do sleep 0.1; done
    r=$((r + 1))
  done
  barrier
  final=same
  for t in $(tables_of "$p"); do
    if [ "$(dump "$M" "SELECT * FROM $t ORDER BY 1")" != "$(dump "$S" "SELECT * FROM $t ORDER BY 1")" ]; then
      final="differ:$t"
    fi
  done
  fail1=$(q "$S" "SELECT fail_counter FROM db_ha_apply_info")
  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$p" "$ROUNDS" "$ahead" "$never" "$final" "$((fail1 - fail0))" \
    "$dw" "$dr" "$df" "$dc"
  dw=0 dr=0 df=0 dc=0
  # The ballast is 20 rounds of it on both nodes; a run of many patterns fills a disk.
  x "DROP TABLE ballast_$p;"
done
