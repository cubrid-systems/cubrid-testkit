#!/usr/bin/env python3
"""ADR-017's gate: CTP's result tree against sqlsuite's, at the same pins.

  gate-compare.py <ctp result dir> <native result dir>

Compares what a run says rather than how long it took: the verdict of every
case, main.info, summary.info, and every summary_info with its counts. The
fields that are a clock or a path into this run are normalised away; everything
else has to be identical.
"""
import os, re, sys

ctp, native = sys.argv[1], sys.argv[2]
bad = 0

def say(ok, what, detail=""):
    global bad
    print(f"{'ok  ' if ok else 'DIFF'} {what}{(': ' + detail) if detail else ''}")
    if not ok:
        bad += 1

def read(path):
    try:
        return open(path, encoding="utf-8", errors="replace").read()
    except OSError as e:
        return f"<<unreadable: {e}>>"

# A run's own clock and place. Times are in the records on purpose -- they are
# CQT's own fields -- and cannot match between two runs of different lengths.
TIMES = re.compile(r"^(elapse_time|totalTime|end_time|start_time|result_path):.*$", re.M)
MS = re.compile(r"^totalTime:\d+ms$", re.M)
RUNDIR = re.compile(r"(?:schedule_[a-z]+_)?[a-z_]*64bit_\d+_[0-9.]+-[0-9a-f]+")

def normalise(text):
    text = TIMES.sub(lambda m: m.group(1) + ":<time>", text)
    text = MS.sub("totalTime:<time>ms", text)
    text = RUNDIR.sub("<run>", text)
    # XStream's own elapsed fields, and the JUnit report's.
    text = re.sub(r"<elapsetime>\d+</elapsetime>", "<elapsetime>0</elapsetime>", text)
    text = re.sub(r'time="[0-9.]+"', 'time="0"', text)
    text = re.sub(r'timestamp="[^"]*"', 'timestamp="0"', text)
    text = re.sub(r"<startTime>[^<]*</startTime>", "<startTime>0</startTime>", text)
    text = re.sub(r"<endTime>[^<]*</endTime>", "<endTime>0</endTime>", text)
    text = re.sub(r"<totalTime>\d+</totalTime>", "<totalTime>0</totalTime>", text)
    # A summary_info's own per-case line: "<case>:ok    227ms".
    text = re.sub(r"^(.*:(?:ok|nok))\s+\d+ms$", r"\1 <time>ms", text, flags=re.M)
    return text

# Two runs cannot be compared line for line: the order of the children in a
# summary_info, and of the cases in the JUnit report, is JDK 8 Hashtable
# iteration over paths that carry the run's timestamp, so it changes with the
# run (sql-native.md §1). That order is checked byte for byte elsewhere, against
# a single run's own verdicts; here what has to match is the content.
def lines(text):
    return sorted(l.rstrip() for l in text.splitlines() if l.strip())

def verdicts(root):
    xml = read(os.path.join(root, "summary.xml"))
    out = {}
    for m in re.finditer(r"<case>([^<]*)</case>.*?<result>([a-z]+)</result>", xml, re.S):
        out[m.group(1)] = m.group(2)
    return out

# 1. Every case's verdict.
a, b = verdicts(ctp), verdicts(native)
say(set(a) == set(b), "the same cases ran",
    f"CTP {len(a)}, sqlsuite {len(b)}, only in one: "
    f"{sorted(set(a) ^ set(b))[:5]}" if set(a) != set(b) else "")
moved = sorted(c for c in a if c in b and a[c] != b[c])
say(not moved, "every verdict is CTP's",
    f"{len(moved)} differ, first: " + ", ".join(f"{c} {a[c]}->{b[c]}" for c in moved[:5]))

# 2. main.info, line for line: it has no order of its own.
x, y = normalise(read(os.path.join(ctp, "main.info"))), normalise(read(os.path.join(native, "main.info")))
detail = ""
if x != y:
    xs, ys = x.splitlines(), y.splitlines()
    for i in range(max(len(xs), len(ys))):
        p = xs[i] if i < len(xs) else "<none>"
        q = ys[i] if i < len(ys) else "<none>"
        if p != q:
            detail = f"line {i+1}: CTP {p[:80]!r}, sqlsuite {q[:80]!r}"
            break
say(x == y, "main.info", detail)

# 3. Every summary_info, and the JUnit report.
def tree(root):
    out = {}
    for d, _, files in os.walk(root):
        for f in files:
            if f == "summary_info" or f.endswith(".xml") or f == "summary.info":
                out[os.path.relpath(os.path.join(d, f), root)] = os.path.join(d, f)
    return out

ta, tb = tree(ctp), tree(native)
say(set(ta) == set(tb), "the same record files",
    f"only in CTP: {sorted(set(ta) - set(tb))[:3]}, only in sqlsuite: {sorted(set(tb) - set(ta))[:3]}"
    if set(ta) != set(tb) else "")
differ = []
for p in sorted(set(ta) & set(tb)):
    x, y = lines(normalise(read(ta[p]))), lines(normalise(read(tb[p])))
    if x != y:
        only = [l for l in x if l not in y][:1] + [l for l in y if l not in x][:1]
        differ.append((p, only))
say(not differ, f"{len(set(ta) & set(tb))} record files hold the same lines",
    f"{len(differ)} differ, first: {differ[:2]}")

print(f"\n{'GATE: every comparison holds' if bad == 0 else f'GATE: {bad} comparisons differ'}")
sys.exit(0 if bad == 0 else 1)
