# Implementation Notes - agent-cli-core
Date: 2026-10-03
Purpose: record decisions and surprises during implementation. Update after each task.

## Technical Decisions

### selftest
- Probe returns (Outcome, error); outcomes are `allow`/`deny`. A probe error fails the row.
- The envelope failure form carries no data, so a failing matrix is a `general` failure whose message lists failing rows; passing results are a success envelope with the full `Result`.
- Skipped (read-only mode) rows never fail the run; an empty matrix passes. Invalid config (nil probe, bad Expect) is a usage error (exit 2) detected before any probe runs.
## Edge Cases and Solutions
## Deviations from Plan
## Lessons Learned
