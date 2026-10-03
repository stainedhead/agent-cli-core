# Implementation Notes - agent-cli-core
Date: 2026-10-03
Purpose: record decisions and surprises during implementation. Update after each task.

## Technical Decisions
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
