# A deadlock victim at the foreign-key index lock keeps running: a hang, or a row no index has

- **Date:** 2026-10-30
- **What this is:** one run of [`parallel-apply-stress.sh`](parallel-apply-stress.sh) on develop with
  CBRD-27464's fix (#8026) did not end: seven clients waited on the master for two hours. That was not
  #8026. A child INSERT that loses a deadlock while it locks its own new row for the foreign key index is
  told nothing and goes on; it then waits again, for good, or commits a row that no index has. The engine
  code is in develop. The fix went to CUBRID/cubrid #8198 (CBRD-27463), together with one more deadlock of
  the same family that #8198's own change had made reachable; a third, already in #7881, is left open.
- **Trees:** develop `3ca42d9`; develop + #8026; #7881 (CBRD-27034) at `a529ea9`; #8198 at `c08963fbc`, at
  `6ce1c05d2` after it merged `feature/lock_new` again, and with the fix at `ef96fd9d0`. All release builds,
  csb HA pairs with a client node.

---

## 1. The case, in three sessions

[`fk-victim-reused-slot.sh`](fk-victim-reused-slot.sh). C holds child keys 100 and 101 in progress. A
inserts child 100 and B inserts child 101; both wait on C and fail with a unique violation together. Each
keeps the X lock it took on its own new row, while the statement rollback frees the row's slot. B (which
has also inserted parent 17 and 2,000 ballast rows, so writes more log) then inserts child 200 into A's
freed slot and waits on A's lock. A inserts child 201, under parent 17, into B's freed slot and waits on
B's. Deadlock; A is the victim.

| build | lock timeout 20 s (2 runs) | infinite lock wait |
|---|---|---|
| develop `3ca42d9` | 2 deadlocks a run. A's first error is logged by the server but never reaches the client; A's statement goes on to its foreign key check, waits on B's MVCCID self-lock, and is chosen again a second later | 1 deadlock, then A and B wait on each other for good (1 run, still waiting at 51 s) |
| develop + #8026 | the same | the same (1 run, 90 s) |
| #7881 `a529ea9` | the same | the same (1 run, 140 s) |
| #8198 `c08963fbc`, `6ce1c05d2` | the same | the same (2 of 2 each) |
| develop + one line (below) | 1 deadlock a run; A gets -966 at the first | 1 deadlock; A gets -72, B commits, nothing left waiting (2 of 2) |
| #8198 `ef96fd9d0` | no deadlock; both commit | the same (2 of 2) |

The stress run that found it hung the same way: the event log recorded one deadlock at 12:04:10.271 with
T6 as the victim and a -72 logged for it (that pair has since been removed, and the run's notes are now the
only record of those two lines), and two hours later gdb showed T6's thread in
`locator_check_foreign_key ()` waiting on T8's MVCCID self-lock with `tran_abort_reason = 1` and the
transaction still active, while T8 waited on T6's X lock on a reused slot.

## 2. Why

- `locator_add_or_remove_index_internal ()` (develop `locator_sr.c:7960-7966`) takes X on the row it is
  inserting before it adds the row to a foreign key index, and when that lock is refused it jumps to its
  exit **without setting `error_code`**, so it returns `NO_ERROR`. A victim's -72 or -966 ends there. The
  index entries after that point are not made, and `locator_insert_force ()` carries on to the foreign key
  check. Adding `ASSERT_ERROR_AND_SET (error_code)` before the jump is what the "one line" row above
  measures.
- By reading the code (no build from before it was run), the wait itself became reachable with
  CBRD-26942 (#7266): an MVCC insert takes no row lock and, on a
  reused slot too, returns before the conditional lock that moves a non-MVCC insert on to the next slot
  (`heap_file.c`). Its comment says a slot is reusable only after the old deleter committed, but a
  statement rollback frees an inserted row's slot while the rolling-back transaction still holds the lock
  it took on that row.
- With an infinite lock wait the detector does not break the second cycle, and only one deadlock is
  logged; with a lock timeout it breaks it a second later. Why was not traced.

## 3. The row no index has

On #8198 `c08963fbc`, three 8-client stress runs on parents and children (operations 5–8) ended with orphan
children on the master after two of them, none on the slave. After the third, the master's heap had child
(15, 17), whose parent was gone, while every read through `pk_s_child_c` or the foreign key index showed
six rows and `COUNT(*)` seven, and checkdb reported:

> INDEX pk_s_child_c ... the number of OIDs in the unique hierarchy: 7 does not equal the number of OIDs:
> 6 found in the unique B+tree

The event log shows (15, 17) was the insert of the deadlock victim that waited for X on its own new row.
Its index entries were never made, and the slave never had the row. The parent's cascade finds children
through the foreign key index, so it would not have seen the row either (a reading; not traced). Three
runs on `6ce1c05d2` left two such rows after one of them (checkdb: 9 against 7).

[`parallel-apply-stress.sh`](parallel-apply-stress.sh) reported `s_child=same` for that run: it compared
the tables through their primary keys, which is exactly where the row is missing. It now also counts each
table's heap through an expression no index covers and reports `<table>.heap(...)` when the master's heap,
the slave's heap and the master's primary-key read disagree. Its queries on the damaged `6ce1c05d2` pair
give a master heap of 9 against a primary-key read of 7, and 7 and 7 on the slave.

## 4. The fix in #8198, and what it found next to it

#8198 removes the foreign key check's S lock on the parent row. Two variants were measured against its
head, on the same set ([`fk-setnull-open-insert.sh`](fk-setnull-open-insert.sh),
[`fk-setnull-open-change.sh`](fk-setnull-open-change.sh),
[`fk-restrict-open-insert.sh`](fk-restrict-open-insert.sh),
[`fk-plain-update-restrict.sh`](fk-plain-update-restrict.sh), CBRD-27464's two cases, three stress runs
with checkdb, and 238 isolation cases of `tc/pr-8198`): v1, the one line; v2, the X lock on the inserted
row removed (with the `skip_checking_fk` parameter only that lock read) and the CASCADE / SET NULL passes
waiting for a child's writer **before** locking the child.

| case (runs) | #8198 head | v1 | v2 (committed as `ef96fd9d0`) |
|---|---|---|---|
| §1, infinite wait (2) | hang | 1 deadlock, A -72 | no deadlock |
| child INSERT open, parent key change with ON UPDATE SET NULL, then the inserter deletes another parent through RESTRICT (2) | clean | clean | clean |
| the same with the child's change an UPDATE (2) | **deadlock**, the key change -72 | deadlock | clean |
| the same with a DELETE (1), CBRD-27464's two cases, RESTRICT behind an open child insert that commits or rolls back | clean | clean | clean |
| stress (3), then checkdb | orphans after 2, a heap-only row | none, clean | none, clean |
| a plain UPDATE of a child another transaction updated, then that transaction's RESTRICT delete (2) | deadlock | deadlock | deadlock |
| isolation (238) | 235, 3 NOK | the same | the same |

- Removing the X lock alone gave the SET NULL row a new deadlock (2 of 2). By reading the code, the pass
  locked the uncommitted child at once and waited for the inserter while holding it, and the inserter's
  RESTRICT check, which asks for S on the row with no owner check, waited behind it. Waiting first is what
  v2 adds.
- The UPDATE row is a deadlock #8198 introduced: develop and #7881 alone do not have it (2 of 2 each).
  A lock table dumped during the wait on #7881 showed why: the child's foreign key check held S on the
  parent rows, and the key change waited on the parent row instead of reaching the child. It was read on
  the screen and not saved.
- The plain-UPDATE row reproduces on #7881 alone (2 of 2) and not on develop. By reading the code, #7881 gives an
  UPDATE's row lock back at the end of its statement, so the second UPDATE locks the row and waits for the
  stamp's owner while holding it. It needs an owner check where the restrict check takes its S, and is left
  open.
- The three isolation NOKs are `owner_retouch/*fk_holder*`, written for #7881 on the premise that the
  foreign key check holds S on the parent. They fail on #8198 with and without the fix; they were not run
  on develop or #7881.

## 5. What is left

- develop has the code in §2 and reaches it through CBRD-26942's slot reuse. It gets the fix when #8198,
  rebuilt after #7881 lands, merges into develop; no separate issue is planned, and the check to repeat
  then is §1 and §4's set on the rebuilt head. CBRD-27773 (open) is the same symptom on another path: an
  online index build chosen as the victim waits again for the same lock, one deadlock is logged, and both
  wait for good.
- The plain-UPDATE deadlock of §4.
- The stress runs before this commit cannot be repeated: mawk's `srand (n)` does not repeat its sequence,
  so a seed labelled a run but did not name its SQL. The generator is now a Park–Miller sequence of its own.

## 6. Reproducing it

```bash
export CSB_HOME=/a/large/disk/csb
csb cluster create --cluster dev --build /path/to/develop/install.out --preset ha --clients 1 --tools $T
csb node exec dev-c1 --cluster dev -- sh /tools/fk-victim-reused-slot.sh dev dev-n1 dev-n2 20
csb node exec dev-c1 --cluster dev --timeout 60s -- sh /tools/fk-victim-reused-slot.sh dev dev-n1 dev-n2 0
# the second leaves two transactions waiting on the master; csb cluster down/up clears them
```
