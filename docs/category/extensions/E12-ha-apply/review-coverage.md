# E12 — What `ha-apply` covers of #8052, and whether it can carry a review

*English · [한국어](review-coverage.ko.md)*

**As of:** 2026-11-02, #8052 at `787defb50`
**Sources of the items:** CBRD-27459's comments (the design document is an attachment and was not
read), #8052's 88 commits not in develop (merges excluded), and the fields of
`SHOW WRITESET STATUS`
**Evidence:** `docs/project/evidence/ha/`, named per row

---

## 1. How to read the tables

- **verified**: a run was missed on a head before the fix that closes the case and held on the fix
  or after it. A held run there is evidence: the check is shown able to fail. Where the head before
  is not the fix's parent, the row says so.
- **check shown**: the check failed on some head through the same observation, though not for this
  row's own case.
- **held only**: run, and held or same on every head tried, with no head shown to fail it. A held
  run there says "nothing showed", not "correct".
- **not run**: nothing here reaches it.

Every testkit run, and every run from `61ee0944a` on, is a RelWithDebInfo build, which compiles the
applier's assertions out. The one debug pass was 12 of 13 shell-prototype patterns on `bf560620e`.

## 2. The master: writeset generation

| item | verb | status |
|---|---|---|
| a key hashed as its index compares it: FLOAT and DOUBLE -0.0, TIMESTAMPTZ and DATETIMETZ zones, a subclass and its superclass | `pairs` (`fp_negzero`, `fk_negzero`, `tstz_zone`, `dttz_zone`, `fk_tz`, `inherit_uk`) | **verified**: missed on `bf560620e`, held on `61ee0944a`, about 24 commits apart, several of them writeset fixes ([ha-apply-gives-the-prototypes-verdicts](../../../project/evidence/ha/ha-apply-gives-the-prototypes-verdicts.md)) |
| a VARCHAR key and the same with trailing spaces, on a pair with `ignore_trailing_space` on | `pairs` (`vc_trail`, `fk_vc_trail`) | **verified**: `61ee0944a`, the parent of `17aae152e`, against `3fb54e549` ([ha-apply-and-the-trailing-space](../../../project/evidence/ha/ha-apply-and-the-trailing-space.md)) |
| MONETARY currencies, VARBIT padding, a NUL and a space, NaN | `pairs` (`mon_cur`, `vb_len`, `str_nul`, `dbl_nan`, `fk_*`) | **verified**: missed on `3fb54e549`, held on `fb1636c34`; no run isolates `eb31453ff` ([ha-apply-and-the-reduced-keys](../../../project/evidence/ha/ha-apply-and-the-reduced-keys.md)) |
| the binary collation's trailing NUL, on a pair with `ignore_trailing_space` off | `pairs` (`bin_nul`, `fk_bin_nul`) | **verified**: missed on `fb1636c34`, held on `b984637bb`; `cf8955a91` is `eb31453ff` rewritten on the same parent, and no run isolates it ([ha-apply-and-the-binary-nul](../../../project/evidence/ha/ha-apply-and-the-binary-nul.md)) |
| a case-insensitive collation | `pairs` (`ci_case`, `fk_ci`) | held only: held on `bf560620e` and on every head after it |
| a CHAR key and the same with trailing spaces | `pairs` (`char_pad`, `fk_char_pad`, since removed) | held only: held on `61ee0944a` and `3fb54e549`; SQL pads a CHAR before it is written, so no pattern can make the two differ ([ha-apply-and-the-trailing-space](../../../project/evidence/ha/ha-apply-and-the-trailing-space.md)) |
| other trailing NULs, an accent-insensitive collation, function-based and filtered indexes, the reduced kinds inside a composite key | none | not run |
| PK, UNIQUE, reverse UNIQUE and FK keys making a dependency | `pairs`, the base patterns | **check shown** by the equality patterns, which fail through it; the base patterns held on every head |
| partitions, savepoints, a DDL before a DML | `pairs` (`part_move`, `sp_*`, `ddl_dml`) | held only |
| the history overflowing and clearing | `pairs --history 50000`, `stress --history 1000` | held only; a clear orders what follows it, so a round with one holds by construction |
| the per-transaction key limit (the same number as the history) | inside `stress --history 1000` | held only; its fallbacks are not counted apart |
| a statement falling back to commit order | `restart --statements` | held only on the master's side (48,005 of 48,007 commits fell back); the slave's restart row is verified for statements |
| a DML, unique or FK check meeting a committing transaction, which waits for its commit | `stress`, indirectly; the prototype `parallel-apply-chaser.sh` on `bf560620e` (300 FK and 300 unique rounds, no control) | held only: a weak no |
| serial fragments: the `_db_serial` write-back of AUTO_INCREMENT and SERIAL, `INCR` | prototype `parallel-apply-serial.sh` | held only: the same on `787defb50` and on `3fb54e549`, which did not reproduce `501e5f315`'s case |
| `COMMIT WORK RETAIN LOCK` keeping the MVCCID self-lock (`787defb50`) | none | not run |

## 3. The slave: the parallel applier

| item | verb | status |
|---|---|---|
| the gate holding a transaction until what it depends on is applied | `pairs` | **check shown**: a transaction applied before its dependency is seen; the equality patterns' misses were dependencies the hash lost, not the gate's |
| the slave stopping | `stress` | **verified** through the lock escalation row below |
| the pair ending the same under concurrent clients (heap, PK, orphans) | `stress` | **check shown** without the foreign-key operations: `--compare-only` calls #8198 `6ce1c05d2`'s damaged pair damaged, and runs with `--ops 0,1,2,3,4,9` were same on every head without `lock_escalation=5`. With the foreign-key operations, 6 of 9 runs diverged on `61ee0944a` and 3 of 3 on `3fb54e549` (`fail_counter`, and once on each a row missing on the slave), read as CBRD-27464, which #8026 fixes; not measured, and not run with them on `fb1636c34` or after |
| a restart not applying a committed transaction twice (the markers) | `restart` | **verified**: applier kills and node crashes, `19e83a5d9` (the parent of `72650ce0d`) against `b984637bb` and `787defb50`; statements, `b84567f36` (the parent of `d398beba2`) against `787defb50` |
| the marker delete escalating into a deadlock | `stress --set lock_escalation=5` | **verified**: `cf8955a91` against `502685e64`, its fix, alone |
| worker counts other than 4 | `--workers 1`, `--workers 16` | held only; shown able to fail at 4 |
| the statistics aborting the apply info store (`b984637bb`) | none | not run: nothing here makes its trigger, a statistics statement chosen as a deadlock victim |
| markers read only in part at start (`9343b73f7`) | none | not run: nothing here makes the read fail |
| the memory limit (`ha_apply_max_mem_size`) | `restart --set ha_apply_max_mem_size=1` | not run: the check skips an applier's first 10 seconds, and the kills came every 5 |
| the copied log's edges (a record past its end, archives removed, records past a page), LOBs | none | not run |
| a read on the standby seeing a state the master never had (independent transactions applied out of commit order) | `pairs` sees it (the control going first) and counts it as allowed | not run as a question: whether such reads are acceptable is the design's |
| the SQL log, the statistics tables, `applyinfo`, memory freed at shutdown | none | not run |

## 4. Faults

| fault | status |
|---|---|
| the slave's applier killed | **verified** |
| the slave node crashed (`csb node kill`) | **verified** |
| the master crashed, and the slave taking over | held only: a prototype, three runs on `787defb50` |
| a crash during the slave's recovery; a graceful restart; failback; another copy mode; a network partition | not run |

## 5. Can it carry a review?

As the main dynamic check beside code review, **yes**, on one condition: a held run counts as
evidence only where the table says **verified** or **check shown**.

- It covers the ways a parallel applier goes wrong: a dependency lost and the order reversed
  (`pairs`), a commit applied twice after a restart (`restart`), and the slave stopping or
  diverging (`stress`).
- For the first two, and for the slave stopping, a run failed on a head before a real #8052 fix and
  held after it, so a held run there is not a check that says nothing. Diverging is weaker: the
  comparison is shown to catch damage, but no #8052 fix was run before and after, and with the
  foreign-key operations it diverged on earlier heads, a reading not settled.
- A head is one command a verb: `787defb50` took an hour or more of runs besides its build,
  estimated from the documents' averages.
- The verdicts are on record in the ledgers, and the rules that give them are in ADR-EXT-012 and the
  requirements, so a reviewer can check both.

What the review still carries itself:
- the rows marked held only, where the code review decides and a held run is support at most;
- the rows marked not run, by the code review or by a fault the engine lets a test inject;
- the `stress` divergence with the foreign-key operations;
- the races: most verdicts are one run a head, so "no defect" is worth as many runs as were made;
- what the assertions would catch on later heads, all run as RelWithDebInfo;
- where a verified row's head before is not the fix's parent, what else lies between;
- the design's own choices (when `max_committed_lsa` is stored, the comparison with MySQL, standby
  reads of states the master never had), which are for the design review, not a test.

What would make it the main check without condition:
1. A head that fails each held-only row with a positive control in reach (the history clear, the
   serial fragments, the failover), or fault points in the engine where none fails.
2. `stress` with the foreign-key operations on the current head, with #8026 or after it merges.
3. Repeated runs for the verdicts that depend on timing.

## 6. Using it on a fix

For each #8052 commit that fixes a case:
1. If a workload reaches the case, ask for a run that is missed on the commit's parent and held on
   the commit.
2. If none does, keep it as a code-review item.
3. Or add a debug-only fault point to the engine, so a run can reach the case.
