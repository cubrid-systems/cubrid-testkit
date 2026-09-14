#!/usr/bin/env python3
"""Drive TestkitExecutor over a case list and write each case's .result, the way
CQT's saveTempResults does: <caseDir>/<name>.result, UTF-8, the text verbatim.

    driver.py <order-file> -- <java command...>

<order-file> is a CTP run's output; the cases are taken in the order its
progress lines name them, so the executor sees them in CQT's order.
"""
import re
import subprocess
import sys
import time

order_file = sys.argv[1]
cmd = sys.argv[sys.argv.index("--") + 1:]

rx = re.compile(r"Testing (\S+\.sql) \(\d+/\d+ [\d.]+%\)")
cases = [m.group(1) for m in rx.finditer(open(order_file, errors="replace").read())]

p = subprocess.Popen(cmd, stdin=subprocess.PIPE, stdout=subprocess.PIPE)
ready = p.stdout.readline().decode()
print("executor:", ready.strip(), "cases to send:", len(cases), flush=True)
if not ready.startswith("READY"):
    sys.exit(2)

t0 = time.time()
errors = 0
for i, case in enumerate(cases, 1):
    p.stdin.write((case + "\n").encode())
    p.stdin.flush()
    head = p.stdout.readline().decode()
    if head.startswith("R "):
        body = p.stdout.read(int(head[2:]))
        with open(case[:-len(".sql")] + ".result", "wb") as f:
            f.write(body)
    else:
        errors += 1
        print(f"{case}: {head.strip()}", flush=True)
    if i % 1000 == 0:
        print(f"{i}/{len(cases)} {time.time() - t0:.0f}s", flush=True)
p.stdin.close()
p.wait()
print(f"done: {len(cases)} cases, {errors} errors, {time.time() - t0:.1f}s", flush=True)
