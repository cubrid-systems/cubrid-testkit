# `testkit ha-apply` and the keys #8052 `eb31453ff` reduces: missed on `3fb54e549`, held on `fb1636c34`

- **Date:** 2026-11-01
- **What this is:** the reduced-key completion condition of ADR-EXT-012, added after #8052
  `eb31453ff` ("master: hash a key the way its index compares it"). Seven patterns of keys the index
  holds equal while `3fb54e549` hashed them apart must be missed on `3fb54e549` and held on
  `fb1636c34`, #8052's head on 2026-11-01, which contains `eb31453ff`. Also here: the whole catalog,
  the trailing-space patterns and a `stress` run on `fb1636c34`.
- **Command:** `testkit ha-apply pairs` and `stress`, built from this change's catalog on the code of
  [ha-apply-and-the-trailing-space](ha-apply-and-the-trailing-space.md); its compiled code is a
  build of this change's tree but for the VCS stamp. 20 rounds and 20,000 ballast rows a pattern,
  `ha_apply_worker_count=4`, the history at its default.
- **Pairs:** RelWithDebInfo, each with a client node, on one host. The two runs of §3 ran at the same
  time.

| pair | #8052 | how it was stood up |
|---|---|---|
| `hap1030f1c2` | `3fb54e549` (`11.5.0.2967-3fb54e5`) | by the command's `--build`, kept; removed after these runs |
| `hap11016471` | `fb1636c34` (`11.5.0.2985-fb1636c`) | by the command's `--build`, kept |

---

## 1. What the index holds equal

`eb31453ff` reduces a value the way the index compares it before hashing it, in a single key, a
composite key element and a reference: a MONETARY amount in one fixed currency, a VARBIT with its
bit length in whole bytes, and a string with each NUL as a space. A DOUBLE NaN, which the index
finds equal to every key, cannot be hashed that way, and its transaction falls back to commit
order.

Before writing patterns, each kind was tried on `hap1030f1c2`'s master (`3fb54e549`, the parameter
`ignore_trailing_space` at its default), in a table with a UNIQUE column:

| kind | first row | second row | the second insert |
|---|---|---|---|
| MONETARY | `$100` | `\100` | refused, key `\KRW100.00` |
| VARBIT | `B'1'` (1 bit) | `B'10000000'` (8 bits) | refused, key `X'80'` |
| NUL | `'a' \|\| CHR(0) \|\| 'b'` | `'a b'` | refused, key `'a b'` |
| NaN | `CAST('NaN' AS DOUBLE)` | `5.0` | refused, key `5.000000e+00` |
| NaN | `5.0` | NaN | refused, key `nan` |
| NaN | NaN | NaN | refused, key `nan` |
| NaN | `3.0` and `7.0` | NaN | refused, key `nan` |

The first row was kept each time; `HEX` of the NUL row read `610062`, so `CHR(0)` wrote a NUL. The
foreign-key forms were taken too: a child `\3` under a parent `$3`, a child of 24 bits under a
parent of 17 with the same bytes, and a child `'Key 3'` under a parent `'Key' || CHR(0) || '3'`; a
child `'Key 4'` with no parent was refused, so the key was checked.

## 2. The seven patterns

In round `r`, as `vc_trail` does: before the round a row is committed, T1 deletes it, and T2 inserts
the value the index holds equal to it; or, for a foreign key, T1 inserts the parent and T2 a child
that names it the other way.

| pattern | column | T1's value | T2's value |
|---|---|---|---|
| `mon_cur` | MONETARY UNIQUE | `$<r>` | `\<r>` |
| `vb_len` | BIT VARYING(32) UNIQUE | `B'<r in 16 bits>1'`, 17 bits | the same with seven zeros, 24 bits |
| `str_nul` | VARCHAR(20) UNIQUE | `'Key' \|\| CHR(0) \|\| '<r>'` | `'Key <r>'` |
| `dbl_nan` | DOUBLE UNIQUE | NaN | `<r>` |
| `fk_mon_cur` | MONETARY FK | parent `$<r>` | child `\<r>` |
| `fk_vb_len` | BIT VARYING(32) FK | parent, 17 bits | child, 24 bits |
| `fk_str_nul` | VARCHAR(20) FK | parent with the NUL | child with the space |

A NaN is one key with every other, so `dbl_nan`'s table holds one row at a time: its committed step
deletes the table's rows before it inserts the NaN. None of the seven needs `ignore_trailing_space`.
The catalog is 33.

## 3. Both heads

```
# cluster=hap1030f1c2 build=11.5.0.2967-3fb54e5 ha_apply_worker_count=4 ha_writeset_history_size=unset ignore_trailing_space=no rounds=20 rows=20000
pattern	rounds	t2_ahead	t2_never_seen	refused	final	fail_counter_delta	conflict_write	conflict_ref	fallback	clear	verdict
ctl	20	20	0	0	same	0	0	0	0	0	control-overtook
mon_cur	20	18	0	0	differ:t_mon_cur	18	0	0	0	0	missed
vb_len	20	18	0	0	differ:t_vb_len	18	0	0	0	0	missed
str_nul	20	18	0	0	differ:t_str_nul	18	0	0	0	0	missed
dbl_nan	20	18	0	0	differ:t_dbl_nan	31	0	0	0	0	missed
fk_mon_cur	20	17	0	0	differ:child_fk_mon_cur	19	0	0	0	0	missed
fk_vb_len	20	13	0	0	differ:child_fk_vb_len	18	0	0	0	0	missed
fk_str_nul	20	15	0	0	differ:child_fk_str_nul	17	0	0	0	0	missed
```

```
# cluster=hap11016471 build=11.5.0.2985-fb1636c ha_apply_worker_count=4 ha_writeset_history_size=unset ignore_trailing_space=no rounds=20 rows=20000
pattern	rounds	t2_ahead	t2_never_seen	refused	final	fail_counter_delta	conflict_write	conflict_ref	fallback	clear	verdict
ctl	20	20	0	0	same	0	0	0	0	0	control-overtook
mon_cur	20	0	0	0	same	0	20	0	0	0	held
vb_len	20	0	0	0	same	0	20	0	0	0	held
str_nul	20	0	0	0	same	0	20	0	0	0	held
dbl_nan	20	0	0	0	same	0	0	0	20	20	held
fk_mon_cur	20	0	0	0	same	0	20	0	0	0	held
fk_vb_len	20	0	0	0	same	0	20	0	0	0	held
fk_str_nul	20	0	0	0	same	0	20	0	0	0	held
```

On `3fb54e549` every one of the seven is missed three ways at once: T2 seen first in 13 to 18 rounds
of 20, the apply failing, and the table differing at the end. The writeset counted no conflict of
any kind in any of their rounds. On `fb1636c34` none let T2 ahead: the six with a reduced value
counted a write conflict in every round, and `dbl_nan` a fallback with a history clear in every
round, as `ddl_dml` does. The control went first in 20 of 20 on both.

What the failed applies were was not read. `dbl_nan`'s `fail_counter` moved 31 times in 20 rounds,
more than T2 was seen ahead. A reading, not a measurement: a missed round leaves the slave without
T2's row, and the next round's committed step deletes that row on the master, a delete the slave
has nothing to apply to. As in the trailing-space document,
a round counts as ahead only when the first look at the slave that finds the witness finds none of
T1's ballast, so the ahead counts are lower bounds.

Of the commits in `3fb54e549..fb1636c34`, only `eb31453ff` touches `src/transaction/writeset/`. The
others are `501e5f315` (when flush-marked replication records are written), `f470fd0af` and
`fb1636c34` (the slave's applied-transaction markers), and a merge of develop. No run isolates
`eb31453ff`; what points to it is the master's counters, none on `3fb54e549` and one a round on
`fb1636c34`.

## 4. The rest of `fb1636c34`

On `hap11016471`, after §3:

| run | result |
|---|---|
| `pairs`, the whole catalog | 31 patterns with the control, all held, the control ahead in 20 of 20; the pair keeps trailing spaces, so `vc_trail` and `fk_vc_trail` were left out and the run said so |
| `stress --ops 0,1,2,3,4,9 --seed 1` | **same**: 468 master errors, 737 write conflicts, one history clear, `fail_counter` unmoved |
| `pairs vc_trail fk_vc_trail`, after `ignore_trailing_space=yes` in each node's `cubrid.conf` and a restart | both held with 20 write conflicts, the control ahead in 20 of 20, the pair answering yes |

No round in the whole-catalog run had a late witness, no table differed and `fail_counter` did not
move. So on `fb1636c34` all 32 patterns besides the control held, on one pair, and the control
overtook in every run.

## 5. Not here

- **`stress` with the foreign-key operations on `fb1636c34`.** On `3fb54e549` and `61ee0944a` those
  runs diverged, by reading through CBRD-27464, which #8026 (open) fixes and #8052 does not; nothing in
  `fb1636c34` suggests otherwise, and it was not run.
- **A composite key element.** `eb31453ff` reduces values in composite keys too; every pattern here
  is a single-column key or a single-column reference.

## 6. Reproducing it

```bash
export CSB_HOME=/data/...  TESTKIT_CSB=.../extensions/cluster-sandbox/bin/csb
P="mon_cur vb_len str_nul dbl_nan fk_mon_cur fk_vb_len fk_str_nul"
testkit ha-apply pairs --build <an install of #8052 3fb54e549> $P    # all seven missed
testkit ha-apply pairs --build <an install of #8052 fb1636c34> $P    # all seven held
```

The second line is what ran. The first ran as `--cluster` on a pair an earlier `--build` of
`3fb54e549` had stood up.
