# E2 — Random SQL Fuzzing (the SQLsmith port) (Requirements)

*English · [한국어](requirements.ko.md)*

**Source:** survey/dbms-testing-ecosystem.md §4 + §11 (a new candidate for the ROADMAP §6a catalogue)
**Status:** incubating (before formal entry — the ADR-EXT-002 slot)
**Axis mapping:** axis 2 (Random SQL generation)
**Companion docs (to follow):** `design.md`, `io-contract.md`, `test-corpus.md`, `implementation-notes.md`

---

## 1. The problem this extension solves

It verifies the **robustness of CUBRID's parser, planner and executor** with *random SQL that is
valid but wide-ranging*.

The existing testkit (sql / medium / shell / isolation) *has no random generation axis*. Only the
cases a case author wrote by hand are regressed, which leaves these areas blind:
- parser crash / UB
- planner assert / stack overflow
- executor segfault / OOM
- internal state corruption in deeply nested combinations of expression / window / lateral /
  subquery

SQLsmith is the source of *100+ bugs* in the PostgreSQL ecosystem. How it works:
1. introspect the target DB's schema
2. generate a type-correct random AST (passing the parser = valid SQL)
3. nested expression / window / lateral / subquery to an arbitrary depth
4. judge on crash / signal / process exit alone (no oracle)

**The kind of bug it catches:** crash, assert, UB. *It cannot catch wrong results* (that area is
§6a-E3).

---

## 2. How it is called from outside (proposed — incubating)

```
ctp.sh sqlsmith [-c <sqlsmith.conf>]
   or
testkit run sqlsmith [-c <conf>] [--seed <N>] [--time <sec>] [--max-depth <D>]
```

The internal entry point (agenda):
```
SqlsmithDriver.exec(config)
  ├─ schema introspect (CUBRID dialect — information_schema or db_class/db_attribute)
  ├─ AST generator loop:
  │    while time_left:
  │      query = generate(seed, depth)
  │      run(query) → {ok | empty | error | CRASH}
  │      if CRASH: corpus.save(query, seed, stack)
  └─ report: { runs, crashes, top stack hashes }
```

**The channel that drives the SUT:** JDBC or CCI (sharing the case-format ingestion interface).
**Effect on the external surface freeze:** none (a new entry point).

---

## 3. What users need (an incubating estimate)

1. **CUBRID schema introspection** — replacing SQLsmith's dependence on PostgreSQL's
   information_schema with CUBRID's system catalog: db_class, db_attribute, db_index and the rest
2. **An option to generate CUBRID's extended SQL** — object-oriented constructs (path expression,
   class hierarchy), serial, hierarchical query (CONNECT BY), method and so on: *the CUBRID dialect
   added on*
3. **crash detection** — process signal (SIGSEGV/SIGABRT) + the cubrid server connection dropping +
   a core file turning up
4. **Accumulating a corpus** — keeping the query that caused a crash as a *reproducible seed plus
   normalised query text*
5. **stack hash dedup** — several crashes with the same stack collapsed into one entry
6. **A time / iteration / depth budget** — fuzzing within a fixed time inside CI
7. **A continue mode** — re-running the corpus's past crash queries against a new build (the
   regression seed function)

---

## 4. Non-functional requirements

| Item | Agenda | What it means in the new system |
|------|------|---------------------|
| Cost of adoption | low (only the schema introspection path is made CUBRID's) | the survey §4 conclusion — a candidate now |
| Immediate ROI | ★★★★ | it fills directly an area testkit is empty in |
| Relation to §6a-E3 (SQLancer) | complementary (SQLancer for wrong results, SQLsmith for crashes) | adopted together when hybrid CI (E8) comes |
| AST nesting depth | SQLsmith's strength (deep) | *an AST deeper* than SQLancer's is the core asset |
| Licence | SQLsmith is custom — the vendoring policy needs checking | the same pattern as ROADMAP §8 risk 6 |
| The testcases repository freeze (NG1) | no conflict (the corpus is kept outside) | but an ADR for where the corpus lives is needed |

---

## 5. External resources it depends on

- **A CUBRID client** — the channel that drives the SUT, JDBC or CCI
- **The CUBRID system catalog** — db_class / db_attribute / db_serial / db_method and the rest
- **SQLsmith itself** — github.com/anse1/sqlsmith (C++), or *a reimplementation* (subordinate to
  ADR-001's implementation language decision)
- **Core dump infrastructure** — it needs to *share the core policy* with the sql and isolation
  modules (bound up with the ADR on handling core files in analysis/sql/requirements.md §4)
- **Fuzz corpus storage** — inside testkit or separate storage (an input to ADR-EXT-002)

---

## 6. The conditions for entering incubating

ADR-EXT-002 can be written once these are decided (owner: hgryoo):

1. **Reuse vs reimplement** — whether to use the SQLsmith C++ body as a *subprocess*, or to
   *reimplement* it in the language ADR-001 decides
2. **How far the dialect is extended** — how much of CUBRID's extended SQL (path expression /
   serial / connect-by / method) goes into the random AST
3. **Where the fuzz corpus lives** — inside testkit / in the cubrid repository / separate storage.
   Check whether it violates the testcases repository freeze (NG1)
4. **Which channel decides a crash** — signal vs core file vs cubrid server log — settle on a single
   channel
5. **The CI integration mode** — a short fuzz on every PR vs a nightly long fuzz vs a lane of
   its own
6. **Sharing the dialect adapter with §6a-E3 (SQLancer)** — bound up with survey Open Question 3
   (where the dialect adapter sits)
7. **Checking for a conflict with the strangler fig's branch gate, §7** — where it competes for
   resources with the first replacement of phases 3 and 4, the strangler fig comes first

**ADR placeholder:**
- ADR-EXT-002 — reuse or reimplement SQLsmith, how far the dialect is extended, where the corpus
  lives, which channel decides a crash

---

## 7. Notes on risk and consistency

- **Possible conflict with NG1** — putting the fuzz corpus into a testcases repository violates the
  freeze. *External corpus storage* is the safe course
- **No conflict with NG2** — a new entry point
- **No conflict with NG4** — CUBRID is the SUT
- **§6a-E3 (SQLancer) together with E2 (SQLsmith)** — survey §4.4 / §11: the *de facto* practice of
  the PostgreSQL ecosystem. Adopting them together is recommended
- **Complementary to §6a-E5 (parser fuzzing)** — SQLsmith is *random inside the grammar*; libFuzzer
  is *byte-level outside the grammar*. Both catch parser crashes, but over different areas
