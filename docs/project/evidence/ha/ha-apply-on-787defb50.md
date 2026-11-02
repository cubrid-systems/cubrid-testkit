# `testkit ha-apply` on #8052 `787defb50`: every verb as it was on `b984637bb`

- **Date:** 2026-11-02
- **What this is:** #8052's head moved from `b984637bb` to `787defb50`. The two commits after
  `b984637bb` are `e89158de1`, which moves the writeset's key hashing into a file of its own, and
  `787defb50`, which keeps the MVCCID self-lock on `COMMIT WORK RETAIN LOCK` as develop does. Neither
  is a fix this tool has a pattern for, so the question is only whether every verb still gives what
  it gave on `b984637bb`.
- **Command:** `testkit ha-apply`, built from `main` at `98e3c51`: `pairs --build <it> --keep`, then
  on the pair it stood up `restart`, `restart --kill node` and `stress --ops 0,1,2,3,4,9 --seed 1`,
  one after the other. `ha_apply_worker_count=4`.
- **Pair:** `hap11021153`, `11.5.0.2991-787defb`, RelWithDebInfo, with a client node, on one host;
  removed afterwards.

---

| verb | result | ledger |
|---|---|---|
| `pairs` | 33 patterns, 32 **held** and the control first in 20 of 20 rounds; no missed, no inconclusive. The two VARCHAR trailing-space patterns were left out, as on any pair whose `ignore_trailing_space` is off | `haapply-c-787d-pairs.tsv` |
| `restart` | **held**: 46 kills, all with markers above the stored position, up to 2,077; both nodes 24,000 rows, the same key sum, no key unmoved; no failure, no -670 | `haapply-c-787d-applier.tsv` |
| `restart --kill node` | **held**: 5 crashes, all with markers, up to 2,081, 5 starts; the same as above | `haapply-c-787d-node.tsv` |
| `stress` | **same**: 466 statements the master refused, 744 write conflicts, one history clear; every table, heap count and the ballast the same, no orphan, no failure | `haapply-c-787d-stress.tsv` |

`b984637bb` gave the same on each: the whole catalog held, `fk_repoint` after a rerun of 50 rounds
([ha-apply-and-the-binary-nul](ha-apply-and-the-binary-nul.md)), `restart` held in both modes
([ha-apply-restart-and-the-applied-tx-marker](ha-apply-restart-and-the-applied-tx-marker.md),
[ha-apply-restart-and-a-crashed-slave](ha-apply-restart-and-a-crashed-slave.md)), and `stress` was
same.

Files, in the working directory: the four ledgers, and `haapply-c-787d-{applier,node,stress}.d`
beside them.

## Not here

- The trailing-space patterns on a pair with `ignore_trailing_space` on.
- What `787defb50` changed: `COMMIT WORK RETAIN LOCK` is not in any workload here.
