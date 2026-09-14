#!/usr/bin/env python3
"""How much of the sql/medium answer corpus is shaped by Java's toString.

Counts answer FILES (not lines) that contain each formatting class. A file can
be in several classes. Prints a table per suite, and the variant-suffix census.
"""
import os
import re
import sys
from collections import Counter

ROOT = sys.argv[1]

CLASSES = [
    # Java Double/Float.toString switches to E notation outside [1e-3, 1e7).
    ("double_E", re.compile(r"(?<![\w.])-?\d\.\d+E-?\d+(?![\w.])")),
    # BigDecimal.toString uses E notation for small scales: 1E-7, 0E-10.
    ("bigdecimal_E", re.compile(r"(?<![\w.])-?\d+E[-+]\d+(?![\w.])")),
    # java.sql.Timestamp.toString: fraction always present, trailing zeros cut.
    ("timestamp_frac", re.compile(r"\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{1,9}(?!\d)")),
    # Region or offset after a time: the driver's timezone types.
    ("tz_value", re.compile(r"\d{2}:\d{2}:\d{2}(\.\d+)?( [AP]M)? +([A-Z][a-z]+/[A-Za-z_]+|[+-]\d{2}:\d{2}|UTC|GMT)")),
    # Query plan text (masked digits appear as '?').
    ("query_plan", re.compile(r"^(Query plan:|Join graph|Query stmt:)", re.M)),
    # Error lines as ConsoleDAO writes them.
    ("error_any", re.compile(r"^Error:-?\d+", re.M)),
    # JDBC-driver-side errors (-21xxx) and the catch-all -10000.
    ("error_driver", re.compile(r"^Error:-(21\d{3}|10000)\b", re.M)),
    # Floats printed with a plain fraction (shortest-repr sensitive).
    ("float_plain", re.compile(r"(?<![\w.:-])-?\d+\.\d{6,}(?![\w.])")),
]

def census(suite_dir):
    files = 0
    hits = Counter()
    variants = Counter()
    for dirpath, _, names in os.walk(suite_dir):
        if os.path.basename(dirpath) != "answers":
            continue
        for n in names:
            m = re.match(r".*\.answer(_.*)?$", n)
            if not m:
                continue
            suffix = m.group(1) or ""
            variants[re.sub(r"\d+", "N", suffix) or "(base)"] += 1
            if suffix:
                continue  # classes are counted over base answers only
            files += 1
            with open(os.path.join(dirpath, n), encoding="utf-8", errors="replace") as f:
                text = f.read()
            found = {name for name, rx in CLASSES if rx.search(text)}
            for name in found:
                hits[name] += 1
            if found & HARD:
                hits["(hard, any)"] += 1
    return files, hits, variants

# The classes that are hard to reproduce outside Java. Timestamps are not in the
# set: their rule is simple. Errors in general are not either: the server's codes
# are the same whatever the driver.
HARD = {"double_E", "bigdecimal_E", "float_plain", "tz_value", "query_plan", "error_driver"}

for suite in ("sql", "medium"):
    d = os.path.join(ROOT, suite)
    files, hits, variants = census(d)
    print(f"== {suite}: {files} base answer files")
    for name in [n for n, _ in CLASSES] + ["(hard, any)"]:
        print(f"  {name:16s} {hits[name]:6d}  {100.0*hits[name]/max(files,1):5.1f}%")
    print("  variants:", ", ".join(f"{k}={v}" for k, v in sorted(variants.items(), key=lambda kv: -kv[1])[:12]))
