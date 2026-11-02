# `testkit ha-apply` and the binary collation's trailing NUL: missed on `fb1636c34`, held on `b984637bb`

- **Date:** 2026-11-01
- **What this is:** #8052's head moved from `fb1636c34` to `b984637bb`, #8052's head on 2026-11-01.
  Of the commits between them, only `cf8955a91` touches `src/transaction/writeset/`: it is
  `eb31453ff` again, except that a string under the binary collation is now hashed with its trailing
  NULs cut, where `eb31453ff` read each NUL as a space. The binary collation ignores a trailing NUL
  and nothing else, so `eb31453ff` hashed `'Key<r>'` and the same with a NUL after it apart while
  the index holds them one key. Two patterns of that key must be missed on `fb1636c34` and held on
  `b984637bb`. Also here: the whole catalog, the trailing-space patterns and a `stress` run on
  `b984637bb`, whose merge of develop brought #7881 into the engine both nodes run.
- **Command:** `testkit ha-apply pairs` and `stress`, built from this change's catalog on the code of
  [ha-apply-and-the-reduced-keys](ha-apply-and-the-reduced-keys.md); its compiled code is a build of
  this change's tree but for the VCS stamp. 20 rounds and 20,000 ballast rows a pattern,
  `ha_apply_worker_count=4`, the history at its default.
- **Pairs:** RelWithDebInfo, each with a client node, on one host. Both runs on `hap11016471` ran
  while the whole catalog was running on `hap11010a6b`.

| pair | #8052 | how it was stood up |
|---|---|---|
| `hap11016471` | `fb1636c34` (`11.5.0.2985-fb1636c`) | by the command's `--build`, kept from [ha-apply-and-the-reduced-keys](ha-apply-and-the-reduced-keys.md) |
| `hap11010a6b` | `b984637bb` (`11.5.0.2989-b984637`) | by the command's `--build`, kept |

---

## 1. What the index holds equal

`lang_fastcmp_binary` compares two strings byte by byte and, past the shorter one, finds them
unequal at any byte that is not 0. A trailing NUL is ignored, a trailing space is not, and
`ignore_trailing_space` is not read. Rows were inserted on `hap11016471`'s master, each after
`'ab'`, into a table with `s VARCHAR(20) COLLATE binary UNIQUE` and one with
`s CHAR(8) COLLATE binary UNIQUE`, twice with the same results: before the patterns were written,
with the parameter on, left in the nodes' `cubrid.conf` by the reduced-keys document's trailing-space
run (the three refusals are in the master's server log); and after the runs on `hap11016471` in §3,
with it taken out and the pair restarted (that output was saved with the ledgers):

| column | row | the insert | `HEX (s)` of the row taken |
|---|---|---|---|
| VARCHAR | `'ab' \|\| CHR(0)` | refused (-670) | |
| VARCHAR | `'ab '` | taken | `616220` |
| VARCHAR | `'ab' \|\| CHR(0) \|\| ' '` | taken | `61620020` |
| VARCHAR | `'a' \|\| CHR(0) \|\| 'b'` | taken | `610062` |
| VARCHAR | `'a b'`, after the row above | taken | `612062` |
| CHAR(8) | `'ab' \|\| CHR(0)` | refused (-670) | |
| CHAR(8) | `'ab' \|\| CHR(0) \|\| CHR(0)` | refused (-670) | |

With the parameter on, `'ab '` was still a key of its own: the binary compare does not drop a
trailing space either way. Unlike `str_nul`'s collation, the binary one does not take a NUL inside a
string for a space: `'a' || CHR(0) || 'b'` and `'a b'` are two keys, and `eb31453ff`, by reading
its code, hashed them alike, which only merges two keys. The trailing NUL is the key it split.

A CHAR is not a pattern. The binary charset pads a CHAR with NULs (`intl_pad_char`), so in a
CHAR(8) `'ab'` and `'ab' || CHR(0)` are the same bytes, and by reading the code (not run) both
heads hash them alike.

## 2. The two patterns

As `str_nul` and `fk_str_nul` do:

| pattern | column | T1's value | T2's value |
|---|---|---|---|
| `bin_nul` | VARCHAR(20) COLLATE binary UNIQUE | deletes `'Key<r>' \|\| CHR(0)` | inserts `'Key<r>'` |
| `fk_bin_nul` | VARCHAR(20) COLLATE binary FK | parent `'Key<r>' \|\| CHR(0)` | child `'Key<r>'` |

On a pair with `ignore_trailing_space` on, `eb31453ff` cut trailing spaces from a VARCHAR, and with
them the space it had made of the NUL, so there it hashed the two values alike (§3). The catalog
is 35.

## 3. Both heads

```
# cluster=hap11016471 build=11.5.0.2985-fb1636c ha_apply_worker_count=4 ha_writeset_history_size=unset ignore_trailing_space=no rounds=20 rows=20000
pattern	rounds	t2_ahead	t2_never_seen	refused	final	fail_counter_delta	conflict_write	conflict_ref	fallback	clear	verdict
ctl	20	20	0	0	same	0	0	0	0	0	control-overtook
bin_nul	20	20	0	0	differ:t_bin_nul	20	0	0	0	0	missed
fk_bin_nul	20	17	0	0	differ:child_fk_bin_nul	20	2	0	0	0	missed
```

```
# cluster=hap11010a6b build=11.5.0.2989-b984637 ha_apply_worker_count=4 ha_writeset_history_size=unset ignore_trailing_space=no rounds=20 rows=20000
pattern	rounds	t2_ahead	t2_never_seen	refused	final	fail_counter_delta	conflict_write	conflict_ref	fallback	clear	verdict
ctl	20	19	0	0	same	0	0	0	0	0	control-overtook
bin_nul	20	0	0	0	same	0	20	0	0	0	held
fk_bin_nul	20	0	0	0	same	0	20	0	0	0	held
```

The second is two rows of the whole-catalog run of §4, with its control. On `fb1636c34`, `bin_nul`
let T2 ahead in all 20 rounds and `fk_bin_nul` in 17, each with the apply failing 20 times and the
table differing at the end. `fk_bin_nul` counted 2 write conflicts in 20 rounds; which writes they
were was not read. On `b984637bb` neither let T2 ahead, and each counted a write conflict in every
round.

The same two patterns ran on `hap11016471` first while its nodes still had
`ignore_trailing_space=yes` from the trailing-space run of the reduced-keys document:

```
# cluster=hap11016471 build=11.5.0.2985-fb1636c ha_apply_worker_count=4 ha_writeset_history_size=unset ignore_trailing_space=yes rounds=20 rows=20000
pattern	rounds	t2_ahead	t2_never_seen	refused	final	fail_counter_delta	conflict_write	conflict_ref	fallback	clear	verdict
ctl	20	19	0	0	same	0	0	0	0	0	control-overtook
bin_nul	20	0	0	0	same	0	20	0	0	0	held
fk_bin_nul	20	0	0	0	same	0	20	0	0	0	held
```

Held, with a write conflict a round, as §2 reads the code. The line was then taken out of each
node's `cubrid.conf`, the pair restarted, and the run above made. So `fb1636c34` misses this key
only where trailing spaces count, which is the parameter's default.

Of the commits in `fb1636c34..b984637bb`, `cf8955a91` is the only one that touches
`src/transaction/writeset/`, and its difference from `eb31453ff` is that one branch of
`wset_pack_string_key` and a unit test. The others are #7881 (`ef4e7e812`) with the merge of develop
that brought it, `502685e64` and `9343b73f7` (unchanged from `f470fd0af` and `fb1636c34`),
`04ef0bc1d` (a stray file removed), and `b984637bb` (§5). No run isolates `cf8955a91`; what points
to it is the master's counters. With the parameter off, `fb1636c34` counted no write conflict in
`bin_nul`'s 20 rounds and 2 in `fk_bin_nul`'s, and `b984637bb` counted 20 in each.

## 4. The rest of `b984637bb`

On `hap11010a6b`, in this order:

| run | result |
|---|---|
| `pairs`, the whole catalog | 33 patterns with the control: 31 held, `fk_repoint` inconclusive, none missed, the control ahead in 19 of 20. The pair keeps trailing spaces, so `vc_trail` and `fk_vc_trail` were left out and the run said so |
| `pairs --rounds 50 fk_repoint` | held: a reference conflict in each of 50 rounds, every witness in time, the control ahead in 45 of 50 |
| `stress --ops 0,1,2,3,4,9 --seed 1` | **same**: 481 master errors, 728 write conflicts, one history clear, `fail_counter` unmoved |
| `pairs vc_trail fk_vc_trail bin_nul fk_bin_nul`, after `ignore_trailing_space=yes` in each node's `cubrid.conf` and a restart | all four held with 20 write conflicts each, the control ahead in 18 of 20, the pair answering yes |

`fk_repoint`'s one inconclusive round was a witness that had not reached the slave within the
30 seconds a round waits; the writeset counted 20 reference conflicts in its 20 rounds. It held in
the seven earlier runs whose ledgers are kept (one of them a single round), `fb1636c34`'s included.
Why that round was slow was not read. No other round in these runs had a late witness, no table
differed and `fail_counter` did not move. So on `b984637bb` all 34 patterns besides the control
held, on one pair, and the control overtook in every `pairs` run.

`stress` with the same seed on `fb1636c34` (the reduced-keys document) gave 468 master errors and
737 write conflicts. The SQL is the same. A reading, not a measurement: which statements the master
refuses depends on how the clients interleave, and `b984637bb` takes its locks for UPDATE and DELETE
differently (#7881).

## 5. Not here

- **`b984637bb` itself.** The coordinator now keeps the applied-tx markers when a statistics
  statement aborts the transaction that stores the apply info. The trigger, a statistics statement
  on the slave chosen as a deadlock victim, is not something a `pairs` or `stress` workload makes,
  and no run here restarted the applier; `testkit ha-apply` has no run that does.
- **`stress` with the foreign-key operations.** #8026 (CBRD-27464) is still open and not in
  `b984637bb`, so the reason of the reduced-keys document still holds.
- **A composite key element.** As before, every pattern is a single-column key or reference.

## 6. Reproducing it

```bash
export CSB_HOME=/data/...  TESTKIT_CSB=.../extensions/cluster-sandbox/bin/csb
testkit ha-apply pairs --build <an install of #8052 fb1636c34> bin_nul fk_bin_nul   # both missed
testkit ha-apply pairs --build <an install of #8052 b984637bb> bin_nul fk_bin_nul   # both held
```

What ran was `--cluster` on the kept `hap11016471` for the first line, and the whole catalog by
`--build` for the second.
