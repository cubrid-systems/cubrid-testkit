# ADR-021: A crash report is a failure

- **Date:** 2026-09-17
- **Status:** Proposed — implemented for isolation, sql, medium and shell
- **Related:** ADR-003 (the external surface is frozen), ADR-013 and ADR-017 and ADR-018 (the equivalence
  gates this deliberately departs from), `evidence/isolation-always-failing.md` §1,
  `category/isolation/05-when-a-case-fails.md`

## Context

CTP decides that a case dumped core by looking for **files named `core.*`**, and that a server failed by looking
for **`FATAL ERROR`** in the log: `runone.sh`'s `checkCoreAndFatalError` for isolation, CQT's `getCoreFiles` for
sql, `do_check_more_errors` for shell. This runner inherited all three.

None of them sees a server that dies of a signal on a machine where
`/proc/sys/kernel/core_pattern` hands cores to a crash handler such as apport. There is no `core.*` to find,
and a SIGSEGV writes no `FATAL ERROR` line. What there is, always, is the engine's own report: `crash_handler`
(`server.c:222`) prints the call stack to `$CUBRID/log/coredump/<program>_<when>.coredump` before the process
ends.

**This is not hypothetical.** `_01_ReadCommitted/partition_table/range/dml_ddl/reorganization_select_01`
crashes `cub_server` every time it runs on this engine, and every run this project has records of — two CTP
runs, four of its own — reported it as an ordinary diff failure. It is upstream's `CBRD-27407`, reported from
QA's own run of the same case, confirmed, and not fixed; the reports were on disk the whole time
(`evidence/isolation-always-failing.md` §1).

A dead server is also not a private matter for the case that met it: the cases after it in that slot run
against a database that has just recovered.

## Decision

**A run looks for what a dying server leaves — itself — and anything new fails the case it appeared under. It
does not copy the install into `$HOME` to do it.**

- After every case — passing or failing, and in the place that owns the install, because a slot's `$CUBRID` is
  an overlay of its own — the runner looks for **three** things and takes what is new since the case before it:
  the engine's report under `$CUBRID/log/coredump`, a `core.*` file where the machine writes one (CTP's own
  pattern), and `FATAL ERROR` lines gained in `$CUBRID/log`. Nothing sweeps those places between cases, so what
  counts against a case is the increase and not the total — where CTP's check fired again for every case that
  followed one.
- A new report makes the case **NOK**, with a result line naming the report, the program and the first frame
  below the crash handler:
  `: NOK found crash report cub_server_20260917125012.888.coredump (cub_server ctldb, qdata_save_agg_hentry_to_list at query_aggregate.cpp:2974)`
- The report is **copied into the run directory** (`crash/<case>.<report>`) before the slot closes, because the
  slot's overlay is thrown away with it, and the report is the only evidence left of a process that no longer
  exists. It is read through the channel rather than copied from the host: the path exists only inside that
  slot's mount namespace. A **core file** is kept as its gdb stack in the same place, as sql already does: a
  core is gigabytes, it belongs to a slot that is about to go, and the stack is what a reader needs.
- **`~/error_backup` stops being written by default** (isolation). CTP's check and its backup are one switch:
  `runone.sh -n` turns off both, and the backup stops the service and copies the whole install, per case that
  finds something. The check is worth having; a copy of the install in `$HOME` is not something a run should do
  unasked. So the runner passes `-n`, does the checking itself, and `backup_core_file_yn=yes` puts CTP's
  behaviour back.
- The run says so on standard error as well, so that a long run does not hide it until the end.
- Anything unreadable — no install, no directory, an empty report — is **not** a crash. Reading the machine may
  not fail a case that otherwise passed.

`internal/coredump` holds it, beside the core-file analyzer that is CTP's. This is the other half of the same
question, and it needs no gdb: the engine has already written the stack.

## Consequences

1. **Verdicts can now differ from CTP's, by design.** A case whose server died but whose output still matched
   its answer passes under CTP and fails here. That is a runner difference under ADR-018 rule 2, and it is the
   one this project chooses to have: a green verdict over a dead server is worse than a disagreement. A
   comparison against CTP declares it rather than explaining it away, and `evidence/` says where it happened.
2. **Standard output changes only where a server crashed.** ADR-003 freezes the surface, and this adds no line
   to a run in which nothing died. Where one did, the `[NOK]` and the result line are the change.
3. **A crash that CTP hid is now a first-class finding.** The reports are kept with the run, so the stack is in
   the results rather than in a directory that the next `clean.sh` may empty.
4. **The check costs one `find` per case**, in the slot, over a directory that is empty on a healthy run.
5. **It replaces CTP's core-file check for isolation, and adds to it elsewhere.** isolation passes `-n`, so the
   `core.*` and `FATAL ERROR` checks are this runner's too; sql and shell keep the ones they had and gain the
   crash report. The core file itself is no longer preserved anywhere for isolation — its stack is. A run that
   needs the core keeps it with `backup_core_file_yn=yes`.
6. **One default now differs from CTP's.** `backup_core_file_yn` is `no` here and `yes` there. It changes what a
   run writes into `$HOME`, not what it judges — except that the judging is now this runner's.

## Alternatives considered

**Report it and leave the verdict alone.** The run would say a server died and still call the case passed,
which is the state this ADR exists to end. Considered and rejected by the operator: a crash is a failure.

**Fail the whole run at the end instead of the case.** It keeps every case's verdict comparable with CTP's, but
it loses which case did it, which is the fact a reader needs first.

**Teach CTP's scripts to look.** `runone.sh` is CTP's and unchanged by decision (ADR-007); a change there would
have to go upstream and be waited for. Upstream should still have it — it is filed in
`evidence/ctp-improvements.md` — and until then this runner does not need CTP's permission to notice a dead
server.

**Set `core_pattern` so that cores land where CTP looks.** It is a machine-wide setting, needs root, and would
make the runner's verdicts depend on how the machine is configured. The engine's report is always written.
