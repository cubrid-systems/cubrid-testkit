# `testkit ha-apply` and the trailing space: missed on `61ee0944a`, held on `3fb54e549`

- **Date:** 2026-11-01 (runs of 2026-10-30 to 11-01)
- **What this is:** the trailing-space completion condition of ADR-EXT-012, added after #8052
  `17aae152e` made the writeset hash a string key without the trailing spaces the index ignores. On
  a pair with `ignore_trailing_space` on, the two VARCHAR trailing-space patterns must be missed on
  `61ee0944a` and held on `3fb54e549`. Also here: the rest of the catalog and `stress` on
  `3fb54e549`, #8052's head when it was built on 2026-10-30. §7 is what #8052 did after it.
- **Command:** `testkit ha-apply pairs` and `stress`, in four builds of this change as it then
  stood: *CHAR only* (the CHAR patterns, the pair not asked), *before asking the pair* (CHAR and
  VARCHAR patterns), *asking before the barrier*, and the *final* build, which asks after the
  replication barrier and has no CHAR pattern. After the final build the reviews removed a refusal
  of `--build` with a trailing-space pattern, made a run that fails before it measures leave a
  ledger already at its path as it was, and then truncated a ledger only when it is a regular file.
  None of that is reached by the final build's runs here, which name a `--cluster` and write new
  ledgers. §4's `--build` run is of the change with all of it but the last, which its new ledger
  file does not reach. 20 rounds and 20,000 ballast rows a pattern, `ha_apply_worker_count=4`, the
  history at its default.
- **Pairs:** RelWithDebInfo, each with a client node, all on one host; most runs here overlapped
  with another pair's run.

| pair | #8052 | `ignore_trailing_space` |
|---|---|---|
| `its61` | `61ee0944a` (`11.5.0.2964-61ee094`) | yes |
| `its3f` | `3fb54e549` (`11.5.0.2967-3fb54e5`) | yes |
| `wsn4` | `61ee0944a` | no, the default |
| `hap1030f1c2` | `3fb54e549` | no key in its confs; not asked, since the builds that ran on it did not ask. Stood up by the command's `--build` and kept |

---

## 1. Which key splits the two heads

`17aae152e` drops, before hashing, what the index does not compare at the end of a string key:
trailing NULs always, and trailing spaces always for CHAR and for VARCHAR when
`ignore_trailing_space` is on. No pattern here writes a NUL. Of the spaces, only the VARCHAR case
splits the heads. CHAR(N)
is padded to its width before it is written, so `'Key1'` and `'Key1 '` are the same bytes in a CHAR
column on either head; the two patterns that wrote them held on both heads in every run (§3). A
VARCHAR keeps what it was given. With the parameter on, the index compares it without its trailing
spaces, and `'Key1'` and `'Key1 '` are one key that the old head hashed as two.

Two other commits lie between the heads, `ed8b462ba` and `3fb54e549`, in the applier and the log
only. No run isolates `17aae152e`; what points to it is the master's count of write conflicts for
these patterns, 0 a run on the old head and 20 on the new.

The two patterns, in round `r`:

- `vc_trail`: `t (id INT PRIMARY KEY, s VARCHAR(20) UNIQUE)`. Before the round `(2r, 'Key<r>')` is
  committed; T1 deletes it and T2 inserts `(2r+1, 'Key<r> ')`, which the unique index refuses until
  T1's delete is applied.
- `fk_vc_trail`: a parent `(s VARCHAR(20) PRIMARY KEY)` and a child with a foreign key on `s`. T1
  inserts the parent `'Key<r>'` and T2 a child that refers to `'Key<r> '`.

On a pair that keeps trailing spaces the two values are two keys, and T2 may rightly go first: a
missed there would be the command's error. §4 is how the command tells the two kinds of pair apart.

## 2. The two patterns on both heads

The final build:

```
# cluster=its61 build=11.5.0.2964-61ee094 ha_apply_worker_count=4 ha_writeset_history_size=unset ignore_trailing_space=yes rounds=20 rows=20000
pattern	rounds	t2_ahead	t2_never_seen	refused	final	fail_counter_delta	conflict_write	conflict_ref	fallback	clear	verdict
ctl	20	18	0	0	same	0	0	0	0	0	control-overtook
vc_trail	20	19	0	0	differ:t_vc_trail	19	0	0	0	0	missed
fk_vc_trail	20	14	0	0	differ:child_fk_vc_trail	19	0	2	0	0	missed
```

```
# cluster=its3f build=11.5.0.2967-3fb54e5 ha_apply_worker_count=4 ha_writeset_history_size=unset ignore_trailing_space=yes rounds=20 rows=20000
pattern	rounds	t2_ahead	t2_never_seen	refused	final	fail_counter_delta	conflict_write	conflict_ref	fallback	clear	verdict
ctl	20	20	0	0	same	0	0	0	0	0	control-overtook
vc_trail	20	0	0	0	same	0	20	0	0	0	held
fk_vc_trail	20	0	0	0	same	0	20	0	0	0	held
```

Two earlier builds ran the same statements: the format strings of both patterns are the same in
the three builds that have them. Their runs:

| build | pair | `vc_trail` | `fk_vc_trail` | control ahead |
|---|---|---|---|---:|
| before asking the pair | `its61` | **missed**: T2 ahead 19/20, `fail_counter` +19, `differ:t_vc_trail` | **missed**: 11/20, +17, `differ:child_fk_vc_trail` | 20/20 |
| asking before the barrier | `its61` | **missed**: 15/20, +15, `differ:t_vc_trail` | **missed**: 18/20, +19, `differ:child_fk_vc_trail` | 19/20 |
| before asking the pair | `its3f` | held | held | 19/20 |
| asking before the barrier, whole catalog (§5) | `its3f` | held | held | 19/20 |

On the old head the writeset counted no write conflict in any round of either pattern, and every
missed shows three ways at once: T2 seen first, the apply failing, and the table differing at the
end. On the new head both counted a write conflict in all 20 rounds of every run and never let T2
ahead. What the failed applies were was not read; a unique key for `vc_trail` and a missing parent
for `fk_vc_trail` is what T2 applied first would meet. For `fk_vc_trail` `fail_counter` moved more
than T2 was seen ahead (19 against 14, 17 against 11, 19 against 18): a round counts as ahead only
when the first look at the slave that finds the witness finds none of T1's ballast, so the ahead
count is a lower bound. The two reference conflicts `fk_vc_trail` counted on the old head in each run, and
`fk_char_pad` too (§3), are not explained here.

## 3. CHAR held on both heads

`char_pad` and `fk_char_pad` were `vc_trail` and `fk_vc_trail` with CHAR(20) for VARCHAR(20).

| pair | `ignore_trailing_space` | `char_pad` | `fk_char_pad` | control ahead |
|---|---|---|---|---:|
| `wsn4` (`61ee0944a`) | no | held | held | 20/20 |
| `its61` (`61ee0944a`), two runs | yes | held, held | held, held | 20/20, 19/20 |
| `hap1030f1c2` (`3fb54e549`) | no | held | held | 20/20 |
| `its3f` (`3fb54e549`), two runs | yes | held, held | held, held | 19/20, 19/20 |

Only the second run on `its61` and on `its3f` asked the pair; the first runs' yes, and the two
no's, are the nodes' confs (on `its61`, with the UNIQUE test of §4). The old head counted the write
conflict in every round of both, as the new head did. Through SQL,
which pads the value before writing it, the CHAR half of `17aae152e` changes nothing a pattern can
show; the two were removed from the catalog, since they hold on either head and ask nothing.

## 4. Asking the pair

cluster-sandbox's flags cannot set the parameter. `--set ignore_trailing_space=yes` is refused as a
key it does not know. `--set-hidden ignore_trailing_space=yes` writes the key to `cubrid_ha.conf`,
where the engine reads only HA parameters (`system_parameter.c`); `its61`, created that way, came up
with the parameter off, and a VARCHAR UNIQUE took `'k'` and `'k '` both.

Two ways work. csb copies the install tree's `conf/cubrid.conf` to every node
(`extensions/cluster-sandbox/internal/assembly/assembly.go`), so the key can go there before the
pair is created, which is also how a pair `--build` stands up gets it. Or it goes into each node's
`cubrid.conf` of a pair that is up, and the servers are restarted. `its61` and `its3f` were made
the second way: `ignore_trailing_space=yes` under the first `[common]` of each node's
`cubrid.conf`, the two servers' and the client's, and the two servers stopped and started. On
`its61` the same UNIQUE then refused `'k '`; on both pairs the question below answered 1. The
first way is the `--build` run at the end of this section.

A key in the wrong file is ignored without a word, so `pairs` asks the comparison, not the conf:
after waiting for the slave it sends `SELECT CAST('k' AS VARCHAR(4)) = CAST('k ' AS VARCHAR(4))`
to both nodes, which answer 1 where trailing spaces are ignored, and the ledger's first line
carries the answer (`ignore_trailing_space=yes` in the blocks above). The final build on `wsn4`:

- Naming `vc_trail`: exit 1, nothing on standard output, no ledger left, and on standard error
  `ha-apply: wsn4 does not compare VARCHAR without trailing spaces (ignore_trailing_space=yes in
  its nodes' cubrid.conf), so vc_trail cannot run there`.
- The whole catalog with `--rounds 1`: `ha-apply: wsn4 does not compare VARCHAR without trailing
  spaces, so vc_trail, fk_vc_trail are not run` on standard error, and a ledger of the other 24
  patterns with `ignore_trailing_space=no` on its first line. One round a pattern judges nothing
  here; the run was for what it left out.

Two nodes that answer differently are covered by the tests with a fake csb, not by a pair.

With `--build`, the command as committed stood up a pair from the `3fb54e549` install tree whose
`conf/cubrid.conf` had the key under its first `[common]`, asked it, ran both patterns, and removed
the pair; the key was taken out of the install tree after the run:

```
# cluster=hap1031f219 build=11.5.0.2967-3fb54e5 ha_apply_worker_count=4 ha_writeset_history_size=unset ignore_trailing_space=yes rounds=20 rows=20000
pattern	rounds	t2_ahead	t2_never_seen	refused	final	fail_counter_delta	conflict_write	conflict_ref	fallback	clear	verdict
ctl	20	18	0	0	same	0	0	0	0	0	control-overtook
vc_trail	20	0	0	0	same	0	20	0	0	0	held
fk_vc_trail	20	0	0	0	same	0	20	0	0	0	held
```

## 5. The rest of the catalog on the new head

| pair | build | patterns | verdicts | control ahead |
|---|---|---:|---|---:|
| `hap1030f1c2` | CHAR only | 26: the 24 of M1 and the two CHAR | all held | 20/20 |
| `its3f` | asking before the barrier | 28: those and the two VARCHAR | all held | 19/20 |

The counts include the control, as the ledgers' do. No round in either run had a late witness, no
table differed and `fail_counter` did not move. Each pattern counted its conflict in all 20 rounds:
a write conflict, or a reference for `fk_delete`, `fk_cascade` and `fk_repoint`, or for `ddl_dml` a
fallback with a history clear. M1's run on `61ee0944a` had two patterns inconclusive for a late
witness, whose cause is not established; these two runs had none, which alone does not say the
head is why.

## 6. `stress` on the new head

`hap1030f1c2`, the build that asked before the barrier; `stress` is the same code as
ADR-EXT-012 M2's commit, but for how the ledger is opened, which the header's later changes touch
and these runs, on new ledgers, do not reach. Eight clients, 200 transactions each, 20 keys, 3,000
ballast rows.

| operations | seed | verdict | master errors | write / ref conflicts | `fail_counter` | master orphans seen while running | final tables |
|---|---:|---|---:|---|---:|---|---|
| 0–4, 9 | 1 | **same** | 449 | 754 / 0 | 0 | 0 polls | same |
| all | 1 | diverged | 696 | 800 / 107 | +3 | 2,197 polls, at most 1 | `s_child`: 11 rows on the master, 10 on the slave |
| all | 2 | diverged | 757 | 770 / 106 | +2 | 612 polls, at most 1 | same |
| all | 3 | diverged | 700 | 742 / 100 | +7 | 473 polls, at most 1 | same |

M2's condition holds on the new head: without the foreign-key operations the run is the same on
both nodes. With them the three runs diverged as they did on `61ee0944a`
([ha-apply-stress-and-the-heap-only-row](ha-apply-stress-and-the-heap-only-row.md) §3). In seed 1
the slave ended without the child `(9, 19)`, by heap and primary key alike, with no orphan on
either node at the end; in seeds 2 and 3 only `fail_counter` says so. Each run had one history
clear. That this is CBRD-27464, which #8026 (open) fixes and #8052 does not, is the same reading as
there, with the same limits: the failing rows were not read. Three runs of three diverged here
against six of nine there, which neither shows nor rules out a difference between the heads.

## 7. After `3fb54e549`

#8052 moved on while these runs went; on 2026-11-01 its head is `fb1636c34`. One of the commits
since, `eb31453ff` ("master: hash a key the way its index compares it"), names four more kinds of
key the index holds equal that the writeset hashed apart: a MONETARY amount in two currencies
(`$100` and `\100`), a VARBIT, which the writeset hashed with its bit length and now with that
length in whole bytes, a string with a NUL where the compare sees a space, and a DOUBLE NaN, which
the index finds equal to every key and whose transaction now falls back to commit order. The
catalog has no pattern for any of them, so "the whole catalog held on `3fb54e549`" (§5) says
nothing about these keys. No run here used `fb1636c34`.

## 8. Reproducing it

```bash
export CSB_HOME=/data/...  TESTKIT_CSB=.../extensions/cluster-sandbox/bin/csb
# the install tree's conf, which csb copies to every node (take the key out after)
sed -i '0,/^\[common\]/s//[common]\nignore_trailing_space=yes/' <an install of #8052>/conf/cubrid.conf
testkit ha-apply pairs --build <that install> vc_trail fk_vc_trail

# or a pair that is up, with a client node and four workers
csb cluster create --name <pair> --preset ha --build <an install of #8052> --clients 1 \
    --set-hidden ha_apply_worker_count=4
for n in n1 n2 c1; do
  sed -i '0,/^\[common\]/s//[common]\nignore_trailing_space=yes/' \
    $CSB_HOME/clusters/<pair>/work/<pair>-$n/cubrid/conf/cubrid.conf
done
for n in n2 n1; do csb node stop <pair>-$n --cluster <pair>; done
for n in n1 n2; do csb node start <pair>-$n --cluster <pair>; done
testkit ha-apply pairs --cluster <pair> vc_trail fk_vc_trail
```

Measured here: on `61ee0944a` both missed, on a pair that was up; on `3fb54e549` both held, both
ways. On a pair without the key the `--cluster` command is refused (§4).
