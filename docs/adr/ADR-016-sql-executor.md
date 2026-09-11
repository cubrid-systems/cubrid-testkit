# ADR-016: A sql case is executed behind an interface, by more than one executor

- **Date:** 2026-09-11
- **Status:** Accepted
- **Related:** ADR-001 Consequence 4 (external tools through a subprocess), ADR-004 §7-3 (why sql was
  not first), ADR-015 (where axis B ends and §6a begins), `design/module-sql.md`,
  `evidence/sql-baseline.md`

## Context

The runner for the `sql` family has two jobs that come apart cleanly: everything around a case —
stages, discovery, answer selection, comparison, the frozen output, slots — and **executing** a
case: sending its statements to CUBRID and rendering what comes back as text. This ADR is about the
second, because the answers pin it down more tightly than anything else in the suite.

### An answer is Java's rendering of a result

Read from the source (`CTP/sql/src/com/navercorp/cubridqa/cqt/`, `cubrid-testtools` develop `a1bec87`):

- Statements run over one JDBC connection that lives for the whole run: `CubridConnManager` caches it
  (`getDbmsConnection`, line 132) and nothing closes it. Before each case the reset scripts and
  `autocommit` are applied (`ConsoleBO.java:812-848`); cases run one after another (`ConsoleBO.java:482`).
- A result set is rendered as each column name followed by four spaces, then each row as each value
  followed by five, and every value is `getColumnValue(…, rs.getObject(i), …)` — `toString()` for
  everything but BIT, OID and collections (`ConsoleDAO.java:873-934`, `957-1019`).
- A case passes when its text equals the answer's with every `\r` and `\n` removed from both
  (`ConsoleBO.java:633-654`). No diff tool is involved.

So the 17,475 base answers in the sql corpus are, character for character, what JDK 8 and the CUBRID
JDBC driver print. How much of the corpus that shapes, counted by class of formatting
(`evidence/sql-baseline.md` §5):

| class | answers | share |
|---|---:|---:|
| `java.sql.Timestamp` fractions (`…:ss.0`) | 3,018 | 17.3% |
| floating point, plain or `E` notation | ~1,200 | ~7% |
| query plans | 966 | 5.5% |
| timezone-typed values | 713 | 4.1% |
| errors raised by the driver itself (`-21xxx`, `-10000`) | 104 | 0.6% |

Timestamps are simple to reproduce. The other four classes are not, and **2,796 answers (16.0%)**
fall into at least one of them.

### Imitating it has been tried

CTP's CCI mode (`sql_by_cci/ccqt`) renders the same results in C and imitates Java's number and date
formatting: `isdouble`, `formatdatetime`, `trimnumeric`, `trimdouble` and `trimfloat`
(`execute.c:266-510`), and a `.0` appended to anything that looks like a time (`execute.c:1535`, `1648`). It does not match, and the corpus shows what that cost:
**2,121 `.answer_cci` files** that exist only because the imitation and the original disagree.

### Go can reach CUBRID

Two Go drivers exist. `CUBRID/cubrid-go` is a cgo binding over CCI; it has not changed since 2022
and carries no licence. `cubrid-labs/cubrid-go` speaks the CAS protocol in pure Go (MIT, v0.2.1).
Neither is enough for a test runner as it stands. The second returns collections as strings, time
zones as UTC only and LOBs as raw bytes, and it has no call for query plans. Its `database/sql`
interface also hides holdcas and autocommit, which the suite changes from one statement to the next.

### And the goal is to compare, not only to reproduce

Running one corpus through the JDBC driver and through CCI or another native path, and comparing
what each returns, needs **both** paths behind one runner. Picking one would rule that out.

## Decision

**The runner owns everything around a case. Executing it is behind an interface, with two
implementations.**

| executor | what renders the text | the parity it is responsible for |
|---|---|---|
| **`jdbc`** | a small Java process that loads CQT's own `SQLParser` and `ConsoleDAO` from the CTP tree and is driven over a line protocol | JDBC mode (`sql_interface_type` unset or `jdbc`, the default) — **identical by construction**, because the rendering code is the same code |
| **`native`** | Go: a CAS client, and one formatter per mode | **CCI mode first** (`sql_interface_type=cci`), replacing `ccqt`. Rendering compatible with JDBC comes later, measured against `jdbc` |

The interface exchanges **text per statement**, not a verdict per case. That keeps the comparison —
strip CR and LF, then compare, as CQT does — in the runner, and gives two executors something finer
to be compared on than pass or fail.

**Tried the same day** (`evidence/sql-baseline.md` §9). A `jdbc` executor of about 100 lines, set up
through `ConsoleBO`'s own methods and driven case by case, wrote **975 of 975** medium `.result`
files and **1,658 of 1,658** from a sample of sql directories byte-identical to CTP's.

## Consequences

1. **Parity for JDBC mode does not wait on reproducing Java in Go.** The 2,796 answers whose
   rendering is hard to reproduce are the `native` executor's problem, not the gate's.
2. **`jdbc` is the oracle for `native`.** Every statement in the corpus becomes a test of the native
   formatter, and a difference names a statement rather than a case. That is the instrument the CCI
   imitation never had.
3. **CCI mode loses its C binary.** Its expected output is `ccqt`'s own rendering (C `printf` family,
   with `.answer_cci` overriding `.answer` where one exists), which is simpler to port than JDK 8's
   `Double.toString`, and it needs no JVM.
4. **Comparing executors is a new oracle.** Running the corpus through two drivers and diffing them
   finds driver bugs that no answer file encodes. Under ADR-015 that is §6a rather than axis B. It is
   recorded as a candidate and gets its number when it incubates; the interface is what makes it
   possible.
5. **Java source in a Go repository.** The `jdbc` executor is small and has one job. It is compiled
   when the run starts with the `javac` the machine check already requires, against the jars the CTP
   tree already ships. ADR-002 keeps `ctltool/Makefile` the same way.
6. **The CAS client is a design decision, not this one.** Whether to extend `cubrid-labs/cubrid-go`
   (MIT, so compatible) or write `internal/cas` is settled in `design/module-sql.md` after a spike.
   What it needs is fixed here: query plans, holdcas, several result sets per statement, OIDs and
   typed collections, and time zones that are not UTC.

## Alternatives considered

**Run CQT whole in each slot** (`ConsoleAgent runCQT` over a filtered list). It needs no new Java.
Rejected: the runner would see a case only after CQT had finished with it, so there are no per-case
timeouts, no live status and no per-statement text. It would also have to parse CQT's result tree
back.

**Go only, from the start.** Rejected for the gate, not for the project. The gate would wait on the
long tail — JDK 8's floating-point printing, the driver's time-zone `toString`, plans, driver error
codes — and there would be nothing to measure the Go rendering against except pass or fail.

**Java only.** Rejected. CCI mode would still need `ccqt`, the runner would stay tied to a JVM, and
there would be no second implementation to compare against.
