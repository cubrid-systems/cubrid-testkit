# ADR-000: Repository Name

- **Status:** Accepted
- **Date:** 2026-04-28
- **Decision:** The new repository is named `cubrid-testkit`.

## Decision Drivers
- Need a name that conveys scope beyond a single test runner (analysis, execution, reporting, generation)
- Break from the "CTP" acronym to signal a fresh identity
- Maintain "cubrid-" prefix for ecosystem membership

## Considered Options
1. **cubrid-testkit** -- "kit" encompasses the full toolset beyond just running tests
2. **cubrid-ctp-next** -- preserves legacy naming
3. **cubrid-testrunner** -- describes only one capability

## Decision
Option 1: `cubrid-testkit`

## Why
- "kit" accurately reflects the broader scope (analysis + execution + reporting + generation)
- Clean break from legacy "CTP" branding while retaining "cubrid-" ecosystem prefix
- Does not lock the project into a single function ("runner") or a legacy identity ("ctp")

## Rejected Alternatives
- `cubrid-ctp-next`: Permanently embeds the legacy acronym; new system should stand on its own identity
- `cubrid-testrunner`: Narrows the perceived scope to execution only; the new system includes analysis, design artifacts, and documentation

## Consequences
- All new development happens in `cubrid-testkit/`
- `cubrid-testtools/` remains operational during the strangler-fig migration (Phase 0-4)
- External references to "CTP" in documentation will be gradually updated
- Analysis baseline: cubrid-testtools @ 86992c1b334d55800f2700d60f9809c2ceca268d

## Follow-ups
- ADR-001: Implementation language decision (triggered at Phase 0 M0 exit)
- ADR-002: Build tool decision (triggered at Phase 0 M0 exit)
