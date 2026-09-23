# Extensions — §6a, beyond the strangler fig

*English · [한국어](README.ko.md)*

The functional requirements collected under ROADMAP §6a, "the extension area". Each entry is
**additive work outside the strangler fig**, and so sits *outside* the NG1 · NG2 · NG4 freeze, with
a lot of room for new decisions.

**This directory holds the case and the agenda, not the decision** — an item formally enters
*incubating* only through an ADR-EXT-NNN.

---

## The catalogue

| ID | Axis | Name | Entry | Prerequisite | requirements |
|---|---|---|---|---|---|
| E1 | 1 | Adopt sqllogictest | candidate now | — | [E1-sqllogictest/](E1-sqllogictest/requirements.md) |
| E2 | 2 | Random SQL fuzzing (SQLsmith) | candidate now | — | [E2-sqlsmith/](E2-sqlsmith/requirements.md) |
| E3 | 3 | Logic bug detection (SQLancer NoREC+TLP) | **in progress, on its own track** | — | [E3-sqlancer/](E3-sqlancer/requirements.md) · [ADR-EXT-003](../../project/adr/ADR-EXT-003-sqlancer-cubrid.md) · repository `cubrid-sqlancer` |
| E4 | 4 | Distributed isolation testing (AWDIT/Jepsen) | conditional | N24 / N11 graduation | [E4-distributed-isolation/](E4-distributed-isolation/requirements.md) |
| E5 | 5 | Parser/protocol fuzzing harness (libFuzzer) | conditional | `-DENABLE_FUZZING` in the cubrid repository | [E5-parser-fuzzing/](E5-parser-fuzzing/requirements.md) |
| E6 | 6 | Differential testing (against PostgreSQL) | conditional | N13 pg-wire-compat at *selected* or beyond | [E6-differential/](E6-differential/requirements.md) |
| E7 | 7 | Stateful / randomised workload | conditional | C-004, the responsibility boundary, defined | [E7-workload/](E7-workload/requirements.md) |
| (E8) | 8 | Hybrid CI integration (the Materialize pattern) | meta | two or more of E2–E7 · E9 adopted | (TBD — not a catalogue entry) |
| E9 | 5 extended × 8 | Storage-engine **concurrency** fuzzing (schedule × interleaving) | conditional | **E5 first**, plus in-process SERVER_MODE startup and **E10** | [E9-storage-fuzzing/](E9-storage-fuzzing/requirements.md) |
| E10 | supporting | Producing and keeping XASL fixtures, version identification included | candidate now | — (no engine change) | [E10-xasl-fixtures/](E10-xasl-fixtures/requirements.md) |

**A note on the numbering.** `E8` is **reserved** for axis 8, *Hybrid CI integration*, as a meta
entry. E9 skipping over E8 is that reservation, not a gap.

---

## The one that has no E number — `extensions/cluster-sandbox`

It is a submodule under `extensions/`, and it is **not a catalogue entry.** E1–E10 are all *testing
capabilities* — a new oracle, a new case format, a new generator — and `cubrid-cluster-sandbox` is
an **environment provider.** It changes where a test runs, not what is verified.

Giving it an E number would break two things. The §6a ladder that decides what to start next would
acquire an "infrastructure you have to do first" competing with real entries, and the fact that it
is a **shared dependency of several of them** would vanish from the table. The roadmap already
records that dependency in three places — the boundary of E4 (distributed isolation), the boundary
of E7 (workload), and ladder rank 6 (recovery/crash).

And before those there are two more dependencies on the strangler-fig side. The `ha_repl` task and
the HA shell suite have never run here for want of a master/slave topology — ADR-013 excludes the
HA tree's 367 cases from the evidence for exactly that reason — and ADR-014 says the runner **must
not build one**:

> **HA is not an exception.** The system under test has a topology; the runner does
> not have a fleet.

So using `cluster-sandbox` is not a way around ADR-014 but **the way of keeping it.** Standing the
topology up belongs over there; running cases on top of it belongs here.

| | |
|---|---|
| Repository | `cubrid-systems/cubrid-cluster-sandbox` (public) |
| Location | `extensions/cluster-sandbox` — a submodule; `bot/bump-cluster-sandbox` moves the pointer |
| Form of integration | subprocess plus the `--json` artifact (ADR-001 Consequence 4). Nothing is linked |
| The code on this side | `internal/sandbox` — one Channel and one topology provider |
| Recorded in | [ADR-022](../../project/adr/ADR-022-topology-provider.md) — accepted 2026-09-20 |

**This location is a pin, not ownership.** Nothing here compiles against `csb`; the submodule exists
so that evidence produced in this repository can name which revision of the provisioner stood the
topology up, which is why ADR-022 rejected finding `csb` on `PATH` — *"a tool found on PATH names
nothing."* The pin therefore belongs beside the evidence, and the evidence is here. When the axis O
layer arrives it will pin both this repository and `cluster-sandbox` for the same reason, and **this
pin stays**: using testkit without that layer does not stop being a way to use it
([`design/slots-and-the-layer-above.md`](../../project/design/slots-and-the-layer-above.md) §8).

**This table does not decide what to start.** The single source for the priority of the fuzzing
family (E3 · E5 · E9) and of the two unregistered candidates is the **§6a ladder** in the ROADMAP.

---

## What each entry is made of

It follows the five-document pattern of `project/analysis/{module}/` — except that at the
*incubating* stage the detail is a stub:

```
extensions/E{N}-{name}/
├── requirements.md        the problem, what users need, non-functional
│                          requirements, the conditions for entering incubating   (FULL)
├── design.md              architecture / where the module sits / data flow       (STUB — filled in after ADR-EXT-NNN)
├── io-contract.md         CLI / conf / output format / exit codes                (STUB — filled in after ADR-EXT-NNN)
└── test-corpus.md         where the input corpus comes from, its licence,
                           how it is kept                                         (STUB — filled in after ADR-EXT-NNN)
```

`implementation-notes.md` is added *once implementation has moved*, and is absent while an entry is
incubating.

---

## The ADR-EXT placeholder index

| ADR | Trigger | What it decides |
|---|---|---|
| ADR-EXT-001 | E1 formally enters incubating | which sqllogictest spec variant, the corpus import policy, the result comparison mode, the SUT client |
| ADR-EXT-002 | E2 formally enters incubating | reuse or reimplement SQLsmith, how far the dialect is extended, where the corpus lives, which channel decides a crash |
| ADR-EXT-003 | ~~trigger~~ **Accepted 2026-09-02** | NoREC first, a separate repository (ServiceLoader SPI), reuse, and the corpus outside testcases |
| ADR-EXT-004 | E4 formally enters incubating | AWDIT or Jepsen first, topology automation, the fault injection channel, the corpus |
| ADR-EXT-005 | E5 formally enters incubating | the fuzz target build option (in the cubrid repository), the fuzzer itself, the corpus, the responsibility boundary |
| ADR-EXT-006 | E6 formally enters incubating | which peer DBMS, the mode (canonical vs rewrite), the dialect rewrite catalogue, the corpus |
| ADR-EXT-007 | E7 formally enters incubating | the first scenarios, the invariant catalogue, the engine/suite responsibility boundary, the corpus |
| (ADR-EXT-008) | E8 (Hybrid CI) formally enters | *reserved* — the axis 8 meta slot |
| ADR-EXT-009 | E9 formally enters incubating | the input IR, **how a schedule is expressed**, the cap on participants, where the corpus lives, the boundary in the cubrid repository (the rendezvous handler) |
| ADR-EXT-010 | E10 formally enters incubating | the production path (csql/CCI/JDBC), the fixture format, how a version is identified, where fixtures are kept |

---

## Priority, as the survey concluded

1. **Candidates now, able to run *alongside* strangler-fig phases 3 and 4:** E2 (SQLsmith),
   E3 (SQLancer NoREC+TLP)
   - cheap to adopt, no dependencies, and they fill an area *testkit is empty in today*
   - the *de facto* practice of the PostgreSQL ecosystem
2. **Conditional, once their prerequisite is met:** E5 (a PR to the cubrid repository),
   **E9 (E5 and E10 first)**, E6 (N13 at *selected*), E4 (HA graduation), E7 (C-004 defined)
   - **E10 is a candidate now** — it needs no engine change and has no prerequisite. It is a
     prerequisite of E9 Tier 2 and can be run independently
   - E5 → E9 are *one strand sharing one set of infrastructure*. Reversing the order means building
     it twice (ROADMAP §8, risk)
3. **Tracked for later:** SQLancer++ (adaptive grammar), the FoundationDB simulation concept, §6a
   ladder ranks 6 and 7 (a recovery/crash framework, and concurrency schedule fuzzing — not
   registered)

---

## Shared notes on risk and consistency

- **NG1 (the testcases repository is frozen)** — putting a fuzz, mismatch, crash or violation corpus
  into testcases violates it. *External storage is recommended* (see §6 of each entry)
- **E9's protobuf is not a protocol** — it has nothing to do with CUBRID's own binary protocol.
  protobuf there is the *fuzzer's internal input IR* and is linked only into the fuzz binary
  (E9 requirements §2). If this misunderstanding keeps recurring, the entry itself may be rejected
  for the wrong reason
- **NG2 (the external surface is frozen)** — every §6a entry is a *new entry point*, so there is no
  conflict
- **NG4 (no compatibility with non-CUBRID DBMSs)** — in every §6a entry *CUBRID is the SUT*, so
  there is no conflict
- **The branch gate, §7** — *the strangler fig comes first* (ROADMAP §8). §6a progress is recorded
  on a separate line
- **The case-format ingestion interface** — phase 2's `project/design/contracts.md` allows for
  hybrid composition, shared by E1–E7. **E9 is the exception**: it does not go through a case format
  and calls the internal API directly

---

## Sources

- `../../project/survey/dbms-testing-ecosystem.md` — the eight-axis classification, the catalogue of
  tools and research, and how §6a-E2–E7 and E9 were arrived at
- `../../project/ROADMAP.md` §6a — the catalogue, phase alignment, open questions, the ADR-EXT
  placeholders
- `../../project/ROADMAP.md` §6a appendix — the **fuzzing priority ladder**, which is the single
  source for what to start first
- `../../project/analysis/{module}/` — the phase 0 output for the modules the strangler fig targets
  (background)
