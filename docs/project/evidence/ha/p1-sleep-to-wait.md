# P1, measured: the wait costs 1.3 seconds where the sleep cost 5 to 30

- **Date:** 2026-09-21
- **What this is:** `design/module-ha.md` P1 says a case should wait on state rather than on the
  clock, and that doing so changes no verdict. Four cases, run twice on a real pair — once as the
  corpus writes them, once with seven `sleep` lines replaced by the corpus's own `wait_for_slave`.
- **Trees:** engine CUBRID 11.5.0 (`11.5.0.2513-5f3a30d`) on both nodes · cases
  `CUBRID/cubrid-testcases-private` `HA/shell` · CTP `cubrid-testtools` as checked out here.
- **Pair:** master `hgryoo-desktop`, slave `hgryoo-notebook`, over tailscale. Runner: **CTP**, not
  testkit — the frozen corpus is CTP's to run (ADR-022), and this measures the corpus rather than a
  runner.

---

## 1. The result

| case | baseline | with waits | Δ | sleep removed |
|---|---|---|---:|---:|
| `_22_ha/bug_xdbms2707` | **OK** 96.6 s | **OK** 61.6 s | **−35.0 s** | 30 + 5 |
| `_22_ha/bug_xdbms3769` | **OK** 75.2 s | **OK** 62.8 s | **−12.4 s** | 5 + 5 + 5 |
| `_22_ha/bug_xdbms3700` | **NOK** 48.5 s | **NOK** 50.8 s | +2.4 s | none reached (§4) |
| `_12_bts_issue/bug_bts_12550` | **OK** 147.9 s | **OK** 120.8 s | **−27.1 s** | 30 |
| **whole run** | 3 OK / 1 NOK · **389 s** | 3 OK / 1 NOK · **316 s** | **−73 s (−18.8 %)** | |

**Every verdict is the same one.** The same three cases pass and the same case fails, in the same
dispatch order. That is the whole of what P1 claims, and it held.

**The arithmetic closes.** 80 seconds of `sleep` actually executed — 3700 never reaches its 30, §4 —
and 72.1 seconds came back. So **six `wait_for_slave` calls cost 7.9 seconds, about 1.3 seconds
each**, against the 5 to 30 they replaced. The per-case deltas sum to −72.1 s against a run-level
−73 s, so the time that went missing is the sleep and not something else.

## 2. What was changed, and the rule

One rule, applied mechanically: **a `sleep N` whose next slave read follows becomes
`wait_for_slave`.** Seven lines, in four files, and nothing else:

```
-sleep 30          -sleep 5           -sleep 30          -sleep 30
+wait_for_slave    +wait_for_slave    +wait_for_slave    +wait_for_slave
```

`wait_for_slave` is not something this project wrote. It is in the corpus's own helper library
(`make_ha_upper.sh:74`): it creates a table on the master, inserts the row
`'replication finished'`, and polls the slave with `-tillcontains` until it arrives. **136 of the
373 cases already call it. 244 sleep instead. 91 do both.** P1 is convergence, not invention.

None of the four cases lists the catalog, so the marker table is invisible to every assertion they
make — checked before the run rather than after.

## 3. What was deliberately not changed

Three sleeps were left in, and the reason is that they answer a different question:

| left alone | what it waits for |
|---|---|
| `bug_xdbms3700:42` — `sleep 40` | a `cub_admin` **process** to appear in `ps`. Not replication |
| `bug_bts_12550:6` — `sleep 30` | nothing. It sits immediately after `setup_ha_environment`, which already polls `changemode` until active |
| `bug_bts_12550:18` — `sleep 30` | nothing. It sits before a local `sed` on files already written |

Deleting a sleep that guards nothing is a **stronger and different claim** than replacing a sleep
with a wait. Mixing them would have made this measurement answer neither question, so the
conservative 110 seconds were targeted and 60 were left on the table. The saving above is therefore
a floor.

`bug_bts_12550` is the case worth reading. It calls `wait_for_slave` at line 9 — the right helper,
already there — and sleeps 30 seconds on either side of it for nothing.

## 4. The failure is the environment's, and it is honest

`bug_xdbms3700` is NOK in both runs, and not because of the engine. Its first check is

```sh
slave_cmd "ulimit -c" > console.log
cnt=`cat console.log | grep -v grep | grep unlimited | wc -l`
if [ $cnt -eq 1 ]; then write_ok; else write_nok; finish; exit; fi
```

The slave's soft core limit is 0, so the case writes NOK and **exits at line 19** — thirteen lines
above the `sleep 30` this experiment would have replaced. It contributes a verdict to the comparison
and nothing to the timing, and it is left in the sample rather than swapped out, because choosing
the sample after seeing the results is how a measurement stops meaning anything.

It is also a requirement `preflight.sh` did not know about. The slave's hard limit is `unlimited`,
so the soft one can be raised — but a CUBRID core here is 10 to 20 GB, and turning it on for a
laptop is a decision rather than a fix.

## 5. What this does not show

- **Four cases are four cases.** This is evidence that P1 holds, not a number for the corpus. The
  corpus-wide figure — 20,370 seconds of `sleep` across 373 cases — is still a static count.
- **Each run happened once.** Which cases move on their own is unmeasured, so *"the verdicts are the
  same"* is a statement about these two runs. ADR-018 measured CTP moving seven verdicts between two
  identical isolation runs; nobody has asked that of HA yet, and `evidence/ha/README.md` says why the
  full baseline is two runs rather than one.
- **The baseline's artifacts were lost.** The result directory was removed before the second run
  instead of copied first; the verdicts, per-case times and totals survive in `results/baseline.txt`
  and in `baseline.log`, and the second run's directory was kept. A full baseline needs the copy to
  come first, the way the shell comparison harness does it per shard.
