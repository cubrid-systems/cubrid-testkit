# `testkit ha-apply stress --set lock_escalation=5`: the slave stops on `cf8955a91`, not on `502685e64`

- **Date:** 2026-11-02
- **What this is:** #8052 `502685e64` ("delete the applied-tx markers last before the coordinator
  commits"). Before it, the coordinator deleted the markers up to `committed_lsa` and then stored
  the statistics rows. Deleting more markers than `lock_escalation` escalates to a table X lock held
  until the commit, and a worker inserting its marker waits for that lock holding
  `la_sql_compile_mutex`, which the coordinator then needs to store the statistics: neither goes on.
  The commit says replication stopped in an HA test with `lock_escalation=5` on the slave. No pair
  this tool stood up had that setting, and none could be given it; `--set` passes any parameter to
  csb's `--set`. The question: does `stress` call the slave stopped on `cf8955a91`, the parent of
  `502685e64`, and not on `502685e64` itself or on #8052's head, `787defb50`?
- **Command:** `testkit ha-apply stress --build <an install of cf8955a91, of 502685e64, of
  787defb50> --set lock_escalation=5 --ops 0,1,2,3,4,9 --seed 1`, built from this change.
  `ha_apply_worker_count=4`. Each pair was removed at the end of its run.
- **Pairs:** RelWithDebInfo, each with a client node, on one host.

---

## 1. The runs

| #8052 | pair | build | verdict | slave |
|---|---|---|---|---|
| `cf8955a91` (before `502685e64`) | `hap11026b97` | `11.5.0.2983-cf8955a` | **stalled** | did not show the final marker within 5 minutes; `s_kv` and the ballast differ, as a slave behind does |
| `502685e64` | `hap110210e2` | `11.5.0.2984-502685e` | **same** | caught up; every table, heap count and the ballast the same |
| `787defb50` | `hap11021149` | `11.5.0.2991-787defb` | **same** | the same |

Each ledger's first line says `lock_escalation=5`, read back from the slave's `cubrid.conf`. No run
moved `fail_counter`: the slave stopped, it did not fail. `cf8955a91` and `502685e64` differ by that
commit alone, so the stall goes with it: two runs stalled on `cf8955a91` (the `--set` run and the
hand-made `lkecf89` below) and one ended the same on `502685e64`. Files, in the working directory:
`haapply-a-cf89-set.tsv`, `haapply-a-5026-set.tsv`, `haapply-a-787d-set.tsv`, and each run's
`a-*-set.out` and `a-*-set.err`.

Before `--set` existed, two pairs stood up by hand (`csb cluster create ... --set
lock_escalation=5`) and run with `--cluster` gave the same verdicts: `lkecf89` on `cf8955a91`
stalled (`haapply-a-cf89-stress.tsv`), `lke787d` on `787defb50` same (`haapply-a-787d-stress.tsv`).

## 2. Why the slave stopped

`cubrid lockdb` on `lkecf89`'s slave, after its run had ended and the slave had stayed where it
stopped (`a-cf89-lockdb.txt`):

```
OID =  0|   205|   1
Object type: Class = dba.ha_applied_tx_marker.
...
LOCK HOLDERS:
    Tran_index =   2, Granted_mode =     X_LOCK, Count =   5, Nsubgranules =  0
BLOCKED LOCK HOLDERS:
    Tran_index =   6, Granted_mode = SCH_S_LOCK, Count =   2, Nsubgranules =  0
	              Blocked_mode =    IX_LOCK
	              Start_waiting_at = Mon Nov  2 03:29:15 2026
	              Wait_for_secs = -1.00
```

The worker had waited since about six minutes before the dump (03:29 UTC, the dump at 03:35), longer
than the run's 5-minute wait for its final marker.

Transaction 2 is `applylogdb`, the coordinator, holding the marker table in X, as an escalation of
its marker delete would; transaction 6 is `applylogdb(4)`, a worker, waiting for IX on that table
with no timeout, to insert its marker. That the worker holds `la_sql_compile_mutex`
and the coordinator waits for it is `502685e64`'s account; a mutex is not in the lock table, and
nothing here read the threads.

## 3. Not here

- **`9343b73f7`**, a marker read that fails partway through when the applier starts: nothing here
  makes one.
- **`b984637bb`'s own case**, a statistics statement chosen as a deadlock victim: nothing here makes
  one either.
- `lock_escalation` on the master too: csb's `--set` writes every node's `cubrid.conf`, and the
  master took the workload with it; whether it changes the master's side was not looked at.
