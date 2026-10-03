# Implementation Notes - agent-cli-core
Date: 2026-10-03
Purpose: record decisions and surprises during implementation. Update after each task.

## Technical Decisions
## Edge Cases and Solutions
## Deviations from Plan
## Lessons Learned

### policy
- policy imports nothing internal (archtest), so it declares its own one-method `Clock` instead of importing `internal/clock`; `clock.System` and `clock.Fake` satisfy it structurally.
- Errors cannot implement `output.CategoryError` (that needs `output.Category`); the caller maps `Decision`/`*InvalidError` to categories. Invalid policy is documented as category `validation`.
- `dry_run_only` decisions have `Allowed=false` so that callers checking only `Allowed` fail safe; `DryRunOnly()` identifies them. Only `allow` decisions consume rate budget.
- Deny rules win regardless of order; unmatched requests are default-denied (`default-deny`).
- Writable check uses `syscall.Access(W_OK)` on the file and its directory (build tag `unix`); default is warn, refuse is opt-in.

