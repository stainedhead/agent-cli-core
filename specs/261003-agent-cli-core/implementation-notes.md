# Implementation Notes - agent-cli-core
Date: 2026-10-03
Purpose: record decisions and surprises during implementation. Update after each task.

## Technical Decisions
## Edge Cases and Solutions
## Deviations from Plan
## Lessons Learned

### httpx
- Decisions: `httpx.Clock` is declared locally (structurally equal to `internal/clock.Clock`) so consumers can name the type; default is `clock.System`. `MaxRetries` is retries after the first attempt (0 means 3, negative means none). Retry-After waits are `min(Retry-After, MaxWait)` without jitter; backoff is jittered. Vendor code comes from response headers only via `Config.VendorCode`, so the body is never read. 401 refresh is allowed for non-idempotent requests if the body is replayable.
- Assumption: persistent or non-retryable 429/502/503/504 and exhausted network errors become `*RateLimitedError` (exit 8), per the exit-code definition "rate limiting or transient failure persisted".
- Deviation: none from the task list; 404/409/500 pass through as responses.
