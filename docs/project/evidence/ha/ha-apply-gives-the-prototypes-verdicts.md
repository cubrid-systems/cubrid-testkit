# `testkit ha-apply` gives the shell prototype's verdicts on both heads of #8052

- **Date:** 2026-10-30
- **What this is:** the completion condition of ADR-EXT-012's M1. On #8052's old head `bf560620e`
  the six patterns whose keys the index holds equal while the writeset saw them as different must
  be missed, and on the new head `61ee0944a` every pattern must hold while the control overtakes:
  the verdicts the shell prototype gave in
  [parallel-apply-keeps-conflict-order](parallel-apply-keeps-conflict-order.md).
- **Command:** `testkit ha-apply pairs`, built from the tree of the commit that adds this file
  (only comments, tests and documents changed after the build). 20 rounds and 20,000 ballast rows a pattern
  unless said otherwise; `ha_apply_worker_count=4`, the history at its default.
- **Pairs:** old head, a pair the command stood up with `--build` and removed after; new head,
  `wsn4` with its client node. Both RelWithDebInfo.

---

## 1. The old head: the six missed

```
# cluster=hap1030798a build=11.5.0.2939-bf56062 ha_apply_worker_count=4 ha_writeset_history_size=unset rounds=20 rows=20000
pattern	rounds	t2_ahead	t2_never_seen	refused	final	fail_counter_delta	conflict_write	conflict_ref	fallback	clear	verdict
ctl	20	20	0	0	same	0	0	0	0	0	control-overtook
fp_negzero	20	19	0	0	differ:t_fp_negzero	19	0	0	0	0	missed
tstz_zone	20	16	0	0	differ:t_tstz_zone	18	0	0	0	0	missed
dttz_zone	20	18	0	0	differ:t_dttz_zone	18	0	0	0	0	missed
ci_case	20	0	0	0	same	0	20	0	0	0	held
inherit_uk	20	19	0	0	differ:t_inherit_uk	19	0	0	0	0	missed
fk_negzero	20	14	0	0	differ:child_fk_negzero	20	0	0	0	0	missed
fk_tz	20	18	0	0	differ:child_fk_tz	20	0	0	0	0	missed
fk_ci	20	0	1	0	same	0	20	0	0	0	inconclusive
```

The six are missed three ways at once: T2 ahead in 14 to 19 rounds, the apply failing 18 to 20
times, and the tables differing at the end. The writeset counted no conflict in any of their
rounds. `ci_case` and `fk_ci`, whose keys the old head already hashed alike, counted one in every
round and never let T2 ahead. `fk_ci` is inconclusive for one round whose witness took longer than
30 seconds (§3).

An earlier run of the same command, before the last review's fixes (which changed only what a
stalled or unreadable slave does and when a ledger is removed after an early failure, and these
runs reached none of it), gave the same six missed and both of the others held, with the control
ahead in 19 of 20.

## 2. The new head: none missed

```
# cluster=wsn4 build=11.5.0.2964-61ee094 ha_apply_worker_count=4 ha_writeset_history_size=unset rounds=20 rows=20000
pattern	rounds	t2_ahead	t2_never_seen	refused	final	fail_counter_delta	conflict_write	conflict_ref	fallback	clear	verdict
ctl	20	14	0	0	same	0	0	0	0	0	control-overtook
fp_negzero	20	0	0	0	same	0	20	0	0	0	held
tstz_zone	20	0	0	0	same	0	20	0	0	0	held
dttz_zone	20	0	0	0	same	0	20	0	0	0	held
ci_case	20	0	0	0	same	0	20	0	0	0	held
inherit_uk	20	0	0	0	same	0	20	0	0	0	held
fk_negzero	20	0	0	0	same	0	20	0	0	0	held
fk_tz	20	0	0	0	same	0	20	0	0	0	held
fk_ci	20	0	1	0	same	0	20	0	0	0	inconclusive
pk_update	20	0	0	0	same	0	20	0	0	0	held
pk_move	20	0	1	0	same	0	20	0	0	0	inconclusive
uk_reinsert	20	0	0	0	same	0	20	0	0	0	held
uk_move	20	0	0	0	same	0	20	0	0	0	held
fk_insert	20	0	0	0	same	0	20	0	0	0	held
fk_delete	20	0	0	0	same	0	0	20	0	0	held
rk_reinsert	20	0	0	0	same	0	20	0	0	0	held
fk_composite	20	0	0	0	same	0	20	0	0	0	held
fk_cascade	20	0	0	0	same	0	0	20	0	0	held
sp_keep	20	0	0	0	same	0	20	0	0	0	held
sp_redo	20	0	0	0	same	0	20	0	0	0	held
ddl_dml	20	0	0	0	same	0	0	0	20	20	held
part_move	20	0	0	0	same	0	20	0	0	0	held
fk_repoint	20	0	0	0	same	0	0	20	0	0	held
fk_cascade_reuse	20	0	0	0	same	0	20	0	0	0	held
```

No pattern but the control let T2 ahead, no table differed and `fail_counter` did not move. The
counters say which path held each one: a reference conflict in the three where T2 deletes a parent
that T1's child names (`fk_delete`, `fk_cascade`, `fk_repoint`), the commit-order fallback with a
history clear for `ddl_dml`, and a write conflict in every other, the foreign-key inserts
included.

`fk_ci` and `pk_move` are inconclusive for one round each whose witness took longer than 30
seconds. Run again for 50 rounds each, with nothing else of this change running on the host, both
held and no witness was late:

```
# cluster=wsn4 build=11.5.0.2964-61ee094 ha_apply_worker_count=4 ha_writeset_history_size=unset rounds=50 rows=20000
pattern	rounds	t2_ahead	t2_never_seen	refused	final	fail_counter_delta	conflict_write	conflict_ref	fallback	clear	verdict
ctl	50	42	0	0	same	0	0	0	0	1	control-overtook
fk_ci	50	0	0	0	same	0	49	0	0	1	held
pk_move	50	0	0	0	same	0	50	0	0	0	held
```

The earlier run of the command before the last fixes held all 23 as well, with the control ahead
in 19 of 20 (and 16 of 20 in the sitting that finished its last three patterns after an
interruption).

## 3. What else showed

- **Three late witnesses in 660 rounds** (the two runs above), none in the 680 rounds of the
  earlier runs or the 150 of the re-run. In each of the three the writeset counted the conflict,
  so the dependency was found and T2 waited for T1; the round is inconclusive and not missed, as
  ADR-EXT-012 §3-3 has it, because a slow slave looks the same. Each late round took over 30
  seconds, and none reached the stall bound. The cause is not established. The two runs shared the
  host, and for part of the new head's run so did a CPU stress (32 busy loops 17:15:48–17:17:50,
  64 loops 17:18:16–17:19:28, a repeat of this change's timing tests), which by the run's average
  pace covered its rounds 50 to 100, `tstz_zone` to `ci_case`. By the same estimate all three
  late rounds came after it: the new head's `fk_ci` at about 17:24, its `pk_move` at about 17:27,
  and the old head's `fk_ci` in its last 20 rounds before 17:31:15.
- **The control** overtook in 14 to 20 rounds of 20 across the runs and 42 of 50, so every held
  verdict came from a pair that reorders.
- **The history filled at 50 rounds.** 50 rounds of 20,000 ballast keys is the default capacity of
  1,000,000. `ctl` and `fk_ci` write no DDL between rounds and each cleared once; `fk_ci` counted
  49 write conflicts in 50 rounds, and none of its 50 let T2 ahead. `pk_move` waits behind a
  `CREATE TABLE` barrier after every round's pre-write, and never filled; the ledger does not
  count the barrier, so that it clears the history rests on
  [parallel-apply-keeps-conflict-order](parallel-apply-keeps-conflict-order.md) and
  [parallel-apply-under-pressure](parallel-apply-under-pressure.md), where a history cleared by
  capacity was also asked on purpose.
- **Time.** Only averages are recorded, from each ledger's first and last write: on the new head
  a round, with its share of the setup and the final compare, averaged 2.3 to 5.5 seconds a run
  (4.5 over the 480 rounds of §2); on the old head, with standing the pair up included, 6.4 and
  11.7. The slowest round was not recorded. The command calls a slave stopped when it has not
  applied a round's T1 in two minutes for every 20,000 rows of ballast.

## 4. Reproducing it

```bash
export CSB_HOME=/data/...  TESTKIT_CSB=.../extensions/cluster-sandbox/bin/csb
testkit ha-apply pairs --build <#8052 bf560620e install> --workers 4 \
    fp_negzero tstz_zone dttz_zone ci_case inherit_uk fk_negzero fk_tz fk_ci
testkit ha-apply pairs --cluster <a pair of #8052 61ee0944a with a client node c1>
```

The first stands a pair up and takes it down after; the second needs `ha_apply_worker_count` set
on the pair (`csb cluster create --preset ha --clients 1 --set-hidden ha_apply_worker_count=4`).
Each writes its ledger beside it and one line a pattern on standard output.
