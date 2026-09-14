#!/bin/bash
# ADR-013 normalisation: mask what a run is allowed to differ in, keep everything
# else. Reads a file on stdin, writes the normalised form on stdout.
#
# The date rules mask a date wherever it appears, not only next to a time. Two
# runs an hour and a half apart across midnight differ on every `date` a case
# calls, and the first comparison left 48 such lines standing -- the day and the
# AM/PM half of `Wed Sep  2 11:36:28 PM KST 2026`, which the weekday rule missed
# because it required a zero-padded day and no meridiem.
sed -E \
  -e 's#/data/cub_sys/projects/regr#<ROOT>#g' \
  -e 's#/tmp/[^ ]*#<TMP>#g' \
  -e 's/(Mon|Tue|Wed|Thu|Fri|Sat|Sun) [A-Z][a-z]{2} +[0-9]{1,2} [0-9]{2}:[0-9]{2}:[0-9]{2}( (AM|PM))? [A-Z]{3,4} [0-9]{4}/<DATE>/g' \
  -e 's/[0-9]{4}-[0-9]{2}-[0-9]{2} [0-9]{2}:[0-9]{2}:[0-9]{2}/<DATE>/g' \
  -e 's/[0-9]{4}-[0-9]{2}-[0-9]{2}/<DATE>/g' \
  -e 's/_[0-9]{8}_[0-9]{4}/_<STAMP>/g' \
  -e 's/\b[0-9]+ seconds\b/<ELAPSED> seconds/g' \
  -e 's/Elapse Time:[0-9]+/Elapse Time:<ELAPSED>/g' \
  -e 's/(time=")[0-9.e+-]+"/\1<ELAPSED>"/g' \
  -e 's/(timestamp=")[^"]*"/\1<DATE>"/g' \
  -e 's/\bpid[= ][0-9]+/pid=<PID>/gi' \
  -e 's/\b[0-9]{3,7}\b/<NUM>/g' \
  -e 's/\b1[0-9]{9}\b/<EPOCH>/g' \
  -e 's/\b[0-9]{2}:[0-9]{2}:[0-9]{2}\b/<TIME>/g' \
  -e 's/\b(duration|time)=[0-9]+/\1=<ELAPSED>/g' \
  -e 's/\b[0-9]{8}_[0-9]{4}\b/<STAMP>/g' \
  -e "s/'\[' [0-9]+ -gt/'[' <ELAPSED> -gt/g" \
  | grep -vE '^#' \
  | grep -vE '^(unix|tcp|tcp6|udp|udp6|raw|netlink|Netid|Proto|Active|RefCnt|key |------|[0-9]+ +[0-9]+ +[?a-zA-Z]|UID +PID|[a-z_]+ +[0-9]+ +[0-9]+ )' \
  | LC_ALL=C sort
