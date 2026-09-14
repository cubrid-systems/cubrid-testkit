#!/usr/bin/env python3
"""Two runs of the same runner over the same corpus: do they agree with themselves?

    selfcheck.py <run1.out> <run2.out> <result-root-1> <result-root-2>

The .out files are what `ctp.sh` printed; the result roots are the two
`schedule_linux_*` directories. Prints the verdicts that moved, the failures
both runs share grouped by what their diff contains, and how much a case's
duration moves between runs -- the floor under which a difference between two
runners means nothing (ADR-013, "measured before judged").
"""
import os
import re
import statistics as st
import sys

run1, run2, root1, root2 = sys.argv[1:5]

# The verdict can land on the next line. The server the run starts inherits the
# run's standard output, and once in three runs it wrote "*** XASL generation
# failed ***" between a case's progress line and its [OK] (sql-baseline.md §6).
VERDICT = re.compile(r"Testing (\S+\.sql) \(\d+/\d+ [\d.]+%\)[^\[]*\[(OK|NOK)\]")


def verdicts(path):
    with open(path, encoding="utf-8", errors="replace") as f:
        return {m.group(1): m.group(2) for m in VERDICT.finditer(f.read())}


def durations(root):
    x = open(os.path.join(root, "summary.xml"), encoding="utf-8", errors="replace").read()
    return {c: int(e) for c, e in re.findall(
        r"<case>(.*?)</case>.*?<elapsetime>(\d+)</elapsetime>", x, re.S)}


# What a failing case's diff is about, in the order the checks are tried.
KINDS = [
    # Plan text is the only output whose digits are masked to '?', so a masked
    # identifier (t?, d?.a_?, term[?]) marks a plan or a rewritten query.
    ("plan or rewritten query", re.compile(
        r"rewritten query|subplan|sscan|iscan|idx-join|nl-join|INDEX SCAN|class: \w+ node|"
        r"index: \S+ term|edge:|order: \S+\[|gather:|Query plan|\w\?[.\s,)]|\[\?\]|\?:\?|"
        r"\(sel \?\)")),
    ("error code", re.compile(r"^Error:-?\d+", re.M)),
    ("timezone value", re.compile(r"[A-Z][a-z]+/[A-Za-z_]+ [A-Z]{2,5}\b|[+-]\d{2}:\d{2}\b")),
]


def kind_of(root, case):
    rel = case.split("/cubrid-testcases/", 1)[-1]
    base = os.path.join(root, os.path.splitext(rel)[0].replace("/cases/", "/"))
    try:
        answer = open(base + ".answer", encoding="utf-8", errors="replace").read().splitlines()
        result = open(base + ".result", encoding="utf-8", errors="replace").read().splitlines()
    except OSError:
        return "no copy in the result tree"
    changed = "\n".join(sorted(set(answer) ^ set(result)))
    if not changed.strip():
        return "whitespace or order only"
    for name, rx in KINDS:
        if rx.search(changed):
            return name
    return "values"


v1, v2 = verdicts(run1), verdicts(run2)
both = set(v1) & set(v2)
moved = sorted(c for c in both if v1[c] != v2[c])
print(f"cases: run1 {len(v1)}, run2 {len(v2)}, in both {len(both)}")
print(f"NOK: run1 {sum(1 for v in v1.values() if v == 'NOK')}, run2 {sum(1 for v in v2.values() if v == 'NOK')}")
print(f"verdicts that moved: {len(moved)}")
for c in moved:
    print(f"  {v1[c]:>3} -> {v2[c]:<3} {c.split('/cubrid-testcases/', 1)[-1]}")

shared = sorted(c for c in both if v1[c] == v2[c] == "NOK")
groups = {}
for c in shared:
    groups.setdefault(kind_of(root1, c), []).append(c.split("/cubrid-testcases/", 1)[-1])
print(f"failures in both runs: {len(shared)}")
for k, cs in sorted(groups.items(), key=lambda kv: -len(kv[1])):
    print(f"  {len(cs):3d}  {k}")
    for c in cs:
        print(f"         {c}")

d1, d2 = durations(root1), durations(root2)
common = [c for c in d1 if c in d2]
t1, t2 = sum(d1[c] for c in common), sum(d2[c] for c in common)
ratio = [d2[c] / d1[c] for c in common if d1[c] >= 1000]
print(f"case time: run1 {t1/1000:.1f}s, run2 {t2/1000:.1f}s ({100*(t2-t1)/t1:+.1f}%)")
if ratio:
    print(f"cases of 1 s or more: {len(ratio)}, run2/run1 median {st.median(ratio):.2f}, "
          f"range {min(ratio):.2f}-{max(ratio):.2f}")
