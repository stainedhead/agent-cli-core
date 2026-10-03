# Data Dictionary - agent-cli-core
Purpose: names and shapes of exported types. Proposed; each workstream finalizes its own section only.

## output (WS0)
- `Envelope` {OK bool, Data any, Meta *Meta, Error *Error}; `Meta` {Truncated bool, NextOffset *int, Count int, RequestID string}; `Error` {Code, Message, Hint string}
- `ExitCode` int constants 0..9 (OK, General, Usage, Auth, Forbidden, NotFound, PolicyDenied, Conflict, RateLimited, Validation)
- `Category` string enum (ok, general, usage, auth, forbidden, not_found, policy_denied, conflict, rate_limited, validation); `ExitFor(Category) ExitCode`; `CategoryError` interface (`Category() Category`) so other packages' errors map without importing each other
- `Untrusted` wrapper {Value, Author, Timestamp}; `Format` enum (json, table, text); `Bounds` {MaxBytes default 32768, Offset}
## internal/redact, internal/clock (WS0)
- `redact.Redactor` (header/string/body scrubbing); `clock.Clock` {Now, Sleep/After}, `clock.Fake`
## auth (WS-A)
- `Token` (redacting String/GoString/Format -> `[redacted]`), `TokenSource` {Token(ctx) (Token, error)}, `DaemonClient` {Fetch(ctx, provider), Refresh(ctx, provider)}, errors `ErrReauthRequired`, `ErrRevoked`, `*UnreachableError{Socket}`; `auth/authtest.Fake`
## policy (WS-B)
- `Policy`, `Rule`, `Request` {Verb, Resource, Fields}, `Decision` {Allowed, Mode (allow|dry_run_only|deny), RuleID, Reason}, `Limits`, `Parse`/`Load` (strict)
## audit (WS-C)
- `Record` (schema_version, ts, tool, agent_id, run_id, verb, resource, outcome, http_status, duration, policy_decision), `Logger`, `WriteFailureMode`
## httpx (WS-D)
- `Client`/`Transport` config {MaxRetries, MaxWait, Jitter, Clock}, `TokenRefresher` {Authorize(ctx, *http.Request) error; Refresh(ctx) error}, trace options, `MarkSafeToRetry`, `*RateLimitedError`, `*ForbiddenError{VendorCode}` (implement `output.CategoryError`)
## selftest (WS-E)
- `Row` {Name, Verb, Resource, Expect, ReadOnly}, `Probe`, `Runner`, `Result`
## docgen (WS-F)
- `CommandTree` {Name, Description, Commands[]{Name, Usage, Examples, Forbidden}}, `Generate(tree) ([]byte, error)`
