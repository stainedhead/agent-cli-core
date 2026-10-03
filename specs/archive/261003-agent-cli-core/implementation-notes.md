# Implementation Notes - agent-cli-core
Date: 2026-10-03
Purpose: record decisions and surprises during implementation. Update after each task.

## Technical Decisions

### selftest
- Probe returns (Outcome, error); outcomes are `allow`/`deny`. A probe error fails the row.
- The envelope failure form carries no data, so a failing matrix is a `general` failure whose message lists failing rows; passing results are a success envelope with the full `Result`.
- Skipped (read-only mode) rows never fail the run; an empty matrix passes. Invalid config (nil probe, bad Expect) is a usage error (exit 2) detected before any probe runs.
### docgen
- `Generate` returns `*docgen.Error` (validation category) for an invalid tree, so `output.ExitOf` gives exit 9.
- Exit code meanings live in a map keyed by `output.Category`; a test asserts every `output.Categories()` entry has a row, so adding a category to `output` cannot silently drift.
- Envelope and untrusted examples are marshaled from `output` at generation time, not hand-written.
- `Command` has an extra optional `Description` field beyond the data dictionary; the dictionary fields are unchanged.
- User text is single-lined or fenced with a fence longer than any backtick run, to block heading/front-matter injection.
## Edge Cases and Solutions
## Deviations from Plan
## Lessons Learned

### auth
- Own interfaces only (D4): `auth` imports neither `httpx` nor the daemon module. The adapter over the daemon's `pkg/client` is deferred until it has a tagged release; it will implement `auth.DaemonClient` and be the only importer. Its Go surface is unconfirmed (assumption); how it signals reauth/revoked/unreachable is mapped in the adapter.
- `Token` has no accessor; the value is read only by unexported `reveal` inside `Authorizer.Authorize`. `Format` makes every fmt verb redact. Custom `TokenSource` implementers build tokens with `NewToken`.
- `Authorizer` does not cache: the daemon owns caching. After `Refresh`, the next `Authorize` fetches the refreshed token. Forced refresh needs the source to implement `Refresher` (`DaemonTokenSource` does); otherwise `ErrRefreshUnsupported`.
- Remediation text is attached by `WithRemediation` on the token source, so the library message stays generic and each tool adds its own command.
- Daemon errors other than reauth/revoked/unreachable become `*TokenError` (category auth, exit 3, message scrubbed by `internal/redact`): a token that cannot be obtained is an auth failure and nothing else is tried.
- `authtest.Fake` also serves a fake resource server (`Handler`) so 401 scenarios are testable without httpx; the 401-refresh-retry policy itself belongs to `httpx`. `UnreachableError` text contains the daemon service name as a string only.
- Assumption: the 401 scenarios model the daemon PRD's refresh endpoint semantics; not verified against a real daemon.
### policy
- policy imports nothing internal (archtest), so it declares its own one-method `Clock` instead of importing `internal/clock`; `clock.System` and `clock.Fake` satisfy it structurally.
- Errors cannot implement `output.CategoryError` (that needs `output.Category`); the caller maps `Decision`/`*InvalidError` to categories. Invalid policy is documented as category `validation`.
- `dry_run_only` decisions have `Allowed=false` so that callers checking only `Allowed` fail safe; `DryRunOnly()` identifies them. Only `allow` decisions consume rate budget.
- Deny rules win regardless of order; unmatched requests are default-denied (`default-deny`).
- Writable check uses `syscall.Access(W_OK)` on the file and its directory (build tag `unix`); default is warn, refuse is opt-in.

### audit
- Record carries `duration` as a Go duration string; encoding via explicit wire struct fixes column order for the golden test.
- Write mode is a Logger setting (warn|block), not per-verb: the library has no read/write classification. Tools wanting the PRD's "block write operations" default use two Loggers or choose per call. Assumption.
- Fields capped at 512 bytes and redacted so free text cannot smuggle bodies or tokens.
- Existing log files wider than 0600 are chmod-ed to 0600 on Open.
### httpx
- Decisions: `httpx.Clock` is declared locally (structurally equal to `internal/clock.Clock`) so consumers can name the type; default is `clock.System`. `MaxRetries` is retries after the first attempt (0 means 3, negative means none). Retry-After waits are `min(Retry-After, MaxWait)` without jitter; backoff is jittered. Vendor code comes from response headers only via `Config.VendorCode`, so the body is never read. 401 refresh is allowed for non-idempotent requests if the body is replayable.
- Assumption: persistent or non-retryable 429/502/503/504 and exhausted network errors become `*RateLimitedError` (exit 8), per the exit-code definition "rate limiting or transient failure persisted".
- Deviation: none from the task list; 404/409/500 pass through as responses.

### integration (WS-I)
- No bridge code: `*auth.Authorizer` has exactly the `httpx.TokenRefresher` methods; `internal/integration` asserts it at compile time.
- Bug found by the leak fuzz test and fixed in `httpx`: a server echoing the credential it was sent into a non-sensitive header (for example a vendor-code header) reached the trace and `ForbiddenError.VendorCode`, because the redactor only knew patterns. The transport now also scrubs the exact Authorization values it attached during that call.
- Boundary documented, not fixed: data a server returns in a 200 body is passed through as the tool's data (marked untrusted by the tool); a daemon that puts a token in its own error text is only scrubbed when the token has a recognizable shape (Bearer, JWT, long opaque). Tokens shorter than 6 bytes are not value-scrubbed.
- CI actions are pinned to release tags, not commit SHAs (SHA lookup needs network).
