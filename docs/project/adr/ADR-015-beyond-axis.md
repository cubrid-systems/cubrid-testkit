# ADR-015: Axis B — what the system should do that CTP does not

- **Date:** 2026-09-03
- **Status:** Accepted
- **Related:** `concept/migration-exclusions.md` (the T/O split), ADR-003 (the freeze),
  ADR-013 (how equivalence is proven), ADR-014 (one machine), `extensions/README.md` (§6a)

## Context

The project has had two axes.

**Axis T** is test execution and **axis O** is QA operations, and the split decides what gets
migrated. But both share something the split does not name: they are **rewrites that preserve
compatibility**. A T item is done when the frozen surface still holds and the regression diff is
zero. An O item is done when the operations layer does what CTP's did. Better performance and a
better structure are expected — required, even — but *the observable contract does not move*.

That is the right discipline and it is also a ceiling. Everything that would make this system better
than CTP, rather than equal to it, has nowhere to live. In practice such ideas end up in one of two
bad places:

- **Smuggled into T.** Then the regression diff stops being readable. The whole value of
  normalise-then-diff-zero is that a difference means a mistake; the moment deliberate improvements
  are mixed in, every difference needs an argument, and the evidence turns back into prose.
- **Deferred to O.** Then they wait for a layer that does not exist yet, and by the time it does
  nobody remembers why the idea mattered.

Both failures are silent. An idea that is smuggled looks like a bug; an idea that is deferred looks
like it was never had.

## Decision

**A third axis, B — beyond.** Work that has no CTP counterpart to be compatible with, and that makes
the system better at what it does rather than equal to what CTP did.

| Axis | | Done when |
|---|---|---|
| **T** | test execution | the frozen surface holds and the regression diff is zero |
| **O** | QA operations | the operations layer does what CTP's did |
| **B** | **beyond** | **its own evidence says so — because there is nothing to diff against** |

B applies to both of the others: a B item improves on a specific T item or a specific O item, and
says which.

The three kinds are capability, performance and usability:

- **capability** — the system finds something it could not find before, or answers a question it
  could not answer
- **performance** — the same work, measurably faster or cheaper
- **usability** — the same result, reached with less of the operator's attention

### Admission criteria

Four, and all four. Without them this is a wish list with a letter in front of it.

1. **It names what it beats.** An entry points at the T or O item it improves on and states how
   today is measured. "Faster" with no baseline is not an entry; "217 cases take four hours
   serially, because a case restores the whole install before it runs" is.
2. **The parity ships first.** Nothing on B ships before the T or O item it improves on has passed
   its own gate. This is not process for its own sake: equivalence cannot be proven against a
   system that has already been improved, so improving first destroys the evidence that the rewrite
   was faithful.
3. **It says in advance what would show it worked.** T and O prove themselves by diffing against
   CTP. B has nothing to diff against, so the evidence has to be named before the work starts — a
   number with a baseline, a task someone completes in less time, a class of failure that gets
   caught that did not before. An entry with no stated evidence is not admitted.
4. **It can be turned off.** A B item that cannot be disabled becomes part of the frozen surface by
   accident, and the next person to prove equivalence inherits it as an obligation.

### Where B ends and §6a begins

The §6a extensions (E1–E9: SQLancer, fuzzing, workload generation, and the rest) are **new kinds of
testing** — new oracles, new inputs, new ways to decide something is wrong. B is about **the system
that runs the tests** being better at running them.

E changes what can be found. B changes how well it is found.

An entry that adds an oracle belongs in §6a with its own ADR-EXT number. An entry that makes the
existing oracles run in a quarter of the time belongs here.

## Consequences

1. **`concept/beyond-axis.md` is the register**, the way `migration-exclusions.md` is the register
   for what axis O excluded. An entry that is not written down there is not on the axis.
2. **A B item may not be used as a reason to delay a T or O item**, and criterion 2 is what enforces
   it. The register records ideas so they can wait without being lost, which is the opposite of a
   backlog that grows until it blocks.
3. **Some B work has already happened, inside T rewrites, because parity was impossible.** Sorting
   the dispatch list is the clearest case: `find` order made CTP's own file irreproducible, F1 was
   not a grade anything could earn, and the deviation was recorded rather than avoided
   (`evidence/spec-corrections.md`). Those are not violations of criterion 2 — they are places where
   there was no parity to ship first. Each one is already recorded as a deviation, and the register
   cross-references them rather than re-deciding them.
4. **The freeze gets a fourth grade in practice, though not in name.** F1/F2/F3/NF describe how
   closely a surface is preserved. A B item adds a surface that CTP did not have, which no grade
   covers. It is graded NF by construction and its own contract is stated in the register entry.

## Alternatives considered

**Keep two axes and put improvements in O.** O is already "rebuilt later as a separate layer", so
this amounts to deferring everything. Rejected: most of the improvements found so far are about the
runner itself — case-level parallelism, structured verdicts, flake classification — and none of them
is an operations concern.

**Keep two axes and allow improvements inside T, recorded as deviations.** This is what has been
happening, and it works only because the deviations were forced and few. Rejected as a general
policy: it makes the regression evidence carry an argument for every difference, and an evidence
artifact that requires argument is prose.

**Fold it into §6a.** The extension space already exists and has its own numbering. Rejected: §6a is
about new testing techniques, and its entries incubate against triggers of their own. "Run the same
suite four times faster" is not a testing technique and would sit in that space badly.
