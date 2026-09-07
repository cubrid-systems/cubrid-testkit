# Sort the differences of one result file into named buckets, and print what
# falls into none of them.
#
# A bucket is a rule in baseline.txt. The rules exist so that silence means
# something: a run that produces only known differences prints nothing, and the
# first line of NEW output is the finding. That is the only job they have --
# a rule is not a judgement that the difference is acceptable, only that it has
# been seen and named before.
#
# Input is the marked difference of two normalised files, one line each:
#   "< text"  present only on the CTP side
#   "> text"  present only on the testkit side
#
# Rules are read first, from a file given as -v rules=<path>, in the format
#   <id> TAB <side> TAB <regex>
# where side is "<", ">" or "*". The first matching rule wins, so order in the
# file is precedence, and a narrow rule must come before a broad one.

BEGIN {
    FS = "\t"
    if (rules == "") { print "classify.awk: -v rules=<path> is required" > "/dev/stderr"; exit 2 }
    n = 0
    while ((getline line < rules) > 0) {
        if (line ~ /^[ \t]*#/ || line ~ /^[ \t]*$/) continue
        split(line, f, FS)
        if (f[1] == "" || f[3] == "") continue
        n++
        id[n] = f[1]; side[n] = f[2]; pat[n] = f[3]
        order[n] = n
    }
    close(rules)
    FS = ""
}

{
    mark = substr($0, 1, 1)
    text = substr($0, 3)
    matched = 0
    for (i = 1; i <= n; i++) {
        if (side[i] != "*" && side[i] != mark) continue
        if (text ~ pat[i]) { hits[id[i]]++; matched = 1; break }
    }
    if (!matched) { news[++nnew] = $0 }
    total++
}

END {
    printf "  differences        %d\n", total + 0
    if (n > 0) {
        printed = 0
        for (i = 1; i <= n; i++) {
            if (!(id[i] in hits)) continue
            if (seen[id[i]]++) continue
            if (!printed) { print "  known"; printed = 1 }
            printf "    %6d  %s\n", hits[id[i]], id[i]
        }
    }
    printf "  NEW                %d\n", nnew + 0
    for (i = 1; i <= nnew; i++) print "    " news[i]
    exit (nnew > 0 ? 1 : 0)
}
