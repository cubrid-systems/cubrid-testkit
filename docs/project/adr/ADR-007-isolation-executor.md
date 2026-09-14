# ADR-007: An isolation case is executed by runone.sh and ctltool, unchanged

- **Date:** 2026-09-15
- **Status:** Accepted (2026-09-15) — the condition it was proposed with, a one-slot native run matching CTP on the
  sample, is met: every verdict file, `feedback.log` and 58 of 58 normalized results identical
  (`evidence/isolation-baseline.md` §2)
- **Related:** ADR-001 Consequence 4 (external tools through a subprocess), ADR-002 (`ctltool/Makefile` is kept),
  ADR-016 (the same split for sql), ADR-008 and ADR-009 (reserved: grammar and normalization),
  `design/module-isolation.md`, `evidence/isolation-baseline.md`

## Context

This number was reserved in Phase 2 for one question: what the new system does with `ctltool` — absorb it, call it,
or cut it down to CUBRID. Reading the source to answer it (`cubrid-testtools` develop `a1bec87`, `CTP/isolation/`)
showed that the question is larger than the C binaries.

### The verdict is made inside the script, not by the runner

CTP's Java does very little per case. It sends one command and reads markers back (`Test.java:174-218`):

```
cd $ctlpath; sh runone.sh [-n] -r <retries+1> <case.ctl> <timeout> qacsql 2>&1
```

Everything that decides the outcome happens in `runone.sh` (`runone.sh:239-407`):

- **setup** when needed — `prepare.sh` deletes and creates `ctldb` and builds `qactl` and `qacsql` against the engine
  under test with `make`;
- **execution** — `timeout3.sh` around `qactl ctldb <case> qacsql`;
- **normalization** — fifteen `sed` steps over the output (`runone.sh:48-73`), which the freeze grades F1;
- **answer selection** — `<name>.sh` if the case ships one, otherwise `answer/<name>.answer*` in `ls` order, the first
  identical one winning;
- **retries** — up to `-r` attempts, stopping at `flag: OK`;
- **core handling** — backing up cores and the whole install, then recreating the database;
- **cleanup** — `clean.sh` dropping every user object through seventeen `csql -u dba` round trips.

So the 6,865 base answers in the corpus are not "what the database returned". They are what `qactl` printed, after that
chain, compared in that order.

### ctltool talks to CUBRID below any driver

`qactl` and `qacsql` link `libcubridcs` and use the C client API: `db_restart` to connect, and `local_tm_isblocked`
and `lock_dump` to see a client waiting on a lock (`qactl.c:441-688`, `cubrid_drv.c:49`, `:594`). `MC: wait until C2
blocked` — the statement the whole DSL exists for — is answered from the lock table, not from a timeout. No Go driver
exposes that.

### The script is also dangerous

It kills by user, not by run: `pkill -9 -u $(whoami) cub` in setup, `pkill -u $(whoami) -9 sleep` after every case,
the user's `qactl` and `qacsql` in cleanup (`prepare.sh:39`, `runone.sh:390`, `clean.sh:33-35`). It writes into the CTP
tree (`make clean`, logs in the working directory) and into the cases tree (three files per case).

## Decision

**The runner owns everything around a case. The case is executed by `runone.sh` and ctltool, unchanged, as a
subprocess.**

| the runner | the executor |
|---|---|
| discovery, the exclusion file, the queue, slots | `runone.sh`, `prepare.sh`, `clean.sh`, `timeout3.sh` |
| the verdict, from the executor's standard output, by CTP's rule: the last `flag: NOK` after the last `flag: OK` fails, any `found core file` or `found fatal error` fails, no `flag: OK` fails | `qactl` and `qacsql`, built with the tree's own `Makefile` against the engine under test |
| every record: the run directory, the worker log, feedback, the archive | `result/<name>.result`, `result/<name>.log` and `<name>.result` in the cases tree |

**CUBRID only.** The MySQL and Oracle builds of ctltool are not built or supported. Their `prepare_*` functions are
`echo "TODO"` in CTP itself (`prepare.sh:68-79`), and `cubrid_testdb_name` has no documented value but `cubrid`.

**Contained, always.** A slot — PID, IPC, network and mount namespaces, an overlay over `$CUBRID`, over the ctltool
directory and over the corpus — is where the executor runs, including when there is only one. The kills then reach only
the slot, and what the script writes is thrown away with it.

## Consequences

1. **Per-case parity is by construction.** The bytes a case produces come from the same script and the same binaries.
   What the gate has to establish is that the runner around them — order, records, verdicts — is CTP's, and how much
   the executor varies against itself.
2. **The DSL and the normalization are not reimplemented for the gate.** ADR-008 and ADR-009 stay reserved. They are
   needed only if execution later moves into Go, and then `runone.sh` is the oracle, as `jdbc` is for `native` in
   ADR-016.
3. **The runner builds C.** `make` and `gcc` become requirements of an isolation run, as they already are of CTP's —
   once per run, against the engine under test, instead of inside the CTP tree on the first case.
4. **The speed ceiling stays the script's.** `clean.sh`'s round trips and `prepare.sh`'s database creation are paid as
   CTP pays them. Parallel slots are the improvement available without changing the executor; changing what happens
   between cases is a separate decision with its own evidence.
5. **`$HOME` is still shared.** The core path writes `~/error_backup` and truncates `~/CUBRID/log`. The slot does not
   cover them yet (`module-isolation.md` §6).

## Alternatives considered

**Absorb ctltool: a Go interpreter for `.ctl` and a Go client.** Rejected. Blocking is observed through the C API's
lock table, which no Go driver reaches; the rewrite would also have to reproduce `qactl`'s own output, the fifteen `sed`
steps and the answer order before a single verdict could be compared.

**Call `qactl` directly and move the rest of `runone.sh` into Go.** Rejected for the gate, kept for later. It is the
natural next step — it is where the time between cases can be won — but for the gate it means rewriting the
normalization and the answer selection only to compare them with the script they replace.

**Run CTP's Java as it is.** That is the legacy runner, which already works. It keeps a JVM for a loop of two hundred
lines, cannot contain the kills, and gives nothing to parallelize.
